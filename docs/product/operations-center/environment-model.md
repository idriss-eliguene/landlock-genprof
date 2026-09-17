# Environment model

The product scope is `Cluster -> Identity / Context -> Namespace`.

Clusters are grouped by durable `UID(kube-system Namespace)`. Kubeconfig
context names and API URLs are locators/display metadata only. Selecting an
identity opens a server-side immutable EnvironmentSession; selecting a
namespace establishes a versioned namespace context. Requests carry the
opaque session, expected context version, and namespace.

Credentials never enter browser state, URLs, API responses, logs, or durable
records. Multiple tabs hold independent sessions. Context changes invalidate
old resource state and stale mutations fail closed.
