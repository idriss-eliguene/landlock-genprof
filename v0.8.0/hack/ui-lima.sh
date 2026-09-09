#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bash-version.sh"
ensure_bash_interpreter 0 "$0" "$@" || exit 2

LIMA_VM="${LIMA_VM:-landlock-genprof-core}"
EXPECTED_CONTEXT="kind-${LIMA_VM}"
UI_PORT="${UI_PORT:-8080}"
UI_NAMESPACE="${UI_NAMESPACE:-default}"
UI_HOST=127.0.0.1
UI_URL="http://${UI_HOST}:${UI_PORT}"
UI_PID=""
UI_MODE="${UI_MODE:-local}"
ENV_DOCTOR_OUTPUT=""

[ "$UI_MODE" = local ] || {
  echo "ERROR: UI_MODE=${UI_MODE} is not supported by the local launcher; use the documented production-like trusted-proxy procedure" >&2
  exit 1
}

die() {
  echo "ERROR: $*" >&2
  exit 1
}

stop_ui() {
  if [ -n "$UI_PID" ] && kill -0 "$UI_PID" 2>/dev/null; then
    echo
    echo "UI_STOPPING"
    kill "$UI_PID" 2>/dev/null || true
    wait "$UI_PID" 2>/dev/null || true
  fi
  if [ -n "$ENV_DOCTOR_OUTPUT" ]; then
    rm -f "$ENV_DOCTOR_OUTPUT"
  fi
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

for command_name in curl docker go kubectl limactl make; do
  command -v "$command_name" >/dev/null 2>&1 || die "$command_name is required"
done

vm_record="$(limactl list --format '{{.Name}} {{.Status}} {{.Arch}}' | awk -v vm="$LIMA_VM" '$1 == vm { print; exit }')"
[ -n "$vm_record" ] || die "Lima VM '${LIMA_VM}' was not found"
vm_status="$(printf '%s\n' "$vm_record" | awk '{print $2}')"
vm_arch="$(printf '%s\n' "$vm_record" | awk '{print $3}')"
if [ "$vm_status" = Stopped ]; then
  echo "LIMA_STARTING vm=${LIMA_VM}"
  limactl start "$LIMA_VM" >/dev/null
  vm_record="$(limactl list --format '{{.Name}} {{.Status}} {{.Arch}}' | awk -v vm="$LIMA_VM" '$1 == vm { print; exit }')"
  vm_status="$(printf '%s\n' "$vm_record" | awk '{print $2}')"
fi
[ "$vm_status" = Running ] || die "Lima VM '${LIMA_VM}' is ${vm_status}; refusing to recreate the canonical VM"
case "$vm_arch" in
  aarch64|arm64) ;;
  *) die "Lima VM '${LIMA_VM}' has unsupported architecture '${vm_arch}'" ;;
esac
echo "LIMA_READY vm=${LIMA_VM} arch=${vm_arch}"

docker_context="$(docker context show 2>/dev/null || true)"
[ "$docker_context" = "lima-${LIMA_VM}" ] || die "Docker context is '${docker_context}', expected 'lima-${LIMA_VM}'; refusing to switch daemons"
docker info >/dev/null 2>&1 || die "Docker is not reachable through context '${docker_context}'"
echo "DOCKER_READY context=${docker_context}"

current_context="$(kubectl config current-context 2>/dev/null || true)"
[ "$current_context" = "$EXPECTED_CONTEXT" ] || die "Kubernetes context is '${current_context}', expected '${EXPECTED_CONTEXT}'"
kubectl cluster-info >/dev/null 2>&1 || die "Kubernetes API is not reachable through context '${EXPECTED_CONTEXT}'"
kubectl get --raw=/version >/dev/null 2>&1 || die "Kubernetes API version endpoint is not readable"
echo "KUBERNETES_READY context=${current_context}"

env_doctor_output="$(mktemp -t landlock-genprof-ui-lima.XXXXXX)"
ENV_DOCTOR_OUTPUT="$env_doctor_output"
make -C "$ROOT_DIR" env-doctor | tee "$env_doctor_output"
grep -q '^LOCAL_ENVIRONMENT_READY=true$' "$env_doctor_output" || die "canonical environment is not ready; see make env-doctor output"
echo "ENVIRONMENT_READY"

required_crds=(
  traininghistories.landlockgenprof.io
  securityprofileproposals.landlockgenprof.io
  applyattempts.landlockgenprof.io
  rollbackattempts.landlockgenprof.io
  observations.landlockgenprof.io
  observationcontributionreceipts.landlockgenprof.io
)
for crd in "${required_crds[@]}"; do
  kubectl wait --for=condition=Established "crd/${crd}" --timeout=30s >/dev/null || die "CRD ${crd} is not Established"
done
kubectl wait --for=condition=Ready "node/${LIMA_VM}-control-plane" --timeout=30s >/dev/null || die "canonical Core node is not Ready"
kubectl -n kube-system rollout status daemonset/cilium --timeout=30s >/dev/null || die "Cilium is not Ready"
kubectl -n kube-system rollout status deployment/coredns --timeout=30s >/dev/null || die "CoreDNS is not Ready"
kubectl -n gadget rollout status daemonset/gadget --timeout=30s >/dev/null || die "Inspektor Gadget is not Ready"
echo "TOPOLOGY_READY node=${LIMA_VM}-control-plane cilium=${CILIUM_VERSION} gadget=${IG_VERSION}"

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
  if [ "$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "$UI_URL/")" = 200 ] &&
     [ "$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "$UI_URL/workbench.js")" = 200 ] &&
     [ "$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "$UI_URL/api/workloads")" = 200 ]; then
    ready=1
    break
  fi
  sleep 1
done
[ "$ready" -eq 1 ] || die "UI did not pass HTTP smoke checks within 60 seconds"

echo "UI_READY"
echo "OPERATIONS_CENTER_STATUS=LOCAL_READ_ONLY_WORKBENCH"
echo "AUTHENTICATED_PRODUCTION_UI=DOCUMENTED_SEPARATELY"
echo
echo "Open in your browser:"
echo
echo "$UI_URL"
echo
echo "Press Ctrl-C to stop."
wait "$UI_PID"
