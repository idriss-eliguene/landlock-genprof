# Observation and Evidence Semantics

v0.7 is an **Observation Workbench** within the broader product position of
Evidence-driven Least-Privilege Governance for Kubernetes. The Workbench
exposes durable workload observations, bounded evidence, uncertainty, and
candidate-v2 proposals. It does not turn observation into approval,
application, enforcement, or verification.

## Identity

`ObservationID` is opaque and immutable. The Observation specification is
immutable, its binding is written once, target changes are append-only, and
the execution/result records become frozen at terminal completion.

`ClusterIdentity` is the UID of the `kube-system` Namespace. A
`ClusterLocator` is display/audit information only. A WorkloadIdentity is:

```text
ClusterIdentity + namespace + GroupKind + name + workload UID
```

`ContainerSlot` adds the container name. `ContainerImageRevision` adds the
image digest. This makes the Observation workload-UID-bound.

Candidate-v2 intentionally has weaker persisted subject identity: Scope,
Target, Container, and ImageIdentity, without workload UID. The UI must not
describe a candidate-v2 Proposal as workload-UID-bound.

## State axes

Execution, attribution, and evidence are independent:

| Axis | Values |
|---|---|
| Execution | `REQUESTED`, `STARTING`, `RUNNING`, `COMPLETING`, `COMPLETED`, `FAILED` |
| Attribution | `NOT_STARTED`, `IN_PROGRESS`, `COMPLETED`, `FAILED` |
| Evidence | `EMPTY`, `AVAILABLE`, `UNKNOWN` |

UNKNOWN is not an execution state, and Proposal eligibility is derived rather
than persisted as a lifecycle state.

## Evidence

Attribution is bounded to the selected container. The sources are filesystem,
exec, network connect, network bind, and capabilities. `AVAILABLE` means
attributable evidence exists within the qualified scope. `EMPTY` means no
attributable evidence was observed within that scope. `UNKNOWN` means the
source-level qualification is insufficient for a complete EMPTY/AVAILABLE
conclusion.

UNKNOWN preserves positive facts. EMPTY does not mean the workload never did
something, and AVAILABLE does not mean complete workload behavior was
observed. Exec is provenance-only for candidate-v2 TrainingHistory policy
derivation.

## Populations and proposals

The legacy BINARY population includes BinaryPath. The v0.7 CONTAINER
population does not. There is no scope fallback.

The contribution key is ObservationID plus PopulationFingerprint, with a
PREPARED-to-COMMITTED receipt. The supported guarantee is an **idempotent
contribution effect**, not exactly-once execution.

Candidate-v2 uses Scope `CONTAINER`, Target, Container, and ImageIdentity. Its
artifact is `CONTAINER_CAPABILITIES`, with Drop `ALL` and canonical observed
CAP_* facts in Add. CandidateDigestV2 identifies candidate content;
ReviewContextDigestV2 identifies review/provenance/qualification context.
Provenance is not silently added to the candidate digest.

## Governance boundary

Generated is not approved; approved is not applied; applied is not enforced;
enforced is not behavior verified. Approval is Proposal-object-scoped and
content mutation makes approval stale. LastApprovalSnapshot is last recorded
approval custody, not complete history.

The browser is read-only. Approve, Reject, Revoke, Apply, and Rollback remain
CLI-only where supported.

These semantics do not claim complete workload behavior, complete least
privilege, global enforcement verification, fleet governance, or a full
Security Operating Center.
