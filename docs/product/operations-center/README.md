# Operations Center

This package describes the Operations Center UX implemented in the current
React migration. It is the engineering reference for the environment-aware
Kubernetes workflow; the public user guide is in `book/src/operations-center.md`.

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
| Cluster/context separation | Context names were mistaken for clusters | Dependent Cluster, Identity, Namespace controls | `web/operations-center/src/app`, server projections | environment tests, browser qualification |
| Workload-first observations | UUIDs were not operator identities | Workload cards with evidence and fact counts | `web/operations-center/src/features/workloads` | UI/value-flow tests |
| Evidence UNKNOWN | Uncertainty looked like success | Explicit explanation and technical disclosure | `web/operations-center/src/features` | evidence tests |
| Restricted discovery | Forbidden list looked like an empty cluster | Explicit namespace entry | M2 API and Environment panel | RBAC demo |
| Progressive disclosure | Technical richness overwhelmed operators | Operator, security, forensic levels | Environment/details and observation cards | visual qualification |
