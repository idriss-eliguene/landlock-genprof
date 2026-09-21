# Setting up a test environment

This is the canonical contributor path for **trying the tool** on the
supported Core topology. It creates a kind cluster with Cilium and then
installs the project layer. If you already have a Kubernetes cluster you
want to install against instead, skip straight to [`INSTALL.md`](../INSTALL.md).

Working on the codebase itself (not just trying it)? See
[`HOW_TO_START.md`](../HOW_TO_START.md) /
[`COMMENT_COMMENCER.md`](../COMMENT_COMMENCER.md) instead — same
cluster setup, plus git workflow, code walkthrough, and first tasks per
role. This page is the lean version: cluster up, nothing else.

## 1. Supported host

The Core substrate is `kind` + Cilium. Linux amd64/arm64 uses the native
container runtime. macOS amd64/arm64 uses a native-architecture Lima Linux
guest; it does not provide kernel-security certification for the host.
Landlock and eBPF are Linux kernel features — see [`README.md`](../README.md)
§6 for kernel requirements.

- Linux amd64/arm64 and macOS amd64/arm64 are supported.
- Windows is not a supported host for this front door; use a supported
  Linux environment or a Linux VM.

## 2. Clone and install prerequisites

```bash
git clone https://github.com/idriss-eliguene/landlock-genprof.git
cd landlock-genprof
go version   # should show go1.26 or later
```

If Go is missing, see
[`HOW_TO_START.md` — Step 3](../HOW_TO_START.md#2-set-up-your-environment)
for the install command (with `amd64`/`arm64` auto-detection).

The contributor path requires Bash 4.0 or newer because the project-layer
installation uses Bash 4 features. macOS's system `/bin/bash` 3.2 is rejected
before setup. On macOS, `bootstrap.sh` reuses a supported Bash when present or
installs the Homebrew `bash` formula when Homebrew is already installed; if
Homebrew is absent, it fails closed with an actionable diagnostic. The
selected interpreter is passed explicitly to the child installation scripts
and is rediscovered by later entrypoints without relying on PATH ordering.

## 3. Bootstrap the Core platform and project layer

```bash
./hack/bootstrap.sh
make env-doctor
make test-env
make test-env                 # safe convergence check
```

`bootstrap.sh` owns only the host/runtime, Lima (on macOS), kind, kubeconfig,
and topology-specific readiness. It does not install project CRDs, project
RBAC, Gadget, SPO, PodLock, proposals, or evidence fixtures. `make test-env`
composes the existing project CRD, RBAC, Gadget, and plugin installation
primitives. SPO and PodLock remain optional.

Every wait is bounded and emits elapsed-time progress; timeout diagnostics
include nodes, pods, Cilium state, and events. `hack/init-vm.sh` remains only
as a deprecated compatibility wrapper to `bootstrap.sh --lane core`.

```bash
kubectl plugin list | grep landlock-genprof   # sanity check
kubectl landlock-genprof doctor
```

## Done — cluster and CLI are ready

Continue to [`INSTALL.md`](../INSTALL.md) §3 ("Install the RBAC and CRDs")
for a separately managed cluster. From there, [`docs/usage.md`](usage.md)
assumes a cluster up and `kubectl landlock-genprof` on your `PATH`.

## 4. Operations Center UI testing on macOS

For fast local visual testing, run the existing canonical Lima environment and
then:

```bash
make env-doctor
make test-env
UI_NAMESPACE=g5-filesystem make ui-lima
```

The launcher verifies the Lima VM, Docker context, Kubernetes context, node,
Cilium, CoreDNS, Gadget, and all six product CRDs before starting the
loopback-only local Operations Center. It prints a URL such as
`http://127.0.0.1:8080/`. This mode is development-only and does not reproduce
the production trusted-proxy authentication boundary.

For production-like authenticated UI qualification, deploy the documented
Operations Center Helm values, place a trusted proxy fixture in front of the
ClusterIP Service, and use a fresh browser profile. The proxy must strip
client-supplied identity headers and inject the signed allowlisted identity.
Verify unsigned `401`, valid signed `200`, and stale signed `401`; then run
the six-surface smoke through the real browser. The browser must never connect
directly to the backend in this mode. See
[`docs/release-notes-v0.8.1.md`](release-notes-v0.8.1.md) and the chart README
for the exact production-like values contract.

For an automated source-mode qualification with a disposable real workload,
use:

```bash
make ui-lima-auth-test
```

This target keeps the source-mode launcher separate from the published-release
harness. It creates a temporary Deployment, discovers its real container
through the Operations Center API, and drives the six navigation surfaces in a
real browser through the trusted-proxy fixture. It removes only the namespace
and processes it created; it does not recreate the canonical Lima VM or Kind
cluster or alter historical specimens.

For an interactive source-mode demo that remains available for manual
inspection, use:

```bash
make ui-lima-demo
```

This creates a disposable real `nginx:1.27` workload, starts the same
authenticated trusted-proxy and Lima executor boundary, and prints
`http://127.0.0.1:8090`. It remains running until Ctrl-C, then removes only
the demo namespace and processes it owns. It does not publish artifacts or
recreate the canonical VM or Kind cluster.

## 5. Published-release qualification

The source-mode commands above use the current checkout. To qualify a
published release, use the separate fail-closed harness:

```bash
make ui-lima-auth-release RELEASE_VERSION=v0.8.1 \
  PUBLISHED_PROXY_NAMESPACE=trusted-proxy \
  PUBLISHED_PROXY_URL=http://127.0.0.1:8090
```

Published mode validates the canonical Lima/Docker/kind/Cilium/CoreDNS/Gadget
environment, resolves the remote OCI image and Helm chart, pulls the chart,
deploys only those remote artifacts, and compares both running Pod `imageID`
values with the resolved immutable OCI digest. It never uses a local chart,
`docker load`, a kind-loaded image, or a source build. The trusted proxy must
be a disposable fixture in `PUBLISHED_PROXY_NAMESPACE` with selector
`PUBLISHED_PROXY_SELECTOR` (default `app=trusted-proxy`); the harness refuses
to bypass the chart NetworkPolicy or authenticated UI path when those fixture
inputs are absent.

Before publication, validate input and reference derivation without registry
or cluster mutation:

```bash
PUBLISHED_RELEASE_VALIDATE_ONLY=1 make ui-lima-auth-release RELEASE_VERSION=v0.8.1
```

After publication, the harness reports the resolved artifact identities,
deployed digest match, and the browser URL supplied by the existing trusted
proxy fixture. Authentication and six-surface browser checks remain through
that proxy only: unsigned requests must be `401`, valid signed requests `200`,
and stale signed requests `401`. Cleanup removes only disposable release,
namespace, credentials, fixtures, and listeners; it never recreates the
canonical VM or cluster or removes CRDs/historical specimens. Re-running the
same command converges the same disposable release.

## 6. Cleanup

`make test-env-clean` is deliberately bounded. It removes only explicitly
owned project-layer resources where ownership is recorded; it never destroys
the kind cluster, Lima VM, shared host tools, or an unowned/shared cluster.
Platform destruction is a separate operator decision and is not implicit in
project cleanup.
