# RFC-0005 — Compatibility Predicate Semantics

## Status

Draft normative extension to RFC-0003.

## Purpose and relation to RFC-0003

RFC-0003 defines directional `Compatible(SourceContext, TargetContext,
RuleVersion)` and the compatibility dimensions. This RFC makes that relation
executable. It clarifies RFC-0003; it does not remove or weaken any RFC-0003
authority distinction.

The result domain is exactly `COMPATIBLE`, `INCOMPATIBLE`, `UNKNOWN`, and
`INVALID`. A caller never supplies the result.

## Direction and operands

For an AuthorityRule compatibility requirement, `candidate` is `SourceContext`
and `baseline` is `TargetContext`:

```text
Compatible(candidate, baseline, ruleVersion)
```

Reversing the operands is a different relation and MUST be evaluated
separately. A baseline supporting a candidate does not imply that the
candidate supports the baseline.

The relationship is bound to candidate identity, baseline identity,
AuthorityRule and exact requirement identity, backend, scope, context,
compatibility rule, validity, revocation source/current result, provenance,
and ResolutionAttemptIdentity. These bindings are identity operands even when
the selected predicate compares only one context dimension.

## Compatibility context

`CompatibilityContext` is the tuple:

```text
architecture, abi, kernel, runtime, libc, image,
privilegeCapabilities, namespaceSecurity,
configuration, environmentVariables, featureFlags, persistentState
```

Each component is an opaque semantic identity, except that structured map and
set components are defined below. Missing components are `UNKNOWN`; malformed components are
`INVALID`. No implicit case, whitespace, locale, alias, or Unicode
normalization is performed. Values are compared by their typed semantic
identity, not presentation text.

`architecture` and `abi` are independently identified scalar values.
`privilegeCapabilities` and `persistentState` are finite sets of opaque
identities. `environmentVariables` and `featureFlags` are finite typed maps
from typed names to typed values. Other components are opaque scalar
identities.

An `EnvironmentVariableName` and a `FeatureFlagName` use the same closed ASCII
grammar: the first character is `A-Z`, `a-z`, or `_`; each following character
is `A-Z`, `a-z`, `0-9`, or `_`. Empty names, leading digits, whitespace,
punctuation, embedded NUL, and non-ASCII/Unicode characters are `INVALID`.
No trimming, case folding, Unicode normalization, or locale processing is
performed. Names are case-sensitive.

An environment or feature-flag value is a scalar UTF-8 string containing no
embedded NUL. Empty strings are valid known values. Bytes that are not valid
UTF-8 are `INVALID`; whitespace and case are significant and no normalization
is performed. Each entry is either `Known(value)` or the distinct
authoritative state `Unknown`; `Unknown` is not an ordinary scalar value.
Consequently, `Known("UNKNOWN")`, `Known("unknown")`, and `Known("")` are
ordinary known values and never encode authoritative unknown information.
Duplicate names in raw material are `INVALID`, even when their values are
identical. Typed maps therefore have unique keys.

## Closed rule-field vocabulary

`CompatibilityRule.field` MUST be exactly one of:

| Token | Context component | Domain |
|---|---|---|
| `architecture` | architecture | scalar |
| `abi` | ABI | scalar |
| `kernel` | kernel | scalar |
| `runtime` | runtime | scalar |
| `libc` | libc | scalar |
| `image` | image | scalar |
| `capabilities` | privilege/capabilities | set |
| `namespaceSecurity` | namespace/security | scalar |
| `configuration` | configuration | scalar |
| `environment` | environment variables | map |
| `featureFlags` | feature flags | map |
| `persistentState` | persistent state | set |
| `ruleVersion` | rule version | version |

Empty, unknown, case-mutated, whitespace-mutated, and aliased tokens are
`INVALID`. The expected operand must have the domain specified by the token.

## Predicate semantics

The rule field selects one dimension. `expected` is interpreted in that
dimension's domain. A predicate evaluates the candidate/source value against
the baseline/target value and expected operand as follows.

### Exact equality

`ExactEquality` is compatible iff both values are present, well formed, and
their semantic identities are equal, and the expected operand is equal to that
identity. A present unequal value is `INCOMPATIBLE`; unknown/missing is
`UNKNOWN`; malformed or wrong-domain input is `INVALID`.

### Scalar membership

`SET_MEMBER` is a constraint predicate and remains exactly scalar membership:
the candidate scalar value MUST belong to the rule-owned `members` set.
`SET_MEMBER` never compares candidate and baseline collections. A valid member
is `COMPATIBLE`, a valid non-member is `INCOMPATIBLE`, unknown/missing input is
`UNKNOWN`, and malformed or wrong-domain input is `INVALID`.

### Set containment

`SET_CONTAINS` is available only under `compatibility-rule.v2`. It is a
relational predicate with no rule-owned expected operand. Candidate is the
`SourceContext` and baseline is the `TargetContext`; both selected fields MUST
be typed sets, and the relation is `candidate[field] ⊆ baseline[field]`.
Known containment is `COMPATIBLE`; a known non-subset is `INCOMPATIBLE`;
missing/unknown authoritative operands are `UNKNOWN`; malformed or wrong-domain
operands, or a supplied expected operand, are `INVALID`. Set order is
irrelevant, duplicate raw members are invalid before typed construction, and
empty/empty and empty/non-empty relations are compatible.

### Map containment

`MAP_CONTAINS` is available only under `compatibility-rule.v2`. It is a
relational predicate with no rule-owned expected operand. Candidate is the
`SourceContext` and baseline is the `TargetContext`; both selected fields MUST
be typed maps, and every candidate `(key,value)` entry MUST occur identically
in baseline. Known containment is `COMPATIBLE`; missing keys or differing
known values are `INCOMPATIBLE`; missing/unknown authoritative operands are
`UNKNOWN`; malformed or wrong-domain operands, duplicate raw keys, or a
supplied expected operand are `INVALID`. Map order is irrelevant. Empty/empty
and empty/non-empty relations are compatible.

For typed-map equality, fully known maps are equal iff their semantic key sets
are identical and every corresponding value is equal; reordered entries are
equal, missing or extra keys are `INCOMPATIBLE`, and an unavailable map or
unknown entry is `UNKNOWN`. During containment, a known source value compared
with a known different target value is `INCOMPATIBLE`; a known source value
compared with an authoritative unknown target value is `UNKNOWN`; an
authoritative unknown source entry is `UNKNOWN` unless a deterministic
mismatch has already been established. Malformed entries are `INVALID`.

### Exact version

Versions use the closed grammar `0` or a dot-separated sequence of non-negative
decimal integers without leading zeroes except `0` itself. Comparison is
numeric component comparison after treating omitted trailing components as
zero. Extra text, prerelease/build syntax, or leading-zero components are
`INVALID`. Compatibility requires source and target versions to be equal and
equal to `expected`.

### Version range

`expected` uses the closed grammar `[lower,upper]`, `(lower,upper)`,
`[lower,upper)`, or `(lower,upper]`, where each endpoint is a valid exact
version and neither endpoint may be omitted. Lower MUST NOT exceed upper.
Compatibility requires the source version to be within the range and the
target version to be within the same range. Boundary inclusion follows the
brackets. Malformed or empty ranges are `INVALID`; missing/unknown versions
are `UNKNOWN`.

### Architecture and ABI

`ArchitectureABI` compares the ordered pair `(architecture, abi)` for exact
semantic equality between source and target. The expected operand is the
canonical pair `architecture/abi` using the same closed scalar domains.
Aliases such as `amd64` and `x86_64` are distinct unless represented by the
same authoritative identity. A mismatch is `INCOMPATIBLE`; missing/unknown is
`UNKNOWN`; malformed input is `INVALID`.

### Digest equality

`DigestEquality` compares algorithm identity and digest bytes. The expected
operand is `algorithm:lowercase-hex`; algorithm names are case-sensitive and
the hex length is determined by the algorithm. Malformed encodings are
`INVALID`; equal algorithm and bytes are `COMPATIBLE`; unequal values are
`INCOMPATIBLE`; missing/unknown is `UNKNOWN`.

## Result algebra and applicability

`COMPATIBLE` means valid bound operands exist and the selected predicate
holds. `INCOMPATIBLE` means valid bound operands exist and it does not hold.
`UNKNOWN` means valid authoritative information is unavailable, unknown, or
stale. `INVALID` means malformed grammar, undeclared predicate/field,
wrong-domain operand, broken binding, mixed attempt, or conflicting result.
Invalid input returns an error.

Predicate comparison is separate from temporal and revocation applicability.
Malformed/inverted validity is `INVALID`; before `notBefore` or after
`notAfter` is non-applicable and cannot contribute positive authority;
`REVOKED` is non-positive; missing, stale, or unknown revocation is `UNKNOWN`.
No ambient clock is used.

Multiple compatibility requirements are a conjunction. Every requirement must
produce `COMPATIBLE` for the relationship to contribute a positive
compatibility result. Any valid `INCOMPATIBLE` dominates; otherwise UNKNOWN
propagates. Input order is irrelevant.

## Conformance vectors

Conforming implementations MUST agree that equal scalar values are positive,
unequal scalar values are negative, source sets must be subsets of target
sets, exact versions use numeric component ordering, range brackets control
boundaries, architecture/ABI is an ordered pair, and digest equality includes
algorithm identity. Missing/unknown is UNKNOWN and malformed is INVALID.

## Attack and directional examples

| Case | Result |
|---|---|
| caller selects `COMPATIBLE` | rejected/INVALID |
| baseline or candidate merely exists | UNKNOWN |
| reverse candidate and baseline | separate relation; no substitution |
| wrong identity/rule/requirement/backend/scope/context | NON_MATCHING or INVALID |
| unknown field or predicate 99 | INVALID |
| malformed version/range/digest | INVALID |
| stale or revoked result | UNKNOWN or non-positive |
| reordered equivalent set | same result |
| duplicate set member | INVALID |
| diagnostic text claims compatibility | not proof |
| complete valid positive comparison | COMPATIBLE |
| complete valid failed comparison | INCOMPATIBLE |

Compatibility evidence is not a `CompatibilityFact`; a fact is not a
`RequirementMatch`; a match is not eligibility, authorization, or runtime
authority.

## Consequences

ADR-0019 may now bind `CompatibilityEvaluationResult` to this closed
predicate vocabulary and result algebra. P2.6-B may implement a derivation
over authoritative typed contexts rather than an assertion-style
`DeriveCompatible` function. P3 remains responsible for eligibility
aggregation.

This RFC preserves P0, P1, P2, P2.5, P2.6-A, ADR-0018, ADR-0019, RFC-0003,
and RFC-0004. It does not define Go packages, constructors, or implementation
libraries.

## Complete-context compatibility

Predicate evaluation is a dimension-level operation. Complete compatibility is
the conjunction of every dimension selected as mandatory by the bound
AuthorityRule. RFC-0003 defines the universe of dimensions; the AuthorityRule
selects the mandatory subset for the particular relationship. A selected
dimension is never optional during that relationship.

For mandatory dimensions the aggregation is:

```text
any INVALID       -> INVALID/error
else any INCOMPATIBLE -> INCOMPATIBLE
else any UNKNOWN  -> UNKNOWN
else all COMPATIBLE -> COMPATIBLE
```

An AuthorityRule with no compatibility requirement is `NOT_APPLICABLE` at the
rule layer and does not manufacture a `COMPATIBLE` compatibility fact. The
mandatory dimension set is derived, rather than stored in a second rule
field, by `SelectedCompatibilityDimensions(rule)`: it is the canonical set of
dimensions named by the rule's valid, exact compatibility requirements. A
present but empty or malformed compatibility-requirement collection is
`INVALID`. Exact duplicate requirements (including their complete semantic
identity) are deduplicated; distinct requirements selecting one dimension are
all evaluated; conflicting requirements with the same semantic identity are
`INVALID`; ordering is irrelevant. Thus the selected set is non-empty exactly
when at least one valid compatibility requirement exists.

The following is normative: architecture and ABI may be compatible while a
mandatory kernel dimension is UNKNOWN; the complete result is UNKNOWN. If a
mandatory kernel dimension is INCOMPATIBLE, the complete result is
INCOMPATIBLE even if all other dimensions are compatible.

`SelectedCompatibilityDimensions(rule)` is the canonical set of field
dimensions appearing in the bound rule's compatibility requirements. No
independent mandatory-dimensions field is introduced. If the rule has no
compatibility requirements, the result is `NOT_APPLICABLE`. If a compatibility
collection is present but empty, malformed, or contains an invalid
field/predicate pair, the result is `INVALID`. Exact duplicate requirements
are deduplicated; distinct requirements for one dimension are all evaluated
and conjoined before that dimension participates in complete aggregation.

## Field and predicate applicability

The complete field × predicate matrix is:

| Field | EQUAL | SET_MEMBER | SET_CONTAINS | MAP_CONTAINS | EXACT_VERSION | VERSION_RANGE | ARCH_ABI_RELATION | DIGEST_EQUAL |
|---|---|---|---|---|---|---|---|---|
| architecture | VALID | VALID | INVALID | INVALID | INVALID | INVALID | VALID | INVALID |
| abi | VALID | VALID | INVALID | INVALID | INVALID | INVALID | VALID | INVALID |
| kernel | VALID | VALID | INVALID | INVALID | VALID | VALID | INVALID | INVALID |
| runtime | VALID | VALID | INVALID | INVALID | VALID | VALID | INVALID | INVALID |
| libc | VALID | VALID | INVALID | INVALID | VALID | VALID | INVALID | INVALID |
| image | VALID | VALID | INVALID | INVALID | INVALID | INVALID | INVALID | VALID |
| capabilities | INVALID | INVALID | VALID | INVALID | INVALID | INVALID | INVALID | INVALID |
| namespaceSecurity | VALID | VALID | INVALID | INVALID | INVALID | INVALID | INVALID | INVALID |
| configuration | VALID | INVALID | INVALID | VALID | INVALID | INVALID | INVALID | VALID |
| environment | VALID | INVALID | INVALID | VALID | INVALID | INVALID | INVALID | INVALID |
| featureFlags | VALID | INVALID | INVALID | VALID | INVALID | INVALID | INVALID | INVALID |
| persistentState | INVALID | INVALID | VALID | INVALID | INVALID | INVALID | INVALID | INVALID |
| ruleVersion | INVALID | INVALID | INVALID | INVALID | VALID | VALID | INVALID | INVALID |

The field tokens `capabilities` and `environment` are the canonical aliases
for the context components named `privilegeCapabilities` and
`environmentVariables`. No other aliases exist. Every invalid cell returns
`INVALID/error` before predicate evaluation.

Scalar operands use exact canonical scalar equality. Architecture/ABI is an
ordered pair. Set operands are mathematical sets of typed members.
Configuration is a typed key/value map; environment is a typed key/value map;
feature flags are a typed flag/value map; persistent state is a typed set of
state identities. Map equality requires identical canonical key sets and
equal values for every key. Set equality ignores order after source-level
duplicate validation. Serialization order is never semantic.

## Closed domains and normalization

Architecture tokens are the closed set `amd64`, `arm64`, `386`, `ppc64le`,
and `s390x`. ABI tokens are the closed set `sysv`, `gnu`, `musl`, and
`eabi`. Unknown or unsupported tokens are `UNKNOWN` only when supplied by an
authoritative source as an explicit unknown value; empty, malformed, or
undeclared tokens are `INVALID`. `amd64` and `x86_64`, and `arm64` and
`aarch64`, are not interchangeable. If an ingestion layer accepts aliases,
it MUST normalize them before typed context construction.

The numeric version grammar is ASCII digits separated by dots, one or more
components, with no leading zero except the component `0`. Components are
unbounded mathematical non-negative integers. `1`, `1.0`, and `1.0.0` compare
equal; `1.0.1 > 1`; `1.9 < 1.10`. Lexical spelling remains distinct source
metadata. `01.2`, `1.02`, prerelease/build suffixes, `v1.2.3`, empty input,
whitespace, and Unicode digits are `INVALID`.

Version ranges use exactly `[a,b]`, `(a,b)`, `[a,b)`, or `(a,b]` with two valid
endpoints and no whitespace. Lower greater than upper is a valid syntactic
range with an empty mathematical interval and evaluates `INCOMPATIBLE`; bad
delimiters, missing endpoints, repeated separators, or invalid endpoints are
`INVALID`. `(a,a)` is a valid empty interval and is `INCOMPATIBLE`.

Digest algorithms are the closed set `sha256` and `sha512`, with lengths 32
and 64 bytes respectively. Presentation is `algorithm:lowercase-hex`.
Algorithm names are lowercase and case-sensitive; uppercase hex is accepted
only after required ingestion normalization and is not the canonical form.
Unknown algorithms, wrong lengths, odd-length hex, and non-hex characters are
`INVALID`. Equality requires both algorithm identity and digest bytes.

Normalization is REQUIRED before typed context construction for approved
architecture aliases and digest uppercase-to-lowercase ingestion. It is
FORBIDDEN during predicate evaluation for field tokens, versions, range
syntax, scalar values, Unicode, and whitespace. Set ordering is
SEMANTICALLY IRRELEVANT; duplicate set members are rejected during source
validation.

## Temporal, revocation, and identity bindings

Predicate truth is separate from applicability. A true predicate with
not-yet-valid or expired validity cannot contribute positive authority but is
not an incompatible comparison. Malformed validity is `INVALID`. A true
predicate with `REVOKED` is non-positive; UNKNOWN or stale revocation is
`UNKNOWN`; `NOT_REVOKED` permits applicability only after all identity,
freshness, provenance, and attempt checks succeed.

Compatibility result identity binds candidate, baseline, AuthorityRule, exact
requirement, backend, scope, context, field, predicate, expected operand,
ruleVersion, and ResolutionAttemptIdentity. Validity and revocation are
applicability bindings; provenance binds the source and producer. A change to
any identity operand prevents reuse of the result.

For `SET_CONTAINS` and `MAP_CONTAINS`, the selected schema identity is
`compatibility-rule.v2`; candidate and baseline identities are required, and
no expected operand participates in identity or evaluation. These predicates
are invalid under `compatibility-rule.v1`; unsupported schema/operator
combinations fail closed.

## Normative conformance vectors

The following vectors are literal requirements. `C`, `I`, `U`, and `X` mean
COMPATIBLE, INCOMPATIBLE, UNKNOWN, and INVALID respectively.

| Predicate/vector | Result |
|---|---|
| ExactEquality architecture `amd64`/`amd64`/`amd64` | C |
| ExactEquality architecture `amd64`/`arm64`/`amd64` | I |
| ExactEquality missing target | U |
| ExactEquality malformed field | X |
| SetMember architecture `amd64`/members `{amd64,arm64}` | C |
| SetMember architecture `ppc64le`/members `{amd64,arm64}` | I |
| SetMember unknown scalar | U |
| SetMember set operand | X |
| ExactVersion kernel `1.9`/`1.10`/`1.10` | I |
| ExactVersion kernel `1.0`/`1`/`1.0` | C |
| ExactVersion malformed `1.0-rc1` | X |
| VersionRange kernel `1.0` in `[1.0,2.0]` | C |
| VersionRange kernel `2.0` in `(1.0,2.0)` | I |
| VersionRange empty `(1.0,1.0)` | I |
| VersionRange malformed endpoint | X |
| ArchitectureABI `(amd64,sysv)` equal pair | C |
| ArchitectureABI `(amd64,sysv)` vs `(amd64,gnu)` | I |
| ArchitectureABI unknown ABI | U |
| DigestEquality equal sha256 algorithm and bytes | C |
| DigestEquality different algorithm | I |
| DigestEquality wrong digest length | X |
| Complete context with mandatory UNKNOWN kernel | U |
| Complete context with mandatory INCOMPATIBLE kernel | I |
| Complete context all mandatory dimensions C | C |

Structured-map vectors are also normative. Environment and feature-flag maps
use the same typed key/value rules:

| Vector | Result |
|---|---|
| environment ExactEquality `{MODE=production}` / `{MODE=production}` | C |
| environment ExactEquality `{MODE=production}` / `{MODE=development}` | I |
| environment ExactEquality `{MODE=production}` / `{MODE=production,REGION=eu}` | I |
| MapContains environment `{MODE=production}` / `{MODE=production,REGION=eu}` | C |
| MapContains environment `{MODE=production}` / `{MODE=development,REGION=eu}` | I |
| MapContains environment expected operand supplied | X |
| MapContains environment unknown target entry | U |
| MapContains environment malformed key | X |
| SetContains capabilities `{A}` / `{A,B}` | C |
| SetContains capabilities `{A,B}` / `{A}` | I |
| SetContains capabilities empty / empty | C |
| SetContains capabilities expected operand supplied | X |
| featureFlags ExactEquality `{FEATURE_X=true}` / `{FEATURE_X=true}` | C |
| featureFlags ExactEquality `{FEATURE_X=true}` / `{FEATURE_X=false}` | I |
| MapContains featureFlags `{FEATURE_X=true}` / `{FEATURE_X=true,MODE=production}` | C |
| MapContains featureFlags malformed value | X |
| reordered equivalent map entries | same result |
| duplicate source key | X |
| unknown authoritative map value | U |
| malformed map key or value | X |

Name/value-domain vectors are normative: `MODE`, `_MODE`, and `MODE_1` are
valid names; `1MODE`, `MODE-NAME`, names containing whitespace, and Unicode
names are X. Empty values and the literal `UNKNOWN` are known values; an
authoritative `Unknown` tag is U; invalid UTF-8 or embedded NUL is X.

Dimension-selection vectors are normative: no compatibility requirements is
`NOT_APPLICABLE`; one architecture requirement selects `{architecture}`;
architecture, kernel, and runtime requirements select
`{architecture,kernel,runtime}`; exact duplicate kernel requirements are
deduplicated; distinct kernel constraints are both evaluated; conflicting
same-identity kernel requirements are X; compatible architecture and runtime
with unknown mandatory kernel is U; incompatible kernel dominates unknown
runtime as I; any malformed requirement is X; and reordering equivalent
requirements leaves both the selected set and result unchanged.

These vectors are language-independent and do not depend on library behavior.
