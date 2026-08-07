# ADR-0002: Generic Behavior IR as the exporter boundary

Status: Accepted

Date: 2026-08-08

## Context

Multiple output formats (PodLock, `NetworkPolicy`, seccomp, capabilities,
a composed `securityContext`, a Markdown report) all need to be produced
from the same observed behavior, without each one re-deriving synthesis
logic or coupling to the others. Without a shared representation between
observation and output, every new format risks depending on — or
duplicating — whichever format was written first.

## Decision

`internal/profile` (`BehaviorProfile`) is the generic, cross-domain
representation every `internal/exporter/*` package may depend on.
Dependency direction is one-way: exporters may depend on `internal/profile`
(and, for their own output schema, on Kubernetes types or a
hand-rolled format like `pkg/podlock`); `internal/profile` itself must
never depend on any output format, Kubernetes type, or specific
collector. This is enforced statically, not just by convention —
`internal/profile/deps_test.go`'s `TestNoOutputFormatDependency` fails
the build if `internal/profile` ever imports `podlock`, `sigs.k8s.io/yaml`,
any `k8s.io/` package, Cilium, or Inspektor Gadget.

This is **not** a claim that the generic IR can losslessly represent
every security mechanism — the codebase already proves otherwise.
Landlock's real ABI rights (e.g. `TRUNCATE`, `REFER`) don't fit
`profile.FileAccess`'s generic read/write/execute model, which is why
`internal/landlock` exists as a **separate, independent kernel**, not a
Landlock-flavored extension of the IR: it does not import
`internal/profile`, and its own `deps_test.go`
(`TestNoExternalDependency`) statically forbids doing so — the identical
restriction `internal/profile` places on itself, applied in the other
direction. `internal/policy.Synthesize` is the one package trusted to
depend on both, translating `tracer.Event` into
`landlock.FilesystemObservation`, calling `landlock.Synthesize`, and
translating the resulting `landlock.Candidate` back into
`profile.FileAccess` for the exporters that only need the generic
shape. `internal/exporter/landlockjson` consumes `landlock.Candidate`
directly instead, for the exporters (`verify`, `explain`, `diff`) that
need what the generic translation deliberately loses.

The general shape: the generic IR is the boundary for mechanisms it can
represent losslessly; a mechanism-specific kernel exists upstream of it,
independently, when it can't.

## Consequences

**Easier:** eight current exporters (`podlock`, `networkpolicy`,
`seccomp`, `capabilities`, `securitycontext`, `report`, `spo`, plus the
domain-agnostic `sarif`/`junit`) were each added as a sibling package
without touching `internal/policy` or `internal/profile`'s own import
graph — only the IR's exported surface grew. A future mechanism
(AppArmor, SELinux, ...) follows the same pattern `internal/landlock`
established rather than being forced through the generic model and
losing its own semantics.

**Costs:** the boundary is enforced by a runtime test matching import
strings against a hand-maintained deny-list, not by the Go compiler or
module system — the list has to be updated by hand in both
`deps_test.go` files as new output formats or mechanism kernels appear.
Any exporter relying solely on `profile.BehaviorProfile` also inherits
its lowest-common-denominator ceiling; Landlock-specific richness is
only available to the commands that go around the IR entirely.

## References

- `internal/profile/deps_test.go` — `TestNoOutputFormatDependency`
- `internal/landlock/deps_test.go` — `TestNoExternalDependency`
- `internal/policy/synthesize.go` — the translation bridge
- `docs/architecture.md`, `docs/packages.md` — full dependency graph
- `docs/landlock-kernel-extraction.md` — full decision record for why
  `internal/landlock` exists as a separate layer, not an IR extension
