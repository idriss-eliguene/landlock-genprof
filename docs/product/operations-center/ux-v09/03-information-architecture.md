# 03 — Information architecture

## Navigation, before and after

Before this pass, the primary nav had seven items: Overview, Workloads,
Observations, Proposals, History, Attention, **Governance**. The
`governance-view` section's entire content was:

> "Review, approval, application, and rollback remain explicit operations.
> A stale resourceVersion is rejected and is never replayed automatically."
> [Open proposals] → navigates to Proposals.

Every actual governance action (Review/Approve/Reject/Apply, capability
gating, disabled reasons, CAS conflict handling) already lived in the
Proposals view (`proposalActions` in `workbench_ui.go`). Governance was IA
debt: a page that existed to describe a workflow that was implemented one
click away.

**Decision:** fold Governance into Proposals. The nav item and view header
are now **Proposals & Governance**; the view intro states the CAS/stale
guarantee inline instead of on a separate page. No functional surface was
removed — every action reachable from the old Governance page is reachable
from the same click depth on the new one.

There was also a second, fully orphaned surface: `environment-view`
(`id="environment-view"`, `data-page="environment"`). It had no nav button
pointing at it, was not in the client router's view list, and stayed
`hidden` forever. Its only live side effect was populating `#attention-list`
and `#environment-boundary`, which is real Attention-page behavior that
had nothing to do with the dead section itself. It has been removed; the
Attention-list population it fed is now inline in `renderEnvironment()`
directly. See [13-implementation-map.md](13-implementation-map.md).

## Current navigation (six items)

| Nav item | Answers | Primary persona |
|---|---|---|
| Overview | Where am I? What needs attention? What's currently observed? | All |
| Workloads | What workloads exist in my authorized namespace? | Platform engineer |
| Observations | What ran, what evidence resulted? | Platform engineer, reviewer |
| Proposals & Governance | What's proposed, and can I act on it? | Reviewer (act), platform engineer (generate/inspect) |
| History | What happened, to what, under whose identity? | Reviewer, auditor |
| Attention | What needs investigation right now? | Reviewer |

## The environment surface is not navigation — it's always-on context

The Environment panel (Cluster / Identity / Namespace, plus the
"Environment details" progressive disclosure) is not a page; it's a fixed
header-region panel rendered above every view (`#operations-context` sits
outside the `<section class="view">` blocks in `workbenchClusterPageTemplate`).
This was already correct in M4 and this pass kept it: the active operating
context must always be visible while an operator is deciding whether to
take an action, and duplicating it per-page would be exactly the kind of
redundant metadata the mission brief warns against.

## The operational journey the IA encodes

```
Environment (Cluster → Identity → Namespace, always visible)
  -> Workloads (discover)
    -> Observations (capture evidence for one workload/container)
      -> Evidence detail (qualify what was captured)
        -> Proposals & Governance (generate, review, approve/reject, apply)
          -> History (audit trail)
Attention (cross-cutting: what in any of the above needs a decision now)
```

Overview is the only view that intentionally summarizes across this whole
chain rather than sitting at one point in it; see
[09-page-specifications.md](09-page-specifications.md#overview).
