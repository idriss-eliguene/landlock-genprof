#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME=landlock-genprof-e2e
KIND_CONFIG="$(pwd)/test/e2e/kind-config.yaml"

echo "[e2e] creating kind cluster: $CLUSTER_NAME"
if kind get clusters | grep -qx "$CLUSTER_NAME"; then
  echo "cluster $CLUSTER_NAME already exists"
  exit 0
fi

kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG"

# After creation, ensure kubeconfig server hostname is TLS-valid (127.0.0.1)
# kind may generate a kubeconfig server that uses 0.0.0.0 when hostPort is used.
# Patch the cluster entry 'kind-<name>' to use https://127.0.0.1:6443 if necessary.
CURRENT_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>/dev/null || true)
if [ -n "$CURRENT_SERVER" ] && echo "$CURRENT_SERVER" | grep -q "0.0.0.0"; then
  echo "[e2e] kubeconfig server uses 0.0.0.0, patching to 127.0.0.1"
  CLUSTER_NAME_IN_KUBECONFIG="kind-$CLUSTER_NAME"
  kubectl config set-cluster "$CLUSTER_NAME_IN_KUBECONFIG" --server="https://127.0.0.1:6443"
  echo "[e2e] patched cluster $CLUSTER_NAME_IN_KUBECONFIG server to https://127.0.0.1:6443"
fi

# Give user guidance about next steps
cat <<EOF
Cluster $CLUSTER_NAME created.
Next steps:
  make e2e-install  # install CRDs and Inspektor Gadget (+ optional backends)

EOF
