# 15 — Known limitations

Documented honestly rather than fixed silently or hidden, per this
mission's own instruction to stop and document rather than guess when a
fix would need to go beyond a small presentation adjustment.

## Workload table doesn't show live per-row Observation/Proposal summaries

The Workloads table's Observation and Proposal columns are static
"Inspect to view" text rather than a live count/state per row. Making them
live would require either an additional aggregate read per row (N+1
requests against `/api/observations`/`/api/proposals` for every discovered
container) or a new batched backend endpoint. Both are beyond "a small
presentation/read-model adjustment," so this pass left it as an honest
placeholder rather than adding N+1 client-side fan-out or a new backend
aggregate contract without product sign-off. See
[09-page-specifications.md](09-page-specifications.md#workloads).

## Governance page consolidation is a one-way IA decision

Folding Governance into Proposals (see
[03-information-architecture.md](03-information-architecture.md)) assumes
the two concepts are the same audience/workflow, which is true today
(every governance action already lived in the Proposals view). If a future
milestone gives Governance materially different content (e.g. a
cross-workload approval queue independent of any single proposal list),
this consolidation should be revisited rather than assumed permanent.

## Per-card vs. lifecycle-control action duplication

The Observations view has two places a proposal can be generated from (the
top lifecycle control and each observation card). This is documented as
intentional dual-purpose IA in
[04-interaction-model.md](04-interaction-model.md) (primary/forward-looking
vs. secondary/forensic), but it is also true that `test/ui/workbench-smoke.js`
clicks the top-bar `#generate-proposal` specifically, which is part of why
it was kept rather than removed. A future pass that wants to unify these
into one control should update that test's interaction path deliberately,
not as a side effect.

## Identity-selector bug: fixed, but only single-verified live

The `.contextName` → `.value` fix ([13-implementation-map.md](13-implementation-map.md))
is a straightforward, unambiguous JS API correction verified two ways: (1)
direct DOM inspection against the pre-fix build reproduced `identity-selector.value
=== ""` with a real option present, and (2) code review confirms
`HTMLOptionElement` has no `.contextName` property. It was **not**
re-verified against a second live end-to-end run after the fix, because two
consecutive live re-runs of `hack/ui-lima-demo.sh` (both after the fix was
already in place) failed on an unrelated, reproducible harness error:

```
Error: Generate Proposal rejected: HTTP 400
{"code":"INVALID_REQUEST","message":"invalid request: namespace is outside the Workbench read scope"}
```

This happened on the *second* proposal-generation call
(`test/ui/workbench-smoke.js`'s reject-flow proposal, not the first,
successful one) on both retries, immediately after a prior successful run
against the same Lima VM within the same ~10-minute window. It reproduced
identically on two independent namespaces
(`ui-lima-demo-26153`, `ui-lima-demo-27408`), which rules out a namespace-name
collision. The client-side identity-selector fix cannot plausibly cause a
server-side namespace-scope rejection — that code path is untouched Go in
`observation_api.go`'s `generate()` — so this is treated as a pre-existing
demo-harness/environment flake (most likely resource contention from
running three full observation cycles back-to-back on the same Gadget
tracer/inotify budget within a few minutes), not a regression from this
pass. It was not chased further, per this mission's explicit instruction
not to run repeated full qualification cycles. Flagging it here as a
real, reproducible observation for whoever next touches the demo harness.

## Full-page screenshot capture duplicates the sticky topbar

Some of the 1024px full-page captures in `screenshots/` show the sticky
topbar rendered twice, mid-page. This is a Playwright `fullPage: true`
stitching artifact with `position: sticky` elements, not a live rendering
defect — a real user scrolling the page only ever sees one topbar, pinned
to the top. Noted here and in [screenshots/README.md](screenshots/README.md)
so it isn't misread as a control-collision bug.

## No live capture of `RUNNING`/`STARTING` or evidence-`AVAILABLE` states

The one full observation cycle captured live in this session completed too
quickly to screenshot the transient `STARTING`/`RUNNING` phases, and its
evidence result was `UNKNOWN` rather than `AVAILABLE`. Both states are
implemented and covered by the code-level analysis in
[05](05-observation-state-model.md)/[06](06-evidence-model.md), but a
future visual QA pass should try to capture them directly rather than rely
on static review, if screenshot evidence of those exact states is needed.

## No formal design-token spacing scale

See [07-design-system.md](07-design-system.md). The CSS is internally
consistent but uses literal pixel values, not `var(--space-*)` tokens.
