# ADR-0036: Operations Center server-owned authorization projection

## Status

Accepted for the Operations Center P3 implementation.

## Context

The Operations Center serves several independently polling browser views. Its
authoritative operations-context projection evaluates Kubernetes permissions
with namespace-scoped SelfSubjectAccessReview calls. A request creates an
impersonated client, and concurrent requests therefore multiply authorization
fanout and client-go rate-limiter demand. Discovery reuse reduced unrelated
amplification, but did not remove this authorization boundary.

## Decision

The server coordinates only identical capability projections that are already
in flight. The coalescing key contains the complete authority input:

```text
cluster identity, API server/context locator, namespace,
authenticated username, normalized groups,
EnvironmentSession identifier, context version
```

The computation result is delivered to current waiters and is not retained
after completion. Resource lists, observations, proposals, history, and
governance state remain request-scoped reads. Authorization results are not
cached across time, identities, namespaces, clusters, or context versions.

Within one capability computation, the exact duplicate Review/Approve SSAR is
performed once and its result is reused locally. The remaining authorization
checks stay sequential because the bounded parallel experiment increased
control-plane contention under load.

## Security and cancellation

Impersonated Kubernetes clients remain request-specific. Coalescing never
shares a client, transport identity, authorization decision, or namespace
binding. A waiter may cancel independently; the leader computation follows
its bounded request deadline so a disconnected leader cannot cancel a valid
waiter. A failed or denied computation is propagated as failure or denial and
never becomes cached success.

## Invalidation and non-decision

No persistent authorization reuse is introduced. This branch has no
authoritative RBAC watch/resourceVersion invalidation contract sufficient to
bound stale privilege. Any future persistent projection must establish that
model first and must prefer recomputation over stale grants.

The following alternatives are rejected for P3:

- increasing the response timeout, QPS, or Burst to conceal fanout;
- arbitrary TTL authorization caching;
- cross-identity or cross-namespace reuse;
- parallel SSAR fanout without a demonstrated capacity budget;
- frontend polling changes used as a substitute for server authority.

## Observability

Request logs expose bounded `authz_projection_executions` and
`authz_projection_coalesced` counts alongside existing request correlation,
authorization, Kubernetes, discovery, and projection timings. These are
focused self-observability fields, not the future custom metrics engine.

## Consequences

Concurrent identical requests can share one in-flight capability computation,
reducing duplicate SSAR work without making resource projections stale. The
server can still approach its existing response budget when many distinct or
non-overlapping requests arrive; polling amplification and persistent
invalidation remain follow-up work.

## P3.1 qualification amendment

The first loaded `26`-SSAR requests were traced to a same-request composition
boundary, not a coalescer collision: trusted-proxy context validation called
`SelectNamespace` and performed a local-session capability projection before
the authenticated handler performed its own projection. The handler then
reported one physical coalescer execution alongside the uncoalesced calls.
Trusted-proxy validation now checks the server-owned session/version binding
without using the local session's credentials for authorization; the
authenticated request remains responsible for its own impersonated SSAR
projection. Local-session requests retain Kubernetes authorization during
namespace selection. This removes duplicate work without sharing an
authorization result across identities, namespaces, sessions, or context
versions.

Qualification also observed that client-go waits and Kubernetes API work are
distinct phases: direct Kubernetes reads remained sub-second in isolation,
while overlapping request fanout increased SSAR wall time and exposed the
existing response budget. Future self-observability should retain separate
request, authorization, client-go throttle, Kubernetes, projection, and
coalescer-wait measurements.

## References

- [SPHM telemetry and qualified signal architecture](0035-sphm-telemetry-temporal-and-qualified-signal-architecture.md)
- [Operations Center namespace authorization](0034-operations-center-namespace-authorization.md)
