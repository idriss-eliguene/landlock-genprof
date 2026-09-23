# Troubleshooting

## Operations Context is not ready

The UI must resolve the Context locator, durable ClusterIdentity, namespace,
and EnvironmentSession before namespace-scoped data is authoritative. Wait for
`Resolving`/`Switching context…` to finish. If it fails, inspect the server
authorization and context error; do not assume that a matching namespace
string belongs to the same cluster.

## Namespace is not bound

In `EXPLICIT_ONLY` mode, namespace enumeration is intentionally unavailable.
Enter a known namespace and let the server validate it. The UI must not select
the first namespace, infer choices from workloads, or treat a browser value as
authority.

## Authorization failure

HTTP 401/403 means the authenticated actor or projected capability is not
authorized for the operation. A hidden or disabled UI control is only an
ergonomic hint; the server remains authoritative. Rebind the intended context
or use an authorized actor according to the deployment contract.

## Stale Proposal or resource version (409)

A 409 means the Proposal changed relative to the submitted resourceVersion,
UID, digest, or review context. Refresh and explicitly review the new server
state. Governance mutations are not automatically replayed.

## Missing exact lineage

An empty or diagnostic lineage result does not mean that a same-name Proposal
belongs to the workload. Every required provenance ObservationID must resolve
to the selected exact workload identity. Mixed, missing, or legacy provenance
remains unassociated.

## Observation or executor uncertainty

`REQUESTED`, `STARTING`, `RUNNING`, `COMPLETING`, `COMPLETED`, and `FAILED` are
distinct lifecycle states. Executor loss or incomplete qualification must stay
visible. `UNKNOWN` is not a successful terminal state and is not automatically
converted into `FAILED`.

## Health and Attention uncertainty

`UNKNOWN`, `NOT_ESTABLISHED`, and `NOT_APPLICABLE` are meaningful states. A
zero observation population is not `HEALTHY`; a successful HTTP refresh is not
evidence freshness; and Health is not a score. Reconciliation Attention and
SPHM Attention remain separate projections.

## Applied versus verified

An ApplyAttempt records an application outcome. It does not by itself prove
backend enforcement or behavioral qualification. Consult the backend-specific
qualification evidence and keep structural and behavioral results separate.
