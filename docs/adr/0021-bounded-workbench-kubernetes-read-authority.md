# ADR-0021: Bounded Workbench Kubernetes read authority

## Status

Accepted

## Context

The v0.5 Cluster Workbench will read Kubernetes state using credentials
supplied by the operator. Those credentials may have broader Kubernetes
authority than the Workbench needs. Credential authority is therefore not a
claim about the application's capability.

Existing CLI commands continue to use broad client-go and dynamic-client
interfaces for legitimate operations such as applying proposals and changing
proposal status. A separate capability is required before that client model
can be used by future Workbench/domain or HTTP code.

## Decision

`internal/k8s.WorkbenchReadCapability` is the application-level read boundary
for the Workbench. Its public interface contains only explicit, bounded GET
and namespace-scoped LIST methods for resources needed by the later v0.5
gates. It exposes no client-go interface, dynamic client, arbitrary GVR,
watch, create, update, patch, delete, or status-write operation.

`ReadSession` owns the concrete core, dynamic, and discovery clients in private
fields. It constructs them once from a copied `rest.Config`; callers cannot
retrieve those clients through the capability. A session requires one
namespace, preventing accidental all-namespaces LIST access. The effective
configuration and session identity are captured at construction and are not
reloaded per request.

`NewReadSessionFromKubeconfig` resolves the selected kubeconfig/context once.
The selected context and cluster server are recorded in `ReadSessionIdentity`.
The existing `RestConfig` behavior remains unchanged for existing CLI paths,
including in-cluster configuration followed by the default local kubeconfig.

Optional CRD-backed reads perform discovery before the object read. An absent
served resource is `BACKEND_NOT_INSTALLED`; a served resource with no named
object is `NOT_FOUND`. Permission, timeout, and other API failures retain
distinct typed `ReadError` states. An empty LIST is a successful empty result.

The initial explicit read envelope is:

| Resource | GET | LIST | WATCH | WRITE |
| --- | --- | --- | --- | --- |
| Pod | yes | yes | no | no |
| Deployment | yes | no | no | no |
| StatefulSet | yes | no | no | no |
| DaemonSet | yes | no | no | no |
| ReplicaSet | yes | no | no | no |
| SecurityProfileProposal | yes | yes | no | no |
| TrainingHistory | yes | no | no | no |
| PodLock/LandlockProfile | yes | no | no | no |
| SPO SeccompProfile | yes | no | no | no |
| NetworkPolicy | no | yes | no | no |

The methods are primitives for later bounded domain reads, not workload
discovery, HTTP, UI, association, projection, or reconciliation.

## Security consequences

The invariant is:

`CredentialAuthority != WorkbenchReadCapability`.

A cluster-admin kubeconfig can still be used to construct the underlying
clients, so this ADR does not claim that the credential itself is read-only.
It establishes only that consumers receiving `WorkbenchReadCapability` have
no typed route to a Kubernetes mutation operation or to the underlying
generic client. Existing mutation paths are not replaced or weakened.

This ADR does not establish HTTP safety, browser-origin policy, least-
privilege RBAC, atomic cluster snapshots, complete workload discovery,
effective authority, enforcement, behavioral verification, or optional
backend availability.

