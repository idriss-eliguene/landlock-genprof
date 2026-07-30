# Setting up a test environment

This is for **trying the tool** — spinning up a disposable `kind`
cluster to run `landlock-genprof` against, from nothing. If you already
have a Kubernetes cluster you want to install against instead, skip
straight to [`INSTALL.md`](../INSTALL.md).

Working on the codebase itself (not just trying it)? See
[`HOW_TO_START.md`](../HOW_TO_START.md) /
[`COMMENT_COMMENCER.md`](../COMMENT_COMMENCER.md) instead — same
cluster setup, plus git workflow, code walkthrough, and first tasks per
role. This page is the lean version: cluster up, nothing else.

## 1. A Linux environment

Landlock and eBPF are **Linux kernel** features (`trace` doesn't work
on macOS/Windows directly). Kernel requirements: Landlock FS ≥ 5.13,
Landlock network ≥ 6.4, eBPF ≥ 5.8 recommended — see
[`README.md`](../README.md) §6 for the exact table. **Ubuntu 24.04
(kernel 6.8) covers all of it.**

- Already on native Ubuntu 24.04/26.04? Skip to step 2.
- On macOS or Windows? You need a VM — see
  [`HOW_TO_START.md` §0](../HOW_TO_START.md#0-create-your-ubuntu-vm-windows)
  for click-by-click VirtualBox/Hyper-V steps. Come back here once the
  VM is up and `uname -r` shows a 6.8.x/7.0.x kernel.

## 2. Clone and install Go

```bash
git clone https://github.com/idriss-eliguene/landlock-genprof.git
cd landlock-genprof
go version   # should show go1.26 or later
```

If Go is missing, see
[`HOW_TO_START.md` — Step 3](../HOW_TO_START.md#2-set-up-your-environment)
for the install command (with `amd64`/`arm64` auto-detection).

## 3. Check the kernel, then create the cluster

```bash
./hack/check-kernel.sh
./hack/init-vm.sh
```

`init-vm.sh` does everything from here: `kind` cluster (Cilium as CNI,
not kindnet — kindnet doesn't enforce `NetworkPolicy`), Inspektor
Gadget, a test pod (`nginx-demo`), and the `landlock-genprof` CLI itself
built and installed as a kubectl plugin. It's idempotent — safe to
re-run if a step fails partway (network hiccup, slow first image pull).
See [`HOW_TO_START.md`'s step table](../HOW_TO_START.md#2-set-up-your-environment)
for exactly what each of its 8 steps does and why, or just read the
script directly — it's short and heavily commented.

```bash
kubectl plugin list | grep landlock-genprof   # sanity check
kubectl get pod nginx-demo
```

## Done — cluster and CLI are ready

Continue to [`INSTALL.md`](../INSTALL.md) §3 ("Install the RBAC and
CRDs") — steps 1-2 there (getting the CLI) are already done by
`init-vm.sh`. From there, [`docs/usage.md`](usage.md) assumes exactly
this state: a cluster up, `kubectl landlock-genprof` on your `PATH`.
