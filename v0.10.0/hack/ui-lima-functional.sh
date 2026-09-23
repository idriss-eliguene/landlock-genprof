#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/hack/versions.env"
source "$ROOT_DIR/hack/bash-version.sh"
ensure_bash_interpreter 0 "$0" "$@" || exit 2
source "$ROOT_DIR/hack/lib-core-readiness.sh"

die() {
  echo "ERROR: $*" >&2
  exit 1
}

LIMA_VM="${LIMA_VM:-landlock-genprof-core}"
EXPECTED_CONTEXT="kind-${LIMA_VM}"
export EXPECTED_CONTEXT
UI_NAMESPACE="${UI_NAMESPACE:-ui-functional-qualification-$$}"
BACKEND_PORT="${BACKEND_PORT:-}"
PROXY_PORT="${PROXY_PORT:-}"
if [[ -z "$BACKEND_PORT" && -z "$PROXY_PORT" ]]; then
  # The functional harness is routinely run beside other local demos and
  # Lima port-forwards. Reserve both sockets together instead of assuming
  # that a historical fixed port is still free.
  read -r BACKEND_PORT PROXY_PORT < <(python3 - <<'PY'
import socket

sockets = []
try:
    for _ in range(2):
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        sockets.append(sock)
    print(sockets[0].getsockname()[1], sockets[1].getsockname()[1])
finally:
    for sock in sockets:
        sock.close()
PY
  )
elif [[ -z "$BACKEND_PORT" ]]; then
  BACKEND_PORT="$(python3 - "$PROXY_PORT" <<'PY'
import socket, sys
avoid = int(sys.argv[1])
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
try:
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    if port == avoid:
        raise SystemExit("selected port equals explicitly requested proxy port")
    print(port)
finally:
    sock.close()
PY
  )"
elif [[ -z "$PROXY_PORT" ]]; then
  PROXY_PORT="$(python3 - "$BACKEND_PORT" <<'PY'
import socket, sys
avoid = int(sys.argv[1])
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
try:
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    if port == avoid:
        raise SystemExit("selected port equals explicitly requested backend port")
    print(port)
finally:
    sock.close()
PY
  )"
fi
WORKLOAD_NAME="${WORKLOAD_NAME:-ui-functional-workload}"
UI_URL="http://127.0.0.1:${PROXY_PORT}"
WORK_DIR="$(mktemp -d -t landlock-genprof-ui-functional.XXXXXX)"
UI_PID=""

cleanup() {
  local status=$?
  if [ -n "$UI_PID" ] && kill -0 "$UI_PID" 2>/dev/null; then kill "$UI_PID" 2>/dev/null || true; wait "$UI_PID" 2>/dev/null || true; fi
  kubectl delete namespace "$UI_NAMESPACE" --ignore-not-found --wait=true >/dev/null 2>&1 || true
  rm -rf "$WORK_DIR"
  lib_core_readiness_cleanup
  exit "$status"
}
trap cleanup EXIT INT TERM

lib_core_readiness_require_commands curl docker go kubectl limactl make npm npx python3
lib_core_readiness_check

kubectl create namespace "$UI_NAMESPACE" >/dev/null 2>&1 || true
kubectl -n "$UI_NAMESPACE" create deployment "$WORKLOAD_NAME" --image=nginx:1.27 >/dev/null
kubectl -n "$UI_NAMESPACE" rollout status deployment/"$WORKLOAD_NAME" --timeout=180s >/dev/null

echo "REAL_WORKLOAD_CREATED name=${WORKLOAD_NAME} namespace=${UI_NAMESPACE}"
echo "REAL_POD_READY=$(kubectl -n "$UI_NAMESPACE" get pod -l app="$WORKLOAD_NAME" -o jsonpath='{.items[0].metadata.name}')"
echo "REAL_CONTAINER_IMAGE=$(kubectl -n "$UI_NAMESPACE" get pod -l app="$WORKLOAD_NAME" -o jsonpath='{.items[0].status.containerStatuses[0].imageID}')"

UI_NAMESPACE="$UI_NAMESPACE" BACKEND_PORT="$BACKEND_PORT" PROXY_PORT="$PROXY_PORT" \
  "$ROOT_DIR/hack/ui-lima-auth.sh" >"$WORK_DIR/ui.log" 2>&1 &
UI_PID=$!
for _ in $(seq 1 90); do
  if ! kill -0 "$UI_PID" 2>/dev/null; then
    cat "$WORK_DIR/ui.log" >&2
    exit 1
  fi
  if curl -fsS --max-time 2 "$UI_URL/" >/dev/null 2>&1; then break; fi
  sleep 1
done
curl -fsS --max-time 5 "$UI_URL/" >/dev/null || { cat "$WORK_DIR/ui.log" >&2; exit 1; }

if [ ! -d "$ROOT_DIR/test/ui/node_modules/playwright" ]; then
  npm install --prefix "$ROOT_DIR/test/ui" --ignore-scripts --no-audit --no-fund >/dev/null
fi
if ! UI_URL="$UI_URL" UI_EXPECTED_WORKLOAD="$WORKLOAD_NAME" \
  UI_NAMESPACE="$UI_NAMESPACE" UI_POD="$(kubectl -n "$UI_NAMESPACE" get pod -l app="$WORKLOAD_NAME" -o jsonpath='{.items[0].metadata.name}')" UI_CONTAINER=nginx \
  NODE_PATH="$ROOT_DIR/test/ui/node_modules" node "$ROOT_DIR/test/ui/workbench-smoke.js"; then
  echo "HARNESS_FAILURE_OBSERVATIONS namespace=${UI_NAMESPACE}" >&2
  kubectl -n "$UI_NAMESPACE" get observations -o yaml >&2 || true
  cat "$WORK_DIR/ui.log" >&2
  exit 1
fi
echo "SOURCE_UI_FUNCTIONAL_RESULT=PASS"
