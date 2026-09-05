#!/usr/bin/env bash
set -Eeuo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
kubectl cluster-info >/dev/null 2>&1 || { echo "ERROR: run ./hack/bootstrap.sh first" >&2; exit 1; }
bash "$ROOT_DIR/test/e2e/install-crds.sh"
if [ "${SKIP_GADGET:-0}" != 1 ]; then bash "$ROOT_DIR/test/e2e/install-gadget.sh"; fi
for f in deploy/rbac-proposal.yaml deploy/rbac-patched-manifest.yaml deploy/rbac-applyattempt.yaml deploy/rbac-rollbackattempt.yaml deploy/rbac-restart.yaml; do
  [ -f "$ROOT_DIR/$f" ] && kubectl apply -f "$ROOT_DIR/$f"
done
make -C "$ROOT_DIR" install-plugin
for crd in traininghistories.landlockgenprof.io securityprofileproposals.landlockgenprof.io applyattempts.landlockgenprof.io rollbackattempts.landlockgenprof.io; do kubectl wait --for=condition=Established "crd/$crd" --timeout=180s; done
echo "ENVIRONMENT_READY topology=kind+cilium project=landlock-genprof optional=SPO,PodLock"
