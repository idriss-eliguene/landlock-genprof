#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../" && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bash-version.sh"
require_modern_bash || exit 2
echo "[e2e] applying deploy/ manifests for project CRDs"
kubectl apply -f "$ROOT_DIR/deploy/crd-traininghistory.yaml"
kubectl apply -f "$ROOT_DIR/deploy/crd-securityprofileproposal.yaml"
kubectl apply -f "$ROOT_DIR/deploy/crd-applyattempt.yaml"
kubectl apply -f "$ROOT_DIR/deploy/crd-rollbackattempt.yaml"
# RBAC is applied after the 'gadget' namespace exists by install-gadget.sh
# (avoid applying RBAC into a namespace that may not yet exist)
# kubectl apply -f "$ROOT_DIR/deploy/rbac.yaml"
# kubectl apply -f "$ROOT_DIR/deploy/rbac-history.yaml"

# wait for CRDs to be Established
echo "[e2e] waiting for CRDs to become Established"
for crd in traininghistories.landlockgenprof.io securityprofileproposals.landlockgenprof.io applyattempts.landlockgenprof.io rollbackattempts.landlockgenprof.io; do
  if ! kubectl wait --for=condition=Established crd/$crd --timeout=60s; then
    echo "ERROR: CRD $crd not Established within timeout" >&2
    kubectl get crd $crd -o yaml || true
    exit 1
  fi
done

kubectl get crd | grep -E 'traininghistor|securityprofileproposal|applyattempt' || true
