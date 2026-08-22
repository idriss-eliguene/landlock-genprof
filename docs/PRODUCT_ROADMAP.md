# Product roadmap

This is the authoritative product roadmap. It records intended capability sequencing, not demonstrated release status. [Demonstrated capabilities](PROGRESS.md) remains the authority for what works today and the evidence supporting each claim.

The roadmap is capability-based, not tied to release numbers or dates. A phase advances only when its trust boundary is supported by the stated acceptance evidence.

## Product direction

landlock-genprof is a governance layer for runtime-derived Kubernetes security policy. It preserves origin, provenance, candidate identity, human authority, and backend ownership while converging policy from sources with different trust semantics.

```text
direct evidence -----------\
                           SecurityProfileProposal -> CandidateDigest -> human approval
SPO-derived policy --------/                                      -> governed apply
                                                                       -> external enforcement
                                                                       -> verification
```

The separations are permanent product constraints:

- observation is not policy; derived policy is not direct evidence;
- coverage is not confidence, and neither is authorization;
- provenance and `CandidateDigest` describe a candidate but do not authorize it;
- learned is not authorized;
- applied is not enforced; enforced is not verified.

In SPO mode, Security Profiles Operator owns syscall observation and its recording lifecycle and produces the `SeccompProfile`. landlock-genprof imports a snapshot of that object as derived policy with provenance; it does not place SPO syscalls in `TrainingHistory`, invent confidence, or build a parallel SPO-equivalent observer. The investigation in [SPO issue #3354](https://github.com/kubernetes-sigs/security-profiles-operator/issues/3354) led to the merged upstream [coverage contribution #3355](https://github.com/kubernetes-sigs/security-profiles-operator/pull/3355). Subsequent maintainer feedback confirmed that this separation makes sense from the SPO side: SPO remains focused on its profile domains while cross-domain convergence happens at the governance layer. This is architectural feedback, not project endorsement or a stability guarantee.

## Current state

The completed observation, multi-run learning, and governance milestones are product baseline, not future roadmap work:

| Capability | Current evidence-supported state | Remaining boundary |
|---|---|---|
| Direct acquisition | Filesystem, network, internal/advisory syscall, and applicable capability evidence are implemented; the demonstrated scope is recorded in [PROGRESS.md](PROGRESS.md) | Internal syscall acquisition is an alternative adapter, not the SPO architecture |
| SPO import | A real SPO-produced `SeccompProfile` can enter as a validated, provenance-bearing governed snapshot | The optional coverage annotation has no stable SPO API guarantee |
| Learning | `TrainingHistory` and cross-run confidence are implemented for direct evidence | SPO-derived syscalls are structurally excluded; coverage never becomes confidence |
| Candidate governance | `SecurityProfileProposal`, deterministic `candidate-v1` identity, review, exact-digest approval, and stale/mutated-candidate rejection are implemented and demonstrated | Reviewer rationale and assurance UX remain incomplete |
| Governed apply | Approved artifacts are planned, revalidated, applied in dependency order, and the workload is bound last after supported readiness checks | Application is sequential, non-transactional, and has no rollback |
| Enforcement and verification | NetworkPolicy denial is demonstrated on Cilium; the SPO/Seccomp path has real-node merged governance and a tested behavioral denial boundary | Evidence is candidate/syscall-specific; PodLock/Landlock kernel denial and capability verification remain unproven, and results are not portable across every backend |

The exact demonstrated scope and run identifiers live in [PROGRESS.md](PROGRESS.md). Roadmap intent never upgrades that ledger.

## Next engineering work

### Phase A — External policy and provenance boundary

**Objective:** Keep core `SeccompProfile` import independent from optional, unstable SPO metadata while preserving truthful provenance.

**Trust boundary:** SPO-derived policy must not be reclassified as landlock-genprof observation, and optional annotations must not become an accidental hard API dependency.

**Deliverables:**

- Document the exact SPO API fields, annotations, and formats consumed by the importer.
- Introduce a version-aware compatibility contract for optional syscall-coverage metadata, including absent, malformed, and unsupported forms.
- Keep valid core `SeccompProfile` enforcement content importable when optional coverage metadata is absent.
- Preserve the existing snapshot, lineage, source-selection, `TrainingHistory`, confidence, and no-live-reference boundaries.
- Decide through an ADR before changing `candidate-v1` whether normalized optional metadata remains approval-relevant provenance. Today the copied provenance annotation is part of the governed artifact and therefore part of `CandidateDigest`.

**Acceptance evidence:** focused compatibility tests cover known, absent, malformed, and unsupported metadata; an SPO interop run proves core import without the annotation; review output labels coverage without converting it to confidence; consumed metadata is documented against tested SPO versions.

**Non-goals:** treating SPO annotations as stable, parsing SPO policy as raw evidence, silently falling back to internal syscall collection, or supporting SELinux/AppArmor through this import contract.

### Phase B — Authorization integrity

**Objective:** Extend the existing exact-digest authority proof across every candidate input and compatibility path.

**Trust boundary:** Authority for one reviewed candidate must never migrate to changed direct evidence, changed derived policy, or differently interpreted provenance.

**Deliverables:**

- Preserve deterministic candidate identity and explicit approval under a versioned mechanism.
- Add reproducible adversarial scenarios for direct-evidence change after approval and SPO-policy re-import after approval.
- Maintain fail-closed rejection for missing, malformed, revoked, stale, or mismatched authority and for candidate mutation during readiness waits.
- Define migration gates before any candidate schema or digest mechanism change.

**Acceptance evidence:** experiments produce `C1/D1`, mutate direct evidence or the SPO snapshot to `C2/D2`, and prove authority for `D1` cannot authorize `C2`; digest vectors remain deterministic; apply-time and pre-binding mutation tests continue to fail closed.

**Non-goals:** per-domain approval, automatic approval, treating digest equality as human authority, or inferring reviewer intent.

### Phase C — Application versus enforcement

**Objective:** Make successful governed apply report only what each backend transition actually establishes.

**Trust boundary:** Kubernetes API acceptance and backend readiness are not equivalent to active kernel or datapath enforcement.

**Deliverables:**

- Define backend-specific states for submitted, API accepted, reconciled/ready, and behaviorally enforced where observable.
- Complete readiness semantics for supported backend paths without transferring authority from a backend to the proposal.
- Make sequential partial failure and absence of rollback explicit in status and operator guidance.
- Preserve external ownership: PodLock/Landlock for filesystem, the CNI for network, Kubernetes/container runtime for capabilities, and SPO/container runtime for seccomp.

**Acceptance evidence:** an approved multi-artifact scenario records each transition, injects a backend failure, proves later artifacts and workload binding stop, and reports earlier applied resources without calling the operation transactional or enforced.

**Non-goals:** claiming landlock-genprof is the enforcement engine, generic readiness inferred from API creation, or silent rollback claims.

### Phase D — Backend-specific verification

**Objective:** Define and demonstrate what verified means independently for each supported enforcement backend.

**Trust boundary:** Reconciled API state must not be presented as behavioral or kernel-level evidence.

**Deliverables:**

- Preserve the demonstrated SPO/Seccomp boundary: approved-policy membership, target binding, and a control-vs-governed allow/`EPERM` experiment.
- Demonstrate allowed and denied filesystem behavior in a compatible PodLock/Landlock environment.
- Retain the Cilium NetworkPolicy experiment and add an explicit portability boundary for other CNIs.
- Define capability/security-context verification evidence separately from artifact application.
- Keep Landlock ABI compatibility checks distinct from workload behavioral verification.

**Acceptance evidence:** the SPO/Seccomp boundary is recorded by real-node run [`32561123023`](https://github.com/idriss-eliguene/landlock-genprof/actions/runs/32561123023); each remaining backend experiment must record the approved digest, applied resource identity, readiness evidence where applicable, and both allowed and denied workload behavior at its enforcement layer. Results are reported per backend, never generalized from one implementation.

**Non-goals:** a universal verification claim, equating `verify` ABI diagnostics with runtime proof, or upgrading reconciliation to behavioral enforcement.

### Phase E — Assurance and adoption

**Objective:** Make the governance contract reproducible and supportable outside creator-operated environments.

**Trust boundary:** A successful demonstration in one controlled cluster is not a stable, portable product contract.

**Deliverables:**

- Reproducible adversarial E2E scenarios and compatibility matrices for Kubernetes, kernels, runtimes, CNIs, PodLock, and tested SPO versions.
- Fuzzing and security tests for import, canonicalization, approval, and apply boundaries.
- Signed artifacts, SBOMs, upgrade/migration tests, diagnostics, and installation hardening.
- Structured reviewer rationale and assurance UX without weakening exact-digest approval.
- External workload pilots with documented failure modes and evidence retention.

**Acceptance evidence:** clean-environment runs reproduce the trust-boundary experiments; supported combinations have explicit results and limitations; upgrades preserve or deliberately invalidate authority according to a tested migration contract; external users complete install, review, approve, apply, and backend-specific verification without creator intervention.

**Non-goals:** claiming universal cluster support, replacing external security review with CI, or treating adoption activity as implementation evidence.

## Longer-term direction

Controller reconciliation, drift handling, detection, and controlled response remain possible later capabilities, not the next default milestones:

- A controller may reconcile approved intent but must never approve, silently replace, or broaden a candidate.
- Drift may explain change or create a new proposal; changed content must make prior authority stale.
- Detection may use confidence-aware evidence but must preserve provenance and uncertainty.
- Response actions must have their own explicit, auditable authority and must not inherit authority from detection.

These capabilities begin only after the relevant provenance, authorization, application, verification, and assurance gates above are explicit. Release numbers remain undecided; repository history shows patch releases in the current line but no contract mapping the next maturity phase to a semantic version.

## Engineering and evidence trajectory

The same work supports product maturity and the technical thesis **“Learned Is Not Authorized: Trust Boundaries for Runtime-Derived Kubernetes Security Policy.”** The thesis must emerge from reproducible mutation, compatibility, partial-failure, and backend-verification experiments—not from product positioning. Additional engineering resources are most useful where they expand compatibility infrastructure, adversarial assurance, backend-specific verification, reviewer UX, documentation, and external pilots.

Normative boundaries remain in [ADR-0007](adr/0007-governed-apply-ordering-and-enforcement-readiness.md) and [ADR-0008](adr/0008-spo-derived-policy-import-boundary.md). Changes to those decisions require a new ADR; the roadmap cannot silently supersede them.
