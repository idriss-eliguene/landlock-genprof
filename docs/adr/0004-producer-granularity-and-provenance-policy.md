# ADR-0004: Producer Granularity and Provenance Policy

Status: Accepted
Date: 2026-08-11
Author: landlock-genprof architecture

## Context

This ADR records a project-level policy for how landlock-genprof maps runtime evidence to semantic producers (the `producer(e)` that appears in AssertionEvent identity per RFC-0001). It does not change RFC-0001 or RFC-0002, does not modify existing semantic identities, and does not alter code or tests. It documents the staged hybrid model chosen by the project and the gates required before any identity-changing migration.

## Current reality (explicit)

For each non-empty semantic build:

```
BuildGraphFromObservations(meta, events) -> one ActContact (A_r)
producer(e) = run-level Act A_r for every AssertionEvent e produced by that build
```

This behavior remains authoritative and unchanged by ADR-0004.

## Provenance taxonomy (definitions)

1. Acquisition provenance
   - Which landlock-genprof acquisition run collected/processed the evidence.
   - Represented today by RunMeta: Source, RecordTime, Start/End.

2. Evidence provenance
   - Which collector/backend/raw mechanism produced a particular observation.
   - May be incomplete or absent in current persisted evidence.

3. Epistemic provenance
   - Which RFC Source + Act epistemically warrants the AssertionEvent.
   - This determines `producer(e)` and participates in AssertionEvent identity.

Important: acquisition provenance != evidence provenance != epistemic provenance, unless explicitly established.

## Decision (staged HYBRID policy)

ADOPT a staged hybrid policy:

- CURRENT STAGE: Existing projections retain the run-level Contact Act as the producer. No semantic identities are changed by this ADR.

- RATIONALE: persisted evidence does not yet include stable, replayable backend SubjectIdentity tokens sufficient to construct truthful backend-level epistemic producers.

- FUTURE STAGE: When persisted evidence includes sufficient, stable epistemic provenance (and minting authority is defined), landlock-genprof MAY represent distinct backend/adviser Sources and Acts; at that point distinct producers asserting the same Proposition will produce distinct AssertionEvents.

- SPECIAL CASE: adviser-derived or inferred syscall/profile outputs must be distinguishable from direct Contact observations before any ActKind refinement for syscalls is applied.

## Producer rules (P1–P9)

P1. Every AssertionEvent has exactly one producer Act or Commitment (RFC-0001). (RFC-derived)

P2. Operational backend components MUST NOT automatically become RFC Subjects. (project rule)

P3. A backend MAY become a Source only when it represents a genuine epistemic agent with a stable identity. (project rule)

P4. Backend SubjectIdentity MUST NOT be manufactured merely to create ActIdentity entropy. (project rule)

P5. Uses MUST NOT encode backend identity. (RFC-derived / project rule)

P6. Outputs MUST NOT be relied upon to distinguish ActIdentity. (RFC-derived)

P7. Until backend identity is truthfully available in persisted evidence, landlock-genprof retains the run-level producer model for existing projections. (future migration rule)

P8. Raw evidence provenance and semantic producer provenance are distinct concepts. (project rule)

P9. If two backend epistemic agents eventually become distinct Sources, the same Proposition asserted by those different producers yields distinct AssertionEvents per existing identity rules. (RFC-derived / project rule)

## Claim vs AssertionEvent (example)

Let P = NetworkAccess(443, egress).

- If producer A asserts P and producer B asserts P and A != B then there are two AssertionEvents (same Proposition structure but distinct AEs because producer identity differs).

- If a single producer emits 100 observations supporting P, current grouping yields one Proposition, one AssertionEvent, and EvidenceGroups recording the 100 occurrences.

## Act identity limit (reminder)

ActIdentity = ⟨Source, ActKind, Interval, Uses⟩ (RFC §8.3.1). Consequence: creating separate Act objects per backend does NOT by itself create distinct ActIdentity unless one of these components differs truthfully. Outputs are NOT part of ActIdentity.

## Source-as-backend gate

A backend may be elevated to an RFC Source only after satisfying all of:

1. It is a genuine epistemic agent.
2. Its identity has semantic meaning for consumers.
3. Its identity is stable and persistable for replay.
4. Its identity can be deterministically reconstructed from persisted evidence.
5. Its identity minting authority is defined.

Do not treat internal software module names as SubjectIdentity without passing this gate.

## Syscall epistemic distinction (important)

- DIRECT OBSERVATION: syscall events observed at runtime are candidate Contact provenance.
- ADVISER / DERIVED RESULT: seccomp/profile advisories or inferred allowed syscalls are candidate Inference or Testimony provenance depending on mechanism.

Current implementation may treat both under run-level Contact; ADR-0004 requires that adviser-derived assertions be distinguishable in evidence before any producer/ActKind refinement is applied.

## Modality vs ActKind

Proposition.Modality (Actual/Dispositional/Deontic) is orthogonal to an Act's Kind. An Inference or Testimony Act MAY produce an Actual Proposition where RFC permits. Refinement of ActKind need not change Proposition.Modality automatically.

## Replay contract (REPLAY-A)

Adopt REPLAY-A as project guarantee:

```
same persisted evidence + same semantic projection semantics/version
  => same ActIdentity, same Proposition identity, same AssertionEvent identity, same EvidenceGroups, same BeliefState
```

Do NOT promise identical semantic identities across future semantic-model changes.

## Identity-change gate

Before shipping any change that can alter ActIdentity or AssertionEventIdentity (including Source mapping, producer mapping, Act granularity, ActKind mappings that affect identity, Proposition vocabulary or term authority semantics), the project MUST first define semantic projection versioning and a replay/migration strategy. Conceptual candidate: `semanticProjectionVersion` (ADR does not choose wire format here).

## Current identity version

Do NOT label the current projection with an implicit numeric version in this ADR. Refer to the current projection as "current semantic projection semantics" until a separate versioning ADR is adopted.

## Projector boundary (ADR-0003 preserved)

Domain projectors remain pure: `Observation -> Proposition` and are limited to domain validation, deterministic normalization, and Proposition construction. They MUST NOT decide Source, Act, ProducerRef, RecordTime, backend SubjectIdentity, minting authority, projection version, or Graph admission.

## Minting authority

If backend identities are introduced in the future, establish minting authority BEFORE allowing any component to emit SubjectIdentity tokens. Projectors MUST NOT mint Subjects.

## Countermodels (illustrative)

- CASE A: Same producer + same Proposition + 100 observations -> 1 Proposition, 1 AE, EvidenceGroups length 100.
- CASE B: Two distinct epistemic producers assert same Proposition -> same Proposition structure, 2 AssertionEvents.
- CASE C: Two implementation modules process same observation but are not epistemic agents -> MUST NOT create two semantic Sources just to force distinct AEs.
- CASE D: Adviser-derived SyscallProfileAllowed(openat) -> epistemic provenance differs from direct Contact observation.
- CASE E: Two backend Acts with identical Source, Kind, Interval, Uses -> same ActIdentity (collision).
- CASE F: Cross-version replay after producer-model migration -> identity may differ unless historical semantics are selected via projection versioning.

## Alternatives considered and rejected

A. Permanent single run-level producer (rejected: loses provenance required for future trust/conflict/federation).

B. Immediate one-Act-per-backend migration (rejected: evidence lacks stable backend identity; would change semantic identities without versioning).

C. Encoding backend IDs in Uses (rejected: semantically invalid).

D. Distinguishing Acts through outputs (rejected: outputs are not part of ActIdentity).

E. Projectors minting Subjects (rejected: violates projector purity and authority control).

Chosen: Staged hybrid policy (this ADR).

## Consequences

Positive:
- preserves current identities and golden tests;
- avoids creating fake Subjects;
- establishes explicit provenance boundaries and upgrade path;
- places a hard gate before identity-changing migrations.

Negative:
- backend epistemic provenance remains unavailable for existing projections;
- same-claim assertions from independent collectors may collapse today;
- syscall adviser vs direct observation provenance must be captured before accurate reclassification.

## Follow-up gates and roadmap

GATE 1 — provenance capture audit (read-only): determine what backend/direct-vs-derived info exists today.
GATE 2 — evidence provenance design: define minimal persisted fields (backend kind, stable id, acquisition method, direct-vs-derived flag).
GATE 3 — semantic projection versioning ADR: design replay/version contract before identity changes.
GATE 4 — backend Source / hybrid Act design: only after provenance+versioning closed.
GATE 5 — implementation & migration: only after prior gates and migration plan in place.

## Feature-work rule

Allow feature work that does NOT alter frozen semantic identities (representation refactors, exporters, policy behaviors that do not change identities, tests/docs). Any change touching Source, ProducerRef, Act granularity, ActKind identity, or Proposition identity requires passing the identity-change gate first.

## Validation checklist

Before acceptance verify:
- ADR does not contradict RFC-0001 or RFC-0002.
- ADR preserves current golden identities and semantics.
- ADR does not require code/test/evidence/RFC modifications now.

## Status

Status: Proposed. After hostile acceptance review and approval by owners, mark as Accepted and publish.

---

*End of ADR-0004: Producer Granularity and Provenance Policy.*
