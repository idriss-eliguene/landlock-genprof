#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
INSTALLER="$ROOT_DIR/test/e2e/install-gadget.sh"

grep -q 'GADGET_STARTUP_FAILURE_THRESHOLD=36' "$INSTALLER"
grep -q 'GADGET_STARTUP_PERIOD_SECONDS=5' "$INSTALLER"
grep -q 'GADGET_STARTUP_TIMEOUT_SECONDS=5' "$INSTALLER"
grep -q 'GADGET_HEALTH_TIMEOUT_SECONDS=15' "$INSTALLER"
grep -q 'startupProbe:' "$INSTALLER"
grep -q 'readinessProbe:' "$INSTALLER"
grep -q 'livenessProbe:' "$INSTALLER"

if [ "${VERIFY_LIVE:-0}" = 1 ]; then
  startup="$(kubectl -n gadget get daemonset gadget -o jsonpath='{.spec.template.spec.containers[?(@.name=="gadget")].startupProbe.failureThreshold}')"
  period="$(kubectl -n gadget get daemonset gadget -o jsonpath='{.spec.template.spec.containers[?(@.name=="gadget")].startupProbe.periodSeconds}')"
  startup_timeout="$(kubectl -n gadget get daemonset gadget -o jsonpath='{.spec.template.spec.containers[?(@.name=="gadget")].startupProbe.timeoutSeconds}')"
  health_timeout="$(kubectl -n gadget get daemonset gadget -o jsonpath='{.spec.template.spec.containers[?(@.name=="gadget")].readinessProbe.timeoutSeconds}')"
  [ "$startup" = 36 ]
  [ "$period" = 5 ]
  [ "$startup_timeout" = 5 ]
  [ "$health_timeout" = 15 ]
fi

echo "GADGET_PROBE_BUDGET_TEST=PASS"
