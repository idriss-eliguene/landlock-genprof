# 08 — Component inventory

Components as actually implemented (HTML/CSS classes + JS renderers), not
an aspirational component library. File references are to
`cmd/landlock-genprof/workbench.go` (markup/CSS) and
`cmd/landlock-genprof/workbench_ui.go` (renderers).

| Component | Markup/class | Renderer | Notes |
|---|---|---|---|
| App shell | `.app-shell`, `.sidebar`, `.topbar`, `.content` | static | Fixed sidebar nav + sticky topbar |
| Environment panel | `.environment-panel`, `.environment-controls`, `.context-chip` | `renderOperationsContext`, `loadEnvironmentContexts` | Always-visible; see [09](09-page-specifications.md#environment-panel) |
| View | `.view[hidden]` | `showView(name)` | One of `overview/workloads/observations/proposals/history/attention` |
| Summary card | `.card.summary-card` | `card()` | Used on Overview |
| Status badge | `.status-badge`, `.outcome-badge` | `badge()`, `outcome()` | See [07](07-design-system.md#status-badges) |
| Data table | `.data-table`, `.table-wrap` | `renderWorkloads`, `renderHistory` | Horizontal scroll, not column-squeeze |
| Workload row | `.workload-row` | `renderWorkloads` | `.selected` state on Inspect |
| Observation lifecycle control | `.observation-lifecycle`, `.lifecycle-status`, `.action-group` | `renderObservationActions` | New in this pass — see [04](04-interaction-model.md) |
| Observation card | `.card.observation-card`, `.observation-status-line` | `renderObservationList` | Workload-first title/subtitle, evidence-state badge, forensic `<details>` |
| Evidence source panel | `.evidence-source`, `.evidence-explanation`, `.fact-list` | `renderEvidenceSource` | Per-source AVAILABLE/EMPTY/UNKNOWN, capability fact list |
| Proposal card | `.card.proposal-row` | `renderProposalList` | State badge, attribution line, technical-metadata disclosure |
| Governance action group | `.action-group`, `.disabled-reason`, `.danger-action` | `proposalActions` | Capability + semantic-eligibility gated; see [12](12-security-ux-constraints.md) |
| Attention item | `.attention-item` | `appendAttentionItem`, `diagnosticsToAttention` | 4-col → 2-col → 1-col responsive grid |
| History table | `.history-table`, `tr.discontinuity` | `renderHistory` | Distinguishes timestamped events from unordered facts |
| Notice / error banner | `.notice`, `.error-state`, `.stale-state` | `showError`, `clearMessage` | Distinct styling for stale/409 vs. generic failure |
| Empty/unavailable/degraded state | `.empty-state`/`.unavailable-state`/`.degraded-state` | inline in each renderer | Never interchanged — see [07](07-design-system.md#empty--unavailable--degraded--stale-states) |
| Progressive disclosure | `<details>`/`<summary>` | inline | "Environment details," "Technical observation details," "Evidence details," "Technical metadata" |
| Primary action | `.primary-action` | applied conditionally | New in this pass |

## Components removed this pass

- `#environment-view` section and its per-subject card list — orphaned,
  never shown. See [03](03-information-architecture.md) and
  [13](13-implementation-map.md).
- `#governance-view` section and `#open-proposals` shortcut button —
  consolidated into Proposals & Governance. See
  [03](03-information-architecture.md).
- Duplicate/dead first definitions of `renderOperationsContext`,
  `loadEnvironmentContexts`, `renderObservation`, `renderObservationList` —
  shadowed by later declarations of the same name and never executed. See
  [13](13-implementation-map.md).
