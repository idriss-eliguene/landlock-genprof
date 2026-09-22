# DOC-R2 Screenshot Specification

No screenshots were captured in DOC-R2. The Book contains seven searchable
placeholders and the following reproducible future capture contract.

| ID | Book location | Route | Required context/state | Must show | Must not show | Caption / purpose |
|---|---|---|---|---|---|---|
| operations-center-context | `operations-center.md` | `/` | bound Context, ClusterIdentity, Namespace, Ready | scope controls and status | security score or authority inferred from Ready | Canonical Operations Center and server-bound scope. |
| workload-identity | `operations-center.md` | workload dossier | exact workload and container | UID, GroupKind, namespace, container | name-only identity | Exact workload identity is server-resolved. |
| observation-evidence | `operations-center.md` | workload dossier / Observations | source-specific evidence with UNKNOWN or NOT_ESTABLISHED | independent source states and reasons | collapsed evidence or implied completeness | Evidence preserves source-level uncertainty. |
| proposal-governance | `operations-center.md` | Proposals & Governance | Proposal review state | CandidateDigestV2, ReviewContextDigestV2, provenance, governance | Candidate YAML called canonical | Review the governed candidate and review context. |
| application-qualification | `operations-center.md` | Proposal / Attempt detail | APPLIED or PARTIALLY_APPLIED plus qualification axes | ApplyAttempt/RollbackAttempt custody | Applied presented as verified | Application custody is not behavioral proof. |
| health-attention | `operations-center.md` | `/health`, `/attention` | dimension ledger and both Attention groups | states, reasons, separate taxonomies | score, severity, age, resolution workflow | Health is a ledger, not a security score. |
| exact-lineage-uncertainty | `operations-center.md` | workload Policy | mixed/insufficient provenance fixture | diagnostic and excluded association | inferred Workload-owned Proposal | No exact lineage means no inferred ownership. |

Capture requirements: real React Operations Center at `/`, real server
projections, exact Git SHA, disposable fixture identifiers, and recorded
context/session. Captions must describe semantic purpose rather than merely
repeat the page title.
