#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="$ROOT_DIR/hack/published-trusted-proxy.yaml"

grep -q '^kind: Deployment$' "$MANIFEST"
grep -q '^kind: Service$' "$MANIFEST"
grep -q 'automountServiceAccountToken: false' "$MANIFEST"
grep -q 'type: ClusterIP' "$MANIFEST"
grep -q 'app: trusted-proxy' "$MANIFEST"
grep -q 'secretName: __SECRET__' "$MANIFEST"
grep -q 'image: __IMAGE__' "$MANIFEST"
grep -q 'readinessProbe:' "$MANIFEST"
grep -q 'livenessProbe:' "$MANIFEST"
grep -q 'runAsNonRoot: true' "$MANIFEST"
grep -q 'readOnlyRootFilesystem: true' "$MANIFEST"

if grep -qE '^kind: (ServiceAccount|Role|ClusterRole|RoleBinding|ClusterRoleBinding)$' "$MANIFEST"; then
  echo 'trusted-proxy fixture must not request Kubernetes authority' >&2
  exit 1
fi
if grep -qE '(^|[[:space:]])(\*|cluster-admin)([[:space:]]|$)' "$MANIFEST"; then
  echo 'trusted-proxy fixture contains wildcard authority' >&2
  exit 1
fi
if grep -qE 'type: (NodePort|LoadBalancer)' "$MANIFEST"; then
  echo 'trusted-proxy fixture must remain ClusterIP-only' >&2
  exit 1
fi

echo 'published trusted-proxy fixture tests: PASS'
