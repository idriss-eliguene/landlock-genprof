# Product Roadmap v1

## Goal

Turn `landlock-genprof` into an operator-grade least-privilege workflow with a
clear progression:

1. Observe real workload behavior
2. Publish a reviewable proposal
3. Let humans approve with confidence
4. Apply enforcement safely

## Product north star

For one Kubernetes workload, a platform engineer can go from runtime evidence to
reviewable least-privilege enforcement in minutes, without hand-authoring raw
security policy from scratch.

## Phase structure

### v0.1 — Reviewable proposal product

Focus: make the proposal-first workflow complete, credible, and demo-ready.

Scope:

- CLI summary with product framing
- `SecurityProfileProposal` as mandatory review artifact
- Proposal-first export/apply workflow
- Stable end-to-end demo for one workload (`nginx-demo`)
- Product docs explaining evidence, recommendation, and enforcement path

Acceptance criteria:

- A user can run one trace and always find the result in a
  `SecurityProfileProposal`
- A user can export artifacts from the proposal without rerunning the trace
- A user can apply approved artifacts from the proposal in a predictable order
- Demo output clearly shows recommendation domains and confidence
- Patched manifest includes the PodLock label automatically when needed

Primary artifacts:

- CLI `trace`
- `SecurityProfileProposal`
- Make targets for export/demo/apply
- Product and design docs

Risks:

- Product value still feels like “generated YAML” instead of “review workflow”
- No approval state beyond human procedure
- Proposal review remains raw YAML-first

### v0.2 — Approval workflow product

Focus: move from reviewable proposal to explicit approval semantics.

Scope:

- Add product-facing approval model
- Define approval status and promotion lifecycle
- Add a CLI review command centered on proposal inspection
- Surface recommendation rationale and evidence more explicitly per domain
- Introduce a first UI mock or lightweight review prototype

Candidate deliverables:

- `landlock-genprof review <proposal>` command
- Approval fields or an approval CRD
- Proposal status model: draft, reviewed, approved, rejected
- Domain-by-domain rationale rendering
- First “Workload Security Review” visual implementation or prototype

Acceptance criteria:

- A reviewer can identify whether a proposal is awaiting review or approved
- A reviewer can inspect backend artifacts and rationale without reading raw CRD
  structure
- The workflow distinguishes generation from approval

Risks:

- Approval semantics become complex before the product loop is stable
- A UI prototype drifts away from the real CRD-driven workflow

### v0.3 — Enforcement orchestration product

Focus: transition from approved review object to controlled enforcement state.

Scope:

- Define approved-policy object model
- Begin operator-driven enforcement orchestration
- Track drift between approved state and applied state
- Add lifecycle controls for update and rollback

Candidate deliverables:

- `WorkloadSecurityProfile` or equivalent approved-policy CRD
- Reconciliation loop for approved state only
- Drift detection signals
- Safer re-apply/update workflow

Acceptance criteria:

- Approved state is modeled separately from proposal state
- The system can distinguish latest evidence from currently enforced state
- Drift becomes visible as a product concept, not just an implementation detail

## UX roadmap by phase

### v0.1

- CLI-first
- YAML review via `kubectl` and exported files
- Makefile shortcuts for demo and application

### v0.2

- Proposal-first review command
- Structured review output per domain
- First operator-facing UI surface

### v0.3

- Stateful approval and enforcement workflow
- Drift-aware review surface
- Role-aware operational actions

## Design roadmap by phase

### v0.1

- Define product language and review-centered visual direction
- Specify primary review screen
- Keep experience serious, technical, and evidence-led

### v0.2

- Build the first review interface prototype
- Introduce artifact tabs, confidence bars, evidence chips, and rationale cards
- Test whether proposal review is faster than raw YAML inspection

### v0.3

- Add approval state, lifecycle cues, and drift surfaces
- Separate “recommended”, “approved”, and “enforced” visually

## Immediate product backlog

1. Add a CLI `review` command that renders one proposal as a product surface.
2. Add structured rationale text to recommendation output by domain.
3. Standardize the live demo around the proposal-first path only.
4. Define the approval-state model before building any operator.
5. Prototype the “Workload Security Review” screen from the screen spec.

## What not to do yet

- Do not build a broad generic dashboard.
- Do not jump to multi-workload inventory views before the single-workload loop
  is excellent.
- Do not automate approval before rationale and review ergonomics are clear.
- Do not build full enforcement reconciliation before approved state is modeled.