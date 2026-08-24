# RFC-0004: Canonical Authority Object Schemas

Status: DRAFT

## Abstract

This RFC closes the canonical semantic boundary between ADR-0011/ADR-0012
resolved bytes and RFC-0003 evaluation. It defines versioned, closed schemas
for authority objects, strict decoding, canonical re-encoding, and reference
binding. It does not redefine eligibility, trust, coverage, adequacy,
authorization, or enforcement semantics owned by RFC-0003.

## 1. Authority and scope

RFC-0003 owns observation, evidence, coverage, completeness, adequacy, trust,
compatibility, authority, eligibility, revocation, and precedence. ADR-0011
owns canonicalization and digest binding; ADR-0012 owns exact resolution and
immutable snapshots. This RFC owns only object kinds, schema identifiers and
versions, field types/cardinality/presence, collection semantics, closed
enums and operand grammars, strict decoding, canonical semantic serialization,
and reference/content identity binding. Implementations may choose language,
storage, transport, and JSON libraries but may not change these semantics.

## 2. Common wire contract

Every object is UTF-8 RFC-8785 JCS JSON without a BOM. Authority numbers are
integers or decimal strings where a schema says so; floating point is
forbidden. Strings are byte-sensitive valid UTF-8, with no normalization,
case folding, trimming, or implicit conversion. NUL is forbidden in identity,
schema, enum, and version strings. Missing, `null`, empty, false, and UNKNOWN
are distinct; a schema states which are permitted.

Every object contains the required envelope members `schema`, `kind`, `id`,
and `version`; `schema` contains `id` and `version`. Digests are lowercase
`sha256:<64 hex>` strings. Object member names are unique. Unknown members,
unsupported schema versions, missing required members, wrong types, invalid
enums, and duplicate semantic set members are invalid. Arrays are either
ordered lists (order significant) or semantic sets (sorted by canonical bytes
and duplicate-rejected); each schema declares which.

## 3. Object envelope and references

The V1 envelope is exactly:

```json
{"schemaId":"...","schemaVersion":"...","kind":"...","id":"...","version":"...",...}
```

`kind` is one of the registered object kinds below. A reference is the closed
tuple `{kind,id,version,digest}`. No field may be omitted, wildcarded, or
resolved as latest. Kind participates in digest domain separation. The nested
member `schema` is not a V1 member: a V1 decoder MUST reject it as an unknown
field. There is no migration alias. Missing `schemaId` or `schemaVersion`, a
nested-only form, or a document containing both nested and top-level forms is
invalid.

## 4. Closed registries

The following V1 schemas are registered: `authority-rule.v1`,
`trust-policy.v1`, `security-field-registry.v1`,
`certification-property-registry.v1`, `architecture-abi-registry.v1`,
`backend-envelope-registry.v1`, `evidence.v1`, `coverage.v1`,
`completeness.v1`, `adequacy-evidence.v1`, `certification.v1`,
`verification-fact.v1`, `baseline.v1`, `compatibility-rule.v1`,
`composition-operator.v1`, `provenance.v1`, `verifier-semantic.v1`, and
`security-context.v1`. Each registry entry has a closed schema, ID, version,
and digest. Unknown entries are invalid, never extensions.

### 4.1 SecurityFieldRegistry

An entry is `{field,type,cardinality,presence,unknown,ordering,comparison,
identitySignificance}`. `type` is a closed scalar/enum/reference/scope type;
cardinality is `required|optional|repeated`; presence and UNKNOWN behavior are
explicit registry values; ordering is `set|ordered`; comparison and identity
significance are closed enums.

### 4.2 CertificationPropertyRegistry

V1 registers exactly `SCOPE_COVERAGE`, `BASELINE_COMPATIBILITY`,
`POLICY_ADEQUACY_BOUNDED`, and `PROVENANCE_VALIDITY`. Each entry binds backend
classes, certified scope form, context requirements, verifier requirements,
validity/revocation requirements, and the precise statement certified. A
property never certifies a wider scope or eligibility.

### 4.3 ArchitectureABIRelationRegistry

V1 relations are directed entries `{sourceArchitecture,sourceABI,
targetArchitecture,targetABI,result}` with result `EQUIVALENT|INCOMPATIBLE`.
No ranges, callbacks, or reversed-direction inference exist. Unknown relation
is unsupported.

### 4.4 BackendEnvelopeRegistry

The Seccomp container-wide deny-by-default V1 envelope is registered with the
exact non-waivable dimensions `STARTUP_BOOTSTRAP`, `CONTAINER_LIFETIME`,
`PROCESS_TREE`, `EXECUTABLE_SET`, `WORKLOAD_STATE`, `ARCHITECTURE_ABI`,
`IMAGE_IDENTITY`, and `KERNEL_RUNTIME_COMPATIBILITY`. An envelope may add
strengthening dimensions but may not omit, downgrade, or rename these. PodLock,
NetworkPolicy, and other envelopes remain undefined for authority until a
future registered schema exists; they cannot be guessed from this RFC.

## 5. AuthorityRule V1

`AuthorityRule` contains: issuer, backend, envelope reference, target `Scope`,
mandatory coverage dimensions, accepted evidence and completeness class sets,
adequacy requirements, baseline reference, compatibility reference, optional
trust-policy reference, provenance requirements, certification-property set,
validity interval, revocation reference, registry-reference set, and
composition reference. Each is required or optional exactly as declared by
the schema; sets are canonical sets. There are no extension members or
free-form expressions. A Seccomp container rule is invalid unless its envelope
reference resolves to the registered eight-dimension envelope. Additional
requirements may only strengthen it.

## 6. TrustPolicy V1

`TrustPolicy` contains an ordered-by-canonical-key set of `AuthorizationEntry`
objects. Each entry binds issuer, artifact/evidence class, backend, allowed
property/class identifiers, scope constraints, context constraints, validity,
and revocation reference. Duplicate identical entries are rejected; same-key
conflicting entries are invalid. No matching entry is an explicit closed-world
absence. TrustPolicy contains no root anchor or root selector; authenticity is
established only against external `RootTrustConfiguration`.

## 7. Evidence, coverage, completeness, and adequacy

`EvidenceObject` binds evidence reference, class, backend, observation and
target scopes, observation identity, SecurityContext, CoverageRecord set,
provenance reference, validity, revocation reference, payload digest, and
source identity. It has no terminal authority fields.

`CoverageRecord` binds evidence set, evidence scope, target scope, dimension
results, method, assumptions, context, provenance, and class
`MEASURED|ASSUMED|EXTERNALLY_CERTIFIED|UNKNOWN`. Dimension results are typed
`COVERS|DOES_NOT_COVER|UNKNOWN`; a measured finite trace never encodes
universal lifetime coverage.

`CompletenessRecord` has one class:
`EMPIRICAL_COMPLETENESS|STRUCTURAL_COMPLETENESS|DECLARED_COMPLETENESS|
EXTERNALLY_CERTIFIED_COMPLETENESS`, plus class-required scope, issuer/model,
assumptions, verifier/certification, validity, revocation, and provenance.
Class names alone are not positive conclusions.

`AdequacyEvidence` is a tagged union of `STRUCTURAL_BASELINE`,
`EXTERNAL_CERTIFICATION`, `BACKEND_FORMAL_INVARIANT`, `BOUNDED_BEHAVIORAL`,
and `TRUSTED_BASELINE_OBSERVED_DELTA`; each variant requires its exact issuer,
backend, scope, context, verifier/certification, validity, revocation, and
provenance bindings. It never contains `ADEQUATE`.

## 8. Certification and verification

`Certification` binds ID/version/digest, issuer, registered property, backend,
scope, context, assumptions, validity, revocation, provenance, verifier
identity, and verification-fact reference. Its property registry limits its
statement and scope.

`VerificationFact` binds exact `VerifierSemanticIdentity`, property,
candidate/subject, backend, scope, context, input identity/digest, result
`VERIFIED|FAILED|UNKNOWN`, provenance, validity/freshness, and revocation.
Duplicate identical facts may be deduplicated; same identity with differing
content is a conflict.

`VerifierSemanticIdentity` is the existing ADR-0012 object with ID, version,
digest, class, input/output schemas, recognized property, procedure/protocol,
and security constraints. Endpoint or executable names never substitute for
this identity.

## 9. Baseline, compatibility, composition, and provenance

`Baseline` binds its reference, owner/source, backend, scope, SecurityContext,
architecture/ABI and kernel/runtime assumptions, image/workload assumptions,
CompatibilityRule reference, validity, revocation, provenance, and payload
identity. Existence does not imply applicability.

`CompatibilityRule` is a tagged union with typed operands for `EQUAL`,
`SET_MEMBER`, `EXACT_VERSION`, `VERSION_RANGE`, `ARCH_ABI_RELATION`, and
`DIGEST_EQUAL`. V1 version ranges use four-part numeric versions and explicit
inclusive lower and upper bounds; an omitted bound is invalid. ARCH_ABI rules
reference an exact ArchitectureABIRelationRegistry entry. Unknown operators
and arbitrary expressions are invalid.

`CompositionOperator` binds its reference, supported input schemas/classes,
ordered operation list, envelope/provenance preservation requirements,
validity, and revocation. Operations are exactly `REQUIRE_EQUAL`,
`SET_UNION_IF_IDENTICAL_ACTION`, `SET_INTERSECTION`, `REJECT_ON_CONFLICT`, or
`PRESERVE_BASELINE`; no arbitrary operation exists.

`ProvenanceRecord` binds provenance ID, producer/source, mechanism and version,
run/recording identity, backend, scope, context, validity, revocation, and
verifier/certification binding. Provenance records are a canonical set, not an
implicit history or trust decision.

## 10. SecurityContextIdentity

The V1 context schema contains image identity, architecture, ABI,
kernel/runtime class, libc, privilege/capability class, namespace/security
state, configuration, environment, feature flags, persistent-state identity,
workload identity, and executable identity. Pod UID, resourceVersion,
container ID, timestamps, and node IP are excluded from semantic context.

## 11. Strict decoding and digest binding

Decoding is deterministic: validate UTF-8; parse JSON with duplicate-key
detection; validate schema and kind; reject unsupported versions and unknown
members; validate required fields, types, cardinality, enums, and object
invariants; canonicalize with JCS; recompute the object-kind-separated
SHA-256 digest; compare it with the expected reference; then return the typed
immutable object. Any failure returns no object.

Semantic digest input is the exact domain-label, one-zero-octet, and canonical
JSON construction defined in section 29. The kind-specific domain prevents
cross-kind substitution. Reference binding requires exact equality of kind,
ID, version, and recomputed digest.

## 12. Evolution and vectors

V1 is closed. A security-semantic change requires a new schema version and
digest. V1 implementations never downgrade or ignore V2 fields. Golden
vectors for every object kind contain source semantics, exact canonical bytes,
reference, and digest. Invalid vectors cover unknown/duplicate/missing fields,
bad enums, wrong schema/version/kind/digest, unsupported registry entries,
scope errors, rule weakening, invalid compatibility operands, and invalid
composition operations. Go and Rust implementations must produce identical
vectors.

## 13. Adversarial closure

The following are invalid with one deterministic outcome: omitted Seccomp
non-waivable dimension; binary scope substituted for container lifetime;
unknown or duplicate field; TrustPolicy root selection; unknown certification
property; reversed architecture relation; inclusive/exclusive range dispute;
wrong operand type; reordered operations where order is declared; unknown
composition; terminal `complete`/`adequate`/`eligible` fields; wider
certification scope; missing context or verifier identity; duplicate JSON
member; V2-as-V1 decoding; Unicode lookalike identity; and collapsed
ABSENT/EMPTY/UNKNOWN. No case is implementation-defined.

## 14. Invariant preservation

This RFC preserves RFC-0003 observation-versus-authority, coverage-versus-
completeness, approval-versus-eligibility, UNKNOWN fail-closed, non-waivable
envelope, root/trust separation, empirical-trace limits, revocation, exact
reference, no-latest/fallback, and closed-registry invariants. It introduces
`TYPED-RESOLVED-SEMANTICS`, `STRICT-SCHEMA-DECODE`,
`REFERENCE-CONTENT-BINDING`, `RULE-NON-WEAKENING`, `TRUST-ROOT-SEPARATION`,
`CLOSED-RULE-SEMANTICS`, `CLOSED-COMPATIBILITY-SEMANTICS`,
`CLOSED-COMPOSITION-SEMANTICS`, `REGISTRY-CLOSED-WORLD`,
`OPAQUE-DIVERGENCE-BLOCKED`, and `IMMUTABLE-RESOLVED-OBJECTS`.

## 15. Boundary

RFC-0004 does not implement P3 evaluation, persistence, authorization,
backend adapters, or enforcement. It defines the only valid interpretation of
resolved semantic bytes that those later layers may consume.

## 16. Normative V1 field-table convention

The following tables are normative. Columns are: `FIELD`, `TYPE`,
`CARDINALITY`, `REQUIRED`, `ABSENT`, `EMPTY`, `UNKNOWN`, `ORDERING`, and
`IDENTITY`. `forbidden` means the member MUST NOT occur. `identity` means a
change changes the semantic digest. Type names are closed vocabulary:
`string`, `boolean`, `integer`, `timestamp`, `digest`, `reference<K>`,
`enum<E>`, `object<O>`, `set<T>`, and `ordered-list<T>`.

Common envelope (all fields are required, scalar, and identity-significant):

| FIELD | TYPE | CARDINALITY | REQUIRED | ABSENT | EMPTY | UNKNOWN | ORDERING | IDENTITY |
|---|---|---:|---|---|---|---|---|---|
| schemaId | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| schemaVersion | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| kind | enum<ObjectKind> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| id | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| version | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |

`digest` is external reference material and is not embedded in the canonical
object envelope. It is recomputed from the canonical object and compared with
the external reference. `schemaId` and `schemaVersion` are top-level wire
members exactly as shown above; there is no nested schema object.

## 17. AuthorityRule V1 field table

| FIELD | TYPE | CARDINALITY | REQUIRED | ABSENT | EMPTY | UNKNOWN | ORDERING | IDENTITY |
|---|---|---:|---|---|---|---|---|---|
| issuer | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| backend | enum<BackendId> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| envelopeRef | reference<BackendEnvelope> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| targetScope | object<Scope> | 1 | yes | invalid | forbidden | forbidden | object | yes |
| mandatoryCoverageDimensions | set<enum<ScopeDimension>> | 1 | yes | invalid | empty invalid | forbidden | canonical set | yes |
| acceptedEvidenceClasses | set<enum<EvidenceClass>> | 1 | yes | invalid | allowed | forbidden | canonical set | yes |
| acceptedCompletenessClasses | set<enum<CompletenessClass>> | 1 | yes | invalid | allowed | forbidden | canonical set | yes |
| adequacyRequirements | set<enum<AdequacyClass>> | 1 | yes | invalid | allowed | forbidden | canonical set | yes |
| baselineRef | reference<Baseline> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| compatibilityRuleRef | reference<CompatibilityRule> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| trustPolicyRef | reference<TrustPolicy> | 0..1 | no | no value | forbidden | forbidden | scalar | yes |
| provenanceRequirements | set<enum<ProvenanceRequirement>> | 1 | yes | invalid | allowed | forbidden | canonical set | yes |
| certificationProperties | set<enum<CertificationProperty>> | 1 | yes | invalid | allowed | forbidden | canonical set | yes |
| validity | object<Validity> | 1 | yes | invalid | forbidden | forbidden | object | yes |
| revocationRef | reference<Revocation> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| registryRefs | set<reference<Registry>> | 1 | yes | invalid | empty invalid | forbidden | canonical set | yes |
| compositionRef | reference<CompositionOperator> | 0..1 | no | no value | forbidden | forbidden | scalar | yes |

For Seccomp container targets, `mandatoryCoverageDimensions` MUST contain all
eight registered non-waivable dimensions. Omission, replacement, or UNKNOWN
is invalid. Additional dimensions are permitted only when registered.

## 17A. Revocation V1 semantic object

`Revocation` is an immutable identity object describing the revocation source
or revocation contract to which current revocation facts are bound. It does
not contain a current revocation result, freshness result, authorization
state, eligibility state, or runtime state.

Its common envelope is the top-level V1 envelope above, with
`kind:"REVOCATION"`. The V1 object-specific fields are:

| FIELD | TYPE | CARDINALITY | REQUIRED | ABSENT | EMPTY | UNKNOWN | ORDERING | IDENTITY |
|---|---|---:|---|---|---|---|---|---|
| sourceId | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| contractVersion | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| subjectKind | enum<ObjectKind> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| mechanism | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |

`sourceId` identifies the immutable producer/source, `contractVersion` is the
source contract version, `subjectKind` identifies the class of subject whose
current facts may be issued under the contract, and `mechanism` identifies
the revocation protocol. No field in this object represents
`REVOKED`, `NOT_REVOKED`, or `UNKNOWN`. Unknown members, duplicate members,
unsupported schema/version, missing required members, and invalid enum values
are invalid.

`reference<Revocation>` is an ordinary V1 `Reference` with
`kind:"REVOCATION"`; its `id`, `version`, and external digest bind exactly to
the canonical Revocation object. `SECURITY_FIELD_REGISTRY` and every other
ObjectKind are invalid for this reference. Current revocation facts remain
separate ADR-0012 facts and are never inferred from Revocation object
existence.

## 18. TrustPolicy and AuthorizationEntry V1

TrustPolicy fields:

| FIELD | TYPE | CARDINALITY | REQUIRED | ABSENT | EMPTY | UNKNOWN | ORDERING | IDENTITY |
|---|---|---:|---|---|---|---|---|---|
| entries | set<object<AuthorizationEntry>> | 1 | yes | invalid | allowed | forbidden | canonical set | yes |
| provenanceRef | reference<Provenance> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| validity | object<Validity> | 1 | yes | invalid | forbidden | forbidden | object | yes |
| revocationRef | reference<Revocation> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |

AuthorizationEntry fields:

| FIELD | TYPE | CARDINALITY | REQUIRED | ABSENT | EMPTY | UNKNOWN | ORDERING | IDENTITY |
|---|---|---:|---|---|---|---|---|---|
| issuer | string | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| artifactClass | enum<ArtifactClass> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| backend | enum<BackendId> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |
| allowedPropertyIds | set<string> | 1 | yes | invalid | empty invalid | forbidden | canonical set | yes |
| allowedClassIds | set<string> | 1 | yes | invalid | empty invalid | forbidden | canonical set | yes |
| scopeConstraint | object<Scope> | 1 | yes | invalid | forbidden | forbidden | object | yes |
| contextConstraint | object<SecurityContextIdentity> | 1 | yes | invalid | forbidden | forbidden | object | yes |
| validity | object<Validity> | 1 | yes | invalid | forbidden | forbidden | object | yes |
| revocationRef | reference<Revocation> | 1 | yes | invalid | forbidden | forbidden | scalar | yes |

Entry identity is the tuple issuer, artifactClass, backend, allowed-property
set, allowed-class set, scope, and context. Identical duplicates are invalid;
same identity with different validity/revocation is a conflict and invalid.
Overlapping non-identical entries remain separate set members.

## 19. Registry field tables

All registry objects have `entries` as a required canonical set and
`provenanceRef`, `validity`, and `revocationRef` as required fields.

SecurityFieldEntry:

| FIELD | TYPE | REQUIRED | EMPTY | UNKNOWN | ORDERING | IDENTITY |
|---|---|---|---|---|---|---|
| fieldId | string | yes | forbidden | forbidden | scalar | yes |
| valueType | enum<FieldType> | yes | forbidden | forbidden | scalar | yes |
| cardinality | enum<Cardinality> | yes | forbidden | forbidden | scalar | yes |
| presenceSemantics | enum<PresenceSemantics> | yes | forbidden | forbidden | scalar | yes |
| unknownSemantics | enum<UnknownSemantics> | yes | forbidden | forbidden | scalar | yes |
| orderingSemantics | enum<OrderingSemantics> | yes | forbidden | forbidden | scalar | yes |
| comparisonMode | enum<ComparisonMode> | yes | forbidden | forbidden | scalar | yes |
| identitySignificant | boolean | yes | forbidden | forbidden | scalar | yes |

CertificationPropertyEntry:

| FIELD | TYPE | REQUIRED | EMPTY | UNKNOWN | ORDERING | IDENTITY |
|---|---|---|---|---|---|---|
| propertyId | enum<CertificationProperty> | yes | forbidden | forbidden | scalar | yes |
| backendSet | set<enum<BackendId>> | yes | allowed | forbidden | canonical set | yes |
| scopeForm | enum<ScopeForm> | yes | forbidden | forbidden | scalar | yes |
| contextRequirement | enum<ContextRequirement> | yes | forbidden | forbidden | scalar | yes |
| verifierRequirement | enum<VerifierRequirement> | yes | forbidden | forbidden | scalar | yes |
| validityRequirement | enum<ValidityRequirement> | yes | forbidden | forbidden | scalar | yes |
| revocationRequirement | enum<RevocationRequirement> | yes | forbidden | forbidden | scalar | yes |
| certifiedStatement | string | yes | forbidden | forbidden | scalar | yes |
| nonClaims | set<string> | yes | allowed | forbidden | canonical set | no |

ArchitectureABIRelationEntry fields are `relationId:string`,
`sourceArchitecture:string`, `sourceABI:string`, `targetArchitecture:string`,
`targetABI:string`, `relationClass:enum<RelationClass>`, and
`direction:enum<Direction>`; all are required, non-empty, scalar, and
identity-significant. V1 relation class is `EQUIVALENT|INCOMPATIBLE` and
direction is `SOURCE_TO_TARGET` only.

BackendEnvelopeEntry fields are `backendId:enum<BackendId>`,
`envelopeVersion:string`, `attachmentModel:enum<AttachmentModel>`,
`targetScopeClass:enum<ScopeForm>`,
`mandatoryDimensions:set<enum<ScopeDimension>>`,
`strengtheningAllowed:boolean`, and
`nonWaivableDimensions:set<enum<ScopeDimension>>`; all are required and
identity-significant. The Seccomp V1 entry has exactly the eight dimensions
listed in section 17 in both dimension sets.

## 20. Evidence and Coverage V1 field tables

EvidenceObject:

| FIELD | TYPE | REQUIRED | ABSENT | EMPTY | UNKNOWN | ORDERING | IDENTITY |
|---|---|---|---|---|---|---|---|
| observationIdentity | string | yes | invalid | forbidden | forbidden | scalar | yes |
| evidenceClass | enum<EvidenceClass> | yes | invalid | forbidden | forbidden | scalar | yes |
| backend | enum<BackendId> | yes | invalid | forbidden | forbidden | scalar | yes |
| observationScope | object<Scope> | yes | invalid | forbidden | forbidden | object | yes |
| targetScope | object<Scope> | no | no value | forbidden | forbidden | object | yes |
| context | object<SecurityContextIdentity> | yes | invalid | forbidden | forbidden | object | yes |
| coverage | set<object<CoverageRecord>> | yes | invalid | empty invalid | forbidden | canonical set | yes |
| provenanceRef | reference<Provenance> | yes | invalid | forbidden | forbidden | scalar | yes |
| validity | object<Validity> | yes | invalid | forbidden | forbidden | object | yes |
| revocationRef | reference<Revocation> | yes | invalid | forbidden | forbidden | scalar | yes |
| payloadDigest | digest | yes | invalid | forbidden | forbidden | scalar | yes |
| sourceIdentity | string | yes | invalid | forbidden | forbidden | scalar | yes |

CoverageRecord fields are `evidenceRefs:set<reference<Evidence>>` (required,
non-empty), `evidenceScope:object<Scope>` (required),
`targetScope:object<Scope>` (required),
`dimensionResults:map<ScopeDimension,enum<CoverageDimensionResult>>` (required,
closed keys), `method:string` (required), `assumptions:set<string>` (required,
empty allowed), `resultClass:enum<CoverageClass>` (required),
`context:object<SecurityContextIdentity>` (required), and
`provenanceRef:reference<Provenance>` (required). Every mandatory target
dimension must be present in `dimensionResults`; unknown keys are invalid.

## 21. Completeness and adequacy variant tables

Common CompletenessRecord fields are `class:enum<CompletenessClass>`,
`scope:object<Scope>`, `validity:object<Validity>`,
`revocationRef:reference<Revocation>`, and
`provenanceRef:reference<Provenance>`; all are required. The variant payload is
closed:

| CLASS | REQUIRED PAYLOAD | FORBIDDEN PAYLOAD |
|---|---|---|
| EMPIRICAL_COMPLETENESS | method:string, runRefs:set<string>, observationRefs:set<reference<Evidence>> | certificationRef, structuralModel |
| STRUCTURAL_COMPLETENESS | modelId:string, verifierRef:reference<Verifier> | runRefs, certificationRef |
| DECLARED_COMPLETENESS | issuer:string, assumptions:set<string> | runRefs, certificationRef |
| EXTERNALLY_CERTIFIED_COMPLETENESS | certificationRef:reference<Certification>, verifierRef:reference<Verifier> | declared-only issuer |

Common AdequacyEvidence fields are `class:enum<AdequacyClass>`,
`identity:string`, `issuer:string`, `backend:enum<BackendId>`,
`scope:object<Scope>`, `context:object<SecurityContextIdentity>`,
`validity:object<Validity>`, `revocationRef:reference<Revocation>`,
`provenanceRef:reference<Provenance>`, and `verificationRef:reference<VerificationFact>`;
all are required. Variants require exactly one additional binding:
StructuralBaseline uses `baselineRef`; ExternalCertification uses
`certificationRef`; BackendFormalInvariant uses `invariantId`; BoundedBehavioral
uses `probeSetRef`; TrustedBaselineObservedDelta uses `baselineRef` and
`observationRefs`. Other variant payload fields are forbidden.

## 22. Certification and VerificationFact tables

Certification fields are `id:string`, `version:string`, `issuer:string`,
`propertyId:enum<CertificationProperty>`, `backend:enum<BackendId>`,
`scope:object<Scope>`, `context:object<SecurityContextIdentity>`,
`assumptions:set<string>` (required, empty allowed),
`validity:object<Validity>`, `revocationRef:reference<Revocation>`,
`provenanceRef:reference<Provenance>`, `verifierRef:reference<Verifier>`, and
`verificationFactRef:reference<VerificationFact>`; all except assumptions are
required and non-empty. Assumptions are a semantic set.

VerificationFact fields are `id:string`, `version:string`,
`verifierRef:reference<Verifier>`, `propertyId:enum<CertificationProperty>`,
`subjectRef:reference<Subject>`, `backend:enum<BackendId>`,
`scope:object<Scope>`, `context:object<SecurityContextIdentity>`,
`inputDigest:digest`, `result:enum<VerificationResult>` with exactly
`VERIFIED|FAILED|UNKNOWN`, `provenanceRef:reference<Provenance>`,
`observedAt:timestamp`, `validUntil:timestamp`, and
`revocationRef:reference<Revocation>`; all are required. Identity is
verifier/property/subject/backend/scope/context/inputDigest; conflicting
content for one identity is invalid.

## 23. Baseline, compatibility, and composition tables

Baseline fields are `owner:string`, `backend:enum<BackendId>`,
`scope:object<Scope>`, `context:object<SecurityContextIdentity>`,
`architectureAssumptions:set<string>`, `abiAssumptions:set<string>`,
`kernelRuntimeAssumptions:set<string>`, `imageWorkloadAssumptions:set<string>`,
`compatibilityRuleRef:reference<CompatibilityRule>`,
`validity:object<Validity>`, `revocationRef:reference<Revocation>`,
`provenanceRef:reference<Provenance>`, and `payloadDigest:digest`; all are
required, sets may be empty, and none is a terminal applicability boolean.

CompatibilityRule has `operator:enum<CompatibilityOperator>` and exactly one
variant payload. `EQUAL` and `DIGEST_EQUAL` use two typed `string`/`digest`
operands; `SET_MEMBER` uses `value:string` and `members:set<string>`;
`EXACT_VERSION` uses `version:string`; `VERSION_RANGE` uses
`lower:string`, `upper:string`, `lowerInclusive:boolean`,
`upperInclusive:boolean` (both bounds required; lower must not exceed upper);
`ARCH_ABI_RELATION` uses `relationRegistryRef:reference<Registry>` and
`relationId:string`. Unknown operators or wrong operands are invalid.

CompositionOperator fields are `supportedInputSchemas:set<string>`,
`supportedConflictClasses:set<string>`, `operations:ordered-list<Operation>`,
`envelopePreservation:enum<PreservationRequirement>`,
`provenancePreservation:enum<PreservationRequirement>`,
`validity:object<Validity>`, and `revocationRef:reference<Revocation>`; all
are required. Operation has `operation:enum<CompositionOperation>` and
`inputSelectors:ordered-list<string>`; operation order is identity-significant.

## 24. Provenance, verifier, context, validity, and revocation tables

ProvenanceRecord fields are `producer:string`, `mechanism:string`,
`mechanismVersion:string`, `runIdentity:string`, `backend:enum<BackendId>`,
`scope:object<Scope>`, `context:object<SecurityContextIdentity>`,
`validity:object<Validity>`, `revocationRef:reference<Revocation>`,
`verifierRef:reference<Verifier>` (optional, ABSENT means no verifier used),
and `certificationRef:reference<Certification>` (optional, ABSENT means no
certification used). Optional references are forbidden when empty or UNKNOWN.

VerifierSemantic fields are `verifierClass:string`, `inputSchema:string`,
`outputSchema:string`, `recognizedPropertyId:enum<CertificationProperty>`,
`procedureProtocol:string`, and `securityConstraints:set<string>`; all are
required, with constraints allowed empty.

SecurityContextIdentity fields are all required strings except
`capabilitySet:set<string>` and `featureFlags:set<string>`, which are required
and may be empty: `imageIdentity`, `architecture`, `abi`, `kernelClass`,
`runtimeClass`, `libcIdentity`, `privilegeClass`,
`namespaceSecurityState`, `configurationIdentity`, `environmentIdentity`,
`persistentStateIdentity`, `workloadIdentity`, and `executableIdentity`.
Diagnostic runtime identifiers are forbidden.

Validity is `{notBefore:timestamp,notAfter:timestamp}` with both required,
inclusive boundaries, and `notBefore <= notAfter`. Revocation is a reference
object, not a current result; current facts separately use the closed enum
`REVOKED|NOT_REVOKED|UNKNOWN`. Missing required revocation is invalid; missing
current fact is UNKNOWN, never NOT_REVOKED.

## 25. Presence, collections, and enum registry

Wire presence is either a member, an absent optional member, or an explicit
typed UNKNOWN value where the field table permits it. EMPTY is an empty
collection/string only where explicitly allowed. FALSE is a JSON boolean and
is never absence. INVALID is a decoder outcome and is not serialized.

Sets are sorted by canonical element bytes, reject duplicate semantic
identity, and preserve EMPTY distinctly from ABSENT. Ordered lists preserve
input order and reorder changes identity. Maps have closed key enums,
unique keys, and JCS key ordering. UNKNOWN elements are forbidden in identity
sets unless the field table explicitly permits them.

The V1 enum registry is closed to the following exact values. ObjectKind is
`AuthorityRule|TrustPolicy|SecurityFieldRegistry|CertificationPropertyRegistry|
ArchitectureABIRelationRegistry|BackendEnvelopeRegistry|Evidence|Coverage|
Completeness|AdequacyEvidence|Certification|VerificationFact|Baseline|
CompatibilityRule|CompositionOperator|Provenance|VerifierSemantic|SecurityContext`.
The closed ObjectKind set also includes `Revocation`.
BackendId is `SECCOMP` in V1. ScopeDimension values are exactly those in
section 17. CoverageClass and
CoverageDimensionResult values in section 7; CompletenessClass and
AdequacyClass values in sections 21; CertificationProperty values in section
4.2; VerificationResult `VERIFIED|FAILED|UNKNOWN`; CompatibilityOperator and
CompositionOperation values in section 23; and all remaining supporting enum
values exactly as enumerated in section 27. Unregistered enum strings are
invalid.

## 26. Closure checks

Every table above gives one canonical name, type, cardinality, required/
optional/forbidden status, presence behavior, ordering, and identity result.
Security fields are identity-significant; diagnostic fields are forbidden.
The same canonical bytes therefore admit one schema, one typed interpretation,
and one digest in Go, Rust, or any other conforming implementation.

## 27. Complete V1 enum registry

The following is the complete serialized V1 enum registry. Tokens are
case-sensitive ASCII and have no aliases. Any other token is invalid.

| ENUM | EXACT VALUES |
|---|---|
| ObjectKind | `AUTHORITY_RULE`, `TRUST_POLICY`, `SECURITY_FIELD_REGISTRY`, `CERTIFICATION_PROPERTY_REGISTRY`, `ARCHITECTURE_ABI_REGISTRY`, `BACKEND_ENVELOPE_REGISTRY`, `EVIDENCE`, `COVERAGE`, `COMPLETENESS`, `ADEQUACY_EVIDENCE`, `CERTIFICATION`, `VERIFICATION_FACT`, `BASELINE`, `COMPATIBILITY_RULE`, `COMPOSITION_OPERATOR`, `PROVENANCE`, `VERIFIER_SEMANTIC`, `SECURITY_CONTEXT`, `REVOCATION` |
| BackendId | `SECCOMP` |
| ScopeDimension | `TEMPORAL`, `STARTUP_LIFECYCLE`, `PROCESS_TREE`, `EXECUTABLE_SET`, `WORKLOAD_CONTAINER`, `WORKLOAD_STATE`, `RUN_HISTORY`, `ARCHITECTURE_ABI`, `IMAGE_CONTEXT` |
| ScopeCoverageState | `COVERS`, `DOES_NOT_COVER`, `UNKNOWN` |
| CoverageClass | `MEASURED`, `ASSUMED`, `EXTERNALLY_CERTIFIED`, `UNKNOWN` |
| CompletenessClass | `EMPIRICAL_COMPLETENESS`, `STRUCTURAL_COMPLETENESS`, `DECLARED_COMPLETENESS`, `EXTERNALLY_CERTIFIED_COMPLETENESS` |
| AdequacyClass | `STRUCTURAL_BASELINE`, `EXTERNAL_CERTIFICATION`, `BACKEND_FORMAL_INVARIANT`, `BOUNDED_BEHAVIORAL`, `TRUSTED_BASELINE_OBSERVED_DELTA` |
| CertificationProperty | `SCOPE_COVERAGE`, `BASELINE_COMPATIBILITY`, `POLICY_ADEQUACY_BOUNDED`, `PROVENANCE_VALIDITY` |
| VerificationResult | `VERIFIED`, `FAILED`, `UNKNOWN` |
| CompatibilityOperator | `EQUAL`, `SET_MEMBER`, `EXACT_VERSION`, `VERSION_RANGE`, `ARCH_ABI_RELATION`, `DIGEST_EQUAL` |
| CompositionOperation | `REQUIRE_EQUAL`, `SET_UNION_IF_IDENTICAL_ACTION`, `SET_INTERSECTION`, `REJECT_ON_CONFLICT`, `PRESERVE_BASELINE` |
| RevocationStatus | `REVOKED`, `NOT_REVOKED`, `UNKNOWN` |
| PresenceSemantics | `REQUIRED`, `OPTIONAL`, `FORBIDDEN` |
| UnknownSemantics | `FORBIDDEN`, `EXPLICIT_UNKNOWN` |
| Cardinality | `EXACTLY_ONE`, `ZERO_OR_ONE`, `ONE_OR_MORE`, `ZERO_OR_MORE` |
| OrderingSemantics | `SET`, `ORDERED_LIST`, `CLOSED_MAP` |
| ComparisonMode | `EXACT`, `SET_MEMBERSHIP`, `VERSION_TUPLE`, `ARCH_ABI_REGISTRY`, `DIGEST_EQUAL` |
| ScopeForm | `BINARY`, `PROCESS`, `PROCESS_TREE`, `CONTAINER`, `WORKLOAD`, `CONTAINER_LIFETIME` |
| ArtifactClass | `AUTHORITY_RULE`, `TRUST_POLICY`, `REGISTRY`, `EVIDENCE`, `CERTIFICATION`, `BASELINE`, `VERIFIER`, `COMPOSITION_OPERATOR`, `PROVENANCE` |
| EvidenceClass | `OBSERVATION`, `COVERAGE_RECORD`, `COMPLETENESS_RECORD`, `CERTIFICATION_RECORD`, `VERIFICATION_RECORD`, `PROVENANCE_RECORD`, `BACKEND_REALIZATION` |
| ProvenanceRequirement | `SOURCE_IDENTITY`, `MECHANISM_IDENTITY`, `RUN_IDENTITY`, `SCOPE_BOUND`, `CONTEXT_BOUND`, `VERIFIER_BOUND`, `CERTIFICATION_BOUND`, `CURRENT_VALIDITY` |
| RelationClass | `EQUIVALENT`, `INCOMPATIBLE` |
| Direction | `SOURCE_TO_TARGET` |
| AttachmentModel | `SYSCALL_FILTER` |
| PreservationRequirement | `REQUIRED`, `NOT_REQUIRED` |
| ArtifactClassTrust | `OBSERVATION`, `AUTHORITY_OBJECT`, `CERTIFICATION`, `VERIFICATION` |
| ContextRequirement | `REQUIRED`, `NOT_REQUIRED` |
| VerifierRequirement | `REQUIRED`, `NOT_REQUIRED` |
| ValidityRequirement | `REQUIRED`, `NOT_REQUIRED` |
| RevocationRequirement | `REQUIRED`, `NOT_REQUIRED` |

`ArtifactClassTrust` is used only where a trust-policy schema needs to
distinguish trust subject classes; it is not an alias for `ArtifactClass`.
There are no other serialized enum types in V1. Enum values are not
case-folded, normalized, or accepted under aliases.

## 28. Version lexical grammar and comparison

Every `EXACT_VERSION` and `VERSION_RANGE` operand uses this grammar:

```text
Version       := Decimal "." Decimal "." Decimal "." Decimal
Decimal       := "0" | NonZeroDigit Digit*
NonZeroDigit  := "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"
Digit         := "0" | NonZeroDigit
```

Each component is an unsigned integer in `0..4294967295` inclusive. Leading
zeroes, signs, whitespace, prefixes, suffixes, alternate separators, missing
components, and overflow are invalid. Version comparison is lexicographic
numeric tuple comparison, not string comparison.

`VERSION_RANGE` requires both `lower` and `upper`, both `string` operands,
with no UNKNOWN, ABSENT, or EMPTY values. Bounds are inclusive and the range
is valid iff `lower <= upper`; matching is exactly
`lower <= candidate <= upper`. Examples:

| candidate | lower | upper | result |
|---|---|---|---|
| `1.2.3.4` | `1.2.3.4` | `2.0.0.0` | MATCH |
| `2.0.0.0` | `1.2.3.4` | `2.0.0.0` | MATCH |
| `1.2.3.3` | `1.2.3.4` | `2.0.0.0` | NO_MATCH |
| `2.0.0.1` | `1.2.3.4` | `2.0.0.0` | NO_MATCH |
| `1.10.0.0` | `1.2.0.0` | `2.0.0.0` | MATCH |

## 29. Exact semantic digest bytes

RFC-0004 aligns with ADR-0011 and does not use an unspecified length prefix.
For each digest-bearing V1 object, the hash input is exactly:

```text
UTF8(DomainLabel) || 0x00 || CanonicalJSONBytes
```

`0x00` is one octet. Domain labels contain only non-NUL ASCII and are
explicitly enumerated:

| KIND | DOMAIN LABEL |
|---|---|
| AUTHORITY_RULE | `landlock-genprof/rfc0004/authority-rule/v1` |
| TRUST_POLICY | `landlock-genprof/rfc0004/trust-policy/v1` |
| SECURITY_FIELD_REGISTRY | `landlock-genprof/rfc0004/security-field-registry/v1` |
| CERTIFICATION_PROPERTY_REGISTRY | `landlock-genprof/rfc0004/certification-property-registry/v1` |
| ARCHITECTURE_ABI_REGISTRY | `landlock-genprof/rfc0004/architecture-abi-registry/v1` |
| BACKEND_ENVELOPE_REGISTRY | `landlock-genprof/rfc0004/backend-envelope-registry/v1` |
| EVIDENCE | `landlock-genprof/rfc0004/evidence/v1` |
| COVERAGE | `landlock-genprof/rfc0004/coverage/v1` |
| COMPLETENESS | `landlock-genprof/rfc0004/completeness/v1` |
| ADEQUACY_EVIDENCE | `landlock-genprof/rfc0004/adequacy-evidence/v1` |
| CERTIFICATION | `landlock-genprof/rfc0004/certification/v1` |
| VERIFICATION_FACT | `landlock-genprof/rfc0004/verification-fact/v1` |
| BASELINE | `landlock-genprof/rfc0004/baseline/v1` |
| COMPATIBILITY_RULE | `landlock-genprof/rfc0004/compatibility-rule/v1` |
| COMPOSITION_OPERATOR | `landlock-genprof/rfc0004/composition-operator/v1` |
| PROVENANCE | `landlock-genprof/rfc0004/provenance/v1` |
| VERIFIER_SEMANTIC | `landlock-genprof/rfc0004/verifier-semantic/v1` |
| SECURITY_CONTEXT | `landlock-genprof/rfc0004/security-context/v1` |
| REVOCATION | `landlock-genprof/rfc0004/revocation/v1` |

The digest is SHA-256 over those exact octets and is encoded as
`sha256:` followed by exactly 64 lowercase hexadecimal digits. There is no
BOM, trailing newline, pretty-print whitespace, UTF-16 conversion, recursive
digest member, algorithm negotiation, or alternate framing. The external
reference digest is exactly this value.

## 30. Micro-repair closure

Unknown enum tokens, invalid versions/ranges, and malformed digest framing
now have one V1 outcome: invalid decoding with no semantic object. The
canonical enum table, version grammar, numeric bounds, tuple comparison, exact
inclusive range rule, domain-label table, separator octet, and SHA-256
contract remove the three previously identified Go/Rust divergence classes.
