# Screenshots

Real captures from a live Operations Center running against the
`landlock-genprof-core` kind cluster (Lima VM), taken with Playwright. Not
mockups, not hand-edited. This set replaces the first-pass screenshots from
before the correction round documented in
[../15-known-limitations.md](../15-known-limitations.md); the earlier set
showed a real bug (`IDENTITY UNKNOWN`) that is fixed as of this set.

## How to reproduce

```bash
# 1. Start the real demo (builds and runs the actual binary, prepopulates
#    a real observation + proposal via test/ui/workbench-smoke.js against
#    a live cluster). Requires the landlock-genprof-core Lima VM/kind
#    cluster (see hack/env-doctor.sh).
./hack/ui-lima-demo.sh
# Wait for: OPERATIONS_CENTER_DEMO_READY / URL=http://127.0.0.1:8090

# 2. In another shell, drive the browser with Playwright
#    (test/ui/node_modules must have playwright installed; ui-lima-demo.sh
#    does this automatically on first run):
NODE_PATH="$PWD/test/ui/node_modules" UI_URL="http://127.0.0.1:8090" \
  node <a script that navigates each view, selects the prepopulated
        workload, starts a new Observation, and screenshots each state>

# 3. Ctrl-C the demo when done; it cleans up its own namespace/processes.
```

The capture used for this set drove: page load (Environment binding
check), Overview, Workloads, Observations (workload selected → `READY`;
**Start observation** clicked → `OBSERVING`; a second, already-`COMPLETED`
observation opened via **Review evidence**), Proposals & Governance,
History, Attention — and asserted zero browser console errors, zero
failed `/api/` requests, zero uncaught exceptions, and that the Environment
panel bound the real namespace the demo created (not `UNKNOWN`, not an
arbitrary one) at every step.

## What's here (representative states, not an exhaustive matrix)

| File | View / state | Width |
|---|---|---|
| `01-environment.png` | Environment panel: Cluster/Identity/Namespace all correctly bound (no `UNKNOWN`) | 1440 |
| `00-overview.png` | Overview | 1440 |
| `00-workloads.png` | Workloads table | 1440 |
| `02-observation-ready.png` | Observations, workload selected, latest Observation `COMPLETED`, state card green, **Start new observation** primary | 1440 |
| `03-observation-active.png` | **Live-captured `OBSERVING` state**: card blue, "Observing runtime activity…", **Start** disabled, **Stop observation** visible/enabled, **Generate proposal** disabled at both the lifecycle-card and per-card level | 1024 |
| `05-observation-completed-unknown.png` | Completed observation opened for review: 6 real capability facts, `Evidence state unknown` badge, **Generate proposal** now enabled/primary (gated on `COMPLETED`, not on evidence quality) | 1440 |
| `06-proposal-governance.png` | Proposals & Governance: `Approved` and `Rejected` proposals, every action's disabled reason visible | 1440 |
| `08-governance-denied-reason.png` | Cropped: **Apply** disabled with `NOT_AUTHORIZED — capability is unavailable.` — the live authorization-denied case for this identity | 1440 |
| `00-history.png` | History table | 1440 |
| `07-attention.png` | Attention: a real `APPROVED_NOT_APPLIED` item | 1440 |
| `observations-1024.png` | Observations at 1024px — responsive/collision check | 1024 |
| `proposals-governance-1024.png` | Proposals & Governance at 1024px — the most control-dense view | 1024 |
| `proposals-governance-1440.png` | Proposal decision surface with structured candidate policy | 1440 |
| `proposals-governance-1280.png` | Proposal decision surface and governance actions | 1280 |

The three proposal screenshots above were freshly captured from the live
demo after the decision-surface correction. They show real candidate-v2
capability material (`Drop`/`Add`), aggregate evidence qualification,
candidate provenance disclosure, and the existing digest/resourceVersion
metadata. The capture used Playwright against `make operations-center-demo`;
the command and browser qualification pattern above are reproducible.

1280px was inspected during the review loop (no control collisions, same
reflow as 1024/1440 bracket) and is not separately archived here, per the
"representative, not exhaustive" instruction.

The lifecycle correction also produced a real authenticated immediate-Stop
capture at `/tmp/pr256-live-immediate-stop-1280.png`. It is intentionally
kept as a local qualification artifact rather than committed generated media.

## What this round of screenshots proves, specifically

This capture pass exists to verify three fixes made after a rejected first
pass (see [../15-known-limitations.md](../15-known-limitations.md) for the
full incident writeup):

1. **Identity/Namespace binding is no longer silently wrong.**
   `01-environment.png` shows `IDENTITY kind-landlock-genprof-core` and
   `NAMESPACE ui-lima-demo-XXXXX` — both real — where the rejected pass
   showed `IDENTITY UNKNOWN` and, in the underlying request traffic, had
   silently rebound to an unrelated namespace (`cilium-secrets`) with no
   operator action involved.
2. **The Workload control no longer collides.** `02-observation-ready.png`
   shows a labeled "Workload" field above its `<select>`, not glued
   together as `WorkloadDeployment/...`.
3. **Generate proposal is genuinely state-gated.** `03-observation-active.png`
   shows it disabled (both instances: lifecycle card and per-card) while an
   Observation is actively running for the workload, even though a
   *different, older* completed Observation had previously been opened for
   review in the same session (a stale-reference bug that existed before
   this correction).

## A capture artifact worth knowing about (from the first pass, still true)

Full-page (`fullPage: true`) screenshots of a sticky-positioned topbar can
render it twice in the stitched image at some viewport heights. That is a
Playwright screenshot-stitching artifact, not a live rendering defect — a
real user scrolling the page only ever sees one topbar, pinned to the top.
None of the screenshots in this set happened to show it, but it remains a
known capture-tooling quirk, not a UI bug, if it recurs.

## What was not captured live in this pass

No `COMPLETED` observation with `AVAILABLE` (qualified) evidence was
reproduced against this cluster in any capture session — every real
completed Observation captured so far reported `Evidence state unknown`.
This is a property of the qualification proof conditions on this test
cluster (see [../06-evidence-model.md](../06-evidence-model.md)), not
something the UI hides; a future pass with different cluster/executor
timing may reproduce it. Not fabricated to fill the gap.
