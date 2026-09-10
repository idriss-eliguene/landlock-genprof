#!/usr/bin/env bash
set -Eeuo pipefail

# Published-artifact-only Lima/IHM qualification harness. It deliberately
# does not share the local launcher path: no local chart, local image,
# docker load, kind load, or source build is permitted here.

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
RELEASE_NAMESPACE="${PUBLISHED_RELEASE_NAMESPACE:-landlock-genprof-release}"
HELM_RELEASE="${PUBLISHED_HELM_RELEASE:-landlock-genprof-release}"
QUALIFICATION_USER="${QUALIFICATION_USER:-qualification-operator}"
PROXY_NAMESPACE="${PUBLISHED_PROXY_NAMESPACE:-}"
PROXY_SELECTOR="${PUBLISHED_PROXY_SELECTOR:-app=trusted-proxy}"
PROXY_URL="${PUBLISHED_PROXY_URL:-}"
WORK_DIR=""
CREATED_NAMESPACE=0

die() { echo "ERROR: $*" >&2; exit 1; }

usage() {
  cat >&2 <<'EOF'
Usage: RELEASE_VERSION=v0.8.1 hack/ui-lima-auth-release.sh

Set PUBLISHED_RELEASE_VALIDATE_ONLY=1 for non-mutating reference/input
validation. Published mode never falls back to a local chart or image.
EOF
  exit 2
}

cleanup() {
  local status=$?
  if [ "$VALIDATE_ONLY" != 1 ] && [ "$CREATED_NAMESPACE" -eq 1 ]; then
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

echo "MODE                         published-release"
echo "RELEASE                      $RELEASE_VERSION"
echo "HELM                         ${CHART_REF}:$CHART_VERSION"
echo "OCI IMAGE                    $IMAGE_REF"
echo "LOCAL_FALLBACK               DISABLED"

if [ "$VALIDATE_ONLY" = 1 ]; then
  echo "VALIDATE_ONLY                PASS"
  exit 0
fi

lib_core_readiness_require_commands curl docker helm kubectl limactl make jq
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

[ -n "$PROXY_NAMESPACE" ] || die "PUBLISHED_PROXY_NAMESPACE is required; published mode will not bypass NetworkPolicy"
[ -n "$PROXY_URL" ] || die "PUBLISHED_PROXY_URL is required; published mode will not bypass authenticated UI qualification"
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
kubectl config view --raw --minify >"$secret_dir/kubeconfig"
chmod 600 "$secret_dir/kubeconfig"
kubectl -n "$RELEASE_NAMESPACE" create secret generic published-release-executor-kubeconfig \
  --from-file=config="$secret_dir/kubeconfig" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

echo "PUBLISHED_HELM_INSTALL_STARTING"
helm upgrade --install "$HELM_RELEASE" "$CHART_ARCHIVE" \
  --namespace "$RELEASE_NAMESPACE" \
  --set namespace.name="$RELEASE_NAMESPACE" \
  --set operationsCenter.enabled=true \
  --set operationsCenter.image.repository="${IMAGE_REF%:*}" \
  --set operationsCenter.image.tag="$RELEASE_VERSION" \
  --set operationsCenter.trustedProxySecret.name=published-release-hmac \
  --set operationsCenter.executorKubeconfigSecret.name=published-release-executor-kubeconfig \
  --set operationsCenter.impersonation.allowedUsers[0]="$QUALIFICATION_USER" \
  --set operationsCenter.teamRoles.create=true \
  --set operationsCenter.networkPolicy.trustedProxy.namespaceSelector.matchLabels.kubernetes\.io/metadata\.name="$PROXY_NAMESPACE" \
  --set operationsCenter.networkPolicy.trustedProxy.podSelector."${PROXY_SELECTOR%%=*}"="${PROXY_SELECTOR#*=}" \
  --set operationsCenter.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 \
  --set observationExecutor.enabled=true \
  --set observationExecutor.image.repository="${IMAGE_REF%:*}" \
  --set observationExecutor.image.tag="$RELEASE_VERSION" \
  --set observationExecutor.targetNamespaces[0]="$RELEASE_NAMESPACE" \
  --set observationExecutor.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 \
  --wait --timeout 5m >/dev/null

kubectl -n "$RELEASE_NAMESPACE" rollout status deployment/landlock-genprof-operations-center --timeout=5m
kubectl -n "$RELEASE_NAMESPACE" rollout status deployment/landlock-genprof-observation-executor --timeout=5m

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
