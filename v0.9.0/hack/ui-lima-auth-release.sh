#!/usr/bin/env bash
set -Eeuo pipefail

# Published-artifact Operations Center qualification harness. Product images
# and charts remain published-only; the Trusted Proxy is a disposable
# qualification fixture built from this exact source revision and loaded into
# the target kind node before its digest-pinned manifest is applied.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/published-release-harness-lib.sh"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bash-version.sh"
ensure_bash_interpreter 0 "$0" "$@" || exit 2
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/lib-core-readiness.sh"

RELEASE_VERSION="${RELEASE_VERSION:-}"
VALIDATE_ONLY="${PUBLISHED_RELEASE_VALIDATE_ONLY:-0}"
LIMA_VM="${LIMA_VM:-landlock-genprof-core}"
# Consumed by hack/lib-core-readiness.sh after it is sourced below.
# shellcheck disable=SC2034
EXPECTED_CONTEXT="kind-${LIMA_VM}"
RELEASE_NAMESPACE="${PUBLISHED_RELEASE_NAMESPACE:-landlock-genprof-release}"
HELM_RELEASE="${PUBLISHED_HELM_RELEASE:-landlock-genprof-release}"
QUALIFICATION_USER="${QUALIFICATION_USER:-qualification-operator}"
OPERATIONS_CENTER_SERVICE_ACCOUNT="${PUBLISHED_OPERATIONS_CENTER_SERVICE_ACCOUNT:-landlock-genprof-operations-center}"
OPERATIONS_CENTER_BACKEND_ROLE="${PUBLISHED_OPERATIONS_CENTER_BACKEND_ROLE:-${HELM_RELEASE}-operations-center-backend}"
OPERATIONS_CENTER_BACKEND_BINDING="${PUBLISHED_OPERATIONS_CENTER_BACKEND_BINDING:-${HELM_RELEASE}-operations-center-backend}"
EXECUTOR_IDENTITY_ROLE="${PUBLISHED_EXECUTOR_IDENTITY_ROLE:-${HELM_RELEASE}-observation-executor-cluster-identity}"
EXECUTOR_GADGET_ROLE="${PUBLISHED_EXECUTOR_GADGET_ROLE:-${HELM_RELEASE}-observation-executor-gadget}"
IDENTITY_READER_ROLE="${HELM_RELEASE}-identity-reader"
IDENTITY_READER_BINDING="${HELM_RELEASE}-identity-reader"
PROXY_NAMESPACE="${PUBLISHED_PROXY_NAMESPACE:-$RELEASE_NAMESPACE}"
PROXY_SELECTOR="${PUBLISHED_PROXY_SELECTOR:-app=trusted-proxy}"
PROXY_PORT="${PUBLISHED_PROXY_PORT:-8090}"
PROXY_URL="${PUBLISHED_PROXY_URL:-}"
PROXY_IMAGE="${PUBLISHED_PROXY_IMAGE:-}"
PROXY_SOURCE_REVISION="$(git -C "$ROOT_DIR" rev-parse HEAD)"
PROXY_IMAGE_TAG="${PUBLISHED_PROXY_IMAGE_TAG:-qualification-${PROXY_SOURCE_REVISION:0:12}}"
PROXY_FORWARD_PID=""
BACKEND_FORWARD_PID=""
WORK_DIR=""
CREATED_NAMESPACE=0

die() { echo "ERROR: $*" >&2; exit 1; }

usage() {
  cat >&2 <<'EOF'
Usage: RELEASE_VERSION=v0.8.1 hack/ui-lima-auth-release.sh

Set PUBLISHED_RELEASE_VALIDATE_ONLY=1 for non-mutating reference/input
validation. Product qualification never falls back to local product images or
charts; the proxy is always a disposable image built from this source tree.
EOF
  exit 2
}

cleanup() {
  local status=$?
  if [ -n "$PROXY_FORWARD_PID" ]; then
    kill "$PROXY_FORWARD_PID" >/dev/null 2>&1 || true
    wait "$PROXY_FORWARD_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "$BACKEND_FORWARD_PID" ]; then
    kill "$BACKEND_FORWARD_PID" >/dev/null 2>&1 || true
    wait "$BACKEND_FORWARD_PID" >/dev/null 2>&1 || true
  fi
  if [ "$VALIDATE_ONLY" != 1 ] && [ "$CREATED_NAMESPACE" -eq 1 ]; then
    helm uninstall "$HELM_RELEASE" --namespace "$RELEASE_NAMESPACE" >/dev/null 2>&1 || true
    kubectl delete namespace "$RELEASE_NAMESPACE" --ignore-not-found >/dev/null 2>&1 || true
    kubectl delete clusterrolebinding "$IDENTITY_READER_BINDING" --ignore-not-found >/dev/null 2>&1 || true
    kubectl delete clusterrole "$IDENTITY_READER_ROLE" --ignore-not-found >/dev/null 2>&1 || true
  fi
  [ -z "$WORK_DIR" ] || rm -rf "$WORK_DIR"
  lib_core_readiness_cleanup
  exit "$status"
}
trap cleanup EXIT

[ -n "$RELEASE_VERSION" ] || usage
published_release_validate_tag_ref "$RELEASE_VERSION" || exit 2
if [ -z "$PROXY_URL" ]; then
  while ! published_release_port_is_free "$PROXY_PORT"; do
    PROXY_PORT=$((PROXY_PORT + 1))
  done
  PROXY_URL="http://127.0.0.1:${PROXY_PORT}"
fi
IMAGE_REF="$(published_release_image_reference "$RELEASE_VERSION")"
CHART_REF="$(published_release_chart_reference "$RELEASE_VERSION")"
CHART_VERSION="$(published_release_chart_version "$RELEASE_VERSION")"

case "$RELEASE_NAMESPACE" in
  ''|*[!a-z0-9-]*|[0-9]*-|-[0-9]*) die "invalid disposable namespace" ;;
esac
case "$HELM_RELEASE" in
  ''|*[!a-z0-9-]*) die "invalid Helm release name" ;;
esac
for name in "$OPERATIONS_CENTER_BACKEND_ROLE" "$OPERATIONS_CENTER_BACKEND_BINDING" "$EXECUTOR_IDENTITY_ROLE" "$EXECUTOR_GADGET_ROLE" "$IDENTITY_READER_ROLE" "$IDENTITY_READER_BINDING"; do
  case "$name" in
    ''|*[!a-z0-9-]*) die "invalid generated cluster-scoped resource name: $name" ;;
  esac
done

echo "MODE                         published-release"
echo "RELEASE                      $RELEASE_VERSION"
echo "HELM                         ${CHART_REF}:$CHART_VERSION"
echo "OCI IMAGE                    $IMAGE_REF"
echo "LOCAL_FALLBACK               DISABLED"

if [ "$VALIDATE_ONLY" = 1 ]; then
  echo "VALIDATE_ONLY                PASS"
  exit 0
fi

lib_core_readiness_require_commands curl docker helm kubectl kind limactl make jq node npm
lib_core_readiness_check

WORK_DIR="$(mktemp -d -t landlock-genprof-published-release.XXXXXX)"
chmod 700 "$WORK_DIR"

echo "OCI_LOOKUP_STARTING"
oci_inspect="$(docker buildx imagetools inspect "$IMAGE_REF" 2>&1)" ||
  die "published OCI image is unavailable: $IMAGE_REF"
OCI_DIGEST="$(printf '%s\n' "$oci_inspect" | awk '/^[[:space:]]*Digest:[[:space:]]*sha256:/ { print $2; exit }')"
published_release_require_full_digest "$OCI_DIGEST" || die "OCI digest could not be resolved"
echo "OCI_REFERENCE                 $IMAGE_REF"
echo "OCI_DIGEST                    $OCI_DIGEST"

echo "HELM_LOOKUP_STARTING"
helm show chart "$CHART_REF" --version "$CHART_VERSION" >"$WORK_DIR/chart.txt" 2>&1 ||
  die "published Helm chart is unavailable: ${CHART_REF}:${CHART_VERSION}"
grep -q "^version: $CHART_VERSION$" "$WORK_DIR/chart.txt" || die "published chart version mismatch"
helm pull "$CHART_REF" --version "$CHART_VERSION" --destination "$WORK_DIR" >/dev/null ||
  die "could not pull published Helm chart"
CHART_ARCHIVE="$WORK_DIR/landlock-genprof-${CHART_VERSION}.tgz"
[ -f "$CHART_ARCHIVE" ] || die "Helm pull did not produce the expected chart archive"
helm show chart "$CHART_ARCHIVE" >"$WORK_DIR/pulled-chart.txt"
grep -q "^version: $CHART_VERSION$" "$WORK_DIR/pulled-chart.txt" || die "pulled chart version mismatch"
grep -Eq "^appVersion: (v)?${CHART_VERSION}$" "$WORK_DIR/pulled-chart.txt" || die "published chart appVersion mismatch"
helm lint "$CHART_ARCHIVE" >/dev/null || die "published chart failed helm lint"
echo "HELM_REFERENCE                ${CHART_REF}:${CHART_VERSION}"
echo "HELM_ARTIFACT                 $CHART_ARCHIVE"

[ "$PROXY_NAMESPACE" = "$RELEASE_NAMESPACE" ] || die "the disposable proxy fixture must share the release namespace"
case "$PROXY_NAMESPACE" in
  ''|*[!a-z0-9-]*) die "invalid PUBLISHED_PROXY_NAMESPACE" ;;
esac

echo "DISPOSABLE_NAMESPACE_CREATING name=$RELEASE_NAMESPACE"
kubectl create namespace "$RELEASE_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
CREATED_NAMESPACE=1

# The published chart's reusable viewer role intentionally contains only
# workload/project reads.  The backend's startup identity handshake also
# requires one read of kube-system's Namespace UID.  Grant that exact
# cluster-scoped read to this disposable qualification identity and remove
# both uniquely named objects in cleanup; no shared RBAC is changed.
kubectl apply -f - >/dev/null <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${IDENTITY_READER_ROLE}
  labels:
    app.kubernetes.io/managed-by: landlock-genprof-published-harness
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    resourceNames: ["kube-system"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${IDENTITY_READER_BINDING}
  labels:
    app.kubernetes.io/managed-by: landlock-genprof-published-harness
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ${IDENTITY_READER_ROLE}
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: ${QUALIFICATION_USER}
EOF

secret_dir="$WORK_DIR/secrets"
mkdir -p "$secret_dir"
secret_file="$secret_dir/hmac"
head -c 32 /dev/urandom | base64 | tr -d '\n' >"$secret_file"
chmod 600 "$secret_file"
kubectl -n "$RELEASE_NAMESPACE" create secret generic published-release-hmac \
  --from-file=hmac-secret="$secret_file" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
# The backend runs in-cluster.  A host kubeconfig commonly points at
# 127.0.0.1 (or a Lima-only endpoint) and is not reachable from the Pod.
# Install first with the disposable secret, then replace its contents with a
# short-lived token for the Helm-created backend ServiceAccount and the
# in-cluster API endpoint.  This preserves the chart's bounded SSAR/impersonate
# authority without granting the fixture any extra RBAC.
kubectl config view --raw --minify >"$secret_dir/kubeconfig"
chmod 600 "$secret_dir/kubeconfig"
kubectl -n "$RELEASE_NAMESPACE" create secret generic published-release-executor-kubeconfig \
  --from-file=config="$secret_dir/kubeconfig" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

echo "PUBLISHED_HELM_INSTALL_STARTING"
helm upgrade --install "$HELM_RELEASE" "$CHART_ARCHIVE" \
  --namespace "$RELEASE_NAMESPACE" \
  --set namespace.create=false \
  --set namespace.name="$RELEASE_NAMESPACE" \
  --set operationsCenter.enabled=true \
  --set operationsCenter.image.repository="${IMAGE_REF%:*}" \
  --set operationsCenter.image.tag="$RELEASE_VERSION" \
  --set operationsCenter.trustedProxySecret.name=published-release-hmac \
  --set operationsCenter.executorKubeconfigSecret.name=published-release-executor-kubeconfig \
  --set rbac.legacyClusterRoles.create=false \
  --set operationsCenter.backendRoleName="$OPERATIONS_CENTER_BACKEND_ROLE" \
  --set operationsCenter.backendRoleBindingName="$OPERATIONS_CENTER_BACKEND_BINDING" \
  --set observationExecutor.clusterIdentityRoleName="$EXECUTOR_IDENTITY_ROLE" \
  --set observationExecutor.gadgetAccess.roleName="$EXECUTOR_GADGET_ROLE" \
  --set operationsCenter.impersonation.allowedUsers[0]="$QUALIFICATION_USER" \
  --set operationsCenter.teamRoles.create=true \
  --set "operationsCenter.teamRoleBindings[0].name=${HELM_RELEASE}-security-operator" \
  --set "operationsCenter.teamRoleBindings[0].namespace=$RELEASE_NAMESPACE" \
  --set operationsCenter.teamRoleBindings[0].subjects[0].kind=User \
  --set "operationsCenter.teamRoleBindings[0].subjects[0].name=$QUALIFICATION_USER" \
  --set "operationsCenter.teamRoleBindings[0].role=landlock-genprof-team-security-operator" \
  --set "operationsCenter.teamRoleBindings[1].name=${HELM_RELEASE}-security-approver" \
  --set "operationsCenter.teamRoleBindings[1].namespace=$RELEASE_NAMESPACE" \
  --set operationsCenter.teamRoleBindings[1].subjects[0].kind=User \
  --set "operationsCenter.teamRoleBindings[1].subjects[0].name=$QUALIFICATION_USER" \
  --set "operationsCenter.teamRoleBindings[1].role=landlock-genprof-team-security-approver" \
  --set "operationsCenter.teamRoleBindings[2].name=${HELM_RELEASE}-viewer" \
  --set "operationsCenter.teamRoleBindings[2].namespace=$RELEASE_NAMESPACE" \
  --set operationsCenter.teamRoleBindings[2].subjects[0].kind=User \
  --set "operationsCenter.teamRoleBindings[2].subjects[0].name=$QUALIFICATION_USER" \
  --set "operationsCenter.teamRoleBindings[2].role=landlock-genprof-team-viewer" \
  --set-string "$(published_release_namespace_selector_set_arg "$PROXY_NAMESPACE")" \
  --set operationsCenter.networkPolicy.trustedProxy.podSelector.matchLabels."${PROXY_SELECTOR%%=*}"="${PROXY_SELECTOR#*=}" \
  --set operationsCenter.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 \
  --set observationExecutor.enabled=true \
  --set observationExecutor.image.repository="${IMAGE_REF%:*}" \
  --set observationExecutor.image.tag="$RELEASE_VERSION" \
  --set observationExecutor.targetNamespaces[0]="$RELEASE_NAMESPACE" \
  --set observationExecutor.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 >/dev/null

kubectl -n "$RELEASE_NAMESPACE" create token "$OPERATIONS_CENTER_SERVICE_ACCOUNT" --duration=1h >"$secret_dir/service-account-token"
kubectl -n "$RELEASE_NAMESPACE" get configmap kube-root-ca.crt -o jsonpath='{.data.ca\.crt}' >"$secret_dir/ca.crt"
kubectl config --kubeconfig "$secret_dir/kubeconfig" set-cluster qualification-in-cluster \
  --server=https://kubernetes.default.svc \
  --certificate-authority="$secret_dir/ca.crt" --embed-certs=true >/dev/null
kubectl config --kubeconfig "$secret_dir/kubeconfig" set-credentials qualification-backend \
  --token="$(cat "$secret_dir/service-account-token")" >/dev/null
kubectl config --kubeconfig "$secret_dir/kubeconfig" set-context qualification-in-cluster \
  --cluster=qualification-in-cluster --user=qualification-backend >/dev/null
kubectl config --kubeconfig "$secret_dir/kubeconfig" use-context qualification-in-cluster >/dev/null
kubectl -n "$RELEASE_NAMESPACE" create secret generic published-release-executor-kubeconfig \
  --from-file=config="$secret_dir/kubeconfig" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n "$RELEASE_NAMESPACE" rollout restart deployment/landlock-genprof-operations-center >/dev/null
kubectl -n "$RELEASE_NAMESPACE" rollout status deployment/landlock-genprof-operations-center --timeout=5m

if [ -z "$PROXY_IMAGE" ]; then
  PROXY_IMAGE="landlock-genprof-trustedproxy:$PROXY_IMAGE_TAG"
fi
echo "TRUSTED_PROXY_BUILD_STARTING source=$PROXY_SOURCE_REVISION image=$PROXY_IMAGE"
docker build --provenance=false -f "$ROOT_DIR/Dockerfile.trustedproxy" -t "$PROXY_IMAGE" "$ROOT_DIR" >/dev/null ||
  die "trusted proxy image build failed"
PROXY_IMAGE_DIGEST="$(docker image inspect "$PROXY_IMAGE" --format '{{index .RepoDigests 0}}' | awk -F@ '{print $2}')"
published_release_require_full_digest "$PROXY_IMAGE_DIGEST" || die "trusted proxy image digest could not be resolved"
echo "TRUSTED_PROXY_LOAD_STARTING digest=$PROXY_IMAGE_DIGEST"
kind load docker-image "$PROXY_IMAGE@$PROXY_IMAGE_DIGEST" --name "$LIMA_VM" >/dev/null ||
  die "trusted proxy image load into kind failed"
proxy_node="$(kind get nodes --name "$LIMA_VM" | head -n 1)"
[ -n "$proxy_node" ] || die "kind node could not be resolved for trusted proxy verification"
node_image_names="$(docker exec "$proxy_node" ctr -n k8s.io images ls -q)"
node_ctr_digest_ref="$(published_release_select_containerd_source "$PROXY_IMAGE_DIGEST" "$node_image_names")"
[ -n "$node_ctr_digest_ref" ] || die "loaded trusted proxy digest is absent from kind containerd"
proxy_image_name="${PROXY_IMAGE%%:*}"
canonical_proxy_ref="docker.io/library/${proxy_image_name}@$PROXY_IMAGE_DIGEST"
docker exec "$proxy_node" ctr -n k8s.io images tag --force "$node_ctr_digest_ref" "$canonical_proxy_ref" >/dev/null ||
  die "could not register trusted proxy digest under its Kubernetes image reference"
node_image_names="$(docker exec "$proxy_node" ctr -n k8s.io images ls -q)"
published_release_verify_containerd_reference "$canonical_proxy_ref" "$node_image_names" ||
  die "canonical trusted proxy digest reference is absent from kind node"
node_arch="$(docker exec "$proxy_node" uname -m)"
case "$node_arch" in
  aarch64) proxy_platform=linux/arm64 ;;
  x86_64) proxy_platform=linux/amd64 ;;
  *) die "unsupported kind node architecture: $node_arch" ;;
esac
node_image_inspect="$(docker exec "$proxy_node" ctr -n k8s.io images inspect --content "$canonical_proxy_ref")"
if printf '%s\n' "$node_image_inspect" | awk '/application\/vnd\.oci\.image\.index\.v1\+json/ { found=1 } END { exit found ? 0 : 1 }'; then
  proxy_platform_digest="$(published_release_platform_manifest_digest "$node_image_inspect" "$proxy_platform")"
else
  proxy_platform_digest="$PROXY_IMAGE_DIGEST"
fi
[ -n "$proxy_platform_digest" ] || die "trusted proxy platform manifest is absent from kind image index"
echo "TRUSTED_PROXY_PLATFORM_VERIFIED platform=$proxy_platform digest=$proxy_platform_digest"
echo "TRUSTED_PROXY_NODE_DIGEST_VERIFIED digest=$PROXY_IMAGE_DIGEST"
PUBLISHED_PROXY_NAMESPACE="$PROXY_NAMESPACE" \
PUBLISHED_PROXY_NODE="$proxy_node" \
PUBLISHED_PROXY_PLATFORM="$proxy_platform" \
PUBLISHED_PROXY_IMAGE="$PROXY_IMAGE" \
PUBLISHED_PROXY_IMAGE_DIGEST="$PROXY_IMAGE_DIGEST" \
PUBLISHED_PROXY_PLATFORM_DIGEST="$proxy_platform_digest" \
PUBLISHED_PROXY_BACKEND_HOST="landlock-genprof-operations-center.${RELEASE_NAMESPACE}.svc:8080" \
PUBLISHED_PROXY_SECRET_NAME=published-release-hmac \
  "$ROOT_DIR/hack/published-trusted-proxy-fixture.sh"

kubectl -n "$RELEASE_NAMESPACE" port-forward --address 127.0.0.1 svc/published-trusted-proxy "$PROXY_PORT:8090" >"$WORK_DIR/proxy-forward.log" 2>&1 &
PROXY_FORWARD_PID=$!
backend_port=$((PROXY_PORT + 1))
while ! published_release_port_is_free "$backend_port"; do
  backend_port=$((backend_port + 1))
done
kubectl -n "$RELEASE_NAMESPACE" port-forward --address 127.0.0.1 svc/landlock-genprof-operations-center "$backend_port:8080" >"$WORK_DIR/backend-forward.log" 2>&1 &
BACKEND_FORWARD_PID=$!
backend_url="http://127.0.0.1:${backend_port}"
backend_host_header="landlock-genprof-operations-center.${RELEASE_NAMESPACE}.svc:8080"
unsigned_status=""
for _ in $(seq 1 30); do
  if ! kill -0 "$PROXY_FORWARD_PID" >/dev/null 2>&1; then
    die "trusted proxy port-forward exited"
  fi
  unsigned_status="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' -H "Host: $backend_host_header" "$backend_url/" || true)"
  echo "TRUSTED_PROXY_UNSIGNED_STATUS status=$unsigned_status"
  if [ "$unsigned_status" = 401 ]; then
    break
  fi
  sleep 1
done
if [ "$unsigned_status" != 401 ]; then
  die "trusted proxy did not reject unsigned request status=$unsigned_status"
fi
curl --silent --show-error --output /dev/null --write-out '%{http_code}' -H "Host: $backend_host_header" "$backend_url/" | grep -qx 401 ||
  die "trusted proxy did not reject unsigned request"
echo "TRUSTED_PROXY_READY       PASS"

declare -a valid_header_args
while IFS='=' read -r header_name header_value; do
  [ -z "$header_name" ] && continue
  if [ -z "$header_value" ]; then
    valid_header_args+=(-H "${header_name//_/-};")
  else
    valid_header_args+=(-H "${header_name//_/-}: ${header_value}")
  fi
done < <(go run "$ROOT_DIR/hack/authfixture" -secret-file "$secret_file" -user "$QUALIFICATION_USER" -format env)
valid_response_file="$WORK_DIR/valid-response"
valid_status="$(curl --silent --show-error --output "$valid_response_file" --write-out '%{http_code}' -H "Host: $backend_host_header" "${valid_header_args[@]}" "$backend_url/" || true)"
if [ "$valid_status" != 200 ]; then
  echo "TRUSTED_PROXY_VALID_STATUS status=$valid_status body=$(tr '\n' ' ' <"$valid_response_file")" >&2
  die "valid signed request was not accepted"
fi
echo "TRUSTED_PROXY_VALID_STATUS status=200"
declare -a stale_header_args
while IFS='=' read -r header_name header_value; do
  [ -z "$header_name" ] && continue
  if [ -z "$header_value" ]; then
    stale_header_args+=(-H "${header_name//_/-};")
  else
    stale_header_args+=(-H "${header_name//_/-}: ${header_value}")
  fi
done < <(go run "$ROOT_DIR/hack/authfixture" -secret-file "$secret_file" -user "$QUALIFICATION_USER" -age 10m -format env)
curl --silent --show-error --output /dev/null --write-out '%{http_code}' -H "Host: $backend_host_header" "${stale_header_args[@]}" "$backend_url/" | grep -qx 401 ||
  die "stale signed request was not rejected"
echo "TRUSTED_PROXY_STALE_STATUS status=401"
curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$PROXY_URL/" | grep -qx 200 ||
  die "trusted proxy did not forward authenticated request"

kubectl -n "$RELEASE_NAMESPACE" rollout status deployment/landlock-genprof-operations-center --timeout=5m
kubectl -n "$RELEASE_NAMESPACE" rollout status deployment/landlock-genprof-observation-executor --timeout=5m

workload_name="published-release-workload"
kubectl -n "$RELEASE_NAMESPACE" create deployment "$workload_name" \
  --image=nginx:1.27 -- /bin/sh -c 'while :; do sleep 3600; done' >/dev/null
kubectl -n "$RELEASE_NAMESPACE" rollout status deployment/"$workload_name" --timeout=180s >/dev/null
workload_pod="$(kubectl -n "$RELEASE_NAMESPACE" get pod -l app="$workload_name" -o jsonpath='{.items[0].metadata.name}')"
[ -n "$workload_pod" ] || die "published smoke workload Pod could not be resolved"

curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$PROXY_URL/" | grep -qx 200 ||
  die "trusted proxy could not reach the authenticated Operations Center"
echo "TRUSTED_PROXY_BACKEND_PATH PASS"

oc_image_id="$(kubectl -n "$RELEASE_NAMESPACE" get pod -l app.kubernetes.io/name=operations-center -o jsonpath='{.items[0].status.containerStatuses[0].imageID}')"
executor_image_id="$(kubectl -n "$RELEASE_NAMESPACE" get pod -l app.kubernetes.io/name=observation-executor -o jsonpath='{.items[0].status.containerStatuses[0].imageID}')"
case "$oc_image_id" in *"@$OCI_DIGEST"*) ;; *) die "Operations Center imageID does not match published digest: $oc_image_id" ;; esac
case "$executor_image_id" in *"@$OCI_DIGEST"*) ;; *) die "executor imageID does not match published digest: $executor_image_id" ;; esac
echo "DEPLOYED_DIGEST_MATCH       PASS"
echo "OPERATIONS_CENTER_READY    PASS"
echo "EXECUTOR_READY             PASS"

if [ ! -d "$ROOT_DIR/test/ui/node_modules/playwright" ]; then
  npm install --prefix "$ROOT_DIR/test/ui" --ignore-scripts --no-audit --no-fund >/dev/null
fi
echo "PUBLISHED_VALUE_FLOW_STARTING namespace=$RELEASE_NAMESPACE workload=$workload_name pod=$workload_pod"
UI_URL="$PROXY_URL" \
UI_EXPECTED_WORKLOAD="$workload_name" \
UI_NAMESPACE="$RELEASE_NAMESPACE" \
UI_POD="$workload_pod" \
UI_CONTAINER=nginx \
UI_DIAGNOSTICS_FILE="$WORK_DIR/observation.json" \
NODE_PATH="$ROOT_DIR/test/ui/node_modules" \
  node "$ROOT_DIR/test/ui/workbench-smoke.js" || {
    if [ -s "$WORK_DIR/observation.json" ]; then
      echo "PUBLISHED_OBSERVATION_LAST_STATE $(cat "$WORK_DIR/observation.json")" >&2
    fi
    kubectl -n "$RELEASE_NAMESPACE" logs deployment/landlock-genprof-observation-executor --all-containers=true --tail=200 >&2 || true
    die "published Operations Center value flow failed"
  }
echo "PUBLISHED_VALUE_FLOW PASS"

echo "PUBLISHED_AUTH_PROXY_REQUIRED namespace=$PROXY_NAMESPACE selector=$PROXY_SELECTOR"
echo "PUBLISHED_UI_URL             $PROXY_URL"
echo "PUBLISHED_UI_AUTHENTICATION  must be qualified through the trusted proxy; direct backend access is not used."
echo "PUBLISHED_RELEASE_MODE_READY"
