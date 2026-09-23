#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
rendered="$(mktemp -t landlock-rbac-ownership.XXXXXX.yaml)"
trap 'rm -f "$rendered"' EXIT

helm template qualification "$ROOT_DIR/deploy/helm/landlock-genprof" \
  --namespace qualification \
  --set namespace.create=false \
  --set namespace.name=qualification \
  --set operationsCenter.enabled=true \
  --set operationsCenter.trustedProxySecret.name=hmac \
  --set operationsCenter.executorKubeconfigSecret.name=kubeconfig \
  --set 'operationsCenter.impersonation.allowedUsers[0]=qualification-operator' \
  --set operationsCenter.teamRoles.create=true \
  --set 'operationsCenter.networkPolicy.trustedProxy.namespaceSelector.matchLabels.kubernetes\.io/metadata\.name=qualification' \
  --set operationsCenter.networkPolicy.trustedProxy.podSelector.matchLabels.app=trusted-proxy \
  --set operationsCenter.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 \
  --set observationExecutor.enabled=true \
  --set observationExecutor.targetNamespaces[0]=qualification \
  --set observationExecutor.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 \
  --set rbac.legacyClusterRoles.create=false \
  --set operationsCenter.backendRoleName=qualification-backend \
  --set operationsCenter.backendRoleBindingName=qualification-backend \
  --set observationExecutor.clusterIdentityRoleName=qualification-executor-identity \
  --set observationExecutor.gadgetAccess.roleName=qualification-executor-gadget \
  --set 'operationsCenter.teamRoleBindings[0].name=qualification-security-operator' \
  --set 'operationsCenter.teamRoleBindings[0].namespace=qualification' \
  --set 'operationsCenter.teamRoleBindings[0].subjects[0].kind=User' \
  --set 'operationsCenter.teamRoleBindings[0].subjects[0].name=qualification-operator' \
  --set 'operationsCenter.teamRoleBindings[0].role=landlock-genprof-team-security-operator' \
  >"$rendered"

if grep -q 'name: landlock-genprof-applyattempt-writer' "$rendered"; then
  echo 'legacy shared ClusterRole rendered despite external ownership mode' >&2
  exit 1
fi
grep -q 'name: qualification-backend' "$rendered"
grep -q 'name: qualification-executor-identity' "$rendered"
grep -q 'name: qualification-executor-gadget' "$rendered"
grep -q 'name: qualification-security-operator' "$rendered"
grep -q 'name: qualification-operator' "$rendered"
grep -q 'kind: NetworkPolicy' "$rendered"
if grep -q 'resources: \["\*"\]' "$rendered" || grep -q 'verbs: \["\*"\]' "$rendered"; then
  echo 'qualification RBAC must not introduce wildcard authority' >&2
  exit 1
fi
if grep -q -- '--take-ownership' "$ROOT_DIR/hack/ui-lima-auth-release.sh"; then
  echo 'qualification harness must not use Helm ownership takeover' >&2
  exit 1
fi

echo 'published RBAC ownership tests: PASS'
