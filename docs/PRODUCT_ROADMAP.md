# Product roadmap

This roadmap records intended capability sequencing, not demonstrated release status. See [Demonstrated capabilities](PROGRESS.md) for what works today and the evidence supporting each claim.

The roadmap is capability-based rather than calendar-based. A milestone advances only when its behavioral gate is demonstrated.

## Direction

```text
observe → derive → propose → review → approve → apply → enforce → verify
                                                            ↓
                                            detect drift → governed response
```

The permanent product boundary is **learned ≠ authorized**. Evidence sources may improve, but imported or generated policy never gains authority without review and digest-bound human approval.

## Capability sequence

| Milestone | Capability | State | Exit gate |
|---|---|---|---|
| Observation foundation | Observation and generation | Achieved | Real evidence and generated artifacts |
| Multi-run learning | Multi-run learning and confidence | Achieved | Persisted history and cross-run confidence |
| Governance | Governed proposal and approval | Achieved | Digest-bound approval and fail-closed apply |
| Enforcement verification | Enforcement and verification | In progress | Behavioral verification across supported backends |
| Kubernetes controller | Continuous reconciliation | Planned | Reconciliation and observable status contract |
| Drift | Drift and continuous learning | Planned | Baseline comparison and governed retraining |
| Detection | Runtime anomaly detection | Planned | Confidence-aware anomaly scoring |
| Controlled response | Governed response | Planned | Auditable, policy-governed response actions |
| Productization | Operational maturity | Planned | Packaging, signed artifacts, upgrades, and external-user success |
| Stable contract | Stable product contract | Planned | Compatibility guarantees and reproducible acceptance |

## Active milestone: enforcement and verification

The active gate is **Enforcement verification**. NetworkPolicy has been behaviorally verified in the demonstrated Cilium scenario. SPO reconciliation and workload binding are proven, but a syscall denial is not. PodLock/Landlock behavioral enforcement is not yet demonstrated.

Required work:

1. Add a seccomp scenario that proves a syscall outside the approved profile is denied.
2. Add a compatible PodLock/Kubewarden environment and prove a generated Landlock policy changes workload behavior.
3. Preserve separate generated, applied, enforced, and verified status for each backend.

The project should not imply controller capability while these enforcement-verification claims remain open.

## Future milestones

### Kubernetes controller

Continuously reconcile approved intent without changing the existing authority model. Reconciliation must expose status and must not approve or silently replace candidates.

### Drift and continuous learning

Compare new evidence with the approved baseline. Drift may trigger explanation or a new proposal; it must make existing approval stale when candidate content changes.

### Detection

Add confidence-aware anomaly detection while retaining evidence provenance and uncertainty.

### Controlled response

Support auditable responses such as warn, propose, deny, or quarantine. Detection alone must not silently grant response authority.

### Productization

Provide installable packaging, SBOMs, signed releases, diagnostics, documentation, and an upgrade path that preserves proposal and approval semantics.

### Stable product contract

Publish stable CLI, CRD, evidence, history, approval, and migration guarantees backed by reproducible E2E and external-user acceptance.

## Scope discipline

- SPO is the real upstream source of derived seccomp policy in SPO mode; landlock-genprof governs that policy but does not claim SPO's observation as its own.
- Filesystem and network evidence remain attributed to landlock-genprof's tracer unless another explicit source contract is introduced.
- External systems—SPO, PodLock, and the CNI—own enforcement behavior.
- Roadmap intent never upgrades a capability on the [progress page](PROGRESS.md); only evidence does.
