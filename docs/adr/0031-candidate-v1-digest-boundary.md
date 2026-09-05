# ADR-0031: candidate-v1 digest boundary

Status: Accepted

Date: 2026-09-05

## Context

The released approval contract uses `candidate-v1` and exact candidate
digests. v0.7 introduces Observation provenance but must not silently change
what existing approvals authorize.

## Problem

Adding observation, cluster, workload, image, backend, or evidence metadata to
the existing candidate digest would invalidate or reinterpret shipped
approvals.

## Decision

ObservationID and Observation provenance remain outside `candidate-v1`.
`candidate-v1` continues to digest the existing derived candidate content
using its shipped canonicalization. Provenance may be durable and immutable,
but is not automatically candidate content.

## Detailed semantics

The current implementation canonicalizes these six `SecurityProfileProposal`
spec fields, in stable order: `container`, `binary`, `podLock`,
`networkPolicy`, `patchedManifest`, and `spoSeccompProfile`, then hashes the
canonical JSON as `sha256:<lowercase-hex>`. Approval records the digest and
`approvalMechanismVersion: candidate-v1`; apply rejects a missing, malformed,
unsupported, stale, or mismatched binding.

Proposal object identity, proposal content digest, evidence provenance, and
authorization binding are separate concepts. ObservationID, raw/normalized
evidence digests, ClusterIdentity, WorkloadIdentity, image digest, backend
version, and source metadata may belong to provenance as appropriate, but are
not added to candidate-v1 merely because v0.7 introduces them.

`candidate-v1` is a content digest. Approval authority is scoped to the
specific `SecurityProfileProposal` object whose status is read or mutated by
the approval/apply path; approval is not discovered by finding another
proposal with the same digest. Consequently, two proposal objects may have
identical candidate content digests without sharing approval authority.
Observation provenance remains outside candidate-v1.

## Invariants

Candidate-v1 semantics and existing approval meaning do not change. Provenance
does not become approval authority, and digest equality does not replace human
approval.

## Consequences

New provenance can explain a candidate without invalidating the shipped
content identity. If future policy requires provenance to affect authorization,
it needs a separately reviewed mechanism and migration contract.

## Rejected alternatives

- Adding ObservationID to candidate-v1.
- Adding raw or normalized evidence digests to candidate-v1.
- Hashing cluster/workload/backend metadata into existing candidates.
- Treating provenance or digest equality as approval.

## Compatibility / migration

No implementation or existing candidate is changed by this ADR. A future
digest mechanism requires a new version, explicit migration, and re-review of
authority semantics.

## Security considerations

Keeping the boundary prevents authorization from silently migrating to a
different candidate representation or provenance interpretation.

## Claim boundary

Candidate-v1 proves only the exact canonical content binding it defines; it
does not prove observation completeness, enforcement, or behavioral
verification.

## Open implementation details

The durable location and schema of future provenance remain implementation
choices outside candidate-v1.

## Non-goals

No digest algorithm change, candidate schema change, approval change, or
implementation is made here.

## References

- [ADR-0006](0006-security-profile-proposal-approval-binding.md)
- [ADR-0014](0014-proposal-lifecycle-and-persistence-mapping.md)
- [ADR-0026](0026-observation-identity-and-record-decomposition.md)
