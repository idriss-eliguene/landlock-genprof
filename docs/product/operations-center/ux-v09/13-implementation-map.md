# 13 — Implementation map

## Durable Observation Stop (current implementation)

- `internal/observation/domain/observation.go` models Stop as a durable
  execution control intent without adding a synthetic lifecycle state.
- `internal/observation/kubernetes/executor.go` persists that intent with
  status resourceVersion CAS and exposes a claim-fenced read for executors.
- `internal/observation/executor/worker.go` receives the Runner's actual
  claim, watches the durable intent, and cancels only its local Runner.
- `internal/observation/runtime/filesystem.go` notifies the owning executor
  after claim acquisition; all drain, attribution, evidence and finalization
  remain in the existing Runner path.
- `cmd/landlock-genprof/observation_api.go` authorizes authenticated Stop
  through `observation.operate`, validates the selected cluster/namespace,
  and returns authoritative state. The UI polls the existing read model and
  presents a stopping state without optimistic completion.

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

## Correction round (post-review, same PR)

A human review of the live UI rejected the UX acceptance claimed by the
first pass above. This section is the diff on top of it.

**`cmd/landlock-genprof/workbench_ui.go`**
- **Bug fix, namespace auto-bind:** `openIdentity()` no longer falls back
  to "the first discovered namespace" when the opened identity's
  kubeconfig context has no explicit default. It uses
  `opened.defaultNamespace` when the backend provides one, otherwise
  preserves the namespace already bound before the identity was (re)opened
  *only if* that namespace is still present in the newly discovered list;
  otherwise it leaves the Namespace control unbound and requires an
  explicit operator choice. Root cause and full incident writeup:
  [15-known-limitations.md](15-known-limitations.md).
- **Bug fix, stale `selectedObservation` across workload/lifecycle
  changes:** `renderObservationActions()`'s `Generate proposal` gate now
  additionally requires (a) no active (non-terminal) Observation for the
  selected workload, and (b) the opened `selectedObservation` actually
  belongs to that workload's own Observation list. Switching workloads
  (`picker.onchange`, the Workloads table's **Inspect** button) now
  explicitly clears `selectedObservation` first. Previously, an operator
  could open a `COMPLETED` Observation for review, start a *new*
  Observation, and see `Generate proposal` stay enabled/primary the whole
  time the new one was `RUNNING`. [05-observation-state-model.md](05-observation-state-model.md)
- Removed the redundant runtime relabeling
  (`workloadLabel.textContent = "Workload"`) in favor of the correct label
  text authored directly in the template (see below) — a JS-rewrites-the-DOM-
  label pattern that had no reason to exist once the label could just say
  the right thing.
- Error UX: `get()` now always populates a usable `reason` even when a
  response is non-JSON or empty (falls back to raw response text, then
  `statusText`, then `"HTTP {status}"`), so `showError()` is never left
  with nothing but its generic headline. `showError()`'s headline is now
  state-aware (`errorHeadline()`): `OBSERVATION_NOT_COMPLETED`,
  `NOT_AUTHORIZED`/401, `NOT_FOUND`, and `NO_CANDIDATE` each get a specific
  sentence instead of the generic "Authoritative read/action failed."; the
  generic fallback itself now always includes the HTTP status/code when
  available rather than a bare, contentless sentence.

**`cmd/landlock-genprof/workbench.go`**
- Workload picker: replaced the adjacent, unstyled `<label>` + `<select>`
  (which rendered visually glued together, e.g. `WorkloadDeployment/api ·
  api`) with a `.form-field` wrapper (label stacked above control, 7px
  gap) — the same structural pattern already used correctly for
  Cluster/Identity/Namespace, applied here for the first time. The label
  text itself is now the literal, correct "Workload" (see above).
- Observations view: the lifecycle control is now a visually distinct
  state card (`.observation-lifecycle-card`, colored left border + tinted
  background per state: empty/ready/active/completed/failed) with a
  workload-identity header (name, kind, namespace, container) inside it,
  instead of a bare status line + button row. This directly addresses "the
  observation surface still looks like a form with buttons."
  [04-interaction-model.md](04-interaction-model.md)

**`cmd/landlock-genprof/observation_api.go`**
- Added a distinct `OBSERVATION_NOT_COMPLETED` error class ahead of the
  generic `"conflict"` substring match, so proposal-generation-on-an-
  incomplete-Observation is never presented with the same "state changed,
  refresh and retry" semantics as an actual CAS staleness conflict — the
  two failures have different, non-overlapping remedies. No HTTP status
  code changed (still 409); no CAS/authorization logic touched.

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
