#!/usr/bin/env bash
set -Eeuo pipefail

# Interactive source-mode demo using the same authenticated launcher and
# Linux-in-Lima executor as the automated qualification harness.
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
UI_NAMESPACE="${UI_NAMESPACE:-ui-lima-demo-$$}"
WORKLOAD_NAME="${WORKLOAD_NAME:-landlock-genprof-demo-workload}"
UI_PID=""
NAMESPACE_CREATED=0
WORK_DIR=""

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [ -n "$UI_PID" ] && kill -0 "$UI_PID" 2>/dev/null; then
    kill -TERM "$UI_PID" 2>/dev/null || true
    wait "$UI_PID" 2>/dev/null || true
  fi
  if [ "$NAMESPACE_CREATED" -eq 1 ]; then
    kubectl delete namespace "$UI_NAMESPACE" --ignore-not-found --wait=true >/dev/null 2>&1 || true
  fi
  if [ -n "$WORK_DIR" ]; then rm -rf "$WORK_DIR"; fi
  lib_core_readiness_cleanup
  echo "DEMO_CLEANUP_DONE namespace=${UI_NAMESPACE}"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

lib_core_readiness_require_commands curl docker go kubectl limactl make npm node
lib_core_readiness_check

# Gadget's capability tracer creates one inotify instance per attached source.
# The preserved Core topology is known to require 256 instances; establish
# that VM-local qualification capacity before starting the executor so a VM
# restart cannot silently regress the self-contained demo.
if ! limactl shell "$LIMA_VM" sudo sysctl -w fs.inotify.max_user_instances=256 >/dev/null; then
  die "could not establish Core VM inotify capacity"
fi
INOTIFY_MAX_USER_INSTANCES="$(limactl shell "$LIMA_VM" sysctl -n fs.inotify.max_user_instances)"
[ "$INOTIFY_MAX_USER_INSTANCES" -ge 256 ] || die "Core VM inotify capacity is ${INOTIFY_MAX_USER_INSTANCES}, expected at least 256"
echo "INOTIFY_MAX_USER_INSTANCES=${INOTIFY_MAX_USER_INSTANCES}"

if [ ! -d "$ROOT_DIR/test/ui/node_modules/playwright" ]; then
  echo "UI_PLAYWRIGHT_INSTALLING"
  npm install --prefix "$ROOT_DIR/test/ui" --ignore-scripts --no-audit --no-fund >/dev/null
fi

kubectl create namespace "$UI_NAMESPACE" >/dev/null
NAMESPACE_CREATED=1
kubectl -n "$UI_NAMESPACE" create deployment "$WORKLOAD_NAME" --image=nginx:1.27 -- /bin/sh -c 'while :; do sleep 3600; done' >/dev/null
kubectl -n "$UI_NAMESPACE" rollout status deployment/"$WORKLOAD_NAME" --timeout=180s >/dev/null
POD_NAME="$(kubectl -n "$UI_NAMESPACE" get pod -l app="$WORKLOAD_NAME" -o jsonpath='{.items[0].metadata.name}')"
CONTAINER_NAME="$(kubectl -n "$UI_NAMESPACE" get pod "$POD_NAME" -o jsonpath='{.spec.containers[0].name}')"

UI_NAMESPACE="$UI_NAMESPACE" BACKEND_PORT=18081 PROXY_PORT=8090 \
  "$ROOT_DIR/hack/ui-lima-auth.sh" &
UI_PID=$!
for _ in $(seq 1 90); do
  if ! kill -0 "$UI_PID" 2>/dev/null; then
    wait "$UI_PID" || true
    exit 1
  fi
  if [ "$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' http://127.0.0.1:8090/ 2>/dev/null || true)" = 200 ]; then break; fi
  sleep 1
done
if [ "$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:8090/ 2>/dev/null || true)" != 200 ]; then
  echo "ERROR: trusted proxy did not become ready on http://127.0.0.1:8090" >&2
  exit 1
fi

# Reuse the same real browser value-flow driver as automated qualification.
# It creates capability evidence, completes the Observation, generates the
# Proposal, verifies History/Attention, and exercises governance before this
# interactive process announces readiness. The namespace remains owned by this
# process after the driver exits so the operator can inspect it manually.
WORK_DIR="$(mktemp -d -t landlock-genprof-ui-demo.XXXXXX)"
chmod 700 "$WORK_DIR"
if ! UI_URL="http://127.0.0.1:8090" UI_EXPECTED_WORKLOAD="$WORKLOAD_NAME" \
  UI_NAMESPACE="$UI_NAMESPACE" UI_POD="$POD_NAME" UI_CONTAINER="$CONTAINER_NAME" \
  NODE_PATH="$ROOT_DIR/test/ui/node_modules" node "$ROOT_DIR/test/ui/workbench-smoke.js" >"$WORK_DIR/value-flow.log" 2>&1; then
  cat "$WORK_DIR/value-flow.log" >&2
  if grep -q '"marker":"OBSERVATION_COMPLETED"' "$WORK_DIR/value-flow.log"; then
    die "real demo browser qualification failed after Observation completion (readiness, UI, or API boundary)"
  fi
  die "real demo value-flow qualification failed before Observation completion"
fi
cat "$WORK_DIR/value-flow.log"
VALUE_FLOW_STATE="$(tail -n 1 "$WORK_DIR/value-flow.log")"
DEMO_OBSERVATION="$(printf '%s\n' "$VALUE_FLOW_STATE" | sed -n 's/.*"observationID":"\([^"]*\)".*/\1/p')"
DEMO_PROPOSAL="$(printf '%s\n' "$VALUE_FLOW_STATE" | sed -n 's/.*"proposalName":"\([^"]*\)".*/\1/p')"
if [ -z "$DEMO_OBSERVATION" ] || [ -z "$DEMO_PROPOSAL" ]; then
  die "value-flow driver did not return authoritative Observation/Proposal identifiers"
fi

echo
echo "OPERATIONS_CENTER_DEMO_READY"
echo "URL=http://127.0.0.1:8090"
echo "NAMESPACE=${UI_NAMESPACE}"
echo "WORKLOAD=${WORKLOAD_NAME}"
echo "POD=${POD_NAME}"
echo "CONTAINER=${CONTAINER_NAME}"
echo "OBSERVATION=${DEMO_OBSERVATION}"
echo "PROPOSAL=${DEMO_PROPOSAL}"
echo "The real value flow is prepopulated; inspect Overview, Workloads, Observations, Proposals, History, and Attention."
echo "Press Ctrl-C to stop and clean up only this demo namespace and its processes."
echo

wait "$UI_PID"
