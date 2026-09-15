#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
gate="$root_dir/hack/release-gate-check.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

sha=1111111111111111111111111111111111111111
wrong_sha=2222222222222222222222222222222222222222
contexts="$tmp_dir/contexts"
pr_only="$tmp_dir/pr-only"
checks="$tmp_dir/checks.json"
e2e="$tmp_dir/e2e.json"

cat > "$contexts" <<'EOF'
build-and-test
security
lint-pr-title
EOF
printf '%s\n' lint-pr-title > "$pr_only"

cat > "$checks" <<EOF
{"check_runs":[
  {"name":"build-and-test","head_sha":"$sha","status":"completed","conclusion":"success","completed_at":"2026-01-01T00:00:01Z"},
  {"name":"security","head_sha":"$sha","status":"completed","conclusion":"success","completed_at":"2026-01-01T00:00:02Z"},
  {"name":"lint-pr-title","head_sha":"$sha","status":"completed","conclusion":"success","completed_at":"2026-01-01T00:00:03Z"}
]}
EOF
cat > "$e2e" <<EOF
[
  {"releaseWorkflow":"core-e2e.yml","headSha":"$sha","status":"completed","conclusion":"success","createdAt":"2026-01-01T00:00:01Z"},
  {"releaseWorkflow":"spo-e2e.yml","headSha":"$sha","status":"completed","conclusion":"success","createdAt":"2026-01-01T00:00:02Z"},
  {"releaseWorkflow":"spo-dmin-e2e.yml","headSha":"$sha","status":"completed","conclusion":"success","createdAt":"2026-01-01T00:00:03Z"}
]
EOF

expect_pass() {
  "$@" >/dev/null
}
expect_fail() {
  if "$@" >/dev/null 2>&1; then
    echo "expected failure but command passed: $*" >&2
    exit 1
  fi
}
run_gate() {
  "$gate" "$1" "$2" "$contexts" "$checks" "$e2e" "$pr_only"
}

expect_pass run_gate "$sha" push
expect_pass run_gate "$sha" workflow_dispatch

# PR-only governance remains required on pull requests.
expect_pass run_gate "$sha" pull_request
jq '.check_runs |= map(if .name == "lint-pr-title" then .conclusion = "failure" else . end)' "$checks" > "$tmp_dir/failed-pr.json"
expect_fail "$gate" "$sha" pull_request "$contexts" "$tmp_dir/failed-pr.json" "$e2e" "$pr_only"

# Every required source failure mode fails closed.
for mode in missing failed cancelled skipped wrong-sha; do
  case "$mode" in
    missing) jq 'del(.check_runs[] | select(.name == "security"))' "$checks" > "$tmp_dir/case.json" ;;
    failed) jq '.check_runs |= map(if .name == "security" then .conclusion = "failure" else . end)' "$checks" > "$tmp_dir/case.json" ;;
    cancelled) jq '.check_runs |= map(if .name == "security" then .conclusion = "cancelled" else . end)' "$checks" > "$tmp_dir/case.json" ;;
    skipped) jq '.check_runs |= map(if .name == "security" then .conclusion = "skipped" else . end)' "$checks" > "$tmp_dir/case.json" ;;
    wrong-sha) jq --arg sha "$wrong_sha" '.check_runs |= map(if .name == "security" then .head_sha = $sha else . end)' "$checks" > "$tmp_dir/case.json" ;;
  esac
  expect_fail "$gate" "$sha" push "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"
done

# Pending, empty, malformed, and ambiguous check-run data fail closed.
jq '.check_runs |= map(if .name == "security" then .status = "in_progress" else . end)' "$checks" > "$tmp_dir/case.json"
expect_fail "$gate" "$sha" push "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"
printf '{"check_runs":[]}' > "$tmp_dir/case.json"
expect_fail "$gate" "$sha" push "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"
printf '{not-json}\n' > "$tmp_dir/case.json"
expect_fail "$gate" "$sha" push "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"
jq '.check_runs += [{"name":"security","head_sha":"'"$sha"'","status":"completed","conclusion":"success","completed_at":"2026-01-01T00:00:02Z"}]' "$checks" > "$tmp_dir/case.json"
expect_fail "$gate" "$sha" push "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"

# A newer failed run cannot be masked by an older successful run.
jq '.check_runs += [{"name":"security","head_sha":"'"$sha"'","status":"completed","conclusion":"failure","completed_at":"2026-01-01T00:01:00Z"}]' "$checks" > "$tmp_dir/case.json"
expect_fail "$gate" "$sha" push "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"

# A tag pointing at a different commit cannot reuse source/E2E evidence: the
# expected tag SHA is the evaluator's exact SHA input.
expect_fail run_gate "$wrong_sha" push

# Incomplete and failed E2E qualification fail closed.
jq 'del(.[] | select(.releaseWorkflow == "spo-e2e.yml"))' "$e2e" > "$tmp_dir/case-e2e.json"
expect_fail "$gate" "$sha" push "$contexts" "$checks" "$tmp_dir/case-e2e.json" "$pr_only"
jq '.[] |= if .releaseWorkflow == "core-e2e.yml" then .conclusion = "failure" else . end' "$e2e" > "$tmp_dir/case-e2e.json"
expect_fail "$gate" "$sha" push "$contexts" "$checks" "$tmp_dir/case-e2e.json" "$pr_only"

# A newer failed E2E run cannot be masked by an older successful run.
jq '. += [{"releaseWorkflow":"core-e2e.yml","headSha":"'"$sha"'","status":"completed","conclusion":"failure","createdAt":"2026-01-01T00:01:00Z"}]' "$e2e" > "$tmp_dir/case-e2e.json"
expect_fail "$gate" "$sha" push "$contexts" "$checks" "$tmp_dir/case-e2e.json" "$pr_only"

# A successful check with a stale SHA is not acceptable.
jq --arg sha "$wrong_sha" '.check_runs |= map(if .name == "security" then .head_sha = $sha else . end)' "$checks" > "$tmp_dir/case.json"
expect_fail "$gate" "$sha" push "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"

echo "release-gate-check adversarial tests passed"
