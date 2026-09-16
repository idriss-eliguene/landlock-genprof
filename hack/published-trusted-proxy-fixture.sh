#!/usr/bin/env bash
set -Eeuo pipefail

# Qualification-only in-cluster fixture. It reuses hack/trustedproxy, mounts
# only the generated HMAC Secret, has no ServiceAccount token/RBAC, and is
# selected by the Operations Center NetworkPolicy as an external proxy.

NAMESPACE=${PUBLISHED_PROXY_NAMESPACE:?PUBLISHED_PROXY_NAMESPACE is required}
NODE=${PUBLISHED_PROXY_NODE:?PUBLISHED_PROXY_NODE is required}
PLATFORM=${PUBLISHED_PROXY_PLATFORM:?PUBLISHED_PROXY_PLATFORM is required}
IMAGE=${PUBLISHED_PROXY_IMAGE:?PUBLISHED_PROXY_IMAGE is required}
IMAGE_DIGEST=${PUBLISHED_PROXY_IMAGE_DIGEST:?PUBLISHED_PROXY_IMAGE_DIGEST is required}
BACKEND_HOST=${PUBLISHED_PROXY_BACKEND_HOST:?PUBLISHED_PROXY_BACKEND_HOST is required}
SECRET=${PUBLISHED_PROXY_SECRET_NAME:-published-release-hmac}
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/published-release-harness-lib.sh"
MANIFEST="$(mktemp -t landlock-genprof-published-proxy.XXXXXX.yaml)"
trap 'rm -f "$MANIFEST"' EXIT

[[ "$NAMESPACE" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]] || {
  echo "invalid proxy namespace" >&2
  exit 2
}
[[ "$IMAGE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || {
  echo "proxy image digest must be a full sha256 digest" >&2
  exit 2
}

sed \
  -e "s|__NAMESPACE__|$NAMESPACE|g" \
  -e "s|__IMAGE__|${IMAGE}@${IMAGE_DIGEST}|g" \
  -e "s|__SECRET__|$SECRET|g" \
  -e "s|__BACKEND_HOST__|$BACKEND_HOST|g" \
  "$ROOT_DIR/hack/published-trusted-proxy.yaml" >"$MANIFEST"

kubectl apply -f "$MANIFEST" >/dev/null
kubectl -n "$NAMESPACE" rollout status deployment/published-trusted-proxy --timeout=180s
kubectl -n "$NAMESPACE" wait --for=condition=available deployment/published-trusted-proxy --timeout=30s >/dev/null
runtime_image_id="$(kubectl -n "$NAMESPACE" get pod -l app=trusted-proxy -o jsonpath='{.items[0].status.containerStatuses[0].imageID}')"
runtime_digest="${runtime_image_id##*@}"
[[ "$runtime_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || {
  echo "runtime imageID has no valid digest: $runtime_image_id" >&2
  exit 1
}
canonical_content_digest="$(published_release_normalize_digest "$IMAGE_DIGEST")"
runtime_content_digest="$(published_release_normalize_digest "$runtime_image_id")"
canonical_index="$(docker exec "$NODE" sh -c "ctr -n k8s.io content get ${canonical_content_digest}")"
runtime_index="$(docker exec "$NODE" sh -c "ctr -n k8s.io content get ${runtime_content_digest}")"
if jq -e 'has("config") and has("layers")' <<<"$canonical_index" >/dev/null; then
  canonical_manifest_digest="$canonical_content_digest"
  runtime_manifest_digest="$canonical_content_digest"
  canonical_manifest="$canonical_index"
  runtime_manifest="$canonical_index"
  if [ "$runtime_content_digest" != "$canonical_content_digest" ]; then
    jq -e --arg digest "$runtime_content_digest" '.config.digest == $digest' <<<"$canonical_manifest" >/dev/null || {
      echo "runtime content differs from the single-platform manifest: expected=$runtime_content_digest" >&2
      exit 1
    }
  fi
else
  canonical_manifest_digest="$(published_release_oci_platform_manifest_digest "$canonical_index" "$PLATFORM")"
  runtime_manifest_digest="$(published_release_oci_platform_manifest_digest "$runtime_index" "$PLATFORM")"
  [ "$canonical_manifest_digest" = "$runtime_manifest_digest" ] || {
    echo "runtime platform manifest differs: canonical=$canonical_manifest_digest runtime=$runtime_manifest_digest" >&2
    exit 1
  }
  canonical_manifest="$(docker exec "$NODE" sh -c "ctr -n k8s.io content get $(published_release_normalize_digest "$canonical_manifest_digest")")"
  runtime_manifest="$(docker exec "$NODE" sh -c "ctr -n k8s.io content get $(published_release_normalize_digest "$runtime_manifest_digest")")"
  published_release_verify_oci_custody "$canonical_index" "$runtime_index" "$canonical_manifest" "$runtime_manifest" "$PLATFORM"
fi
echo "PUBLISHED_TRUSTED_PROXY_READY namespace=$NAMESPACE image=${IMAGE}@${IMAGE_DIGEST}"
