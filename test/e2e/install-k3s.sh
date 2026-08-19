#!/usr/bin/env bash
# install-k3s.sh — provision a real-node Kubernetes cluster for the D-MIN
# recorder E2E.
#
# WHY THIS EXISTS, AND WHY IT IS NOT kind
#
# SPO's eBPF recorder associates a container with a recording by resolving
# the pids its BPF programs observe:
#
#   handleNewPidEvent -> ContainerIDForPID -> /proc/<pid>/stat
#
# BPF reports pids in the HOST kernel's pid namespace. Under kind the
# Kubernetes node is itself a container with its own pid namespace, so those
# pids do not exist in spod's /proc and every lookup fails — observed as 156
# consecutive "No container ID found for PID" with zero associations (run
# 32255710044).
#
# The fix is topological, not a tuning knob: the node must BE the Linux host.
# k3s installs a real kubelet and containerd directly on the runner VM, so
# the pid namespace BPF observes is the one spod reads. Any containerized
# node — kind, k3d, minikube's docker driver — reproduces the original
# failure exactly and must not be used here.
#
# k3s over kubeadm: it is a single pinned installer that brings its own
# containerd and a working CNI, boots in well under a minute, and uninstalls
# deterministically. kubeadm would need kubelet, a container runtime and a
# CNI provisioned and version-matched separately, for no property this proof
# needs. Nothing here depends on k3s-specific behavior — only on the node
# being the host.
#
# Deliberately NOT installed: Cilium. The kind-based E2Es install it because
# their cluster is created with disableDefaultCNI; k3s ships flannel, which
# is sufficient for this proof. D-MIN certifies SPO recording, import and the
# seccomp lifecycle; NetworkPolicy behavioral enforcement is certified by the
# Core E2E and is not re-proven here.

set -euo pipefail
IFS=$'\n\t'

# Pinned; never `latest`.
K3S_VERSION="${K3S_VERSION:-v1.31.5+k3s1}"

fail() { echo "ERROR: $*" >&2; exit 1; }
stage() { printf '\n[k3s] %s\n' "$*"; }

# --- host preconditions ----------------------------------------------------
# Checked before installing anything, so a host that cannot run SPO's
# recorder says so immediately instead of failing deep inside the scenario.
stage "host preconditions"

echo "[info] kernel:  $(uname -srmo)"
echo "[info] arch:    $(uname -m)"
echo "[info] cgroup:  $(stat -fc %T /sys/fs/cgroup 2>/dev/null || echo unknown)"

# BTF is what lets SPO's recorder load CO-RE BPF objects against this kernel.
if [ -r /sys/kernel/btf/vmlinux ]; then
  echo "[info] BTF:     /sys/kernel/btf/vmlinux present ($(stat -c %s /sys/kernel/btf/vmlinux) bytes)"
else
  fail "no /sys/kernel/btf/vmlinux — this kernel cannot load SPO's CO-RE BPF recorder"
fi

# Seccomp must be available for Localhost profiles to mean anything.
if grep -qE '^Seccomp:' /proc/self/status; then
  echo "[info] seccomp: supported ($(awk '/^Seccomp:/{print $2}' /proc/self/status))"
else
  fail "the kernel reports no seccomp support"
fi

echo "[info] runtime before install: $(command -v containerd || echo none)"

# The property this whole script exists to provide: pid 1 must be the host's
# init, in the host pid namespace, not a container's.
echo "[info] pid1:    $(cat /proc/1/comm 2>/dev/null || echo unknown)"

# --- install ---------------------------------------------------------------
stage "installing k3s ${K3S_VERSION}"

# Disabled add-ons are pure boot-time cost for this proof: no Ingress, no
# LoadBalancer, no metrics. Flannel (the default CNI) is deliberately kept —
# pods must be able to reach the API and each other.
curl -sfL https://get.k3s.io \
  | INSTALL_K3S_VERSION="${K3S_VERSION}" \
    INSTALL_K3S_EXEC="server --write-kubeconfig-mode 644 --disable traefik --disable servicelb --disable metrics-server" \
    sh -

stage "waiting for the node"

export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
for _ in $(seq 1 60); do
  if kubectl get nodes >/dev/null 2>&1; then break; fi
  sleep 5
done
kubectl get nodes >/dev/null 2>&1 || fail "the k3s API never became reachable"

kubectl wait --for=condition=Ready node --all --timeout=300s \
  || { kubectl get nodes -o wide >&2; kubectl describe nodes >&2; fail "the node never became Ready"; }

kubectl get nodes -o wide

# --- topology assertion ----------------------------------------------------
# This is the whole point of the script, so it is asserted rather than
# assumed. If the node is not this host, D-MIN will fail later for the exact
# reason it already failed on kind, and it should fail here instead.
stage "verifying the node is this host"

NODE_NAME="$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')"
HOSTNAME_LOWER="$(hostname | tr '[:upper:]' '[:lower:]')"
echo "[info] node=${NODE_NAME} hostname=${HOSTNAME_LOWER}"
[ "${NODE_NAME}" = "${HOSTNAME_LOWER}" ] \
  || fail "node ${NODE_NAME} is not this host (${HOSTNAME_LOWER}); a containerized node reproduces the pid-namespace failure this script exists to avoid"

# A containerized node would run its own init; the host's kubelet runs under
# the host's systemd and is visible as a normal host process.
pgrep -x k3s >/dev/null 2>&1 || fail "no k3s process on this host — the node is not local"
echo "[info] k3s pid(s) on host: $(pgrep -x k3s | tr '\n' ' ')"

NODE_COUNT="$(kubectl get nodes --no-headers | wc -l | tr -d ' ')"
[ "${NODE_COUNT}" = "1" ] || fail "expected a single-node cluster, found ${NODE_COUNT}"

CRI="$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.containerRuntimeVersion}')"
echo "[info] container runtime: ${CRI}"

stage "ready"
echo "KUBECONFIG=/etc/rancher/k3s/k3s.yaml"
