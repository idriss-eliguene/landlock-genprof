# ADR-0014: Proposal Lifecycle and Persistence Mapping

Status: Proposed

Date: 2026-08-23

## Decision question

How should landlock-genprof project RFC-0003 authority objects and results
onto persistent proposal state without conflating review, approval,
eligibility, authorization, materialization, runtime authority, or behavioral
verification?

RFC-0003, ADR-0010, ADR-0011, ADR-0012, and ADR-0013 remain authoritative.
Persistence is a projection of those domain objects, never their semantic
definition.

## Current proposal model

The current `SecurityProfileProposal` stores rendered artifact strings in
`spec` (`Container`, `Binary`, `PodLock`, `NetworkPolicy`,
`PatchedManifest`, and `SPOSeccompProfile`), plus `GeneratedAt` and
`HistoryUsed`. Its status contains `ApprovalState` (`Draft`, `Reviewed`,
`Approved`, `Rejected`), free-text `Reason`, `UpdatedAt`,
`ApprovedCandidateDigest`, and `ApprovalMechanismVersion`.

The current digest is legacy artifact-only `candidate-v1`; approval verifies
that digest, while current `apply-proposal` performs artifact and readiness
work. This is the observed implementation, not the target RFC authority
model. Existing approval does not currently represent eligibility or
authorization.

## Decision: hybrid orthogonal persistence projection

Retain `SecurityProfileProposal` as the proposal/artifact and human-review
aggregate, but project authority and evaluation through immutable typed
references and status dimensions. Do not use one monolithic phase enum.
Conditions MAY provide human/API diagnostics, but typed dimensions and exact
identity references are the semantic source of status.

The conceptual status dimensions are:

* `ReviewState`: `DRAFT`, `REVIEWED`, `APPROVED`, `REJECTED`;
* `EligibilityState`: `UNKNOWN`, `ELIGIBLE`, `INELIGIBLE`, `STALE`,
  `INVALIDATED`;
* `AuthorizationState`: `NOT_AUTHORIZED`, `AUTHORIZED`, `REVOKED`,
  `SUSPENDED_UNKNOWN`;
* `MaterializationState`: `NOT_MATERIALIZED`, `MATERIALIZING`,
  `MATERIALIZED`, `MATERIALIZATION_FAILED`;
* `RuntimeAuthorityState`: `NOT_ACTIVE`, `ACTIVE`, `ACTIVE_UNAUTHORIZED`,
  `SUSPENDED_UNKNOWN`; and
* `BehavioralVerificationState`: `NOT_VERIFIED`, `VERIFIED`, `UNKNOWN`.

These names are persistence labels for RFC/domain states, not new security
semantics. Approval never sets eligibility; eligibility never sets
authorization; authorization never sets materialization; materialization
never sets runtime active; active never sets behavioral verification.

## Domain and persistence boundary

The domain objects remain ADR-0010 types. Persistence stores either an
immutable serialized object/reference governed by ADR-0011/0012 or a status
projection containing its exact identity. CRD shape, field naming, and
subresource mechanics are implementation concerns and MUST NOT redefine the
RFC.

`AuthorityMetadata` is preferably stored as an immutable external authority
object or immutable artifact referenced from the proposal, with only its
exact digest/reference summary persisted on the proposal. This avoids mutable
large copies, preserves auditability, and keeps registry/rule/evidence
objects independently versioned. A bounded embedded snapshot MAY be used for
offline review only when it remains cryptographically bound to the same
`AuthorityMetadataDigest`.

## Identity persistence

Modern proposals persist, together and without substitution:

* `ArtifactDigest`;
* `AuthorityMetadataDigest`;
* `BoundCandidateDigest`;
* bound-candidate mechanism ID/version;
* authority schema and registry bindings; and
* the exact authority-object references required by ADR-0012.

Legacy `candidate-v1` remains an artifact identity and is explicitly
distinguished from a modern bound identity. No field named CandidateDigest may
be interpreted as modern authority identity without its mechanism/version.

## Approval persistence

Approval persists the exact modern `BoundCandidateDigest`, binding mechanism
version, approval time, actor/mechanism identity where supported, and optional
human reason. It does not persist `eligibility=true`, adequacy, authorization,
or any equivalent derived assertion. A legacy approval remains legacy and
cannot be promoted. Any artifact or authority interpretation mutation creates
a new bound identity and invalidates old approval applicability.

## EligibilityRecord persistence

ADR-0013 supplies immutable `EligibilityRecord` material. Persistence MAY
store a current-record reference plus bounded or external immutable history;
it MUST NOT rewrite historical records. Every record binds:

* BoundCandidateDigest and AuthorityMetadataDigest;
* AuthorityRule and all registry identities;
* RootTrustConfiguration identity;
* SecurityContextIdentity and current context identity;
* baseline, composition, evidence, certification, compatibility, and
  provenance identities;
* CurrentAuthorityFact identities/provenance;
* EvaluationTime and validity deadline;
* final result; and
* complete canonical PredicateID/reason trace.

The existence of a historical `ELIGIBLE` record does not mean current
eligibility or authorization.

## Eligibility currentness and authorization

Persistence distinguishes `lastEvaluation.result` from the current
`EligibilityState`. Current state is bound to the exact current
EligibilityRecord identity and candidate identity. Re-evaluation creates a
new immutable record; stale, revoked, or context-invalid records become
`STALE`/`INVALIDATED` projections without rewriting history.

`AUTHORIZED` is a separate record/projection owned by ADR-F and must carry
the exact EligibilityRecord identity, BoundCandidateDigest, authorization
decision identity, and decision time. An eligible proposal with no authorization
remains unauthorized; an authorization referring to another candidate is
invalid.

## Materialization, runtime, and behavioral state

Materialization records backend realization separately from authority. A
created Landlock/PodLock object, NetworkPolicy, SeccompProfile, or Ready
resource is not authorization and does not imply `ACTIVE`.

Runtime state is persisted separately and is populated by ADR-G/H from
backend evidence and revalidation. Revocation or loss of a mandatory current
fact preserves historical eligibility while projecting `ACTIVE_UNAUTHORIZED`
for known invalidity or `SUSPENDED_UNKNOWN` for unavailable facts.

Behavioral verification is an independent immutable verification/evidence
record or status dimension. `ACTIVE` and `BEHAVIORALLY_VERIFIED` are never
collapsed.

## Specification, status, and external objects

The proposal spec contains candidate intent and exact rendered artifacts. Its
security-significant authority interpretation is immutable by identity, not
by mutable status text. Status contains observed lifecycle projections,
current references, and diagnostics. Large authority objects, registries,
trust policies, evidence, certifications, and EligibilityRecords are external
immutable objects/references when reuse, size, or independent lifecycle
requires it.

Status Conditions are optional projections for consumers. They are not the
primary semantic source of truth and cannot override exact typed state or
identity references.

## Mutation and generation rules

Changing artifacts, AuthorityMetadata, a bound reference, registry binding,
or context binding creates a new BoundCandidateDigest. It invalidates approval
and current eligibility applicability; it does not mutate an approved
interpretation in place. Status updates MUST never rewrite the approved
identity or historical EligibilityRecord.

Generation follows:

`evidence/trace → DERIVED_ADVISORY → proposal → AuthorityMetadata bound → REVIEWABLE_CANDIDATE`.

Proposal creation and review do not set eligibility. Evaluation is an explicit
ADR-D operation or controlled lifecycle action whose resulting immutable
record is persisted by ADR-E. Apply is not the only place evaluation may
occur, and ADR-F remains the enforcement gate.

## Legacy migration

Existing objects lacking modern authority metadata are recognized by absence
of the bound mechanism/schema and remain:

`LEGACY / AUTHORITY_UNKNOWN / ADVISORY_ONLY`.

They receive no default AuthorityRule, registry, TrustPolicy, synthetic
AuthorityMetadata, eligibility, or authorization. Regeneration, new binding,
and reapproval are required before modern authority can be claimed.

## Schema and resource evolution

The first implementation SHOULD prefer a versioned extension of the existing
proposal API only when its schema can represent the orthogonal dimensions and
immutable references without ambiguity. A new API/CRD version is required if
the existing schema cannot preserve those invariants. Separate immutable
AuthorityMetadata, EligibilityRecord, AuthorizationRecord, registry, trust,
and evidence resources are justified by independent lifecycle, reuse, size,
or audit history; they are not required merely because RFC nouns exist.

Conceptual impact of current fields:

| Existing field | Target treatment |
|---|---|
| artifact strings | `UNCHANGED` as exact rendered artifact inputs |
| `GeneratedAt`, `HistoryUsed` | `UNCHANGED`; diagnostic/provenance, not authority by themselves |
| `ApprovedCandidateDigest` | `DEPRECATED` for legacy-only use; modern approval stores BoundCandidateDigest |
| `ApprovalMechanismVersion` | `NEW_VERSION_REQUIRED` for modern binding mechanism |
| `ApprovalState`, `Reason`, `UpdatedAt` | `UNCHANGED` as review projection, extended by orthogonal status dimensions |
| authority/evaluation references | `NEW_FIELD_REQUIRED` or immutable external reference |
| EligibilityRecord history | `NEW_ARTIFACT` or immutable external resource/reference |

## Status applicability and concurrency

BoundCandidateDigest, not only Kubernetes generation, controls whether a
status projection applies to the candidate. A status for C1 cannot be read as
status for C2. `observedGeneration` MAY supplement this check but cannot
replace digest identity.

Each lifecycle dimension has one conceptual owner: proposal generation owns
reviewable artifacts; review/approval owns ReviewState and approval identity;
ADR-D owns EvaluationResult material; ADR-F owns AuthorizationState; ADR-G
owns materialization observations; ADR-G/H own runtime and behavioral facts;
ADR-H owns currentness/revalidation projections. Concurrent writers use the
status-subresource's optimistic concurrency and merge disjoint dimensions;
stale writes are rejected and retried, never blindly overwrite revocation or
another owner's state.

## Invalid combinations

The following are domain-invalid and MUST be rejected or projected as an
invalid/stale state, never interpreted as authority:

* `REJECTED + AUTHORIZED`;
* unknown/invalid current eligibility + newly authorized;
* `NOT_MATERIALIZED + ACTIVE`;
* legacy + modern eligible/authorized;
* approval for C1 + EligibilityRecord for C2;
* authorization for C1 + artifacts for C2;
* materialized/Ready + authorization absent;
* historical eligibility used as current authorization.

Some combinations may exist transiently during asynchronous reconciliation,
but they remain invalid semantic projections and cannot authorize enforcement.

## History, CLI, and compatibility

Immutable EligibilityRecords and AuthorizationRecords are the audit history;
Kubernetes Events and TrainingHistory are diagnostic/evidence channels and do
not replace them. Existing `review`, `approve`, and `apply-proposal` commands
retain legacy behavior until their owning ADRs add modern identity handling.
Modern approval must bind BoundCandidateDigest, and future evaluation/status
commands may expose records without changing approval meaning. Existing
scripts and objects cannot silently gain authority.

## Minimal migration

The smallest safe migration is:

1. preserve existing proposals as legacy advisory objects;
2. allow modern proposals to carry immutable AuthorityMetadata references and
   BoundCandidateDigest;
3. persist immutable EligibilityRecord material and currentness separately;
4. keep internal bounded Seccomp observation advisory until RFC-0003
   eligibility succeeds; and
5. have the later ADR-F gate refuse authorization when modern identity,
   eligibility, or currentness is absent.

No future backend feature is required before legacy objects are safely
classified.

## Rollout modes and compliance boundary

The product has one explicit, operator/product-controlled rollout mode. A
candidate cannot select or alter it. The modes are:

* `LEGACY_ENFORCEMENT_MODE`: pre-RFC behavior for explicitly recognized
  legacy proposals. Legacy `candidate-v1` identity and documented legacy
  apply behavior may remain, but RFC-0003 authority metadata is not
  interpreted and RFC-0003-compliant enforcement MUST NOT be claimed.
* `AUTHORITY_ADVISORY_MODE`: modern AuthorityMetadata, digests, approval
  identity, EligibilityRecords, and traces may be persisted and displayed.
  Modern proposals remain advisory/non-enforceable through the modern path;
  eligibility is not authorization. If legacy compatibility remains enabled,
  it is an explicitly recognized legacy path only. A modern proposal MUST NOT
  fall through to that path.
* `RFC0003_ENFORCEMENT_MODE`: available only after modern binding,
  evaluation, persistence, and the ADR-F authorization/apply gate are active
  and verified on every enforcement path covered by the compliance claim.
  Only this mode may claim RFC-0003-compliant enforcement.

The presence of AuthorityMetadata, BoundCandidateDigest, approval,
EligibilityRecord, an `ELIGIBLE` result, modern status fields, materialized
artifacts, backend Ready, or runtime Active is individually and collectively
insufficient to claim RFC-0003 enforcement compliance. Compliance requires
an active ADR-F-compatible gate that verifies exact approved, evaluated,
authorized, and applied identities.

Before ADR-F is active, a modern proposal reaching `apply-proposal` is
`ENFORCEMENT_UNAVAILABLE / ADVISORY_ONLY` and fails closed. It is not retried
through legacy approval semantics. A legacy proposal may use the explicitly
identified legacy compatibility path while that path remains supported, but
modern identity cannot be downgraded into it. Conversely, a legacy proposal
cannot acquire modern authority from a new evaluator result; regeneration,
modern binding, and reapproval are mandatory. Mixed candidate-v1 and bound
candidate identities are invalid and non-enforcing.

The rollout capability is explicit and non-candidate-controlled. It MUST NOT
be inferred from CRD fields, EligibilityRecords, status conditions, backend
installation, or Ready state. A safe sequence is: E0 persistence support
exists while modern creation is disabled; E1 enables modern advisory records
with no modern enforcement; F implements and certifies ADR-F; then
EF-ACTIVE enables `RFC0003_ENFORCEMENT_MODE`. ADR-0014 or ADR-0013 being
implemented is not enforcement compliance. No release may claim that
compliance until ADR-F is active on every claimed enforcement path.

If mode capability is unavailable, the result is an authorization-infrastructure
unavailable condition distinct from proposal `INELIGIBLE` or eligibility
`UNKNOWN`; exact gate/error behavior belongs to ADR-F.

## Security invariants

1. Approval does not imply eligibility.
2. Eligibility does not imply authorization.
3. Authorization does not imply materialization.
4. Materialization does not imply active enforcement.
5. Active enforcement does not imply behavioral verification.
6. Every authority/evaluation status binds an exact candidate identity.
7. Historical EligibilityRecords are immutable.
8. Legacy proposals cannot obtain modern authority without regeneration.
9. Candidate mutation invalidates approval and evaluation applicability.
10. Current authority is never derived from historical record existence alone.
11. Backend Ready is not authorization or Active.
12. Status projections never redefine RFC semantics.
13. Concurrent status updates cannot erase a newer revocation or identity.
14. Authorization and applied artifacts must bind the same BoundCandidateDigest.
15. Modern authority persistence does not imply modern enforcement capability.
16. Modern enforcement fails closed until the ADR-F gate is active.
17. Modern proposals never fall back to legacy enforcement semantics.
18. Legacy proposals never gain modern authority without regeneration and reapproval.
19. RFC-0003 enforcement compliance is not claimed before ADR-F is active on every claimed path.
20. Backend readiness/materialization cannot substitute for rollout capability.
21. Rollout mode and capability are operator/product controlled, never candidate controlled.

## Handoffs

* **ADR-F** receives the exact approved BoundCandidateDigest, current
  EligibilityRecord identity/result, authorization prerequisites, and artifact
  identity material. ADR-F owns the authorization decision, apply-time gate,
  identity-equality verification, and enforcement rejection semantics.
* **ADR-H** receives current EligibilityRecord identity, validity/deadline,
  root/trust context, and runtime state hooks. It defines refresh scheduling,
  revocation re-evaluation, and remediation.
* ADR-G supplies backend/materialization and verification observations; ADR-E
  persists their projections without treating readiness as authority.

## Testing requirements

Future implementation work MUST include schema round trips and unknown-state
rejection; migration tests proving old approvals remain legacy; state tests
for approval/eligibility/authorization/materialization/active separation;
negative tests for stale records, candidate mismatch, legacy escalation,
backend Ready shortcuts, and artifact mutation; concurrency tests for
status-subresource optimistic updates; and envtest coverage for status
applicability and immutable identity references.

## Alternatives considered

* **Status-only extension:** rejected because a monolithic status projection
  cannot safely express immutable record identity and independent lifecycles.
* **Versioned proposal with everything embedded:** rejected as the default
  because large independently versioned authority/evidence objects become
  mutable or duplicated.
* **Separate resource for every RFC noun:** rejected as unnecessary resource
  proliferation; separate resources are used only where lifecycle, size,
  reuse, or audit requires them.
* **Hybrid proposal plus immutable references:** selected for identity safety,
  auditability, migration, and compatibility with the current proposal CRD.

## Consequences and open questions

This design preserves existing proposal artifacts and review workflow while
introducing explicit modern identity and orthogonal status projections. It
requires schema/version work and immutable-record persistence before governed
enforcement can consume modern authority. Concrete CRD fields, controllers,
storage, CLI UX, and physical remediation remain later decisions.
