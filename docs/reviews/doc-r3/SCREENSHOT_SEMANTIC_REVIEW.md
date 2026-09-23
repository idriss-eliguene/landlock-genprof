# DOC-R3 Screenshot Semantic Review

| ID | Claims visible | Safety review |
|---|---|---|
| operations-center-context | Context, ClusterIdentity, Namespace, session, Ready, Unknown, attention | Ready is shown as binding status; no authority or security-score claim is made. |
| workload-identity | Namespace, Deployment, container, workload representation | The caption states that names alone are not ownership. |
| observation-evidence | Completed lifecycle, source evidence, `Evidence state unknown` | Unknown is preserved and is not presented as failure or success. |
| proposal-governance | Candidate content, evidence qualification, aggregate provenance, governance state, derived YAML | Candidate-v2 and derived YAML remain distinct; no CLI candidate-v1 equivalence is implied. |
| application-qualification | `NOT_AUTHORIZED` Apply reason | Capability projection is not treated as server authority; Apply is not Verified. |
| health-attention | SPHM read failure with explicit no-inference message | The image does not imply a security score, Healthy state, freshness, or severity. |
| exact-lineage-uncertainty | Aggregate provenance and incomplete qualification context | Aggregate provenance is not promoted to exact workload ownership. |

Global checks:

- Applied = Verified: **NO**
- Structural = Behavioral: **NO**
- Health = security score: **NO**
- UI capability = authorization: **NO**
- UI refresh = semantic freshness: **NO**
- CLI candidate-v1 = Operations Center candidate-v2: **NO**
- Evidence source = behavior domain: **NO**
- Backend artifact = Proposal: **NO**
- Missing lineage = inferred Proposal: **NO**
- Legacy Workbench or `/next/*`: **NONE**
