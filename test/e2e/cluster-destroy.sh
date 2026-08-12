#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME=landlock-genprof-e2e
if kind get clusters | grep -qx "$CLUSTER_NAME"; then
  echo "Deleting kind cluster $CLUSTER_NAME"
  kind delete cluster --name "$CLUSTER_NAME"
else
  echo "Cluster $CLUSTER_NAME not present"
fi
