# landlock-genprof Product Roadmap

> **Canonical authority:** This document is the canonical Product Roadmap for
> intended capabilities, sequencing, gates, dependencies, and future scope.
> Current demonstrated implementation status is recorded separately in
> [`PROGRESS.md`](PROGRESS.md). Existing milestone and evidence content below
> is preserved during this governance phase.
>
> Release note: the milestone labels below preserve roadmap sequencing. The
> current v0.2.0 release contract and demonstrated release status are owned by
> `PROGRESS.md`, not by these historical version labels.

## Product vision

Observe -> Evidence -> Learn -> TrainingHistory -> Synthesize -> Confidence -> Propose -> Review -> Approve -> Apply -> Enforce -> Verify -> Drift -> Detect -> Respond

This roadmap is capability-based, not calendar-based. Versions advance when evidence proves milestone gates. The document is the authoritative, versioned product roadmap and living record of milestone status and evidence.

---

| Version | Capability | Status | Gate |
|---------|------------|--------|------|
| v0.1 | Observation & Generation | ACHIEVED | tracer events, Evidence v2, >=1 artifact generated, tests green |
| v0.2 | Multi-run learning & confidence | ACHIEVED | TrainingHistory persisted, multi-run SeenInRuns semantics |
| v0.3 | Governed proposal & approval | ACHIEVED | proposal/approval flow validated end-to-end |
| v0.4 | Enforcement & verification | IN_PROGRESS | generated -> applied -> enforced -> verified across artifacts |
| v0.5 | Kubernetes operator | PLANNED | controller reconciliation + status model |
| v0.6 | Drift / continuous learning | PLANNED | baseline/alerting for divergent behavior |
| v0.7 | Detection | PLANNED | anomaly engine + scoring |
| v0.8 | Controlled response | PLANNED | governed actions + audit/warn/apply capabilities |
| v0.9 | Productization | PLANNED | packaging, docs, SBOM, signed releases, upgrade path |
| v1.0 | Stable product contract | PLANNED | reproducible E2E, external-user success, compatibility guarantees |

---

Milestone states (canonical): PLANNED, IN_PROGRESS, ACHIEVED, BLOCKED

Each version entry below follows this template:

Status:
Product promise:
Functional scope:
Milestone gates:
Evidence required:
Current evidence:
Remaining blockers:
Next blocking capability:

---

## v0.1 — OBSERVATION & GENERATION

Status: ACHIEVED

Product promise: Observe a real Kubernetes workload and generate security artifacts.

Functional scope:
- trace CLI (cmd/landlock-genprof trace)
- Inspektor Gadget / eBPF acquisition
- Evidence v2
- filesystem, network, syscall, capability observations
- synthesis and exporters (NetworkPolicy, PodLock/Landlock, Seccomp, securityContext)

Milestone gates:
- real tracer event captured
- valid Evidence v2 produced
- at least one generated security artifact
- unit/race/build validation green

Evidence required to declare ACHIEVED:
- tracer CLI and code: cmd/landlock-genprof/trace.go
- tracing implementation: internal/tracer/*
- TrainingHistory & Evidence model: internal/history/* and deploy/crd-traininghistory.yaml
- generated artifacts in demos: demo/golden (patched manifest, networkpolicy, spo)
- unit tests and CI job entries (go test, gosec, Trivy runs)

Current evidence (repository):
- trace CLI implementation: cmd/landlock-genprof/trace.go
- tracer internals: internal/tracer and internal/observation
- Evidence/TrainingHistory code: internal/history and CRD templates at deploy/helm/landlock-genprof/crds/
- demo/golden manifests and demo harness scripts that generate artifacts (demo/golden/workload.yaml, demo/golden/run-actions.sh, demo/golden README)
- smoke-networkpolicy.sh and smoke-tracer.sh e2e scripts under test/e2e/
- CI jobs run and tests passing locally (go build, go test)

Remaining blockers: none for this milestone; evidence present in repo and E2E harness demonstrates generation. Any achieved declaration must cite E2E run artifacts when recorded.

Next blocking capability: none — move focus to multi-run learning (v0.2).

---

## v0.2 — MULTI-RUN LEARNING & CONFIDENCE

Status: ACHIEVED

Product promise: Learn stable workload behavior across multiple runs and produce confidence levels per observation.

Functional scope:
- TrainingHistory CRD and controller support
- RunsRecorded, SeenInRuns counters
- identity deduplication, multi-run merge
- confidence calculation

Milestone gates:
- TrainingHistory objects created and updated across runs
- SeenInRuns reflects observed counts
- confidence map matches 3-run expectations in Golden E2E

Evidence required:
- demo/golden multi-run E2E that records TrainingHistory runsRecorded=1,2,3
- JSON outputs showing SeenInRuns per artifact

Current evidence (authoritative):
- Core E2E run 32037183484 (SHA 902f992, 2026-08-17) — SUCCESS
- TrainingHistory `tools-curl-f0da955d` in landlock-genprof-e2e:
    runsRecorded=3, generation=3, stable uid across all three runs
- Filesystem SeenInRuns (3/2/1 proven):
    /dev write:3  /etc read:3  /etc/ssl read:3  /lib read:3
    /usr/bin exec:3  /usr/lib read:3
    /var/tmp/nginx-demo-2 write:2  /srv/nginx/data write:1
- Network SeenInRuns (3/2/1 proven):
    port 8080 egress:3 (high)  port 8081 egress:2 (medium)  port 8082 egress:1 (low)
- Syscall: 46 names persisted, seenInRuns=3 except mkdir=2 (intentional)
- Capability: CAP_SYS_ADMIN seenInRuns=3
- Confidence: run1=100%, run2=98%, run3=96%
- NetworkPolicy confidence annotations: high/medium/low correct
- Event counts: run1=469 run2=323 run3=186
- nginx-demo-events-run-{1,2,3}.json artifacts in CI diagnostics

Remaining blockers: none — all milestone gates proven by authoritative E2E run.

Next blocking capability: none — milestone complete. Active gate is v0.4.

---

## v0.3 — GOVERNED PROPOSAL / APPROVAL

Status: ACHIEVED

Product promise: Generated security policy does not automatically gain deployment authority; approvals govern application.

Functional scope:
- SecurityProfileProposal CRD flows
- CandidateDigest and digest validation
- approval state machine, approval mechanisms
- apply-time revalidation and fail-closed behavior

Milestone gates:
- proposal generated and stored
- CandidateDigest validated, wrong digest rejected, correct accepted
- approvalState == Approved persisted
- apply-proposal succeeds and reproduces expected candidate

Current evidence (authoritative):
- Core E2E run 32037183484 (SHA 902f992, 2026-08-17) — SUCCESS
- SecurityProfileProposal nginx-demo: historyUsed=true, generatedAt set
- CandidateDigest:
    sha256:2763e8b8ba893229bea5de3243657de8329699faa93841e3901fea655e6897f1
- Wrong-digest rejection confirmed:
    "expected candidate digest mismatch: provided sha256:0000..., computed sha256:2763..."
- Correct approval: approvalState=Approved, Reason=demo approval
- approvedCandidateDigest==CandidateDigest (proven by kubectl jsonpath read-back)
- approvalMechanismVersion=candidate-v1 (persisted in cluster status)
- INV-PROPOSAL-APPROVAL-01 enforced (digest binding, RetryOnConflict)
- INV-PROPOSAL-APPLY-01 enforced (apply-proposal validated digest before applying)
- apply-proposal output: NetworkPolicy applied, Patched Manifest applied
- Artifacts available 4/4; 2 applied; 2 skipped (see environment note below)
- ADR-0006 (Accepted) fully implemented and proven

Environment limitation (recorded, not a blocker for v0.3):
  PodLock and SPO SeccompProfile were skipped during apply-proposal because
  the PodLock/Kubewarden and security-profiles-operator CRDs are not installed
  in the E2E test cluster. This is an apply-time environment constraint, not a
  gap in the proposal/approval model. These artifacts were generated (4/4
  available), correctly carried in the approved proposal, and would apply if
  the required CRDs were present. Actual enforcement of these artifacts belongs
  to v0.4.

Schema parity hardening (H-REL-001) is closed: both shipped CRDs declare
  `approvedCandidateDigest` and `approvalMechanismVersion` with matching
  release validation, and `internal/proposal/crd_schema_test.go` protects
  root/Helm parity against drift.

Remaining blockers: none for v0.3 gate definition.

Next blocking capability: none — milestone complete. Active gate is v0.4.

---

## v0.4 — ENFORCEMENT & VERIFICATION

Status: IN_PROGRESS

Product promise: Prove that approved policy changes actual workload behavior (generated -> applied -> enforced -> verified).

Functional scope:
- track lifecycle: GENERATED -> APPROVED -> APPLIED -> ENFORCED -> VERIFIED
- independent verification per artifact type (NetworkPolicy/Cilium, SPO/seccomp, PodLock/Landlock, capability enforcement)

Milestone gates (core):
- NetworkPolicy GENERATED
- NetworkPolicy APPLIED
- NetworkPolicy ENFORCED (datapath realized)
- NetworkPolicy VERIFIED (fresh connections denied)

Current evidence:
- Core E2E run 32037183484 confirms NetworkPolicy fully proven:
    GENERATED ✓ (nginx-demo NetworkPolicy in SecurityProfileProposal)
    APPLIED ✓ (apply-proposal applied NetworkPolicy to cluster)
    ENFORCED ✓ (Cilium Endpoint API shows policy active — smoke-networkpolicy.sh)
    VERIFIED ✓ (curl blocked with timeout after policy applied; smoke confirmed)
- authoritative Cilium endpoint polling and smoke-networkpolicy harness
  (test/e2e/smoke-networkpolicy.sh) implemented and passing in CI
- demo/golden apply-proposal and verification steps exist and pass

Remaining blockers:
- SPO/seccomp: the operator dependency is CLOSED — security-profiles-operator
  v1.0.0 runs in the SPO Interop E2E cluster and reconciles the approved
  profile, which is bound to a running workload (SHA 0e6ce70, run
  32230551571). What remains is a denial scenario: no syscall has been
  denied, so behavioral enforcement is still unproven. See
  docs/PROGRESS.md for the evidence and its explicit limits.
- PodLock/Landlock enforcement requires Kubewarden/PodLock in test cluster

Release rule: a release claiming SPO interoperability must have SPO Interop
E2E green on the exact RC SHA. Normative statement in docs/PROGRESS.md,
procedure in CONTRIBUTING.md.

Next blocking capability: a seccomp denial scenario, and a PodLock/Landlock
enforcement adapter in CI

---

## v0.5 — KUBERNETES OPERATOR

Status: PLANNED

Product promise: Reconcile approved security intent continuously through Kubernetes (controller-runtime based operator delivering artifacts and status).

Scope and gates: controller code, CR status conditions, reconciliation tests. No controller currently present in repository; planned.

---

## v0.6 — DRIFT / CONTINUOUS LEARNING

Status: PLANNED

Promise: detect divergence between runtime and learned baseline; retrain and evolve confidence.

---

## v0.7 — DETECTION

Status: PLANNED

Promise: runtime anomaly detection with confidence-aware scoring.

---

## v0.8 — CONTROLLED RESPONSE

Status: PLANNED

Promise: governed responses to dangerous deviations (audit, warn, generate proposal, deny, quarantine).

---

## v0.9 — PRODUCTIZATION

Status: PLANNED

Promise: make the product installable and operable by others (packaging, docs, SBOM, signed releases, diagnostics).

---

## v1.0 — STABLE PRODUCT CONTRACT

Status: PLANNED

Promise: stable public contracts (CLI, CRDs, evidence schema, TrainingHistory semantics, approval semantics, migration policy).

Gate: reproducible E2E, external-user success, documented guarantees.

---

## Milestone notification contract

Whenever repository work proves all gates for a roadmap milestone, the implementation/review agent must report in the project report:

MILESTONE ACHIEVED: <version> — <milestone name>

and include evidence:
- tests (unit/integration)
- E2E run artifacts (links / logs)
- produced artifacts (files generated / CR status)
- relevant commit(s) (SHA + message)

Only after the evidence is published will the roadmap be updated: Status -> ACHIEVED.

If only partial gates pass: Status remains IN_PROGRESS.
If a blocking defect exists: Status -> BLOCKED with explicit blocker description.

---

## Current product focus

Active milestone: v0.4 — Enforcement & verification.

Evidence base (as of 2026-08-17, run 32037183484, SHA 902f992):
- v0.1: ACHIEVED — observation + generation proven
- v0.2: ACHIEVED — multi-run TrainingHistory, SeenInRuns, confidence proven
- v0.3: ACHIEVED — proposal/approval/apply pipeline proven end-to-end
- v0.4: IN_PROGRESS — NetworkPolicy enforcement pipeline fully proven;
        SPO interoperability proven (0e6ce70) but seccomp denial unproven;
        PodLock enforcement still requires operator presence in test cluster

Immediate P1 (must close before v0.4 can be marked ACHIEVED):
1. SPO / security-profiles-operator is now in the E2E test cluster and
   SeccompProfile reconciliation, identity and binding are proven; prove
   enforcement by denying a syscall outside the approved profile.
2. Add PodLock / Kubewarden to the E2E test cluster and prove Landlock
   enforcement.

Progress decisions: product must NOT expand into v0.5 until v0.4 gates are
ACHIEVED or explicitly deferred.

---

## Cross-check against current code (summary)

Notable artifacts found in repo (evidence cited above):
- trace CLI: cmd/landlock-genprof/trace.go
- tracer internals: internal/tracer
- TrainingHistory: internal/history and CRDs in deploy/helm/landlock-genprof/crds
- proposal: internal/proposal and cmd apply/review
- e2e harness: test/e2e/* and demo/golden/*
- smoke-networkpolicy: test/e2e/smoke-networkpolicy.sh (authoritative Cilium polling implemented)

No operator/controller code present (v0.5 PLANNED).

---

## Docs index update

A docs/ directory exists. To avoid churn, this roadmap is written to docs/PRODUCT_ROADMAP.md. Add the roadmap reference to the docs index only if maintainers request; no automatic index edit performed in this change.

---

## Validation

Ran local repository checks; file created in working tree (uncommitted). Please review before commit.

---

## Recommendations

- Execute Golden multi-run to close v0.2 and publish TrainingHistory artifacts; when observed, follow the milestone notification contract.
- Add Dependabot config and SBOM generation to support v0.9 productization.
- Triage gosec findings as separate work.



(End of PRODUCT_ROADMAP.md)
