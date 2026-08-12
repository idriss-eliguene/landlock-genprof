#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../" && pwd)"
echo "[e2e] applying deploy/ manifests for project CRDs"
kubectl apply -f "$ROOT_DIR/deploy/crd-traininghistory.yaml"
kubectl apply -f "$ROOT_DIR/deploy/crd-securityprofileproposal.yaml"
# apply any RBAC pieces required (non-idempotent safe)
kubectl apply -f "$ROOT_DIR/deploy/rbac.yaml" || true
kubectl apply -f "$ROOT_DIR/deploy/rbac-history.yaml" || true

# wait for CRDs to be Established
echo "[e2e] waiting for CRDs to become Established"
for crd in traininghistories.landlockgenprof.io securityprofileproposals.landlockgenprof.io; do
  kubectl wait --for=condition=Established crd/$crd --timeout=60s || {
    echo "WARNING: CRD $crd not Established within timeout"
  }
done

kubectl get crd | grep -E 'traininghistor|securityprofileproposal' || true
