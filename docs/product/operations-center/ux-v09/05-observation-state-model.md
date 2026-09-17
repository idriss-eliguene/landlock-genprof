# 05 — Observation state model

## Backend states → operator labels

Source of truth: `internal/observation/domain/observation.go`,
`ExecutionState` enum. Mapping lives in `executionLabel()`,
`workbench_ui.go`.

| Backend `ExecutionState` | Operator label | Meaning | What's safe to do |
|---|---|---|---|
| `REQUESTED` | Preparing | Durable request persisted, executor hasn't claimed it | Wait |
| `STARTING` | Attaching evidence capture | Executor is establishing the trace/capture attachment | **Do not** rely on workload activity yet — it may precede the qualified capture window |
| `RUNNING` | Observing runtime activity | Capture is attached; workload activity is capturable | Workload activity now produces evidence |
| `COMPLETING` | Finalizing evidence | Stream draining/persisting | Wait |
| `COMPLETED` | Completed | Terminal, frozen (`Frozen: true`) | Inspect evidence; proposal generation becomes reachable |
| `FAILED` | Failed | Terminal, unsuccessful | Investigate; never fabricate a proposal from it |

`REQUESTED`, `STARTING`, `RUNNING`, `COMPLETING` are the **non-terminal**
set (`workbenchNonTerminalStates` in `workbench_ui.go`) that the lifecycle
control uses to decide whether **Stop observation** should be offered.

## UI state machine (what the operator sees)

```
 no workload selected
        │  select workload
        ▼
 READY (no active observation)
   primary: Start observation
        │  start
        ▼
 STARTING / RUNNING / COMPLETING   ──┐
   status: phase label               │ Stop observation (danger, secondary)
   Start: disabled                   │
   Stop: visible, enabled            │
   Generate proposal: disabled,      │  (see "Generate proposal" rule below)
   unconditionally, regardless       │
   of any previously-opened          │
   completed Observation             │
        │  terminal                  │
        ▼                            │
 COMPLETED  ◄─────────────────────────┘  (stop forces early completion)
   status: "Latest observation: completed."
   primary: Start new observation
   per-card: Review evidence (primary) → Generate proposal (enabled once open)
        │
        ▼
 FAILED
   status via executionLabel: "Failed"
   no evidence/proposal actions implied
```

This mapping is deliberately **not** the same thing as the evidence state
machine — see [06-evidence-model.md](06-evidence-model.md). `COMPLETED`
only means the observation lifecycle reached its terminal, frozen state; it
says nothing about whether evidence is usable.

## The "Generate proposal" rule (fixed after an incorrect first pass)

`Generate proposal` (top lifecycle control) is enabled if and only if
**both**:

1. `selectedObservation` (the Observation currently opened for review) is
   `COMPLETED`, **and**
2. `selectedObservation` actually belongs to the currently selected
   workload's own list of Observations (`selectedData.observations`) —
   i.e. it was not left over from a workload the operator previously
   inspected.

An earlier version of this pass only checked condition 1. That meant: if an
operator opened a `COMPLETED` Observation for review, then started a *new*
Observation for the same (or a different) workload, `Generate proposal`
stayed enabled/primary the whole time the new Observation was `RUNNING` —
because the stale `selectedObservation` reference was still `COMPLETED`.
This was caught live during a mandatory review pass (the state card showed
"Observing runtime activity…" with `Generate proposal` simultaneously
active) and is exactly the kind of internal inconsistency this document
exists to prevent. It is now additionally, unconditionally forced closed
whenever the workload has an active (non-terminal) Observation — see the
diagram above — so it cannot be true regardless of what `selectedObservation`
still points at. The backend independently enforces the same rule (see
`internal/proposalapp/generate.go`: `if !observation.Frozen() ||
observation.Execution().State != ExecutionCompleted { return error }`);
the client-side gate exists so the operator never sees the option offered
in a state where it cannot possibly succeed, not merely so it fails safely
if clicked.

## Where this is enforced in code

- `activeObservationFor(items)` — finds the most recent non-terminal
  observation for the selected workload.
- `renderObservationActions()` — the single function that renders the
  lifecycle control from that state; called after every state-changing
  event (`loadSelected`, `clearResourceState`, governance actions, opening
  an observation's evidence, switching workloads).
- `executionLabel(raw)` — the state → label map above.
- Switching the selected workload (`picker.onchange`, the Workloads table's
  **Inspect** button) now explicitly clears `selectedObservation` first, so
  a stale reference from a previously-inspected workload can never leak
  into the new workload's action gating.

## What was verified, not fabricated

The `REQUESTED → STARTING → RUNNING → COMPLETING → COMPLETED/FAILED`
sequence and the "activity before RUNNING may precede the qualified trace
window" caveat both come directly from the domain enum and from
`test/ui/workbench-smoke.js`'s own polling comment ("the executor's RUNNING
transition is the observable completion of its source-attachment
barrier"). No additional states were invented for this document.
