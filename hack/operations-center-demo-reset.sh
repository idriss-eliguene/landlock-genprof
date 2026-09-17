#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
LIMA_VM="${LIMA_VM:-landlock-genprof-core}"
EXPECTED_CONTEXT="kind-${LANDLOCK_CORE_CLUSTER:-$LIMA_VM}"
export EXPECTED_CONTEXT

command -v kubectl >/dev/null 2>&1 || { echo "ERROR: kubectl is required" >&2; exit 2; }
kubectl config current-context | grep -Fx "$EXPECTED_CONTEXT" >/dev/null || { echo "ERROR: refusing reset outside ${EXPECTED_CONTEXT}" >&2; exit 1; }
for binding in landlock-genprof-demo-developer-identity landlock-genprof-demo-developer-user-identity landlock-genprof-demo-reviewer-identity landlock-genprof-demo-reviewer-user-identity landlock-genprof-demo-restricted-identity; do
  kubectl delete clusterrolebinding "$binding" --ignore-not-found >/dev/null
done
kubectl delete clusterrole landlock-genprof-demo-cluster-identity --ignore-not-found >/dev/null
for ns in payments development platform security; do
  kubectl delete namespace "$ns" --ignore-not-found --wait=true >/dev/null
done
echo "OPERATIONS_CENTER_DEMO_RESET=YES"
