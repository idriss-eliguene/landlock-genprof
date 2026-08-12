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

# Give user guidance about next steps
cat <<EOF
Cluster $CLUSTER_NAME created.
Next steps:
  make e2e-install  # install CRDs and Inspektor Gadget (+ optional backends)

EOF
