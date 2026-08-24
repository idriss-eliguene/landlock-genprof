# ADR-0016: Backend and Verifier Adapter Architecture

Status: Proposed

Date: 2026-08-23

## Decision question

How do live runtimes and external APIs become canonical RFC-0003 evidence,
current facts, verification facts, and validated ApplyPlan steps without
manufacturing authority?

RFC-0003 and ADR-0010 through ADR-0015 remain authoritative. This ADR owns
the impure integration boundary only. It does not evaluate eligibility,
authorize mutation, or schedule revalidation.

## Decision

Use narrow typed adapter families around immutable canonical snapshots:

1. observation adapters collect raw observations;
2. evidence/provenance adapters canonicalize observations and artifacts;
3. current-fact adapters publish freshness-bound mutable facts;
4. verifier adapters execute only the exact registered verifier identity;
5. planning adapters build deterministic backend plan steps from explicit
   artifacts, target identity, semantics, and pre-state;
6. realization/runtime adapters report materialization, active state, drift,
   and mutation outcomes.

No single generic backend adapter owns all responsibilities. The authority
domain remains backend-neutral and imports no Kubernetes, SPO, PodLock, CRI,
or backend SDK types. Adapters are trusted to faithfully observe,
canonicalize, identify, transform, and execute declared operations, but ADR-D
and ADR-F remain the sole owners of eligibility and authorization.

## Immutable snapshot boundary

A mutable external object is first converted to a validated immutable snapshot
containing source identity, API/schema version, object identity, content
digest, observation time, scope, context, provenance, and backend semantic
identity. The snapshot, never the live object, is consumed by domain
evaluation. Mutation after snapshot creation cannot alter its meaning.

Canonical security semantics are typed and registry/version bound; authority
inputs are not `map[string]any`. Unknown security-significant fields are
unsupported or unknown and fail closed. Diagnostic unknown fields may be
retained as non-normative diagnostics. Unsupported external versions never
fall back to “latest”, best effort, or field dropping.

## Evidence and current facts

Evidence adapters may produce canonical evidence containing an evidence
reference/type, backend, observation scope, security-context identity,
coverage, provenance, validity, and payload digest. Evidence is not
completeness, adequacy, eligibility, or authorization.

Current-fact adapters produce ADR-0012-compatible facts with exact subject,
fact kind, source identity, observation time, freshness contract,
source epoch/version, and verification status. Examples include revocation,
backend availability, trust observations, runtime realization, drift, and
verifier service state. Polling and revalidation remain ADR-H responsibilities.

## Verification adapters

Verifier execution occurs before ADR-0013. The resolved
`VerifierSemanticIdentity` selects an implementation whose executable,
declarative semantics, or attested remote identity is content/version bound
to that identity. No fallback, “latest”, substitution, or result-schema
reinterpretation is permitted.

The resulting immutable VerificationFact binds verifier identity and version,
property, exact input and subject/candidate digest, backend, scope, context,
input digest, result, execution/provenance identity, time, validity, and
source. It cannot be replayed for another candidate, context, scope, or
property. Verifier adapters do not make eligibility decisions.

## SPO and internal Seccomp

SPO ProfileRecording, generated SeccompProfile, merged recordings, coverage
metadata, and reconciliation status are separate snapshots. `Ready` is
realization evidence only: it is not coverage, adequacy, eligibility, or
authorization. Consumed annotations such as
`spo.x-k8s.io/syscall-coverage` require an adapter/schema version, defined
parsing, absence/malformed handling, and provenance. Annotation presence is
never authority.

Merged syscall evidence retains each source identity, scope, and provenance.
A union of names never becomes container-lifetime coverage without explicit
RFC authority evidence.

Internal syscall, filesystem, network, and capability tracing records source,
binary/process scope, interval, lifecycle phases, context, provenance, and
known/unknown coverage. Bounded tracing remains advisory and cannot directly
produce enforcement authority.

Seccomp synthesis is separated into evidence, policy synthesis, and
authority. Canonical policy semantics include default action, architectures,
syscall actions, argument filters, errno, and provenance; YAML serialization
does not define or erase those semantics.

## PodLock/Landlock

Landlock observation, generated LandlockProfile, PodLock representation,
NRI adjustment, profile.json, hook execution, kernel seal activation, and
behavioral verification are distinct objects/facts. Profile creation or an
adjustment log is not Active.

PodLock TargetIdentity is supplied canonically for workload, container/binary,
namespace, profile, node/runtime environment, and attachment mechanism fields
that affect enforcement. Diagnostic UIDs are preconditions only unless they
distinguish the enforcement target. Pre-state classes include absent, exact
profile, differing profile, alternate active profile, and unavailable; ADR-G
exposes these to ADR-F as required predicates.

## NetworkPolicy and workload security

NetworkPolicy semantics are canonicalized across namespace, pod selector,
policy types, ingress/egress, peers, IP blocks, ports, protocols, and
ownership/binding identity. Plan identity may retain exact authored bytes
through ADR-0011 while also binding canonical Kubernetes-semantic form for
target and pre-state validation; byte equality and semantic equivalence are
not conflated.

Capability and securityContext changes are decomposed into explicit semantic
steps: capability add/drop, privileged, escalation, Seccomp attachment, and
future AppArmor/SELinux dimensions. A patched manifest is not an opaque
security mutation. Labels, attachments, restarts, recreate/rollout actions,
and helper resources affecting confinement are explicit ApplyPlan steps.
Logical workload identity is distinct from ephemeral object UID; UID may be
a precondition, not the workload authority identity.

## Planning and realization

I/O observation is separated from pure planning:

`ObservePreState() -> immutable TargetPreState`

`BuildBackendPlan(authorized artifact, TargetIdentity, pre-state,
registered backend semantics) -> deterministic plan`

The same explicit inputs produce the same semantic plan. Security-significant
ordering and dependencies are preserved from ADR-0015. No ambient lookup is
performed by pure planning.

The mutation adapter receives only a validated PlanStep and current gate
permit/snapshot. It executes exactly that declared operation and returns one
of `APPLIED`, `ALREADY_SATISFIED`, `FAILED`, or `UNKNOWN_OUTCOME`. Timeout or
transport success does not prove application or Active; exact re-observation
is required. Ownership/provenance is part of idempotency and pre-state, and
external drift is reported as a fact rather than silently overwritten.

Adapters cannot resolve trust, evaluate eligibility, authorize, widen scope,
change targets, add helpers, omit steps, reorder security-significant steps,
or apply unrepresented security defaults. Multi-backend ordering comes only
from the validated ApplyPlan and cannot bypass ADR-F.

## Materialized, active, and verified

`MATERIALIZED` means the desired representation is realized in the backend.
`ACTIVE` means the intended runtime target is actually subject to it. These
require distinct backend observations; Ready, API acceptance, object
existence, NRI adjustment, or hook execution alone may be insufficient.
`BEHAVIORALLY_VERIFIED` is a separate VerificationFact and never implies
eligibility, authorization, or Active.

Positive and negative probes bind exact test specification, subject, context,
expected result, and execution identity. A governed EACCES experiment can
provide baseline, realization, active, and verification evidence as separate
facts, but cannot establish universal Landlock correctness.

The certified Seccomp A/B/C/D startup experiment remains a required
regression: startup outcome, profile identity, attachment, container state,
context, and provenance are retained; generic `RunContainerError` is not a
semantic substitute.

## Defaults, versions, and errors

Security-significant backend defaults become explicit canonical semantics
before authorization. Every adapter contract binds backend ID, adapter
semantic version, supported external API/schema versions, canonicalization
version, TargetIdentity version, pre-state semantics version, and realization
semantics version. Security semantic changes require a version change.

Typed adapter outcomes distinguish `MALFORMED_INPUT`, `UNSUPPORTED_VERSION`,
`UNSUPPORTED_SECURITY_FIELD`, `SOURCE_UNAVAILABLE`, `NOT_FOUND`, `CONFLICT`,
`BACKEND_FAILURE`, and `UNKNOWN_OUTCOME`, with normalized mapping to ADR-0012
through ADR-0015. Raw API errors remain diagnostics, not authority semantics.

## Current project mapping

| Current artifact/state | Canonical role |
|---|---|
| filesystem/network/syscall/capability trace | OBSERVATION → EVIDENCE |
| ProfileRecording | EVIDENCE/PROVENANCE snapshot |
| SPO SeccompProfile | DERIVED_ARTIFACT and plan input |
| SPO coverage annotation | versioned EVIDENCE metadata |
| TrainingHistory | audit/diagnostic history, not authority |
| LandlockProfile | DERIVED_ARTIFACT and plan input |
| NRI adjustment/profile.json/hook | realization evidence/current fact |
| Landlock seal activation | ACTIVE evidence when target-bound |
| NetworkPolicy | canonical DERIVED_ARTIFACT and plan steps |
| patched workload manifest | decomposed PLAN_INPUT, never opaque authority |
| securityContext/capabilities | explicit PLAN_INPUT/mutation steps |
| container startup state | CURRENT_FACT/evidence |
| behavioral probe result | VERIFICATION_FACT |
| backend mutation response | MUTATION_RESULT |

## Security invariants

1. Live mutable objects are never direct authority inputs.
2. Ready never implies eligibility or authorization.
3. Adapters cannot widen authority semantics.
4. Unknown security fields fail closed.
5. Adapter semantic versions are identity-bound.
6. Observation produces evidence, not authority.
7. Verification executes only the exact verifier identity.
8. Verification facts bind candidate, subject, context, and property.
9. Plan generation is deterministic from explicit inputs.
10. Mutation adapters execute only validated steps.
11. Unknown write outcomes are not success.
12. Materialized does not imply Active.
13. Active does not imply behavioral verification.
14. Drift produces facts, not silent reauthorization.
15. Security defaults become explicit semantics.
16. Security-significant helpers are explicit plan steps.
17. Adapters do not evaluate or authorize.
18. Multi-backend orchestration cannot bypass ADR-F.

## Alternatives

## Normative semantic closure

Every backend contract is identified by an immutable
`BackendSemanticRegistry` containing backend ID, registry ID/version/digest,
adapter version, supported external schemas, field-classification schema,
effective-defaulting schema, target and pre-state schemas,
mutation-decomposition schema, and realization-state schema. A security
semantic change requires a new registry identity. Fields are classified by
that exact registry as `SECURITY_SIGNIFICANT`, `DIAGNOSTIC`,
`STRUCTURAL_NON_SECURITY`, or `UNSUPPORTED_RESERVED`. An unregistered field
may be ignored only when the registry proves it cannot affect security;
otherwise the result is unsupported security semantics. Newer schemas are
never reinterpreted with older registries.

Authored bytes remain part of `ArtifactDigest`, while backend snapshots also
carry one registry-defined canonical `EffectiveSemantics` representation,
including every security-relevant default and its registered source. Local
or ambient defaults are forbidden; unknown required defaults are
`UNKNOWN`/unsupported. Identical authored objects under the same registry
therefore produce identical effective semantics.

The registry's versioned `MutationDecompositionSchema` determines canonical
security mutation units. It distinguishes PodLock/Landlock binding, Seccomp
attachment, capabilities, privileged/escalation settings, registered
labels/annotations, restart/replacement, helpers, and other registered
confinement changes. A patched manifest is never an opaque security step.
Semantic mutations differ from transport requests: one request may contain
several units only when every unit is explicit, currently permitted,
target-equivalent, order-preserving, and exactly represented by the request.
The closed `BatchEquivalence` result is `BATCH_EQUIVALENT`,
`BATCH_NOT_EQUIVALENT`, or `BATCH_UNKNOWN`; only the first permits batching,
with per-unit failure/unknown projection. Batching adds no authority.

Restarts and rollouts are explicit registered mutation units whenever they
can change confinement or create an unconfined interval. A hidden restart is
non-conforming. `LogicalTargetIdentity` is distinct from
`RuntimeInstanceIdentity`: workload, namespace, container, binary, selector,
and attachment identify the logical target; Pod UID, container ID, and
resourceVersion are runtime preconditions unless the registry explicitly
requires them. Target components and canonical equivalence are registry
defined.

Each registry supplies a deterministic `PreStateSchema` with at least
`ABSENT`, `EXACT_MATCH`, `OWNERSHIP_CONFLICT`, `CONTENT_CONFLICT`,
`BOUND_TO_OTHER_POLICY`, `ACTIVE_UNDER_OTHER_POLICY`, `UNAVAILABLE`,
`MALFORMED`, and `UNSUPPORTED`. `OwnershipIdentity` has the closed relation
`OWNED_BY_CURRENT_AUTHORITY`, `EQUIVALENT_OWNERSHIP`, `OWNED_BY_OTHER`,
`UNOWNED`, or `UNKNOWN`; payload equality alone never establishes ownership.

Each registry defines separate `MaterializationCriterion` and
`ActiveCriterion`. Implementations cannot choose lifecycle thresholds
locally. `MATERIALIZED` requires the exact authorized representation for the
exact target; `ACTIVE` requires proof that the logical runtime target is
subject to it. Missing active evidence is `UNKNOWN`; Ready or API existence
only satisfies a criterion when the registry explicitly says so.

Verifier execution is a deterministic function of declared canonical inputs
and explicitly bound execution-environment inputs. Ambient Kubernetes,
filesystem, network, policy, environment, clock, or mutable remote lookups
are forbidden unless first captured as immutable provenance-bound inputs.
Remote endpoint/name is insufficient without current implementation identity.
Outcome observations and causal verification are distinct: an EACCES result
proves only the observed outcome. A Landlock-specific denial requires causal
evidence satisfying the bound verifier's attribution requirement; otherwise
that causal result is `UNKNOWN`.

Reconciliation follows: observe desired and actual, emit drift/current facts,
construct or revalidate a canonical PlanStep, obtain fresh ADR-0015 permission,
then mutate. Desired-state mismatch alone is not authority. If current
authorization is `UNKNOWN` or `DENIED`, authority-changing repair, deletion,
replacement, or cleanup does not occur.

Adapter outcomes are closed: `MALFORMED_INPUT`, `UNSUPPORTED_VERSION`,
`UNSUPPORTED_SECURITY_SEMANTICS`, `SOURCE_UNAVAILABLE`, `NOT_FOUND`,
`CONFLICT`, `BACKEND_REJECTED`, and `UNKNOWN_OUTCOME`. A known rejection is
not unknown; a write timeout is never `APPLIED`. Transport success never
independently establishes Materialized, Active, or BehaviorallyVerified.

The following invariants extend G1–G18: registry-defined field
classification; canonical effective defaults; registry-defined manifest
decomposition; semantic identity independent of transport shape; exact
batch equivalence; registry-defined target/pre-state/ownership semantics;
registered Materialized/Active criteria; explicit verifier inputs and causal
attribution; fresh ADR-F permission for reconciliation; and no authority from
drift or transport success.

Backend logic in the evaluator/gate, generic untyped adapters, one giant
adapter per backend, and an external policy engine owning semantics were
rejected. The selected narrow typed boundaries minimize the trusted code at
the impure edges while preserving canonical domain ownership.

## Handoffs and tests

ADR-F receives canonical TargetIdentity, backend semantics/version, pre-state,
deterministic steps, helper steps, ownership, provenance, and mutation
results. ADR-H receives drift, availability, materialization/active evidence,
revocation consequences, and runtime facts; it owns scheduling and
remediation. ADR-G owns adapter implementation and does not schedule
revalidation.

Future tests include canonical/golden vectors, unsupported-field/version
negatives, target/pre-state and ownership tests, verifier substitution and
replay tests, SPO/PodLock/NetworkPolicy/securityContext integration, materialized
versus active tests, unknown write outcomes, A/B/C/D Seccomp differential
tests, and cross-language canonical semantic vectors.

## Threat outcomes

Ready shortcuts, live-object mutation, unknown-field dropping, latest-version
fallback, verifier substitution, evidence replay, scope widening, hidden
restarts/helpers, target or ownership substitution, adapter reordering,
timeout-as-success, transport-success-as-Active, silent drift overwrite,
backend defaults, and ADR-F bypass are prohibited by the contracts above.
Behavioral success remains verification evidence; EACCES and startup failures
retain their bounded semantics.

## Claim boundary

This ADR defines adapter contracts and canonical integration boundaries only.
It does not claim implementation, runtime enforcement, verifier deployment, or
backend support. Concrete Go types, CRDs, storage, controller mechanics,
locking, and backend technology remain implementation concerns.
