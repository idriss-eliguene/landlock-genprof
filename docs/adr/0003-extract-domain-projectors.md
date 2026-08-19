# ADR-0003: Extract Plain Per‑Domain Projectors from internal/semantic/adapter

Status: Accepted

Date: 2026-08-11

## 1. Context

- BuildGraphFromObservations currently performs domain dispatch, domain validation, Proposition construction, Act construction, ProducerRef wiring, grouping/deduplication, EvidenceGroups, AssertionEvent construction, and Graph admission for four domains: filesystem, network, capability, syscall.
- Deterministic replay, AE identity stability, and RFC alignment are critical. Tests and existing invariants must be preserved.

## 2. Problem

- A single central switch mixes per-domain projection logic with orchestration, increasing file complexity, test surface, and the risk of accidental changes to Act/producer/identity semantics.
- This makes future domain additions or small corrections error-prone and increases drift between tracer/policy/adapter logic.

## 3. Decision

- Extract plain, package-private per-domain projector functions inside internal/semantic/adapter that:
  - Own ONLY domain-specific payload validation, derivation of domain semantic values, and construction of semantic.Proposition.
  - Return a semantic.Proposition (or an error).
- Do NOT change any runtime semantics, AE identities, terms, Act construction, ProducerRef wiring, EvidenceGroups, RecordTime, or Belief algorithms.
- Keep the BuildGraphFromObservations orchestrator responsible for interval derivation, Act construction, grouping/dedup, EvidenceGroups, producer wiring, AssertionEvent construction, and Graph admission.

## 4. Architectural boundary (projector responsibilities)

A projector MUST ONLY:
- Validate the payload for its domain.
- Derive domain-specific semantic values (args record).
- Construct and return a semantic.Proposition with the same Phase/Modality/Term tokens as before.

A projector MUST NOT:
- Create Acts.
- Choose or construct ProducerRefs.
- Set AssertionEvent.RecordTime.
- Append to Graph or create AssertionEvents.
- Perform global deduplication or manipulate EvidenceGroups.
- Evaluate BeliefState.
- Orchestrate replay.

Architectural rule (enforced): projector != semantic engine != provenance engine != graph admission != evidence manager.

## 5. Formal projector model

For each domain d ∈ {filesystem, network, capability, syscall}, define a pure mapping:

P_d : Observation_d -> Proposition ∪ Error

where Observation_d denotes an observation.Observation whose Kind is already known by orchestration to belong to domain d.

Properties required for each P_d:

- Purity: P_d(o) depends only on o.

- Determinism: If o1 == o2 then

  semantic.StructuralEqual(P_d(o1), P_d(o2)) == true

  AND

  P_d(o1).CanonicalString() == P_d(o2).CanonicalString()

- No orchestration dependency: Projectors MUST NOT depend on RunMeta, RecordTime, Act, ActIdentity, ProducerRef, EvidenceGroups, Graph, BeliefState, replay state, or input position/index.

Conceptually:

```
Observation_d
     |
     v
   P_d
     |
     v
Proposition
```

NOT:

```
Observation
     |
     v
mini semantic engine
```

## 6. Dispatch authority

BuildGraphFromObservations remains the ONLY owner of domain dispatch with the mapping:

- KindFilesystem -> projectFilesystem only
- KindNetwork -> projectNetwork only
- KindCapability -> projectCapability only
- KindSyscall -> projectSyscall only
- KindOther -> existing orchestration admission/ignore behavior

A projector MUST NOT reclassify an observation. If an observation declares a supported Kind but its payload is malformed for that domain, the projector MUST return an error (not reclassification).

## 7. Invariants (identity freeze — mandatory)

For any run:

same RunMeta + same normalized observations (including identical ordering)
  => same ActIdentity
  => structurally equal Propositions (semantic.StructuralEqual == true)
  => same Proposition CanonicalString()
  => same AssertionEvent identities
  => same AssertionEvent.RecordTime
  => same EvidenceGroups
  => same BeliefState

Notes:
- "same normalized observations" includes same observation ordering because EvidenceGroups map to original input indexes. The adapter preserves EvidenceGroups mapping behavior exactly; permutations will legitimately change EvidenceGroups indexes.
- Tests must assert BOTH StructuralEqual(before, after) AND exact CanonicalString equality.

## 8. Exact unchanged vocabulary (MUST NOT CHANGE)

- FileAccess
- NetworkAccess
- CapabilityCheckObserved
- SyscallProfileAllowed

Do not rename, change case, argument structure, or introduce Minting Authority in this slice. Vocabulary constant extraction is a separate follow-up if desired.

## 9. Alternatives considered

A. Keep monolithic switch — no refactor (low risk, but maintenance pain).
B. Extract package-private projector functions — chosen (low-risk, testable, minimal).
C. Projector interface/registry — supports plugins, dynamic extensions; premature complexity.
D. Type Observation first — highest correctness but large migration risk.

Why B now: provides clear, immediate maintainability and testability gains with minimal migration risk and zero semantic change. C/D remain possible later; not invalidated.

## 10. Consequences

Positives:
- Smaller, testable domain units.
- Reduced central-switch complexity.
- Easier to add domain unit tests and hostile cases.
- Localizes future domain-specific changes.

Negatives (explicitly NOT fixed by this ADR):
- Flat Observation ambiguity remains.
- Tracer/policy/adapter duplicated classification logic remains.
- No change to Act/provenance; backend provenance still unresolved.
- No Minting Authority/RFC change.

## 11. Identity / replay safety strategy

- Add a safety-net test commit (Commit 1) that freezes golden outputs for representative fixtures:
  - filesystem-only, network-only, capability-only, syscall-only (zero timestamps), mixed four-domain, and v2 semantic replay fixtures.
- Each golden contains a TEST-ONLY ProjectionSnapshot (not a new semantic serialization):
  - ActIdentity
  - AssertionSnapshots[]
  - EvidenceGroups mapping

Where AssertionSnapshot contains:
- AssertionEventIdentity
- PropositionCanonicalString
- RecordTime
- BeliefStatus

The test harness must verify:
1. ActIdentity equality
2. Proposition Semantic.StructuralEqual(before, after) == true
3. Proposition CanonicalString equality
4. AssertionEventIdentity equality
5. AssertionEvent.RecordTime equality
6. EvidenceGroups equality
7. BeliefState equality

Any deviation fails the gate.

## 12. Implementation sequence (two required commits)

**Commit 1 — identity/replay safety net (must precede refactor):**
- test(semantic): freeze adapter projection identities
- Add fixtures & tests that compute and serialize canonical identity snapshot for each fixture; store golden fixtures under testdata for read-only comparison.
- Tests assert snapshot equality.

**Commit 2 — representation-only refactor:**
- refactor(semantic): extract domain projectors
- Create package-private functions:
  - projectFilesystem(obs observation.Observation) (semantic.Proposition, error)
  - projectNetwork(obs observation.Observation) (semantic.Proposition, error)
  - projectCapability(obs observation.Observation) (semantic.Proposition, error)
  - projectSyscall(obs observation.Observation) (semantic.Proposition, error)
- Replace per-domain branches in adapter.go with calls to these projectors; orchestrator captures errors and handles KindOther admission/ignore decisions as before.
- Ensure the exact content of constructed Propositions (term tokens, arg keys & literal values) is byte-for-byte identical in terms of CanonicalString and StructuralEqual semantics.

**Optional later commit:**
- refactor(semantic): centralize adapter vocabulary (internal/semantic/adapter/vocab.go) — separate PR only after identity tests pass.

## 13. Exact approved function signatures (must be used)

func projectFilesystem(obs observation.Observation) (semantic.Proposition, error)
func projectNetwork(obs observation.Observation) (semantic.Proposition, error)
func projectCapability(obs observation.Observation) (semantic.Proposition, error)
func projectSyscall(obs observation.Observation) (semantic.Proposition, error)

Important: projectors must return an error on invalid payloads for their declared Kind; admission/ignore for KindOther remains the orchestrator's responsibility. Do NOT use a (Proposition,bool,error) signature.

## 14. Proposed file layout

internal/semantic/adapter/
  - adapter.go                 // orchestration unchanged (reduced)
  - projector_filesystem.go
  - projector_network.go
  - projector_capability.go
  - projector_syscall.go
  - (optional later) vocab.go

All new files are package-private and reuse existing imports.

## 15. Testing strategy (exact)

- Baseline golden capture tests (Commit 1).
- Per-extraction-step regression checks: run golden compare after extracting each projector to pin regressions early.
- Hostile test coverage (duplicate counts, zero-timestamp syscalls, interval derivation).
- Full-suite validation: unit tests, race tests, go vet, go build.
- CI gating: the golden compare step must block merges.

## 16. YAGNI / rejected abstractions (explicit)

Do NOT add in this slice:
- SemanticProjector interface/registry
- plugin mechanism
- visitor pattern/dynamic dispatch
- typed Observation IR
- persisted semantic intermediate representation
- Act/provenance changes

Rationale: premature complexity & migration risk; refactor goal is limited and representation-only.

## 17. Known architecture debts NOT addressed here

- Act meaning and hierarchy
- Producer / backend provenance representation
- Syscall epistemic origin (testimony vs runtime observation)
- Flat observation typing and Syscall field overload
- Classification drift across tracer/policy/adapter
- Vocabulary / Minting Authority and RFC-0002 alignment
- Backend-specific Acts

These debts must be addressed in separate ADRs.

## 18. Relationship to RFC-0001 / RFC-0002

- This ADR is an implementation architecture decision only.
- It does NOT alter or reinterpret RFC-0001 or RFC-0002.
- If extraction requires any change that would affect semantic identity or RFC-mandated behavior, STOP and raise a blocker (do not implement).

## 19. Follow-up architecture reviews

- After extractor refactor and identity regression success:
  - Plan separate ADRs for (a) provenance/Act model, (b) vocabulary/minting authority, (c) typed Observation if warranted.
- Each follow-up must include migration strategy and golden-replay safety.

## 20. Implementation gate (must pass before merge)

- Commit 1 (identity freeze) merged.
- Commit 2 runs:
  - All golden comparisons pass (StructuralEqual + CanonicalString + AE identity + EvidenceGroups + BeliefState).
  - All unit/race tests and vet/build succeed.
  - Code review approves that projectors only contain domain-specific logic and no producer/Act/RecordTime/Graph append changes.
  - No change to Term tokens or Proposition args.

## 21. Emergency stop condition

- If any refactor step changes Proposition canonical-string, AE identities, or other identity invariants, immediately abort and revert — do not attempt silent migration.

## 22. Implementation commit guidance

- Single focused refactor commit for projectors:
  - Subject: refactor(semantic): extract domain projectors
- Separate commit for vocabulary extraction (optional):
  - Subject: refactor(semantic): centralize adapter vocabulary

## 23. Documentation & developer notes

- Tests should use semantic.StructuralEqual to assert proposition equality and also compare CanonicalString to protect indexing invariants.
- Golden fixtures should be compact, clearly labeled, and stored under internal/semantic/adapter/testdata/golden/ with git tracking.

## 24. Status & next steps

- ADR Status: Accepted
- Do not implement until Commit 1 is created and merged per the two-commit gate.
- Circulate ADR for review with architecture group.
