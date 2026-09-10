#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
workflow="$root_dir/.github/workflows/release-recovery.yml"

grep -Eq '^  workflow_dispatch:' "$workflow"
if grep -Eq '^  (pull_request|push|release|workflow_run|schedule|repository_dispatch):' "$workflow"; then
  echo "automatic recovery trigger present" >&2
  exit 1
fi

qualify_block="$(sed -n '/^  qualify:/,/^  publish:/p' "$workflow")"
publish_block="$(sed -n '/^  publish:/,$p' "$workflow")"

grep -Fq "if: \${{ inputs.mode == 'publish' }}" <<<"$publish_block"
grep -Fq 'needs: qualify' <<<"$publish_block"
grep -Fq 'packages: write' <<<"$publish_block"
grep -Fq 'contents: write' <<<"$publish_block"
grep -Fq 'path: control-plane' <<<"$publish_block"
grep -Fq "refs/tags/recovery-\${tag}" <<<"$publish_block"
grep -Fq 'product_sha' <<<"$publish_block"
grep -Fq 'docker/build-push-action@v6' <<<"$publish_block"
grep -Fq 'push: true' <<<"$publish_block"
grep -Fq 'helm push' <<<"$publish_block"
grep -Fq 'goreleaser-action@v7' <<<"$publish_block"

assert_absent() {
  if grep -Fq "$1" <<<"$2"; then
    echo "unexpected qualification publication construct: $1" >&2
    exit 1
  fi
}

assert_absent 'push: true' "$qualify_block"
assert_absent 'helm push' "$qualify_block"
assert_absent 'goreleaser-action@v7' "$qualify_block"
assert_absent 'packages: write' "$qualify_block"

echo "recovery publication gate structural tests passed"
