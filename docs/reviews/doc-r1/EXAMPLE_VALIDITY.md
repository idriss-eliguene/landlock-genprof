# DOC-R1 Example Validity Audit

Examples were compared against the generated Cobra reference and source
command definitions without mutating a cluster.

| Example family | Locations | Classification | Finding |
|---|---|---|---|
| `doctor` | README, Start Here, workflow | VALID | Current command and purpose match source. |
| `trace --pod --namespace --binary --duration` | README, Book index/workflow, usage | VALID / ENVIRONMENT-DEPENDENT | Flags match; requires the supported cluster, executor, and source prerequisites. |
| `review <proposal>` | README, workflow, usage | VALID | Current command and digest-review role match. |
| `approve <proposal> --expected-digest` | README, workflow, usage | VALID | Exact digest binding is documented correctly. |
| `apply-proposal <proposal>` | README, workflow, usage | VALID | Current command exists; approval/readiness semantics must remain adjacent. |
| `rollback <attempt> --namespace` | README, workflow | VALID | Current command exists and custody semantics are documented. |
| `ui --namespace` | README, Book, CLI page | PARTIAL / STALE | Syntax is valid, but the UI description says Workbench/read-only and does not describe final React/authenticated behavior. |
| `observe` | source only | MISSING | Current command has no user-facing Book example. |
| `executor` | source only | MISSING / DEVELOPER | Current command is absent from Book; classify its intended audience before publishing. |
| `synthesize`, `verify`, `explain`, `diff`, `export` | CLI reference and usage pages | VALID / PARTIAL | Syntax is generated and mostly aligned; relationship to complete Proposal/candidate-v2 semantics needs explicit wording. |
| `kubectl apply` examples in install/deploy docs | INSTALL, deployment docs | DANGEROUS_TO_RUN without environment review | Commands are technically plausible but must retain warnings about cluster scope, RBAC, CRDs, and disposable environments. |

No examples were executed against a cluster during this audit.
