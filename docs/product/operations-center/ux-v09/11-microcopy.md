# 11 — Microcopy

Principle: translate backend truth into operator language in the primary
position; keep the exact backend token available in a secondary/technical
position. Never conceal the technical truth — relabel it.

## Changes made this pass

| Before | After | Why |
|---|---|---|
| "Stop selected" | "Stop observation" | Named the action's object explicitly; "selected" required the reader to already know what was selected. See [10-accessibility.md](10-accessibility.md). |
| "Select to inspect" (table cell) | "Inspect to view" | Named the actual row control instead of a vague instruction. |
| "Rebinding…" (environment status) | "Rebinding environment…" | Said what was being rebound; bare ellipsis text reads as a stall, not a described action. |
| Observations view: "Observations / Evidence" | "Observations" | The subtitle ("Start capturing real runtime behavior, then review the evidence it produces.") already carries the Evidence framing; the heading didn't need to duplicate it. |
| Proposals view: "Proposals / Governance" | "Proposals & Governance" | Matches the merged nav label; "&" reads as one concept, "/" read as two separate pages (which, before this pass, it literally was). |
| Degraded-observations notice: "malformed observations remain excluded from decisions." | "malformed Observations remain visible in Attention for investigation but are excluded from these decisions." | Restored a fact the shorter version dropped: malformed items aren't hidden, they're still inspectable in Attention — just not usable as governance evidence. |
| *(no prior UI text — silent gap)* | "Behavioral verification: UNKNOWN — No enforcement/runtime behavioral verification is recorded for this Observation; evidence above describes capture, not enforcement." | Named a real absence instead of leaving it unstated. See [06-evidence-model.md](06-evidence-model.md). |

## Already-correct patterns kept as-is

These match the mission brief's intent and were left unchanged:

- `evidenceExplanation()` — full sentences for `AVAILABLE`/`EMPTY`/`UNKNOWN`
  instead of raw enum tokens as primary copy.
- `stateCopy()` / `attentionCopy()` — dictionaries translating dozens of
  backend codes (`STALE_APPROVAL`, `NOT_PROJECTABLE_AMBIGUOUS_GOVERNANCE`,
  `NEW_CONTRIBUTION_SINCE_CANDIDATE`, ...) into full sentences, with the raw
  code always still available via `technical()` in a disclosure.
- Disabled-reason strings on governance actions
  (`NOT_SEMANTICALLY_ELIGIBLE — Approve requires Reviewed state and a
  candidate digest.`) — specific and actionable, not a generic "unavailable."
- `showError`'s stale/409 copy: *"State changed since this decision was
  loaded. The previous decision was not applied. Refresh and decide again
  against the current state."* — explicit about what did and didn't
  happen, which matters a great deal for a security decision.

## Explicitly not softened

Raw technical truth is never hidden, only relabeled with the token kept
visible nearby: `UNKNOWN`, `NOT_AVAILABLE`, observation IDs, candidate
digests, resourceVersions all remain literally on screen (via
`technical()`/`<code>`), in `<details>` blocks, for the security reviewer
persona that needs them. See [12-security-ux-constraints.md](12-security-ux-constraints.md).
