# ADR-0028: Workload identity, container slot, and revision split

Status: Accepted

Date: 2026-09-05

## Context

Kubernetes rollouts replace runtime instances and images without necessarily
replacing the workload object. Observation binding and future Drift work need
to distinguish stable object identity from revisions.

## Problem

Treating every rollout as a new workload, or treating every runtime instance
as the same subject, can aggregate evidence across incompatible revisions and
make authority stale without detection.

## Decision

`WorkloadIdentity` is `ClusterIdentity + namespace + GroupKind + name +
Kubernetes object UID`. `WorkloadRevision` is separate. `ContainerSlot` is
`WorkloadIdentity + container name`; `ContainerImageRevision` is
`ContainerSlot + image digest`; `RuntimeContainerInstance` is a concrete
running container associated with a concrete Pod.

## Detailed semantics

A rollout normally preserves `WorkloadIdentity` and creates a new
`WorkloadRevision`. Deletion and recreation of the Kubernetes object creates a
new object UID and therefore a new identity. For `Deployment/api`, container
`backend`, digest A changing to digest B means the same workload and slot but
a new image revision, not a new workload/container-slot identity.

Observation records requested intent separately from resolved runtime
instances. A same-revision Pod replacement can be recorded as a new runtime
instance. A `WorkloadRevision` or `ContainerImageRevision` change terminates
the bounded observation with `TARGET_REVISION_CHANGED`; evidence already
collected remains qualified with its actual binding. Drift is not run inside
an individual Observation.

`WorkloadIdentity` is deliberately narrower and stricter than the existing
name-based `GovernedTarget` / `CanonicalTargetBinding`. Observation
epistemic correctness must distinguish Kubernetes object deletion and
recreation, which requires the object UID. Existing declarative
governance/apply/rollback semantics intentionally retain their shipped
binding model. Therefore `WorkloadIdentity != GovernedTarget`; this ADR does
not retroactively redefine `GovernedTarget` and proposes no migration.

## Invariants

Identity, revision, and runtime instance are distinct. No observation silently
aggregates evidence across a revision change.

## Consequences

Stable workload history can be compared with explicit revision boundaries.
Implementations must resolve object UID and image digest where required and
must preserve target-change events.

## Rejected alternatives

- Pod name as workload identity.
- Container name alone as identity.
- Image digest as workload identity.
- Automatic cross-revision aggregation.

## Compatibility / migration

Existing canonical target and TrainingHistory records remain valid. New
Observation provenance may add revision facts; historical records are not
retroactively assigned invented identities.

## Security considerations

Revision separation prevents stale observations or proposals from being
presented as evidence about a changed image.

## Claim boundary

Identity and revision facts do not prove observed behavior, policy correctness,
enforcement, or verification.

## Open implementation details

The exact revision derivation and runtime-instance identifier are reversible
implementation choices, subject to preserving these distinctions.

## Non-goals

No Drift engine, NOT_COMPARABLE state, or continuous observation scheduler is
defined here.

## References

- [ADR-0026](0026-observation-identity-and-record-decomposition.md)
- [ADR-0027](0027-cluster-identity-resolution-model.md)
