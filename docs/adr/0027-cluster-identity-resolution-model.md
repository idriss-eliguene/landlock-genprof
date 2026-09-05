# ADR-0027: ClusterIdentity resolution model

Status: Accepted

Date: 2026-09-05

## Context

Observation binding and proposal provenance need a Kubernetes-local cluster
anchor, while kubeconfig context, endpoint, and credentials can change
independently.

## Problem

Using a context name, endpoint, or certificate fingerprint as authority
identity can confuse locator/credential changes with cluster recreation.

## Decision

`ClusterIdentity` is the UID of the `kube-system` Namespace, resolved at the
session or operation boundary. `ClusterLocator` and `CredentialContext` are
separate concepts and never substitute for it.

## Detailed semantics

`ClusterLocator` may describe kubeconfig source, context name, and API URL.
`CredentialContext` describes the credentials used to resolve/access the
cluster. A CA fingerprint is informational, not a co-equal identity
component. Routine CA or endpoint rotation therefore does not manufacture a
new cluster identity. If the anchor cannot be read or resolved, the product
fails closed with `CLUSTER_IDENTITY_UNRESOLVED`; it does not fall back to a
context name.

The kube-system Namespace UID is the strongest currently selected defensible
Kubernetes-local anchor. It changes when the cluster is recreated. It is not
cryptographic attestation and does not defend against a fully compromised or
malicious API server. Resolution requires sufficient RBAC to read the anchor.
No universally stable RBAC-independent cluster identifier is claimed.

## Invariants

Cluster identity, location, and credentials remain separately represented.
An unresolved identity cannot qualify binding or authority.

## Consequences

Cluster recreation is visible as an identity change, while normal locator or
credential rotation is not. Implementations must perform an explicit read and
must report failure rather than weakening the identity contract.

## Rejected alternatives

- Context-name identity.
- API endpoint identity.
- CA fingerprint as a co-equal identity component.
- A claim of cryptographic cluster attestation.

## Compatibility / migration

This is a v0.7 prerequisite for new Observation binding and provenance. It
does not alter canonical `GovernedTarget`, proposal digests, or existing
Apply/Rollback records.

## Security considerations

The anchor prevents accidental cross-cluster attribution within the limits of
the Kubernetes API trust model; it does not create authority beyond existing
RBAC and authentication.

## Claim boundary

`ClusterIdentity` is an identity anchor, not proof of cluster integrity,
kernel state, backend health, or enforcement.

## Open implementation details

The read client and exact session cache lifetime remain implementation policy.

## Non-goals

No fleet identity, remote attestation, or RBAC-independent identifier is
defined.

## References

- [ADR-0026](0026-observation-identity-and-record-decomposition.md)
- [ADR-0028](0028-workload-identity-container-slot-revision-split.md)
