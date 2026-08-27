# ADR-0010: RFC Authority Domain Model

Status: Proposed

Date: 2026-08-22

## Context

RFC-0003 is the normative security boundary for observation, coverage,
adequacy, compatibility, trust, eligibility, authorization, materialization,
active authority, and revocation. The existing implementation is centered on
rendered Kubernetes artifacts and approval status: `internal/proposal.Spec`
stores artifact strings, `Status` stores Draft/Reviewed/Approved/Rejected,
and `CandidateDigest` binds the current artifact projection. Those structures
are not sufficient as the domain model for RFC-0003 and must not become the
semantic authority model accidentally.

The model must remain independent of Kubernetes API types, CRD layout, CLI
workflow, backend artifacts, and CandidateDigest canonicalization. RFC-0003
defines those security semantics; ADR-B owns canonical binding, ADR-C owns
reference resolution, ADR-D owns evaluation, ADR-E owns proposal lifecycle,
and ADR-F owns apply-time enforcement.

## Decision

Introduce a pure internal authority domain boundary (proposed package:
`internal/authority`). It contains language-neutral value concepts and typed
references required to carry RFC-0003 meaning. It does not import Kubernetes,
SPO, PodLock, CLI, persistence, or backend packages.

### Domain classifications

* **Value objects**: `SecurityContextIdentity`, execution scopes,
  `ValidityWindow`, tri-state facts, `RegistryBinding`, and typed digests.
  They are immutable and compared by value.
* **References**: typed immutable references to rules, policies, baselines,
  registries, evidence, verifiers, certifications, compatibility rules, and
  composition operators. A reference contains kind, ID, version, and digest;
  it never means “latest”.
* **Entities**: `AuthorityMetadata` and `EligibilityRecord`, identified by
  their bound candidate/evaluation identity and immutable after issuance.
* **Enums/results**: authority lifecycle dimensions, eligibility results,
  validity/revocation results, and explicit unknown/invalid states.
* **Persistence concerns**: Kubernetes objects, CRD status, JSON/YAML field
  names, resource versions, and storage indexes. These remain outside the
  domain.

### AuthorityMetadata

`AuthorityMetadata` is the immutable security interpretation associated with
one candidate. It contains typed references or value objects for:

* backend and enforcement scope;
* observation/evidence scope and evidence references;
* `SecurityContextIdentity`;
* completeness claims and adequacy-evidence references;
* baseline reference and compatibility-rule reference;
* authority-rule reference and trust-policy reference;
* certification, provenance, and verifier references;
* registry bindings;
* validity and revocation references.

It does not contain rendered PodLock, SPO, NetworkPolicy, or Kubernetes
manifests. It does not contain canonical bytes or compute a digest; ADR-B owns
that binding.

### SecurityContextIdentity

`SecurityContextIdentity` is an immutable value object containing the
security-relevant image identity, architecture, ABI, kernel/runtime classes,
libc identity where applicable, privilege/capability state, namespace/security
state, configuration identity, relevant environment and feature flags,
persistent-state class, workload identity, and executable identity where
applicable. Comparison semantics are those of RFC-0003 and later ADR-C
resolution.

Pod UID, resourceVersion, container ID, diagnostic timestamps, node IP, and
other diagnostic/runtime identifiers are not semantic identity fields.

### Typed references

The model uses distinct reference types rather than an untyped resource name:

`AuthorityRuleRef`, `TrustPolicyRef`, `BaselineRef`, `RegistryRef`,
`EvidenceRef`, `VerifierRef`, `CertificationRef`, `CompatibilityRuleRef`, and
`CompositionOperatorRef`.

Each reference is immutable and carries ID, version, and digest. A reference
with an empty or ambiguous identity is invalid; resolution semantics belong to
ADR-C. This prevents type confusion and ambient latest-version lookup.

### Unknown and invalid values

Security-significant facts MUST use explicit typed states. `UNKNOWN`,
`ABSENT`, `EMPTY`, `FALSE`, and `INVALID` are distinct semantic values.
Security objects MUST NOT rely on zero-value strings, booleans, nil slices, or
missing map keys to represent a positive or negative security fact. Constructors
or validation boundaries must reject incomplete objects rather than silently
promoting them.

### EligibilityRecord

`EligibilityRecord` is an immutable evaluation record produced by ADR-D. It
binds:

* candidate identity;
* exact authority-rule identity/digest;
* registry bindings;
* context identity/fingerprint;
* baseline and composition identities;
* evidence/provenance identities;
* eligibility result and reason references;
* evaluation time and validity deadline;
* revocation state/version.

It is evidence of an evaluation, not an evaluator and not authorization by
itself. Reuse with another candidate, context, rule, registry, or validity
window is invalid.

### Authority state representation

The lifecycle is represented as orthogonal dimensions rather than one linear
enum:

* review/approval: advisory, reviewable, approved bytes;
* semantic eligibility: eligible, ineligible, unknown;
* authorization: not authorized, authorized for enforcement;
* backend realization: not materialized, materialized;
* runtime authority: inactive, active, active-unauthorized,
  suspended-unknown, behaviorally verified.

This prevents impossible combinations such as approval implying eligibility,
materialization implying active authority, or active attachment implying
behavioral verification. ADR-E maps these dimensions into proposal
lifecycle/status; ADR-F consumes them for apply gating; ADR-H owns temporal
invalidations.

### Immutability

AuthorityMetadata, typed references, SecurityContextIdentity values,
RegistryBinding values, and EligibilityRecord instances are immutable once
published. A changed interpretation creates a new identity and requires the
binding/review path owned by ADR-B/E. Revocation and invalidation are new
records or state observations; they do not rewrite historical evaluation
records.

### Domain/persistence boundary

Adapters map domain objects to and from persistence representations:

`AuthorityMetadata <-> proposal/authority persistence`

`EligibilityRecord <-> proposal status or authority persistence`

`SecurityContextIdentity <-> collectors and backend adapters`

The domain does not import Kubernetes API types or CRD structs. CRD/schema
decisions are deferred to ADR-E and related schema work.

### Legacy representation

An existing proposal without RFC-0003 metadata is represented explicitly as
`LEGACY`, `AUTHORITY_UNKNOWN`, and `ADVISORY_ONLY`. Its existing artifact and
approval bytes remain available for inspection, but it cannot be represented
as semantically eligible or authorized without regeneration and reapproval.

### Backend neutrality

The domain has no PodLock or SPO structs. Seccomp, PodLock/Landlock,
NetworkPolicy, capabilities, and future backends identify their backend and
provide typed evidence/reference objects through adapters. Backend envelopes
and semantics remain RFC-0003 concepts resolved by ADR-C/D, not redefined in
this model.

## Security invariants

1. No security-significant reference is represented only by an ambient mutable
   name or latest lookup.
2. UNKNOWN cannot be encoded as a zero-value positive or negative fact.
3. AuthorityMetadata and EligibilityRecord are immutable once issued.
4. Domain authority types do not depend on Kubernetes API types.
5. Legacy proposals cannot imply eligibility or authorization.
6. Approval cannot imply eligibility.
7. Backend readiness cannot imply authorization.
8. SecurityContextIdentity excludes diagnostic-only runtime identity.
9. Reference types structurally prevent cross-kind substitution.
10. Domain objects preserve all security-significant data needed for later
    deterministic canonicalization by ADR-B.
11. EligibilityRecord replay with a different bound identity or context is
    invalid.
12. Partial object construction cannot produce a valid authority object.

## Alternatives considered

### A — Put authority fields directly in SecurityProfileProposal

Rejected as the primary model. It couples semantics to CRD persistence,
encourages zero-value ambiguity, and makes non-Kubernetes consumers depend on
workflow schema.

### B — Pure internal authority domain with adapters

Selected. It preserves RFC semantics independently from persistence and lets
ADR-E choose CRD mappings without changing domain meaning.

### C — Generic `map[string]any` authority metadata

Rejected. It permits type confusion, missing/empty ambiguity, unregistered
fields, and language-specific interpretation.

### D — Backend-specific authority models

Rejected as the primary model. Backend adapters remain necessary, but a
backend-first authority model would duplicate lifecycle, trust, eligibility,
and provenance semantics and permit inconsistent enforcement gates.

## Threat-model consequences

* Type confusion is blocked by typed references.
* Stale references are detected by immutable ID/version/digest binding.
* Latest-version lookup is excluded from the domain.
* Zero-value UNKNOWN collapse is blocked by explicit states.
* Metadata mutation after approval requires a new identity.
* Eligibility records cannot be replayed under another context.
* Backend readiness is a separate realization dimension.
* Legacy objects cannot escalate into modern authority.
* Kubernetes representation cannot redefine domain semantics.

## Go and cross-language implementability

The model is implementable in Go using private fields plus validating
constructors, explicit enums, typed reference wrappers, and immutable value
objects. No Go zero-value convention carries authority semantics.

The same model is language-neutral: a Rust implementation must preserve the
same typed identities, explicit states, immutable bindings, and domain
separation. Serialization and cryptographic canonicalization are deliberately
owned by ADR-B, not this ADR.

## ADR handoffs

* **ADR-B** receives the complete set of security-significant domain fields,
  typed references, registry bindings, and immutable identity boundaries. It
  decides canonical bytes and digest binding, not their meaning.
* **ADR-C** resolves the exact typed references, registries, rules, trust
  policies, verifiers, and composition operators.
* **ADR-D** receives AuthorityMetadata, evidence references, context, and
  EligibilityRecord types and implements RFC-0003 evaluation.
* **ADR-E** maps orthogonal authority states and legacy markers into proposal
  persistence and approval lifecycle.
* **ADR-F** consumes eligibility/authorization records at the apply gate.

None of these ADRs may redefine RFC-0003 result semantics, envelopes,
adequacy, trust, compatibility, composition, revocation, or precedence.

## Future test requirements

The model requires unit and property tests for explicit-state handling,
typed-reference rejection, immutable-record binding, and impossible-state
prevention. Negative-security tests must cover type confusion, stale-reference
replay, zero-value UNKNOWN, legacy escalation, approval-to-eligibility,
backend-ready-to-authorization, and context-mismatched EligibilityRecord
reuse. Serialization-boundary tests must confirm domain/persistence adapters
do not introduce semantic defaults.

## Consequences

This decision creates a stable, backend-neutral security domain and prevents
CRDs or CLI workflows from becoming accidental authority semantics. It adds
typed validation, explicit state dimensions, and adapter code. It deliberately
defers canonicalization, rule resolution, evaluator implementation, proposal
schema, apply ordering, and physical remediation to the ADRs that own those
decisions.

## References

* RFC-0003 — Observation, Coverage, Adequacy, and Enforcement Authority
* ADR-0006 — Security Profile Proposal approval binding
* ADR-0007 — Governed apply ordering and enforcement readiness
* ADR-0008 — SPO derived-policy import boundary
* ADR-0009 — SPO merged Seccomp profile provenance and target separation
* `internal/proposal/types.go`
* `internal/proposal/digest.go`
* `internal/proposal/validate.go`
* `internal/k8s/apply.go`
