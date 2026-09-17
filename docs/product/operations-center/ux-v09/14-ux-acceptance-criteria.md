# 14 — UX acceptance criteria

Self-assessment against the mission's acceptance standard, each line
pointing at the evidence for the claim.

| Criterion | Status | Evidence |
|---|---|---|
| Cluster → Identity → Namespace → Workload → Observation → Evidence → Proposal → Governance → History reads as one coherent journey | Met | [02](02-user-journeys.md), [03](03-information-architecture.md) |
| Evidence UNKNOWN never visually confused with successful evidence | Met | [06](06-evidence-model.md); live-captured screenshot, [screenshots/README.md](screenshots/README.md) |
| Observation lifecycle is not represented as unrelated buttons | Met | [04](04-interaction-model.md), [05](05-observation-state-model.md) |
| Actions follow state | Met | [04](04-interaction-model.md) |
| Workload identity dominates UUID identity | Met (was already largely true pre-pass) | `observationTitle`/`observationSubtitle`; UUIDs behind "Technical observation details" |
| Security details remain inspectable | Met | `<details>` disclosures throughout; nothing was removed, only reordered |
| No UI control collisions | Met, with one caveat noted | Fixed the missing `.observation-status-line` CSS rule found live; see [15](15-known-limitations.md) for the one un-investigated case (a sticky-header artifact in full-page screenshot capture, not a live rendering bug — see [screenshots/README.md](screenshots/README.md)) |
| No credential leakage | Met (unchanged) | [12](12-security-ux-constraints.md) |
| No fake authorization model | Met (unchanged) | [12](12-security-ux-constraints.md), [01](01-personas.md) |
| No fake evidence | Met (unchanged/reinforced) | [06](06-evidence-model.md) |
| No fabricated metrics | Met (unchanged) | [09](09-page-specifications.md#overview) |
| Functional surfaces remain reachable after IA changes | Met | Governance actions still one click away, under Proposals & Governance; [03](03-information-architecture.md) |
| Real screenshots captured, not mockups | Met | Captured via `hack/ui-lima-demo.sh` against a live kind cluster; [screenshots/README.md](screenshots/README.md) |
| Focused tests pass | Met | `go test ./cmd/landlock-genprof/...` full package (not just a name filter) passes after this pass's changes |
| Backend/security semantics unchanged unless explicitly flagged | Met | [12](12-security-ux-constraints.md); one client-side bug fix explicitly called out, no backend semantic changes |

## Explicitly not claimed

- Not claiming pixel-perfect visual polish across every state — the visual
  review loop covered the representative states listed in
  [screenshots/README.md](screenshots/README.md), not an exhaustive matrix.
- Not claiming a full design-token spacing scale was introduced — see
  [07-design-system.md](07-design-system.md).
- Not claiming automated accessibility scanning was run — see
  [10-accessibility.md](10-accessibility.md).
- Not claiming the `RUNNING`/`STARTING` transient states or a clean
  `AVAILABLE`-evidence run were captured live in this session — only
  `COMPLETED` with `UNKNOWN` evidence was reproduced end-to-end against a
  real cluster during this pass. See [15-known-limitations.md](15-known-limitations.md).
