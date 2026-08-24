# ADR-0012: Versioned Registry, Rule, and Trust Resolution

Status: Proposed

Date: 2026-08-23

## Decision question

How does landlock-genprof resolve an immutable RFC-0003 typed reference
`(kind, ID, version, digest)` into the exact object intended by an approved
candidate, while keeping identity resolution separate from trust, validity,
revocation, and eligibility?

RFC-0003 is the normative security boundary. ADR-0010 defines the pure
authority-domain objects and ADR-0011 defines their canonical bytes and
cryptographic identities. This ADR defines resolution architecture only.

## Baseline

The current implementation is not an authority resolver. `internal/proposal`
computes the legacy artifact-only `candidate-v1` digest from a fixed JSON
projection, persists it in proposal status, and validates it during
`apply-proposal`. `internal/spoimport` reads named Kubernetes SPO resources
through a dynamic client and snapshots their content, but that is a backend
adapter boundary, not RFC authority resolution. No current registry, typed
rule, trust-policy, or content-addressed authority bundle resolver exists.

The current SPO path therefore MUST remain an adapter that produces a copied
domain evidence/artifact object. A Kubernetes object being Ready, readable,
or named by a user is not a resolved authority object.

## Decision

Use one shared, backend-neutral resolution engine with typed facades. The
engine performs common identity, schema, digest, version, and snapshot checks;
typed facades expose only the expected domain type:

`AuthorityRuleResolver`, `TrustPolicyResolver`, `RegistryResolver`,
`BaselineResolver`, `EvidenceResolver`, `CertificationResolver`,
`CompatibilityRuleResolver`, `CompositionOperatorResolver`, and
`VerifierResolver`.

The core resolver package depends only on ADR-0010 domain types and ADR-0011
digest/canonicalization contracts. Storage adapters implement a source
interface and may be filesystem, embedded bundle, OCI, remote service,
ConfigMap/Secret, or Kubernetes-backed later. The core package MUST NOT import
Kubernetes API or dynamic-client types.

### Source authority

Resolution is performed against an operator-configured, candidate-independent
ordered set of `AuthoritySource` descriptors. Each descriptor has a canonical
`sourceID`, `sourceKind`, the object kinds it may serve, an integer priority,
an enabled state, and a configuration identity/digest. The source set and its
order are not fields supplied by a candidate. A source has one closed mode:

* `AUTHORITATIVE`: its successful observations participate in identity
  conflict detection and its unavailability can prevent a definitive result;
* `MIRROR`: it is consulted for the same exact content but cannot override an
  authoritative observation;
* `FALLBACK`: it is consulted only when no authoritative source is configured
  for that object kind and is never silently mixed with an authoritative set.

For a kind with one or more `AUTHORITATIVE` sources, every enabled source
capable of serving that kind MUST be queried or have a source policy proving
that it cannot affect the requested identity. Priority orders the queries and
diagnostics; it never hides a conflict. A verified exact object from one
source plus a different object for the same kind/ID/version from another
authoritative source is `AMBIGUOUS`, even when the first source has higher
priority. An unavailable authoritative source yields `UNAVAILABLE` unless its
non-participation is explicitly proven by the source configuration. Source
identity, mode, configuration digest, observations, and query result are
recorded as resolution provenance.

`MIRROR` sources must agree with an authoritative result when successfully
queried; disagreement is recorded as a negative integrity observation and
cannot replace the authoritative result. `FALLBACK` sources are used only in
the structurally declared no-authoritative-source case. There is no implicit
source precedence, fallback, or adapter-specific aggregation.

## Exact reference contract

Every resolution request carries the typed reference kind, non-empty ID,
explicit version, and valid digest. All four fields are mandatory and are
matched exactly. Resolution MUST reject kind mismatch, ID mismatch, version
mismatch, digest mismatch, malformed references, and duplicate conflicting
records. It MUST NOT choose latest, nearest, default, fallback, storage order,
or a compatible version.

The conceptual contract is:

`Resolve(ref) -> ResolutionResult(resolvedSnapshot, state, diagnostics)`.

The returned snapshot is immutable for the duration of an evaluation and is
content-bound to the requested digest. A successful identity resolution does
not imply trust, adequacy, compatibility, validity, or eligibility.

## Resolution result domain

The closed resolution states are:

| State | Meaning | RFC-D-usable classification |
|---|---|---|
| `RESOLVED` | Exact kind, ID, version, schema, and digest verified; immutable snapshot returned. | Positive identity fact |
| `NOT_FOUND` | An authoritative source was successfully read and contains no exact reference. | Known negative / `INELIGIBLE` where required |
| `AMBIGUOUS` | Conflicting records share the same identity, or duplicate identity has different content. | Known negative / `INELIGIBLE` |
| `DIGEST_MISMATCH` | Candidate digest differs from object content. | Known negative / `INELIGIBLE` |
| `TYPE_MISMATCH` | Object kind/type does not match the typed reference. | Known negative / `INELIGIBLE` |
| `VERSION_MISMATCH` | Object version is not the exact requested version. | Known negative / `INELIGIBLE` |
| `MALFORMED` | Reference, object, schema, signature binding, or required field is malformed. | Known negative / `INELIGIBLE` |
| `UNAVAILABLE` | Required source or verification fact cannot currently be obtained. | Unknown / `UNKNOWN` |

An exact duplicate with identical canonical content and digest MAY be
deduplicated. No other duplicate-selection behavior is permitted.

## Immutability and snapshots

An immutable reference does not make a mutable referent immutable. A source
adapter MUST read a snapshot, validate its canonical content and digest, and
return a representation that cannot change underneath the evaluation. A
mutable source is acceptable only when the snapshot is copied or otherwise
content-addressed and subsequent mutation is detectable as a different
digest. A cache HIT is valid only when it returns the same verified snapshot
for the same complete reference.

The resolved immutable bundle and mutable current facts are separate:

* immutable bundle: exact authority objects and their referenced registry,
  rule, baseline, evidence, certification, compatibility, composition, and
  verifier snapshots;
* current facts: time, revocation status, current context, trust-source
  availability, and other RFC-0003 evaluation inputs.

## Registry resolution

`RegistryResolver` requires exact `RegistryID`, `RegistryVersion`, and
`RegistryDigest`. A successfully loaded authoritative source with no exact
record returns `NOT_FOUND`; unavailable source returns `UNAVAILABLE`. Same
ID/version with distinct digests returns `AMBIGUOUS`. Unsupported registry
kind/schema or malformed registry returns `MALFORMED`.

Several registry references are resolved independently and assembled into an
immutable `ResolvedRegistrySet` whose identity is the ordered set of exact
typed registry references. There is no ambient current registry. A candidate
bound to an old registry is evaluated against that exact registry, or returns
unavailable/unknown if it cannot be resolved; it is never silently upgraded.

## AuthorityRule resolution

The resolver returns exactly the bound `AuthorityRuleID`, version, and digest.
Revocation does not prevent identity resolution; it is exposed as a separate
current fact for RFC-0003 trust/validity evaluation. A revoked rule therefore
can be `RESOLVED` as an object while its current authority result is negative.

## TrustPolicy and root of trust

`TrustPolicy` is itself resolved by exact typed identity, but resolving it
does not establish that it is trusted. Trust-policy authenticity and root
authorization are bootstrapped from explicit operator-controlled
`RootTrustConfiguration`. This configuration is a candidate-independent
security object with canonical `rootTrustID`, version/epoch, configuration
digest/fingerprint, trust-anchor identities/fingerprints, applicable
artifact/verifier classes, and its own source/provenance identity. Root
anchors are outside candidate-controlled authority metadata and outside
arbitrary Kubernetes objects.

The architecture permits a local configured anchor set or a signed immutable
trust bundle rooted in that set. Concrete key/certificate technology is an
implementation decision. A trust bundle MUST prove its own identity,
integrity, issuer authorization, scope, validity, and non-revocation through
the pre-existing operator root. Candidate data cannot introduce or replace a
root anchor, a remote source cannot supply its own root, and implicit
delegation is forbidden unless a future RFC explicitly defines it. A signed
TrustPolicy cannot establish trust in its own signer.

The exact `RootTrustConfiguration` identity is recorded in every
`ResolvedAuthorityBundle` and evaluation input under which authenticity facts
were established. Changing root configuration does not reinterpret a
historical evaluation; a current authorization decision under a new root
requires re-resolution/re-evaluation.

Resolution, cryptographic integrity, issuer identity, issuer authorization,
and current issuer revocation are distinct facts. ADR-C supplies them;
RFC-0003's `EvaluateIssuerTrust` consumes them and retains its
`TRUSTED | UNTRUSTED | UNKNOWN` semantics.

## Current authority facts

Mutable security facts are represented separately from immutable resolved
objects as `CurrentAuthorityFact` records. Each record contains `factKind`,
subject identity, source identity, observed value/result, observation time,
validity/freshness metadata, verification status, and source version/epoch
when available. This applies to revocation, current issuer status, mutable
trust status, and other security-significant external availability facts.

An applicable source or policy MUST provide a deterministic freshness rule:
`validUntil`, `maxAge`, a source epoch/version contract, or an explicit
non-expiring declaration permitted by RFC-0003. If the required freshness
rule or verification status is unavailable, the fact is `UNKNOWN`. An expired
or stale fact MUST NOT be reused as current positive evidence, although it
MAY be retained for historical replay. ADR-H owns polling, watches, refresh
cadence, and remediation; ADR-C owns this fact shape and freshness handoff.

## Revocation and validity resolution

An immutable `RevocationReference` resolves to a current observation with
`NOT_REVOKED`, `REVOKED`, or `UNKNOWN`, plus observation identity/time. It is
not folded into the immutable authority digest. The observation is a
`CurrentAuthorityFact` and includes source identity, observation time,
freshness/validity, verification status, and source epoch where available.
Missing or stale current revocation data is `UNKNOWN`; an authoritative known
revocation is `REVOKED`.

Immutable validity intervals are part of the resolved object. Current time is
provided by the evaluation context; ADR-C does not silently apply an
arbitrary local wall-clock policy. ADR-H owns scheduling and re-evaluation;
ADR-C supplies the interval and current fact with its provenance.

## Verifier resolution

A `VerifierRef` resolves to an immutable `VerifierSemanticIdentity` with
verifier ID, version, digest, verifier class, accepted input schema/version,
output/result schema/version, recognized RFC-0003 verification property or
class, semantic procedure/protocol identity, and security-significant
implementation constraints. A human-readable verifier name is never enough.

The local execution implementation is separately checked against that
descriptor. Its closed binding result is `VERIFIER_BOUND`,
`VERIFIER_MISMATCH`, or `VERIFIER_UNKNOWN`; mismatch is a known negative and
unknown cannot produce positive verification authority. A digest-bound
executable, plugin, or remote service does not automatically acquire
authority merely by being present. Concrete binding mechanisms are later
implementation decisions, but they MUST demonstrate the exact semantic
identity. ADR-D invokes only the resolved RFC-0003 verifier semantics.

## Compatibility and composition resolution

Compatibility rules and composition operators use the same exact typed
resolution contract. The resolver returns immutable rule/operator semantic
descriptors and their identity; ADR-D performs the RFC-0003 closed predicate
or composition evaluation. No local heuristic, alias, optimizer, plugin, or
similar implementation may substitute for the resolved semantics. A local
operator implementation must be bound to the exact resolved semantic
descriptor; otherwise its result is `VERIFIER_MISMATCH`-equivalent and cannot
contribute authority.

Composition resolution does not make a baseline safe. Baseline identity,
trust, compatibility, adequacy, and composition validity remain separate
facts.

## Evidence and certification resolution

Historical evidence and certification are resolved as immutable snapshots,
not live observation streams. An evidence source mutation after resolution
must either produce a different digest or invalidate the snapshot. A
certification resolver proves exact object identity, type, version, and
content only; ADR-D evaluates certification property, scope, trust,
validity, assumptions, and provenance.

SPO live resources are handled by ADR-G. `kubectl get SeccompProfile`, Ready
status, recording existence, or a merged syscall set is never a generic
authority resolution shortcut. ADR-G creates canonical domain evidence and
provenance snapshots; ADR-C may resolve those exact snapshots afterward.

## Storage and Kubernetes boundary

Storage is replaceable. Sources MAY include embedded registries, immutable
bundles, filesystem, OCI, remote service, ConfigMap/Secret, or Kubernetes
adapters. Each adapter must expose the same exact-reference and snapshot
contract. Kubernetes namespace/name, UID, resourceVersion, and API time are
persistence or diagnostic values unless separately represented as RFC
security-context fields. The authority resolver has no Kubernetes dependency.

## Offline bundles and atomicity

The architecture SHOULD support an offline `ResolvedAuthorityBundle` for
audit, CI, replay, and deterministic evaluation. It contains exact snapshots
of all immutable objects and the complete typed reference graph. It excludes
mutable current revocation/time/context facts. It MUST also contain the
candidate-bound references, source-resolution provenance, source-set
configuration identity/digest, and exact `RootTrustConfiguration` identity.
The bundle does not become a new trust root merely by being stored or
replayed.

Bundle construction resolves all immutable references against one snapshot
boundary. Because each object is content-addressed, storage changes after a
successful snapshot cannot alter its meaning. If any immutable reference
cannot be resolved consistently, the bundle is not produced. Current mutable
facts are sampled separately for ADR-D/H and may yield `UNKNOWN` when their
source is unavailable.

The ADR-D handoff is conceptually:

`EvaluateEligibility(AuthorityMetadata, ResolvedAuthorityBundle, CurrentAuthorityFacts, CurrentSecurityContext, EvaluationTime)`.

ADR-D performs no ambient authority lookup. `CurrentAuthorityFacts` carries
revocation/current-trust observations, source identity, observation time,
freshness, and verification status. `CurrentSecurityContext` and evaluation
time remain separate inputs.

Historical replay uses the recorded bundle, root configuration identity,
current-fact records, and original evaluation time to reproduce what was
known then. It is not current authorization. Current authorization requires
current applicable root trust and current facts.

### Deterministic source aggregation examples

For jointly authoritative sources, all enabled participating sources are
considered before a definitive result:

| Source observations | Result |
|---|---|
| A exact, B exact with identical canonical digest | `RESOLVED` |
| A exact, B same kind/ID/version with different digest | `AMBIGUOUS` |
| A exact, B `NOT_FOUND` | `RESOLVED` only if B's source policy proves non-participation; otherwise `UNAVAILABLE` |
| A exact, B `UNAVAILABLE` | `UNAVAILABLE` |
| A `NOT_FOUND`, B `NOT_FOUND` | `NOT_FOUND` |
| A `NOT_FOUND`, B `UNAVAILABLE` | `UNAVAILABLE` |
| A `DIGEST_MISMATCH`, B exact | `AMBIGUOUS`, never `RESOLVED` |
| duplicate byte-identical canonical objects | one `RESOLVED` snapshot, with duplicate provenance retained |
| same identity with different canonical digest | `AMBIGUOUS` |

Priority orders work; it does not suppress an authoritative conflict. A
`FALLBACK` source is considered only when the declared source set contains no
authoritative source for that kind.

## Legacy behavior

Legacy proposals have no modern references. ADR-C MUST NOT synthesize a
default rule, trust policy, registry, baseline, or current object to upgrade
them. They remain `LEGACY / AUTHORITY_UNKNOWN / ADVISORY_ONLY` until
regenerated, rebound, and reapproved under ADR-0011 and the later lifecycle
decision.

## Security invariants

1. Resolution always matches kind, ID, version, and digest.
2. No implicit latest, fallback, storage-order, or compatible-version selection exists; any `FALLBACK` mode is explicit in source configuration.
3. Digest verification precedes object use.
4. Typed object kind and schema must match the reference.
5. Immutable identity and mutable revocation/validity facts remain separate.
6. Resolving a TrustPolicy never itself establishes issuer trust.
7. Root trust is explicit, configured, and outside candidate-controlled data.
8. Legacy proposals receive no synthesized authority references.
9. Storage adapters are replaceable without changing authority semantics.
10. Caches cannot return changed content for the same verified reference.
11. Known absence is distinct from source unavailability.
12. Backend Ready state is not authority-object resolution.
13. A resolver never returns a mutable live object as an immutable snapshot.
14. Verifier identity never authorizes arbitrary code execution.
15. RootTrustConfiguration identity is recorded with every authenticity fact.
16. Mutable current facts never inherit immutable-object cache semantics.
17. A local verifier/operator must bind to the resolved semantic descriptor.

## Threat model

* Latest substitution is blocked by exact version matching.
* Same ID/version with attacker content is blocked by digest verification and
  ambiguity detection.
* Cross-kind substitution is blocked by typed references and type checks.
* Mutable referents are copied/content-verified before use.
* Cache poisoning is detected by digest and snapshot validation.
* Candidate-controlled TrustPolicy roots are rejected by the root-anchor
  boundary.
* Remote outage is `UNAVAILABLE`, not `NOT_FOUND`.
* Kubernetes mutation after lookup cannot alter a copied snapshot.
* Legacy default-rule injection is forbidden.
* Verifier substitution requires an exact trusted verifier reference.
* Registry/rule rollback is harmless to an exact old digest and cannot be
  silently selected for a different digest.
* Split-brain stores produce `AMBIGUOUS` or `UNAVAILABLE`, never arbitrary
  selection.
* A revoked object may resolve for audit, but ADR-D receives revocation as a
  separate negative fact and cannot treat it as trusted.

## Alternatives considered

### Kubernetes-native CRDs as the authority source of truth

Rejected as the core architecture. CRDs may be a storage adapter, but making
them authoritative would couple the security domain to API shape and Ready
status and would allow mutable-name lookup.

### Local static files only

Rejected as the only source. They are useful for an offline adapter but do
not cover future distribution and remote verification needs.

### Generic URI resolver

Rejected as the domain contract. An opaque URI loses typed kind/version/digest
constraints and invites backend-specific fallback semantics.

### Fully embedded authority bundle per proposal

Rejected as the only representation because it duplicates large objects and
does not remove the need for exact typed identities. An immutable resolved
bundle is retained as an optional evaluation snapshot.

### Content-addressed typed resolver with pluggable storage

Selected. It preserves exact identity and testability while allowing offline,
filesystem, OCI, remote, and Kubernetes adapters without changing RFC
semantics.

## ADR handoffs

* **ADR-B:** owns canonical bytes and digest calculation. ADR-C consumes its
  exact contract and never redefines it.
* **ADR-D:** receives an immutable resolved bundle plus current facts and
  performs `EvaluateEligibility` and all RFC terminal evaluators. It performs
  no ambient lookup.
* **ADR-E:** persists references, resolution state, evaluation records, and
  proposal lifecycle mappings.
* **ADR-F:** consumes current authorization/evaluation identities; it does not
  perform ad-hoc name lookup.
* **ADR-G:** adapts SPO, PodLock/Landlock, and other backends into canonical
  evidence/provenance and immutable snapshots.
* **ADR-H:** owns re-resolution scheduling, current-fact refresh, expiry,
  revocation handling, and active-state transitions.

None of these ADRs may redefine RFC-0003 result semantics or exact-reference
requirements.

## Testing requirements

Future implementation work MUST provide:

* unit tests for exact kind/ID/version/digest, wrong digest/type/version,
  malformed refs, ambiguity, and no fallback;
* property tests that the same verified ref returns the same semantic content;
* negative security tests for latest substitution, cache poisoning,
  cross-kind confusion, root-trust injection, mutable referents, and verifier
  substitution;
* offline bundle replay and cross-language digest-vector tests;
* concurrency tests for immutable snapshots during source mutation;
* migration tests proving legacy proposals receive no synthesized refs;
* tests distinguishing `NOT_FOUND` from `UNAVAILABLE` and revoked identity
  from failed identity resolution.

## Consequences and open implementation questions

This decision adds typed resolution, explicit root trust, immutable snapshots,
and a clean handoff to the pure evaluator. It requires resolver adapters and
authority-bundle storage work before governed enforcement can consume modern
references. Concrete Go interfaces, persistence/CRD representation, cache
policy, signature technology, PKI, bundle distribution, and watch mechanics
remain implementation decisions for later ADRs and MUST preserve this
contract.
