# ADR-0033: Operations Center environment foundation

## Status

Accepted for v0.9 M1.

## Decision

Operations Center environment selection is represented by a server-owned,
immutable `EnvironmentSession`. Its context keeps these concepts separate:

- `ClusterLocator`: kubeconfig source, context name, and API URL;
- `ClusterIdentity`: the UID of the `kube-system` Namespace;
- `CredentialContext`: safe credential provenance and method only;
- `ProductInstallationIdentity`: the installation scope;
- namespace: an optional locator scope in M1, not an authorization grant;
- opaque session ID and context version.

Context names and API URLs are locators, never durable cluster identity. Two
contexts addressing one cluster therefore resolve to one ClusterIdentity;
context renames do not create a new cluster identity.

`LocalKubeconfigConnector` is the first connector. Discovery reads context
metadata only and does not execute credential plugins. Opening an explicit
context resolves its REST configuration server-side, verifies connectivity by
reading `kube-system`, and stores the resulting Kubernetes client privately.
Only safe metadata is returned to callers. Cluster-identity or connector
failures are errors and are never represented as an empty environment list.

Environment sessions are immutable and coexist independently. A caller must
validate the opaque session ID, context version, and ClusterIdentity before
using a session. Changing context creates a new session rather than mutating
an existing one.

Exec credential plugins are not silently enabled. M1 permits explicit local
opt-in with a bounded open operation; plugin output and credential material
are never persisted, serialized, logged, or exposed to the browser. Remote
arbitrary kubeconfig upload, OIDC/SSO, ServiceAccount fleet connectors, and
remote agents are deferred.

## Consequences

The browser can hold only opaque session identifiers, context versions, and
safe display metadata. Kubernetes credentials remain in the connector/backend
boundary. Namespace discovery and SSAR-based authorization are M2 concerns;
the default namespace in a kubeconfig is not proof of permission.

The current Workbench remains single-session and namespace-pinned until the
later Global Operations Context milestone threads this contract through all
routes. Existing CandidateDigest, provenance, observation, governance CAS,
RBAC, NetworkPolicy, Trusted Proxy, and seccomp semantics are unchanged.
