#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/published-release-harness-lib.sh"

expect_ok() { "$@" >/dev/null; }
expect_fail() { if "$@" >/dev/null 2>&1; then echo "unexpected success: $*" >&2; exit 1; fi; }

[ "$(published_release_image_reference v0.8.1)" = ghcr.io/idriss-eliguene/landlock-genprof-operations-center:v0.8.1 ]
[ "$(published_release_chart_reference v0.8.1)" = oci://ghcr.io/idriss-eliguene/charts/landlock-genprof ]
[ "$(published_release_chart_version v0.8.1)" = 0.8.1 ]
expect_ok published_release_validate_tag_ref v0.8.1
expect_fail published_release_validate_tag_ref ''
expect_fail published_release_validate_tag_ref latest
expect_fail published_release_validate_tag_ref --v0.8.1
expect_fail published_release_validate_tag_ref 'v0.8.1:refs/tags/x'
expect_fail published_release_validate_tag_ref 'v0.8.1 bad'
expect_fail published_release_validate_tag_ref 'refs/heads/master'
expect_fail published_release_require_full_digest sha256:deadbeef
expect_ok published_release_require_full_digest sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
expect_ok published_release_port_is_free 65530
digest_value=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
[ "$(published_release_normalize_digest "$digest_value")" = "$digest_value" ]
[ "$(published_release_normalize_digest "image:tag@$digest_value")" = "$digest_value" ]
[ "$(published_release_normalize_digest $'  '$digest_value$'\r\n')" = "$digest_value" ]
expect_fail published_release_normalize_digest "sha256:$digest_value"
expect_fail published_release_normalize_digest 'sha256:0123456789abcdef'
expect_fail published_release_normalize_digest 'sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdeg'
expect_fail published_release_normalize_digest "$digest_value,$digest_value"

grep -q 'LOCAL_FALLBACK.*DISABLED' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q "^EXPECTED_CONTEXT=\"kind-\${LIMA_VM}\"$" "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'docker build --provenance=false -f .*Dockerfile.trustedproxy' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'kind load docker-image' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'ctr -n k8s.io images tag' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'node_ctr_digest_ref=' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'node_ctr_digest_ref"' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'TRUSTED_PROXY_NODE_DIGEST_VERIFIED' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
expected_reference='docker.io/library/trustedproxy@sha256:0123456789012345678901234567890123456789012345678901234567890123'
matching_names=$'docker.io/library/trustedproxy:qualification@sha256:0123456789012345678901234567890123456789012345678901234567890123\ndocker.io/library/trustedproxy@sha256:0123456789012345678901234567890123456789012345678901234567890123\nimport-2026-09-17@sha256:0123456789012345678901234567890123456789012345678901234567890123\nimport-2026-09-16@sha256:0123456789012345678901234567890123456789012345678901234567890123'
[ "$(published_release_select_containerd_source "${expected_reference##*@}" "$matching_names")" = "import-2026-09-16@${expected_reference##*@}" ]
expect_ok published_release_verify_containerd_reference "$expected_reference" "$matching_names"
expect_fail published_release_verify_containerd_reference "$expected_reference" "${matching_names//$expected_reference/}"
expect_fail published_release_verify_containerd_reference "${expected_reference/@sha256:*/@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff}" "$matching_names"
index_inspect=$'application/vnd.oci.image.index.v1+json @sha256:0123456789012345678901234567890123456789012345678901234567890123\napplication/vnd.oci.image.manifest.v1+json @sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nPlatform: linux/arm64\napplication/vnd.oci.image.manifest.v1+json @sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\nPlatform: linux/amd64'
[ "$(published_release_platform_manifest_digest "$index_inspect" linux/arm64)" = sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ]
[ "$(published_release_platform_manifest_digest "$index_inspect" linux/amd64)" = sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb ]
expect_fail published_release_platform_manifest_digest "$index_inspect" linux/s390x
canonical_index='{"manifests":[{"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","platform":{"os":"linux","architecture":"arm64"}}]}'
runtime_index='{"manifests":[{"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","platform":{"os":"linux","architecture":"arm64"}}]}'
canonical_manifest='{"config":{"digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},"layers":[{"digest":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}]}'
runtime_manifest="$canonical_manifest"
expect_ok published_release_verify_oci_custody "$canonical_index" "$runtime_index" "$canonical_manifest" "$runtime_manifest" linux/arm64
expect_fail published_release_verify_oci_custody "$canonical_index" '{"manifests":[]}' "$canonical_manifest" "$runtime_manifest" linux/arm64
expect_fail published_release_verify_oci_custody "$canonical_index" "$runtime_index" "$canonical_manifest" '{"config":{"digest":"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},"layers":[]}' linux/arm64
set +e
set -o pipefail
yes | grep -q y
sigpipe_rc=$?
set +o pipefail
set -e
[ "$sigpipe_rc" -eq 141 ]
safe_names="$matching_names"
expect_ok published_release_verify_containerd_reference "$expected_reference" "$safe_names"
if grep -q 'imagePullPolicy: Always' "$ROOT_DIR/hack/published-trusted-proxy.yaml"; then exit 1; fi
grep -q 'fsGroup: 65532' "$ROOT_DIR/hack/published-trusted-proxy.yaml"
grep -q 'defaultMode: 0440' "$ROOT_DIR/hack/published-trusted-proxy.yaml"
grep -q 'runAsNonRoot: true' "$ROOT_DIR/hack/published-trusted-proxy.yaml"
if grep -q 'defaultMode: 0\?644' "$ROOT_DIR/hack/published-trusted-proxy.yaml"; then exit 1; fi
grep -q 'helm pull' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'rbac\.legacyClusterRoles\.create=false' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'operationsCenter.backendRoleName=' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'observationExecutor.clusterIdentityRoleName=' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'observationExecutor.gadgetAccess.roleName=' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'trustedProxy.podSelector.matchLabels' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'backend_host_header="landlock-genprof-operations-center.' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'header_name//_/-};' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'teamRoleBindings\[2\].role=landlock-genprof-team-viewer' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'workbench-smoke.js' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'observationState(observation) === "RUNNING"' "$ROOT_DIR/test/ui/workbench-smoke.js"
grep -q 'resourceNames: \["kube-system"\]' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'delete clusterrolebinding.*IDENTITY_READER_BINDING' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
rendered_network_policy="$(mktemp -t landlock-published-network-policy.XXXXXX.yaml)"
network_policy_selector_excerpt="${rendered_network_policy}.excerpt"
trap 'rm -f "$rendered_network_policy" "$network_policy_selector_excerpt"' EXIT
helm template qualification "$ROOT_DIR/deploy/helm/landlock-genprof" \
  --namespace qualification \
  --set namespace.create=false \
  --set namespace.name=qualification \
  --set operationsCenter.enabled=true \
  --set operationsCenter.trustedProxySecret.name=hmac \
  --set operationsCenter.executorKubeconfigSecret.name=kubeconfig \
  --set operationsCenter.impersonation.allowedUsers[0]=qualification-operator \
  --set-string "$(published_release_namespace_selector_set_arg qualification)" \
  --set operationsCenter.networkPolicy.trustedProxy.podSelector.matchLabels.app=trusted-proxy \
  --set operationsCenter.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 \
  >"$rendered_network_policy"
grep -A3 '^        - namespaceSelector:' "$rendered_network_policy" >"$network_policy_selector_excerpt"
grep -q 'kubernetes.io/metadata.name: qualification' "$network_policy_selector_excerpt"
if grep -q 'kubernetes:$' "$rendered_network_policy"; then
  echo 'namespace selector label key was parsed as a nested Helm object' >&2
  exit 1
fi
grep -q 'helm uninstall' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
if grep -q -- '--take-ownership' "$ROOT_DIR/hack/ui-lima-auth-release.sh"; then exit 1; fi
if grep -q 'go run ./cmd/landlock-genprof' "$ROOT_DIR/hack/ui-lima-auth-release.sh"; then exit 1; fi
if grep -qE '^[[:space:]]*docker[[:space:]]+load' "$ROOT_DIR/hack/ui-lima-auth-release.sh"; then exit 1; fi
"$ROOT_DIR/hack/published-trusted-proxy-fixture-test.sh"
"$ROOT_DIR/hack/published-rbac-ownership-test.sh"
echo 'published-release-harness tests: PASS'
