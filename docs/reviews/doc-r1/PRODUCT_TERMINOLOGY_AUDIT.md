# DOC-R1 Product Terminology Audit

## Current positioning

The strongest current product language appears in `README.md`:
“Evidence-driven Least-Privilege Governance for Kubernetes,” with the model
`Workload → Observe / Ingest → Attribute → Derive → Govern → Apply → Verify`.
That is materially compatible with the defensible category:

> Governed policy lifecycle for Kubernetes workloads.

The repository nevertheless presents several competing eras: Landlock profile
generator, Observation Workbench, v0.7 technical baseline, v0.8 Governance
Operations, v0.9 Operations Center, and M10.9 final React migration.

## Findings

| Location | Current wording / framing | Assessment |
|---|---|---|
| `README.md:1-40` | Evidence, governance, candidate-v2, Operations Center | CURRENT direction, but “read-only” is too broad for the current authenticated product. |
| `book/src/index.md:8` | “v0.7 introduces the Observation Workbench” | STALE front-door positioning. |
| `book/src/observation-semantics.md:3` | “v0.7 is an Observation Workbench” | Useful semantic record, not suitable as current product introduction without historical label. |
| `book/src/workbench.md:1-6` | v0.8 server-rendered Governance Operations Workbench | STALE user-facing product identity. |
| `docs/product/operations-center/product-vision.md` | v0.9 local-kubeconfig workspace | PARTIAL; useful internal product intent, not final M10.9 identity. |
| `docs/product/operations-center/information-architecture.md` | final six-item L1 shell and authority model | CURRENT semantic source, but not connected from the Book. |
| `docs/PROGRESS.md` | demonstrated capabilities and explicit limitations | CURRENT evidence ledger, though v0.7 heading and old SHA framing need a current-baseline section. |

## Vocabulary that is safe and should be retained

- Observation is not authorization.
- Evidence is source-specific and can be `EMPTY`, `AVAILABLE`, or `UNKNOWN`.
- A `SecurityProfileProposal` is the governed review object.
- CandidateDigest and ReviewContextDigest are distinct.
- Applied, Enforced, and Verified are distinct claims.
- Operations Center is an operational view, not enforcement authority or a
  universal IR.

## Vocabulary requiring remediation

- “Workbench” should be historical or replaced by “Operations Center” in
  current user-facing chapters.
- “Read-only” must be qualified by deployment/authentication mode; it is false
  as a blanket description of the current authenticated Operations Center.
- “v0.7”, “v0.8”, and “v0.9” should be release/history labels, not current
  product identity.
- “Health”, “Attention”, and “History” need current semantic definitions,
  especially unknown, not-established, bounded, and non-score behavior.
