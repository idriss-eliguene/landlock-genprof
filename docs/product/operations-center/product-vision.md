# Product vision

Landlock-genprof turns observed Kubernetes workload behavior into reviewable,
evidence-bound security proposals. The Operations Center helps platform,
DevSecOps, and security teams move from “what is running?” to “what was
actually observed?” and then to an auditable governance decision.

The v0.9 product is a local-kubeconfig, single-operator workspace with real
Kubernetes environments, namespace-scoped authorization, observations,
capability facts, proposals, history, attention, and governance. It is not a
SaaS control plane.

Trust model: credentials and Kubernetes clients stay in the backend;
Kubernetes RBAC/SSAR authorizes actions; observation provenance and candidate
digests remain authoritative; stale governance state is rejected.

Non-goals for v0.9 are OIDC/SSO, remote agents, fleet management,
multi-tenancy, billing, cloud credential vaulting, and cross-cluster
transactions. They may be future directions, not current behavior.
