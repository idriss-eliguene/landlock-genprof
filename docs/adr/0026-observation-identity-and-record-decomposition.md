# ADR-0026: Observation identity and record decomposition

Status: Accepted

Date: 2026-09-05

## Context

Runtime acquisition currently produces events and contributes to
`TrainingHistory`, while proposals and governance already have durable
identities. v0.7 needs one bounded, attributable record for an observation
without treating mutable execution state or evidence as identity.

## Problem

Without a decomposition, retries, target changes, partial results, and
derivation provenance can be mistaken for a new or changed observation
identity. That would weaken custody and make later proposal provenance
ambiguous.

## Decision

v0.7 defines `Observation` as a durable domain concept with an opaque,
immutable `ObservationID`, assigned once. The record is decomposed into
`ObservationSpec`, `ObservationBinding`, `ObservationExecution`,
`ObservationResult`, and `ObservationProvenance`; only `ObservationID` is
identity.

## Detailed semantics

- `ObservationSpec` is immutable requested intent: `RequestedTarget`, source
  selection, bounded duration/parameters, and requester/session context.
- `ObservationBinding` records runtime resolution: `ResolvedTargetSet`, image
  revisions at claim, backend/version, and applicable executor metadata.
  Historical facts are not rewritten when a target changes.
- `ObservationExecution` is mutable only while running and is terminally
  frozen. It owns lifecycle and execution ownership fields.
- `ObservationResult` accumulates bounded counts, attribution/exclusion facts,
  per-source evidence state, and evidence references/digests. It never embeds
  unbounded raw evidence.
- `ObservationProvenance` is frozen derivation provenance explaining the
  binding/result facts used downstream; it is not identity.
- `RequestedTarget` is immutable intent. `ResolvedTargetSet`,
  `ObservedTargetSet`, `ExcludedTargetSet`, and append-only
  `TargetChangeEvents` remain distinct.
- A same-revision Pod replacement may continue as a new runtime instance.
  A workload or container-image revision change ends the bounded observation
  with `TARGET_REVISION_CHANGED`; disappearance without a valid replacement
  ends it with `TARGET_UNAVAILABLE`. No linked-observation splitting is
  required in v0.7.
- Browser control is bounded to Start, Stop, Status, and Generate Proposal;
  target, duration, sources, concurrency, authorization, and durable custody
  remain explicit constraints. Backends are not installed by this control
  path.

## Invariants

Identity, intent, runtime binding, execution, result, and provenance are not
interchangeable. `UNKNOWN` is not an execution state. Evidence is not policy;
observed is not required or complete; and partial observation is not complete
observation.

## Consequences

Individual runs become attributable and resumable without changing existing
proposal identity. Implementations must bound status and evidence references,
and must preserve partial positive evidence even when execution fails.

## Rejected alternatives

- Treating result or mutable status fields as identity.
- Embedding raw event streams in the durable record.
- Automatically splitting every target change into linked observations.
- Making proposal generation or browser governance implicit.

## Compatibility / migration

This is a forward domain contract. Existing `TrainingHistory`, proposals,
ApplyAttempt, and RollbackAttempt retain their shipped semantics. Historical
records without an `ObservationID` remain legacy records; no identifier is
fabricated.

## Security considerations

Bounded custody and explicit target binding prevent a later runtime subject
from silently inheriting an earlier observation's facts or authority.

## Claim boundary

An observation records successfully acquired and qualified evidence; it does
not prove complete backend coverage, kernel enforcement, or behavioral
verification. Apply and rollback remain nontransactional.

## Open implementation details

Storage fields, exact duration ceilings, and evidence-reference formats remain
implementation choices subject to this contract.

## Non-goals

No Observation CRD, executor, Drift engine, Continuous Assurance, or browser
governance/enforcement is implemented by this ADR.

## References

- [ADR-0024](0024-applyattempt-durable-mutation-custody.md)
- [ADR-0025](0025-explicit-rollback.md)
- [ADR-0027](0027-cluster-identity-resolution-model.md)
- [ADR-0028](0028-workload-identity-container-slot-revision-split.md)
