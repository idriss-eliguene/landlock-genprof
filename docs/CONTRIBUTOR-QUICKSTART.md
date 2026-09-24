# Contributor quickstart

Choose one lane before installing infrastructure. Start with the smallest
lane that can exercise your change; do not create a Kubernetes cluster for a
CLI, documentation, or frontend-only change.

## Lane 1: CLI-only or Go package work

**Host:** macOS, Linux, or Windows for packages that do not touch the real
Linux tracer. **Dependencies:** Go 1.26 or newer. No Kubernetes, Docker,
Lima, or Inspektor Gadget is required.

```bash
git clone https://github.com/idriss-eliguene/landlock-genprof.git
cd landlock-genprof
make build
make test-unit
make verify-install INSTALL_DIR="$(go env GOPATH)/bin"
```

Expected result: the CLI builds, package tests pass, and `version`/`--help`
work. `make test-unit` is source-only and excludes generated `book/dist`.
For an offline first result, run `kubectl-landlock_genprof --help` or
`landlock-genprof doctor --help`; this proves command availability only, not
observation, policy safety, or runtime enforcement.

Common failures:

* Go is older than the `go.mod` requirement: install Go 1.26+.
* `PATH_MISSING`: add the displayed user-owned Go bin directory to PATH.
* A local socket test is denied by a sandbox: rerun on a normal developer
  host or classify the result as an environment restriction.

No cleanup is required beyond `make uninstall` if the plugin was installed.

## Lane 2: Existing Kubernetes cluster

**Host:** Linux is required for live tracing; macOS can use a suitable remote
cluster. **Dependencies:** kubectl, Go or a published binary, a kubeconfig,
and Inspektor Gadget already deployed in the target cluster. Helm is required
only for the chart path.

Read [`INSTALL.md`](../INSTALL.md) first. Build/install the CLI, apply the
project CRDs and RBAC, then verify:

```bash
make install
make verify-install
kubectl landlock-genprof doctor
```

Only after the diagnostic is healthy, run the documented `trace`/`observe`
workflow against a disposable namespace and inspect the resulting proposal.
SPO, PodLock, Cilium, and Operations Center are optional integrations with
additional prerequisites; installation of the CLI does not install or prove
any of them.

Cleanup is namespace- and project-resource-specific. Use the documented
manifest/chart uninstall procedure; retain CRDs unless the administrator
explicitly intends to remove all project objects.

## Lane 3: Disposable kind+Cilium environment

**Host:** native Linux, or macOS with a native-architecture Lima Linux guest.
**Dependencies:** Docker or Lima, kind, Cilium, kubectl, Helm, network access,
and the pinned versions in `hack/versions.env`.

```bash
./hack/bootstrap.sh
make dev-doctor
make test-env
make test-envtest
```

Run `make test-e2e` only when the project layer is ready. Expected results are
a Ready Core cluster, installed project CRDs/RBAC and Inspektor Gadget, and
passing API-server semantics. This lane does not establish host-level SPO
eBPF behavior or kernel enforcement.

Common failures include Docker daemon access, kind node image availability,
kernel capability gaps, Cilium readiness, and missing Gadget permissions.
Capture `make dev-doctor` output before changing host limits.

Cleanup only owned project resources with `make dev-down`/`make test-env-clean`.
Destroy a cluster only if this lane created it and the relevant E2E destroy
target identifies it as owned.

## Lane 4: Operations Center/frontend work

**Frontend-only host:** Node/npm; no Kubernetes is needed. From
`web/operations-center`:

```bash
npm ci
npm run lint
npm test -- --run
npm run build
```

Expected result: TypeScript checks, frontend tests, and a production bundle
complete independently of the backend. Do not claim API or governance support
from this lane.

**Full UI work:** additionally requires the project chart, Operations Center
Secrets, an executor kubeconfig, an external trusted proxy, explicit
impersonation/RBAC configuration, and a prepared Kubernetes environment. Use
the chart README and the existing `ui-lima-*` targets. Never expose the
Operations Center Service directly through an Ingress, NodePort, or
LoadBalancer unless the documented trust boundary is deliberately redesigned.

Cleanup is the owned demo reset or Helm uninstall path; preserve CRDs and
shared cluster resources unless explicitly authorized.

## Lane 5: Security and integration work

**Host:** Linux kernel >=6.8 is the documented live-tracing baseline.
**Dependencies:** Go, envtest assets, disposable Kubernetes, Docker/kind or
Lima, Inspektor Gadget, and any backend under test such as SPO or PodLock.

```bash
make test-unit
make test-envtest
make test-security       # requires a separately installed gosec
```

Use the relevant workflow or disposable-cluster harness for Core, SPO,
PodLock, and real-node checks. Do not use production clusters. Separate API
resource creation from behavioral enforcement evidence, and preserve the
GENERATED / APPLIED / BEHAVIORALLY_VERIFIED / UNKNOWN boundaries.

## Before opening a pull request

```bash
git diff --check
make test-unit
make vet
make test-envtest       # when API-server semantics are affected
make docs-build         # when documentation changes
```

Run the smallest relevant frontend, Helm, E2E, or security lane as well.
Describe blocked environment checks explicitly; do not replace them with
historical CI results.
