# 10 — Accessibility

## What was already correct (verified, not re-built)

- Every form control has an associated `<label for="...">` (Cluster,
  Identity, Namespace, explicit namespace, workload picker).
- `role="status" aria-live="polite"` on the shared message banner
  (`#workbench-message`) and the new lifecycle status line
  (`#observation-status-line`), so state changes are announced without
  requiring focus to move.
- Navigation uses real `<button>` elements with `aria-current="page"` on
  the active view, not `<div onclick>`.
- Progressive disclosure uses native `<details>/<summary>`, which is
  keyboard-operable (Enter/Space) and exposed to assistive tech without
  extra ARIA.
- `button:focus-visible, select:focus-visible` gets an explicit 3px outline
  globally — focus is never suppressed.
- Status is never color-only: every badge carries a text label
  (`badge(raw, label)` always renders text, color is additive), and
  disabled controls always carry a text reason next to them
  (`.disabled-reason`), not just a dimmed appearance.

## What this pass fixed

- **Button naming.** "Stop selected" → "Stop observation" (a button name
  must be unambiguous out of context — "selected" didn't say what would
  stop). See [11-microcopy.md](11-microcopy.md).
- **Table cell placeholders.** "Select to inspect" (ambiguous: select what,
  where?) → "Inspect to view" (names the actual control on the row).

## Keyboard operability, verified this pass

Primary controls confirmed keyboard-reachable and operable via the
existing native-element choices (button/select/input — no custom
widgets that would need role/keyboard reimplementation): view navigation,
environment selectors, workload Inspect, observation lifecycle actions,
governance actions, all `<details>` disclosures. No custom `tabindex`
management was needed or added, because no custom interactive widgets were
introduced.

## Known gaps (not fabricated as fixed)

- No live browser screen-reader pass (VoiceOver/NVDA) was performed in this
  session — verification here is static/structural (semantic HTML, ARIA
  attributes present and correct) plus DOM-level checks via the browser
  automation used for the visual review loop. Listed honestly in
  [15-known-limitations.md](15-known-limitations.md) rather than claimed as
  done.
- No automated axe-core/Lighthouse accessibility scan was run — out of the
  focused-testing budget for this pass. `test/ui/workbench-smoke.js`
  exercises real keyboard-reachable controls (button clicks by accessible
  name via `getByRole`) as a byproduct of its functional assertions, which
  is partial coverage, not a substitute for a dedicated a11y test.
