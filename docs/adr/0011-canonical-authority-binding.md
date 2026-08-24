# ADR-0011: Canonical Authority Binding and CandidateDigest Evolution

Status: Proposed

Date: 2026-08-22

## Context

The current `internal/proposal.CandidateDigest` marshals a fixed JSON struct
containing `Container`, `Binary`, `PodLock`, `NetworkPolicy`,
`PatchedManifest`, and `SPOSeccompProfile`, then hashes it with SHA-256. The
artifact strings are preserved verbatim. `GeneratedAt` and `HistoryUsed` do
not participate. `ApprovedCandidateDigest` and
`ApprovalMechanismVersion` are persisted in proposal status, and current
validation detects malformed or changed candidate artifacts.

This establishes artifact identity only. It does not bind RFC-0003 authority
interpretation, scope, context, registries, evidence, eligibility, or
validity. Consequently, changing authority metadata without changing the
existing artifact projection could preserve an approved digest while changing
its security meaning.

## Problem

The system needs a reproducible identity chain:

exact artifact bytes + exact authority interpretation
→ approved identity
→ evaluated identity
→ future apply identity

Security-significant differences must change the bound identity. Diagnostic
differences must not change it. Legacy digests must not silently acquire the
new semantics.

## Decision

Use a three-level digest topology:

1. **ArtifactDigest** identifies the exact rendered backend artifact projection
   currently hashed by the existing CandidateDigest implementation.
2. **AuthorityMetadataDigest** identifies canonical RFC-0003 authority
   metadata, including registry/rule/reference/context/scope bindings.
3. **BoundCandidateDigest** identifies the pair
   `(ArtifactDigest, AuthorityMetadataDigest)` under a versioned,
   domain-separated binding envelope.

The existing digest remains valid as a legacy artifact identity. New
RFC-0003-aware proposals use `BoundCandidateDigest` and an explicit digest
mechanism/version. There is no transparent reinterpretation of an old digest.

## Digest semantics

* `ArtifactDigest` proves exact artifact bytes and their fixed projection. It
  does not prove authority, adequacy, eligibility, or runtime success.
* `AuthorityMetadataDigest` proves exact canonical authority metadata. It does
  not prove that the referenced objects resolve, are trusted, or are eligible.
* `BoundCandidateDigest` proves that the exact artifact identity and exact
  authority identity were bound together. It does not prove correctness,
  completeness, adequacy, eligibility, authorization, materialization, active
  enforcement, or behavior.

All three are integrity/binding identities, not security decisions.

## Canonical authority representation

The canonical wire format is **RFC 8785 JSON Canonicalization Scheme (JCS)**,
using the published RFC 8785 rules and UTF-8 without a byte-order mark. This
is a binding decision, not a library choice: ordinary language JSON
serialization is not sufficient. `CanonicalAuthorityBytes(metadata)` is the
JCS encoding of a fixed authority root object after validation against
SecurityFieldRegistry version 1 and the declared authority schema version.
The root contains exactly the registered field names, including schema and
registry ID/version/digest, backend, both scopes, all typed rule/envelope/
trust/baseline/compatibility/composition/certification/verifier/evidence/
provenance references, completeness and adequacy evidence, the complete
SecurityContextIdentity, and immutable validity/revocation references.
Unknown fields, unsupported schema versions, missing required fields,
duplicate keys, invalid values, or any failed validation produce no bytes and
no digest.

All strings MUST be valid UTF-8. No Unicode normalization, case folding, or
whitespace trimming is performed; distinct valid Unicode scalar sequences are
distinct values. Identifiers, enum labels, backend names, paths, and versions
are case-sensitive and preserve whitespace. Embedded NUL is rejected for
identifiers, enum labels, versions, digests, and paths. Authority metadata has
no floating-point fields. Signed and unsigned integers use canonical decimal
strings (optional leading minus only for signed values; no plus sign, leading
zero, or alternate base). Booleans are JSON `true`/`false`. Immutable
timestamps use UTC RFC3339Nano with `Z`, no offset, and no redundant trailing
fractional zeroes. Durations, if registered, use decimal integer nanoseconds
as strings.

Digest values are structured objects with `algorithm` and `hex` fields. The
current registry accepts only lowercase `sha256` and exactly 64 lowercase
hexadecimal characters. Uppercase, malformed, or wrong-length digests fail;
other algorithms require a new registry/binding version.

`UNKNOWN`, `ABSENT`, and `EMPTY` are explicit tagged values and are never
represented by omission, `null`, zero values, or false. Their canonical forms
are respectively `{\"state\":\"UNKNOWN\"}`, `{\"state\":\"ABSENT\"}`, and
`{\"state\":\"EMPTY\"}`; a field may use one only when its registry entry
permits it. Sets are arrays whose elements are recursively canonicalized and
sorted by canonical UTF-8 bytes; duplicate canonical elements are rejected.
Ordered lists preserve order. Maps have fixed string keys, reject duplicate
keys, and use JCS key ordering. These rules apply recursively. Null,
unsupported extensions, illegal state transitions, and duplicate semantic
fields fail rather than being ignored.

## Domain separation

Hash inputs use distinct domain labels and schema versions:

* `landlock-genprof/artifact/v1`;
* `landlock-genprof/authority-metadata/v1`;
* `landlock-genprof/bound-candidate/v1`;
* `landlock-genprof/registry/v1`;
* `landlock-genprof/eligibility-record/v1`.

SHA-256 remains the selected hash algorithm because it is already used by
the repository, has stable cross-language implementations, and is sufficient
for content identity here. The encoded form remains `sha256:<lowercase hex>`.
Every digest envelope carries its algorithm and mechanism/schema version; no
surrounding API field is required to interpret it. Domain labels prevent
cross-object digest substitution.

## Exact digest envelopes

Artifact bytes are the exact existing rendered artifact projection. The
artifact envelope is a fixed JCS object containing `artifactSchema` version 1
and the six named fields (`Container`, `Binary`, `PodLock`, `NetworkPolicy`,
`PatchedManifest`, `SPOSeccompProfile`) as UTF-8 strings, so field boundaries
cannot be confused by concatenation. Define:

`ArtifactDigest = SHA256(ASCII("landlock-genprof/artifact/v1") || 0x00 || ArtifactCanonicalBytes)`.

`AuthorityMetadataDigest` is:

`SHA256(ASCII("landlock-genprof/authority-metadata/v1") || 0x00 || CanonicalAuthorityBytes(metadata))`.

`BoundCandidateDigest` uses this exact JCS object (JCS determines its actual
key ordering):

```json
{"artifactDigest":{"algorithm":"sha256","hex":"<64 lowercase hex>"},"authorityMetadataDigest":{"algorithm":"sha256","hex":"<64 lowercase hex>"},"bindingMechanism":{"id":"landlock-genprof/bound-candidate","version":"1"},"hashAlgorithm":"sha256"}
```

Its digest is:

`BoundCandidateDigest = SHA256(ASCII("landlock-genprof/bound-candidate/v1") || 0x00 || CanonicalBoundCandidateBytes)`.

Thus the binding mechanism ID/version, hash algorithm, artifact digest, and
authority metadata digest are cryptographically committed. Registry and
future EligibilityRecord digests use the same framing rule with their own
distinct domain labels. A domain label is never reused across object kinds.

## Typed reference binding

Every reference is encoded as a typed structure containing its kind, ID,
version, and digest. The kinds are distinct even when their strings match:

`AuthorityRuleRef`, `TrustPolicyRef`, `BaselineRef`, `RegistryRef`,
`EvidenceRef`, `VerifierRef`, `CertificationRef`, `CompatibilityRuleRef`, and
`CompositionOperatorRef`.

An AuthorityRuleRef and BaselineRef with the same ID/version/digest therefore
produce different canonical bytes. Empty references, ambiguous duplicate
identities, and malformed digests fail. Resolution and trust remain ADR-C
responsibilities.

## Context and scope binding

Authority metadata binds the complete SecurityContextIdentity and both
ObservationScope and EnforcementScope. Security-relevant image, architecture,
ABI, kernel/runtime class, libc, privilege, namespace/security state,
configuration, environment, feature flags, persistent state, workload, and
executable fields participate according to RFC-0003 comparison mode.

Pod UID, resourceVersion, container ID, incidental timestamps, and node IP do
not participate unless separately promoted by a future registry version.
Changing binary-scoped observation to container-wide scope changes canonical
authority bytes and therefore the AuthorityMetadataDigest and
BoundCandidateDigest.

## Validity and revocation boundary

Immutable interpretation binds validity-window and revocation-reference
identities, not mutable revocation answers. Current revocation and context
facts belong to EligibilityRecord/current evaluation state and do not require
CandidateDigest changes for every revocation event. Changing the referenced
validity policy, revocation source, or context binding does change the
authority digest.

## Approval binding

Future approval binds `BoundCandidateDigest`, its digest mechanism version,
and the exact bound authority metadata. Approval means approval of those
bytes and identities only. It is not eligibility or authorization. Changing
authority rule, scope, baseline, registry, evidence, certification,
provenance, context, validity, or composition binding requires a new bound
digest and a new approval.

## EligibilityRecord binding

An EligibilityRecord must carry the BoundCandidateDigest plus exact
AuthorityMetadataDigest, AuthorityRule identity, registry bindings, context
identity/fingerprint, baseline and composition identities, evidence and
provenance identities, evaluation time/deadline, and revocation state/version.
It cannot be replayed for another candidate, metadata object, rule, registry,
context, baseline, composition operator, or validity interval.

## Future apply-proposal identity chain

ADR-F will later verify:

`stored artifacts → ArtifactDigest → AuthorityMetadataDigest →
BoundCandidateDigest → approved digest → current EligibilityRecord →
authorized identity → backend application`.

ADR-B does not define the gate or evaluation algorithm; it guarantees that
the identities needed by that gate are available and unambiguous.

## Migration

Existing `candidate-v1` digests remain artifact identities only. Existing
proposals and approvals are `LEGACY / AUTHORITY_UNKNOWN / ADVISORY_ONLY` for
RFC-0003 purposes. The authority-bound mechanism is named
`BoundCandidateDigest/v1` and is not accepted by legacy artifact-digest
validation. A legacy digest is not accepted in a BoundCandidateDigest field,
and a bound digest is not accepted by legacy validation. Regeneration plus
reapproval is required; there is no compatibility shortcut.

The mechanism identifier/version, authority schema ID/version, artifact
schema version, SecurityFieldRegistry ID/version/digest, and SHA-256
algorithm are all part of the canonical inputs. A schema, registry, or
mechanism change that changes security semantics requires a new version and
therefore a new digest. An old candidate is evaluated against its exact
bound versions or is unavailable/UNKNOWN; it is never silently reinterpreted.

## Approval and evaluation handoff

Approval MUST carry the exact `BoundCandidateDigest`, binding mechanism ID and
version, and (where not self-describing) the hash algorithm. It approves only
that identity. An EligibilityRecord MUST reference the same
BoundCandidateDigest and the exact AuthorityMetadataDigest, rule/registry
bindings, context identity, baseline/composition identities, evidence and
provenance identities, and evaluation identity. Current validity, revocation,
and time remain mutable evaluation facts and are not silently folded into the
immutable authority digest.

## Cross-language golden-vector contract

The canonical bytes and digest values are normative compatibility fixtures.
The implementation MUST publish vectors containing the semantic source
object, exact canonical bytes (hex or base64), expected ArtifactDigest,
AuthorityMetadataDigest, BoundCandidateDigest, and expected failure for
invalid inputs. Independent Go and Rust implementations MUST byte-compare
these vectors rather than compare parsed objects. The minimum vector set is:

* minimal and maximal representative authority metadata;
* map insertion reorder, set insertion reorder, and ordered-list reorder;
* UNKNOWN versus ABSENT and EMPTY versus ABSENT;
* non-ASCII UTF-8, malformed UTF-8 rejection, and malformed digest;
* typed-reference kind substitution and registry digest change;
* context and scope changes, authority-schema and binding-mechanism version
  changes;
* duplicate set member, unsupported field, and legacy digest rejection;
* unchanged artifacts with changed authority metadata.

For a simple valid authority object, the required derivation is:

`semantic object → validate registry/schema → fixed JCS root bytes →`
`SHA256("landlock-genprof/authority-metadata/v1" || 0x00 || bytes)`.

Because RFC 8785 fixes UTF-8 JSON escaping, key ordering, and array/object
serialization, and this ADR fixes all field types and state encodings, a Go
and Rust implementation cannot choose different canonical bytes while
conforming. The same contract applies to the bound-candidate envelope.

## Failure semantics

Canonicalization/binding fails closed for unsupported schema or registry
version, unknown security field, malformed digest, missing required
reference, duplicate set member, duplicate identity with different digest,
invalid enum, illegal UNKNOWN/ABSENT representation, unsupported algorithm,
or registry mismatch. No partial identity is persisted or approved.

## Threat model

* Unchanged artifacts with changed authority metadata produce a new bound
  digest.
* Swapping any rule, registry, baseline, evidence, certification, provenance,
  scope, context, or composition reference changes authority bytes.
* Registry contents changing under a nominal version changes the bound
  registry digest and fails the binding.
* UNKNOWN versus ABSENT has distinct bytes.
* Reference kind substitution has distinct bytes.
* Map ordering is canonical; duplicate fields/members fail.
* Legacy digests cannot validate as v2 bound identities.
* Eligibility records cannot replay against a different bound identity.
* Cross-object digest substitution is blocked by domain separation.

## Alternatives rejected

### Extend the existing CandidateDigest directly

Rejected as the sole topology: it would blur legacy artifact identity with
new authority identity and make migration/error classification harder.

### Independent AuthorityMetadataDigest without a bound digest

Rejected: separately stored digests permit accidental pairing of artifacts
and authority metadata unless a higher-level binding is mandatory.

### Merkle tree as the primary model

Rejected for now: it adds structure without improving the semantic boundary;
the bound pair is sufficient and easier to audit. A future implementation may
use an internal tree only if it produces the same specified bound identity.

### Opaque independent binding object

Rejected as the primary identity: it creates another pairing layer that could
be lost in persistence. A binding object may be an implementation transport,
but its digest must equal the specified BoundCandidateDigest.

## Testing requirements

Future tests must include:

* golden vectors for independent Go/Rust canonicalization;
* map/set ordering invariance and ordered-list sensitivity;
* same semantic object → same bytes/digest;
* scope, context, rule, baseline, registry, evidence, certification,
  provenance, validity, and composition changes → changed bound digest;
* UNKNOWN versus ABSENT distinction;
* typed-reference substitution rejection;
* unsupported field/schema/digest failure;
* registry-version/digest mutation detection;
* legacy v1 rejection as modern authority;
* approval replay and EligibilityRecord replay rejection.

## Handoffs and firewall

RFC-0003 owns security meaning and result semantics. ADR-0010 owns domain
object responsibilities. ADR-B owns only canonical representation,
cryptographic identity, semantic binding, digest evolution, and migration
identity rules.

ADR-C owns exact resolution and trust; ADR-D owns evaluation; ADR-E owns
proposal persistence/lifecycle; ADR-F owns apply gating; ADR-G owns backend
adapters; ADR-H owns revalidation/revocation. None may redefine digest
meaning or RFC security semantics.

## Consequences

The design makes authority interpretation auditable and reproducible while
preserving current artifact-digest behavior for legacy objects. It requires
new versioned metadata, golden vectors, migration handling, and explicit
binding validation. It does not implement CandidateDigest v2 or change any
runtime path.

## Open implementation questions

Later implementation ADRs may choose the JCS library, golden-vector storage,
and persistence mapping. They MUST use the fixed
`BoundCandidateDigest/v1` mechanism, RFC 8785 bytes, field/state rules, exact
domain framing, and digest topology specified here. They may not substitute a
different wire format, digest envelope, or compatibility interpretation.
