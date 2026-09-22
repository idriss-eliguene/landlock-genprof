# DOC-R3 Screenshot Capture Register

| ID | File | Route | Context/workload | State | Authentic | Book location |
|---|---|---|---|---|---|---|
| operations-center-context | `book/src/assets/operations-center/doc-r3/operations-center-context.png` | `/` | `kind-landlock-genprof-core / payments` | Ready; server-bound session; Health Unknown; active attention | YES — recaptured DOC-R3 | `book/src/operations-center.md` |
| workload-identity | `book/src/assets/operations-center/doc-r3/workload-identity.png` | workload dossier / Observations | `kind-landlock-genprof-core / payments / Deployment api / container api` | real workload YAML and Observation records | YES — prior canonical Playwright capture | `book/src/operations-center.md` |
| observation-evidence | `book/src/assets/operations-center/doc-r3/observation-evidence.png` | workload dossier / Observations | disposable `ui-lima-demo-42463` fixture | Completed Observation; capability evidence Unknown | YES — prior canonical Playwright capture | `book/src/operations-center.md` |
| proposal-governance | `book/src/assets/operations-center/doc-r3/proposal-governance.png` | Proposal/Governance | `payments / api` | candidate-v2 decision surface; evidence/provenance/governance visible | YES — prior canonical Playwright capture | `book/src/operations-center.md` |
| application-qualification | `book/src/assets/operations-center/doc-r3/application-qualification.png` | Proposal/Governance | disposable qualification fixture | Apply capability explicitly `NOT_AUTHORIZED` | YES — prior canonical Playwright capture | `book/src/operations-center.md` |
| health-attention | `book/src/assets/operations-center/doc-r3/health-attention.png` | Health | `kind-landlock-genprof-core / payments` | authoritative SPHM read failure; no inferred health | YES — recaptured DOC-R3 | `book/src/operations-center.md` |
| exact-lineage-uncertainty | `book/src/assets/operations-center/doc-r3/lineage-uncertainty.png` | Proposal/Governance | `payments / api` | aggregate provenance and qualification limitations visible | YES — prior canonical Playwright capture | `book/src/operations-center.md` |

All seven files are PNGs from the real React UI. No image was generated,
redrawn, composited, or text-edited. The mixed capture dates are disclosed
because the current live harness did not produce a stable workload projection
after its stale bundled smoke selector failed; the reused frames remain the
repository's existing real-browser evidence.
