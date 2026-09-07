# v0.7 Observation and evidence semantics

v0.7 is an Observation Workbench for an evidence-driven governance path. It
does not claim complete workload behavior, complete least privilege, or
backend enforcement merely because evidence or a proposal exists.

## Product boundary

The product position is **Evidence-driven Least-Privilege Governance for
Kubernetes**. The v0.7 product surface is the workload-centric Observation
Workbench:

```text
Workload → Observe / Ingest → Attribute → Derive → Govern → Apply → Verify
```

The browser exposes durable Observation and Proposal state, but approval,
rejection, revocation, application, and rollback remain outside the browser.

## Identity

An `ObservationID` is opaque and immutable. `ObservationSpec` is immutable;
the binding is written once when the claim is made. Target change events are
append-only. Execution is mutable until terminal, while the result is
accumulated and then frozen. Provenance is derived from the frozen binding and
result.

`ClusterIdentity` is the UID of the cluster's `kube-system` Namespace.
`ClusterLocator` is display/audit context, not authority. A `WorkloadIdentity`
contains the ClusterIdentity, namespace, GroupKind, name, and workload UID.
Workload revision is a separate concept.

`ContainerSlot` is a WorkloadIdentity plus a container name. A
`ContainerImageRevision` is a ContainerSlot plus an image digest.

The Observation is therefore workload-UID-bound. Candidate-v2 deliberately is
not: its Proposal subject contains only Scope, Target, Container, and
ImageIdentity.

## Independent state axes

These axes must not be collapsed into one status:

| Axis | Values |
|---|---|
| ExecutionState | `REQUESTED`, `STARTING`, `RUNNING`, `COMPLETING`, `COMPLETED`, `FAILED` |
| AttributionState | `NOT_STARTED`, `IN_PROGRESS`, `COMPLETED`, `FAILED` |
| EvidenceState | `EMPTY`, `AVAILABLE`, `UNKNOWN` |

`UNKNOWN` is an EvidenceState, not an ExecutionState. Proposal eligibility is
derived from the persisted Observation; it is not a lifecycle state stored in
the Observation.

## Evidence and uncertainty

v0.7 uses bounded container-scoped attribution for filesystem, exec, network
connect, network bind, and capability sources. `AVAILABLE` means attributable
evidence exists within the qualified scope. `EMPTY` means no attributable
evidence was observed within that scope. `UNKNOWN` means qualification is
insufficient to make an EMPTY or AVAILABLE source-level conclusion complete.

UNKNOWN does not erase positive facts. EMPTY does not mean that the workload
never performed the behavior. AVAILABLE does not mean that all workload
behavior was observed. Exec facts are retained for provenance and are not
TrainingHistory policy input for candidate-v2 container capability derivation.

The central distinctions are:

```text
observed  ≠ complete
generated ≠ correct
approved  ≠ applied
applied   ≠ enforced
enforced  ≠ behavior verified
```

## Population and contribution

The legacy `BINARY` population is identified by Target, Container,
ImageIdentity, and BinaryPath. The v0.7 `CONTAINER` population is identified
by Target, Container, and ImageIdentity; it has no BinaryPath. There is no
implicit fallback or cross-scope confidence.

The contribution key is `ObservationID + PopulationFingerprint`. Receipt
progression is `PREPARED → COMMITTED`. The certified property is an
**idempotent contribution effect**, not exactly-once execution. Filesystem,
network, and capability facts may contribute according to the qualified model;
exec remains provenance-only.

## Candidate-v2 and governance

Candidate-v1 remains frozen. Candidate-v2 has:

- Subject scope `CONTAINER`;
- Target, Container, and ImageIdentity;
- artifact `CONTAINER_CAPABILITIES`;
- Drop `ALL`;
- canonical observed `CAP_*` facts in Add.

CandidateDigestV2 binds candidate content. ReviewContextDigestV2 binds the
review/provenance/qualification context. Provenance is metadata and is not
silently inserted into CandidateDigestV2. Zero eligible capability facts do
not fabricate a candidate; UNKNOWN plus positive capability facts may still
produce one.

Approval is Proposal-object-scoped. A matching digest on another Proposal
does not transfer authority. Relevant Proposal mutation makes prior approval
stale. `LastApprovalSnapshot` is custody of the last recorded approval, not a
complete approval history.

Backend claims remain bounded: PodLock plus application-derived Seccomp
runtime compatibility is unproven and the composition is refused fail-closed;
Landlock kernel denial and capability enforcement are unproven; NetworkPolicy
behavioral denial is limited to the qualified Cilium scope; and SPO is a
derived-policy source, not a landlock-genprof observation source.
