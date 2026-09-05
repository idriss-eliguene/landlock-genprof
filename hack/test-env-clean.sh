#!/usr/bin/env bash
set -Eeuo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bash-version.sh"
require_modern_bash || exit 2
CLUSTER_NAME="${LANDLOCK_CORE_CLUSTER:-landlock-genprof-core}"
OWNERSHIP_FILE="${XDG_STATE_HOME:-${HOME}/.local/state}/landlock-genprof/${CLUSTER_NAME}.json"
if [ ! -f "$OWNERSHIP_FILE" ]; then echo "REFUSED shared/pre-existing cluster: no bootstrap ownership record for ${CLUSTER_NAME}"; exit 2; fi
grep -q '"owner":"landlock-genprof"' "$OWNERSHIP_FILE" || { echo "REFUSED invalid ownership record" >&2; exit 2; }
echo "Cleaning project-layer resources only; preserving cluster, Lima VM, host tools, Cilium, and Inspektor Gadget."

# PROJECT_TEST_ENV_OWNED: resources hack/test-env.sh applies directly and
# exclusively for this project — safe to remove once ownership of the
# cluster itself is established above. Cilium and Inspektor Gadget are
# SHARED_DEPENDENCY (platform CNI / a general-purpose profiling tool other
# work may rely on) and are deliberately never touched here, even though
# test-env knows their names.
for f in deploy/rbac-workbench.yaml deploy/rbac-proposal.yaml deploy/rbac-patched-manifest.yaml deploy/rbac-applyattempt.yaml deploy/rbac-rollbackattempt.yaml deploy/rbac-restart.yaml; do
  [ -f "$ROOT_DIR/$f" ] && kubectl delete -f "$ROOT_DIR/$f" --ignore-not-found >/dev/null 2>&1 || true
done
for crd in traininghistories.landlockgenprof.io securityprofileproposals.landlockgenprof.io applyattempts.landlockgenprof.io rollbackattempts.landlockgenprof.io; do
  kubectl delete crd "$crd" --ignore-not-found >/dev/null 2>&1 || true
done
echo "TEST_ENV_CLEAN complete; platform, Cilium, and Inspektor Gadget remain available"
