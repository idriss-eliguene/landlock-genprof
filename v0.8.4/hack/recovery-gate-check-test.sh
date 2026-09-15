#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
gate="$root_dir/hack/recovery-gate-check.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

sha=a7e4c39041c239b37aca693973c32da147eb6b26
wrong_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
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

expect_pass() { "$@" >/dev/null; }
expect_fail() { if "$@" >/dev/null 2>&1; then echo "unexpected pass: $*" >&2; exit 1; fi; }
run_gate() { "$gate" "$1" "$2" "$3" "$4" "$contexts" "$checks" "$e2e" "$pr_only"; }

# Positive model and PR-only absence on recovery/tag events.
expect_pass run_gate v0.8.1 "$sha" "$sha" "$sha"

# Identity and source-boundary failures.
expect_fail run_gate v0.8.0 "$sha" "$sha" "$sha"
expect_fail run_gate v0.8.1 "$wrong_sha" "$sha" "$sha"
expect_fail run_gate v0.8.1 "$sha" "$wrong_sha" "$sha"
expect_fail run_gate v0.8.1 "$sha" "$sha" "$wrong_sha"

# Required source check countermodels.
for mode in missing failed cancelled skipped wrong-sha; do
  case "$mode" in
    missing) jq 'del(.check_runs[] | select(.name == "build-and-test"))' "$checks" > "$tmp_dir/case.json" ;;
    failed) jq '.check_runs |= map(if .name == "build-and-test" then .conclusion = "failure" else . end)' "$checks" > "$tmp_dir/case.json" ;;
    cancelled) jq '.check_runs |= map(if .name == "build-and-test" then .conclusion = "cancelled" else . end)' "$checks" > "$tmp_dir/case.json" ;;
    skipped) jq '.check_runs |= map(if .name == "build-and-test" then .conclusion = "skipped" else . end)' "$checks" > "$tmp_dir/case.json" ;;
    wrong-sha) jq --arg sha "$wrong_sha" '.check_runs |= map(if .name == "build-and-test" then .head_sha = $sha else . end)' "$checks" > "$tmp_dir/case.json" ;;
  esac
  expect_fail "$gate" v0.8.1 "$sha" "$sha" "$sha" "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"
done

# A newer failed check cannot be masked by an older success.
jq '.check_runs += [{"name":"build-and-test","head_sha":"'"$sha"'","status":"completed","conclusion":"failure","completed_at":"2026-01-01T00:01:00Z"}]' "$checks" > "$tmp_dir/case.json"
expect_fail "$gate" v0.8.1 "$sha" "$sha" "$sha" "$contexts" "$tmp_dir/case.json" "$e2e" "$pr_only"

# E2E countermodels.
for wf in spo-e2e.yml spo-dmin-e2e.yml; do
  jq --arg wf "$wf" 'del(.[] | select(.releaseWorkflow == $wf))' "$e2e" > "$tmp_dir/case-e2e.json"
  expect_fail "$gate" v0.8.1 "$sha" "$sha" "$sha" "$contexts" "$checks" "$tmp_dir/case-e2e.json" "$pr_only"
  jq --arg wf "$wf" '.[] |= if .releaseWorkflow == $wf then .conclusion = "failure" else . end' "$e2e" > "$tmp_dir/case-e2e.json"
  expect_fail "$gate" v0.8.1 "$sha" "$sha" "$sha" "$contexts" "$checks" "$tmp_dir/case-e2e.json" "$pr_only"
done
jq '.[] |= if .releaseWorkflow == "core-e2e.yml" then .headSha = "'"$wrong_sha"'" else . end' "$e2e" > "$tmp_dir/case-e2e.json"
expect_fail "$gate" v0.8.1 "$sha" "$sha" "$sha" "$contexts" "$checks" "$tmp_dir/case-e2e.json" "$pr_only"

# Failed PR governance remains rejected when evaluated as a PR, while its
# absence on a recovery/tag event remains non-blocking.
jq '.check_runs |= map(if .name == "lint-pr-title" then .conclusion = "failure" else . end)' "$checks" > "$tmp_dir/failed-pr.json"
expect_fail "$root_dir/hack/release-gate-check.sh" "$sha" pull_request "$contexts" "$tmp_dir/failed-pr.json" "$e2e" "$pr_only"

echo "recovery-gate-check adversarial tests passed"
