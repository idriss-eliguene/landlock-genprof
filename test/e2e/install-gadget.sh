#!/usr/bin/env bash
set -euo pipefail

IG_VERSION="v0.55.0"
# determine host OS and arch separately to pick correct release artifacts
HOST_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$HOST_OS" in
  darwin) OSNAME=darwin ;;
  linux) OSNAME=linux ;;
  *) OSNAME="$HOST_OS" ;;
esac
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  amd64) ARCH=amd64 ;;
  *) ARCH="$ARCH" ;;
esac

echo "[e2e] installing Inspektor Gadget CLI and deploying gadget (os=$OSNAME arch=$ARCH)"
GOBIN="$(go env GOPATH)/bin"
mkdir -p "$GOBIN"
# select correct artifact names for host OS
IG_ART=ig-${OSNAME}-${ARCH}-${IG_VERSION}.tar.gz
KG_ART=kubectl-gadget-${OSNAME}-${ARCH}-${IG_VERSION}.tar.gz

# Host-side IG/kubectl-gadget are OPTIONAL for E2E. The tracer uses the Go SDK directly
# to talk to the Inspektor Gadget DaemonSet in-cluster (see internal/tracer), so
# a host plugin is not required for production tracing. Do not attempt ad-hoc
# go install or binary downloads here; if present they are used for manual
# debugging only.
if command -v ig >/dev/null 2>&1; then
  echo "host ig binary present: $(command -v ig)"
else
  echo "host ig binary NOT present (optional). Continuing — cluster Helm install will proceed."
fi
if kubectl gadget version >/dev/null 2>&1; then
  echo "host kubectl-gadget plugin present"
else
  echo "host kubectl-gadget plugin NOT present (optional). Continuing — cluster Helm install will proceed."
fi
# Require Helm to be present (workflow must install it)
if ! command -v helm >/dev/null 2>&1; then
  echo "ERROR: helm not found in PATH. The workflow must install Helm before running install-gadget.sh" >&2
  exit 1
fi

# Ensure Cilium CNI is present (kind created with disableDefaultCNI: true).
#
# SKIP_CILIUM=1 is for clusters that already have a working CNI — notably the
# real-node k3s cluster the D-MIN recorder E2E uses, which ships flannel.
# Installing Cilium there would be several minutes of boot time for a
# property that proof does not exercise: D-MIN certifies SPO recording,
# import and the seccomp lifecycle, while NetworkPolicy behavioral
# enforcement is certified by the Core E2E.
if [ "${SKIP_CILIUM:-0}" = "1" ]; then
  echo "SKIP_CILIUM=1 — leaving the cluster's existing CNI in place"
elif kubectl get daemonset -n kube-system cilium >/dev/null 2>&1; then
  echo "Cilium already deployed on cluster"
else
  echo "Installing Cilium via Helm (for NetworkPolicy enforcement)"
  helm repo add cilium https://helm.cilium.io/
  helm repo update cilium
  helm install cilium cilium/cilium --version "1.19.6" --namespace kube-system --create-namespace \
    --set image.pullPolicy=IfNotPresent --set ipam.mode=kubernetes --set operator.replicas=1
  echo "Waiting for Cilium to become Ready (up to 900s)"
  START_TS=$(date +%s)
  DEADLINE=$((START_TS + 900))
  INTERVAL=10
  while true; do
    now=$(date +%s)
    elapsed=$((now - START_TS))
    # query DS status
    if ds_out=$(kubectl -n kube-system get ds cilium -o jsonpath='{.status.desiredNumberScheduled} {.status.currentNumberScheduled} {.status.numberReady} {.status.numberAvailable}' 2>/dev/null || true); then
      read -r desired current ready available <<<"$ds_out" || true
      desired=${desired:-0}
      current=${current:-0}
      ready=${ready:-0}
      available=${available:-0}
      echo "Cilium readiness: desired=${desired} current=${current} ready=${ready} available=${available} elapsed=${elapsed}s"
      # success predicate
      if [ "$desired" -gt 0 ] && [ "$current" -eq "$desired" ] && [ "$ready" -eq "$desired" ] && [ "$available" -eq "$desired" ]; then
        echo "Cilium DaemonSet is fully available"
        break
      fi
    else
      echo "Warning: failed to query Cilium DaemonSet status (transient). elapsed=${elapsed}s"
    fi
    if [ "$now" -ge "$DEADLINE" ]; then
      echo "ERROR: Cilium daemonset not Available within timeout (${elapsed}s)" >&2
      echo "Immediate state dump:" >&2
      kubectl -n kube-system get ds cilium -o wide || true
      kubectl -n kube-system get pods -l k8s-app=cilium -o wide || true
      echo "Recent events (last 100):" >&2
      kubectl get events -A --sort-by=.lastTimestamp | tail -n 100 || true
      exit 1
    fi
    sleep $INTERVAL
  done
fi

# deploy gadget to cluster using official Helm chart (do not rely on kubectl-gadget CLI)
CHART_VERSION="${IG_VERSION#v}"
if kubectl get ns gadget >/dev/null 2>&1 && kubectl get daemonset -n gadget gadget >/dev/null 2>&1; then
  echo "Inspektor Gadget already deployed"
else
  if ! command -v helm >/dev/null 2>&1; then
    echo "ERROR: helm is required to install Inspektor Gadget via Helm. Install helm and retry." >&2
    exit 1
  fi
  echo "Installing Inspektor Gadget via Helm chart (version=$CHART_VERSION)"
  helm upgrade --install gadget \
    --namespace gadget \
    --create-namespace \
    oci://ghcr.io/inspektor-gadget/inspektor-gadget/charts/gadget \
    --version "${CHART_VERSION}"
  # After ensuring the gadget namespace exists, apply local RBAC manifests that reference it
  ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../" && pwd)"
  if [ -f "$ROOT_DIR/deploy/rbac.yaml" ]; then
    echo "Applying project RBAC manifests into gadget namespace"
    kubectl apply -f "$ROOT_DIR/deploy/rbac.yaml" || true
  fi
  if [ -f "$ROOT_DIR/deploy/rbac-history.yaml" ]; then
    kubectl apply -f "$ROOT_DIR/deploy/rbac-history.yaml" || true
  fi
fi

# wait for daemonset(s) in gadget namespace to become ready
echo "Waiting for gadget daemonsets and pods to be Ready"
# list ds to find actual names
kubectl get daemonset -n gadget -o wide
# attempt to wait for any daemonset in namespace (safe parsing)
mapfile -t GADGET_DS_NAMES < <(
  kubectl get daemonset -n gadget -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null
)
# require exactly one Gadget DaemonSet (fail-closed)
if [ "${#GADGET_DS_NAMES[@]}" -eq 0 ]; then
  echo "ERROR: no Gadget DaemonSet found in namespace 'gadget'" >&2
  kubectl -n gadget get daemonset -o wide || true
  exit 1
fi
if [ "${#GADGET_DS_NAMES[@]}" -gt 1 ]; then
  echo "ERROR: multiple Gadget DaemonSets found in namespace 'gadget': ${GADGET_DS_NAMES[*]}" >&2
  kubectl -n gadget get daemonset -o wide || true
  exit 1
fi
# single name assigned
ds="${GADGET_DS_NAMES[0]}"
# fail-closed validation: accept only valid Kubernetes DNS-1123 label for resource names
if ! [[ "$ds" =~ ^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$ ]]; then
  echo "ERROR: invalid Gadget DaemonSet name: [$ds]" >&2
  kubectl -n gadget get daemonset -o wide || true
  exit 1
fi

echo "waiting for daemonset $ds"
if ! kubectl rollout status daemonset/"$ds" -n gadget --timeout=180s; then
  echo "ERROR: daemonset $ds failed to rollout within timeout" >&2
  kubectl -n gadget get pods -o wide || true
  exit 1
fi

if ! kubectl wait --for=condition=Ready pod -n gadget --all --timeout=180s; then
  echo "ERROR: gadget pods not ready within timeout" >&2
  kubectl get pods -n gadget || true
  exit 1
fi
kubectl get pods -n gadget || true
