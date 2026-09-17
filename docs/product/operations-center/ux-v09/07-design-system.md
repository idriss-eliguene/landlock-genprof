# 07 — Design system

The Operations Center's design system is a single embedded CSS block in
`workbenchClusterPageTemplate` (`cmd/landlock-genprof/workbench.go`). It is
intentionally not a separate build pipeline — this is a Go binary serving a
read-only local console, and a bundler/design-token pipeline would be
scope the product doesn't need. The system below is what's actually in that
CSS, documented so it can be extended consistently rather than by
convention-guessing.

## Tokens (CSS custom properties, `:root`)

```
--surface / --surface-subtle / --surface-raised   backgrounds
--nav / --nav-hover / --nav-selected              sidebar
--text / --text-muted / --text-inverse            typography
--border / --border-strong                        dividers, control borders
--primary / --primary-hover                        brand/action color
--success / --success-surface
--warning / --warning-surface
--danger / --danger-surface
--unknown / --unknown-surface
--focus                                            focus ring color
```

Every status color has a paired `-surface` token (a light background tint)
so badges and banners get a consistent fill/foreground pairing instead of
ad hoc colors.

## Typography

- Body: `16px/1.45 system-ui, -apple-system, sans-serif`.
- Page `<h1>` (topbar): 22px. View `<h2>`: 24px. Card `<h3>`: inherits (bold
  via context). Metric numbers (`summary-card .metric`): 26px/750 weight.
- Eyebrow labels (`.eyebrow`, `.context-chip strong`): 12–13px, uppercase,
  `.04–.08em` letter-spacing, `--text-muted`.

## Spacing

No numeric spacing scale was formalized as tokens (the CSS uses literal
`8px`/`12px`/`14px`/`16px`/`24px` consistently by convention rather than
`var(--space-*)`). This pass did not introduce a token scale — doing so
would mean rewriting every rule in the block for no behavior change and
was judged out of scope for a redesign focused on structure and state.
Documented here as the honest current state, not as a target already met.

## Buttons

```
button           neutral outline: 1px var(--border-strong), 6px radius,
                 8px/12px padding, hover → --primary border + light tint
.primary-action  solid --primary fill, --text-inverse text, 650 weight
                 (added this pass — see 04-interaction-model.md)
.danger-action   --danger outline + text, --danger-surface on hover
:disabled        opacity .62, cursor not-allowed (never removed from DOM
                 when semantically meaningful — see 04-interaction-model.md)
```

Only one `.primary-action` should be visible per decision point. Danger
actions (`Stop observation`, `Reject`) stay outline-style, not solid red —
they're consequential, not the primary path.

## Status badges

`.status-badge` / `.outcome-badge`: pill, 1px `currentColor` border, bold.
Semantic classes: `.success` (green), `.partial`/`.degraded` (amber),
`.failed`/`.unavailable` (red), `.unknown`/`.not-eligible` (grey). Mapping
from raw backend tokens is centralized in `stateClass()` — new states
default to `.unknown` rather than silently falling through to `.success`.
This default-to-unknown behavior is load-bearing for
[06-evidence-model.md](06-evidence-model.md) and must not be changed to
default-success.

## Cards, panels, tables

- `.panel`/`.card`: white surface, 1px border, 8px radius; `.card` adds a
  1px soft shadow.
- `.data-table`: left-aligned, muted header row, 1px row dividers,
  horizontally scrollable wrapper (`.table-wrap`) rather than squeezed
  columns at narrow widths.
- `.attention-item`: 4-column grid ≥1100px (category / reason / impact /
  disposition), collapses to 2 columns ≤1100px, 1 column ≤700px.

## Empty / unavailable / degraded / stale states

Four distinct, non-interchangeable visual states, each with its own class
and always with an explicit label prefix in the copy (`EMPTY —`,
`UNAVAILABLE —`, `Projection DEGRADED`, stale-state red on 409):

```
.empty-state         neutral surface — "nothing here, read succeeded"
.unavailable-state    danger surface  — "the read itself failed"
.degraded-state       warning surface — "partial/malformed data excluded"
.stale-state           danger surface — "your decision was against old state"
```
Never substitute one for another — an `UNAVAILABLE` read must not render as
`EMPTY` (see the explicit `"UNAVAILABLE — read failed; no empty state
substituted."` strings in `workbench_ui.go`).

## Labeled form fields

`.form-field`: label stacked above its control (7px gap), added in the
correction round to fix a real collision where a label and `<select>` had
no spacing between them at all. Any single-control field (a picker, a
search box) should use this instead of placing a bare `<label>` and
control adjacent in markup with no wrapper.

## Operational state cards

`.observation-lifecycle-card`: a bordered card with a 4px colored left
edge and a tinted background, one class per lifecycle state
(`.state-empty`/`.state-ready`/`.state-active`/`.state-completed`/`.state-failed`).
Used for the Observations view's lifecycle control so the current
operational state reads as a state, not a caption above a button row —
see [04-interaction-model.md](04-interaction-model.md). This is the first
use of a colored-border state-card pattern in the design system; a future
surface that needs to communicate "what state is this thing in, right now"
prominently should reuse it rather than inventing a new treatment.

## Progressive disclosure

`<details>`/`<summary>` is the one mechanism used throughout — "Environment
details," "Technical observation details," "Evidence details," "Technical
metadata." No custom modal/drawer/tooltip components were introduced; this
keeps disclosure keyboard-accessible for free (native `<details>` toggles
on Enter/Space and is announced by screen readers without extra ARIA).

## Focus and responsive rules

`button:focus-visible, select:focus-visible { outline: 3px solid
var(--focus); outline-offset: 2px }` — applied globally, not per-component.
Responsive breakpoints in the shipped CSS: `max-width:1100px` (sidebar
narrows, summary/attention grids reflow to 2 columns) and `max-width:700px`
(sidebar becomes static/top, nav becomes a 2-column grid, all multi-column
grids collapse to 1 column). Verified visually at 1440/1280/1024 in this
pass — see [screenshots/README.md](screenshots/README.md).
