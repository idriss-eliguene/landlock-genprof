#!/usr/bin/env bash
set -Eeuo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER_NAME="${LANDLOCK_CORE_CLUSTER:-landlock-genprof-core}"
OWNERSHIP_FILE="${XDG_STATE_HOME:-${HOME}/.local/state}/landlock-genprof/${CLUSTER_NAME}.json"
if [ ! -f "$OWNERSHIP_FILE" ]; then echo "REFUSED shared/pre-existing cluster: no bootstrap ownership record for ${CLUSTER_NAME}"; exit 2; fi
grep -q '"owner":"landlock-genprof"' "$OWNERSHIP_FILE" || { echo "REFUSED invalid ownership record" >&2; exit 2; }
echo "Cleaning project-layer resources only; preserving cluster, Lima VM, and host tools."
kubectl delete -f "$ROOT_DIR/deploy/rbac-workbench.yaml" --ignore-not-found >/dev/null 2>&1 || true
echo "TEST_ENV_CLEAN complete; platform remains available"
