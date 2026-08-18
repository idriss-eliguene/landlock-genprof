# Product Definition v1

> **Canonical authority:** This document is the canonical Product Vision,
> product positioning, and responsibility-boundary source. Current
> demonstrated progress is recorded in [`PROGRESS.md`](PROGRESS.md); roadmap
> intent is recorded in [`PRODUCT_ROADMAP.md`](PRODUCT_ROADMAP.md); current
> and target architecture is recorded in [`architecture.md`](architecture.md).

## Product statement

A Kubernetes least-privilege platform driven by real workload behavior.
It observes, learns, recommends, requires human approval, then generates
enforcement artifacts across multiple security backends.

## Problem

Kubernetes already provides strong least-privilege controls, but teams
struggle to configure them correctly because policy authoring is manual,
error-prone, and demands deep platform expertise.

## Value proposition

Workload behavior evidence is converted into explainable, human-approved
security recommendations that can be enforced through multiple native
Kubernetes mechanisms.

## Positioning

This project is not a replacement for PodLock, security-profiles-operator,
or CNI policy engines. It is the intelligence and orchestration layer above
those mechanisms.

**Nor is it a replacement for static compliance scanners** (CIS Kubernetes
Benchmark tooling like kube-bench, Kubescape, Kyverno, Polaris, Checkov).
Those answer *"is this configuration compliant with a known-good rule?"*
— a static check against a fixed policy. This project answers a different
question: *"what privileges does this workload actually use?"* — runtime
evidence, not configuration inspection. The two are complementary, not
competing: a static scanner can flag that `capabilities.drop: [ALL]` is
recommended; this project can additionally say *why* — `NET_BIND_SERVICE`
was observed on 27/27 training runs, every other capability was never
exercised. Confidence and cross-run evidence (`TrainingHistory`) already
carry this distinction today, per-path/port/syscall — formalizing it as a
reusable, named concept and mapping it onto specific compliance framework
control IDs (CIS, Pod Security Standards, ...) is a real but separate
extension, not a near-term commitment — see "Development phases" below.

## Core product loop

1. Select workload target
2. Observe runtime behavior
3. Build BehaviorProfile IR
4. Accumulate evidence in TrainingHistory
5. Generate explainable recommendation
6. Human review and approval
7. Generate enforcement artifacts
8. Apply with existing platform controls

## MVP scope

One workload, one behavioral model, one recommendation set, multiple
enforcement backends.

| Domain | Observation | Backend artifact |
|---|---|---|
| Filesystem | file access behavior | PodLock profile |
| Network | connect/bind behavior | NetworkPolicy |
| Syscalls | syscall behavior | SPO SeccompProfile |
| Hardening | capability and runtime signals | securityContext fragment |

## Out of scope for MVP

- Multi-cluster coordination
- Dashboard/UI
- Automated approval
- Full lifecycle operator
- Cross-workload correlation

## Architecture components

- Profiler/Sensor: runtime behavior collection
- Behavior Engine: IR, history, confidence, drift inputs
- Policy Engine: explainable recommendation generation
- Enforcement Plane: backend renderers and Kubernetes artifacts

## Key differentiator

Traditional flow:

Human -> policy authoring -> enforcement

Product flow:

Workload -> behavior evidence -> recommendation -> human approval -> enforcement

## Explainability contract

Every recommendation must include:

- Why this recommendation exists
- What evidence supports it
- Confidence level
- Which backend artifact will enforce it

## Product APIs and data model (v1)

- BehaviorProfile: backend-independent runtime behavior IR
- SecurityRecommendation: explainable decision object
- SecurityProfileProposal: reviewable cluster artifact
- TrainingHistory: persisted evidence across runs

## Product experience direction

The current product surface is CLI-first and proposal-first:

- CLI: trigger observation and present recommendation summary
- SecurityProfileProposal: canonical review artifact in-cluster
- Export/apply workflow: proposal-first operational handoff

See [docs/product-design-v1.md](product-design-v1.md) for the first
design brief: personas, user journey, review surfaces, and visual
direction for a future UI.

## Repository structure direction

Keep monorepo and evolve without big-bang refactor:

- internal/tracer: observation
- internal/profile: behavior IR
- internal/history: persisted training evidence
- internal/analysis: recommendation logic
- internal/exporter (future internal/backend): enforcement renderers

## Community strategy

### Entry experience

A new user should be able to:

1. Understand the value in less than 30 seconds
2. Run a demo in less than 5 minutes
3. Produce at least one useful policy artifact

### Initial open source package

Start with a focused promise:

Run your Kubernetes workload and generate least-privilege security
recommendations from observed behavior.

### First public milestone

v0.1.0 should include:

- Reproducible end-to-end demo
- Clear quick start
- Explainable recommendation output
- CONTRIBUTING and roadmap clarity

## Development phases

### Phase 1: Technical MVP (2-3 months)

- Stabilize filesystem, network, seccomp and hardening outputs
- Consolidate IR and TrainingHistory
- Ship explainable recommendations

### Phase 2: Kubernetes product workflow (2-4 months)

- Add recommendation and approval CRDs
- Add approval workflow
- Add operator-driven enforcement orchestration

### Phase 3: Platform expansion

- Drift detection
- Multi-cluster and GitOps workflows
- Audit and approval RBAC
- Lifecycle governance
- **Exploratory, not committed:** map generated recommendations onto
  named compliance framework controls (CIS Kubernetes Benchmark, Pod
  Security Standards) as evidence-backed assessments — "this control is
  supported by observed runtime behavior," not a general-purpose
  compliance scanner (cluster-level controls — API server, etcd,
  kubelet, RBAC — stay explicitly out of scope, that's kube-bench's
  job). Before any implementation: confirm CIS Benchmark content's
  actual redistribution terms (CIS SecureSuite licensing) — not yet
  verified — and decide whether this is worth the ongoing maintenance
  of tracking framework version drift on a small-maintainer project,
  ideally after real community feedback on the core loop, not before.

## Immediate next actions

1. Wire internal/analysis.SecurityRecommendation into CLI output
2. Add recommendation-specific integration tests
3. Add one command focused on product output presentation
4. Publish v0.1.0 narrative around evidence and human approval
5. Stabilize proposal-first demo and review workflow
6. Define UI review surface from SecurityProfileProposal as source of truth
