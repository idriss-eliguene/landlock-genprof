# DOC-R1 Documentation Remediation Backlog

## P0 — technically false or dangerous

| Work | Files | Estimate |
|---|---|---|
| Correct current Operations Center action/read-only description and link | `README.md`, `book/src/workbench.md`, `book/src/cli/landlock-genprof_ui.md` | 1 focused pass |
| Remove retired `/next/` and legacy-root instructions | `docs/operations-center-demo.md`, demo-facing Book pages | 1 focused pass |
| Prevent historical Workbench from presenting as current product | `book/src/index.md`, `book/src/workbench.md`, `book/src/observation-semantics.md` | 1 chapter rewrite |

## P1 — missing current capability

| Work | Files | Estimate |
|---|---|---|
| Regenerate CLI reference and add current commands to summary | `book/src/cli/*`, `book/src/SUMMARY.md` | 1 generation/update pass |
| Add public Operations Center guide | new Book chapter plus links from Start Here/workflow | 1 chapter |
| Document Health/SPHM, Attention taxonomies, History boundedness, and exact lineage | new chapter or integrated guide | 1 chapter |
| Document candidate-v1 versus candidate-v2 and separate digests | workflow, architecture, usage | 1 focused pass |

## P2 — confusing or stale UX

| Work | Files | Estimate |
|---|---|---|
| Mark milestone/migration documents historical or current | `docs/product/operations-center/*`, `docs/PROGRESS.md`, release records | 1 editorial pass |
| Update screenshot README and capture plan for final root/Health/context | `docs/product/operations-center/ux-v09/screenshots/*` | 5–7 real captures |
| Clarify CLI design document as design history, not current command reference | `docs/cli-design.md` | 1 short edit |

## P3 — discoverability

- Add a Book “What is landlock-genprof?” lifecycle diagram tied to current
  terms.
- Link stable Operations Center semantics from the public Book.
- Add explicit Troubleshooting and failure-state navigation.
- Add a current evidence/qualification matrix near the public workflow.

## P4 — optional polish

- Harmonize headings and version labels.
- Add generated-document freshness checks to CI.
- Add link checking and command-reference drift checks.
- Add alt text/captions for the final screenshot set.
