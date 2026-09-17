# 09 — Page specifications

## Environment panel

Always visible above every view (`#operations-context`). Primary chips:
**Cluster**, **Namespace**, **Identity**, **Connection**. Secondary
(behind "Environment details" `<details>`): durable cluster identity,
context/credential method, authority/platform/projection status, read
time, context version. Never renders kubeconfig contents, tokens, or
certificates — see [12-security-ux-constraints.md](12-security-ux-constraints.md).

Controls: Cluster select → Identity select (dependent, disabled until a
cluster is chosen) → Namespace select **or** explicit-namespace text input
+ Open (dependent on the identity's discovery permissions). See Journey C
in [02-user-journeys.md](02-user-journeys.md).

## Overview

Answers: where am I, what needs attention, what's currently observed, what
can I do next. Four summary cards (Platform, Projection, Attention count,
Visible workloads) plus an operational-summary panel that states either
"Select a canonical workload to inspect Observation and Proposal decisions"
or that selected-workload data is available, and surfaces a `DEGRADED`
projection banner when the backend excluded malformed objects. No
fabricated metrics — every number here is a real count from
`/api/v08/environment`, not a synthesized KPI.

## Workloads

A single data table: Workload / Kind / Container / Observation / Proposal
/ Attention / Action. Per-row **Inspect** is the only action; it selects
the workload/container and navigates to Observations. Observation/Proposal
columns currently read "Inspect to view" rather than a live per-row summary
— see [15-known-limitations.md](15-known-limitations.md) for why that
wasn't changed in this pass.

## Observations

Workload picker + the state-driven lifecycle control (see
[04-interaction-model.md](04-interaction-model.md)), then two columns:
**Observation records** (workload-first cards, most recent first) and
**Evidence summary** (detail for whichever observation is currently open).
Evidence is never summarized as a single pass/fail — see
[06-evidence-model.md](06-evidence-model.md).

## Proposals & Governance

One card per proposal: name + approval-state badge, current authority,
`reviewedBy`/`approvedBy`/`rejectedBy` attribution line, a
"Technical metadata" disclosure (candidate digest, resourceVersion), and
the four governance actions (Review/Approve/Reject/Apply), each
independently gated on capability + semantic eligibility + a valid
resourceVersion, each carrying a stated reason when disabled. Stale (409)
decisions are surfaced distinctly from generic failures. See
[12-security-ux-constraints.md](12-security-ux-constraints.md).

## History

A single chronological table per selected subject: Time / Event / Resource
/ Actor / Outcome. Timestamped events and unordered facts are visually
distinguished (`tr.discontinuity`) rather than interleaved as if equally
ordered. `EMPTY`/`UNAVAILABLE`/`DEGRADED` are each labeled explicitly
rather than left to look the same as a quiet page.

## Attention

An investigation queue, not a dashboard widget: one item per bounded
condition, each with Category, Resource, Reason (translated via
`attentionCopy()` from backend codes like `APPROVED_NOT_APPLIED`,
`BINDING_INVALID`, `NEW_CONTRIBUTION_SINCE_CANDIDATE`), Impact, and a
disposition badge. What the backend can and cannot yet distinguish here is
documented honestly in [15-known-limitations.md](15-known-limitations.md)
rather than papered over with invented categories.
