# States and errors

Loading states explain that authoritative state is being read. Empty states
give the next action. Forbidden namespace listing becomes explicit namespace
mode, never “zero namespaces”. Unauthorized operations are safe denials;
connection and backend failures are unavailable/degraded states. Unknown
evidence is visibly distinct from available evidence and keeps technical
details accessible. Stale context and governance conflicts explain that state
changed and require an explicit retry. No error path leaks credentials or raw
exec-plugin output.
