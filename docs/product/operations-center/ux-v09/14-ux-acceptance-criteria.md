# 14 — UX acceptance criteria

## Observation Stop acceptance

- [x] Authenticated Stop is a durable request, not a process-local HTTP
      cancellation handle.
- [x] The UI distinguishes accepted Stop intent from completed/frozen state.
- [x] Duplicate Stop does not create a second intent or replay cancellation.
- [x] Backend authorization, environment/namespace binding and status CAS
      remain authoritative.

This document was first written after an initial implementation pass that
self-assessed `UX_ACCEPTANCE=MET`. A human review of the live Operations
Center rejected that assessment: it found a visible, unqualified error
banner, an internally inconsistent observation state machine (`Generate
proposal` active while "Observing runtime activity…" was also shown), and
a real control collision (`WorkloadDeployment/api · api`). All three were
real defects, not review artifacts — see
[15-known-limitations.md](15-known-limitations.md) for the incident
writeup on the two bugs the first review missed and this pass found and
fixed. This table reflects the **corrected and re-verified** state, with
the prior false-positive rows marked explicitly rather than quietly
overwritten.

| Criterion | Status | Evidence |
|---|---|---|
| Cluster → Identity → Namespace → Workload → Observation → Evidence → Proposal → Governance → History reads as one coherent journey | Met | [02](02-user-journeys.md), [03](03-information-architecture.md) |
| Evidence UNKNOWN never visually confused with successful evidence | Met | [06](06-evidence-model.md); live-captured `05-observation-completed-unknown.png` |
| Observation lifecycle is not represented as unrelated buttons | Met, corrected | First pass claimed this; live review found `Generate proposal` active during `OBSERVING` due to a stale `selectedObservation` reference. Fixed and re-verified live: `03-observation-active.png` shows it disabled. See [05](05-observation-state-model.md) |
| Actions follow state | Met, corrected | Same defect and fix as above — the gate now also requires no active Observation for the workload, not just that some previously-opened Observation was `COMPLETED` |
| Workload identity dominates UUID identity | Met | `observationTitle`/`observationSubtitle`; UUIDs behind "Technical observation details" |
| Security details remain inspectable | Met | `<details>` disclosures throughout; nothing was removed, only reordered |
| No UI control collisions | Met, corrected | First pass claimed this after fixing one missing CSS rule but missed a second, worse one: the Workload label and `<select>` had no wrapper/spacing at all and rendered glued together. Restructured with `.form-field` (label above control); re-verified in `02-observation-ready.png` at 1440 and `observations-1024.png` at 1024 |
| Generic/unqualified error banners never reach the operator | Met, new | The live review's core complaint ("Authoritative read/action failed. The authoritative state was not changed.") is traced to its root cause (silent namespace mis-binding — see [15](15-known-limitations.md)) and fixed at the source; `showError()` additionally hardened so it can no longer render with zero specific content even for a non-JSON error response |
| No credential leakage | Met (unchanged) | [12](12-security-ux-constraints.md) |
| No fake authorization model | Met (unchanged) | [12](12-security-ux-constraints.md), [01](01-personas.md) |
| No fake evidence | Met (unchanged/reinforced) | [06](06-evidence-model.md) |
| No fabricated metrics | Met (unchanged) | [09](09-page-specifications.md#overview) |
| Functional surfaces remain reachable after IA changes | Met | Governance actions still one click away, under Proposals & Governance; [03](03-information-architecture.md) |
| Proposal decision object is primary before governance action | Met, re-verified | Live `proposals-governance-{1440,1280,1024}.png`: workload target, structured candidate capability policy, aggregate evidence/provenance, truthful no-baseline statement, Apply effect, and secondary technical details |
| Governed candidate identity remains inspectable and bound | Met | Candidate digest and resourceVersion remain in Technical metadata; the projection test verifies the displayed candidate artifact is the persisted candidate-v2 artifact |
| Real screenshots captured, not mockups | Met | Captured via `hack/ui-lima-demo.sh` against a live kind cluster, across two separate correction-round runs; [screenshots/README.md](screenshots/README.md) |
| Focused tests pass | Met | `go test ./cmd/landlock-genprof/...` full package passes; one unrelated concurrent test confirmed pre-existing/flaky in isolation (passes 3/3 alone), not a regression from this diff |
| Backend/security semantics unchanged unless explicitly flagged | Met | [12](12-security-ux-constraints.md); client-side bug fixes and one additive, non-behavioral error-classification fix explicitly called out; one deeper architectural finding (trusted-proxy namespace scoping) explicitly flagged rather than silently resolved |

## What "re-verified live" actually means here

Not a claim that the first pass's screenshots were reused. Both the
namespace-binding fix and the `Generate proposal`/state-machine fix were
verified against **fresh, independent live runs** of `hack/ui-lima-demo.sh`
after the code changes, including deliberately starting a *second*
Observation while a *first, completed* Observation was still open for
review in the same browser session — the exact sequence that exposed the
original defect — and confirming `Generate proposal` stayed disabled
throughout via both a scripted assertion (Playwright) and a visual
screenshot.

## Explicitly not claimed

- Not claiming pixel-perfect visual polish across every state — the visual
  review loop covered the representative states listed in
  [screenshots/README.md](screenshots/README.md), not an exhaustive matrix.
- Not claiming a full design-token spacing scale was introduced — see
  [07-design-system.md](07-design-system.md).
- Not claiming automated accessibility scanning was run — see
  [10-accessibility.md](10-accessibility.md).
- Not claiming the brief `STARTING` transient state or a clean
  `AVAILABLE`-evidence run were captured live — `RUNNING`/`OBSERVING` and
  `COMPLETED` with `UNKNOWN` evidence were both reproduced end-to-end
  against a real cluster. See [15-known-limitations.md](15-known-limitations.md).
- Not claiming the trusted-proxy/production namespace-scoping question
  (documented in [15](15-known-limitations.md)) is resolved — it is
  deliberately left as an open, flagged architectural question rather than
  silently decided either way within this pass.
