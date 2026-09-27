#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACTS="${OC_LIVE_ARTIFACTS:-$ROOT_DIR/artifacts/operations-center-live}"
NAMESPACE="${OC_LIVE_NAMESPACE:-operations-center-live}"
PRIVATE_NAMESPACE="${OC_PRIVATE_NAMESPACE:-operations-center-live-private}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
BACKEND_PORT=18080
OPERATOR_PROXY_PORT=18090
REVIEWER_PROXY_PORT=18091
APPROVER_PROXY_PORT=18092
DENIED_PROXY_PORT=18093
BACKEND_HOST="127.0.0.1:${BACKEND_PORT}"
WORK_DIR="$(mktemp -d -t landlock-genprof-oc-live.XXXXXX)"
BACKEND_PID=""
EXECUTOR_PID=""
PROXY_PIDS=()

mkdir -p "$ARTIFACTS"
chmod 700 "$WORK_DIR"
stage() { printf '%s\n' "$*" | tee -a "$ARTIFACTS/stages.txt"; }
die() { stage "FAIL:$*"; exit 1; }
cleanup() {
  local pid
  for pid in "${PROXY_PIDS[@]}"; do kill "$pid" 2>/dev/null || true; done
  [ -z "$BACKEND_PID" ] || kill "$BACKEND_PID" 2>/dev/null || true
  [ -z "$EXECUTOR_PID" ] || kill "$EXECUTOR_PID" 2>/dev/null || true
  for pid in "${PROXY_PIDS[@]}"; do wait "$pid" 2>/dev/null || true; done
  [ -z "$BACKEND_PID" ] || wait "$BACKEND_PID" 2>/dev/null || true
  [ -z "$EXECUTOR_PID" ] || wait "$EXECUTOR_PID" 2>/dev/null || true
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

command -v gcc >/dev/null || die "gcc unavailable for deterministic live workload"
kubectl get nodes >/dev/null
for crd in observations.landlockgenprof.io securityprofileproposals.landlockgenprof.io; do
  kubectl get crd "$crd" >/dev/null || die "required project CRD missing: $crd"
done
kubectl -n gadget get daemonset >/dev/null || die "Gadget namespace/DaemonSet missing"
kubectl -n security-profiles-operator get daemonset spod >/dev/null || die "SPO recorder missing"
stage "PASS:runner-is-real-node-and-live-dependencies-present"

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl create namespace "$PRIVATE_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

kubectl apply -f - >/dev/null <<'YAML'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: oc-live-cluster-identity}
rules:
- apiGroups: [""]
  resources: [namespaces]
  resourceNames: [kube-system]
  verbs: [get]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: oc-live-cluster-identity}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: oc-live-cluster-identity}
subjects:
- {kind: User, name: ci-operator, apiGroup: rbac.authorization.k8s.io}
- {kind: User, name: ci-reviewer, apiGroup: rbac.authorization.k8s.io}
- {kind: User, name: ci-approver, apiGroup: rbac.authorization.k8s.io}
- {kind: User, name: ci-denied, apiGroup: rbac.authorization.k8s.io}
YAML

kubectl -n "$NAMESPACE" apply -f - >/dev/null <<'YAML'
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: oc-live-reader}
rules:
- apiGroups: [""]
  resources: [pods]
  verbs: [get, list]
- apiGroups: [apps]
  resources: [deployments, replicasets]
  verbs: [get, list]
- apiGroups: [landlockgenprof.io]
  resources: [observations, observationcontributionreceipts, traininghistories, securityprofileproposals, applyattempts, rollbackattempts]
  verbs: [get, list]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: oc-live-operator}
rules:
- apiGroups: [landlockgenprof.io]
  resources: [observations]
  verbs: [get, list, create]
- apiGroups: [landlockgenprof.io]
  resources: [observations/status]
  verbs: [get, update, patch]
- apiGroups: [landlockgenprof.io]
  resources: [securityprofileproposals, traininghistories, observationcontributionreceipts]
  verbs: [get, list, create, update]
- apiGroups: [landlockgenprof.io]
  resources: [securityprofileproposals/status]
  verbs: [get, update, patch]
- apiGroups: [landlockgenprof.io]
  resources: [observationcontributionreceipts/status]
  verbs: [update]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: oc-live-governance}
rules:
- apiGroups: [landlockgenprof.io]
  resources: [securityprofileproposals]
  verbs: [get, list]
- apiGroups: [landlockgenprof.io]
  resources: [securityprofileproposals/status]
  verbs: [get, update, patch]
YAML

for actor in ci-operator ci-reviewer ci-approver; do
  kubectl -n "$NAMESPACE" create rolebinding "${actor}-reader" --role=oc-live-reader --user="$actor" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
done
kubectl -n "$NAMESPACE" create rolebinding ci-operator-observe --role=oc-live-operator --user=ci-operator --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n "$NAMESPACE" create rolebinding ci-reviewer-governance --role=oc-live-governance --user=ci-reviewer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n "$NAMESPACE" create rolebinding ci-approver-governance --role=oc-live-governance --user=ci-approver --dry-run=client -o yaml | kubectl apply -f - >/dev/null

image="localhost/landlock-genprof-oc-live:${GITHUB_SHA:-local}"
gcc -static -O2 -Wall -Wextra "$ROOT_DIR/test/e2e/operations-center-live-workload.c" -o "$WORK_DIR/live-workload"
docker build --provenance=false -t "$image" -f - "$WORK_DIR" <<'DOCKERFILE'
FROM scratch
COPY live-workload /live-workload
ENTRYPOINT ["/live-workload"]
DOCKERFILE
docker save "$image" | sudo k3s ctr images import - >/dev/null
kubectl -n "$NAMESPACE" create deployment live-probe --image="$image" --dry-run=client -o yaml > "$WORK_DIR/workload.yaml"
kubectl apply -f "$WORK_DIR/workload.yaml" >/dev/null
kubectl -n "$NAMESPACE" rollout status deployment/live-probe --timeout=180s
kubectl -n "$PRIVATE_NAMESPACE" create deployment private-probe --image="$image" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n "$PRIVATE_NAMESPACE" rollout status deployment/private-probe --timeout=180s
stage "PASS:deterministic-workloads-ready"

go build -o "$WORK_DIR/landlock-genprof" ./cmd/landlock-genprof
go build -o "$WORK_DIR/trustedproxy" ./hack/trustedproxy
head -c 32 /dev/urandom | base64 | tr -d '\n' > "$WORK_DIR/hmac"
chmod 600 "$WORK_DIR/hmac"
export KUBECONFIG
export LANDLOCK_GENPROF_DEPLOYMENT_MODE=production
LANDLOCK_GENPROF_TRUSTED_PROXY_HMAC_SECRET="$(<"$WORK_DIR/hmac")"
export LANDLOCK_GENPROF_TRUSTED_PROXY_HMAC_SECRET
export LANDLOCK_GENPROF_ALLOWED_USERS=ci-operator,ci-reviewer,ci-approver,ci-denied
export LANDLOCK_GENPROF_REVIEW_GROUPS=proposal-reviewers
export LANDLOCK_GENPROF_APPROVER_GROUPS=proposal-approvers
export LANDLOCK_GENPROF_PROFILE_REALIZER_KUBECONFIG="$KUBECONFIG"
export LANDLOCK_GENPROF_OBSERVATION_EXECUTOR_KUBECONFIG="$KUBECONFIG"
export LANDLOCK_GENPROF_ALLOWED_HOST="$BACKEND_HOST"
"$WORK_DIR/landlock-genprof" ui --namespace "$NAMESPACE" --port "$BACKEND_PORT" > "$ARTIFACTS/backend.log" 2>&1 &
BACKEND_PID=$!
"$WORK_DIR/landlock-genprof" executor --namespace "$NAMESPACE" > "$ARTIFACTS/executor.log" 2>&1 &
EXECUTOR_PID=$!

start_proxy() {
  local name="$1" port="$2" groups="$3"
  "$WORK_DIR/trustedproxy" -listen "127.0.0.1:${port}" -backend "http://${BACKEND_HOST}" -backend-host "$BACKEND_HOST" -secret-file "$WORK_DIR/hmac" -user "$name" -groups "$groups" > "$ARTIFACTS/proxy-${name}.log" 2>&1 &
  PROXY_PIDS+=("$!")
}
start_proxy ci-operator "$OPERATOR_PROXY_PORT" workbench-operators
start_proxy ci-reviewer "$REVIEWER_PROXY_PORT" proposal-reviewers
start_proxy ci-approver "$APPROVER_PROXY_PORT" proposal-approvers
start_proxy ci-denied "$DENIED_PROXY_PORT" ""

ready=0
for _ in $(seq 1 90); do
  if ! kill -0 "$BACKEND_PID" 2>/dev/null || ! kill -0 "$EXECUTOR_PID" 2>/dev/null; then
    tail -100 "$ARTIFACTS/backend.log" "$ARTIFACTS/executor.log" >&2 || true
    die "Operations Center backend or Observation executor exited during startup"
  fi
  code="$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${OPERATOR_PROXY_PORT}/" 2>/dev/null || true)"
  if [ "$code" = 200 ]; then ready=1; break; fi
  sleep 1
done
[ "$ready" -eq 1 ] || die "authenticated Operations Center did not become ready"
stage "PASS:authenticated-backend-and-trusted-proxy-ready"

export OC_LIVE_OPERATOR_URL="http://127.0.0.1:${OPERATOR_PROXY_PORT}/"
export OC_LIVE_REVIEWER_URL="http://127.0.0.1:${REVIEWER_PROXY_PORT}/"
export OC_LIVE_APPROVER_URL="http://127.0.0.1:${APPROVER_PROXY_PORT}/"
export OC_LIVE_DENIED_URL="http://127.0.0.1:${DENIED_PROXY_PORT}/"
export OC_LIVE_NAMESPACE="$NAMESPACE"
export OC_LIVE_PRIVATE_NAMESPACE="$PRIVATE_NAMESPACE"
OC_LIVE_CONTEXT_NAME="$(kubectl config current-context)"
export OC_LIVE_CONTEXT_NAME
export OC_LIVE_ARTIFACTS="$ARTIFACTS"
export OC_LIVE_EXPECTED_WORKLOAD=live-probe
export NODE_PATH="$ROOT_DIR/test/ui/node_modules"
node "$ROOT_DIR/test/ui/operations-center-live-journey.js" | tee "$ARTIFACTS/browser-results.jsonl"

stage "NOT_RUN:governed-Seccomp-apply-and-twin-control-verification"
stage "LIMITATION:candidate-v2-live-observation-derives-container-capabilities-only; Seccomp derivation is NOT_AVAILABLE, so no approved SPO SeccompProfile can be applied from this proposal"

go test ./internal/seccompverification -run '^TestRunControlFailureIsUnknownAndCleanupFailureInvalidatesResult$' -count=1 | tee "$ARTIFACTS/control-failure-regression.txt"
stage "PASS:control-failure-remains-UNKNOWN-unit-regression"
