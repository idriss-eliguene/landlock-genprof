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

## 4. Cleanup

`make test-env-clean` is deliberately bounded. It removes only explicitly
owned project-layer resources where ownership is recorded; it never destroys
the kind cluster, Lima VM, shared host tools, or an unowned/shared cluster.
Platform destruction is a separate operator decision and is not implicit in
project cleanup.
