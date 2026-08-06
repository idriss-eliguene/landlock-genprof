# The `internal/landlock` kernel extraction

## Context

`internal/profile` (the "Behavior IR") is not a Landlock-specific model.
It's a generic, technology-neutral, cross-domain observation model —
filesystem, network, syscalls, capabilities — deliberately kept
independent of any output format (`internal/profile/deps_test.go` enforces
this statically). That's the right shape for `internal/policy.Synthesize`
today, but it means two things worth being explicit about before this
project commits to anything publicly:

1. **A published, stable `BehaviorProfile` would compete with, not
   complement, Kubescape's Software Bill of Behaviors** (`ApplicationProfile`/
   `NetworkNeighborhood`, migrating toward `ContainerProfile`) — Kubescape
   already records processes, files, syscalls, capabilities, and network
   behavior at CNCF scale. This project's own differentiated territory,
   confirmed against a full ecosystem scan (SPO, Kubescape, KubeArmor,
   Falco, Tracee, Tetragon, PodLock, Inspektor Gadget), is narrower and
   sharper: **nobody else turns observed filesystem behavior into a
   Landlock policy.** That's the one domain worth building a durable
   abstraction around.
2. **There is currently no Landlock-specific semantic layer anywhere in
   this codebase.** `internal/profile.FilePermission` is a generic
   3-verb (read/write/execute) model, not Landlock's real ABI rights
   (`LANDLOCK_ACCESS_FS_EXECUTE`/`WRITE_FILE`/`READ_FILE`/`READ_DIR`/... —
   16 rights across ABI levels 1-6+). The actual translation into
   Landlock rights happens nowhere in this repository today — it's fully
   deferred to PodLock's own operator. Anything calling itself "the
   Landlock domain model" has to be built, not merely relabeled.

## Decision

Extract a narrow, filesystem-only synthesis kernel — `internal/landlock`
— staged deliberately, with no public API commitment until it's earned
one:

- **Phase 0 (done):** characterization tests around the current
  `internal/policy.Synthesize` and `internal/exporter/podlock` output
  (`internal/policy/golden_test.go`,
  `internal/exporter/podlock/golden_test.go`), plus a verified (not
  hand-duplicated) dependency map — [`packages.md`](packages.md) already
  served this and was checked against the real `go list` import graph.
  No public API commitment made at this stage.
- **Phase 1 (done):** `internal/landlock/kernel.go` — `Operation`,
  `EvidenceRef`, `FilesystemObservation`, `FilesystemRight`, `Rule`,
  `Candidate`, `SynthesisReport`, `Synthesize(...)`. Zero imports beyond
  the standard library (`internal/landlock/deps_test.go` enforces this
  the same way `internal/profile/deps_test.go` guards its own package).
  Not wired into anything yet — `internal/policy` and
  `internal/exporter/podlock` are untouched.
- **Phase 2a (done):** `internal/policy.Synthesize`'s filesystem branch
  delegates to `internal/landlock.Synthesize` — see
  [`packages.md`](packages.md) for exactly how the translation works in
  both directions. The Phase 0 golden tests caught a real bug on the
  first attempt (a bare string cast from `tracer.Event.Mode`'s `"exec"`
  to `internal/landlock`'s `OperationExecute = "execute"` silently
  dropped every execute right — fixed by making the translation an
  explicit `operationFor` mapping instead of an implicit cast), then
  passed unchanged once fixed — that's the proof this refactor changed
  nothing observable, not just a claim.
- **Phase 2b (deliberately split off, not started):** switch
  `internal/exporter/podlock` to consume `landlock.Candidate` directly
  instead of `profile.FilesystemProfile`. Split from 2a on purpose rather
  than done together: 2a alone was already enough surface area for the
  golden tests to catch a real bug, and bundling a second package's
  signature change (plus its `cmd`/proposal-publishing call sites) into
  the same commit would have made that bug harder to isolate. Revisit
  once there's a concrete reason to — e.g. alongside Phase 3's second
  exporter, so both migrate together instead of one going first in
  isolation.
- **Phase 3 (done, with a correction):** a canonical JSON exporter
  (`internal/exporter/landlockjson`) consumes `landlock.Candidate`
  directly. Turns out to be the *first* direct consumer, not the second
  as originally planned — `internal/exporter/podlock` still only sees
  the collapsed `profile.FilesystemProfile` (Phase 2b, still deferred).
  Not just a format exercise either: this exporter exists because it's
  the *only* format that can carry `LandlockRightTruncate` at all —
  PodLock's own schema collapses it into "write" and can never recover
  it, which is exactly why `verify` (also shipped) reads a
  `landlockjson` file, not a PodLock `LandlockProfile`.
- **Phase 4:** only once the kernel has survived two real exporters
  (still one down, `internal/exporter/podlock` via Phase 2b to go),
  promote `internal/landlock` to `pkg/landlock` — same Go module, no
  second `go.mod` (Go's own `internal/` rule is the only real visibility
  barrier; a second module buys nothing here and adds real release-
  management cost with zero external consumers today). A signature-
  freeze/`apidiff`-style test is added at this point, not before —
  behavioral tests protect an unproven refactor; API-compatibility tests
  only start earning their cost once there's a real external contract to
  protect.
- **Phase 5 (not started, not committed to):** input adapters beyond the
  current tracer path — potentially native tracing, Kubescape SBOB/
  `ApplicationProfile`, or SPO artifacts, if and when there's a concrete
  reason to add one.

## What this package is, and isn't, on purpose

- It is a filesystem-observations-in, evidence-backed-rules-out kernel.
  Every `Rule` traces back to at least one real `EvidenceRef` — enforced
  by `TestSynthesize_ProvenancePreserved`, not just documented.
- It is **not** a generic cross-domain Behavior IR. Network, syscalls, and
  capabilities stay exactly where they are (`internal/profile`,
  unchanged) — publishing those alongside Landlock risks the SBOB overlap
  above for no benefit specific to this project's actual differentiation.
- `Rule.Rights` carries real `LandlockRight` values directly (the
  earlier, separate coarse `FilesystemRight` type is gone) — but only a
  subset of the full ABI vocabulary is ever actually produced; see the
  "known gap" section below for exactly which rights and why.
- PodLock stays a downstream adapter of `Candidate`, never the domain
  model itself. Its schema has already changed twice under this project
  (missing `ReadWriteExec`, missing network fields — see
  `pkg/podlock`'s own doc comment) and has never been validated end to
  end even in this project's own hands (see
  [`enforcement-prerequisites.md`](enforcement-prerequisites.md)) —
  freezing an API around it would be a bet this project can't currently
  back up.
- No Kubernetes, SPO, PodLock, or tracer import, enforced statically by
  `internal/landlock/deps_test.go`. No second Go module. No `apidiff`
  before there's a real external consumer to protect.

## Known gap: `Rule.Rights` vs. the full Landlock ABI vocabulary

`internal/landlock/abi.go` holds a verified table of Landlock's real,
ABI-versioned rights (`LandlockRight` — `execute`, `write_file`,
`read_file`, `read_dir`, `refer`, `truncate`, `net_bind_tcp`,
`ioctl_dev`, ...), confirmed against `docs.kernel.org`/`man7.org` on
2026-08-06, and backs the CLI's `abi check`/`abi list` commands.

**Update:** `Rule.Rights` now carries `LandlockRight` values directly
(no longer a separate, coarse `FilesystemRight` type) — `Synthesize`
distinguishes `LandlockRightReadFile` from `LandlockRightReadDir` using
the `IsDir` bit every `FilesystemObservation` already carries, and
`internal/policy.collapsePermissions` folds that back down to
`profile.FilePermission`'s coarser read/write/execute vocabulary for
every existing exporter — a real fidelity gain with zero externally
visible behavior change, proven by the existing golden tests passing
unchanged.

**Update 2:** a fifth right, `LandlockRightTruncate` (**ABI3**, kernel
>= 6.2), is now also produced — read from the `O_TRUNC` bit of the same
raw `openat(2)` flags value `trace_linux.go`'s `runOpenTracer` already
captures for `Operation`/`IsDir` (see `tracer.Event.Truncate`'s own doc
comment). No new gadget or syscall hook needed — this was already
flowing through the pipeline, just never read. `collapsePermissions`
folds it into "write" for every existing exporter (never dropped
silently — a rule needing truncate always needs at least write-level
access in the coarse view). This is the first right this package
produces that isn't ABI1, which means `verify`'s filesystem pass can
now say something a bare kernel-version check (`doctor`) cannot: "this
candidate needs kernel >= 6.2 for truncate support," not just "Landlock
is supported at all."

**Update 3:** `verify` (`cmd/landlock-genprof/verify.go`) now exists for
real — reads a `landlockjson` candidate file, checks every rule's rights
against `--kernel`'s (or the local host's) `landlock.ABIForKernel`, and
exits `2` if any rule needs a right unavailable at that ABI level.
Confirmed live: a candidate with a `truncate` rule reports "needs ABI 3,
kernel >= 6.2" against a 5.19 (ABI2) target, and passes clean against
6.5.

**Update 4:** `trace --candidate-out` now produces `verify`'s input from
a real run — `internal/policy.SynthesizeCandidate(events)` re-derives
the raw `Candidate` directly, a small second pass over the same events
rather than a breaking change to `Synthesize`'s own signature (see that
function's own doc comment). Confirmed end to end, not just per-piece:
`writeCandidateJSON`'s output, fed straight into `runVerify`, correctly
flags a `truncate` rule against an ABI2 kernel and passes against ABI3+.
The remaining gap is the *command surface*, not the data: there's still
no standalone `synthesize` verb, only `trace`'s implicit last step.

**What's still the honest ceiling:** `REMOVE_FILE`/`REMOVE_DIR`/the
`MAKE_*` rights (all ABI1) and `REFER` (ABI2) still need the tracer to
observe syscalls it doesn't capture today (`unlink`/`rmdir`/`mkdir`/
`mknod`/`rename`/`link`/..., not just `openat`/`execve`).

**Correction (previously wrong in this doc):** an earlier draft of this
section proposed wiring the *network* domain
(`NET_BIND_TCP`/`NET_CONNECT_TCP`, ABI4) into a future `verify` pass,
reasoning that it was "already observed" and would give real cross-ABI
value cheaply. That's a category error, caught before being built: this
project's `NetworkPolicy` exporter is enforced by a CNI (e.g. Cilium),
entirely unrelated to Landlock's own kernel ABI. Landlock's network
rights have no artifact anywhere in this codebase — PodLock's own schema
has no field for them at all (see `pkg/podlock`'s doc comment) — so
there is nothing to check them against. Network stays out of scope for
Landlock ABI verification entirely, not just out of `internal/landlock`'s
own package.

## Invariants held through every phase

- No user-visible CLI behavior change.
- No silent permission widening (no `Rule` for a directory that was never
  observed — `TestSynthesize_NoRightsInventedForUnobservedPaths`).
- No generic Behavior IR commitment.
- No Kubernetes dependency in the synthesis kernel.
- Provenance preserved for every generated right.
- PodLock remains an output adapter, not the domain model.

Every phase above is independently revertable: nothing is published, and
no external consumer exists yet, so each step's blast radius is limited
to this repository until Phase 4 makes a real promise to someone else.
