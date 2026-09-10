#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 TAG EXPECTED_SHA REMOTE_TAG_SHA BUILD_SOURCE_SHA REQUIRED_CONTEXTS CHECK_RUNS E2E_RUNS PR_ONLY_CONTEXTS" >&2
  exit 2
}

[ "$#" -eq 8 ] || usage
tag=$1
expected_sha=$2
remote_tag_sha=$3
build_source_sha=$4
required_contexts=$5
check_runs=$6
e2e_runs=$7
pr_only_contexts=$8

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 2; }
[ -f .github/recovery-release-authorizations ] || { echo "missing recovery authorization file" >&2; exit 2; }

[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "invalid tag format" >&2; exit 1; }
[[ "$expected_sha" =~ ^[0-9a-fA-F]{40}$ ]] || { echo "expected SHA must be full length" >&2; exit 1; }
[[ "$remote_tag_sha" =~ ^[0-9a-fA-F]{40}$ ]] || { echo "remote tag target must be full length" >&2; exit 1; }
[[ "$build_source_sha" =~ ^[0-9a-fA-F]{40}$ ]] || { echo "build source SHA must be full length" >&2; exit 1; }

authorized_sha="$(awk -v wanted="$tag" '$1 == wanted { print $2 }' .github/recovery-release-authorizations)"
[ -n "$authorized_sha" ] || { echo "tag is not explicitly authorized for recovery" >&2; exit 1; }
[ "$expected_sha" = "$authorized_sha" ] || { echo "expected SHA is not independently authorized for tag" >&2; exit 1; }
[ "$remote_tag_sha" = "$expected_sha" ] || { echo "remote tag target does not match authorized SHA" >&2; exit 1; }
[ "$build_source_sha" = "$expected_sha" ] || { echo "build source is not the authorized product SHA" >&2; exit 1; }

hack/release-gate-check.sh \
  "$expected_sha" \
  push \
  "$required_contexts" \
  "$check_runs" \
  "$e2e_runs" \
  "$pr_only_contexts"

echo "BUILD_ELIGIBLE tag=$tag product_sha=$expected_sha"
