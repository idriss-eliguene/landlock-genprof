# ADR-0019: Authoritative Proof-Source Contracts

Status: Proposed

Date: 2026-08-25

## Decision question

What semantic source results are sufficient for trusted P2.6-B producers to
derive typed authority facts without treating caller assertions or structural
validity as authority?

RFC-0003, RFC-0004, ADR-0010 through ADR-0018, and accepted P0–P2.6-A
semantics remain authoritative.

## Source-to-fact pipeline

```text
raw/observed material
  -> parsing and structural validation
  -> authoritative resolution, verification, or pure derivation
  -> proof-bearing source result
  -> producer-owned typed fact
  -> coherent EvaluationFactSnapshot
  -> typed requirement matching
  -> P3 eligibility
```

Source material, a source result, a typed fact, a requirement relationship,
and eligibility are distinct objects. A verifier output is not a
`VerificationFact`; a registry entry is not a `CurrentRevocationResult` or
`CurrentRevocationFact`; and a fact is not a requirement match.

## Authenticity invariant

A proof-bearing source result is authoritative only when it is produced by the
designated resolution, verification, or derivation procedure over validated
source material and carries the family-specific bindings in this ADR.
Structural validity, type membership, immutability, private construction,
package location, or caller possession never establishes authenticity.
Copying fields into another wrapper does not transfer authority.

Every source result contributing to a positive fact binds, as applicable,
semantic identity, producer/authority, subject, source material or reference,
scope, context, validity, current revocation relationship, typed provenance,
and the orchestration-owned `ResolutionAttemptIdentity`.

## Resolution attempt identity

The trusted orchestration/resolution boundary creates one immutable
`ResolutionAttemptIdentity` when an attempt begins. Every source result and
fact belonging to that attempt propagates the same value unchanged. Snapshot
assembly receives facts, rejects zero or mixed identities, and preserves the
identity; it never creates or replaces it.

## Source-result state rules

Malformed semantic input is `INVALID` and returns an error. Valid but
unavailable or unresolved authoritative information is `UNKNOWN`. A valid
authoritative negative result remains negative. Only a complete authoritative
positive result may produce a positive typed fact. Errors are not ordinary
negative security results, and UNKNOWN is never normalized to positive or
negative.

## Family source contracts

### Trust

`TrustResolutionResult` is produced by trust resolution over bound
TrustPolicy/root material. It binds subject, policy/root identity, scope,
context, validity, current revocation result, provenance, source reference,
and attempt identity. `TRUSTED` requires the normative trust relationship;
policy or root existence alone is insufficient. `UNTRUSTED` is an explicit
authoritative negative result; unavailable or unresolved material is
`UNKNOWN`.

### Verification

`VerificationExecutionResult` is produced by the designated verifier or an
accepted verifier-evidence procedure. It binds subject, verifier identity,
property, scope, context, source/evidence identity, validity, revocation,
provenance, and attempt identity. `VERIFIED`, `FAILED`, `UNKNOWN`, and
`INVALID` are distinct. Certification existence never implies VERIFIED.

### Current revocation

`CurrentRevocationResult` is separate from `RevocationReference` and
`CurrentRevocationFact`. It binds subject, exact source/reference, current
state, observation/freshness validity, provenance, and attempt identity.
`NOT_REVOKED` and `REVOKED` require authoritative current resolution;
missing information, lookup failure, stale data, UNKNOWN, wrong source,
wrong subject, or wrong attempt are not NOT_REVOKED. No ambient clock is used.

### Compatibility

`CompatibilityEvaluationResult` is a pure deterministic derivation over the
complete ADR-0018 tuple: candidate, baseline, AuthorityRule and exact
requirement, backend, scope, context, compatibility rule/predicate, validity,
revocation source/current result, provenance, and attempt identity.
The selected schema is `compatibility-rule.v1` or `compatibility-rule.v2`.
V1 contains the original operators only; V2 retains them and adds the
relational `SET_CONTAINS` and `MAP_CONTAINS` operators. Unsupported
schema/operator combinations fail closed and are never downgraded or skipped.

Constraint predicates compare candidate/source material with rule-owned
constraints. `SET_MEMBER` is scalar membership only: the candidate scalar
must belong to the rule-owned `members` set. It is never set or map
containment. Relational predicates compare the `SourceContext` candidate with
the `TargetContext` baseline. `SET_CONTAINS` requires typed sets and derives
`candidateSet ⊆ baselineSet`; `MAP_CONTAINS` requires typed maps and derives
exact candidate key/value entry containment in the baseline map. Both require
candidate and baseline operands and forbid a rule-owned expected operand.
Operand reversal is a distinct relation.

Known true and false predicate results are `COMPATIBLE` and `INCOMPATIBLE`;
authoritatively unavailable or unknown operands are `UNKNOWN`; malformed or
wrong-domain operands, unsupported schema/operator combinations, and supplied
expected operands for containment are `INVALID`. Baseline existence alone is
not compatibility. Predicate results, dimension results, and complete-context
compatibility remain distinct; all selected mandatory requirements participate
with precedence `INVALID > INCOMPATIBLE > UNKNOWN > COMPATIBLE`.

### Coverage

`CoverageObservationResult` is produced from typed backend evidence. It binds
subject, backend, exact required dimensions, scope/context, evidence/source,
provenance, validity where required, and attempt identity. Seccomp retains
the accepted mandatory dimensions. Missing, unknown, duplicate, conflicting,
mismatched, or malformed dimensions cannot produce COVERED.

### Completeness

`CompletenessEvidenceResult` is produced by the accepted evidence source and
binds subject, completeness evidence identity/classification, scope, source or
issuer, validity, revocation, provenance, and attempt identity. The producer
owns the classification; a caller-selected class is not evidence. Completeness
is evidence only and does not imply adequacy or eligibility.

### Adequacy

`AdequacyEvidenceResult` is produced from typed evidence/verifier material and
binds the subject, adequacy evidence identity, scope/context, verifier,
validity, revocation, provenance, and attempt identity. It remains distinct
from adequacy satisfaction, which only the matcher may derive.

### Certification

`CertificationResolutionResult` binds certification identity, subject,
property, issuer/verifier, source certificate identity, scope/context,
validity, revocation, provenance, and attempt identity. Missing, malformed,
expired, not-yet-valid, revoked, or unknown certification is not a valid
positive certification fact. Certification never implies VERIFIED, TRUSTED,
NOT_REVOKED, SATISFIED, or ELIGIBLE.

## Requirement matching

Matching consumes a bound `TypedResolvedAuthorityRule`, the exact requirement,
authentic typed facts, the same attempt identity, and explicit `EvaluationAt`
when temporal applicability is required. No generic predicate map, callback,
expression language, identity-only constructor, or caller-selected outcome is
allowed.

For each family, `SATISFIED` requires a valid proof relationship with all base
identity dimensions (AuthorityRule, requirement, subject, backend, scope,
context, attempt) and the family-specific operands. Wrong identity is
`NON_MATCHING`; a valid negative fact is `REFUTED`; valid unavailable or
unknown information is `UNKNOWN`; malformed, conflicting, or mixed input is
`INVALID`.

Temporal applicability is inclusive: before `notBefore` and after `notAfter`
are `REFUTED`; the boundaries and interior are applicable; malformed or
inverted intervals are `INVALID`.

## Snapshot admission

`EvaluationFactSnapshot` receives already-produced facts. It validates
structure, closed states, semantic identity, required bindings, one attempt,
duplicate/conflict semantics, immutability, and deterministic ordering. It
does not authenticate source material, create positive facts, resolve missing
evidence, assign attempt identity, or perform eligibility.

Exact duplicates deduplicate; conflicting same-identity source results or
facts are errors; reordered equivalent sets have identical semantics.

## Provenance and freshness

Every source result and positive fact carries producer/authority identity,
subject, source material/reference, attempt identity, and family-specific
provenance. Diagnostic text is never proof. Freshness is represented by
explicit validity/observation metadata; stale information is UNKNOWN or the
defined negative result, never silently positive. No ambient clock is used.

## Threat model and TCB

ADR-0018 MODEL A applies. Ordinary callers and malformed runtime input are in
scope; they cannot select positive conclusions. Trusted producer, resolver,
verifier, matcher, and snapshot implementations compiled into the binary are
the TCB. Malicious source deliberately compiled into that TCB is a TCB
compromise outside P2.6-B. Go visibility is API integrity, not cryptographic
authority.

## Conceptual Go consequences

Implementations may introduce family-specific result types such as
`TrustResolutionResult`, `VerificationExecutionResult`,
`CurrentRevocationResult`, `CompatibilityEvaluationResult`,
`CoverageObservationResult`, `CompletenessEvidenceResult`,
`AdequacyEvidenceResult`, and `CertificationResolutionResult`. These names
are illustrative. No universal proof object or state-bearing generic factory
is permitted.

## Attack outcomes

| Attack | Required outcome |
|---|---|
| caller states TRUSTED/VERIFIED/NOT_REVOKED/COMPATIBLE/COVERED/SATISFIED | INVALID/rejected |
| policy/reference/certificate exists alone | UNKNOWN or non-positive |
| missing or stale revocation | UNKNOWN |
| baseline without evaluated relationship | UNKNOWN |
| incomplete or unknown coverage | non-positive/UNKNOWN |
| caller-selected completeness or adequacy class | INVALID/rejected |
| wrong subject/property/scope/context/source/attempt | NON_MATCHING or INVALID |
| unauthenticated wrapper | INVALID/rejected |
| conflicting authoritative results | INVALID/error |
| reordered equivalent results | same semantics |
| expired/not-yet-valid result | REFUTED when matched |
| malformed validity or enum 99 | INVALID/error |
| diagnostic text claiming verification | not proof |
| complete legitimate proof path | corresponding positive fact |

## Compatibility and regressions

This ADR preserves P0, P1, P2, P2.5, P2.6-A, ADR-0018, RFC-0003, and RFC-0004:
evidence is not authority; certification is not verification; revocation
identity is not current state; completeness is not adequacy; adequacy evidence
is not satisfaction; satisfaction is not eligibility; and eligibility is not
authorization or runtime authority.

The accepted RFC-0003/RFC-0004 compatibility schema dispatch is part of this
preservation. `compatibility-rule.v1` cannot contain `SET_CONTAINS` or
`MAP_CONTAINS`; `compatibility-rule.v2` may contain them. No containment source
contract introduces an expected map, expected set, `target == expected`, or
synthetic containment operand.

## Implementation boundary

P2.6-B may be implemented only after these source-result contracts are
represented by trusted derivation/resolution code. P3 remains responsible for
eligibility aggregation and is not defined by this ADR.

Completeness and adequacy matching bind the selected requirement class to the
corresponding fact class with exact equality. A completeness requirement is
SATISFIED only when `requiredClass == CompletenessFact.class`; an adequacy
requirement is SATISFIED only when `requiredClass == AdequacyFact.class`. No
class hierarchy, wildcard, coercion, or stronger/weaker-class implication is
applied. These class operands are explicit family-specific `MatchRequest`
inputs; all other applicability, attempt, validity, and revocation rules are
unchanged.
