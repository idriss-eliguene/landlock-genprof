# DOC-R1 Book Information Architecture Review

## Current sequence

The current Book is:

```text
Home → Start Here → disposable cluster / install / demo → governed workflow
→ observation semantics → Workbench → usage and CLI reference
→ architecture / security / roadmap
```

This is coherent for the historical CLI/Workbench product, but not for the
final React migration.

## Recommended disposition

| Current chapter / group | Action | Reason |
|---|---|---|
| `index.md` | REWRITE | Replace v0.7 Workbench status with current product identity and explicit demonstrated limits. |
| `start-here.md` | KEEP + UPDATE | Add “Use Operations Center” path and canonical `/` access. |
| `workflow.md` | KEEP + UPDATE | Preserve lifecycle and claim boundaries; add Operations Center as a view/governance surface. |
| `observation-semantics.md` | KEEP + LABEL | Retain normative semantics; remove current-product Workbench framing. |
| `workbench.md` | REWRITE / RENAME | Convert into current Operations Center user guide or mark fully historical. |
| `docs/usage.md` | KEEP + UPDATE | Add current browser and CLI relationship; clarify candidate-v1/v2 boundary. |
| `cli/*` | REGENERATE | Source-generated command pages are out of sync. |
| `docs/product/operations-center/*` | KEEP | Internal reference set; link selected stable chapters from public guide. |
| `docs/architecture.md` | KEEP + UPDATE | Preserve architecture; clarify current lifecycle and digest terminology. |
| `project/progress.md` | KEEP + UPDATE | Make current certified baseline prominent; separate historical v0.7 ledger. |
| `project/roadmap.md` | KEEP | Explicitly mark roadmap intent versus current capability. |
| `demo/*` | KEEP + UPDATE | Correct retired `/next/` and legacy-root instructions. |
| `docs/adr`, `docs/rfc` | KEEP | Add historical/superseded labels where needed; do not erase decision history. |

## Proposed public sequence

```text
What is landlock-genprof?
  → Install / choose environment
  → First workload
  → Observe and understand evidence
  → Derive a candidate
  → Review and govern a SecurityProfileProposal
  → Apply and inspect custody
  → Qualify backend-specific behavior
  → Operations Center
  → Troubleshooting
  → CLI / API reference
```

The Operations Center chapter should be adjacent to the governed workflow,
not buried under historical Workbench material.
