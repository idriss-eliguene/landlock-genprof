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

Focus: move from reviewable proposal to explicit approval semantics, with
enough rationale behind each recommendation that approving one means
something.

Scope:

- ~~Add a CLI review command centered on proposal inspection~~ — done,
  landed ahead of this phase (`landlock-genprof review <proposal>`)
- Add structured, per-domain rationale to recommendation output — not just
  a confidence label, the reasoning behind it (pulled forward from a later
  phase: a review command without this is just recycled YAML)
- Add product-facing approval model
- Define approval status and promotion lifecycle
- Introduce a first UI mock or lightweight review prototype, built on top
  of that rationale rather than ahead of it

Candidate deliverables:

- Domain-by-domain rationale rendering, extending `review`'s existing
  output
- Approval fields or an approval CRD/status subresource
- Proposal status model: draft, reviewed, approved, rejected
- First “Workload Security Review” visual implementation or prototype

Acceptance criteria:

- A reviewer can identify whether a proposal is awaiting review or approved
- A reviewer can inspect backend artifacts and rationale without reading raw CRD
  structure
- The workflow distinguishes generation from approval

Risks:

- Approval semantics become complex before the product loop is stable
- A UI prototype drifts away from the real CRD-driven workflow
- A UI prototype built before rationale exists ends up decorating YAML
  instead of explaining it

### v0.3 — Explainable proposal product

Focus: make a proposal fully understandable on its own — a foundation the
review UI and any future automation both depend on, not a screen in
itself.

Scope:

- Semantic diff between two versions of the same proposal (what actually
  changed and why, not a raw YAML diff)
- `landlock-genprof explain <proposal>` command

Explicitly out of scope this phase: no additional UI work, no long-term
history storage — this is about the model and the CLI.

### v0.4 — Continuous confidence product

Focus: move from “a single point-in-time observation” to “is this profile
still true” — without yet acting on what it finds.

Scope:

- Re-tracing on a schedule, not just once on demand — the first departure
  from this project's CLI-only, no-controller architecture so far.
  Treat this as an explicit architecture decision to revisit deliberately
  when the time comes, not just another feature to add.
- Drift detection, with severity classification
- A one-shot `drift-check` command against a single cluster

Explicitly out of scope this phase: alerting integrations, long-term
drift history, multi-cluster aggregation — all of those need real usage
feedback this phase doesn't have yet.

### v0.5 — Guided remediation product

Focus: close the loop from a detected drift back to a reviewable action,
and extend output coverage without duplicating what security-profiles-operator
already does natively.

Scope:

- Auto-generate a new proposal from a detected drift, rationale included,
  re-approved through the same standard workflow as any other proposal —
  never auto-applied
- Native SPO `SeccompProfile` emission from the IR (already the case
  today via `internal/exporter/spo` — this extends the same pattern)
- Native SPO `AppArmorProfile` emission from the IR (feeds SPO's own
  format; no duplicate AppArmor tracer)
- Ingestion of externally-written SELinux profiles (`audit2allow`,
  hand-authored) into the review workflow — governance of what's already
  there, not generation of anything new

## Phases beyond v0.5

Not yet scoped — revisit once the above has real usage and feedback to
build on, not before.

## UX roadmap by phase

### v0.1

- CLI-first
- YAML review via `kubectl` and exported files
- Makefile shortcuts for demo and application

### v0.2

- Proposal-first review command, with rationale
- Structured review output per domain
- First operator-facing UI surface

### v0.3 – v0.5

- CLI-only by design — no new UX surface until the v0.2 review prototype
  has proven itself with real usage

## Design roadmap by phase

### v0.1

- Define product language and review-centered visual direction
- Specify primary review screen
- Keep experience serious, technical, and evidence-led

### v0.2

- Build the first review interface prototype
- Introduce artifact tabs, confidence bars, evidence chips, and rationale cards
- Test whether proposal review is faster than raw YAML inspection

## Immediate product backlog

1. ~~Add a CLI `review` command that renders one proposal as a product
   surface.~~ — done
2. Add structured rationale text to recommendation output by domain.
3. Define the approval-state model before building any operator.
4. Prototype the “Workload Security Review” screen from the screen spec,
   using that rationale.
5. Standardize the live demo around the proposal-first path only. — done

## What not to do yet

- Do not build a broad generic dashboard.
- Do not jump to multi-workload inventory views before the single-workload loop
  is excellent.
- Do not automate approval before rationale and review ergonomics are clear.
- Do not build the review UI before rationale exists behind it.
- Do not introduce a background daemon (scheduled re-tracing) before the
  CLI-only approval and rationale loop has real usage — see v0.4's note
  above.
- Do not build full enforcement reconciliation before approved state is modeled.