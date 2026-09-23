#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/hack/operations-center-demo.sh"

bash -n "$SCRIPT"

required_patterns=(
  "react_url=\"\${proxy_url}/next/\""
  "legacy_url=\"\${proxy_url}/\""
  "startup_health_url=\"\${proxy_url}/healthz/startup\""
  "live_health_url=\"\${proxy_url}/healthz/live\""
  "ready_health_url=\"\${proxy_url}/healthz/ready\""
  "for health_url in \"\$startup_health_url\" \"\$live_health_url\" \"\$ready_health_url\" \"\$api_health_url\"; do"
  'OPERATIONS_CENTER_REACT_URL='
  'OPERATIONS_CENTER_LEGACY_URL='
  'OPERATIONS_CENTER_PROXY_URL='
  'OPERATIONS_CENTER_BACKEND_URL='
  'OPERATIONS_CENTER_STARTUP_HEALTH_URL='
  'OPERATIONS_CENTER_LIVE_HEALTH_URL='
  'OPERATIONS_CENTER_READY_HEALTH_URL='
  'OPERATIONS_CENTER_METRICS_URL=DISABLED'
  'EXECUTOR_METRICS_URL=DISABLED'
  'Recommended UI: NEW Operations Center (React)'
)

for pattern in "${required_patterns[@]}"; do
  grep -Fq "$pattern" "$SCRIPT" || {
    echo "missing Operations Center demo startup contract: $pattern" >&2
    exit 1
  }
done

if grep -Fq 'echo "URL=http://' "$SCRIPT"; then
  echo "legacy ambiguous URL banner remains in Operations Center demo" >&2
  exit 1
fi

echo "OPERATIONS_CENTER_DEMO_STARTUP_CONTRACT=PASS"
