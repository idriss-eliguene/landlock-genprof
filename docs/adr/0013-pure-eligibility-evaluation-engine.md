# ADR-0013: Pure Eligibility and Evidence Evaluation Engine

Status: Proposed

Date: 2026-08-23

## Decision question

How should landlock-genprof implement RFC-0003 `EvaluateEligibility` as a
deterministic, side-effect-free domain service over explicit snapshots and
current facts?

RFC-0003 owns every security predicate and result rule. ADR-0010 owns domain
objects, ADR-0011 owns canonical identity, and ADR-0012 owns exact resolution
and snapshot construction. This ADR defines the evaluator boundary and
composition architecture; it does not change those semantics.

## Baseline

Today validation and approval operate on the legacy artifact-only
`CandidateDigest`; `apply-proposal` performs Kubernetes reads and backend
readiness checks; internal Seccomp synthesis derives bounded observations; and
SPO import snapshots live resources through a Kubernetes dynamic client. None
of those paths is the RFC-0003 evaluator. They remain adapters or lifecycle
concerns around the future pure service.

## Decision: pure domain evaluator

Implement one backend-neutral domain service:

`EvaluateEligibility(AuthorityMetadata, ResolvedAuthorityBundle, CurrentAuthorityFacts, CurrentSecurityContext, EvaluationTime) -> EvaluationResult`.

The service performs no I/O and has no resolver, Kubernetes, SPO, PodLock,
filesystem, network, global configuration, ambient clock, cache, proposal
mutation, artifact application, or verifier/plugin execution dependency. All
authority objects are already resolved by ADR-0012. Verification adapters
produce immutable verification facts before evaluation.

The evaluator may be parallelized internally, but its semantic output is
identical to sequential evaluation.

## Input contract

* `AuthorityMetadata`: the candidate's immutable RFC-0003 interpretation and
  bound identity.
* `ResolvedAuthorityBundle`: exact immutable authority objects, registries,
  source provenance, and root-trust configuration identity from ADR-0012.
* `CurrentAuthorityFacts`: explicit current revocation, trust, verifier
  binding, freshness, source-epoch, and related mutable observations.
* `CurrentSecurityContext`: the current canonical security context to compare
  with the bound context.
* `EvaluationTime`: explicit evaluation instant; no `time.Now()` is allowed.

No additional input may be obtained by lookup. A missing fact is evaluated
according to RFC-0003 as negative or `UNKNOWN`, never completed implicitly.

## Output contract

`EvaluationResult` is an immutable semantic record containing:

* `EligibilityResult`: exactly `ELIGIBLE`, `INELIGIBLE`, or `UNKNOWN`;
* BoundCandidate, AuthorityMetadata, AuthorityRule, registry, and root-trust
  identities;
* evaluation context identity and `EvaluationTime`;
* normalized predicate results and stable reason codes;
* evidence, certification, provenance, baseline, composition, compatibility,
  and current-fact references used;
* derived validity deadline, when RFC-0003 permits one; and
* the identity material required to construct ADR-0010's immutable
  `EligibilityRecord`.

This result is not approval, authorization, materialization, or active
enforcement. It cannot by itself authorize an apply operation.

## Predicate and trace model

Each RFC sub-evaluator returns its own closed domain (for example,
`COMPATIBLE | INCOMPATIBLE | UNKNOWN` or `ADEQUATE | INADEQUATE | UNKNOWN`).
The aggregation layer maps each result to `POSITIVE`, `NEGATIVE`, or
`UNKNOWN` without erasing the original domain. An immutable `EvaluationTrace`
records `PredicateID`, subject references, sub-result, normalized reason code,
and optional diagnostic data. Reason codes, not error strings, carry meaning.

The normative trace reports every applicable mandatory predicate in one
canonical `PredicateID` order defined by this ADR: binding, bundle, envelope,
scope, evidence-relation, coverage, completeness, certification,
verification, adequacy, baseline, composition, provenance, trust,
compatibility, context, freshness, temporal-validity, and revocation. Within
one predicate, subject references and reason codes use ADR-0011 canonical
ordering. A decisive negative MUST NOT omit later applicable predicates;
`UNKNOWN` MUST NOT short-circuit. Internal optimization is permitted only if
it produces exactly the same complete predicate-result set and trace as full
evaluation. Structurally non-applicable predicates are represented by the
trace-only `NOT_APPLICABLE` state, which contributes neither positive,
negative, nor unknown aggregation, rather than silently omitted. Parallel
execution and map iteration cannot alter the trace or result.

The ordered reason-code set is authoritative; ADR-D defines no separate
primary-reason ranking. A programming invariant violation may be surfaced as
an implementation error only when the supplied domain snapshot could not have
been constructed under ADR-0010/0011. A known malformed security snapshot or
fact remains a semantic negative or `UNKNOWN` result according to RFC-0003,
not a generic runtime error.

## Closed reason-code registry

ADR-0013 ReasonCodeRegistry version 1 is closed. The following identifiers
are the only normative reason codes for this ADR. Each code is emitted at
most once per applicable predicate/subject occurrence, except where the
listed condition applies to a set; non-matching facts explicitly marked
irrelevant emit no code. Diagnostic prose is non-normative.

| Code | Predicate | Condition | Class |
|---|---|---|---|
| `BUNDLE_CONSISTENT` | `bundle` | All referenced objects and bindings consistent | POSITIVE |
| `BUNDLE_IDENTITY_MISMATCH` | `bundle` | Referenced object identity differs | NEGATIVE |
| `BUNDLE_KIND_MISMATCH` | `bundle` | Kind differs | NEGATIVE |
| `BUNDLE_ID_MISMATCH` | `bundle` | ID differs | NEGATIVE |
| `BUNDLE_VERSION_MISMATCH` | `bundle` | Version differs | NEGATIVE |
| `BUNDLE_DIGEST_MISMATCH` | `bundle` | Digest differs | NEGATIVE |
| `BUNDLE_REGISTRY_MISMATCH` | `bundle` | Registry binding differs | NEGATIVE |
| `BUNDLE_ROOT_TRUST_MISMATCH` | `bundle` | Root-trust identity differs | NEGATIVE |
| `BUNDLE_DUPLICATE_CONFLICT` | `bundle` | Same identity has conflicting content | NEGATIVE |
| `BUNDLE_NOT_FOUND` | `bundle` | Mandatory object proven absent | NEGATIVE |
| `BUNDLE_UNAVAILABLE` | `bundle` | Mandatory resolution unavailable | UNKNOWN |
| `BUNDLE_EXTRA_IGNORED` | `bundle` | Unreferenced object ignored | POSITIVE |
| `FACT_ACCEPTED_FRESH` | `freshness` | Matching fresh fact accepted | POSITIVE |
| `FACT_DUPLICATE_DEDUPLICATED` | `freshness` | Identical canonical duplicate removed | POSITIVE |
| `FACT_CONFLICT_FRESH` | `freshness` | Conflicting fresh authoritative facts | NEGATIVE |
| `FACT_WRONG_SUBJECT` | `freshness` | Subject does not match requirement | NEGATIVE |
| `FACT_WRONG_KIND` | `freshness` | Fact kind does not match | NEGATIVE |
| `FACT_WRONG_SOURCE` | `freshness` | Required source does not match | NEGATIVE |
| `FACT_STALE` | `freshness` | Mandatory fact is stale | UNKNOWN |
| `FACT_STALE_POSITIVE_FRESH_NEGATIVE` | `freshness` | Stale positive plus fresh negative | NEGATIVE |
| `FACT_FRESH_POSITIVE_FRESH_NEGATIVE` | `freshness` | Fresh positive plus fresh negative | NEGATIVE |
| `FACT_ALL_STALE` | `freshness` | All matching facts stale | UNKNOWN |
| `FACT_MALFORMED_FRESHNESS` | `freshness` | Freshness metadata malformed | UNKNOWN |
| `FACT_UNAVAILABLE` | `freshness` | Mandatory fact unavailable | UNKNOWN |
| `FACT_EPOCH_MISMATCH` | `freshness` | Source epoch/version mismatch | UNKNOWN |
| `FACT_IDENTITY_CONFLICT` | `freshness` | Same fact identity differs semantically | NEGATIVE |
| `VERIFICATION_ACCEPTED` | `verification` | Verification accepted | POSITIVE |
| `VERIFICATION_FAILED` | `verification` | Verification failed | NEGATIVE |
| `VERIFICATION_UNKNOWN` | `verification` | Verification unavailable/unknown | UNKNOWN |
| `VERIFICATION_CONFLICT` | `verification` | Verified and failed or contradictory facts | NEGATIVE |
| `VERIFICATION_DUPLICATE_DEDUPLICATED` | `verification` | Identical verified duplicate | POSITIVE |
| `VERIFICATION_IDENTITY_MISMATCH` | `verification` | Verifier semantic identity differs | NEGATIVE |
| `VERIFICATION_WRONG_SUBJECT` | `verification` | Subject/candidate differs | NEGATIVE |
| `VERIFICATION_WRONG_SCOPE` | `verification` | Scope differs | NEGATIVE |
| `VERIFICATION_WRONG_CONTEXT` | `verification` | Context differs | NEGATIVE |
| `VERIFICATION_WRONG_BACKEND` | `verification` | Backend differs | NEGATIVE |
| `VERIFICATION_STALE` | `verification` | Verification fact stale | UNKNOWN |
| `VERIFICATION_PROVENANCE_INVALID` | `verification` | Provenance malformed/invalid | NEGATIVE |
| `EVIDENCE_COMPATIBLE` | `evidence-relation` | Evidence composes compatibly | POSITIVE |
| `EVIDENCE_DUPLICATE_DEDUPLICATED` | `evidence-relation` | Identical canonical duplicate | POSITIVE |
| `EVIDENCE_IDENTITY_CONFLICT` | `evidence-relation` | Same identity differs in digest/content | NEGATIVE |
| `EVIDENCE_CONTRADICTORY` | `evidence-relation` | RFC contradiction | NEGATIVE |
| `EVIDENCE_NON_COMPARABLE` | `evidence-relation` | RFC non-comparable relation | UNKNOWN |
| `EVIDENCE_RELATION_UNKNOWN` | `evidence-relation` | Relation unavailable/unknown | UNKNOWN |
| `EVIDENCE_WRONG_SCOPE` | `evidence-relation` | Evidence cannot cover target scope | NEGATIVE |
| `EVIDENCE_WRONG_CONTEXT` | `evidence-relation` | Evidence context differs | NEGATIVE |
| `EVIDENCE_STALE` | `evidence-relation` | Evidence stale | NEGATIVE |
| `EVIDENCE_REVOKED` | `evidence-relation` | Evidence revoked | NEGATIVE |

Positive codes are emitted for every applicable predicate under one global
policy; this includes accepted, duplicate-deduplicated, and ignored-extra
conditions. `NOT_APPLICABLE` remains trace-only and has no reason code.
Multiple applicable codes are all emitted; none is selected as a primary
reason. Codes are ordered first by the canonical PredicateID order, then by
the registry table order above, then by canonical subject/reference identity.
Input order, concurrency, and discovery order cannot affect this sequence.

ReasonCodeRegistry version 1 is a compatibility surface. Existing codes
cannot be silently reused for another condition. A new semantic condition
requires a new code or registry version; historical traces retain the
semantics of the version they record.

### Reason specificity and membership normalization

ReasonCodeRegistry version 1 assigns each code one relation: `LEAF`,
`PARENT_OF(codes)`, or `INDEPENDENT`. A parent is generic. The normative
`NormalizeReasons(applicableConditions)` operation first derives canonical
semantic conditions, maps each to its exact registered code, removes a parent
when an applicable descendant represents the same condition, retains codes
for independent conditions, deduplicates identical code-plus-subject
bindings, and then orders the result by PredicateID, registry order, and
canonical subject/reference identity. This is reason membership normalization,
not result precedence. There is no primary reason.

The same-condition rule is exact: specificity suppresses only a generic code
describing the same canonical condition. Independent failures are never
suppressed.

| Family | Parent / generic | Specific leaves | Exact emission |
|---|---|---|---|
| Bundle identity | `BUNDLE_IDENTITY_MISMATCH` (`PARENT_OF`) | `BUNDLE_KIND_MISMATCH`, `BUNDLE_ID_MISMATCH`, `BUNDLE_VERSION_MISMATCH`, `BUNDLE_DIGEST_MISMATCH` | Emit applicable specific leaves; emit the parent only for an identity mismatch with no applicable specific leaf. Multiple differing fields emit all corresponding leaves, never the parent. |
| Current-fact conflict | `FACT_CONFLICT_FRESH` (`PARENT_OF` same-identity conflict only) | `FACT_IDENTITY_CONFLICT` | Same canonical fact identity with differing semantic content emits `FACT_IDENTITY_CONFLICT` only. Distinct applicable fresh authoritative facts with contradictory results emit `FACT_CONFLICT_FRESH`. Separate independent subject pairs may emit both. |
| Verification conflict | `VERIFICATION_CONFLICT` (`INDEPENDENT`) | `VERIFICATION_IDENTITY_MISMATCH` (`LEAF`) | A fact bound to the wrong verifier emits `VERIFICATION_IDENTITY_MISMATCH`. Otherwise correctly bound applicable facts with contradictory verification states emit `VERIFICATION_CONFLICT`; the generic conflict never describes an identity mismatch. |
| Evidence conflict | `EVIDENCE_IDENTITY_CONFLICT` (`LEAF`) / `EVIDENCE_CONTRADICTORY` (`INDEPENDENT`) | none | Same evidence identity with different digest/content emits `EVIDENCE_IDENTITY_CONFLICT`. Distinct valid evidence objects whose RFC-0003 relation is contradictory emit `EVIDENCE_CONTRADICTORY`. They are not both emitted for the same pair. |

Thus the canonical examples are: kind mismatch →
`BUNDLE_KIND_MISMATCH`; ID plus digest mismatch →
`BUNDLE_ID_MISMATCH`, `BUNDLE_DIGEST_MISMATCH`; same fact identity with
different content → `FACT_IDENTITY_CONFLICT`; distinct fresh contradictory
facts → `FACT_CONFLICT_FRESH`; wrong verifier →
`VERIFICATION_IDENTITY_MISMATCH`; correctly bound `VERIFIED` plus `FAILED` →
`VERIFICATION_CONFLICT`; same evidence identity with different content →
`EVIDENCE_IDENTITY_CONFLICT`; distinct contradictory evidence →
`EVIDENCE_CONTRADICTORY`. A bundle kind mismatch plus root-trust mismatch
emits `BUNDLE_KIND_MISMATCH`, `BUNDLE_ROOT_TRUST_MISMATCH`.

## Evaluation phases

The implementation evaluates the following complete predicate set, derived
from RFC-0003:

1. binding and exact bundle consistency;
2. AuthorityRule, registry, and root-trust identity consistency;
3. BackendSecurityEnvelope;
4. scope containment;
5. evidence relation and composition;
6. coverage;
7. completeness;
8. certification and verification facts;
9. independent adequacy;
10. baseline applicability and composition;
11. provenance and issuer trust;
12. exact CompatibilityRule and context equivalence;
13. current-fact freshness and validity;
14. temporal validity and revocation; and
15. aggregate result and immutable record material.

The list is a predicate inventory, not an authority-changing short-circuit
order. No phase may treat a prior positive result as proof of a later one.

## Bundle consistency

Before consuming any resolved object, the evaluator performs the pure
`ValidateBundleConsistency(AuthorityMetadata, ResolvedAuthorityBundle)`
operation with result `CONSISTENT`, `INCONSISTENT`, or `UNKNOWN`. A requested
reference supplied with a different kind, ID, version, digest, registry
binding, root-trust identity, or semantic type is `INCONSISTENT`. Duplicate
same-identity objects with different canonical content are `INCONSISTENT`.
An authoritative resolution proving a mandatory object absent is a known
negative; an unavailable resolution fact is `UNKNOWN`. No re-resolution is
performed.

Unreferenced immutable objects are ignored deterministically and cannot enter
evaluation. They are retained only as non-semantic bundle provenance. Known
bundle inconsistency maps to a negative predicate and therefore
`INELIGIBLE`; unavailable consistency evidence maps to `UNKNOWN`.

## Current authority fact aggregation

The pure `EvaluateCurrentFacts(requiredSubjects, CurrentAuthorityFacts,
EvaluationTime)` operation matches facts by exact canonical subject identity,
fact kind, required source identity, and source epoch/version. It rejects
wrong-subject and wrong-kind facts as non-satisfying; they never satisfy
another requirement. Identical canonical duplicates may be deduplicated.

For each required subject, conflicting fresh authoritative facts are a known
contradiction and produce a negative result; they are never resolved by
"newest wins" or input order. A fresh negative dominates unknown and stale
positive facts. All matching facts stale, malformed freshness metadata, or an
unavailable mandatory source produces `UNKNOWN` unless a known negative is
present. Conflicting freshness contracts for one canonical fact identity
cannot produce positive authority and are negative when contradictory, or
`UNKNOWN` when the contradiction itself cannot be established.

Freshness boundaries are inclusive: a fact is fresh iff
`EvaluationTime <= ObservedAt + maxAge`, and validity is current iff
`EvaluationTime <= validUntil`. No ambient clock is used. Mutable facts never
inherit immutable-object cache semantics.

## Verification and evidence aggregation

Every `VerificationFact` must bind the verifier semantic identity and digest,
property, subject/artifact and BoundCandidate identities, scope, context,
backend, validity/freshness, and provenance. A fact for another candidate,
scope, context, or verifier is non-satisfying and cannot be replayed. The
evaluator never executes a verifier.

For one required verification subject, aggregation is deterministic:

* all `VERIFIED` facts with identical canonical identity may be deduplicated;
* `FAILED` or known-invalid/contradictory facts dominate `UNKNOWN` and
  `VERIFIED`, yielding a negative predicate;
* `VERIFIED + UNKNOWN` yields `UNKNOWN` when the unknown fact is mandatory;
* `UNKNOWN` alone yields `UNKNOWN`;
* conflicting verifier identities, wrong subjects, malformed or stale
  verification facts are negative when known and otherwise `UNKNOWN`.

Evidence uses the same canonical deduplication and ordering. Same identity
with different digest/content is a contradiction. RFC-0003
`EvidenceRelation` remains authoritative: `CONTRADICTORY` is negative,
`NON_COMPARABLE` and `UNKNOWN` cannot widen authority, wrong-scope/context
evidence cannot satisfy a target, and stale or revoked evidence cannot
contribute positive authority. Evidence input order has no semantic effect.

## Deterministic precedence and short-circuiting

All applicable mandatory predicates MUST be evaluated for the normative
operation. No negative or unknown result may omit later predicates from the
normative trace. An implementation MAY optimize internal computation only
when it still emits the complete canonical predicate set and trace defined
above; there is no "first failure wins" behavior.

Aggregation is a lattice with this precedence:

`NEGATIVE > UNKNOWN > POSITIVE`.

Therefore any known invalid, contradictory, revoked, incompatible, malformed,
or explicit negative predicate yields `INELIGIBLE`, even when another
predicate is `UNKNOWN`. If no predicate is negative and any mandatory fact is
missing, unavailable, stale, or unknown, the result is `UNKNOWN`. Only when
every mandatory predicate is positive is the result `ELIGIBLE`.

## Backend, scope, evidence, and coverage

The evaluator consumes the resolved backend envelope and does not duplicate
backend minimums in backend code. For container-wide deny-by-default
Seccomp, the envelope includes `STARTUP_BOOTSTRAP`, `CONTAINER_LIFETIME`,
`PROCESS_TREE`, `EXECUTABLE_SET`, `WORKLOAD_STATE`, `ARCHITECTURE_ABI`,
`IMAGE_IDENTITY`, and `KERNEL_RUNTIME_COMPATIBILITY`.

`ScopeCovers`, evidence relation, coverage, completeness, and adequacy remain
separate pure predicates. `COMPATIBLE` evidence may compose under the bound
rule; `CONTRADICTORY` is negative; `NON_COMPARABLE` and `UNKNOWN` cannot widen
authority. Finite empirical observation cannot establish universal lifetime
authority. SPO Ready and backend materialization are evidence/provenance
inputs only.

ObservationScope and EnforcementScope are never substituted. Consequently,
bounded binary observation lacking startup or lifecycle authority makes the
container-wide Seccomp envelope `UNSATISFIED` and returns `INELIGIBLE`; if the
missing facts are genuinely unavailable, it returns `UNKNOWN`.

## Adequacy, certification, trust, compatibility, and composition

The evaluator consumes only the closed RFC-0003 classes:
`STRUCTURAL_BASELINE`, `EXTERNAL_CERTIFICATION`, `BACKEND_FORMAL_INVARIANT`,
`BOUNDED_BEHAVIORAL`, and `TRUSTED_BASELINE_OBSERVED_DELTA`. It validates
immutable evidence identity, scope, context, verifier semantic identity,
current trust/revocation facts, validity, and freshness. It does not execute
verifiers or accept owner assertions, approval, CandidateDigest, SPO Ready,
materialization, attachment, or one successful run as adequacy.

Certification is evaluated from its canonical object and verification facts;
missing trust, provenance, revocation, or verification facts produce the RFC
result dictated by precedence. Compatibility and composition use only the
exact resolved semantic rule/operator. No local architecture alias, version
heuristic, callback, optimizer, or plugin may replace it.

Verification occurs before this service through controlled adapters. A
`VerificationFact` must bind subject, verifier semantic identity, input
schema, scope, context, result, freshness, provenance, and validity. The
evaluator verifies those bindings; it never performs verification I/O.

## Current facts, context, and time

Every positive mutable fact must satisfy its source/policy freshness contract
at `EvaluationTime`. Stale or unavailable mandatory facts cannot be refreshed
inside the evaluator and therefore produce `UNKNOWN` unless RFC-0003 defines
a known negative. A known context mismatch is `INELIGIBLE`; an unavailable
mandatory current context field is `UNKNOWN`. Diagnostic Kubernetes values
such as Pod UID, resourceVersion, container ID, timestamps, and node IP are
not used as equivalence heuristics.

## EligibilityRecord boundary and replay

The evaluator returns immutable record material; the lifecycle/persistence
owner creates and stores `EligibilityRecord`. The material binds the exact
BoundCandidateDigest, AuthorityMetadataDigest, rule and registry identities,
root-trust identity, context, evidence/provenance, current-fact identities,
evaluation time, deadline, and result. It contains no authorization state.

Historical replay uses the recorded bundle, CurrentAuthorityFacts, context,
and time and must reproduce the historical result without ambient state. It
does not establish current authorization; current decisions require current
facts and applicable root trust.

## Backend migration and extensibility

Internal bounded Seccomp observation remains `DERIVED_ADVISORY` until its
resolved envelope and evidence satisfy RFC-0003. SPO adapters provide copied
profiles, recording identity, coverage, and provenance, but have no evaluator
shortcut. PodLock is evaluated only against a resolved PodLock/backend
envelope; this ADR does not impose Seccomp dimensions on another backend.

An unknown backend or envelope is never positive. A new backend adds a
registered envelope and recognized evidence semantics without changing the
aggregation lattice or defaulting unknown values to success.

## Normative regression matrix

The following outcomes are required: an `UNKNOWN` predicate followed by a
`NEGATIVE`, or a `NEGATIVE` followed by `UNKNOWN`, yields `INELIGIBLE` with
the complete canonical trace; two negatives also yield `INELIGIBLE`; all
positive predicates except one unknown yield `UNKNOWN`; all positive
predicates yield `ELIGIBLE`. A bundle R1/R2 mismatch is `INELIGIBLE`, a
proven mandatory object absence is negative, and an unavailable mandatory
object is `UNKNOWN`. Identical current-fact/evidence duplicates are
deduplicated; conflicting fresh facts, same-identity different-digest
evidence, or `VERIFIED + FAILED` are negative; wrong-subject facts are
non-satisfying; stale positive plus fresh negative is negative; evidence
reordering and concurrent execution produce identical traces. Historical
replay uses only recorded facts and time. The certified bounded Seccomp case
remains `INELIGIBLE` for known missing envelope dimensions and `UNKNOWN` when
those dimensions are unavailable.

## Stable reason codes

Reason codes are stable identifiers mapped to RFC predicates, including:
`BINDING_INVALID`, `RULE_UNAVAILABLE`, `ENVELOPE_UNSATISFIED`,
`ENVELOPE_UNKNOWN`, `SCOPE_UNSATISFIED`, `COVERAGE_UNKNOWN`,
`ADEQUACY_INADEQUATE`, `ADEQUACY_UNKNOWN`, `BASELINE_INCOMPATIBLE`,
`PROVENANCE_INVALID`, `TRUST_UNKNOWN`, `REVOKED`, `CONTEXT_MISMATCH`, and
`FACT_STALE`. Implementations may add diagnostics but may not change a
reason's semantic mapping. Expected security outcomes are semantic results,
not generic programming errors. Programming errors are reserved for an
impossible internal invariant or unsupported evaluator implementation
version, and cannot be interpreted as `ELIGIBLE`.

## Security invariants

1. The evaluator performs no ambient lookup, clock read, or side effect.
2. Identical canonical inputs produce identical semantic results and traces.
3. Known negative dominates `UNKNOWN`; `UNKNOWN` never becomes `ELIGIBLE`.
4. Approval, readiness, materialization, and attachment are not adequacy.
5. Authority objects are not mutated by evaluation.
6. Positive current facts must be freshness-valid at explicit EvaluationTime.
7. Historical replay cannot become current authorization.
8. Unknown backend/envelope cannot be positive.
9. The bounded Seccomp counterexample cannot be `ELIGIBLE`.
10. Legacy proposals receive no synthesized authority metadata.
11. Unsupported verifier execution or callback cannot contribute authority.
12. Bundle/reference inconsistency is never silently ignored.
13. Conflicting authoritative current facts cannot produce positive authority.
14. Wrong-subject, wrong-kind, stale, or malformed facts cannot satisfy a requirement.
15. Normative traces are complete and canonical, independent of computation order.

## Alternatives considered

* **Evaluator embedded in apply-proposal:** rejected because it couples
  security semantics to Kubernetes side effects and prevents deterministic
  replay.
* **Evaluator coupled to resolvers/Kubernetes:** rejected because ambient
  lookup makes identical inputs non-reproducible and creates TOCTOU paths.
* **External policy engine:** rejected as the authority boundary because it
  would introduce another semantic interpreter; it may be an adapter only if
  it returns the exact RFC predicate facts.
* **Backend-specific evaluators:** rejected as the primary design because
  they duplicate precedence and permit inconsistent unknown handling.
* **Pure explicit-snapshot evaluator:** selected for deterministic testing,
  replay, and cross-language implementation.

## Handoffs

* ADR-E owns proposal persistence, lifecycle transitions, and creation of the
  stored EligibilityRecord from evaluator material.
* ADR-F owns the apply-time authorization gate and backend side effects.
* ADR-G owns backend evidence collection, SPO/PodLock adapters, and verifier
  execution adapters.
* ADR-H owns revalidation scheduling, current-fact refresh, expiry,
  revocation handling, and runtime remediation.

None may redefine RFC-0003 predicates, precedence, or result meanings.

## Testing requirements

Future implementation work MUST include:

* unit and table tests for every predicate and RFC totality/precedence case;
* properties that identical inputs reproduce results, adding `UNKNOWN`
  cannot improve a result, and adding a known negative cannot improve it;
* evidence-set reordering tests, stale-fact tests, context mismatch tests,
  trust and provenance negatives, and unsupported-backend tests;
* negative-security tests for hidden lookup, ambient clock, approval as
  adequacy, SPO Ready authority, arbitrary verifier execution, and legacy
  synthesis;
* Go/Rust semantic result vectors and historical replay tests; and
* the certified A/B/C/D Seccomp differential regression.

## Consequences and open questions

The evaluator is cheap, replayable, and independently testable, but adapters
must prepare complete immutable snapshots and current facts before evaluation.
Concrete Go types, persistence, controller scheduling, verifier execution,
and physical remediation remain later ADR/implementation decisions.
