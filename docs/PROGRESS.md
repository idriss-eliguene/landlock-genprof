# Demonstrated Project Progress

This is the canonical source for demonstrated current project progress. It
is not the product roadmap: roadmap intent, sequencing, gates, and future
scope belong to [`PRODUCT_ROADMAP.md`](PRODUCT_ROADMAP.md).

Normative semantic contracts belong to accepted [`rfc/`](rfc/) documents;
durable architectural decisions belong to [`adr/`](adr/) documents; current
and target architecture belongs to [`architecture.md`](architecture.md).
Evidence references support status but do not redefine architecture.
Historical roadmap and E2E documents retain their original context and are
not current progress authority without verification against implementation
and tests.

The ledger distinguishes **CURRENTLY IMPLEMENTED** from **TARGET / INTENDED**.
The strategic SPO boundary is a product decision; it does not claim that an
external SPO adapter or external enforcement is implemented.

## Status vocabulary

Only these statuses are valid here:

`DONE` · `IN PROGRESS` · `NOT STARTED` · `BLOCKED` · `DEFERRED` ·
`SUPERSEDED` · `UNKNOWN`

`BLOCKED` means a dependency or decision prevents progress. Intentional
postponement is `DEFERRED`; insufficient proof is an evidence gap attached
to the relevant capability, not an invented capability status.

## Definition of Done

`DONE` never means only “code exists”. The evidence threshold is capability
specific:

- **Pure semantic/library:** implementation, focused tests, relevant
  invariant/conformance tests, and scope documentation.
- **CLI behavior:** production command path, behavior tests, and build/test
  validation.
- **Kubernetes integration:** implementation, focused unit/fake-client tests,
  schema/RBAC validation where applicable, and authoritative E2E when the
  claim depends on real cluster behavior.
- **Approval/authority:** production implementation, positive and fail-closed
  negative tests, mutation/adversarial tests where relevant, and persisted
  schema/runtime alignment.
- **External enforcement:** the external operator/mechanism is installed, the
  artifact is applied, and behavioral enforcement is demonstrated on the
  actual target. Artifact generation or API persistence alone is not
  external enforcement.
- **Documentation:** the canonical source is updated, claims are consistent,
  and relevant links resolve.

Every entry follows:

`CLAIM → IMPLEMENTATION → TEST → CI/E2E/AUDIT → STATUS → REMAINING GAP → NEXT GATE`

## Current capability ledger

| Capability | Status | Implemented scope | Evidence | Remaining scope / gap | Blocker or dependency | Next gate |
|---|---|---|---|---|---|---|
| Semantic runtime foundation | IN PROGRESS | Semantic graph, identity, assertion, derivation, and belief primitives exist | [`rfc/0001-semantic-foundation.md`](rfc/0001-semantic-foundation.md), `internal/semantic/` tests (focused suite passes) | Full product-wide semantic assurance path is not demonstrated | Integration scope | Demonstrate a product-path run from evidence through semantic assurance |
| Vocabulary/evolution implementation | IN PROGRESS | Term identity, structural equality, and related runtime primitives exist | [`rfc/0002-vocabulary-declaration-and-semantic-evolution.md`](rfc/0002-vocabulary-declaration-and-semantic-evolution.md) (draft), `internal/semantic/` tests | RFC-0002 remains a draft; complete implementation/conformance coverage is absent | Draft contract | Define and run conformance tests for the implemented subset |
| Evidence model | DONE | Evidence types, serialization, and validation are implemented | `internal/evidence/`, focused tests | External source adapters remain separate work | None for current model | Preserve model/schema tests |
| Trace acquisition adapter | DONE | Internal tracer can acquire workload evidence | `internal/tracer/`, tracer tests, historical live-run evidence | Tracer is an acquisition adapter, not the strategic product boundary | None for adapter scope | Preserve adapter while external sources are evaluated |
| Strategic external-observation boundary decision | DONE | Observation is upstream; interpretation and assurance are this product’s strategic responsibility | [`product-definition-v1.md`](product-definition-v1.md), accepted product direction | Current wording and diagrams still require later alignment | Phase 4 documentation work | Record the boundary consistently in current architecture/product documentation |
| Boundary documentation alignment | IN PROGRESS | Authority markers distinguish the boundary at a high level | `docs/product-definition-v1.md`, `docs/architecture.md`, this ledger | Existing diagrams and wording still require Phase 4 reconciliation | Documentation reconciliation | Update current wording without claiming external ingestion |
| External evidence ingestion adapter | DEFERRED | No demonstrated SPO import adapter exists in the current path | [`landlock-kernel-extraction.md`](landlock-kernel-extraction.md) Phase 5; [`cli-design.md`](cli-design.md) says import is deliberately deferred | Supported external inputs and adapter contract are not implemented | Explicit product/adapter deferral | Reopen only with approved adapter scope and conformance plan |
| Candidate generation | DONE | Evidence is synthesized into security-profile candidates/artifacts | `internal/landlock/`, exporters, focused tests, E2E artifact evidence | Broader semantic candidate classes remain future scope | None for current classes | Preserve generator invariants |
| SecurityProfileProposal persistence | DONE | Proposal spec/status are persisted through the proposal store | `internal/proposal/store.go`, proposal tests, root/Helm CRDs, `internal/proposal/crd_schema_test.go` | Broader schema evolution is future work | None for current contract | Keep the schema/runtime parity test green |
| Candidate deterministic digest | DONE | Canonical selected spec fields produce `sha256:<hex>` identity | `internal/proposal/digest.go`, proposal tests, E2E run `32037183484` | Expand coverage if spec fields evolve | Schema evolution | Add digest vectors when fields change |
| Digest-bound approval mechanics | DONE | Approve/reject, persisted state, digest binding, mechanism version, fail-closed validation, and adversarial protections exist | `internal/proposal/store.go`, `internal/proposal/validate.go`, proposal/apply tests, E2E approval evidence | Complete reviewer UX and structured rationale are separate scope | None for current mechanics | Preserve positive, negative, and adversarial approval tests |
| ApprovedCandidateDigest persistence | DONE | Approved digest is persisted in status | `internal/proposal/types.go`, `store.go`, root/Helm CRDs, `internal/proposal/crd_schema_test.go` | Future schema changes require renewed parity evidence | Schema/runtime maintenance | Keep the schema/runtime parity test green |
| ApprovalMechanismVersion persistence | DONE | `candidate-v1` mechanism version is persisted | `internal/proposal/store.go`, root/Helm CRDs, `internal/proposal/crd_schema_test.go`, E2E run `32037183484` | Future migration policy is undefined | Future versioning decision | Record any mechanism change in an ADR |
| Fail-closed approval validation | DONE | Missing/invalid/mismatched approval digest is rejected | `internal/proposal/validate.go`, negative tests, E2E wrong-digest rejection | Continue coverage for new approval states | None for current states | Add a negative test for each new approval state |
| Candidate mutation rejection | DONE | Spec mutation between planning and revalidation is rejected | `cmd/landlock-genprof/apply_proposal_test.go` adversarial tests (current-worktree suite passes) | Authoritative CI publication is absent | CI evidence | Publish an authoritative run ID |
| Approval revocation rejection | DONE | Revocation before revalidation is rejected | `cmd/landlock-genprof/apply_proposal_test.go` adversarial test (current-worktree suite passes) | Authoritative CI publication is absent | CI evidence | Publish an authoritative run ID |
| Approval digest replacement rejection | DONE | Replacing the approved digest before revalidation is rejected | `cmd/landlock-genprof/apply_proposal_test.go` adversarial test (current-worktree suite passes) | Authoritative CI publication is absent | CI evidence | Publish an authoritative run ID |
| Pre-apply revalidation | DONE | Proposal is reloaded and digest-validated before apply | `cmd/landlock-genprof/apply_proposal.go`, apply tests | Broader cluster-race evidence remains future hardening | Runtime/E2E coverage | Demonstrate the revalidation gate in an authoritative run |
| Planned payload continuity | DONE | Immutable planned payload prevents later proposal substitution | `cmd/landlock-genprof/apply_proposal.go`, payload mutation test | External operator behavior is separate | External enforcement | Preserve plan/apply separation in E2E evidence |
| Official CLI apply authority | DONE | `kubectl landlock-genprof apply-proposal` is approval-bound | `cmd/landlock-genprof/apply_proposal.go`, CLI tests, B-REL-001 workflow documentation, E2E apply evidence | Broader documentation reconciliation remains outside this release-preparation step | Documentation follow-on | Preserve the governed workflow and its help/documentation contract |
| Makefile apply authority continuity | DONE | `make apply-proposal` delegates to the authoritative CLI path | Current `Makefile`; current-worktree `Makefile_test.go` passes | Current implementation/test are uncommitted WIP | WIP review | Preserve the delegation test in the committed baseline |
| Non-authoritative export behavior | DONE | Export is informational and not the apply authority | `Makefile`, CLI export path, B-REL-001 documentation | Historical examples and broader documentation phases remain separate | Documentation follow-on | Preserve explicit non-authoritative warnings |
| Landlock/PodLock artifact generation | DONE | Landlock/PodLock artifacts can be rendered | `internal/landlock/`, exporters, focused tests, E2E artifact evidence | Generation is not enforcement | External operator | Use the separate API/enforcement gates below |
| LandlockProfile approval-bound/API application | DONE | Approved LandlockProfile artifacts are planned and submitted through the governed path; Kubernetes API application behavior is tested | `cmd/landlock-genprof/apply_proposal.go`, `internal/k8s/apply.go`, fake-client tests, E2E proposal/apply scope | PodLock operator consumption and kernel enforcement are not demonstrated | External operator/runtime | Install a compatible PodLock CRD/operator and run the behavioral gate |
| Landlock application semantic-continuity assurance | DONE | Planned LandlockProfile semantics survive both apply-path parsing boundaries and persist unchanged through the Kubernetes API object | `internal/k8s/apply_test.go`, `cmd/landlock-genprof/apply_proposal_test.go` semantic-preservation tests; full and race suites pass | PodLock operator consumption and kernel enforcement remain outside this assurance claim | External operator/runtime | Preserve semantic-continuity tests when the Landlock schema changes |
| PodLock behavioral enforcement | BLOCKED | Artifact generation and API path exist; operator/kernel behavior is not demonstrated | [`enforcement-prerequisites.md`](enforcement-prerequisites.md), E2E environment lacked required external CRD/operator | No behavioral denial caused by generated LandlockProfile | External cluster/operator dependency | Install a compatible PodLock operator/CRD and demonstrate behavioral denial |
| NetworkPolicy generation/proposal inclusion | DONE | NetworkPolicy is generated and included in the proposal | `internal/exporter/networkpolicy/`, exporter tests, E2E run `32037183484` | No claim beyond artifact/proposal scope | None for current scope | Preserve generation/proposal tests |
| NetworkPolicy approval-bound/API application | DONE | Approved proposal path applies NetworkPolicy and persists it through the Kubernetes API | `cmd/landlock-genprof/apply_proposal.go`, `internal/k8s/apply.go`, fake-client tests, E2E run `32037183484` | API persistence is not generic behavioral enforcement | Kubernetes/CNI environment | Keep API-persistence evidence linked to the apply path |
| NetworkPolicy Cilium behavioral enforcement | DONE | Fresh connections were denied in the demonstrated Cilium scenario | E2E run `32037183484`, HEAD `902f99228203a27aeb52da11f301760d8bc5ff60`, smoke-networkpolicy evidence | Does not generalize to all CNIs, workloads, PodLock, or SPO | Cilium-specific evidence | Repeat only when implementation or scenario changes |
| SPO SeccompProfile generation/API path | DONE | SPO-shaped SeccompProfile generation, proposal representation, and API plumbing exist | `internal/exporter/spo/`, exporter tests, `internal/k8s/apply.go` fake-client tests | External SPO reconciliation is not demonstrated | External CRD/operator | Apply with compatible SPO installation and record API evidence |
| SPO Seccomp behavioral enforcement | BLOCKED | No external operator reconciliation or syscall denial is demonstrated | Historical E2E environment lacked required SPO operator/CRD | Behavioral seccomp proof is absent | External CRD/operator | Run with compatible SPO reconciliation and demonstrate syscall denial |
| Complete reviewer UX/rationale | IN PROGRESS | Approval mechanics expose status/digest information | `cmd/landlock-genprof/approve.go`, review/approval tests | Complete reviewer experience and structured rationale are not demonstrated | Product/UX scope; Phase 3 docs | Define reviewer criteria and demonstrate the workflow |
| Explain CLI | DONE | `explain` renders rights, ABI, confidence, counts, and evidence | `cmd/landlock-genprof/explain.go`, `explain_test.go` (focused suite passes) | Broader semantic assurance remains separate | None for CLI scope | Preserve command behavior tests |
| Candidate diff CLI | DONE | `diff` compares candidate rules and supports text/JUnit output | `cmd/landlock-genprof/diff.go`, `diff_test.go` (focused suite passes) | Broader semantic assurance remains separate | None for CLI scope | Preserve exit-code/output contract tests |
| Broader semantic assurance/explanation model | IN PROGRESS | Explanation and diff primitives exist | `internal/explain/`, `explain.go`, `diff.go`, focused tests | Product-level semantic rationale/diff contract is incomplete | Product scope | Define and test the candidate-level assurance contract |
| Demonstration scope | DONE | One-workload live demonstration scope is recorded | [`e2e-demo.md`](e2e-demo.md), E2E run `32037183484` | Demonstration is not formal pilot acceptance | Historical/demo scope | Preserve run context and scope boundaries |
| Formal pilot acceptance | NOT STARTED | No formal pilot gate or acceptance record exists | No authoritative pilot artifact | Acceptance criteria and evidence gates are absent | Pilot decision | Define explicit pilot criteria and publish acceptance evidence |
| Product documentation canonicalization | IN PROGRESS | Phase 1 authority markers and this Phase 2 ledger are established | This file and authority markers in canonical docs | Phases 3–7 remain | Governance follow-on phases | Review this ledger before Phase 3 |
| Operator/controller reconciliation | NOT STARTED | No demonstrated controller reconciliation capability | Roadmap future scope; no authoritative implementation/E2E evidence | Product scope and controller contract are undefined | Product decision | Establish the controller contract before implementation |

## Supporting evidence and historical security notes

### Authoritative E2E evidence

Run `32037183484` at HEAD
`902f99228203a27aeb52da11f301760d8bc5ff60` supports only the capabilities
listed against it above: proposal generation, digest-bound approval/apply,
and the demonstrated Cilium NetworkPolicy behavioral scenario. It does not
prove PodLock or SPO external enforcement.

### Current race-suite evidence gap

Focused adversarial tests are present and pass in the current worktree, but
an authoritative published race-suite run is not recorded. This is an
evidence gap attached to approval/apply continuity, not a separate product
capability or an `UNKNOWN` capability status.

### Superseded Makefile security path

The former export-then-`kubectl apply` path is retained as a historical
security note. The current Makefile delegates governed application to the
approval-bound CLI path; the former bypass is not an active capability.

## Strategic boundary snapshot

### CURRENTLY IMPLEMENTED

- The existing internal tracer remains an acquisition adapter.
- Evidence interpretation, candidate construction, digest identity,
  approval, and controlled application are implemented for the demonstrated
  scope.
- Landlock/PodLock artifacts and approval-bound/API application plumbing are
  implemented; external operator and kernel enforcement are not demonstrated.

### TARGET / INTENDED

- SPO and other external sources provide upstream observation/evidence.
- `landlock-genprof` owns interpretation and policy assurance.
- PodLock/Kubernetes and other mechanisms provide downstream enforcement.

The strategic boundary decision does not establish external ingestion or
external enforcement evidence.

## Documentation governance invariants

- **INV-DOC-001:** Each current factual question has one canonical source.
- **INV-DOC-002:** Roadmap intent is not presented as implemented state.
- **INV-DOC-003:** `DONE` requires capability-appropriate evidence.
- **INV-DOC-004:** Accepted RFCs and ADRs remain historically accurate and
  are not silently rewritten.
- **INV-DOC-005:** README summarizes canonical sources and does not
  independently redefine architecture, roadmap, or status.
- **INV-DOC-006:** Evidence acquisition is distinct from evidence
  interpretation and policy assurance.
- **INV-DOC-007:** External enforcement is not claimed without external
  behavioral evidence.
- **INV-DOC-008:** `BLOCKED` means dependency/decision prevents progress;
  intentional postponement is `DEFERRED`.
- **INV-DOC-009:** Every non-terminal capability identifies remaining scope
  and a next gate.
- **INV-DOC-010:** Every `DONE` claim references implementation plus
  appropriate verification evidence.
- **INV-DOC-011:** Historical evidence retains its original run context.
- **INV-DOC-012:** Published/generated documentation mirrors are not
  independent authorities.

## Next verification gate

Review this calibrated ledger, then reconcile current workflow documentation
(Phase 3). Do not treat roadmap content, historical E2E reports, artifact
generation, or Kubernetes API persistence as proof of external PodLock/SPO
enforcement.
