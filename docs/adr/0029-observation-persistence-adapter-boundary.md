# ADR-0029: Observation persistence adapter boundary

Status: Accepted

Date: 2026-09-05

## Context

v0.7 needs durable Observation metadata while the current product is
Kubernetes-based and already uses TrainingHistory and Kubernetes API types.

## Problem

Defining Observation as a CRD shape would couple domain semantics to one
transport and encourage putting unbounded evidence into Kubernetes objects.

## Decision

Observation is a domain contract. A bounded Kubernetes representation is a
v0.7 persistence/transport adapter, not the domain definition. Domain types
and semantics remain separable from Kubernetes API types.

## Detailed semantics

The adapter may persist bounded metadata, status, counts, provenance, and
references. Raw evidence remains in an external or local artifact mechanism
and is referenced by digest/reference; it is not embedded unbounded in the
CRD. Normalized evidence continues through existing `TrainingHistory`
semantics, and new contributions may reference their ObservationID where
compatible. Historical records have absent/legacy provenance; identifiers are
never fabricated.

Observation history must not be owner-referenced to a workload when that
would make workload deletion erase historical custody. Retention and garbage
collection remain implementation-policy questions until proven.

## Invariants

Domain contract, Kubernetes serialization, and evidence artifact storage are
separate boundaries. TrainingHistory is extended, not replaced.

## Consequences

Another persistence or fleet mechanism can be added without redefining
Observation. Implementations must manage references, retention, and bounded
status explicitly.

## Rejected alternatives

- Making CRD serialization the domain model.
- Storing raw evidence unbounded in Kubernetes.
- Replacing TrainingHistory.
- Designing SaaS/fleet storage in v0.7.
- Owner-referencing history in a way that erases it with a workload.

## Compatibility / migration

The adapter is additive and backward-compatible. Existing TrainingHistory and
proposal schemas retain their semantics; legacy data remains legacy.

## Security considerations

Digest/reference custody must not be confused with authorization. Artifact
access and Kubernetes RBAC remain explicit controls.

## Claim boundary

Persistence of a record or reference does not prove evidence completeness,
backend health, enforcement, or behavioral verification.

## Open implementation details

The adapter API, reference store, retention policy, and exact bounded fields
are intentionally deferred to implementation review.

## Non-goals

No CRD, storage engine, fleet service, or retention guarantee is implemented
by this ADR.

## References

- [ADR-0026](0026-observation-identity-and-record-decomposition.md)
- [ADR-0032](0032-per-source-empty-unknown-precondition-rule.md)
