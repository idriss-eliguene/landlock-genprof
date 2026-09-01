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

## Attributed and excluded proposals

Proposal/Governance State carries the same separation, with one addition:
interpretation happens before association is reachable, so a proposal can stop
being usable at two different stages and the projection keeps them distinct.

Every listed proposal object produces either an attributed `ProposalState` or
an `ExcludedProposal` carrying its source reference, a typed `ProposalExclusion`
of `NOT_INTERPRETED` or `NOT_ASSOCIATED`, the exact `association.Result` when
association was reached (absent when it was not), and the reason.

`NOT_INTERPRETED` covers a missing spec, a spec that is not an object, a spec
that does not convert to a candidate, and a status that does not convert. An
unreadable status is deliberately not reported as "not approved": that would
state a conclusion the object does not support, so the object is excluded
whole and its approval state is described as unreadable.

Retained identity is provenance, never attribution. An excluded proposal
carries no approval, application, or enforcement meaning for the target, and
approval binding continues to be validated independently for attributed
proposals — association is not approval.

## Section state semantics

For Declared Configuration, Runtime Evidence, and Proposal/Governance State:

- `AVAILABLE` means every relevant observation succeeded and contributed.
- `EMPTY` means genuine absence within the section's bounded read scope. It
  never means an object was observed and then failed to be read, interpreted,
  or attributed, and it is not a claim about the cluster beyond that scope.
- `UNKNOWN` means knowledge is partial: something was observed that did not
  contribute, and the reason names the counts.

`UNKNOWN` never erases positive facts. When some proposals are attributed and
others excluded, the attributed ones remain fully populated alongside the
exclusions; partial knowledge is neither complete knowledge nor no knowledge.

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
without overstating authority. An observation G2 has already acquired does not
silently become absence because it failed to be read, interpreted, or
attributed: absence is not observation failure, no evidence is not excluded
evidence, and no proposal is not excluded proposal. Fail-closed attribution is
unchanged — recording that something was observed is never permission to
attribute it. The projection is assembled from bounded reads and is not an
atomic Kubernetes snapshot.

Three existing fields gained a value rather than changing meaning:
`Declared.State`, `Runtime.State`, and `Governance.State` can now be `UNKNOWN`.
Each previously reported a confident state in situations where knowledge was
in fact partial, so the added value narrows an overclaim rather than widening
a contract.

Two defensive paths remain where a malformed object would degrade silently: a
NetworkPolicy whose `podSelector` cannot be interpreted is skipped, and
non-slice `spec.ingress`/`spec.egress` yield a zero rule count. Both require
input a conformant Kubernetes API server rejects, so neither is reachable
through a validated cluster read.
