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

## Observation completeness

Absence is not observation failure, and a partial observation is not a
complete one. Discovery enumerates every Pod carrying the target, so that set
is the authoritative denominator; the number of successful reads never
redefines it.

Declared Configuration therefore records one `PodReadObservation` per
target-carrying Pod: the Pod reference, the normalized read state from the
existing `internal/k8s` taxonomy, the reason, and whether that Pod actually
contributed declarations. The aggregate state is `AVAILABLE` only when every
target-carrying Pod was read. When some Pods were read and others were not,
the aggregate is `UNKNOWN` and names the counts — the successful declarations
are retained rather than discarded, and the failures stay visible with their
distinct states rather than being collapsed into absence. When no Pod could be
read at all, the read error is returned and the projection fails closed, which
is unchanged.

## Attributed and excluded evidence

"No evidence was observed" and "evidence was observed but could not be
attributed to this target" are different facts and must not render alike.

Runtime Evidence separates attributed evidence from excluded evidence. Only
`ASSOCIATED` sources are attributed. Every other G1.5 association state is
recorded as `ExcludedEvidence` carrying the source reference and the exact
association state and reason. Attribution remains fail-closed: recording that
a source exists is never permission to associate it, and excluded evidence
carries no authority and never contributes to attributed evidence. In
particular the `INSUFFICIENT_PROVENANCE` state that G1.6 preserves for
legacy objects stays explainable instead of vanishing.

The section state distinguishes the two cases: `EMPTY` when no source was
supplied, and `UNKNOWN` when sources were considered and none could be
attributed.

## Known limitation

Proposal/Governance State does not yet carry the equivalent record. Proposals
whose `spec` cannot be read or converted are skipped during load, and
proposals that are not `ASSOCIATED` are dropped rather than recorded as
excluded. A Governance state of `EMPTY` therefore does not by itself prove
that no proposal object exists for the namespace. This is tracked separately
and is not claimed as resolved here.

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
remain explicit per Pod, as do Pod read failures and excluded evidence; the
Proposal/Governance limitation above is the one known exception. The
projection is assembled from bounded reads and is not an atomic Kubernetes
snapshot.

Two existing fields gained a value rather than changing meaning:
`Declared.State` and `Runtime.State` can now be `UNKNOWN`. Both previously
reported a confident state in situations where knowledge was in fact partial,
so the added value narrows an overclaim rather than widening a contract.
