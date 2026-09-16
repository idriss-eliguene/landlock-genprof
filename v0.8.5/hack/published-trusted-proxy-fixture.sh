#!/usr/bin/env bash
set -Eeuo pipefail

# Qualification-only in-cluster fixture. It reuses hack/trustedproxy, mounts
# only the generated HMAC Secret, has no ServiceAccount token/RBAC, and is
# selected by the Operations Center NetworkPolicy as an external proxy.

NAMESPACE=${PUBLISHED_PROXY_NAMESPACE:?PUBLISHED_PROXY_NAMESPACE is required}
IMAGE=${PUBLISHED_PROXY_IMAGE:?PUBLISHED_PROXY_IMAGE is required}
IMAGE_DIGEST=${PUBLISHED_PROXY_IMAGE_DIGEST:?PUBLISHED_PROXY_IMAGE_DIGEST is required}
SECRET=${PUBLISHED_PROXY_SECRET_NAME:-published-release-hmac}
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
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
  "$ROOT_DIR/hack/published-trusted-proxy.yaml" >"$MANIFEST"

kubectl apply -f "$MANIFEST" >/dev/null
kubectl -n "$NAMESPACE" rollout status deployment/published-trusted-proxy --timeout=180s
kubectl -n "$NAMESPACE" wait --for=condition=available deployment/published-trusted-proxy --timeout=30s >/dev/null
kubectl -n "$NAMESPACE" get pod -l app=trusted-proxy -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' | grep -Fq "@${IMAGE_DIGEST}"
echo "PUBLISHED_TRUSTED_PROXY_READY namespace=$NAMESPACE image=${IMAGE}@${IMAGE_DIGEST}"
