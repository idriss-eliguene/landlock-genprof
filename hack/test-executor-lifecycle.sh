#!/usr/bin/env bash
set -Eeuo pipefail

# Observable Lima regression for the deleted-executable leak. This test uses
# disposable sleep processes with the same guest-process ownership topology as
# the real executor; it does not touch Kubernetes or production state.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/lib-executor-lifecycle.sh"

LIMA_VM="${LIMA_VM:-landlock-genprof-core}"
EXECUTABLE="/tmp/landlock-genprof-lifecycle-regression-${$}"
HOST_PID=""

cleanup() {
  terminate_remote_executor "$LIMA_VM" "$EXECUTABLE" "$HOST_PID" || true
  limactl shell "$LIMA_VM" -- rm -f "$EXECUTABLE" >/dev/null 2>&1 || true
}
trap cleanup EXIT

start_executor() {
  local stubborn=$1
  if [ "$stubborn" = 1 ]; then
    # shellcheck disable=SC2016
    limactl shell "$LIMA_VM" -- bash -c \
      'cp /bin/sleep "$1"; trap "" TERM; exec -a "$1" sleep 300' bash "$EXECUTABLE" &
  else
    # shellcheck disable=SC2016
    limactl shell "$LIMA_VM" -- bash -c \
      'cp /bin/sleep "$1"; exec -a "$1" sleep 300' bash "$EXECUTABLE" &
  fi
  HOST_PID=$!
  for _ in $(seq 1 50); do
    if remote_executor_state "$LIMA_VM" "$EXECUTABLE"; then return 0; fi
    sleep 0.1
  done
  echo "executor did not become observable" >&2
  return 1
}

start_executor 0
terminate_remote_executor "$LIMA_VM" "$EXECUTABLE" "$HOST_PID"
HOST_PID=""
echo "NORMAL_EXECUTOR_CLEANUP=PASS"

start_executor 1
terminate_remote_executor "$LIMA_VM" "$EXECUTABLE" "$HOST_PID"
HOST_PID=""
echo "SIGKILL_FALLBACK_CLEANUP=PASS"
