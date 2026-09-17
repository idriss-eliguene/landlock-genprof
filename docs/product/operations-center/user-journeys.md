# User journeys

The happy path is: enter Operations Center -> select cluster -> select
identity -> select namespace -> inspect workload -> start observation -> wait
for trace readiness -> trigger activity -> capture evidence -> complete
observation -> review capabilities -> generate proposal -> review proposal ->
govern -> audit history.

Each transition is an authenticated backend request bound to the immutable
environment/session and namespace context. Kubernetes authorization is
checked server-side. Failures include explicit namespace mode, unavailable
evidence, failed observation, stale context, revoked permission, unavailable
proposal, and governance conflict; none silently changes scope or replays a
mutation.
