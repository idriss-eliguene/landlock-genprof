# RFC-0003 Implementation Roadmap

Status: Implementation roadmap

This document turns accepted RFC-0003 and ADR-0010 through ADR-0017 into
incremental Go work. It does not change normative architecture. Every phase
must run the invariant regression gate for the surfaces it touches; a
`VIOLATED` result is a stop condition.

## Current repository baseline

The repository currently has proposal persistence and validation in
`internal/proposal`, legacy digesting in `internal/proposal/digest.go`, CLI
review/approve/apply paths in `cmd/landlock-genprof`, Kubernetes mutation
helpers in `internal/k8s`, observation and synthesis in `internal/observation`
and `internal/policy`, provenance/evidence in `internal/evidence` and
`internal/semantic`, SPO import/backend packages, exporters for Seccomp,
PodLock, NetworkPolicy, capabilities and securityContext, and shell/envtest/
unit coverage under `test/e2e`, `internal/*`, and `cmd/*`.

Current apply validates legacy `ApprovedCandidateDigest`, builds an in-memory
plan, waits for readiness, rereads proposal state, recomputes the legacy
digest, and applies artifacts sequentially. This remains legacy behavior until
the modern gate is active.

## Dependency graph and rollout

The critical path is:

`P0 domain → P1 canonical binding → P2 registries/resolution → P3 evaluator →
P4 advisory persistence → P5 authorization/apply gate → P6 adapters →
P7 revalidation/remediation → P8 enforcement activation → P9 certification`.

P2 registry work, P3 evaluator test vectors, and P6 adapter snapshot work can
proceed in parallel after P1. P4 may ship only in `AUTHORITY_ADVISORY_MODE`.
P5 must gate every claimed modern mutation before
`RFC0003_ENFORCEMENT_MODE` is enabled. P7 may develop current-fact producers
in parallel with P6, but activation depends on P5 and P6 contracts.

## Phase P0 — pure authority domain foundation

**Goal:** Add backend-neutral immutable types and constructors without changing
current enforcement.

**Sources/dependencies:** RFC-0003, ADR-0010; none.

**Likely surfaces:** new `internal/authority` (or equivalent), typed IDs,
references, result enums, CurrentAuthorityFact, SecurityContextIdentity,
AuthorityMetadata, rule/evidence/certification models. Reuse existing
`internal/evidence`, `internal/semantic`, and profile types only through
explicit mapping constructors.

**Tasks:** define absent/empty/unknown/false/invalid distinctions; immutable
value construction; validation boundaries; typed backend/verifier/registry
references; no Kubernetes imports. Do not remove legacy types.

**Proof:** unit construction tests, immutability tests, malformed-input tests,
cross-package dependency test proving no backend imports. Acceptance requires
no ambient lookup or clock and no positive default for missing data.

## Phase P1 — canonical binding and digest vectors

**Goal:** Implement ADR-0011 identity exactly.

**Surfaces:** `internal/proposal/digest.go` migration plus new canonical
package; preserve candidate-v1 as a separate legacy path.

**Tasks:** RFC 8785/JCS canonicalization; domain-separated envelopes; typed
ArtifactDigest, AuthorityMetadataDigest, BoundCandidateDigest; authority and
binding mechanism versions; canonical set/map/list rules; explicit empty and
absent forms; golden vectors and negative collision tests.

**Proof:** golden byte/digest vectors, reordered-map/set properties, changed
authority/artifact tests, cross-language vectors, legacy-v1 isolation tests.

**Stop:** any modern digest equals or is accepted as legacy candidate-v1, or
any mutable/status field changes the bound identity unexpectedly.

## Phase P2 — registries, resolution, and verification facts

**Goal:** Implement ADR-0012 and ADR-0016 registry-bound resolution inputs.

**Tasks:** typed BackendSemanticRegistry, AuthorityRule/CompatibilityRule,
TrustPolicy/root identity, verifier semantic registry, source aggregation,
CurrentAuthorityFact aggregation, freshness boundaries, verification-fact
binding, immutable resolution bundles, and typed adapter outcomes.

**Surfaces:** new `internal/authority/registry`, `internal/resolution`,
`internal/verification`; adapt `internal/spoimport`, `internal/spobackend`,
and provenance packages.

**Proof:** split-brain/source precedence tables, duplicate/conflict/stale fact
tests, verifier replay/substitution tests, registry version tests, unknown
field/defaulting tests. Resolution never claims trust; facts never execute
verifiers inside the evaluator.

## Phase P3 — pure eligibility evaluator

**Goal:** Implement ADR-0013 as a deterministic domain service.

**Tasks:** explicit five-input evaluator and EvaluationTime; complete
mandatory predicate evaluation; canonical PredicateID ordering; predicate
aggregation `NEGATIVE > UNKNOWN > POSITIVE`; closed ReasonCodeRegistry;
NormalizeReasons specificity and deduplication; immutable EvaluationResult,
EvaluationTrace, and EligibilityRecord material; backend envelope and
certification consumption from facts only.

**Surfaces:** new `internal/eligibility`; adapters only at boundaries.

**Proof:** RFC totality tables, predicate/unit tests, property tests, golden
trace vectors, Go/Rust differential vectors, historical replay tests, and the
bounded Seccomp counterexample. No ambient lookup, clock, cache, resolver, or
verifier execution is permitted.

## Phase P4 — lifecycle persistence and advisory migration

**Goal:** Persist modern identity and evaluation without granting enforcement.

**Surfaces:** `internal/proposal`, store/envtest tests, existing proposal API
projection, CLI review/approve output.

**Tasks:** persist AuthorityMetadata reference/digest, ArtifactDigest,
BoundCandidateDigest, immutable EligibilityRecord references/history, current
projection, orthogonal review/eligibility/authorization/materialization/runtime
states, and schema/version discriminators. Preserve legacy objects as
LEGACY/ADVISORY_ONLY.

**Rollout:** E0 persistence plumbing disabled; E1 modern records in
`AUTHORITY_ADVISORY_MODE`; modern proposals cannot call legacy apply.

**Proof:** round-trip/envtest, candidate mismatch, stale pointer, legacy
escalation, approval-not-eligibility, and advisory apply fail-closed tests.

## Phase P5 — AuthorizationRecord and ADR-0015 gate

**Goal:** Make every modern security mutation require current exact authority.

**Surfaces:** new `internal/authorization`, `internal/applyplan`,
`cmd/landlock-genprof/apply_proposal.go`, `internal/k8s/apply.go`, restart and
readiness helpers.

**Tasks:** immutable AuthorizationRecord and ApprovalRecord binding;
authorization issuance; immutable ApplyPlan and PlanStep; TargetIdentity and
TargetPreState; per-mutation `PERMITTED|DENIED|UNKNOWN|INVALID`; exact
artifact dispositions; wait/retry invalidation; batching/compensation rules;
CAS/lease-equivalent identity protection; legacy path isolation.

Inventory every current side effect: Seccomp resource, SPO readiness/binding,
PodLock, NetworkPolicy, capabilities/securityContext, labels, restart/recreate,
helper resources, filesystem installation, and cleanup. Each becomes an
explicit PlanStep or remains legacy-only until gated.

**Proof:** mutation boundary tests, candidate/artifact/target substitution,
capability/root/revocation races, readiness waits, unknown writes, partial
apply, retry, concurrent plans, helper/reorder/batch tests, and envtest hooks.

**Stop:** any modern proposal can reach old apply semantics or any side effect
has no per-mutation gate.

## Phase P6 — backend and verifier adapter tracks

**Goal:** Convert live systems to canonical snapshots and validated steps.

**SPO/Seccomp:** adapt ProfileRecording, SeccompProfile, coverage metadata,
merge provenance, profile identity, attachment, startup and runtime evidence;
register effective semantics and Materialized/Active criteria. Preserve
bounded internal syscall observation as advisory.

**PodLock/Landlock:** map observation, LandlockProfile, NRI adjustment,
profile.json, hook, seal activation, runtime target, ownership and pre-state;
no profile creation or log implies Active.

**NetworkPolicy:** canonicalize namespace/selectors/types/peers/IPBlocks/
ports/protocols/defaults and dataplane evidence; API acceptance is at most
Materialized.

**Capabilities/securityContext:** decompose add/drop, privileged,
escalation, Seccomp attachment, labels, restart and workload binding into
registry-defined semantic steps.

**All tracks:** immutable snapshots, versioned defaults/unknown-field rules,
logical target versus runtime instance, ownership/pre-state, typed mutation
outcomes, no hidden helpers/restarts, no adapter authority decisions.

**Proof:** golden external-object vectors, unsupported-field/version tests,
target/pre-state/ownership tests, verifier purity/replay, integration tests,
and cross-language canonical mappings.

## Phase P7 — currentness, revalidation, and remediation

**Goal:** Implement ADR-0017 after initial gate integration.

**Surfaces:** new `internal/revalidation`, `internal/remediation`, current
projection store/status, fact collectors, runtime projector.

**Tasks:** `CURRENT_VALID|CURRENT_INVALID|CURRENT_UNKNOWN`; inclusive expiry;
lineage/epoch supersession; revocation/root/trust/baseline/provenance changes;
context/image/kernel/runtime drift; target replacement; backend drift and
availability; event-loss-safe resync; `ACTIVE_UNAUTHORIZED`,
`SUSPENDED_UNKNOWN`, partial multi-backend aggregation; explicit
RemediationPolicy/Authorization and deterministic RemediationPlan; every
remediation step through P5.

**Proof:** expiry boundaries, revocation/unknown, stale Active, replacement
runtime, missed event, capability disable, drift, remediation policy revocation,
partial remediation, and multi-backend tests.

## Phase P8 — enforcement activation

**Goal:** Enable `RFC0003_ENFORCEMENT_MODE` only after P5/P6/P7 coverage.

**Tasks:** explicit non-candidate-controlled capability; enumerate every
claimed enforcement path; remove accidental modern-to-legacy fallback;
preserve legacy compatibility only for explicitly legacy candidates; update
CLI commands to distinguish advisory, unauthorized, unknown, and operational
failure without changing semantics.

**Proof:** feature-capability tests, old CLI against modern proposals,
capability-disable races, all-backend gate coverage, and release checklist.

**Stop:** compliance cannot be claimed while any in-scope mutation path is
ungated.

## Phase P9 — certification and complete E2E

**Goal:** establish executable evidence for the full architecture.

**Proof ladder:** L0 compile/type safety; L1 unit; L2 negative security; L3
properties; L4 canonical golden vectors; L5 evaluator differential vectors;
L6 envtest/persistence; L7 backend integrations; L8 apply-gate adversarial
tests; L9 multi-backend E2E; L10 expiry/revocation/drift E2E; L11 certified
Seccomp A/B/C/D regression; L12 complete invariant traceability audit.

The permanent counterexample is bounded binary observation without startup/
bootstrap and container-lifetime authority: known missing coverage is
`INELIGIBLE`, genuinely unavailable mandatory coverage is `UNKNOWN`, and it
never becomes ELIGIBLE through approval, Ready, trust, digest, apply,
behavioral probe, composition, or historical authorization.

## Invariant traceability matrix

The following matrix is the implementation ownership index. Each row has at
least one required proof; phase gates must report `PRESERVED`, `STRENGTHENED`,
or `VIOLATED` with evidence.

| Source IDs | Invariant summary | Owner / phase | Proof obligation | Failure behavior |
|---|---|---|---|---|
| RFC-0003 predicates | observation, proof, inference, verdict remain distinct; eligibility is not authorization | `internal/eligibility` P3 | RFC totality/golden traces | INELIGIBLE/UNKNOWN |
| RFC-0003 envelope/scope | backend envelope, scope, coverage, completeness, adequacy, compatibility are exact | P2/P3 | Seccomp counterexample and predicate tables | negative/unknown |
| ADR-0010 domain | orthogonal authority objects and explicit negative/unknown states | P0/P4 | constructors and round-trip tests | reject malformed state |
| ADR-0011 binding | exact artifact + metadata → BoundCandidateDigest; canonical/JCS/domain separation | P1 | golden/differential vectors | binding invalid |
| ADR-0012 resolution | configured source authority, conflict ambiguity, root identity, immutable bundle, fresh facts | P2 | source/conflict/freshness tables | AMBIGUOUS/UNAVAILABLE/UNKNOWN |
| ADR-0013 D1–D12 | pure evaluator, explicit time, deterministic aggregation, no ambient lookup, unknown cannot improve | P3 | property, replay, cross-language tests | UNKNOWN/INELIGIBLE |
| ADR-0014 E1–E19 | approval/eligibility/authorization/materialization/runtime orthogonal; legacy isolated; exact candidate status | P4 | envtest/migration/state tests | advisory/invalid |
| ADR-0015 F1–F27 | exact AuthorizationRecord, per-mutation gate, plan identity, target/pre-state, partial/retry/concurrency safety | P5 | adversarial apply matrix | no mutation |
| ADR-0016 G1–G18 | immutable snapshots, registry semantics, adapter limits, realization distinction, unknown write safety | P2/P6 | golden/integration/adapter tests | unsupported/unknown |
| ADR-0016 G19–G34 | registry-owned fields/defaults/decomposition, batching, target/pre-state, Active criteria, verifier purity, attribution, reconciliation, transport boundary | P2/P6/P7 | cross-language and backend vectors | fail closed |
| ADR-0017 H1–H24 | immutable history, current projection, expiry/revocation/drift, unknown, replacement, remediation and event-loss safety | P7 | time/revocation/drift/E2E tests | suspended/unauthorized |

The grouped rows cover every numbered ADR invariant; implementation issues
must reference the exact source ID when a phase regression occurs.

## Global invariant regression gate

Every coding change records the touched architectural surfaces and reports:

`INVARIANT / SOURCE / STATUS / EVIDENCE`.

Changes crossing binding, evaluator, persistence, authorization, backend
mutation, or currentness boundaries run the broader security regression set.
Compile-time ownership checks, construction-time validation, evaluation-time
purity, persistence identity checks, apply-time gates, runtime freshness, and
cross-cutting differential tests are all required at their respective phases.

## Legacy and migration strategy

Existing candidate-v1 proposals and approvals remain explicitly legacy.
Modern metadata may first ship advisory-only. No modern record, ELIGIBLE
result, Ready state, materialization, or historical authorization grants
enforcement before P5 and the explicit capability gate. Modern proposals never
fall through to legacy apply. Regeneration, modern binding, evaluation, and
reapproval are required for migration.

## Earliest safe milestones

**First executable vertical slice:** P0–P3 for one in-memory canonical
candidate and evaluator, including golden traces and the Seccomp counterexample.

**First advisory integration:** P4 with persisted modern identity and
EligibilityRecord in `AUTHORITY_ADVISORY_MODE`; review/explain may consume it,
but apply cannot enforce it.

**First RFC-0003 enforcement milestone:** P5 plus one fully enumerated backend
path from P6, currentness checks from P7, and certification of every mutation
in that claimed scope. Broader multi-backend compliance waits for all claimed
paths.

## Architecture blockers

None identified from repository inspection. If implementation exposes a
requirement that contradicts RFC-0003 or ADR-0010–0017, stop and report
`ARCHITECTURE_BLOCKER` with the exact source section, counterexample, and
smallest unresolved question rather than changing semantics in code.

