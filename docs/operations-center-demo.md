# Operations Center V2 demo

This is the supported local demonstration path for the v0.9 Operations
Center. It uses the owned `landlock-genprof-core` kind cluster and creates
only disposable resources labeled as part of the demo. Kubernetes remains
the authorization authority; the persona names below are fixture identities,
not product policy.

## Prerequisites

Use the repository's supported macOS/Lima or Linux Core environment.

```sh
make operations-center-demo
```

The command runs the repository-supported Core bootstrap and project-layer
installation as idempotent prerequisites.

The command is idempotent and prints a readiness-gated URL manifest when the
backend, trusted proxy, and health endpoints are reachable. The recommended
entrypoint is the React migration UI:

```text
NEW / RECOMMENDED Operations Center (React)  http://127.0.0.1:<proxy>/next/
LEGACY Operations Center                    http://127.0.0.1:<proxy>/
```

The exact URLs, configured ports, backend/API address, health endpoints, and
metrics status are printed by the manifest. The backend address is localhost
only; browser traffic must continue through the trusted proxy. Metrics are
disabled in the default disposable demo unless explicitly enabled by the
observability environment configuration, so no unavailable metrics URL is
printed.

The command creates the
`payments`, `development`, `platform`, and `security` namespaces, four small
workloads, and real service-account/RBAC contexts for `developer`,
`security-reviewer`, and `restricted-user`. Generated tokens are held only in
a private temporary kubeconfig and are never printed or served to the browser.

## Five-minute demo

1. Open the printed URL and confirm the active context, cluster identity,
   namespace, and safe identity metadata in the context bar.
2. With `developer`, select `payments`, open Workloads, and choose
   `frontend` or `api`.
3. Start an observation, wait for tracing readiness, trigger workload
   activity, inspect the attributable capability facts, and generate the
   proposal.
4. Open the proposal, History, Attention, and Governance views. Review the
   target and namespace before any consequential action.
5. Switch to `security-reviewer/security`, then `restricted-user`. Namespace
   discovery is unavailable for the restricted identity, but a known
   authorized namespace can be entered explicitly.

The demo uses actual Kubernetes objects and RBAC/SSAR decisions. It does not
seed fake observations, capability facts, candidates, or proposals.

## Reset and troubleshooting

Stop the local server with Ctrl-C, then run:

```sh
make operations-center-demo-reset
```

Reset refuses to run outside the owned Core context and deletes only the four
named demo namespaces and demo-labeled identity bindings. If the Core VM is
stopped, rerun `make bootstrap`; if a local port is busy, set
`DEMO_BACKEND_PORT` to another free port.

Never copy the generated kubeconfig or token into an issue, screenshot, URL,
or browser storage.
