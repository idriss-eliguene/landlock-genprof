# Policy synthesis — `internal/policy.Synthesize`

This document explains the design decisions behind `Synthesize()`
(milestone M2). The code itself documents the *what* in comments; this
file documents the *why*, for whoever needs to modify the algorithm
without re-reading the whole design history.

## The problem

Input: a raw `[]tracer.Event` (one syscall access = one event, potentially
hundreds per training run). Output: a `[]Rule`, one per directory, with an
access category and a confidence level — in a format consumable by
`pkg/podlock.BinaryProfile`.

The risk to avoid: one rule per individual file. That would produce
unreadable profiles (hundreds of entries) overfitted to the exact training
run rather than generalizable to normal application use.

## Aggregation by directory, capped at 3 segments

`aggregationDir()` doesn't just take `filepath.Dir(path)` — it truncates
the result to `maxAggregationDepth = 3` segments from the root. Without
this truncation, two files in different subdirectories of the same
project would produce two distinct rules:

```
/usr/share/nginx/html/index.html   → filepath.Dir alone: /usr/share/nginx/html
/usr/share/nginx/css/style.css     → filepath.Dir alone: /usr/share/nginx/css
```

whereas the reference example (README §8) expects a single rule
`/usr/share/nginx` for both. Depth 3 is an empirical choice calibrated on
that example — not a constant derived from some filesystem property. If
generated profiles ever turn out too broad (lumping together subdirectories
that should be distinguished) or too narrow, this is the parameter to
revisit first, not the algorithm around it.

## Directory opens vs file opens: why `aggregationDir` needs `isDir`

Found the hard way, on the very first end-to-end run against a live
cluster (`kubectl exec nginx-demo -- ls /etc`): `aggregationDir` used to
always take `filepath.Dir(path)`, assuming every observed path was a
*file*. But `ls /etc` calls `openat("/etc", O_DIRECTORY, ...)` to list
`/etc` itself — `/etc` here is the thing being opened, not a file inside
it. Taking its parent produced `filepath.Dir("/etc")` = `/`, i.e. a
`readOnly: [/]` rule — read access to the entire filesystem, which
defeats the whole point of generating a Landlock policy.

`tracer.Event.IsDir` (set from the `O_DIRECTORY` bit in the raw openat
flags, see `trace_linux.go`) fixes this: when true, `aggregationDir` uses
the path itself as the rule target instead of its parent. `/etc` opened
directly becomes the rule `/etc`, not `/`.

A related bug surfaced in the same run: some observed opens carried a
**relative** path (no leading `/`) — likely some process in the container
opening a file relative to its own working directory, which we don't
track. `filepath.Dir("nginx.conf")` returns `"."`, which used to become a
nonsensical `/.` rule. `Synthesize` now skips any event whose `Path`
doesn't start with `/` — a relative reference has no single absolute
filesystem location to turn into a rule without knowing the emitting
process's cwd, so guessing would be worse than dropping it.

Neither of these was caught by unit tests with hand-crafted events,
because the hand-crafted events never included a directory-open or a
relative path — only real captured data exposed them. See
`TestSynthesize_DirectoryOpenIsNotItsOwnParent` and
`TestSynthesize_IgnoresRelativePaths` in `synthesize_test.go`.

## Categorization: why `write` takes priority over `read`

```go
switch {
case acc.write:
    access = append(access, "readWrite")
case acc.read:
    access = append(access, "readOnly")
}
```

`readWrite` is treated as a superset of `readOnly`, not a separate
category to stack on top. A directory where at least one write was seen
is classified entirely as `readWrite`, never `readOnly` + `readWrite` at
the same time. `readExec`, on the other hand, is independent and can
combine with either — a directory can legitimately contain both an
executed binary and a config file that's read.

## Why network events (`connect`/`bind`) are ignored

`Synthesize` filters out any event with no `Path` (`ev.Path == ""`).
That's not an oversight: `pkg/podlock.BinaryProfile` (see
`pkg/podlock/types.go`) only has `ReadExec`/`ReadOnly`/`ReadWrite` — no
field to represent Landlock network rights
(`LANDLOCK_ACCESS_NET_BIND_TCP` / `LANDLOCK_ACCESS_NET_CONNECT_TCP`).
Generating a `Rule` for a network event would produce data that could
never be serialized in the output. As long as the PodLock schema doesn't
cover networking, these events have nowhere to land.

**Known limitation:** if `pkg/podlock.BinaryProfile` ever gains a network
field, this filter will need to be removed and an equivalent aggregation
(by port? by range?) added to `dirAccess`.

## Confidence: a deliberately provisional heuristic

```go
func confidenceFor(seenCount int) Confidence {
    switch {
    case seenCount >= 3: return ConfidenceHigh
    case seenCount == 2: return ConfidenceMedium
    default:             return ConfidenceLow
    }
}
```

The official definition of `Confidence` (see the type's comment, and
`docs/threat-model.md` §2) is "seen across how many distinct **training
runs**" — the README example literally says *"seen on every run"* vs
*"seen once out of 5 runs"*. What `confidenceFor` computes today is
different: the number of events aggregated within a **single**
`Synthesize` call, so within a **single** run.

It's a reasonable proxy (a directory hit 3 times within one run is
statistically more likely to be a stable path), but **it is not the real
measure**. The real measure requires persisting state across multiple
`Synthesize` calls (one per run), which isn't wired up — see roadmap M5.
Don't present current `Confidence` values as reliable in the threat-model
sense until that limitation is lifted.

## Determinism

The keys of `map[string]*dirAccess` are sorted (`sort.Strings`) before
building the final `[]Rule`. Without this sort, a Go map's iteration order
isn't guaranteed stable from one run to the next — two calls to
`Synthesize` on the same data could produce a `[]Rule` in a different
order, breaking tests and making generated YAML diffs unreadable in review.

## See also

- `internal/policy/synthesize.go` — the implementation
- `internal/policy/synthesize_test.go` — test cases (aggregation by
  directory, mocked nginx events, empty input)
- [`docs/architecture.md`](architecture.md) — where `Synthesize` sits in
  the full pipeline
- [`docs/threat-model.md`](threat-model.md) §2 — multi-run validation
  methodology (not yet implemented)
