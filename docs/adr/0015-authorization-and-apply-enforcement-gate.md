# ADR-0015: Authorization and Apply-Proposal Enforcement Gate

Status: Proposed

Date: 2026-08-23

## Decision question

How does landlock-genprof authorize one exact modern candidate and prevent
every security-significant backend mutation unless the approved, evaluated,
authorized, current candidate and exact artifacts are identical?

RFC-0003 through ADR-0014 remain authoritative. This ADR defines the
authorization and side-effect boundary only; it does not redefine eligibility,
resolution, canonical binding, lifecycle semantics, backend evidence, or
revalidation.

## Current apply baseline

The current `apply-proposal` loads a `SecurityProfileProposal` and status,
validates legacy `candidate-v1` `ApprovedCandidateDigest`, builds and parses
an in-memory plan, orders artifacts, waits for SPO readiness, re-reads the
proposal/status, recomputes the legacy digest, and then applies manifests
sequentially. It has mutation seams before apply and before workload binding,
and stops after the first backend error; already-applied artifacts remain.
Current validation proves only legacy artifact approval, not RFC-0003
eligibility or authorization. This ADR is the migration target, not a claim
that the current path already satisfies it.

## Decision: immutable authorization plus one mandatory gate

Introduce an immutable `AuthorizationRecord` and one conceptual
`ValidateAndAuthorizeApply` gate through which every modern security-significant
backend mutation passes. The gate consumes an immutable authorized plan and
current facts; no backend adapter may bypass it.

### AuthorizationRecord

The record binds:

* AuthorizationRecord identity/version/digest;
* BoundCandidateDigest, AuthorityMetadataDigest, and ArtifactDigest;
* EligibilityRecord identity/digest and expected result `ELIGIBLE`;
* AuthorityRule, registry, root-trust, baseline, and composition identities;
* authorization mechanism/version and decision (`AUTHORIZED`, `DENIED`, or
  `UNKNOWN`);
* authorization time, validity/deadline, currentness/revocation references;
* authorizer identity and provenance.

`AUTHORIZED` is permission for this exact candidate under these exact
conditions. It is not approval, eligibility, materialization, or proof of
successful application. `DENIED` is known non-authorization; `UNKNOWN` is
unavailable or indeterminate authorization information. Historical records
remain immutable.

## Authorization issuance

Authorization may be issued only when the modern BoundCandidateDigest is
valid; approval binds that exact digest; the exact EligibilityRecord exists,
binds the same candidate, and is `ELIGIBLE`; all authority identities match;
the record and current facts are valid and unexpired; RFC0003 enforcement mode
and ADR-F capability are active; and no revocation, drift, ambiguity, or
unknown mandatory fact exists. Known negative conditions produce `DENIED`;
mandatory unavailable/unknown conditions produce `UNKNOWN`.

Authorization does not perform backend mutation. The conceptual sequence is:

`EvaluateEligibility → Authorize → BuildApplyPlan → ValidateApplyPlan → CommitSideEffects`.

## Exact identity chain and gate

Immediately before the first side effect, the gate MUST verify:

1. rollout mode is `RFC0003_ENFORCEMENT_MODE`;
2. current candidate is modern;
3. current BoundCandidateDigest equals approved, EligibilityRecord, and
   AuthorizationRecord bindings;
4. AuthorityMetadataDigest and all relevant rule/registry/root/baseline/
   composition identities match those records;
5. current ArtifactDigest equals the exact artifact set in the plan;
6. EligibilityRecord result is `ELIGIBLE` and current;
7. AuthorizationRecord decision is `AUTHORIZED` and current;
8. required revocation/current facts are fresh and non-negative;
9. no known invalidation or context drift exists; and
10. the plan contains only the exact authorized artifacts and targets.

Any failed or unknown mandatory check prevents mutation. No valid digest,
approval, eligibility record, Ready state, materialization, or historical
authorization can substitute for this gate.

## Per-mutation authorization and TOCTOU

For `Plan = [M1, M2, ..., Mn]`, validation of M1 does not authorize later
mutations. Every security-significant mutation crosses its own
`ValidateMutationAuthority` boundary immediately before the side effect. The
operation consumes the current candidate, exact approval/eligibility/
authorization records, ApplyPlan and PlanStep, current facts/context,
rollout capability, and TargetPreState and returns exactly `PERMITTED`,
`DENIED`, `UNKNOWN`, or `INVALID`. Only `PERMITTED` permits that mutation.

Any wait, retry, readiness observation, external observation, or interval
between mutations invalidates prior permission assumptions. If capability,
root trust, revocation, context, eligibility, authorization, or a mandatory
fact changes between steps, the next step cannot execute unless fresh
validation returns `PERMITTED`. Completed mutations remain partial
realization and are not retroactively authorized.

Security-significant mutations include confinement, workload binding,
security contexts, Seccomp, Landlock/PodLock, NetworkPolicy, capabilities,
backend identity/payload/selectors, security labels/annotations, helper
resources affecting enforcement, policy removal/replacement, and rollback or
compensation that can change effective authority. ADR-G may classify backend
operations, but cannot classify an authority-widening operation outside this
gate.

## Record applicability and supersession

AuthorizationRecord binds the exact ApprovalRecord identity used to issue it,
including approval mechanism/version, approved BoundCandidateDigest, record
identity/digest, validity/revocation reference, and semantically relevant
approver/epoch. Replacing approval with another record having the same bound
digest does not preserve authorization applicability unless the exact bound
approval remains current.

EligibilityRecords and AuthorizationRecords carry a monotonic evaluation or
authorization epoch (or an exact supersession relation). Currentness is never
inferred from wall-clock list order, Kubernetes resourceVersion, discovery
order, or latest-object lookup. A newer same-lineage `INELIGIBLE` or `DENIED`
invalidates an older positive record; a newer `UNKNOWN` also prevents an
older positive record from authorizing current mutation. Conflicting records
in one lineage/epoch are `INVALID` and fail closed. Historical records remain
immutable.

## Immutable ApplyPlan

`ApplyPlan` is immutable/content-bound and contains plan identity/version/
digest, BoundCandidateDigest, ArtifactDigest, AuthorizationRecord identity/
digest, backend target identities, exact payload identities/content digests,
namespace/scope, and planner semantic identity where security-significant.
Each authorized artifact has exactly one disposition: `APPLY`,
`ALREADY_SATISFIED`, or `NOT_APPLICABLE`. `ALREADY_SATISFIED` requires
verified exact authorized target pre-state. `NOT_APPLICABLE` is allowed only
when authority/backend semantics explicitly permit it. Missing artifacts,
unknown applicability, or unsupported mandatory backends are not silently
optional. The plan cannot add undeclared resources, drop authorized
resources, widen scope, substitute targets, or introduce content outside the
authorized ArtifactDigest. Security-significant step order and dependency
semantics are part of plan identity; different effective order requires a
different plan identity unless ADR-G has registered the operation class as
order-independent.

`ValidateApplyPlan` returns `VALID`, `INVALID`, or `UNKNOWN`. Payload/identity
mismatch, undeclared or missing mandatory artifact, malformed target, known
target or pre-state mismatch, stale/revoked authorization, and conflicting
same-lineage authorization are `INVALID`. Mandatory current fact, target, or
pre-state unavailability and unknown applicability are `UNKNOWN`. Neither
result permits side effects. Only a `VALID` plan reaches per-mutation
authorization, and every mutation must still return `PERMITTED`. The commit
boundary accepts only one validated immutable plan and invokes backend
adapters with no authority discretion.

## Target identity and pre-state

`TargetIdentity` is a canonical backend-neutral value containing every
security-relevant backend identity, enforcement environment, resource/workload
identity, namespace/scope where applicable, selector/binding identity, and
container/process identity. Diagnostic identifiers are excluded unless
required to distinguish the actual enforcement target. ADR-G supplies
backend-specific canonical components; substitution changes plan identity or
fails validation.

`TargetPreState` is required whenever existing backend state affects security
meaning. ADR-G classifies each step as pre-state irrelevant, exact pre-state
required, or an allowed pre-state predicate/set. Known mismatch is `INVALID`;
unavailable mandatory pre-state is `UNKNOWN`; only a satisfied requirement
can produce `PERMITTED`.

## TOCTOU and side-effect boundary

Authorization, plan construction, plan validation, and commit operate on
snapshotted/copied content identities. Any proposal, authority, approval,
authorization, payload, target, namespace, or current-context mutation after
authorization changes the identity or invalidates the plan. The gate
revalidates immediately before every security-significant mutation; a
readiness wait or other interval does not carry permission forward.

Before commit there is no Kubernetes apply, filesystem installation, SPO,
PodLock, NetworkPolicy, Seccomp, capability, or future-backend mutation. A
backend adapter receives only the validated plan and MUST NOT resolve,
authorize, evaluate, modify, add, or widen it.

## Partial application and idempotency

Multi-artifact backend application is not assumed transactional. A failure
after one artifact leaves a partial realization, which MUST be recorded as
materialization failure/partial state and MUST NOT be represented as complete
authorized enforcement. Runtime/current authority remains separately
revalidated by ADR-H.

Retrying the same BoundCandidateDigest + AuthorizationRecord + ApplyPlan is
allowed only after revalidation. Existing resources must match authorized
content and target identity; externally modified or ambiguous resources cause
`INVALID`/fail-closed behavior. Retries cannot add scope or silently replace a
plan. Backend reconciliation and drift detection belong to ADR-G/H.

If M1 succeeds and M2 fails or becomes unauthorized, M3 and later steps do
not execute unless each independently becomes `PERMITTED` under current
state. The realization is `PARTIAL`, never `COMPLETE` or `ACTIVE` merely
because some artifacts succeeded. Retry requires fresh candidate,
authorization, plan, target, and pre-state validation. Rollback, deletion,
replacement, cleanup, or compensation is itself a security-significant
mutation when it can change effective authority; it is not implicitly
authorized by the failed plan and cannot automatically widen authority.

For retry classification, exact authorized content and ownership already
satisfied is `ALREADY_SATISFIED`; known different content, target, ownership,
or pre-state is `INVALID`; unavailable backend reads are `UNKNOWN`; missing
state may be created only after current per-mutation `PERMITTED`. A timeout
never proves that a write succeeded, and matching payload bytes alone never
proves ownership.

Two concurrent plans conflict when security-significant steps target the same
or overlapping TargetIdentity/enforcement scope and their combined execution
is not explicitly registered as commutative and authority-preserving.
Conflicting plans cannot independently mutate from the same stale pre-state.
An implementation must provide serialization, CAS/preconditions, a lease, or
an equivalent mechanism; if conflict cannot be determined, new mutation is
`UNKNOWN` and fails closed.

## Legacy and advisory behavior

In `AUTHORITY_ADVISORY_MODE`, modern proposals may be approved and eligible,
but authorization is unavailable and `apply-proposal` MUST NOT mutate modern
security backends. Modern proposals never enter legacy apply semantics. A
legacy proposal may use its explicitly supported legacy path only while the
product permits that compatibility mode. Mixed candidate-v1 and modern
identities fail closed.

Only `RFC0003_ENFORCEMENT_MODE` with an active gate can claim compliant modern
enforcement. Legacy and advisory modes cannot make that claim.

## Revocation and unknown

Authorization applicability is invalidated by candidate/artifact/authority
change, EligibilityRecord invalidation or expiry, rule/trust-root/baseline/
provenance/compatibility failure, context drift, authorization expiry or
revocation, and disabled rollout capability. Historical AuthorizationRecords
remain auditable. Unknown current context, revocation, root status,
currentness, or authorization causes fail-closed non-mutation without being
rewritten as historical denial.

## Ownership and concurrency

One component owns each operation: the authorizer creates
AuthorizationRecords; the planner creates ApplyPlans; the gate validates;
the commit coordinator invokes adapters. Optimistic/content-addressed
concurrency rejects stale authorization, proposal replacement, approval
replacement, or concurrent conflicting applies. A stale authorizer cannot
write `AUTHORIZED` for a different candidate. ADR-E persists records; ADR-G
reports realization; ADR-H handles ongoing revocation, drift, and remediation.

## Failure and audit model

Semantic denial, unknown authority, invalid plan, backend operational failure,
partial backend failure, and impossible internal invariant failure remain
distinct. Operational failure does not create a new eligibility result.

An attempt is reconstructable from proposal identity, all three digest
identities, approval, EligibilityRecord, AuthorizationRecord, ApplyPlan,
target identities, timestamps, and backend results/failure codes.

## Compliance claim boundary

The product may claim an RFC-0003-compliant enforcement path only when modern
identity, current authorization, an immutable validated plan, and an active
ADR-F gate cover every backend path included in the claim. The claim names or
canonically identifies that enforcement-path scope. Seccomp and
NetworkPolicy being gated while an in-scope securityContext mutation is not
gated cannot be claimed as one combined compliant scope; only a narrower
explicitly identified gated scope may be claimed.

## Security invariants

1. Approval does not imply authorization.
2. Eligibility does not imply authorization.
3. Authorization does not imply successful application.
4. Ready/materialized does not imply authorization.
5. Only exact candidate-bound authorization crosses the side-effect boundary.
6. Modern candidates never use legacy enforcement semantics.
7. Unknown facts never permit mutation.
8. ApplyPlan cannot widen authorized artifact scope.
9. Post-authorization mutation invalidates applicability.
10. Partial application cannot be represented as complete active authority.
11. Historical authorization cannot prove current authority.
12. Every claimed backend passes the same conceptual gate.
13. A valid plan cannot be reused with another AuthorizationRecord.
14. Authorization for one mutation never authorizes a subsequent mutation.
15. Every security-significant mutation requires current permission at its own boundary.
16. A wait or retry never carries mutation permission forward.
17. Superseded or conflicting current authorization cannot be replaced by an older positive record.
18. Every authorized artifact has exactly one explicit plan disposition.
19. Security-significant plan ordering is identity-bound.
20. Target substitution invalidates plan applicability.
21. Required backend pre-state is part of mutation validation.
22. Partial success grants no authority for remaining mutations.
23. Rollback/compensation is not implicitly authorized.
24. Conflicting concurrent applies cannot rely on the same stale pre-state.
25. Historical replay cannot cross the current side-effect boundary.
26. Backend adapters cannot synthesize security-significant behavior outside the validated plan.
27. Compliance claims are scoped to explicitly identified gated paths.

## Adversarial outcomes

Approval without eligibility, eligibility without authorization, candidate or
artifact substitution, stale/expired records, unknown revocation/context,
root changes, Ready shortcuts, legacy fallback, mixed identities,
undeclared plan resources, dropped resources, namespace changes, retries,
concurrent stale authorizers, external drift, disabled capability, SPO Ready,
legacy modern-record combinations, and historical replay all fail closed or
remain operational failures without granting authority. Seccomp may succeed
while NetworkPolicy fails; the result is partial realization, never complete
active authority.

## Alternatives considered

* apply directly after `ELIGIBLE`: rejected;
* approval plus eligibility without AuthorizationRecord: rejected;
* backend-specific authorization: rejected;
* authorization embedded only in ApplyPlan: rejected because permission and
  executable plan require separate immutable identities;
* separate AuthorizationRecord plus validated immutable ApplyPlan and one
  mandatory gate: selected.

## Handoffs and tests

ADR-G owns backend adapters and realization/drift observations. ADR-H owns
revalidation, revocation, currentness, and remediation. ADR-E owns lifecycle
persistence. This ADR owns authorization and the gate, not runtime
implementation.

Future tests MUST include unit/table authorization cases, negative-security
identity and TOCTOU tests, plan mutation and concurrency tests,
partial-application and idempotency tests, legacy isolation, envtest/E2E
gate coverage, and cross-language identity/gate vectors.

## Consequences and open questions

The gate adds immutable authorization and plan records and requires every
claimed enforcement backend to pass one conceptual boundary. It preserves
partial backend failure as an operational reality and requires later ADR-G/H
integration. Concrete Go types, CRDs, storage, controller mechanics,
signature technology, and remediation remain implementation decisions.
