# Policy synthesis — `internal/policy.Synthesize`

This document explains the design decisions behind `Synthesize()`
(milestone M2). The code itself documents the *what* in comments; this
file documents the *why*, for whoever needs to modify the algorithm
without re-reading the whole design history.

## The problem

Input: a raw `[]tracer.Event` (one syscall access = one event, potentially
hundreds per training run). Output: an `internal/profile.BehaviorProfile`
(the Behavior IR — see `docs/architecture.md` §3) with two independent
halves: a `FilesystemProfile` holding one `FileAccess` per directory, each
with a *set* of observed permissions and a confidence level, and a
`NetworkProfile` holding one `NetworkAccess` per `(port, direction)` pair
(see "Network aggregation" below). `Synthesize` has no notion that
PodLock or Kubernetes `NetworkPolicy` exist: turning this IR into either
exporter's specific YAML shape is `internal/exporter/podlock`'s and
`internal/exporter/networkpolicy`'s job entirely (see "Categorization"
below, which used to live here and moved out with it).

The risk to avoid: one access per individual file. That would produce
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

## The IR keeps a permission *set*; only the exporter collapses it into a joint category

`Synthesize` accumulates read/write/exec bits per directory and turns
them into `internal/profile.FileAccess.Permissions`, a **set**
(`[]FilePermission`, e.g. `{PermissionWrite, PermissionExecute}`) — see
`permissionsFor()` in `synthesize.go`. It does *not* collapse that set
into a single joint label. That collapsing is specific to PodLock's own
schema, which has no "execute but not read" bucket and, crucially, no way
to express "both executed and written" other than a fourth, distinct
category — so it belongs in `internal/exporter/podlock`, not here:

```go
// internal/exporter/podlock/export.go
func categoryFor(access profile.FileAccess) string {
    switch {
    case access.HasPermission(profile.PermissionExecute) && access.HasPermission(profile.PermissionWrite):
        return "readWriteExec"
    case access.HasPermission(profile.PermissionExecute):
        return "readExec"
    case access.HasPermission(profile.PermissionWrite):
        return "readWrite"
    case access.HasPermission(profile.PermissionRead):
        return "readOnly"
    default:
        return ""
    }
}
```

An earlier version of this logic (back when it still lived in
`Synthesize` and produced a PodLock-shaped `Rule` directly) treated
`readExec` as independent, addable alongside `readWrite` for the same
directory — a directory both executed and written to got
`Access: ["readExec", "readWrite"]`, two separate entries. That was
wrong, and only caught by actually reading PodLock's real CRD source
(`github.com/flavio/podlock`, `api/v1alpha1/landlockprofile_types.go`)
instead of continuing to assume the three fields our own
`pkg/podlock/types.go` had guessed at:

```go
type Profile struct {
    ReadOnly      []string `json:"readOnly,omitempty"`
    ReadWrite     []string `json:"readWrite,omitempty"`
    ReadExec      []string `json:"readExec,omitempty"`
    ReadWriteExec []string `json:"readWriteExec,omitempty"`
}
```

`ReadWriteExec` is a **fourth, distinct** category — not a combination
communicated by populating two lists at once. `categoryFor` always
returns exactly one label.

Every named category also implies read access — there's no "execute but
not read" bucket in PodLock's schema, matching the practical reality that
executing or writing a file requires reading it first. This is a
PodLock-specific convention, not a universal truth about filesystem
permissions — which is exactly why it lives in the exporter and not in
the IR: a hypothetical future exporter for a technology with a real
"execute without read" concept wouldn't have to work around PodLock's
assumption baked into shared code.

This split (IR keeps the raw set, exporter decides how to name it) is
what made the architecture refactor introducing `internal/profile` and
`internal/exporter/podlock` a natural fit rather than a rewrite: the
directory-aggregation algorithm below didn't change at all, only the
shape of what it hands back.

### A second, deeper bug: `Mode: "exec"` was never actually reachable

Getting the *categorization* right (above) wasn't the whole story. Testing
the fix end to end on the live cluster — trying to force a `readWriteExec`
rule by writing and then executing a script in the same directory —
produced `readWrite` only, never `readWriteExec`, no matter what was run
in the container.

The cause was in `internal/tracer`, not here: `trace_open` (the only
gadget `Trace()` ran at the time) subscribes to `openat(2)` events, and
`openat(2)` simply has no "this file is being executed" bit in its flags
(`O_ACCMODE` only distinguishes read/write/read_write — Linux, unlike
FreeBSD, has no `O_EXEC`). So `modeFromOpenFlags()` could never return
`"exec"` from real data; the `"exec"` cases in `Synthesize()` and
`categoryFor()` above were only ever exercised by hand-crafted test
events, never by anything the real tracer could produce.

Fixed by also running Inspektor Gadget's `trace_exec` gadget (hooks
`execve(2)`/`execveat(2)` directly, with its `paths` param enabled to get
the executed binary's path) concurrently with `trace_open`, merging both
into a single `[]tracer.Event` — see `docs/architecture.md` §2-3 for the
concurrent-gadgets design. `Synthesize()` itself didn't need to change:
once real `Mode: "exec"` events exist, `permissionsFor()` picks them up
and the categorization logic described above (wherever it happens to live
at the time — originally in `Synthesize`, now in the exporter) just works.

This is the second real bug in this pipeline (after the directory-open
one above) that only surfaced through live end-to-end testing, not unit
tests — the risk of trusting hand-crafted fixtures for a bridge to a
system (the kernel's syscall ABI) that unit tests can't faithfully
simulate.

Wiring `trace_exec` in wasn't quite the end of it either: the first
attempt still produced zero exec events, even with `trace_exec` running
and the script actually executing in the container (confirmed via the
raw `kubectl gadget run trace_exec:latest --paths ...` CLI). The
`--paths` eBPF param (needed to populate `exepath`/`file`) was being
submitted as `"operator.ebpf.paths"`, guessed from the generic
`"operator.<operatorName>.<key>"` convention that already worked for
`trace_open`/`KubeManager`. That guess was wrong: `runtime.GetGadgetInfo()`
against the live cluster showed the real identifier is
`"operator.oci.ebpf.paths"` — the `oci` operator (per-image loading) owns
a per-image `ebpf` sub-instance, so the prefix compounds. An unknown
param key isn't rejected, just silently ignored, which is why this failed
quietly instead of erroring.

## Network aggregation: `connect`/`bind` are no longer ignored

`Synthesize` used to filter out every event with no `Path`
(`ev.Path == ""`), which silently dropped `connect`/`bind` events (they
carry a `Port`, not a `Path`). That wasn't an oversight, and it wasn't a
limitation specific to our own `pkg/podlock` mirror either — checked
directly against PodLock's real schema: it has **no field for network
rights at all** (`LANDLOCK_ACCESS_NET_BIND_TCP` /
`LANDLOCK_ACCESS_NET_CONNECT_TCP` had nowhere to go). That's still true of
PodLock specifically. It stopped being a reason to drop the data itself
once `internal/exporter/networkpolicy` gave it a destination that isn't
PodLock (see `docs/roadmap.md` M1/M2).

`Synthesize` now branches on `ev.Syscall` before the path check: `connect`
events (egress) and `bind` events (ingress) with a nonzero `Port` are
aggregated by the `(port, direction)` pair into `NetworkProfile.Accesses`,
using the exact same `confidenceFor(seenCount)` heuristic as the
filesystem side (see "Confidence" below) — a port dialed/bound 3 times in
one run is `ConfidenceHigh`, the same threshold as a directory accessed 3
times. Events with `Port == 0` are skipped: a zero port would silently
degrade `internal/exporter/networkpolicy.ToPolicy` into treating it as a
wildcard-relevant value, which no filter field of that gadget should ever
actually produce for a real connect/bind.

### `bind` on an ephemeral port is dropped: it's an outbound client port, not a listener

Found live, on the very first end-to-end run against a real cluster:
tracing a plain outbound `nc <ip> <port>` (never anything listening on the
traced pod) still produced a `bind` event on a high, kernel-assigned local
port — busybox's `nc` binds explicitly before `connect()`ing, the same
way the kernel's own implicit ephemeral-port assignment would if `nc`
didn't. `trace_bind` hooks `bind(2)` itself, and `bind(2)` looks
identical at the syscall level whether it's "grab a throwaway local port
before dialing out" or "claim this port before calling `listen()`" — the
gadget (and therefore `Synthesize`) has no way to see the `listen()` call
that would prove the second case.

`ephemeralPortStart` (`internal/policy/synthesize.go`, `= 32768`, Linux's
default `net.ipv4.ip_local_port_range` floor) is the heuristic fix: `bind`
events on a port `>= ephemeralPortStart` are dropped before aggregation.
It's a heuristic, not a certainty — same spirit as `maxAggregationDepth`
above, same caveat: a service deliberately listening above that threshold
would be filtered out too, a false negative traded for far fewer false
positives (every outbound connection would otherwise mint a bogus
`ingress` rule). See `TestSynthesize_SkipsEphemeralBindPorts`.

## Syscall aggregation: presence, not occurrence

`internal/tracer`'s `runSeccompTracer` (`trace_linux.go`) reuses Inspektor
Gadget's own `advise_seccomp` gadget rather than a raw syscall tracer —
confirmed against its vendored source, not reimplemented from scratch.
It already deduplicates: its `advise` datasource reports one syscall set
per container, once, when the gadget's context is cancelled at the end of
the training run (`ebpf.map.flush-on-stop: true`), not one event per
`sys_enter`. `runSeccompTracer` emits one `Event{Syscall: name, Mode:
"syscall"}` per name in that set, and `Synthesize` aggregates those into
`SyscallProfile.Accesses` by name — same `confidenceFor(seenCount)`
heuristic as the other two domains, but `seenCount` can never exceed 1
within a single run here, since a name appears at most once in the set
`advise_seccomp` already deduplicated. See "Confidence" below for what
this means in practice.

## Confidence: a deliberately provisional heuristic

`Confidence` (`ConfidenceLow`/`Medium`/`High`) is defined in
`internal/profile`, not here: it's a property of the IR (any exporter may
want to flag low-confidence accesses for human review), while
`confidenceFor()` — the heuristic that *computes* it from `seenCount` —
stays in `internal/policy`, next to the aggregation algorithm that feeds
it:

```go
func confidenceFor(seenCount int) profile.Confidence {
    switch {
    case seenCount >= 3: return profile.ConfidenceHigh
    case seenCount == 2: return profile.ConfidenceMedium
    default:             return profile.ConfidenceLow
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
measure**. Without `--history`, don't present current `Confidence`
values as reliable in the threat-model sense.

**The syscall domain has no single-run proxy at all — it's always
`Low`.** Unlike filesystem/network `seenCount` (which can grow within one
run as an access repeats), a syscall's `seenCount` is capped at 1 within
a single run by construction (see "Syscall aggregation" above), so
`confidenceFor` always falls into its `default` case. This is
intentional, not a gap to fix: a single 60-second run can never prove a
syscall is safe to leave out of an enforced profile going forward, and
seccomp is unforgiving of a false negative (a missing syscall breaks the
container outright, unlike an overly-narrow filesystem/network rule). Only
`--history`'s cross-run ratio can honestly raise it above `Low` for this
domain.

**Fixed, opt-in, via `trace --history`.** `internal/history` persists
exactly the state this section describes as missing: a `TrainingHistory`
custom resource (`internal/history/store.go`, no controller — the CLI
reads/writes it directly) accumulating, across every `--history` run for
a given container/binary, how many runs were recorded in total
(`RunsRecorded`) and how many of them observed each access
(`SeenInRuns`). `internal/history.ApplyConfidence` computes `Confidence`
from that real ratio — high only if seen on *every* recorded run,
matching this section's own "seen on every run" / "seen once out of 5
runs" framing, not `confidenceFor`'s single-run proxy. Query it directly:
`kubectl get traininghistory <container>-<binary-basename> -o yaml`.

**Confirmed live** (see `docs/roadmap.md` M5): four runs against
`nginx-demo` (two idle, two with real traffic) produced `runsRecorded: 4`
and `/usr/share/nginx` at `seenInRuns: 2` — a 2/4 ratio, which
`confidenceForHistory` correctly resolves to `ConfidenceMedium`, not
`ConfidenceHigh`. The two idle runs measurably diluted the confidence a
single-run heuristic would never have caught, which is the entire point
of persisting this across runs instead of trusting one.

**Update: the exporter-side gap above is closed.**
`internal/exporter/podlock`/`internal/exporter/networkpolicy`'s `ToYAML`
functions now attach a trailing `# confidence: ...` comment per path/port
(`annotateConfidence`) — invisible to `kubectl apply`, visible to the
human reviewer. `cmd/landlock-genprof/trace.go`'s `recordHistory` calls
`ApplyConfidence` on `behavior` before export, so with `--history` the
comments show the real cross-run ratio; without it, they still show
`confidenceFor`'s single-run proxy — a real number either way, just a
more or less trustworthy one, consistently labeled as `Confidence` in
both cases rather than one being silently hidden.
`internal/exporter/seccomp` can't do the same — its output must stay
plain JSON, no comments — so `cmd/landlock-genprof/trace.go`'s
`writeSeccompProfile` prints the same information to stdout instead,
right after writing the file.

**Confirmed live**: a `trace --history` run against `nginx-demo` (on top
of the four earlier accumulated runs) produced `readOnly: [/usr/share/nginx
# confidence: medium]` in the generated `profile.yaml` — the diluted
ratio from the earlier idle runs, visible directly in the file a human
reviewer actually opens, not just in `kubectl get traininghistory`.

## Determinism

The keys of `map[string]*dirAccess` are sorted (`sort.Strings`) before
building the final `FilesystemProfile.Accesses`, the keys of
`map[netKey]int` are sorted by `(port, direction)` before building
`NetworkProfile.Accesses`, and the keys of `map[string]int` (syscall
names) are sorted (`sort.Strings`) before building
`SyscallProfile.Accesses`. Without this sort, a Go map's iteration order
isn't guaranteed stable from one run to the next — two calls to
`Synthesize` on the same data could produce accesses in a different
order, breaking tests and making generated YAML diffs unreadable in
review. `permissionsFor()` also always appends permissions in a fixed
read/write/execute order, for the same reason.

## See also

- `internal/policy/synthesize.go` — the aggregation algorithm (events ->
  IR)
- `internal/policy/synthesize_test.go` — test cases (aggregation by
  directory, mocked nginx events, empty input, permission-set correctness)
- `internal/profile/profile.go` — the Behavior IR itself
  (`BehaviorProfile`/`FilesystemProfile`/`FileAccess`/`FilePermission`/
  `NetworkProfile`/`NetworkAccess`/`NetworkDirection`/`SyscallProfile`/
  `SyscallAccess`/`Confidence`)
- `internal/profile/deps_test.go` — static check that the IR never
  imports PodLock/YAML/Kubernetes
- `internal/exporter/podlock/export.go` — the IR -> PodLock conversion
  (`ToProfile`/`ToYAML`/`categoryFor`)
- `internal/exporter/networkpolicy/export.go` — the IR -> `NetworkPolicy`
  conversion (`ToPolicy`/`ToYAML`)
- `internal/exporter/seccomp/export.go` — the IR -> seccomp profile
  conversion (`ToProfile`/`ToJSON`)
- [`docs/architecture.md`](architecture.md) — where `Synthesize` and the
  exporter sit in the full pipeline
- [`docs/threat-model.md`](threat-model.md) §2 — multi-run validation
  methodology (not yet implemented)
