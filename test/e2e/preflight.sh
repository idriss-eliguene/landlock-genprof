#!/usr/bin/env bash
set -euo pipefail

EXPECTED_CONTEXT=kind-landlock-genprof-e2e
CUR_CONTEXT=$(kubectl config current-context 2>/dev/null || true)

echo "Component                        Status"
echo "-------------------------------------"
# Kubernetes
if [ -n "$CUR_CONTEXT" ]; then
  printf "Kubernetes context: %-20s READY\n" "$CUR_CONTEXT"
else
  printf "Kubernetes context: %-20s NOT-FOUND\n" "(none)"
fi

# nodes
if kubectl get nodes >/dev/null 2>&1; then
  kubectl get nodes -o wide | sed -n '1,3p'
  echo "Node details (name kernel-version container-runtime architecture):"
  kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name} {.status.nodeInfo.kernelVersion} {.status.nodeInfo.containerRuntimeVersion} {.status.nodeInfo.architecture}{"\n"}{end}' || true
else
  echo "Nodes: unreachable"
fi

# CRDs
for crd in traininghistories.landlockgenprof.io securityprofileproposals.landlockgenprof.io; do
  if kubectl get crd $crd >/dev/null 2>&1; then
    printf "%s %-20s READY\n" "$crd" ""
  else
    printf "%s %-20s MISSING\n" "$crd" ""
  fi
done

# Inspektor Gadget
if kubectl get ns gadget >/dev/null 2>&1 && kubectl get daemonset -n gadget gadget >/dev/null 2>&1; then
  printf "Inspektor Gadget               READY\n"
else
  printf "Inspektor Gadget               MISSING\n"
fi

# landlock-genprof binary detection
LANDLOCK_GENPROF_PATH=""
LANDLOCK_GENPROF_MODE=""
if [ -n "${LANDLOCK_GENPROF_BIN:-}" ] && [ -x "${LANDLOCK_GENPROF_BIN}" ]; then
  LANDLOCK_GENPROF_PATH="${LANDLOCK_GENPROF_BIN}"
  LANDLOCK_GENPROF_MODE="local"
elif [ -x ./bin/landlock-genprof ]; then
  LANDLOCK_GENPROF_PATH="$(pwd)/bin/landlock-genprof"
  LANDLOCK_GENPROF_MODE="local"
elif command -v kubectl-landlock_genprof >/dev/null 2>&1; then
  # verify the kubectl plugin runs
  if kubectl landlock-genprof --help >/dev/null 2>&1; then
    LANDLOCK_GENPROF_PATH="$(command -v kubectl-landlock_genprof)"
    LANDLOCK_GENPROF_MODE="kubectl-plugin"
  fi
fi

if [ -n "$LANDLOCK_GENPROF_PATH" ]; then
  printf "LANDLOCK_GENPROF=READY (mode=%s path=%s)\n" "$LANDLOCK_GENPROF_MODE" "$LANDLOCK_GENPROF_PATH"
else
  printf "LANDLOCK_GENPROF=MISSING (set LANDLOCK_GENPROF_BIN, or build ./bin/landlock-genprof, or install kubectl-landlock_genprof)\n"
fi

# Optional backends
if kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io >/dev/null 2>&1; then
  printf "SPO (Seccomp)                  PRESENT\n"
else
  printf "SPO (Seccomp)                  MISSING\n"
fi
if kubectl get crd landlockprofiles.podlock.kubewarden.io >/dev/null 2>&1; then
  printf "PodLock (Landlock)             PRESENT\n"
else
  printf "PodLock (Landlock)             MISSING\n"
fi

# CNI / NetworkPolicy enforcement
# Minimal test: report if a known CNI DaemonSet present (cilium/calico)
if kubectl get daemonset -n kube-system cilium >/dev/null 2>&1; then
  printf "CNI (Cilium)                   PRESENT\n"
elif kubectl get daemonset -n kube-system calico-typha >/dev/null 2>&1; then
  printf "CNI (Calico)                   PRESENT\n"
else
  printf "CNI                             UNKNOWN / custom\n"
fi

# Safety check on context
if [ "$CUR_CONTEXT" != "$EXPECTED_CONTEXT" ]; then
  echo "ERROR: current context '$CUR_CONTEXT' != expected '$EXPECTED_CONTEXT'. Aborting to avoid accidental cluster mutation."
  exit 2
fi

echo "Preflight checks completed (no mutations)."
