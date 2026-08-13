#!/usr/bin/env bash
set -euo pipefail

IG_VERSION="v0.54.1"
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

# Ensure Cilium CNI is present (kind created with disableDefaultCNI: true)
if kubectl get daemonset -n kube-system cilium >/dev/null 2>&1; then
  echo "Cilium already deployed on cluster"
else
  echo "Installing Cilium via Helm (for NetworkPolicy enforcement)"
  helm repo add cilium https://helm.cilium.io/
  helm repo update cilium
  helm install cilium cilium/cilium --version "1.19.6" --namespace kube-system --create-namespace \
    --set image.pullPolicy=IfNotPresent --set ipam.mode=kubernetes --set operator.replicas=1
  echo "Waiting for Cilium to become Ready"
  if ! kubectl wait --for=condition=Available daemonset -n kube-system cilium --timeout=180s; then
    echo "ERROR: Cilium daemonset not Available within timeout" >&2
    kubectl -n kube-system get pods -o wide || true
    exit 1
  fi
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
fi

# wait for daemonset(s) in gadget namespace to become ready
echo "Waiting for gadget daemonsets and pods to be Ready"
# list ds to find actual names
kubectl get daemonset -n gadget -o wide
# attempt to wait for any daemonset in namespace
for ds in $(kubectl get daemonset -n gadget -o jsonpath='{range .items[*]}{.metadata.name} {"\n"}{end}' 2>/dev/null); do
  echo "waiting for daemonset $ds"
  if ! kubectl rollout status daemonset/$ds -n gadget --timeout=180s; then
    echo "ERROR: daemonset $ds failed to rollout within timeout" >&2
    kubectl -n gadget get pods -o wide || true
    exit 1
  fi
done

if ! kubectl wait --for=condition=Ready pod -n gadget --all --timeout=180s; then
  echo "ERROR: gadget pods not ready within timeout" >&2
  kubectl get pods -n gadget || true
  exit 1
fi
kubectl get pods -n gadget || true
