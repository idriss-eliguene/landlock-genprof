# Observation workflow

| Internal state | Operator state | Meaning | Evidence/proposal |
|---|---|---|---|
| REQUESTED | Preparing | durable request exists | no result yet |
| STARTING | Attaching evidence capture | executor is establishing capture | do not trigger activity yet |
| RUNNING | Observing runtime activity | trace window is active | activity may be captured |
| COMPLETING | Finalizing evidence | stream is draining/persisting | wait |
| COMPLETED | Completed | terminal frozen observation | inspect evidence; proposal depends on evidence |
| FAILED | Failed | terminal unsuccessful execution | investigate; no fabricated proposal |

The qualified demo synchronizes activity after executor/Gadget trace readiness.
The UI presents workload, namespace, time, evidence, and fact count before
technical IDs.
