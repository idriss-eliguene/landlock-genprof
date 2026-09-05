# ADR-0032: Per-source EMPTY and UNKNOWN precondition rule

Status: Accepted

Date: 2026-09-05

## Context

Observation may use several evidence sources with different backends,
coverage, attribution, and failure modes. Existing projections already keep
attributed and excluded evidence distinct.

## Problem

A single observation-wide state can turn a failed or unqualified source into
`EMPTY`, or erase positive evidence from another source.

## Decision

Evidence epistemic state is evaluated per source. `EMPTY(source)` is valid
only when backend health for the window is confirmed, the source was attached
and active for the qualified window, flush/read-back completed, attribution
processing completed, `attributed(source) == 0`, and `excluded(source) == 0`.
Otherwise the source state is `UNKNOWN` rather than `EMPTY`.

## Detailed semantics

The epistemic states are `AVAILABLE`, `EMPTY`, and `UNKNOWN`. Operational
states such as `NOT_AVAILABLE`, `BACKEND_NOT_INSTALLED`, `PERMISSION_DENIED`,
`NOT_FOUND`, `TIMEOUT`, and `UNSUPPORTED` remain available where applicable
and are not collapsed into epistemic states unless an existing domain mapping
requires it. `excluded > 0` implies `UNKNOWN`, even when attributed evidence
is positive; positive attributed facts are retained.

There is no lossy single-state aggregation. For example, filesystem
`AVAILABLE` and syscall `UNKNOWN` remain both true. Any UI summary is derived
presentation and cannot erase source-level facts. `UNKNOWN` never silently
collapses into `EMPTY`; absence is not observation failure.

Execution and knowledge remain orthogonal: `COMPLETED` may have source
evidence `UNKNOWN`, and `FAILED` may retain `AVAILABLE` evidence. The
execution lifecycle is `REQUESTED`, `STARTING`, `RUNNING`, `COMPLETING`, then
`COMPLETED` or `FAILED`; `UNKNOWN` is not an execution state. A requested Stop
uses bounded completion with reason `STOPPED_BY_REQUEST` rather than inventing
a new lifecycle enum.

## Invariants

Evidence is not policy; coverage is not confidence; observed is not required
or complete. `NO_EVIDENCE_OBSERVED` differs from
`EVIDENCE_OBSERVED_BUT_UNATTRIBUTABLE`, and partial observation differs from
complete observation.

## Consequences

Consumers must carry source-level qualification and exclusion facts. A
summary may be more complex, but it remains honest about missing
qualification and preserves positive partial knowledge.

## Rejected alternatives

- Treating zero returned items as proof of true emptiness.
- One observation-wide EMPTY/UNKNOWN value replacing per-source truth.
- Dropping attributed evidence when another source or event is excluded.
- Introducing generic score or severity values.

## Compatibility / migration

This extends existing per-domain projection semantics without replacing
TrainingHistory or SPO ownership of syscall observation. Historical data stays
historical and may lack the qualification facts required for a new EMPTY
claim.

## Security considerations

Failing closed to UNKNOWN prevents missing backend, attribution, or flush
evidence from becoming an allow-oriented absence claim.

## Claim boundary

AVAILABLE means qualified evidence was acquired and attributed; it does not
prove completeness, enforcement, kernel denial, or behavioral verification.

## Open implementation details

Exact backend health signals, flush acknowledgements, and serialized reason
vocabularies remain implementation details.

## Non-goals

No evidence-model rewrite, generic score, generic severity, or backend
installation is implemented here.

## References

- [ADR-0026](0026-observation-identity-and-record-decomposition.md)
- [ADR-0029](0029-observation-persistence-adapter-boundary.md)
