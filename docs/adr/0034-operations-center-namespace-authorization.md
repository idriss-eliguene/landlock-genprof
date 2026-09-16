# ADR-0034: Operations Center namespace authorization

## Status

Accepted for v0.9 M2.

## Decision

Namespace access is evaluated through the effective Kubernetes client owned by
an `EnvironmentSession`. `DiscoverNamespaces` performs a Namespace LIST. A
`403 Forbidden` is a supported `EXPLICIT_ONLY` result, not an empty success,
permission grant, or fatal connector failure.

An explicitly selected namespace does not require Namespace LIST or Namespace
GET. `ExplicitNamespaceAccess` evaluates the bounded product capabilities with
namespace-scoped SelfSubjectAccessReview requests. If no bounded capability is
allowed, the selection fails without claiming whether the namespace exists.
Subsequent Kubernetes operations remain authoritative and can still be denied.

The capability mapping is deliberately product-level:

| Capability | Kubernetes authorization checked |
|---|---|
| `workload.view` | list Pods |
| `observation.view` | list Observations |
| `observation.operate` | create Observations and update Observation status |
| `proposal.view` | list SecurityProfileProposals |
| `proposal.review` | update Proposal status |
| `proposal.approve` | update Proposal status |
| `proposal.apply` | create ApplyAttempts |
| `rollback.execute` | create RollbackAttempts |
| `workload.restart` | delete Pods |
| `history.view` | list TrainingHistories, contribution receipts, ApplyAttempts, and RollbackAttempts |

Review and approval remain separate product operations even though the current
Kubernetes resource permission is the same status update permission. The
capability result is advisory; it never replaces authorization on the actual
operation.

Selecting a namespace creates a new immutable namespace context and increments
the context version. Session ID, version, ClusterIdentity, and namespace must
match before a later request is accepted. There is no process-global current
namespace or authorization cache shared across sessions.

## Consequences

Users with namespaced access but no cluster-wide Namespace LIST permission can
still operate by explicit namespace selection. Namespace visibility is not
treated as authorization, and UI state is not treated as permission.

SSAR requests use the same effective client as the selected environment. The
Operations Center does not ask its own administrative identity to answer for a
different user. Credentials remain connector/backend-only.

Complete namespace discovery UX and broader context routing across all
Operations Center endpoints remain M3 work. No RBAC, NetworkPolicy, proposal,
observation, governance CAS, or runtime semantics are changed by M2.
