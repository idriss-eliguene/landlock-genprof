#!/usr/bin/env bash
set -u

log_dir="$(mktemp -d)"
trap 'rm -rf "$log_dir"' EXIT

failures=0

classify_failure() {
	local log="$1"
	if grep -Eqi 'setup failed|failed to start|connection refused|KUBEBUILDER_ASSETS|no such file|dial tcp|context deadline exceeded|lookup .*proxy' "$log"; then
		printf 'INFRASTRUCTURE_FAILURE\n'
		return 0
	fi
	printf 'UNEXPECTED_FAILURE\n'
}

expected_failure() {
	local name="$1"
	local pattern="$2"
	shift 2
	local log="$log_dir/$name.log"
	local rc

	set +e
	"$@" >"$log" 2>&1
	rc=$?
	set -e

	if [ "$rc" -eq 0 ]; then
		printf '%s: UNEXPECTED_PASS\n' "$name"
		failures=$((failures + 1))
	elif grep -Eq "$pattern" "$log"; then
		printf '%s: EXPECTED_DIAGNOSTIC_FAILURE\n' "$name"
	else
		printf '%s: ' "$name"
		classify_failure "$log"
		failures=$((failures + 1))
	fi
}

expected_pass() {
	local name="$1"
	shift
	local log="$log_dir/$name.log"
	local rc

	set +e
	"$@" >"$log" 2>&1
	rc=$?
	set -e

	if [ "$rc" -eq 0 ]; then
		printf '%s: EXPECTED_PASS\n' "$name"
	else
		printf '%s: ' "$name"
		classify_failure "$log"
		failures=$((failures + 1))
	fi
}

if [ -n "${KUBEBUILDER_ASSETS:-}" ]; then
	assets="$KUBEBUILDER_ASSETS"
	setup_rc=0
else
	set +e
	assets="$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.24 use -p path 1.36.2 2>"$log_dir/setup-envtest.log")"
	setup_rc=$?
	set -e
fi
if [ "$setup_rc" -ne 0 ] || [ -z "$assets" ]; then
	echo 'setup-envtest: INFRASTRUCTURE_FAILURE'
	cat "$log_dir/setup-envtest.log"
	exit 1
fi

expected_failure \
	observation-contribution-e7 \
	'store_envtest_test\.go:310: E7 legacy =' \
	env KUBEBUILDER_ASSETS="$assets" go test -tags=envtest -count=1 \
	-run '^TestObservationContributionEnvtestE1ToE7$' ./internal/history/...

expected_pass \
	receipt-concurrency \
	go test -race ./internal/history -count=10 \
	-run '^TestReceiptConcurrencySameKeyConvergesOnOneEffect$'

expected_failure \
	observation-adapter-distinct \
	'observation_contribution_test\.go:390: concurrent accumulation =' \
	go test -race ./internal/history -count=10 \
	-run '^TestObservationAdapterConcurrentDifferentObservationsAccumulate$'

if [ "$failures" -ne 0 ]; then
	echo "diagnostics: $failures contract violation(s)"
	exit 1
fi

echo 'diagnostics: all accepted contracts matched'
