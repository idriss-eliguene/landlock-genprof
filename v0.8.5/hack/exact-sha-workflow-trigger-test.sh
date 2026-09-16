#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"

assert_workflow_triggers_on_pull_request_and_master_push() {
  local workflow=$1
  grep -Eq '^  push:$' "$root_dir/.github/workflows/$workflow"
  grep -Eq '^    branches: \[master\]$' "$root_dir/.github/workflows/$workflow"
  grep -Eq '^  pull_request:$' "$root_dir/.github/workflows/$workflow"
}

for workflow in core-e2e.yml spo-e2e.yml spo-dmin-e2e.yml; do
  assert_workflow_triggers_on_pull_request_and_master_push "$workflow"
done

security_block="$(awk '
  /^  security:/ { in_block=1 }
  in_block { print }
  in_block && /^  [a-zA-Z0-9_-]+:/ && $0 !~ /^  security:/ { exit }
' "$root_dir/.github/workflows/ci.yml")"
grep -Eq "github\.event_name == 'pull_request'" <<<"$security_block"
grep -Eq "github\.event_name == 'push'" <<<"$security_block"
grep -Eq "github\.event_name == 'workflow_dispatch'" <<<"$security_block"

if grep -R -nE 'pull_request_target|branches/master/protection|required_status_checks' \
  "$root_dir/.github/workflows/release.yml"; then
  echo "unsafe release-gate trigger or branch-protection read found" >&2
  exit 1
fi

echo "exact-SHA workflow trigger tests passed"
