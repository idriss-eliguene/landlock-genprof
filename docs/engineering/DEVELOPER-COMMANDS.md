# Developer command guide

This page describes the supported local command lanes. Targets that create
Kubernetes, Lima, Docker, or demo resources are explicitly marked; no default
target creates or destroys infrastructure.

## Fast source-only lane

Requires Go 1.26 or newer. It does not require Kubernetes, Docker, Linux, or a
cluster:

```bash
make build
make test-unit
make vet
make lint
```

`test-unit`, `build`, and `vet` derive package names with `go list` and exclude
the ignored generated `book/dist` tree. This avoids treating generated
documentation copies as source packages.

## API-server semantics lane

Requires network access to download controller-runtime envtest assets. It
starts disposable local API-server/etcd processes and cleans them up through
the test harness:

```bash
make test-envtest
```

`make test-integration` is a compatibility alias. These tests establish API
server semantics, not host-kernel tracing or runtime enforcement.

## Kubernetes E2E lane

Requires Linux kernel support, Docker or Lima, kind, Cilium, kubectl, Helm,
Inspektor Gadget, and the project test layer. It creates and mutates a
disposable cluster; use the bounded cleanup targets documented by the
bootstrap scripts:

```bash
make dev-bootstrap
make dev-doctor
make test-env
make test-e2e
make test-env-clean
```

The project cleanup preserves the cluster, VM, and shared host tools. Use the
explicit E2E cluster-destroy target only for a cluster created by that E2E
lane. SPO, PodLock, and real-node tests have additional workflow-specific
requirements and are not implied by `make test-e2e`.

## Security and documentation lanes

```bash
make test-security   # requires a separately installed gosec binary
make docs-build
mdbook test book
make generate
```

`make test-security` does not download or install scanners. Release and
publication targets are intentionally not part of the default test lanes.

## Side-effect summary

| Target family | External dependencies | Side effects |
|---|---|---|
| `build`, `test-unit`, `vet`, `lint` | Go | build/cache files only |
| `test-envtest` | envtest asset download | local API-server/etcd processes |
| `dev-bootstrap`, `test-env` | Docker/Lima/kind/Cilium/kubectl | creates or mutates disposable platform/project resources |
| `test-e2e` | prepared Kubernetes environment | runs workload and policy test fixtures |
| `test-env-clean` | prepared project environment | removes only owned project resources |
| `docs-build`, `generate` | mdBook/Go | writes ignored generated documentation |
| `test-security` | preinstalled gosec | scanner output only |

For a first contribution, start with `make test-unit`; do not run the
Kubernetes lane unless the change requires it.
