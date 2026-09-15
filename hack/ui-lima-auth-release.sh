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
PROXY_NAMESPACE="${PUBLISHED_PROXY_NAMESPACE:-$RELEASE_NAMESPACE}"
PROXY_SELECTOR="${PUBLISHED_PROXY_SELECTOR:-app=trusted-proxy}"
PROXY_URL="${PUBLISHED_PROXY_URL:-http://127.0.0.1:8090}"
PROXY_IMAGE="${PUBLISHED_PROXY_IMAGE:-}"
PROXY_SOURCE_REVISION="$(git -C "$ROOT_DIR" rev-parse HEAD)"
PROXY_IMAGE_TAG="${PUBLISHED_PROXY_IMAGE_TAG:-qualification-${PROXY_SOURCE_REVISION:0:12}}"
PROXY_FORWARD_PID=""
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
  if [ "$VALIDATE_ONLY" != 1 ] && [ "$CREATED_NAMESPACE" -eq 1 ]; then
    helm uninstall "$HELM_RELEASE" --namespace "$RELEASE_NAMESPACE" >/dev/null 2>&1 || true
    kubectl delete namespace "$RELEASE_NAMESPACE" --ignore-not-found >/dev/null 2>&1 || true
  fi
  [ -z "$WORK_DIR" ] || rm -rf "$WORK_DIR"
  lib_core_readiness_cleanup
  exit "$status"
}
trap cleanup EXIT

[ -n "$RELEASE_VERSION" ] || usage
published_release_validate_tag_ref "$RELEASE_VERSION" || exit 2
IMAGE_REF="$(published_release_image_reference "$RELEASE_VERSION")"
CHART_REF="$(published_release_chart_reference "$RELEASE_VERSION")"
CHART_VERSION="$(published_release_chart_version "$RELEASE_VERSION")"

case "$RELEASE_NAMESPACE" in
  ''|*[!a-z0-9-]*|[0-9]*-|-[0-9]*) die "invalid disposable namespace" ;;
esac
case "$HELM_RELEASE" in
  ''|*[!a-z0-9-]*) die "invalid Helm release name" ;;
esac
for name in "$OPERATIONS_CENTER_BACKEND_ROLE" "$OPERATIONS_CENTER_BACKEND_BINDING" "$EXECUTOR_IDENTITY_ROLE" "$EXECUTOR_GADGET_ROLE"; do
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

lib_core_readiness_require_commands curl docker helm kubectl kind limactl make jq
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
  --set operationsCenter.networkPolicy.trustedProxy.namespaceSelector.matchLabels.kubernetes\.io/metadata\.name="$PROXY_NAMESPACE" \
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
docker build -f "$ROOT_DIR/Dockerfile.trustedproxy" -t "$PROXY_IMAGE" "$ROOT_DIR" >/dev/null ||
  die "trusted proxy image build failed"
PROXY_IMAGE_DIGEST="$(docker image inspect "$PROXY_IMAGE" --format '{{index .RepoDigests 0}}' | awk -F@ '{print $2}')"
published_release_require_full_digest "$PROXY_IMAGE_DIGEST" || die "trusted proxy image digest could not be resolved"
echo "TRUSTED_PROXY_LOAD_STARTING digest=$PROXY_IMAGE_DIGEST"
kind load docker-image "$PROXY_IMAGE@$PROXY_IMAGE_DIGEST" --name "$LIMA_VM" >/dev/null ||
  die "trusted proxy image load into kind failed"
proxy_node="$(kind get nodes --name "$LIMA_VM" | head -n 1)"
[ -n "$proxy_node" ] || die "kind node could not be resolved for trusted proxy verification"
node_digest_ref="$(docker exec "$proxy_node" crictl images -o json |
  jq -r --arg digest "$PROXY_IMAGE_DIGEST" '
    .images[] | select(any(.repoDigests[]?; endswith($digest))) |
    .repoDigests[] | select(endswith($digest))' | head -n 1)"
[ -n "$node_digest_ref" ] || die "loaded trusted proxy digest is absent from kind node"
canonical_proxy_ref="docker.io/library/${PROXY_IMAGE%@*}@$PROXY_IMAGE_DIGEST"
docker exec "$proxy_node" ctr -n k8s.io images tag --force "$node_digest_ref" "$canonical_proxy_ref" >/dev/null ||
  die "could not register trusted proxy digest under its Kubernetes image reference"
docker exec "$proxy_node" ctr -n k8s.io images ls | grep -Fq "$canonical_proxy_ref" ||
  die "canonical trusted proxy digest reference is absent from kind node"
echo "TRUSTED_PROXY_NODE_DIGEST_VERIFIED digest=$PROXY_IMAGE_DIGEST"
PUBLISHED_PROXY_NAMESPACE="$PROXY_NAMESPACE" \
PUBLISHED_PROXY_IMAGE="$PROXY_IMAGE" \
PUBLISHED_PROXY_IMAGE_DIGEST="$PROXY_IMAGE_DIGEST" \
PUBLISHED_PROXY_SECRET_NAME=published-release-hmac \
  "$ROOT_DIR/hack/published-trusted-proxy-fixture.sh"

kubectl -n "$RELEASE_NAMESPACE" port-forward svc/published-trusted-proxy 8090:8090 >"$WORK_DIR/proxy-forward.log" 2>&1 &
PROXY_FORWARD_PID=$!
for _ in $(seq 1 30); do
  if ! kill -0 "$PROXY_FORWARD_PID" >/dev/null 2>&1; then
    die "trusted proxy port-forward exited"
  fi
  if curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$PROXY_URL/" | grep -qx 401; then
    break
  fi
  sleep 1
done
curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$PROXY_URL/" | grep -qx 401 ||
  die "trusted proxy did not reject unsigned request"
echo "TRUSTED_PROXY_READY       PASS"

kubectl -n "$RELEASE_NAMESPACE" rollout status deployment/landlock-genprof-operations-center --timeout=5m
kubectl -n "$RELEASE_NAMESPACE" rollout status deployment/landlock-genprof-observation-executor --timeout=5m

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

echo "PUBLISHED_AUTH_PROXY_REQUIRED namespace=$PROXY_NAMESPACE selector=$PROXY_SELECTOR"
echo "PUBLISHED_UI_URL             $PROXY_URL"
echo "PUBLISHED_UI_AUTHENTICATION  must be qualified through the trusted proxy; direct backend access is not used."
echo "PUBLISHED_RELEASE_MODE_READY"
