# Observation lifecycle

Operations Center treats an Observation as an immutable record with separate
execution, attribution, and evidence dimensions. The persisted execution
states are `REQUESTED`, `STARTING`, `RUNNING`, `COMPLETING`, `COMPLETED`, and
`FAILED`; terminal states are frozen.

```text
REQUESTED -> STARTING -> RUNNING -> COMPLETING -> COMPLETED
                 |          |           |
                 +----------+-----------+-- durable Stop intent
                            |              (executor-owned finalization)
                            +-----------> COMPLETING
STARTING/RUNNING/COMPLETING ----------------> FAILED (backend failure)
expired active claim ------------------------> FAILED (EXECUTOR_LOST)
```

`STARTING` includes claim, target binding, and Gadget attachment. A durable
Stop intent is accepted for every non-terminal cancellable state, including
`REQUESTED`, `STARTING`, `RUNNING`, and `COMPLETING`. The API records intent;
the executor holding the live `executorID` and `claimGeneration` cancels its
local Runner, drains sources, persists results that actually exist, and then
freezes the record with completion reason `STOPPED_BY_REQUEST`. Stop never
fabricates execution, events, facts, evidence, candidates, or proposals.

The request is authenticated against the selected immutable environment
session, namespace, cluster identity, context version, and Kubernetes
RBAC/SSAR capability. The Observation ID alone is not authority. Persistence
uses resourceVersion CAS. Duplicate requests are idempotent; stale claims
cannot consume or finalize another claim's intent.

## Independent dimensions

Execution completion describes the lifecycle outcome. Attribution records
whether runtime events could be assigned to a bound target. Evidence
qualification additionally requires the source/backend, attached qualified
window, flush confirmation, and attribution proof. Thus attribution
`COMPLETED` does not imply evidence `AVAILABLE`, and `frozen=true` does not
imply success. Early cancellation before a qualified window leaves evidence
unknown when no source result can establish a stronger state.

Proposal generation remains governed by the existing authoritative candidate
and evidence checks; a stopped or failed record is not automatically eligible.

## Failure semantics

Attachment timeout/failure and executor lease loss persist a bounded failure
diagnostic under execution: stage, code, reason, source, timestamp, retryable
flag, executor ID, and claim generation. The diagnostic is explanatory data,
not an authorization decision. Failure, attribution, evidence, and fact count
remain independently readable. A failed terminal record is immutable; the
operator may inspect it or start a new Observation when the persisted
diagnostic says retry is appropriate.

## Restart and loss

Operations Center restart does not erase a persisted Stop intent. The active
fenced executor remains the only technical cancellation authority. If its
lease expires first, the existing recovery path records `FAILED` with
`EXECUTOR_LOST`; it does not claim successful user cancellation or fabricate
evidence. A replacement claim must satisfy the existing executor fencing and
lease rules.

## UI control contract

| Execution state | Operator status | Start | Stop | Proposal |
| --- | --- | --- | --- | --- |
| REQUESTED/STARTING | Preparing/attaching | disabled | enabled while non-terminal | disabled |
| RUNNING | Observing runtime activity | disabled | enabled | disabled |
| COMPLETING | Finalizing evidence | disabled | idempotent intent while live | disabled |
| COMPLETED | Completed; evidence is shown separately | new observation as allowed | unavailable | backend eligibility only |
| FAILED | Observation failed with stage/reason | new observation as allowed | unavailable | backend eligibility only |

The UI projects `stopEligible` from the same domain predicate used by the
durable store. It polls authoritative state after acceptance and shows
stopping/finalizing without declaring completion optimistically. Technical
IDs, claim data, and raw state remain secondary forensic details.
