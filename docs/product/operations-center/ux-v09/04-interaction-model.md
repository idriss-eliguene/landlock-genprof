# 04 — Interaction model

## Core rule: actions follow state, not the reverse

The concrete example the mission brief calls out — `[Start observation]
[Stop selected] [Generate proposal]` as three permanently-visible,
permanently-enabled buttons — was real. It has been replaced with a single
lifecycle control (`renderObservationActions()` in `workbench_ui.go`,
wired to fire after every event that can change the selected workload's
observation state: workload selection, `loadSelected`, governance actions,
and opening an observation's evidence).

### Before

```html
<button id="start-observation">Start observation</button>
<button id="stop-observation">Stop selected</button>
<button id="generate-proposal">Generate proposal</button>
```
All three always visible, always enabled, regardless of whether an
observation was running, finished, or existed at all.

### After

```html
<p id="observation-status-line">…</p>
<button id="start-observation" disabled>Start observation</button>
<button id="stop-observation" hidden class="danger-action">Stop observation</button>
<button id="generate-proposal" disabled>Generate proposal</button>
```

| Selected workload state | Status line | Start | Stop | Generate proposal |
|---|---|---|---|---|
| none selected | "Select a workload to begin." | disabled | hidden | disabled |
| no observations yet | "No observations yet for this workload." | enabled (primary) | hidden | disabled |
| cancellable observation (`REQUESTED`/`STARTING`/`RUNNING`) | "Preparing/attaching/observing…" (matching phase label) | disabled | visible, enabled, danger-styled | disabled |
| finalizing observation (`COMPLETING`) | "Finalizing evidence…" | disabled | unavailable after cancellation boundary | disabled |
| latest observation `COMPLETED`, none active | "Latest observation: completed." | enabled, labeled "Start new observation" | hidden | enabled once that observation is opened via **Review/View evidence** |

`Generate proposal`'s enablement is intentionally decoupled from evidence
quality: it is enabled whenever the currently *open* observation
(`selectedObservation`) is `COMPLETED`, whether its evidence is `AVAILABLE`,
`EMPTY`, or `UNKNOWN`. The backend is the actual eligibility authority — a
`NO_CANDIDATE` request is rejected server-side with a clear error surfaced
through the existing error banner. The UI does not duplicate that business
rule; see [06-evidence-model.md](06-evidence-model.md) for why guessing at
it client-side would be worse than deferring to the backend.

## Primary vs. secondary action hierarchy

A `.primary-action` class (solid `--primary` fill) was added to the design
system and applied to whichever single control is the correct next step for
the current state:

- **Start observation** — primary when no observation is active.
- **Review evidence** / **Generate proposal** — primary once an observation
  is `COMPLETED` (`observation-card` in `renderObservationList`).
- Everything else (Stop, per-proposal Review/Approve, Reject) keeps the
  neutral outline button style; **Stop observation** and **Reject** use
  `.danger-action` (red outline) because they are consequential/destructive
  relative to the happy path, matching existing governance styling.

## Per-observation actions vs. the lifecycle control — two paths, one honest about it

`Observation records` cards each carry their own **View
evidence**/**Review evidence** and (when facts exist) **Generate proposal**
buttons. This looks like it could be the same "static duplicate buttons"
anti-pattern called out in the brief, so it's worth being explicit about
why it isn't: the top lifecycle control always acts on *the current/latest
observation for the selected workload* (the primary, forward-looking flow).
The per-card buttons act on *whichever specific observation that card
represents* (the secondary, backward-looking/forensic flow — reviewing an
older observation, or regenerating a proposal from one that isn't the most
recent). Both are state-gated (buttons only render as active where
meaningful), and both drive the same `selectedObservation` state that the
lifecycle control reads. This is intentional dual-purpose IA, not leftover
duplication — see [15-known-limitations.md](15-known-limitations.md) if a
future pass wants to unify them further.

## Governance actions

Unchanged in this pass because they already matched the target model:
`reviewEligible`/`approveEligible`/`rejectEligible`/`applyEligible` are
computed from `approvalState` + `candidateDigest` + `currentAuthority`, each
button is gated on both **capability** (`capabilitiesLoaded && capability(name)`)
and **semantic eligibility**, and a missing `resourceVersion` universally
forces a `STALE` disabled reason. See
[12-security-ux-constraints.md](12-security-ux-constraints.md).

## Proposal generation feedback

Generation is a synchronous authenticated mutation. While the request is in
flight the primary action is disabled and the live status reads
`Generating proposal…`. On success the proposal list is reloaded from the
authoritative read model and the UI opens **Proposals & Governance**. On
failure the action is restored and the error is shown without claiming
success. There is no global “latest proposal” state.

The proposal decision surface defaults to structured Drop/Add capability
panels. Its Raw candidate-v2 JSON is progressive-disclosure forensic detail;
no YAML is synthesized because no canonical YAML serialization is persisted
for this candidate.
