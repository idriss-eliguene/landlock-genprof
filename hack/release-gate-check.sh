#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 EXPECTED_SHA EVENT REQUIRED_CONTEXTS CHECK_RUNS E2E_RUNS PR_ONLY_CONTEXTS" >&2
  exit 2
}

[ "$#" -eq 6 ] || usage
expected_sha=$1
event=$2
required_contexts=$3
check_runs=$4
e2e_runs=$5
pr_only_contexts=$6

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 2; }
[ -f "$required_contexts" ] || { echo "missing required-contexts file" >&2; exit 2; }
[ -f "$check_runs" ] || { echo "missing check-runs JSON" >&2; exit 2; }
[ -f "$e2e_runs" ] || { echo "missing E2E-runs JSON" >&2; exit 2; }
[ -f "$pr_only_contexts" ] || { echo "missing PR-only contexts file" >&2; exit 2; }

if [[ ! "$expected_sha" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo "invalid expected SHA: $expected_sha" >&2
  exit 2
fi

is_pr_only() {
  awk -v wanted="$1" '
    /^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
    { gsub(/^[[:space:]]+|[[:space:]]+$/, ""); if ($0 == wanted) found=1 }
    END { exit(found ? 0 : 1) }
  ' "$pr_only_contexts"
}

fail() {
  echo "::error::$*" >&2
  exit 1
}

check_run_for() {
  local context=$1
  jq -e -c --arg name "$context" --arg sha "$expected_sha" '
    [ .check_runs[]?
      | select(type == "object" and .name == $name and .head_sha == $sha)
    ] as $runs
    | if ($runs | length) == 0 then empty
      else
        ($runs | map(.completed_at // .started_at // "") | max) as $latest
        | [ $runs[] | select((.completed_at // .started_at // "") == $latest) ] as $latest_runs
        | if ($latest_runs | length) != 1 then empty
          else $latest_runs[0] | select(.status == "completed" and .conclusion == "success")
          end
      end
  ' "$check_runs" 2>/dev/null || true
}

while IFS= read -r context || [ -n "$context" ]; do
  context="${context#"${context%%[![:space:]]*}"}"
  context="${context%"${context##*[![:space:]]}"}"
  [ -z "$context" ] && continue
  [[ "$context" == \#* ]] && continue

  if is_pr_only "$context"; then
    if [ "$event" != "pull_request" ]; then
      echo "check '$context': not applicable on event '$event' (PR-only governance)"
      continue
    fi
  fi

  run="$(check_run_for "$context")"
  if [ -z "$run" ]; then
    fail "required source check '$context' has no successful completed run on exact SHA $expected_sha"
  fi
  echo "check '$context' on $expected_sha: success"
done < "$required_contexts"

for workflow in core-e2e.yml spo-e2e.yml spo-dmin-e2e.yml; do
  run="$(jq -e -c --arg workflow "$workflow" --arg sha "$expected_sha" '
    [ .[]
      | select(.releaseWorkflow == $workflow and .headSha == $sha)
    ]
    | sort_by(.createdAt // "")
    | last // empty
    | select(.status == "completed" and .conclusion == "success")
  ' "$e2e_runs" 2>/dev/null || true)"
  if [ -z "$run" ]; then
    fail "mandatory E2E '$workflow' has no successful completed run on exact SHA $expected_sha"
  fi
  echo "workflow '$workflow' on $expected_sha: success"
done

echo "release source and E2E gates passed for exact SHA $expected_sha"
