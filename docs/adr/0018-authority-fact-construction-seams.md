# ADR-0018: Authority-Fact Construction and Requirement-Matching Seams

Status: Proposed

Date: 2026-08-25

## Decision question

How does the system move from resolved P2/P2.5/P2.6-A material to a future
pure eligibility evaluator without allowing callers to mint security
conclusions?

RFC-0003, RFC-0004, ADR-0010 through ADR-0017, and the accepted P2.5 and
P2.6-A implementations remain authoritative.

## Central construction-authority invariant

Callers may provide observations, references, candidate material, and other
non-authoritative inputs. They must not directly construct a positive
security conclusion by selecting a boolean, enum, or state value.

Positive facts are constructed only by the component that owns the relevant
resolution, verification, or derivation authority. In particular, ordinary
callers cannot mint `TRUSTED`, `VERIFIED`, `NOT_REVOKED`, `COMPATIBLE`,
requirement satisfaction, or `ELIGIBLE`.

## Semantic pipeline

The normative direction is:

```text
resolved/bound material
    -> producer-owned typed facts
    -> typed requirement relationships
    -> pure eligibility evaluation
```

This layer and P3 do not parse raw RFC-0004 JSON, resolve arbitrary
references, canonicalize AuthorityRule objects, compute semantic identity,
perform external I/O, read an ambient clock, authorize, approve, apply, or
mutate runtime state.

Identity and current state remain separate: a reference is not its resolved
object; `RevocationReference` is not a current revocation fact; TrustPolicy
identity is not a current trust conclusion; certification identity is not
current certification validity; and baseline identity is not compatibility.

Observation, evidence, coverage, completeness, adequacy, verification,
certification, trust, eligibility, authorization, approval, and runtime state
remain distinct concepts.

## AuthorityRule boundary

P3 consumes `TypedResolvedAuthorityRule` (or an immutable requirement view
derived from it). It never consumes unbound or raw AuthorityRule data. P2.6-A
continues to own strict decoding, canonical identity, digesting, and exact
reference binding.

## Producer-owned fact seams

### Trust fact

A trust fact is an immutable, producer-owned fact. A positive fact binds the
trusted subject, TrustPolicy identity, applicable root-trust configuration,
scope, context, provenance, validity, and current revocation state. Its state
is one of `TRUSTED`, `UNTRUSTED`, or `UNKNOWN`.

Trust resolution owns construction. Security-significant fields are private;
there is no public constructor equivalent to `NewTrustFact(..., Trusted)`.
Root trust is explicit and outside candidate-controlled data. Unknown or
untrusted facts never become positive through P3.

### Compatibility fact

Compatibility is a derived relationship, not an assertion. A compatibility
fact binds the candidate/baseline identities, the bound compatibility-rule
identity, backend, scope/context, validity, and provenance. The positive
state is produced by applying the normative compatibility rule to those typed
operands. Baseline existence does not imply compatibility.

`COMPATIBLE`, `INCOMPATIBLE`, and `UNKNOWN` remain distinct. Compatibility
resolution owns construction; callers cannot set `Compatible: true`.

### Current revocation fact

`CurrentRevocationFact` is separate from `RevocationReference`. It binds the
subject, revocation source/reference, current state, observation/resolution
instant, freshness/validity metadata, and provenance. Its
states are `NOT_REVOKED`, `REVOKED`, and `UNKNOWN`.

Missing or stale current information is `UNKNOWN`, never `NOT_REVOKED`.
`RevocationReference` alone has no current-state meaning. Revocation
resolution owns construction; P3 performs no lookup and no ambient-clock
access.

### Verification fact

A verifier-owned fact binds subject, verifier identity, property,
scope/context, validity, and the revocation relationship required for the
subject, plus provenance. Its
state is `VERIFIED`, `FAILED`, or `UNKNOWN`. Only the verifier/resolution
boundary MUST construct any positive form. Verification is not certification and
neither is eligibility.

### Certification fact

A certification fact binds the certified subject, property, issuer/verifier,
scope, context, validity, revocation relationship, provenance, and immutable
identity. Certification existence does not imply current validity or
successful verification.

### Coverage fact

Coverage facts originate from typed evidence and backend resolution. Seccomp
coverage preserves exactly the RFC-0004 mandatory dimensions. Missing,
unknown, not-covered, invalid, duplicate, or mismatched dimensions cannot
establish positive coverage.

### Completeness fact

`CompletenessClass` is evidence, not proof by itself. A completeness fact
binds subject, scope, evidence, issuer/provenance, validity, and a
revocation relationship for every revocable subject. Non-revocable subjects
carry an explicit absence of that relationship. Unknown or unavailable
completeness remains unresolved.

### Adequacy evidence

`AdequacyEvidence` is distinct from adequacy satisfaction and eligibility.
P3 derives adequacy satisfaction by matching an AuthorityRule requirement
against producer-owned typed evidence. There is no public `Adequate=true`
fact.

## Requirement matching

The relationship between an AuthorityRule requirement and a fact is derived
from typed operands. Security-significant satisfaction is not represented by
`map[string]bool`, generic predicate inputs, callbacks, arbitrary expressions,
or free-form semantic strings.

A relationship has one of these outcomes:

* `SATISFIED` / proven;
* `REFUTED` / negative;
* `UNKNOWN` / unresolved;
* `NON_MATCHING`;
* `INVALID`.

Matching requires every identity dimension mandated by the relevant object
and rule: AuthorityRule identity, requirement identity, subject, backend,
scope, context, verifier/property, baseline and compatibility identities,
validity, revocation source, and the family-specific provenance requirements
listed in the matching table below.
A mismatched fact never contributes positively.

## Validity and evaluation time

P3 receives an explicit `EvaluationAt`. Where direct interval evaluation is
normatively appropriate, validity is inclusive:

```text
notBefore <= EvaluationAt <= notAfter
```

Malformed intervals are invalid. Expired and not-yet-valid facts are not
positive. No component in this layer reads `time.Now()`.

## Immutable fact snapshot

One immutable `EvaluationFactSnapshot` represents the facts for one
evaluation attempt. It prevents mixing facts from incompatible resolution
attempts and carries the source/epoch consistency already supplied by the
resolution model. The snapshot is not an eligibility result and has no
terminal authority meaning.

Fact replacement supports non-monotonic transitions such as trusted to
untrusted, not-revoked to revoked, verified to failed, compatible to
incompatible, and valid to expired. A prior eligibility result has no
authority over a later snapshot; P3 reevaluates the new snapshot.

## Unknown, absent, negative, positive, and invalid

| Fact family | ABSENT | UNKNOWN | NEGATIVE | POSITIVE | INVALID |
|---|---|---|---|---|---|
| Trust | unavailable/unknown as required | unresolved | untrusted | trusted proof | construction error |
| Revocation | unknown, never not-revoked | unresolved | revoked | not-revoked fact | construction error |
| Verification | unavailable | unknown | failed | verified fact | construction error |
| Compatibility | unknown | unknown | incompatible | compatible fact | construction error |
| Coverage | missing requirement | unknown | not covered | covered proof | construction error |
| Completeness | missing evidence | unknown | insufficient | proven fact | construction error |
| Adequacy | missing evidence | unknown | insufficient | matched evidence | construction error |
| Certification | missing certification | unknown | invalid/failed | valid bound fact | construction error |

`UNKNOWN` is not `INVALID`, `ABSENT`, or positive. Invalid input is an error,
not a normalized unknown value. Required absence never contributes positively.

## Fail-closed eligibility rule

No mandatory AuthorityRule requirement contributes positively unless a
validated typed proof relationship exists. Unknown, absent, malformed, or
mismatched facts cannot establish eligibility.

P3 derives only `ELIGIBLE`, `INELIGIBLE`, or `UNKNOWN`, with deterministic
provenance-preserving reasons. It must not produce `AUTHORIZED`, `APPROVED`,
`READY_TO_APPLY`, `APPLIED`, or runtime state.

## Construction-authority table

| Fact/relationship | Producer | Publicly mintable? | Required inputs | Positive-state authority |
|---|---|---:|---|---|
| TypedResolvedAuthorityRule | P2.6-A binder | No | strict bytes, exact reference, digest | binding authority |
| Trust fact | trust resolver | No | TrustPolicy, root trust, subject, scope/context, validity, revocation, provenance | trust resolver |
| Current revocation fact | revocation resolver | No | source/reference, subject, current observation, freshness, provenance | revocation resolver |
| Verification fact | verifier boundary | No | verifier, subject, property, scope/context, validity, revocation, provenance | verifier |
| Certification fact | certification resolver | No | subject, property, issuer/verifier, scope/context, validity, revocation, provenance | certification resolver |
| Compatibility fact | compatibility resolver | No | candidate, baseline, rule, backend/scope/context, validity, provenance | compatibility resolver |
| Coverage fact | evidence/backend resolver | No | typed dimensions, evidence, scope/context, validity, provenance | evidence/backend resolver |
| Completeness fact | evidence resolver | No | subject, class, scope, evidence, issuer/provenance, validity | evidence resolver |
| Adequacy evidence | evidence/verifier boundary | No positive conclusion | typed evidence and bindings | P3 derives match |
| Requirement match | package-internal derivation | No | bound AuthorityRule requirement and typed fact | derivation |
| EvaluationFactSnapshot | snapshot assembler | No | one consistent fact set/epoch | snapshot assembler |
| EligibilityResult | P3 only | No direct construction | bound rule, snapshot, EvaluationAt | P3 derivation |

## Go visibility consequences

Security-significant positive fields MUST be private. An exported type does
not imply public minting authority. If package-private constructors are too
broad because unrelated files in `internal/authority` can invoke them,
producer-specific internal packages may own construction while a neutral
immutable fact package owns value types. Any package restructuring is an
implementation choice, but the resulting supported API MUST satisfy the
producer derivation property defined below.

## Future P3 boundary

The conceptual input is:

```go
type EligibilityEvaluationInput struct {
    Authority    TypedResolvedAuthorityRule
    Facts        EvaluationFactSnapshot
    EvaluationAt time.Time
}
```

`PredicateInput` and every equivalent caller-controlled terminal predicate
state API are prohibited.

## Determinism and duplicate facts

Equivalent fact sets produce equivalent relationships independent of source
ordering. Duplicate/conflicting facts have explicit fail-closed semantics;
they are never last-write-wins. Snapshot assembly rejects incompatible
duplicates or preserves multiple observations only where the object family
normatively defines observation multiplicity.

## Attack consequences

* caller-selected `TRUSTED`, `VERIFIED`, `NOT_REVOKED`, or `COMPATIBLE` is impossible;
* a reference without a current revocation fact is not positive;
* stale revocation is not positive;
* wrong rule, subject, scope, context, verifier, or property is non-matching;
* inadequate provenance is rejected or unresolved;
* conflicting duplicates, mixed snapshots, zero values, and enum `99` fail closed;
* reordered equivalent facts retain identical semantics;
* a prior positive snapshot is reevaluated after a negative replacement;
* direct `ELIGIBLE` construction is prohibited.

## Rejected alternatives

This decision rejects:

1. generic `PredicateInput`;
2. caller-supplied positive enum states;
3. map-based requirement satisfaction;
4. treating `RevocationReference` as `NOT_REVOKED`;
5. treating TrustPolicy existence as `TRUSTED`;
6. treating certification existence as `VERIFIED`;
7. treating baseline existence as `COMPATIBLE`;
8. treating evidence existence as `ADEQUATE`;
9. cached eligibility across fact snapshots;
10. ambient clock lookup in P3.

## Compatibility with accepted phases

P0 typed-domain safety is preserved because positive conclusions are produced
only by the accepted validated resolution/verification/derivation paths, and
because invalid and unknown states remain explicit. Private fields and
constructors provide API encapsulation and help prevent ordinary callers from
selecting positive states; they are not the semantic root of authority or a
same-process security boundary. P1 canonical and digest determinism remains owned by
binding and semantic identity. P2 resolution and immutable snapshot semantics
remain upstream. P2.5 remains an evaluation-domain fact/requirement layer,
not a terminal evaluator. P2.6-A remains the sole AuthorityRule identity and
binding layer. RFC-0003 distinctions among observation, evidence, authority,
authorization, and runtime state remain explicit.

## Implementation consequences

Implementation must proceed in this order:

1. typed authority-fact primitives;
2. controlled producer-owned construction seams;
3. immutable `EvaluationFactSnapshot`;
4. typed requirement matching;
5. adversarial acceptance review;
6. P3 evaluator rewrite.

No evaluator implementation is authorized by this ADR alone.

## Normative precision rules

The following rules are binding and remove implementation discretion from the
construction and matching seams.

### Compatibility operands and result

Every `CompatibilityFact` binds this exact operand tuple:

```text
candidate identity
baseline identity
AuthorityRule identity and compatibility requirement identity
backend identity
scope
context
compatibility-rule identity and predicate
validity
revocation source/current fact
typed provenance
```

No operand may be omitted for convenience. A predicate that does not inspect
an operand still records that operand as part of the bound relationship; the
predicate's definition, not an implementation choice, determines that it is
non-discriminating. The compatibility resolver alone derives `COMPATIBLE` or
`INCOMPATIBLE`. Unavailable but structurally valid evidence is `UNKNOWN`;
malformed or inconsistent operands are `INVALID` and cause an error.

### Temporal classification

For a structurally valid interval, validity is inclusive:

```text
EvaluationAt < notBefore  -> REFUTED / temporally non-applicable
EvaluationAt == notBefore -> applicable
notBefore < EvaluationAt < notAfter -> applicable
EvaluationAt == notAfter  -> applicable
EvaluationAt > notAfter   -> REFUTED / temporally non-applicable
```

Thus a valid but not-yet-valid or expired fact cannot satisfy a mandatory
requirement and contributes a `REFUTED` requirement outcome. A malformed or
inverted interval is `INVALID` and causes an evaluation error. Unknown
current information remains `UNKNOWN` and is not converted into temporal
non-applicability.

### Snapshot identity and consistency

`ResolutionAttemptIdentity` originates at the trusted orchestration/resolution
boundary when one coherent resolution/evaluation attempt begins. It is
immutable, opaque, equality-comparable, and is not derived from wall-clock
time or mutable global state. Producers propagate that exact identity into
every fact belonging to the attempt; they MUST NOT invent unrelated identities
for facts intended for one snapshot. An `EvaluationFactSnapshot` receives
already identified facts, preserves the identity unchanged, and MUST contain
only facts carrying that exact identity. Snapshot assembly never creates or
replaces the identity.

A fact lacking an identity, or carrying a different identity, makes snapshot
construction `INVALID` and causes an evaluation error. A snapshot assembler
must reject mixed attempts before P3 receives the snapshot. A snapshot is not
an eligibility result.

### Duplicate and conflict semantics

For singleton security relationships, an exact duplicate with identical
semantic identity and value is canonically deduplicated. Two facts with the
same semantic identity and different security state or value are a conflict:
the snapshot is `INVALID` and evaluation returns an error. There is never
last-write-wins, first-write-wins, or insertion-order precedence.

Fact families that normatively represent multiple observations must assign a
distinct semantic observation identity to each observation. Repeating the
same observation identity is an exact duplicate and is deduplicated;
conflicting repeats are invalid. Reordering equivalent facts produces the
same snapshot semantics and result.

### Requirement-match identity and outcomes

Every `RequirementMatch` has this mandatory base identity tuple:

```text
AuthorityRule identity
exact requirement identity
subject identity
backend identity
scope
context
ResolutionAttemptIdentity
```

Family-specific required identity is fixed as follows:

* trust: TrustPolicy/root-trust identity, trust subject, revocation source;
* verification: verifier identity, verification subject, property;
* certification: issuer/verifier, certification property, certification identity;
* compatibility: candidate, baseline, compatibility-rule identity and predicate;
* coverage: exact required dimensions and evidence/backend source;
* completeness: completeness class and evidence identity;
* adequacy: adequacy requirement/class and evidence identity;
* revocation: RevocationReference/source and subject;
* provenance: the producer/source identity required by the owning fact family.

These dimensions are mandatory for the named family; implementations may not
drop them. A fact from another identity is `NON_MATCHING`, never positive.

Match outcomes are exact:

* `SATISFIED`: a valid typed proof establishes the requirement;
* `REFUTED`: a valid applicable typed fact establishes the negative condition;
* `UNKNOWN`: valid required information is unavailable or explicitly unknown;
* `NON_MATCHING`: the fact is valid but belongs to another semantic identity;
* `INVALID`: malformed, inconsistent, conflicting, mixed-snapshot, or impossible input.

`INVALID` causes an error, `NON_MATCHING` is excluded from evidence for the
requirement, and only `SATISFIED` contributes positive proof.

For all relevant facts for one requirement, aggregation precedence is:

```text
INVALID > REFUTED > SATISFIED > UNKNOWN
```

If there is no relevant matching fact and only non-matching facts, the result
is `UNKNOWN`.

### Eligibility aggregation

After all matches are derived:

```text
any INVALID              -> evaluation error
any REFUTED              -> INELIGIBLE
all mandatory SATISFIED  -> ELIGIBLE
otherwise                -> UNKNOWN
```

This is the sole positive-proof rule. Absence, unknown, stale, expired,
not-yet-valid, non-matching, or malformed facts cannot establish `ELIGIBLE`.

### Positive-fact provenance

Every positive security fact must carry typed provenance containing, at
minimum, producer/authority identity, subject, `ResolutionAttemptIdentity`,
and the source material or reference used by the owning fact family.
Family-specific provenance requirements remain mandatory. Diagnostic text is
never semantic proof.

### Cross-implementation contract

| Case | Required result |
|---|---|
| caller-selected positive state | construction rejected / `INVALID` |
| missing mandatory fact | `UNKNOWN` |
| explicit unknown fact | `UNKNOWN` |
| explicit negative fact | `REFUTED` / `INELIGIBLE` |
| structurally invalid fact | `INVALID` / error |
| wrong semantic identity | `NON_MATCHING` |
| wrong snapshot identity | `INVALID` / error |
| conflicting same-identity facts | `INVALID` / error |
| reordered equivalent facts | same result |
| not-yet-valid or expired fact | `REFUTED` / `INELIGIBLE` |
| evaluation at `notBefore` or `notAfter` | applicable |
| missing or stale revocation fact | `UNKNOWN` |
| `REVOKED` | `REFUTED` |
| correctly bound `NOT_REVOKED` | can satisfy the revocation requirement only after all required identity, freshness, validity, provenance, and snapshot checks pass |
| all mandatory requirements satisfied | `ELIGIBLE` |
| one mandatory requirement refuted | `INELIGIBLE` |
| no refutation and at least one unknown | `UNKNOWN` |

These classifications are normative for Go, Rust, and any other conforming
implementation.

## Root of authority and threat model

This ADR adopts the API/TCB model. Trusted producer, resolver, verifier, and
matching implementations compiled into the landlock-genprof binary are part
of the trusted computing base (TCB). P2.6-B protects untrusted runtime input
and ordinary API consumers from selecting positive conclusions; it does not
defend against malicious source code deliberately compiled into that same
trusted binary. Defending against that actor requires a stronger isolated
producer architecture (separate process or binary, authenticated IPC,
attestation, or equivalent), which is outside the accepted RFC/ADR scope.

Go visibility is therefore an API-integrity mechanism, not a cryptographic
same-process boundary. Supported APIs MUST NOT accept caller-selected positive
enums, booleans, or terminal outcomes. A producer may receive typed material,
but its positive result MUST be the output of the complete normative
resolution, verification, or pure derivation. Constructing a resolver or
invoking a pure derivation is not itself a positive fact.

The root-of-authority chain is:

```text
validated source material/reference
  -> designated resolution, verification, or pure derivation
  -> producer-owned immutable typed fact
  -> snapshot admission and requirement matching
  -> P3 eligibility result
```

Trust, verification, revocation, compatibility, coverage, completeness,
adequacy, certification, and requirement matching remain separate producer
contracts. No producer may use a supported API to mint another family's
positive conclusion. Requirement satisfaction is derived only by the typed
matching seam and is never implied by fact existence.

## Construction and authenticity rules

Fact authenticity is established by the complete validated construction path,
not by package location alone. A positive fact MUST retain semantic identity,
producer/provenance, source reference or material, validity, revocation
relationship, and resolution-attempt identity. Snapshot admission validates
structure, bindings, closed states, identity, duplicate/conflict semantics,
immutability, and one-attempt coherence; it does not turn an unvalidated
assertion into an authentic fact.

Implementations MUST use narrow derivation APIs whose inputs are proof-bearing
typed material. They MUST NOT expose `NewFact(state)`, generic capability
issuers, universal fact factories, runtime caller inspection, reflection or
stack checks, global registration, hidden string tokens, or equivalent
security theater. Security-significant representation fields MUST not be
externally assignable.

The enforceable cross-producer property is:

> No supported API in producer A permits producer A, an ordinary consumer, or
> a caller to select or construct producer B's positive semantic conclusion.

This property is normative and language-independent. Go package placement,
private fields, and package-specific constructors are implementation tools;
they are not proof of semantic truth. A Rust implementation MUST enforce the
same derivation and construction invariant.

## Threat actors and deterministic outcomes

| Actor/input | May provide material? | May select a positive conclusion? | Result |
|---|---:|---:|---|
| External API caller | Yes | No | inputs are validated; no direct positive fact |
| Ordinary non-producer consumer | Yes, through supported APIs | No | cannot construct positive fact |
| Legitimate producer | Validated family-specific material | No; derives it | may emit its own positive fact |
| Malformed or stale runtime input | Yes | No | invalid/error or UNKNOWN/negative per family rules |
| Malicious source inserted into trusted producer implementation | Not a P2.6-B actor | Not constrained by package visibility | inside TCB; stronger isolation is required if in scope |

The following cases are normative: caller-selected positive enums are
rejected; legitimate trust or compatibility resolution derives its own
result; forged source material fails validation or remains non-positive;
stale revocation is UNKNOWN; wrong subject, scope, context, or snapshot is
non-matching or invalid; valid authoritative inputs may produce the producer's
own positive fact; and a producer cannot use a supported API to produce a
different family's positive fact.

## Rejected authority models

Package-private constructors or unexported capability tokens in a broad
package are not a same-process security boundary. Universal capabilities,
exported `NewProducer` or factory APIs that let callers select an authority
family, forgeable interfaces, runtime caller inspection, reflection checks,
stack inspection, global registries, and `init`-based authorization are also
rejected.

If future requirements classify producer source code itself as untrusted,
this ADR is insufficient and a new isolated-producer architecture MUST be
approved before implementation.

## Proof-bearing intermediate authenticity

Structural validity is not authenticity. A security-positive producer MUST
NOT treat type membership, non-zero fields, immutability, private
construction, or caller possession of an intermediate object as proof that
the object is authoritative evidence. Each proof-bearing intermediate object
consumed toward a positive conclusion MUST have an explicit authority/source
contract establishing, as applicable, its semantic identity, originating
authority or producer, subject, source material or bound reference, scope,
context, validity, current revocation relationship, typed provenance,
`ResolutionAttemptIdentity`, and family-specific operands.

The required authenticity chain is:

```text
authoritative or validated source material
  -> trusted normative resolution, verification, or derivation
  -> authenticated proof-bearing intermediate object, if any
  -> producer-owned typed fact
  -> snapshot admission
  -> requirement matching
  -> P3 eligibility aggregation
```

Every arrow is a semantic transition. A caller assertion may not be wrapped
as a merely well-formed intermediate object and then treated as authoritative
by a producer. Snapshot admission validates coherence, identity, bindings,
closed states, duplicate/conflict rules, immutability, and attempt identity;
it MUST NOT authenticate an assertion whose upstream authority/source contract
has not already been established.
