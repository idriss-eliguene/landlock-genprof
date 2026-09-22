# DOC-R1 Claim Safety Audit

## High-risk claims

| File / line | Claim | Status | Recommended boundary |
|---|---|---|---|
| `README.md:129-151` | Operations Center is local/read-only and cannot Approve, Reject, Apply, or Rollback. | UNSUPPORTED for the final authenticated product; may describe an older local mode. | State the deployment mode and say server authorization remains authoritative. |
| `book/src/workbench.md:1-6` | v0.8 server-rendered Workbench is the current product surface. | UNSUPPORTED / STALE. | Mark historical and link to React Operations Center. |
| `book/src/index.md:8` | v0.7 Observation Workbench is the current certified baseline. | STALE. | Replace with current product identity and link to evidence ledger. |
| `docs/operations-center-demo.md:20-22` | `/next/` is recommended React and `/` is legacy. | UNSUPPORTED. | `/` is canonical; `/next/*` is retired 410. |
| `docs/product/operations-center/frontend-migration-v1.md:40` | Legacy implementation remains in source for later deletion. | STALE after M10.9-B4. | State physical legacy removal is complete, while historical records remain. |
| `docs/product/operations-center/frontend-migration-v1.md:123` | Full SPHM explanation is reserved for M9. | STALE after M10.8/M10.9. | Describe current dimension-ledger behavior and explicit limits. |
| `README.md:196-224` | Least privilege is the product problem and goal. | PARTIALLY_SUPPORTED. | Retain as goal; do not imply completeness or minimality. |
| `README.md:266-299` | Project does not enforce; external backends do. | SUPPORTED. | Preserve and connect to current qualification boundaries. |
| `docs/PROGRESS.md:60-69` | NetworkPolicy and SPO behavioral boundaries are demonstrated. | SUPPORTED WITH SCOPE. | Keep Cilium/candidate/syscall-specific limits adjacent to claims. |
| `docs/PROGRESS.md:43-44` | No complete behavior, complete least privilege, global verification, or transactional apply. | SUPPORTED LIMITATION. | Make this current baseline language, not only v0.7 history. |

## Claim classes searched

- Minimal / least privilege: valid as an objective and bounded recommendation;
  unsafe when presented as universally complete.
- Complete observation / coverage: not established globally; source and
  population bounds must remain explicit.
- Secure / protected: unsafe as a universal workload verdict.
- Verified / enforced: valid only when tied to named backend and evidence;
  Applied must not be used as shorthand.
- Continuous drift detection / autonomous remediation / autonomous approval:
  roadmap or unsupported; not current product claims.
- Universal policy / universal IR: unsupported. `BehaviorProfile` is an
  internal cross-domain IR; candidate-v2 and candidate-v1 are not the same
  public governance identity.
- Landlock enforcement: artifact generation/application is demonstrated;
  kernel denial is explicitly not proven.

## Audit result

The repository contains many good negative boundaries, especially in
`README.md`, `docs/PROGRESS.md`, `docs/G5-CERTIFICATION.md`, and the ADRs. The
primary risk is contradictory current-vs-historical presentation, not a total
absence of claim safety.
