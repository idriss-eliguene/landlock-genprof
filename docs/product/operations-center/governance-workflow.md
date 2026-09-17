# Governance workflow

Review identifies the proposal and its evidence basis. Approval and Apply are
separate deliberate actions, authorized by Kubernetes and protected by the
canonical candidate digest and resourceVersion. A stale context or 409 is
reported, state is refreshed where safe, and the previous mutation is never
automatically replayed.
