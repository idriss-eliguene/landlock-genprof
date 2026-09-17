# 13 — Implementation map

Full file-level diff for this pass, with rationale. Base:
`origin/master` at `0245ea7a` (PR #255, "feat(m4): make Operations Center
V2 commercially demonstrable").

## Product/UI code

**`cmd/landlock-genprof/workbench.go`** (HTML shell + CSS template)
- Removed the nav's `Governance` button; renamed `Proposals` →
  `Proposals & Governance`. [03](03-information-architecture.md)
- Removed the orphaned `#environment-view` section (dead markup, never
  shown; no nav button pointed at it).
- Removed the `#governance-view` section and its `#open-proposals`
  shortcut button (content consolidated into Proposals).
- Observations view: replaced the static three-button action group with
  the lifecycle status line + state-driven controls.
  [04](04-interaction-model.md)
- Added `.primary-action`, `.observation-lifecycle`, `.lifecycle-status`,
  `.danger-action:hover` CSS rules.
- Added the missing `.observation-status-line` CSS rule (previously
  unstyled — badge and "Observation finalized" text rendered without a
  gap; a real control-collision bug found in the visual review loop).
  [15](15-known-limitations.md)
- Microcopy: view headings/descriptions per [11](11-microcopy.md).

**`cmd/landlock-genprof/workbench_ui.go`** (client script)
- Removed dead/duplicate first definitions of `renderOperationsContext`,
  `loadEnvironmentContexts`, `renderObservation`, `renderObservationList` —
  each immediately shadowed by a later declaration of the same name and
  never executed (confirmed via `go build`/`go test` before and after; no
  behavior change from removal). [08](08-component-inventory.md)
- Removed the `#open-proposals` click binding (element removed).
- Trimmed `renderEnvironment()` to stop building the dead per-subject
  card list; kept and preserved its live side effect (populating
  `#attention-list`).
- Added `activeObservationFor()`, `renderObservationActions()`
  (state-driven lifecycle control) and wired it into `loadSelected`,
  `clearResourceState`, the "View evidence" per-card handler, and the
  three action-button click handlers. [04](04-interaction-model.md),
  [05](05-observation-state-model.md)
- Rewrote the Stop-observation handler to target the workload's currently
  active observation directly (`stopBtn.dataset.observationId`) instead of
  requiring the operator to have manually selected an observation first.
- Rewrote the Generate-proposal top-bar handler's guard to require the open
  observation be `COMPLETED` (was: merely "selected"), with a clearer
  error message when it isn't.
- **Bug fix:** `identities.options[0]?.contextName` → `?.value`.
  `HTMLOptionElement` has no `.contextName` property, so the Identity
  selector's default-binding expression always evaluated to `""`,
  silently short-circuiting `openIdentity()`'s `if (!identities.value)
  return;` guard on every fresh page load. Found via the mandated visual
  review loop (Identity chip showed "UNKNOWN" against a fully-bound
  session) and confirmed via direct DOM inspection against the pre-fix
  build. [15](15-known-limitations.md)
- Added `Behavioral verification: UNKNOWN` disclosure to observation
  forensic details. [06](06-evidence-model.md)
- Microcopy changes per [11](11-microcopy.md).

## Tests

**`cmd/landlock-genprof/workbench_g7_test.go`,
`cmd/landlock-genprof/workbench_g8_test.go`**
- Updated string-presence assertions that encoded the old IA/copy as a
  literal-text contract (`data-view="governance"` count, `"Observations /
  Evidence"`, `contextChip("Platform"` call-site literal) to match the
  intentional redesign. No assertion was weakened or removed to make a
  test pass — each was replaced with an equivalent check against the new,
  intentional implementation (e.g. `contextChip("Platform"` →
  `["Platform",platform.status` since the label is now built from a
  label/value array instead of a literal call site).
- `TestG7WorkbenchScriptPreservesCertifiedBoundaries` and
  `TestWorkbenchV08NavigationAndSemanticBoundaries` needed **no changes** —
  their required strings ("Rebinding environment", "Behavioral
  verification", "malformed Observations remain visible") now pass because
  the microcopy fixes in this pass restored or improved on the exact
  phrases they were guarding, which is a useful signal that those guardrail
  tests were protecting real intent, not just literal text.

## Repo hygiene

**`.gitignore`** — added `/test/ui/node_modules/` (Playwright deps, not
meant to be committed; were previously untracked-but-unignored).

**`hack/lib-core-readiness.sh`, `hack/ui-lima-demo.sh`** — pre-existing
uncommitted local edits found on the source branch before this pass began
(VM control-plane recovery bounding, Playwright auto-install, Gadget
inotify capacity, non-exiting demo workload command). Not authored in this
session; carried forward because they are exactly the tooling this pass's
mandated visual-review loop needed and were otherwise going to be lost.
Reviewed and used as-is; no further changes made to them.

## Not changed (and why)

No file under `internal/`, `deploy/`, `internal/proposal/`, `internal/authz/`,
or any CRD/RBAC manifest was touched. No backend read-model shape changed.
See [12-security-ux-constraints.md](12-security-ux-constraints.md) for the
full invariant list this was checked against.
