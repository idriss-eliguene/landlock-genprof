# Screenshots

Real captures from a live Operations Center running against the
`landlock-genprof-core` kind cluster (Lima VM), taken with Playwright
during this convergence pass. Not mockups, not hand-edited.

## How to reproduce

```bash
# 1. Start the real demo (builds and runs the actual binary, prepopulates
#    a real observation + proposal via test/ui/workbench-smoke.js against
#    a live cluster). Requires the landlock-genprof-core Lima VM/kind
#    cluster (see hack/env-doctor.sh).
./hack/ui-lima-demo.sh
# Wait for: OPERATIONS_CENTER_DEMO_READY / URL=http://127.0.0.1:8090

# 2. In another shell, capture screenshots with Playwright
#    (test/ui/node_modules must have playwright installed; ui-lima-demo.sh
#    does this automatically on first run).
NODE_PATH="$PWD/test/ui/node_modules" UI_URL="http://127.0.0.1:8090" \
  node path/to/a/script/that/drives/each/view/and/breakpoint.js

# 3. Ctrl-C the demo when done; it cleans up its own namespace/processes.
```

The exact capture script used for this pass drove each of the six primary
views (`data-view` buttons) at 1440/1280/1024px viewport widths, selecting
the prepopulated workload to reach the Observations/Evidence/Proposals
states, and asserted zero browser console errors, zero failed `/api/`
requests, and zero uncaught exceptions at every step (all three held for
every capture).

## What's here (representative states, not an exhaustive matrix)

| File | View / state | Width |
|---|---|---|
| `environment-selector-1440.png` | Environment panel close-up (Cluster/Identity/Namespace + Connection) | 1440 |
| `overview-1440.png` | Overview, real Platform/Projection/Attention/Workloads counts | 1440 |
| `workloads-1440.png` | Workloads table with one real discovered container | 1440 |
| `observations-1440.png` | Observations, `COMPLETED` lifecycle state, state-driven action bar | 1440 |
| `observation-evidence-1440.png` | Observation evidence detail: **`COMPLETED` + 6 real capability facts + `Evidence state unknown`** — the fail-closed case from [../06-evidence-model.md](../06-evidence-model.md) | 1440 |
| `proposals-governance-1440.png` | Proposals & Governance: one `Approved` proposal, one `Rejected` proposal, `Apply` disabled with `NOT_AUTHORIZED` for this identity | 1440 |
| `history-1440.png` | History table for the captured observation's subject | 1440 |
| `attention-1440.png` | Attention: a real `APPROVED_NOT_APPLIED` item with reason/impact | 1440 |
| `observations-1024.png` | Observations at 1024px — responsive check | 1024 |
| `proposals-governance-1024.png` | Proposals & Governance at 1024px — the most control-dense view, responsive check | 1024 |

1280px was captured and inspected for all views during the review loop but
is not duplicated here to avoid three near-identical copies of every view;
1440 and 1024 bracket it and show the actual reflow points documented in
[../07-design-system.md](../07-design-system.md).

## A capture artifact worth knowing about

The 1024px full-page captures show the sticky topbar (`Operations Center V2
· bounded environment` / `Operations Center`) rendered a second time,
mid-page. This is Playwright's `fullPage: true` screenshot stitching a
`position: sticky` element into more than one stitched segment — it is not
something a real user scrolling the page would ever see (the topbar is
genuinely pinned once, at the top). Documented in
[../15-known-limitations.md](../15-known-limitations.md) rather than left
to be misread as a layout bug.

## Real data behind these captures

Observation ID `b030aa17ee746c6cd382b467d29f6e2a`, workload
`landlock-genprof-demo-workload` (Deployment, container `nginx`), namespace
`ui-lima-demo-23547`, 6 real capability facts (`CAP_CHOWN`,
`CAP_DAC_OVERRIDE`, `CAP_SETGID`, `CAP_SETPCAP`, `CAP_SETUID`,
`CAP_SYS_ADMIN`), evidence state `UNKNOWN`, proposal
`observation-b030aa17ee746c6cd382b467d29f6e2a` (`Approved`) and
`observation-b030aa17ee746c6cd382b467d29f6e2a-reject` (`Rejected`). All
produced by the real executor/Gadget tracer against the real kind cluster,
not fixtures. (A second and third live run, taken later in this pass to
re-verify the identity-selector fix, used different namespaces/observation
IDs and hit the harness flake described in
[../15-known-limitations.md](../15-known-limitations.md); the screenshots
here are unaffected by that and predate it.)
