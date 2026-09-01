# ADR-0022: Workload security projection proof levels

## Status

Accepted

## Decision

G2 exposes independent workload security projections for Declared
Configuration, Materialized Policy, Binding/Attachment Evidence, Enforcement
Evidence, Behavioral Verification, Runtime Evidence, Derived Policy, and
Proposal/Governance State. No projection combines these into a Current
Authority or Effective Policy value.

Each section has its own state and provenance. `NOT_AVAILABLE`, `EMPTY`,
`BACKEND_NOT_INSTALLED`, `PERMISSION_DENIED`, `NOT_FOUND`, `TIMEOUT`,
`UNSUPPORTED`, and `UNKNOWN` retain their distinct meanings where applicable.

## Backend boundaries

NetworkPolicy projection identifies each selecting policy together with the
exact runtime Pod set selected by its selector and presents its declared
rules; this is selection evidence, not generic workload attachment and does
not compute a universal effective allow-set.

SeccompProfile existence is materialized policy, not attachment or enforcement.
PodLock existence/readiness is not kernel Landlock enforcement or behavioral
verification. Declared capabilities are not effective process capabilities
without target-bound runtime proof.

Approved proposals remain distinct from applied objects, enforcement, and
behavioral verification. Approval binding is validated against the current
candidate digest; a status of Approved alone does not authorize a changed
candidate. Runtime population compatibility is recorded separately for every
matching RuntimeSubject/incarnation. Repository CI results are not current
workload verification.

## Consequences

The Workbench can explain which facts are available and their proof level
without overstating authority. Optional backend absence and read failures
remain explicit. The projection is assembled from bounded reads and is not an
atomic Kubernetes snapshot.
