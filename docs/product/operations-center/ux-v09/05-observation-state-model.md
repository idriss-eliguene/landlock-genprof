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

## Where this is enforced in code

- `activeObservationFor(items)` — finds the most recent non-terminal
  observation for the selected workload.
- `renderObservationActions()` — the single function that renders the
  lifecycle control from that state; called after every state-changing
  event (`loadSelected`, `clearResourceState`, governance actions, opening
  an observation's evidence).
- `executionLabel(raw)` — the state → label map above.

## What was verified, not fabricated

The `REQUESTED → STARTING → RUNNING → COMPLETING → COMPLETED/FAILED`
sequence and the "activity before RUNNING may precede the qualified trace
window" caveat both come directly from the domain enum and from
`test/ui/workbench-smoke.js`'s own polling comment ("the executor's RUNNING
transition is the observable completion of its source-attachment
barrier"). No additional states were invented for this document.
