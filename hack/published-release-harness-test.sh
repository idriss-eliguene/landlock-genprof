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

grep -q 'LOCAL_FALLBACK.*DISABLED' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'docker buildx imagetools inspect' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
grep -q 'helm pull' "$ROOT_DIR/hack/ui-lima-auth-release.sh"
if grep -q 'go run ./cmd/landlock-genprof' "$ROOT_DIR/hack/ui-lima-auth-release.sh"; then exit 1; fi
if grep -qE '^[[:space:]]*(docker[[:space:]]+load|kind[[:space:]]+load)' "$ROOT_DIR/hack/ui-lima-auth-release.sh"; then exit 1; fi
echo 'published-release-harness tests: PASS'
