#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bash-version.sh"
ensure_bash_interpreter 0 "$0" "$@" || exit 2
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/lib-core-readiness.sh"

LIMA_VM="${LIMA_VM:-landlock-genprof-core}"
EXPECTED_CONTEXT="kind-${LIMA_VM}"
UI_PORT="${UI_PORT:-8080}"
UI_NAMESPACE="${UI_NAMESPACE:-default}"
UI_HOST=127.0.0.1
UI_URL="http://${UI_HOST}:${UI_PORT}"
UI_PID=""
UI_MODE="${UI_MODE:-local}"

[ "$UI_MODE" = local ] || {
  echo "ERROR: UI_MODE=${UI_MODE} is not supported by the local launcher; use the documented production-like trusted-proxy procedure" >&2
  exit 1
}

die() {
  echo "ERROR: $*" >&2
  exit 1
}

http_status() {
  curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "$1" 2>/dev/null || true
}

stop_ui() {
  if [ -n "$UI_PID" ] && kill -0 "$UI_PID" 2>/dev/null; then
    echo
    echo "UI_STOPPING"
    kill "$UI_PID" 2>/dev/null || true
    wait "$UI_PID" 2>/dev/null || true
  fi
  lib_core_readiness_cleanup
}

cleanup() {
  local status=$?
  stop_ui
  exit "$status"
}
handle_signal() {
  trap - INT TERM
  stop_ui
  exit 0
}
trap cleanup EXIT INT TERM
trap handle_signal INT TERM

case "$UI_PORT" in
  ''|*[!0-9]*) die "UI_PORT must be a numeric TCP port (default: 8080)" ;;
esac
[ "$UI_PORT" -ge 1 ] && [ "$UI_PORT" -le 65535 ] || die "UI_PORT must be between 1 and 65535"

lib_core_readiness_require_commands curl docker go kubectl limactl make
lib_core_readiness_check

if command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:"$UI_PORT" -sTCP:LISTEN 2>/dev/null | tail -n +2 | grep -q .; then
  die "port ${UI_PORT} is already in use; retry with UI_PORT=8081 make ui-lima"
fi

echo "UI_MODE=LOCAL_DEVELOPMENT"
echo "AUTH_MODE=LOCAL_DEVELOPMENT_NO_TRUSTED_PROXY"
echo "UI_STARTING address=${UI_HOST}:${UI_PORT} namespace=${UI_NAMESPACE}"
(
  cd "$ROOT_DIR"
  exec go run ./cmd/landlock-genprof ui --namespace "$UI_NAMESPACE" --port "$UI_PORT"
) &
UI_PID=$!

ready=0
for _ in $(seq 1 60); do
  if ! kill -0 "$UI_PID" 2>/dev/null; then
    wait "$UI_PID" || true
    die "UI process exited before becoming ready"
  fi
  root_body="$(curl -fsS --max-time 2 "$UI_URL/" 2>/dev/null || true)"
  root_status="$(http_status "$UI_URL/")"
  favicon_status="$(http_status "$UI_URL/landlock-favicon.png")"
  workloads_status="$(http_status "$UI_URL/api/workloads")"
  missing_asset_status="$(http_status "$UI_URL/assets/missing-ui-lima.js")"
  if [ "$root_status" = 200 ] &&
     [[ "$root_body" == *"Operations Center"* ]] &&
     [ "$favicon_status" = 200 ] &&
     [ "$workloads_status" = 200 ] &&
     [ "$missing_asset_status" = 404 ]; then
    ready=1
    break
  fi
  sleep 1
done
[ "$ready" -eq 1 ] || die "UI did not pass HTTP smoke checks within 60 seconds"

echo "UI_READY"
echo "OPERATIONS_CENTER_STATUS=LOCAL_DEVELOPMENT_REACT"
echo "AUTHENTICATED_PRODUCTION_UI=make ui-lima-auth"
echo
echo "Open in your browser:"
echo
echo "$UI_URL"
echo
echo "Press Ctrl-C to stop."
wait "$UI_PID"
