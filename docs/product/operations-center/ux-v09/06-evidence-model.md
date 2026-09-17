# 06 — Evidence model

This is the most security-sensitive document in this set. Get this wrong
and the UI can visually launder an unproven claim into an apparent fact.

## Three axes, not one

The mission brief's example is real and was reproduced live during this
convergence pass (see [screenshots/README.md](screenshots/README.md)):

```
Observation ID  b030aa17ee746c6cd382b467d29f6e2a (captured run)
Execution:      COMPLETED       — observation lifecycle is terminal
Frozen:         true            — immutable
Attribution:    COMPLETED       — attribution reached a terminal state
Evidence:       UNKNOWN         — 6 capability facts exist, but qualification
                                   proof is insufficient to call this "available"
```

These three facts are independent and the UI must never collapse them into
one badge:

1. **Observation completion** (`ExecutionState`) — did the capture run to
   completion?
2. **Attribution completion** (`AttributionState`) — did the backend finish
   attributing captured events to this workload?
3. **Evidence qualification** (`EvidenceState`) — can the backend actually
   prove the captured facts are complete and trustworthy enough to act on?

## Evidence state derivation (ground truth)

`internal/observation/domain/observation.go`, `DeriveEvidenceState`:

```go
func DeriveEvidenceState(q SourceQualification) EvidenceState {
    if !q.BackendHealthConfirmed || !q.SourceAttachedForBoundWindow ||
       !q.FlushConfirmed || q.Attribution != AttributionCompleted ||
       q.ExcludedCount > 0 {
        return EvidenceUnknown
    }
    if q.AttributedCount > 0 {
        return EvidenceAvailable
    }
    return EvidenceEmpty
}
```

`UNKNOWN` is the default whenever **any** proof condition is unmet — backend
health, full-window source attachment, flush confirmation, completed
attribution, or zero excluded events. Facts can exist (`AttributedCount >
0`, and capability facts can be non-empty, as in the captured example)
while evidence is still `UNKNOWN`, because "facts were captured" and "we
can prove the capture was complete and clean" are different claims.

## How the UI keeps these distinct

| Backend fact | Where it's shown | How it's worded |
|---|---|---|
| `Frozen: true` | Observation detail, status line | "Observation finalized" — a statement about the record being immutable, nothing else |
| `execution.State` | Badge + `executionLabel()` | "Completed" / "Failed" / phase label — lifecycle only |
| `source.attributionState` | Forensic `<details>` and evidence-source panel | Raw state token, technical disclosure |
| `source.evidenceState` | Its own badge, per evidence source, with plain-language explanation (`evidenceExplanation()`) | "Evidence captured" / "No attributable evidence" / **"Evidence state unknown"** |
| behavioral/enforcement verification | Forensic `<details>` (added in this pass) | Always literally `UNKNOWN`, because the backend records none — see below |

`evidenceExplanation()` (`workbench_ui.go`) gives `UNKNOWN` this exact
sentence: *"The observation is finalized, but one or more proof conditions
for evidence availability cannot be established."* This is deliberately
specific rather than a generic "unknown" — it tells the reviewer the
finalization and the evidence proof are different things without them
having to read source code.

## Fail-closed rules this UI follows

- `UNKNOWN` never uses `.success`/green styling. `stateClass()`/`badge()`
  map only `HEALTHY`/`SUCCESS`/`AVAILABLE`/`APPLIED` to success; everything
  else (including `UNKNOWN`) falls through to warning/failed/unknown
  treatment.
- Capability-fact counts are shown (`6 capability facts`) because they are
  a true fact about what was captured; they are shown **next to**, never
  **instead of**, the evidence-state badge, so a nonzero fact count never
  reads as "evidence qualified."
- **Generate proposal** stays available even when evidence is `UNKNOWN`
  (see [04-interaction-model.md](04-interaction-model.md)) — the UI does
  not pretend to know in advance that the backend will reject it, but it
  also never claims the evidence supports it. If the backend does reject
  it, the rejection ships through the same error banner as any other
  authoritative-read failure.
- **Behavioral verification** — whether the proposed policy was ever
  actually enforced and observed to hold — has no backend source at all
  today. This pass added an explicit, permanent disclosure for it
  (`Behavioral verification: UNKNOWN`, with the sentence "No
  enforcement/runtime behavioral verification is recorded for this
  Observation; evidence above describes capture, not enforcement.") in the
  observation's forensic details, rather than leaving it unsaid. Silence
  here would have been indistinguishable from "verified and fine."

## What this document is not claiming

It is not claiming evidence UNKNOWN is rare or an edge case — in the one
live capture taken during this pass, it's exactly what happened on a normal
run. Treat it as an expected, first-class state to design and test for, not
an exceptional path.
