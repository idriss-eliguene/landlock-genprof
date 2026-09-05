#!/usr/bin/env bash
set -Eeuo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Reuses the exact bootstrap ownership/provenance model (hack/bootstrap.sh):
# a reachable API alone, or a same-named context alone, is not ownership.
CLUSTER_NAME="${LANDLOCK_CORE_CLUSTER:-landlock-genprof-core}"
EXPECTED_CONTEXT="kind-${CLUSTER_NAME}"
OWNERSHIP_FILE="${XDG_STATE_HOME:-${HOME}/.local/state}/landlock-genprof/${CLUSTER_NAME}.json"

kubectl cluster-info >/dev/null 2>&1 || { echo "ERROR: no reachable Kubernetes API; run ./hack/bootstrap.sh first" >&2; exit 1; }
CURRENT_CONTEXT="$(kubectl config current-context 2>/dev/null || true)"
if [ "$CURRENT_CONTEXT" != "$EXPECTED_CONTEXT" ]; then
  echo "ERROR: current context '${CURRENT_CONTEXT}' is not the bootstrap-owned Core context '${EXPECTED_CONTEXT}'; run ./hack/bootstrap.sh, or set LANDLOCK_CORE_CLUSTER to match an already-bootstrapped cluster, before running test-env" >&2
  exit 1
fi
if [ ! -f "$OWNERSHIP_FILE" ]; then
  echo "ERROR: no bootstrap ownership record for '${CLUSTER_NAME}'; refusing to install project resources onto a cluster this environment does not own" >&2
  exit 1
fi
grep -q '"owner":"landlock-genprof"' "$OWNERSHIP_FILE" || { echo "ERROR: invalid ownership record for '${CLUSTER_NAME}'" >&2; exit 1; }

bash "$ROOT_DIR/test/e2e/install-crds.sh"
if [ "${SKIP_GADGET:-0}" != 1 ]; then bash "$ROOT_DIR/test/e2e/install-gadget.sh"; fi
for f in deploy/rbac-proposal.yaml deploy/rbac-patched-manifest.yaml deploy/rbac-applyattempt.yaml deploy/rbac-rollbackattempt.yaml deploy/rbac-restart.yaml; do
  [ -f "$ROOT_DIR/$f" ] && kubectl apply -f "$ROOT_DIR/$f"
done
make -C "$ROOT_DIR" install-plugin
for crd in traininghistories.landlockgenprof.io securityprofileproposals.landlockgenprof.io applyattempts.landlockgenprof.io rollbackattempts.landlockgenprof.io; do kubectl wait --for=condition=Established "crd/$crd" --timeout=180s; done
echo "ENVIRONMENT_READY topology=kind+cilium project=landlock-genprof optional=SPO,PodLock"
