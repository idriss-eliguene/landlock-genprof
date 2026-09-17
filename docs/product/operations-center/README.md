# Operations Center

This package describes the Operations Center UX implemented in v0.9. It is
the product reference for the local, environment-aware Kubernetes workflow.

## Start here

- [Product vision](product-vision.md)
- [Information architecture](information-architecture.md)
- [Observation workflow](observation-workflow.md)
- [Evidence UX](evidence-ux.md)
- [Commercial demo](acceptance/commercial-demo.md)
- [UX acceptance](acceptance/ux-acceptance.md)

The product uses `Cluster -> Identity / Context -> Namespace`. Credentials
remain server-side; Kubernetes RBAC and SSAR remain authoritative.

## Traceability

| Decision | User problem | Product solution | Implementation | Test |
|---|---|---|---|---|
| Cluster/context separation | Context names were mistaken for clusters | Dependent Cluster, Identity, Namespace controls | `workbench.go`, `workbench_ui.go` | environment tests, browser demo |
| Workload-first observations | UUIDs were not operator identities | Workload cards with evidence and fact counts | `workbench_ui.go` | UI/value-flow smoke |
| Evidence UNKNOWN | Uncertainty looked like success | Explicit explanation and technical disclosure | `workbench_ui.go` | evidence tests |
| Restricted discovery | Forbidden list looked like an empty cluster | Explicit namespace entry | M2 API and Environment panel | RBAC demo |
| Progressive disclosure | Technical richness overwhelmed operators | Operator, security, forensic levels | Environment/details and observation cards | visual qualification |
