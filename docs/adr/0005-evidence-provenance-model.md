# ADR-0005 — Evidence Provenance Model

Status: Accepted
Date: 2026-08-11
Authors: idriss-eliguene

Purpose
-------
This ADR specifies the minimal, backward-compatible evidence provenance *persistence* model and the validation, scope and identity-firewall invariants required to prevent provenance laundering and accidental semantic identity mutation. ADR-0005 does not define semantic projection versioning, projection registries, or migration/replay policies — those responsibilities are reserved to ADR-0006.

Context
-------
A provenance capture audit shows evidence v2 persists Run.Source, Run.RecordTime and tracer.Event fields, but lacks document-local per-event provenance references, adviser flags, and authenticated backend agent identities. The project requires a minimal persistence model that is:

- Persistent (survives serialization),
- Replayable when paired with explicit projection governance (ADR-0006),
- Truthful (does not permit provenance laundering),
- Minimal and privacy-aware,
- Separable: passive evidence metadata must not become semantic identity without explicit governance.

Decision (summary)
------------------
1. Persist provenance as a document-local, optional map of ProvenanceSources and per-event provenance_id references.
2. Validation: dangling or malformed provenance is structural corruption and MUST be rejected (FAIL CLOSED). Absence of provenance is valid legacy evidence.
3. Reserve `Run.SemanticProjectionVersion` as passive metadata only. ADR-0005 MUST NOT define projection registry, selection, ordering, or compatibility rules; ADR-0006 owns those decisions.
4. Enforce identity-firewall invariants (INV-PROV-01..04) so passive metadata does not alter current semantic identities under the current projection.
5. Projectors remain pure Observation → Proposition and MUST NOT consume passive provenance metadata or Run-level provenance fields.

Conceptual document model (conceptual only — no wire change implied here)
--------------------------------------------------------------------------
Document {
  Version: "v2",
  Run: {
    Source,
    RecordTime,
    RunID? (optional UUID),
    Start?,
    End?,
    TargetRef? (optional acquisition/target metadata),
    SemanticProjectionVersion? (optional, reserved; ADR-0006 owns semantics)
  },
  ProvenanceSources?: { id -> {
       BackendKind (required),
       OriginType (required: one of direct, derived, advisory, imported, unknown),
       BackendAgentID? (optional opaque metadata),
       CollectorVersion? (optional)
  } },
  Events: [ { existing tracer fields..., provenance_id? } ]
}

Normative structural contract for provenance
-------------------------------------------
- `provenance_id` is a document-local reference string. It MUST be non-empty when present and correspond to a key in `ProvenanceSources`.
- `ProvenanceSources` keys are document-local logical identifiers and MUST be unique within the document.
- `BackendKind` is REQUIRED and non-empty. It classifies the evidence source (examples: filesystem, network, capability, seccomp-adviser, external). It is evidence classification only and MUST NOT be treated as semantic SubjectIdentity.
- `OriginType` is REQUIRED and MUST be exactly one of: `direct`, `derived`, `advisory`, `imported`, `unknown`.
- `BackendAgentID` is OPTIONAL, opaque evidence metadata. Presence alone does NOT convey authentication or SubjectIdentity; it is not a semantic SubjectIdentity unless later promoted by ADR-0006+minting authority.
- `CollectorVersion` is OPTIONAL opaque metadata.

Explicit absence vs empty distinction
------------------------------------
- `provenance_id` absent on an Event means legacy/unknown provenance and is allowed: do not treat it as proof that the run-level Act was the epistemic producer. Consumers MAY apply a documented legacy compatibility policy (ADR-0006) when interpreting such documents.
- `provenance_id` present but empty (""), or present but mapped to a missing `ProvenanceSources` entry, is MALFORMED and MUST be rejected (FAIL CLOSED).
- `OriginType` absent in a ProvenanceSources entry is MALFORMED and MUST be rejected. `OriginType = unknown` is a valid, explicit value distinct from absence.

Duplicate JSON-key handling
---------------------------
- JSON object duplicate keys are an encoding-level concern. The default Go `encoding/json` decoder will accept duplicate keys and the last key wins. If duplicate-key rejection is required for strict validation, an implementation MUST use a strict JSON decoder that detects duplicate keys. ADR-0005 requires the ingestion/validation pipeline to detect duplicate provenance keys and treat duplicated keys as malformed (implementation requirement), but does NOT mandate the encoding library.

Validation rules (normative)
----------------------------
- Case A — provenance absent: allowed (legacy). Document accepted; provenance unknown.
- Case B — provenance reference present but dangling: REJECT document (FAIL CLOSED).
- Case C — provenance source present but malformed (missing required fields, invalid values): REJECT document (FAIL CLOSED).
- No best-effort mapping or silent fallback is permitted for B/C. A and B/C are distinct.

Provenance authenticity and authority
-------------------------------------
- Evidence-declared provenance ≠ authenticated provenance ≠ semantic epistemic producer.
- Presence of `BackendAgentID` or `OriginType=direct` in persisted evidence MUST NOT by itself: mint SubjectIdentity, change ProducerRef, alter ActIdentity/AssertionEvent/Proposition identities, confer trust, or be treated as authenticated evidence. Authentication and trust are out of scope and require separate authority (ADR-0006 or other governance).

Document-local provenance identifier scope
-----------------------------------------
- All `provenance_id` keys are scoped to the containing Document. The same identifier string may name different sources in different documents. `provenance_id` is not globally meaningful and MUST NOT be used directly as SubjectIdentity or trust token.

Structural equality vs duplication
----------------------------------
- Multiple `ProvenanceSources` entries that are structurally equal are allowed. Structural duplication does not imply semantic distinction. IDs are references; duplication is permitted and SHOULD NOT change semantic projection under the current projection.

SemanticProjectionVersion responsibility boundary
-----------------------------------------------
- ADR-0005 MAY reserve `SemanticProjectionVersion` as passive metadata. ADR-0005 MUST NOT define syntax, registry, ordering, compatibility, selection, fallback, or migration behavior. ADR-0006 will define the projection registry and mapping from legacy documents to projection identifiers.
- An explicit, unsupported `SemanticProjectionVersion` referenced at replay time MUST NOT silently fall back to the current projection; ADR-0006 will define error handling.

Projector boundary (normative)
-------------------------------
- Domain projectors (Observation → Proposition) MUST NOT consume Run-level or Event-level provenance metadata (RunID, TargetRef, provenance_id, BackendAgentID, ProducerRef, SemanticProjectionVersion). Projectors operate on observation content only.

RunID and TargetRef identity-firewall
------------------------------------
- `RunID` is acquisition-instance correlation metadata only. It is NOT Source, SubjectIdentity, ProducerRef, ActIdentity component, Proposition identity component, or AssertionEvent identity component under the current projection.
- `TargetRef` is acquisition/target metadata only and MUST NOT be treated as Source or Producer. Presence or mutation of `TargetRef` MUST NOT change semantic identities under the current projection.

Event order & EvidenceGroups non-interference
--------------------------------------------
- Provenance metadata MUST NOT reorder, regroup, renumber, or otherwise alter event ordering or EvidenceGroups semantics. Existing decoding/encoding behavior preserving event order must be maintained.

Privacy considerations
----------------------
- By default avoid high-churn or host-identifying fields (node IDs, kernel boot IDs, PIDs, container runtime instance IDs). `BackendAgentID` and `TargetRef` are optional and privacy-sensitive; implementations ought to make them opt-in and subject to privacy review before enabling default emission.

Migration, implementation stages (non-normative terminology)
-----------------------------------------------------------
Use durable stage names rather than transient workflow gate labels:

- Implementation stage 1 — Passive persistence: add optional fields to schema and store them when present; do not consume them semantically.
- Implementation stage 2 — Provenance capture: production emitters may populate provenance metadata.
- Implementation stage 3 — Non-interference verification: validate round-trip preservation and identity non-interference via golden tests.

ADR-0006 will specify projection registry, legacy mapping, and replay/migration governance required before any semantic use of provenance metadata.

Acceptance criteria (for ADR adoption)
--------------------------------------
- Validation rules (FAIL CLOSED for B/C) are codified in ingestion/replay checks.
- Projector purity and identity-firewall invariants are documented and enforced in code review and tests (non-normative verification plan to be executed in Implementation stage 3).
- ADR-0006 drafted to govern projection selection and historical mapping before any semantic migration.

Rationale
---------
- Prevent provenance laundering where corrupted provenance is silently ignored and replaced with run-level fallback that masks tampering.
- Preserve backward compatibility for legacy evidence while ensuring documents that assert provenance do so in a structurally valid, auditable way.
- Keep provenance passive until governance and minting authority exist to avoid premature identity mutation.

Status
------
Status: Accepted

Notes
-----
This ADR specifies conceptual persistence and validation. It does not change code, tests, RFC-0001, ADR-0003 or ADR-0004. If any contradiction with existing normative RFC/ADR documents is discovered during implementation, STOP and escalate for resolution; do not silently alter accepted documents.
