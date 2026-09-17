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

## Incident: identity-selector fix exposed a real namespace-auto-bind bug (now fixed, re-verified live)

A first pass of this document incorrectly concluded that a repeated,
reproducible `"invalid request: namespace is outside the Workbench read
scope"` failure after the `.contextName` → `.value` identity-selector fix
was an unrelated demo-harness flake ("resource contention... not a
regression from this pass"). **That conclusion was wrong**, and a
subsequent human review of the live UI correctly rejected the UX
acceptance that had been claimed on the strength of it. Recorded here in
full because the earlier document actively misdiagnosed the failure it was
supposed to be honest about.

**What actually happened:** the page is server-rendered with
`data-namespace="{{.Namespace}}"` already correctly bound to the namespace
this backend process is scoped to. Before the identity-selector fix,
`openIdentity()`'s auto-bind flow silently no-opped on every page load
(the bug), so that correct, server-provided value was never touched. After
the fix, `openIdentity()` started actually running on load — and its
namespace-selection fallback (`namespaces.value = opened.defaultNamespace
|| namespaces.options[0]?.value || ""`) picked **whatever namespace
happened to be first in the discovery list** whenever the opened identity's
kubeconfig context had no explicit default namespace (true for `kind-*`
contexts). In the captured failure, that was `cilium-secrets` — a Cilium
system namespace with no relationship to the operator's actual workload —
silently overwriting the correct server-provided binding with no operator
action involved. Every subsequent namespace-scoped request (including
proposal generation) then legitimately failed, because it targeted a
namespace the backend was never scoped to.

**The fix:** `openIdentity()` no longer falls back to "the first
discovered namespace." It binds to a namespace only when the backend
reports a genuine default (`opened.defaultNamespace`, sourced from the
kubeconfig context's own `namespace:` field) or when a namespace already
bound before the identity was (re)opened is confirmed present in the newly
discovered list for that identity. Otherwise the Namespace control is left
unbound, and the operator must choose explicitly — see
[12-security-ux-constraints.md](12-security-ux-constraints.md).

**Re-verified live, twice, after the fix:** two full live runs of
`hack/ui-lima-demo.sh` after this fix both completed end-to-end
successfully (Observation start → evidence capture → proposal generate →
review → stale-409 → approve → independent reject-flow generate → review →
reject), with the Environment panel correctly showing the real bound
namespace and a real identity (`kind-landlock-genprof-core`, not
`UNKNOWN`) at every step. Screenshots in `screenshots/` are from these
runs.

## A second, deeper architectural finding — documented, not silently patched

While root-causing the above, a separate, pre-existing fact was confirmed
by code inspection: **in trusted-proxy/production deployment mode** (the
mode `hack/ui-lima-demo.sh` itself uses, and the mode real deployments
use), the Observation/Proposal backend (`humanObservationAPI` built inside
`enableWorkbenchAuthorizationWithResolver`, `workbench_authorization.go`)
is permanently scoped to the single namespace the process was started with
(the `--namespace` CLI flag), for the life of the process. The
`X-Environment-Namespace` header and the entire `/api/v09/environments`
selection flow are only honored for observation/proposal routing in
**local kubeconfig mode** (`forEnvironmentRequest`, gated on
`s.requestContext == nil`) — a condition that is false in trusted-proxy
mode. `handleEnvironments`/`handleEnvironmentSession`
(`environment_api.go`) are not themselves gated by deployment mode, so they
remain fully queryable in production, discovering and offering namespace
choices from the **local kubeconfig visible to the server process**,
independent of the trusted-proxy-authenticated identity or the fixed
observation namespace.

Practical effect: in production/trusted-proxy mode, the Namespace selector
can only ever safely resolve to the one namespace the deployment was
actually configured with (which the fix above ensures it now does,
correctly, by preservation rather than by a hardcoded assumption) — it
cannot legitimately be used to *switch* the observation/proposal scope to
a different namespace, even though the control does not visually
communicate that limit. This is a real product/backend design question
(should the selector be read-only/informational in this mode? should
trusted-proxy mode instead honor `X-Environment-Namespace` the way local
mode does?) that changes routing/authorization behavior, not a UI
presentation choice — per this mission's explicit instruction, it is
flagged here for a deliberate follow-up decision rather than silently
resolved in either direction within this pass.

## Full-page screenshot capture duplicates the sticky topbar

Some of the 1024px full-page captures in `screenshots/` show the sticky
topbar rendered twice, mid-page. This is a Playwright `fullPage: true`
stitching artifact with `position: sticky` elements, not a live rendering
defect — a real user scrolling the page only ever sees one topbar, pinned
to the top. Noted here and in [screenshots/README.md](screenshots/README.md)
so it isn't misread as a control-collision bug.

## No live capture of `STARTING`, or evidence-`AVAILABLE` states

`RUNNING`/`OBSERVING` **was** captured live (`screenshots/03-observation-active.png`)
in the correction round, including confirming `Generate proposal` stays
disabled throughout. The brief `STARTING` phase and an evidence result of
`AVAILABLE` (as opposed to `UNKNOWN`) were not reproduced in any capture
session — every real completed Observation on this cluster so far reported
`Evidence state unknown`. Both are implemented and covered by the
code-level analysis in [05](05-observation-state-model.md)/[06](06-evidence-model.md),
but a future visual QA pass should try to capture them directly if
screenshot evidence of those exact states is needed.

## No formal design-token spacing scale

See [07-design-system.md](07-design-system.md). The CSS is internally
consistent but uses literal pixel values, not `var(--space-*)` tokens.
