# RFC-0003 — Observation, Coverage, Adequacy, and Enforcement Authority

## Status

Draft normative specification. This document defines semantics only.

## Core rule

Observation, derivation, approval, realization, and verification are distinct
claims. The following inference is forbidden:

NOT_OBSERVED(s) => UNNECESSARY(s) => SAFE_TO_DENY(s)

## Terminology and domains

An execution event is e = (t,l,p,x,c,w,r,env,s): time, lifecycle phase,
process/tree, executable, container, workload path/state, run, environment,
and operation.

Execution scope is S = (T,L,P,X,C,W,R). Membership requires every declared
dimension predicate to match. Environment identity is
Env = (arch,abi,kernel,runtime,libc,image,privilege,namespace,security).
Workload state is
State = (configuration,environmentVariables,featureFlags,externalInputs,
persistentState). Evidence MUST identify observer, mechanism, source,
version, run, scope, environment, and relevant state assumptions.

For Kubernetes container Seccomp, the enforcement scope is all processes and
descendants in the named container for its lifecycle. A bounded binary trace
is not automatically that scope.

## Observation and coverage

Observed(s,Q) means an observed event in Q has operation s. It proves
occurrence in Q, not universal necessity, allowlist sufficiency, or safe
denial. Absence is not negative knowledge.

Coverage is the following record, not a Boolean:

Coverage = (evidenceSet,evidenceScope,targetScope,dimensions,method,
assumptions,result)

result is MEASURED, ASSUMED, EXTERNALLY_CERTIFIED, or UNKNOWN.
dimensions MUST include temporal, lifecycle, process/tree, executable,
container, workload-path, run/history, and relevant environment/state
dimensions.

ScopeCovers(A,B) is evaluated dimension by dimension. For each mandatory
dimension it returns COVERS, DOES_NOT_COVER, or UNKNOWN. It returns COVERS
only if every mandatory dimension returns COVERS; any DOES_NOT_COVER returns
DOES_NOT_COVER; otherwise it returns UNKNOWN. Partial coverage MUST NOT be
promoted to COVERS.

Coverage composition is defined as follows:

* Same-run records MAY combine only when their target identity, environment,
  rule version, and scope predicates are compatible.
* Cross-run records MAY combine only after Compatible(source,target,rule)
  returns COMPATIBLE for every contributing run.
* Disjoint scopes produce their union only if all required dimensions remain
  covered; otherwise the result is UNKNOWN.
* Overlapping scopes are deduplicated by identity but retain provenance.
* Contradictory security evidence produces CONTRADICTORY and therefore
  INELIGIBLE unless an authority rule explicitly resolves the contradiction.
* Stale, revoked, or differently scoped records cannot widen authority.
* Merged SPO recordings retain each source scope; unioning syscall names
  never widens target scope by itself.

## Completeness authority

EMPIRICAL_COMPLETENESS proves observation over a finite declared scope only
and MUST NOT alone authorize container-lifetime deny-by-default enforcement.
STRUCTURAL_COMPLETENESS is derived from a closed validated transition model
and is limited to that model. DECLARED_COMPLETENESS is an owner's assertion
and contributes authority only when an AuthorityRule names that owner,
assumptions, scope, validity, and review requirements. EXTERNALLY_CERTIFIED_
COMPLETENESS requires issuer, certification scope, version, validity interval,
and revocation status. Every class is bounded by its declared domain.

## Adequacy

AdequacyResult is ADEQUATE, INADEQUATE, or UNKNOWN.
EvaluateAdequacy(C,Evidence,Baseline,Context) MUST use policy semantics,
target scope, independent evidence, baseline composition, environment/state,
and the applicable authority rule. It MUST NOT consult eligibility,
authorization, or enforcement status. If the selected authority model cannot
positively establish adequacy, it returns UNKNOWN or INADEQUATE.

## Compatibility

Compatibility is directional:

Compatible(SourceContext,TargetContext,RuleVersion)

returns COMPATIBLE, INCOMPATIBLE, or UNKNOWN. It is not assumed symmetric or
transitive. The comparison includes architecture, ABI, kernel, runtime,
libc, image, privilege/capabilities, namespace/security state,
configuration, environment variables, feature flags, persistent state, and
rule version. A context fingerprint and validity interval are required.
Security-relevant drift invalidates the result and requires re-evaluation.

## Authority rule

An AuthorityRule MUST contain rule ID/version, backend, target-scope
requirements, accepted evidence/completeness classes, mandatory coverage
dimensions, adequacy requirements, baseline requirements, compatibility
predicate, provenance requirements, validity interval, and invalidation
conditions. It MUST NOT declare arbitrary partial observation sufficient for
container-wide deny-by-default enforcement unless an independent trusted
source covers the missing dimensions.

## Eligibility function

EvaluateEligibility(Candidate C, EnforcementContext X, EvaluationTime T,
AuthorityRule R) returns exactly ELIGIBLE, INELIGIBLE, or UNKNOWN.
Inputs MUST include C's canonical policy and AuthorityMetadata, digest and
bindings, enforcement scope, evidence/observation scope, composed coverage,
completeness class, adequacy result, environment/state fingerprints,
baseline and compatibility, provenance, rule version, validity/revocation
status, and contradiction status.

Decision precedence is normative:

1. Digest/binding failure, invalid or revoked rule, contradiction,
   DOES_NOT_COVER, INADEQUATE, INCOMPATIBLE, revoked provenance, or any
   explicit mandatory failure => INELIGIBLE.
2. If no mandatory failure exists but any mandatory input/result is UNKNOWN,
   including scope, coverage, completeness assumptions, adequacy,
   compatibility, provenance, context, baseline applicability, or rule
   lookup => UNKNOWN.
3. Only if every mandatory predicate is positive, current, bound, and
   authority-compatible => ELIGIBLE.

Missing rule is UNKNOWN; a revoked/invalid rule is INELIGIBLE.
No default may promote UNKNOWN.

Normative pseudocode:

    verify canonical digest and AuthorityMetadata binding
    if failure: return INELIGIBLE
    if R missing: return UNKNOWN
    if R invalid or revoked: return INELIGIBLE
    if any contradiction or mandatory explicit failure: return INELIGIBLE
    evaluate ScopeCovers for every required dimension
    evaluate completeness and authority-class requirements
    evaluate adequacy independently
    evaluate Compatible for every source/baseline/context
    evaluate provenance, validity interval, and revocation
    if any result is UNKNOWN: return UNKNOWN
    if any required predicate is negative: return INELIGIBLE
    return ELIGIBLE

Implementations MUST NOT add unstated promotion rules.

## Authority states

The states are OBSERVED_EVIDENCE, DERIVED_ADVISORY, REVIEWABLE_CANDIDATE,
APPROVED_BYTES, SEMANTICALLY_ELIGIBLE, AUTHORIZED_FOR_ENFORCEMENT,
MATERIALIZED, ACTIVE, and BEHAVIORALLY_VERIFIED.

Each transition requires the evaluator above and current context. In
particular APPROVED_BYTES -> AUTHORIZED_FOR_ENFORCEMENT is prohibited without
a current ELIGIBLE result. Materialized does not imply active; active does
not imply behavioral verification.

Mutation, context drift, baseline/rule/provenance revocation, or validity
expiry transitions SEMANTICALLY_ELIGIBLE, AUTHORIZED, MATERIALIZED, and ACTIVE
to INVALIDATED_PENDING_REEVALUATION. Active policy authority MUST NOT silently
survive invalidation. Before authorization, materialization, and activation,
eligibility MUST be re-evaluated when context may have changed.

## Semantic binding and CandidateDigest

AuthorityMetadata MUST canonically serialize: interpretation; observation
and enforcement scopes; coverage; completeness; adequacy assumptions;
baseline identity; compatibility rule/result and context fingerprints;
authority-rule ID/version; environment/state assumptions; provenance; and
validity/revocation references. This canonical representation MUST be inside
CandidateDigest, or its independent immutable binding digest MUST itself be
bound to CandidateDigest. One digest MUST NOT admit two authority
interpretations.

## Baselines and Seccomp composition

P = B union O is conceptual only. For Seccomp, composition MUST account for
default action, architectures, syscall names, argument filters, actions,
errno values, and provenance. Conflicting defaults, architectures, rules, or
provenance produce CONTRADICTORY and therefore INELIGIBLE unless R defines a
deterministic safe resolution. A baseline MUST carry owner, version, target
environment, compatibility rule, and provenance. Compatibility UNKNOWN is
never eligible.

## SPO boundary and advisory mode

SPO generation, Ready status, recording provenance, coverage annotations,
or merged syscall sets are evidence inputs, not authority alone. Recording
identity, scope, merge semantics, target identity, partial/unknown status,
environment assumptions, and provenance schema/version are mandatory.
Internal Seccomp without a positive eligibility result remains
ADVISORY_ONLY or INELIGIBLE; it may be reviewed, approved as bytes, stored,
compared, or exported, but not enforced.

## Failure taxonomy

ADVISORY_ONLY, INELIGIBLE, UNKNOWN, REJECTED, UNAVAILABLE,
MATERIALIZATION_FAILED, and ACTIVE_UNVERIFIED are distinct. Fail-closed means
insufficient authority cannot result in enforcement; it does not collapse
semantic uncertainty, integrity rejection, backend unavailability, and runtime
failure into one result.

## Countermodels

Startup outside the trace, rare paths, descendants, different executables,
later lifecycle, configuration branches, image drift, runtime drift,
authority-rule drift, contradictory evidence, SPO scope inflation, and stale
eligibility all fail through missing ScopeCovers, incompatible/unknown
Compatible, contradiction precedence, invalid rule/validity, or
invalidation-before-activation. None may return ELIGIBLE without an
independent authority satisfying the missing predicate.

## Migration and testable properties

Proposals without this metadata are LEGACY / AUTHORITY_UNKNOWN /
ADVISORY_ONLY. Regeneration and reapproval are required for stronger claims.
Tests MUST verify digest binding, scope containment, composition,
contradiction precedence, UNKNOWN propagation, compatibility drift,
revocation, state-transition barriers, and the ten countermodels.

## Claim boundary and open questions

This RFC defines semantics and evaluator obligations, not an observer,
baseline, SPO completeness, Seccomp algorithm, or runtime proof. Remaining
implementation decisions concern baseline ownership, lifecycle collectors,
SPO coverage verification, rule publication, and risk-acceptance UX.

## Normative closure of remaining degrees of freedom

This section supersedes any earlier generality and is the sole authority for
the following security decisions.

### Closed BackendSecurityEnvelope

The envelope schema is versioned and contains this closed mandatory
enumeration for container-level deny-by-default Seccomp:

`STARTUP_BOOTSTRAP`, `CONTAINER_LIFETIME`, `PROCESS_TREE`, `EXECUTABLE_SET`,
`WORKLOAD_STATE`, `ARCHITECTURE_ABI`, `IMAGE_IDENTITY`, and
`KERNEL_RUNTIME_COMPATIBILITY`.

Each dimension is exactly `SATISFIED`, `UNSATISFIED`, or `UNKNOWN`. Missing is
`UNKNOWN`; explicit non-coverage is `UNSATISFIED`; positive accepted evidence
is `SATISFIED`. Envelope evaluation is `ENVELOPE_UNSATISFIED` if any dimension
is unsatisfied, `ENVELOPE_UNKNOWN` if none is unsatisfied and any is unknown,
otherwise `ENVELOPE_SATISFIED`. An AuthorityRule MUST contain every
dimension and MUST NOT weaken, remove, or reinterpret one. A weakening rule is
`INVALID`.

### Closed adequacy evidence

The only AdequacyEvidence classes are `STRUCTURAL_BASELINE`,
`EXTERNAL_CERTIFICATION`, `BACKEND_FORMAL_INVARIANT`, `BOUNDED_BEHAVIORAL`,
and `TRUSTED_BASELINE_OBSERVED_DELTA`. Each record MUST bind issuer/source,
scope, context identity, provenance, validity, and revocation. Only the
class-specific predicates below may establish `ADEQUATE`:

* `STRUCTURAL_BASELINE`: a closed transition model and its validation record;
* `EXTERNAL_CERTIFICATION`: issuer, certification scope, version, validity,
  and non-revocation;
* `BACKEND_FORMAL_INVARIANT`: the exact versioned invariant and proof record;
* `BOUNDED_BEHAVIORAL`: every required bounded behavior in the declared
  scope, with no failed property;
* `TRUSTED_BASELINE_OBSERVED_DELTA`: compatible trusted baseline plus an
  accepted observed delta and envelope coverage.

An AuthorityRule may select these classes but cannot create another class or
assert adequacy. Known failed evidence is `INADEQUATE`; missing or unknown
mandatory evidence is `UNKNOWN`; only all class predicates positive is
`ADEQUATE`.

### Context equivalence

For the envelope fields, `image`, `architecture`, `ABI`, `privilege`,
`namespace/security`, `workload identity`, and applicable `executable` are
`EXACT`. `kernel`, `runtime`, `libc`, `environment`, `configuration`,
`feature flags`, and `persistent state` are `RULED_EQUIVALENCE` and MUST name
one exact versioned, digest-bound CompatibilityRule. Pod UID,
resourceVersion, container ID, timestamps, node IP, and diagnostic metadata
are `IRRELEVANT`. Any exact mismatch is `NOT_EQUIVALENT`; any ruled mismatch
is `NOT_EQUIVALENT`; otherwise an unknown required comparison is `UNKNOWN`;
only all positive comparisons yield `EQUIVALENT`.

### Trust, revocation, and exact rule selection

IssuerTrust is `TRUSTED`, `UNTRUSTED`, or `UNKNOWN`; RevocationStatus is
`NOT_REVOKED`, `REVOKED`, or `UNKNOWN`. Trust MUST come from the explicitly
bound issuer source. Untrusted or revoked is negative; unknown is unknown;
known negative dominates unknown. Malformed bindings are negative.

The candidate binds exactly RuleID, RuleVersion, and RuleDigest. Duplicate
identity records with differing digests are `AMBIGUOUS_IDENTITY` and
`INELIGIBLE`; identical canonical duplicates are deduplicated. No latest,
first, storage-order, or fallback selection is permitted. The same uniqueness
rule applies to baselines, evidence, provenance, operators, and compatibility
rules.

### Composition and evidence relations

Composition operators bind OperatorID, version, and digest. Relation results
are `COMPOSABLE`, `CONTRADICTORY`, `NON_COMPARABLE`, or `UNKNOWN`. Exact
compatible Seccomp values are composable; differing default actions,
architectures, syscall actions, argument filters, errno semantics, baseline
versions, or security provenance are contradictory unless the bound operator
defines their deterministic resolution. Undefined resolution is unknown;
envelope-violating resolution invalidates the operator. Non-comparable or
unknown evidence cannot widen authority. SPO merge is subject to the same
ContextEquivalent and EvidenceRelation rules; syscall-name union never erases
source scope.

### Canonical authority schema

Authority metadata has fixed schemaVersion and fixed field names/types. Every
security field is required or optional by this RFC: omitted differs from
empty, UNKNOWN is an explicit tagged value, and unsupported security fields
are rejected. Collections are classified as `ORDERED_LIST`, `SET`, or `MAP`.
Ordered lists preserve order; sets are sorted by canonical scalar encoding
and duplicate elements are rejected; maps sort keys and reject duplicate
keys. Scalars use one canonical type encoding; aliases such as omitted/zero,
false/omitted, or string/number are forbidden. Canonicalization failure
produces no digest and is `INELIGIBLE`. Schema version, rule, scope, coverage,
completeness, adequacy, baseline, compatibility, provenance, validity,
revocation, environment, and workload-state fields are all bound.

### Authoritative precedence

For every evaluator and sub-evaluator, the following precedence is fixed:

1. known malformed binding, contradiction, revocation, incompatibility,
   explicit non-coverage, unsupported class/operator, duplicate ambiguity,
   or other known invalidity => negative (`INELIGIBLE` at top level);
2. otherwise any absent, unavailable, or unknown mandatory fact =>
   `UNKNOWN`;
3. otherwise all mandatory predicates positive => positive.

Absent, unknown, invalid, unsupported, revoked, expired, and contradictory
are not interchangeable. Missing rule is unknown; malformed rule is
negative; expired eligibility without explicit revocation is unknown; known
revocation dominates expiry. This table is exhaustive.

### Temporal and active authority

An eligibility record is valid only for its bound digest, rule, context,
baseline, provenance, evaluation time, deadline, and revocation version.
Before authorization, materialization, and activation, current values are
compared. Known changes invalidate; unavailable current facts are unknown.
For an active policy, known revocation/incompatibility becomes
`ACTIVE_UNAUTHORIZED`; unavailable mandatory facts become
`SUSPENDED_UNKNOWN`; only all current positive predicates remain `ACTIVE`.
These are semantic authority states, independent of physical detachment.

### Total evaluator

```text
EvaluateEligibility(C,X,T,Rules):
  canonicalize and bind C metadata; failure => INELIGIBLE
  resolve exactly C's rule ID/version/digest; absent => UNKNOWN
  validate issuer, revocation, backend, envelope, and mandatory fields
  invalid/revoked/weak/ambiguous => INELIGIBLE; unknown => UNKNOWN
  evaluate all envelope dimensions and ScopeCovers
  unsatisfied => INELIGIBLE; unknown => UNKNOWN
  classify evidence relations and composition
  contradiction/non-comparable widening/unsafe operator => INELIGIBLE
  unknown relation or unresolved operator => UNKNOWN
  evaluate completeness class and closed AdequacyEvidence class
  negative => INELIGIBLE; unknown => UNKNOWN
  evaluate baseline composition and ContextEquivalent
  mismatch => INELIGIBLE; unknown => UNKNOWN
  evaluate provenance, validity, current context, and revocation
  invalid/revoked/expired-with-negative => INELIGIBLE
  unknown/unavailable/expired-without-negative => UNKNOWN
  return ELIGIBLE
```

No implementation may add a promotion, fallback, heuristic, or permissive
default branch.

## Terminal schema closure

This section closes the terminal evaluator schemas for Registry Version 1.

### SecurityFieldRegistry v1

The registry is the closed set of fields below; no other security field is
recognized under version 1:

`schemaVersion`, `candidateDigest`, `backend`, `enforcementScope`,
`observationScope`, `evidenceReferences`, `authorityRuleID`,
`authorityRuleVersion`, `authorityRuleDigest`, `envelopeID`,
`envelopeVersion`, `adequacyEvidenceReferences`, `completenessClaims`,
`baselineID`, `baselineVersion`, `baselineDigest`, `compositionOperatorID`,
`compositionOperatorVersion`, `compositionOperatorDigest`,
`compatibilityRuleID`, `compatibilityRuleVersion`, `compatibilityRuleDigest`,
`trustPolicyID`, `trustPolicyVersion`, `trustPolicyDigest`,
`certificationReferences`, `provenanceReferences`, `validity`,
`revocationReferences`, `registryVersion`, `imageDigest`, `architecture`,
`abi`, `kernelClass`, `runtimeClass`, `libcIdentity`, `privilegeState`,
`namespaceSecurityState`, `configurationIdentity`, `environmentIdentity`,
`featureFlags`, `persistentStateIdentity`, `workloadIdentity`,
`containerIdentity`, `executableIdentity`, `evidenceID`, `evidenceType`,
`evidenceVersion`, `sourceIdentity`, `observationMechanism`, `runIdentity`,
`verificationRecord`, `issuerIdentity`, and `issuerTrustBinding`.

Each field is a required or optional scalar/struct/set/list/map according to
its name's schema category: identity/version/digest fields are required
scalars; references are sets; scope and context are structs; ordered test
steps are ordered lists; keyed properties are maps. Omitted required fields
are invalid; omitted optional fields equal an explicit `ABSENT` tag; empty is
distinct from absent; UNKNOWN is an explicit tagged value. Sets sort by
canonical scalar encoding and reject duplicates; maps sort keys and reject
duplicates; ordered lists preserve order. Identity fields are `EXACT`;
kernel/runtime/configuration/environment/persistent-state classes are
`RULED_EQUIVALENCE` through the exact bound CompatibilityRule; diagnostic
metadata is not in this registry and is `IRRELEVANT`. An unregistered
security field is canonicalization failure and `INELIGIBLE`.

### Closed adequacy schemas

Every adequacy record contains `evidenceID`, `classID`, `classVersion`,
`issuerIdentity`, `backend`, `scope`, `context`, `propertyID`, `verifierID`,
`verifierVersion`, `verifierDigest`, `verificationResult`, `validity`,
`revocation`, and `provenance`. `verificationResult` is exactly
`VERIFIED`, `FAILED`, or `UNKNOWN`.

For `STRUCTURAL_BASELINE`, the verifier must be a recognized
`StructuralBaselineVerifier` record binding baseline digest, envelope,
model/version, assumptions, scope, context, procedure digest, and result.
For `BACKEND_FORMAL_INVARIANT`, the verifier must reference a recognized
`BackendInvariantRegistry` entry with the same bindings. For
`BOUNDED_BEHAVIORAL`, the test-set digest, expected-result specification,
observed outcomes, scope, context, and verifier are mandatory. For
`TRUSTED_BASELINE_OBSERVED_DELTA`, independently verified baseline,
compatible context, valid delta provenance, and valid composition are all
mandatory. `EXTERNAL_CERTIFICATION` uses the certification registry below.
No free-form document is a verifier record.

### CertificationPropertyRegistry v1

The only recognized certification properties are
`SCOPE_COVERAGE`, `BASELINE_COMPATIBILITY`, `POLICY_ADEQUACY_BOUNDED`, and
`PROVENANCE_VALIDITY`. Each entry fixes backend, required scope dimensions,
required context fields, verifier ID/version/digest, validity, revocation,
and the single property it may establish. An unregistered property is
`CERTIFICATION_INVALID`; a missing verifier fact is
`CERTIFICATION_UNKNOWN`. A property cannot establish any other property.

### Trust authorization

TrustPolicy v1 contains policy ID/version/digest, TrustAuthorizationEntry
sets, revocation source, validity, and trust-source identity. Each entry
contains issuer, artifact class, backend, allowed property/class IDs, scope
constraint, context constraint, validity, and revocation reference. A loaded
policy is closed-world: absent matching entry is `UNTRUSTED`; unavailable
policy is `UNKNOWN`; known invalid, revoked, wrong-class, wrong-backend, or
wrong-scope is `UNTRUSTED`. Delegation is not supported.

### Compatibility predicate grammar

CompatibilityRule v1 permits only:

`EXACT_EQUAL(field)`, `VALUE_IN_SET(field, canonicalSet)`,
`VERSION_EQUAL(field, canonicalVersion)`,
`VERSION_IN_CLOSED_RANGE(field, min, max, orderingID)`,
`ARCH_ABI_MATCH(sourceField, targetField, relationID)`, and
`DIGEST_EQUAL(field)`.

CompatibilityRule schema `compatibility-rule.v2` additionally permits the distinct relational predicates
`SET_CONTAINS(field)` and `MAP_CONTAINS(field)`. `VALUE_IN_SET(field,
canonicalSet)` and its scalar membership semantics are unchanged. For both
new predicates, `candidate` is the `SourceContext` and `baseline` is the
`TargetContext`; no rule-owned expected operand is permitted. `SET_CONTAINS`
requires typed sets and is true exactly when
`candidate[field] ⊆ baseline[field]`. `MAP_CONTAINS` requires typed maps and
is true exactly when every `(key,value)` entry in `candidate[field]` exists
identically in `baseline[field]`. Operand reversal is a distinct relation.
Missing or authoritative-unknown operands are `UNKNOWN`; malformed or
wrong-domain operands are `INVALID`; a valid false relation is
`INCOMPATIBLE`; and a valid true relation is `COMPATIBLE`.

The v2 predicates are relational, whereas `EXACT_EQUAL`, `VALUE_IN_SET`,
`VERSION_EQUAL`, `VERSION_IN_CLOSED_RANGE`, and `DIGEST_EQUAL` consume
candidate material and rule-owned constraints. Unsupported v2 operators are
rejected by readers that support only v1; they MUST NOT be reinterpreted as
an existing predicate or silently skipped.

Version ordering uses the canonical numeric tuple specified by the rule;
malformed versions are incompatible. Architecture/ABI relations are looked
up only in ArchitectureABIRelationRegistry v1, whose recognized relation is
`LINUX_X86_64_ABI` with exact values `x86_64` and `amd64`; all other pairs
are unknown. Unsupported or malformed predicates are incompatible; missing
fields are unknown; false predicates are incompatible; all true predicates
are compatible.

### Composition language and conflict table

CompositionOperator v1 is a closed expression over `REQUIRE_EQUAL`,
`SET_UNION_IF_IDENTICAL_ACTION`, `SET_INTERSECTION`,
`REJECT_ON_CONFLICT`, and `PRESERVE_BASELINE`. DefaultAction, architecture,
syscall action, argument filters, errno, and provenance use
`REQUIRE_EQUAL` unless the operator explicitly selects another listed
operation. A known unresolved conflict is `COMPOSITION_INVALID`; missing
input is `COMPOSITION_UNKNOWN`; envelope weakening or unsupported expression
is invalid. Operator ID/version/digest and the complete expression are bound.

### Terminal graph

Every leaf used by eligibility terminates in a closed enum: envelope states,
scope states, evidence relations, verification results, certification results,
trust/revocation results, composition results, compatibility results,
provenance results, or the global eligibility result. No free-form assertion
can produce a positive leaf.

## Terminal predicate closure

This section is authoritative for terminal predicate evaluation.

### AdequacyEvidenceClass

The class enumeration is closed: `STRUCTURAL_BASELINE`,
`EXTERNAL_CERTIFICATION`, `BACKEND_FORMAL_INVARIANT`, `BOUNDED_BEHAVIORAL`,
and `TRUSTED_BASELINE_OBSERVED_DELTA`. Every record MUST contain identity,
class/version, issuer/source, backend, claimed scope, SecurityContextIdentity,
validity interval, revocation state, provenance, and verification record.

`STRUCTURAL_BASELINE` requires a closed transition model plus a reproducible
validation record. `EXTERNAL_CERTIFICATION` requires the certification
predicate below. `BACKEND_FORMAL_INVARIANT` requires the exact versioned
invariant and proof record. `BOUNDED_BEHAVIORAL` requires every required
behavioral property in its declared bounded scope and no failed property.
`TRUSTED_BASELINE_OBSERVED_DELTA` requires compatible baseline, envelope
coverage, and accepted observed delta. Mere possession, assertion, approval,
signature, readiness, materialization, attachment, or one successful action
is insufficient.

### CompatibilityRule

A CompatibilityRule is a canonical object containing ID, version, digest,
issuer/trust binding, source/target schemas, field predicates, validity, and
revocation. Predicates are closed to equality, enumerated membership, exact
version, bounded version range, architecture/ABI relation, and exact digest.
Unsupported predicates are `INCOMPATIBLE`; missing facts are `UNKNOWN`; a
false predicate is `INCOMPATIBLE`; all predicates true are `COMPATIBLE`.
Rules are directional, versioned, and digest-bound; no local callback or
heuristic may participate.

### Issuer trust and certification

TrustPolicy is canonical and contains policy ID/version/digest, authorized
issuers, artifact classes, scope/backend restrictions, validity, revocation
source, and trust-source identity. `EvaluateIssuerTrust` returns
`TRUSTED`, `UNTRUSTED`, or `UNKNOWN`: explicit authorization plus valid
identity, scope, policy, and non-revocation is trusted; known unauthorized,
invalid, revoked, wrong-class, or wrong-scope is untrusted; unavailable facts
are unknown.

An external certification is canonical over identity/version/digest, issuer,
class, backend, property, scope, context, assumptions, validity, revocation,
provenance, and verification mechanism. `EvaluateCertification` returns
`CERTIFICATION_VALID`, `CERTIFICATION_INVALID`, or `CERTIFICATION_UNKNOWN`.
Valid requires trusted issuer, intact identity, recognized property, scope
coverage, equivalent context, satisfied assumptions, current validity,
non-revocation, and valid provenance. A mismatch is invalid; a missing fact is
unknown. Certification cannot widen its certified scope.

### CompositionOperator

A composition operator is canonical over ID/version/digest, backend, input
schemas, supported conflict classes, deterministic transformation semantics,
envelope compatibility, provenance preservation, validity, and revocation.
For Seccomp the closed conflict domains are default action, architectures,
syscall identity/action, argument filters, errno, and provenance.
`EvaluateComposition` returns `COMPOSITION_VALID`, `COMPOSITION_INVALID`, or
`COMPOSITION_UNKNOWN`. Unsupported semantics, unresolved known conflicts,
envelope weakening, malformed/invalid/revoked operator, or unauthorized
resolution is invalid; unavailable mandatory input is unknown. Only valid
composition contributes to adequacy or eligibility.

### SecurityFieldRegistry

The registry is closed and versioned. It enumerates all fields influencing
candidate interpretation, observation/enforcement scope, coverage,
completeness, adequacy, baseline, context, provenance, trust, rule selection,
composition, validity, and revocation. Each field has canonical name, type,
cardinality, omitted/empty/UNKNOWN semantics, ordering, identity significance,
and comparison mode (`EXACT`, `RULED_EQUIVALENCE`, or `IRRELEVANT`). An
unregistered security-significant field is canonicalization failure and
`INELIGIBLE`. Registry evolution requires a new schema version.

### Integrated evaluator

The complete evaluation path is:

```text
canonicalize registry fields and bind AuthorityMetadata
resolve the exact bound AuthorityRule
evaluate issuer trust and rule validity
evaluate the non-waivable BackendSecurityEnvelope
evaluate ScopeCovers and EvidenceRelation/composition
evaluate completeness and recognized adequacy evidence
evaluate certification and baseline composition when present
evaluate CompatibilityRule and current context
evaluate provenance, validity, and revocation
apply precedence: known negative -> INELIGIBLE;
  otherwise mandatory unknown/unavailable -> UNKNOWN;
  otherwise all positive -> ELIGIBLE
```

No terminal predicate is prose-only, and no AuthorityRule may introduce a
new adequacy class, compatibility primitive, trust interpretation, or
composition semantics.

## Final normative repair: deterministic security envelope and evaluator

This section is normative and supersedes any less-specific wording above.

### Backend security envelopes

Each backend has a non-waivable `BackendSecurityEnvelope`. For Kubernetes
container-level deny-by-default Seccomp, the envelope requires authority over:

* container startup and bootstrap;
* the declared container lifetime;
* the relevant process tree;
* all relevant executables;
* declared workload paths and states;
* architecture and ABI;
* image identity; and
* compatible kernel and container runtime.

An `AuthorityRule` MAY strengthen these requirements but MUST NOT weaken
them. A rule weaker than its envelope is `INVALID`.

### Authority-rule trust and selection

An AuthorityRule MUST contain rule ID, version, issuer identity and trust
binding, backend and envelope version, validity interval, revocation state,
and canonical rule digest. `ValidateAuthorityRule(rule, envelope, time)`
returns `VALID`, `INVALID`, or `UNKNOWN`.

`INVALID` includes bad binding, unauthorized issuer, revocation, expiry,
backend/envelope mismatch, envelope weakening, and malformed mandatory
fields. `UNKNOWN` means a mandatory external trust fact cannot be established
and explicit invalidity is absent.

Candidate authority metadata MUST bind exactly one rule ID, version, and
digest. Missing exact rule returns `UNKNOWN`; digest mismatch or invalid rule
returns `INELIGIBLE`. No latest-version lookup or fallback is permitted.

### Context identity and equivalence

`SecurityContextIdentity` canonically includes image digest, architecture, ABI,
kernel/runtime compatibility classes, libc where relevant,
privilege/capabilities, namespace/security mode, relevant configuration and
environment, feature flags, persistent-state class, workload identity, and
executable identity where applicable. Diagnostic-only fields are excluded.

`ContextEquivalent(source, target, rule)` returns `EQUIVALENT`,
`NOT_EQUIVALENT`, or `UNKNOWN`. Missing mandatory identity is `UNKNOWN`; a
known mandatory mismatch is `NOT_EQUIVALENT`. Rule-specific equivalence MAY
be stricter but MUST NOT ignore envelope-mandated fields.

### Adequacy evidence

`AdequacyEvidence` is independent evidence, not an AuthorityRule assertion.
`EvaluateAdequacy(candidate, scope, evidence, baseline, context, rule)`
returns `ADEQUATE`, `INADEQUATE`, or `UNKNOWN`.

Acceptable classes are a structurally justified baseline, externally
certified policy/baseline, backend-declared formal invariant, bounded
behavioral validation, or a trusted compatible baseline plus observed delta.
Approval, signatures, CandidateDigest, SPO Ready, and rule assertions alone
MUST NOT produce `ADEQUATE`.

An explicit failed adequacy property returns `INADEQUATE`; missing or unknown
mandatory adequacy evidence returns `UNKNOWN`; only all independent required
predicates return `ADEQUATE`. Adequacy MUST NOT consult eligibility or
authorization.

### Evidence relation and composition

`EvidenceRelation(E1,E2)` returns `COMPATIBLE`, `CONTRADICTORY`,
`NON_COMPARABLE`, or `UNKNOWN`. Compatibility requires equivalent target
semantics and compatible contexts under the bound rule. Mutually incompatible
security claims about the same target are contradictory. Different domains
that cannot be safely combined are non-comparable. Insufficient information
is unknown.

Compatible evidence MAY compose only under the bound composition rule.
Contradictory evidence is `INELIGIBLE` unless an explicitly bound,
deterministic safe resolver exists. Non-comparable evidence MUST NOT widen
authority. Unknown evidence MUST NOT widen authority. SPO merged recordings
retain every source scope and context identity.

### Provenance evaluation

`EvaluateProvenance(evidence, rule, envelope, time)` returns `VALID`,
`INVALID`, or `UNKNOWN`. Known forged, revoked, mismatched, or bad provenance
is invalid; unavailable mandatory provenance is unknown; all bound rule and
envelope provenance predicates established is valid. SPO Ready, approval, and
digest equality are not provenance validity.

### Canonical authority metadata

`CanonicalAuthorityBytes(metadata)` is versioned and deterministic. Field
names and types are fixed; map ordering is canonical; list ordering semantics
are declared per field; omitted and empty values differ unless the schema
explicitly equates them; UNKNOWN has an explicit value; duplicate semantic
fields and unsupported fields are rejected; numeric, string, and Boolean
representations cannot alias. Schema version participates in the bytes.

`AuthorityMetadataDigest = HASH(CanonicalAuthorityBytes(metadata))`.
This digest MUST be in CandidateDigest or be independently bound by a digest
that is itself included in CandidateDigest. Semantically distinct authority
metadata MUST NOT canonicalize to identical bytes.

### Eligibility result table

The following mapping is exhaustive and has precedence in order:

| Condition | Result |
|---|---|
| digest or semantic binding mismatch | INELIGIBLE |
| malformed rule or envelope violation | INELIGIBLE |
| revoked/expired/invalid rule or provenance | INELIGIBLE |
| mandatory scope does not cover target | INELIGIBLE |
| insufficient declared completeness | INELIGIBLE |
| adequacy inadequate | INELIGIBLE |
| compatibility mismatch | INELIGIBLE |
| contradiction or unsafe composition | INELIGIBLE |
| exact rule absent | UNKNOWN |
| mandatory scope/coverage unknown | UNKNOWN |
| completeness or adequacy unknown | UNKNOWN |
| compatibility/context/provenance unknown | UNKNOWN |
| expired evaluation without explicit invalidity | UNKNOWN |
| all mandatory predicates positive and current | ELIGIBLE |

No other result is permitted.

### Temporal validity and revocation

An `EligibilityRecord` binds CandidateDigest, AuthorityRuleDigest, context
fingerprint, baseline identity/digest, provenance identities, evaluation time,
validity deadline, and revocation state/version. Before authorization,
materialization, and activation, current values MUST be compared with the
record. Known security-relevant differences invalidate eligibility; unknown
current facts return `UNKNOWN`; expiry returns `UNKNOWN` unless explicit
revocation makes it `INELIGIBLE`.

Authority invalidation transitions authorized/materialized/active authority
to `ACTIVE_UNAUTHORIZED` with reason `REVOKED`, `INCOMPATIBLE`, or
`SUSPENDED_UNKNOWN`. An invalidated active policy MUST NOT remain represented
as authorized. Physical detachment MAY be deferred for fail-safe reasons and
is an implementation decision; semantic authority changes immediately.

### Seccomp composition

`B union O` is not executable Seccomp semantics. Composition MUST compare
default actions, architectures, syscall actions, argument filters, errno
values, and provenance. Conflicts are `CONTRADICTORY`; baseline identity or
version mismatch is `INCOMPATIBLE`. Unknown conflict resolution returns
`UNKNOWN`. A custom composition operator is allowed only when identified,
versioned, digest-bound, deterministic, envelope-compliant, and provenance-
preserving.

### Final evaluator

```text
EvaluateEligibility(C, X, T, Rules):
  verify canonical candidate and authority binding
  if failure: return INELIGIBLE
  resolve exactly C.authorityRuleID/version/digest
  if absent: return UNKNOWN
  validate rule against its BackendSecurityEnvelope
  if invalid: return INELIGIBLE
  evaluate scope, evidence relations, and composition
  if contradiction, non-comparable widening, or DOES_NOT_COVER: return INELIGIBLE
  if any mandatory scope or coverage result is UNKNOWN: return UNKNOWN
  evaluate completeness against envelope and rule
  if unsupported/insufficient: return INELIGIBLE
  if unknown: return UNKNOWN
  evaluate independent adequacy
  if INADEQUATE: return INELIGIBLE
  if UNKNOWN: return UNKNOWN
  evaluate baseline composition and directional compatibility
  if mismatch: return INELIGIBLE
  if unknown: return UNKNOWN
  evaluate provenance, validity, revocation, and current context
  if invalid/revoked: return INELIGIBLE
  if unknown or expired: return UNKNOWN
  return ELIGIBLE
```

The certified experiment fails the Seccomp envelope's startup/lifecycle
coverage predicate and therefore cannot be `ELIGIBLE` solely from its bounded
binary observation.
