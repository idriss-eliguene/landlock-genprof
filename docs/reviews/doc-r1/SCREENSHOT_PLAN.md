# DOC-R1 Screenshot Audit and Plan

## Existing inventory

The repository has real React-era captures under
`docs/product/operations-center/ux-v09/screenshots/`: overview, environment,
workloads, observation ready/active/completed-unknown, proposal governance,
history, attention, and responsive proposal/observation captures. They are
real Playwright captures, not synthetic images.

They are nevertheless incomplete for the final product: no dedicated final
Health/SPHM ledger, no final branded canonical-root shell, no clear context
switch transition, no explicit lineage diagnostic, and several README/capture
instructions still call the application Workbench or use old demo routes.

No dedicated server-rendered legacy screenshot set was found. The existing
set is best classified as `CURRENT_REACT=PARTIAL`, `LEGACY_SCREENSHOTS=NO
DIRECT LEGACY ASSET / STALE TERMINOLOGY PRESENT`.

## Minimal future set

| Screenshot | Purpose | Page / state | Reader learns | Semantic risk | Caption / reproduction |
|---|---|---|---|---|---|
| 1 | Product identity and scope | Home, bound Context/Cluster/Namespace, 1440 | Operations Center is the canonical operational view | Could imply overall health if overdesigned | “Canonical Operations Center and server-bound operational context”; real authenticated root. |
| 2 | Exact workload identity | Workload dossier Overview, UID visible | Context, namespace, workload UID, container are distinct | Avoid calling locator a durable identity | “Exact workload identity is server-resolved”; disposable workload. |
| 3 | Evidence uncertainty | Observation detail with per-source EMPTY/AVAILABLE/UNKNOWN | Evidence is source-specific and uncertainty is visible | Do not imply complete behavior | “Observation evidence preserves source-level uncertainty.” |
| 4 | Governance identity | Proposal Review with CandidateDigest, ReviewContextDigest, provenance, qualification | Candidate content and review context are separate | Do not call YAML canonical | “Review the governed candidate and its review context.” |
| 5 | Custody / qualification | Approved Proposal with ApplyAttempt and structural/behavioral distinction | Applied is not Verified | Avoid showing a green enforcement conclusion | “Application custody and independent verification axes.” |
| 6 | Health and Attention | Health ledger plus separately grouped reconciliation/SPHM Attention | Health is not a score; taxonomies are distinct | Avoid severity or security-grade visuals | “Dimension-ledger health and bounded diagnostics.” |
| 7 | Exact-lineage uncertainty | Partial lineage / mixed identity diagnostic | Missing or mixed provenance is not ownership | Ensure no false association is shown | “Partial lineage remains explicit”; controlled malformed/mixed fixture. |

All captures should be from the real React Operations Center, current `/`,
with exact Git SHA and fixture state recorded. Do not reuse a screenshot as
proof of a semantic state it does not visibly establish.
