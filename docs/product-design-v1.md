# Product Design v1

## Design objective

Turn `landlock-genprof` from a collection of generated security files into a
clear product workflow: observe a workload, review evidence, approve a
recommendation, then apply enforcement safely.

This document is intentionally product-facing rather than implementation-
facing. It defines the operator experience the current CLI and future UI
should converge on.

## Primary user

### Platform security engineer

- Owns Kubernetes guardrails and workload hardening
- Understands cluster primitives but does not want to hand-author every policy
- Needs evidence before approving enforcement
- Needs outputs that can survive review, GitOps, and audit

### Secondary user

- Application platform engineer
- Wants a safe default path to least privilege for a single workload
- Needs a fast demo and low-friction artifact export/apply flow

## Core user job

When a workload is running in Kubernetes, help me generate and review
least-privilege controls from real behavior so I can enforce them with
confidence and explain every decision.

## Product promise

The product does not ask the user to trust a black box. It turns runtime
behavior into a reviewable recommendation package with evidence, confidence,
 and explicit enforcement artifacts.

## Product flow

1. Select a workload target
2. Observe real runtime behavior
3. Aggregate evidence into a reusable behavioral model
4. Produce a recommendation summary
5. Publish a `SecurityProfileProposal` as the canonical review object
6. Export or apply approved enforcement artifacts

See [docs/product-roadmap-v1.md](product-roadmap-v1.md) for the concrete
product milestones and [docs/product-screen-workload-review-v1.md](product-screen-workload-review-v1.md)
for the first UI screen spec.

## Canonical artifact model

`SecurityProfileProposal` is the product center of gravity.

It should be treated as:

- The review surface of record
- The source for exported files
- The future input to approval and enforcement workflows

Local files remain a convenience layer, not the product's source of truth.

## UX principles

### 1. Evidence before action

Every enforcement action must be preceded by a visible explanation of what was
observed and why a recommendation exists.

### 2. One workload at a time

The MVP should feel extremely clear for one workload rather than vaguely
capable for many.

### 3. Proposal-first, file-second

The cluster object is the canonical package. File export is an operational
bridge, not the main user mental model.

### 4. Explainability over automation theater

Confidence, evidence, and backend choice must be understandable without reading
source code.

### 5. Operationally credible

The product must feel safe for real cluster operators: predictable ordering,
explicit labels, explicit apply steps, explicit prerequisites.

## MVP surfaces

### Surface 1 — CLI trace summary

Goal: give immediate signal that the run produced something useful.

Must show:

- Workload identity
- Training run count
- Which protection domains produced recommendations
- Overall confidence
- Where to review next (`SecurityProfileProposal`)

### Surface 2 — Proposal review

Goal: provide a single object that a human can inspect before enforcement.

Must contain:

- PodLock artifact
- NetworkPolicy artifact when available
- Patched manifest when available
- SPO SeccompProfile when available
- Timestamps and run context

### Surface 3 — Proposal export/apply workflow

Goal: let operators move from review to action without rerunning observation.

Current product entrypoints:

- `make export-proposal PROPOSAL=<name>`
- `make demo-proposal PROPOSAL=<name>`
- `make apply-proposal PROPOSAL=<name>`

## Future UI information architecture

If a UI is added later, the first screen should not be a dashboard full of
generic charts. It should be a workload review workspace.

### Primary screen: Workload Security Review

Top section:

- Workload name, namespace, container, binary
- Recommendation status
- Overall confidence
- Last generated timestamp

Middle section:

- Evidence summary by domain: filesystem, network, syscalls, hardening
- Recommendation cards with backend mapping
- Confidence and rationale per domain

Bottom section:

- Artifact tabs: PodLock, NetworkPolicy, Patched Manifest, SPO SeccompProfile
- Raw YAML review with copy/export actions
- Apply/approve actions gated by role and workflow stage

This screen is specified in detail in
[docs/product-screen-workload-review-v1.md](product-screen-workload-review-v1.md).

### Secondary screen: Training history view

- Number of recorded runs
- Stability trend across runs
- Drift indicators for newly seen behavior

### Secondary screen: Proposal list

- Workload identity
- Confidence
- Generated date
- Approval status
- Available artifact types

## Visual direction

The product should look like a review console, not a marketing dashboard.

Visual keywords:

- precise
- industrial
- evidence-led
- high signal
- operator-grade

Suggested direction:

- Backgrounds: off-black, slate, or warm graphite
- Accent colors: safety green for validated actions, amber for caution, steel
  blue for neutral structure
- Typography: strong monospace support for artifact review, paired with a
  serious editorial sans for headings
- Components: confidence bars, evidence chips, diff-style panels, artifact
  tabs, command snippets

Avoid:

- playful gradients disconnected from the product domain
- abstract AI imagery
- dashboard filler charts with no operational meaning
- too many simultaneous calls to action

## Demo narrative

The MVP demo should tell a clean story in under five minutes:

1. Start with a live workload
2. Run `trace`
3. Show the recommendation summary in the CLI
4. Inspect the `SecurityProfileProposal`
5. Export the proposal artifacts
6. Show that the patched manifest already carries the PodLock label
7. Apply approved artifacts

The emotional outcome should be: "this is understandable and ready to fit into
real platform operations", not merely "it generated YAML".

## Product gaps still open

- Approval state is procedural, not yet modeled as its own workflow object
- Drift is implied by history but not yet surfaced as a product concept
- No first-class UI review surface yet
- No policy lifecycle or rollback model yet

## Design acceptance criteria for v0.1

- A new user can explain the product loop in one sentence
- A demo user can move from trace to proposal review without searching docs
- A reviewer can identify the enforcement backend for each domain
- A reviewer can export and apply artifacts from the proposal without rerunning
  trace
- The product story feels centered on reviewable evidence, not raw file output