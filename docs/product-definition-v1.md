# Product Definition v1

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

## Immediate next actions

1. Wire internal/analysis.SecurityRecommendation into CLI output
2. Add recommendation-specific integration tests
3. Add one command focused on product output presentation
4. Publish v0.1.0 narrative around evidence and human approval
