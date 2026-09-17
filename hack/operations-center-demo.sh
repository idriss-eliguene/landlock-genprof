#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

# Canonical local Operations Center V2 demo. Generated kubeconfig/token state
# is private temporary state and is never printed or served to the browser.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bash-version.sh"
ensure_bash_interpreter 0 "$0" "$@" || exit 2
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/lib-core-readiness.sh"

LIMA_VM="${LIMA_VM:-landlock-genprof-core}"
CLUSTER_NAME="${LANDLOCK_CORE_CLUSTER:-$LIMA_VM}"
EXPECTED_CONTEXT="kind-${CLUSTER_NAME}"
export EXPECTED_CONTEXT
DEMO_KUBECONFIG="${LANDLOCK_GENPROF_DEMO_KUBECONFIG:-$(mktemp -t landlock-genprof-operations-center-demo.XXXXXX)}"
DEMO_BACKEND_PORT="${DEMO_BACKEND_PORT:-18080}"
DEMO_PROXY_PORT="${DEMO_PROXY_PORT:-18083}"
DEMO_PROXY_USER="${DEMO_PROXY_USER:-security-reviewer}"
UI_PID=""
PROXY_PID=""
EXECUTOR_PID=""
WORK_DIR=""
GUEST_EXECUTOR_BIN=""
GUEST_EXECUTOR_KUBECONFIG=""

die() { echo "ERROR: $*" >&2; exit 1; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [ -n "$UI_PID" ] && kill -0 "$UI_PID" 2>/dev/null; then
    kill -TERM "$UI_PID" 2>/dev/null || true
    wait "$UI_PID" 2>/dev/null || true
  fi
  if [ -n "$PROXY_PID" ] && kill -0 "$PROXY_PID" 2>/dev/null; then
    kill -TERM "$PROXY_PID" 2>/dev/null || true
    wait "$PROXY_PID" 2>/dev/null || true
  fi
  if [ -n "$EXECUTOR_PID" ] && kill -0 "$EXECUTOR_PID" 2>/dev/null; then
    kill -TERM "$EXECUTOR_PID" 2>/dev/null || true
    wait "$EXECUTOR_PID" 2>/dev/null || true
  fi
  if [ -n "$GUEST_EXECUTOR_BIN" ]; then
    limactl shell "$LIMA_VM" -- pkill -TERM -f "$GUEST_EXECUTOR_BIN" 2>/dev/null || true
    limactl shell "$LIMA_VM" -- rm -f "$GUEST_EXECUTOR_BIN" 2>/dev/null || true
  fi
  if [ -n "$GUEST_EXECUTOR_KUBECONFIG" ]; then limactl shell "$LIMA_VM" -- rm -f "$GUEST_EXECUTOR_KUBECONFIG" 2>/dev/null || true; fi
  rm -rf "$WORK_DIR"
  rm -f "$DEMO_KUBECONFIG"
  lib_core_readiness_cleanup
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

lib_core_readiness_require_commands curl docker go kubectl limactl make

# The public entry point owns the complete disposable setup. These repository
# targets are idempotent and are deliberately run before readiness inspection
# so a clean supported machine needs no remembered bootstrap sequence.
make bootstrap
make test-env
lib_core_readiness_check
case "$DEMO_BACKEND_PORT" in ''|*[!0-9]*) die "DEMO_BACKEND_PORT must be numeric" ;; esac

kubectl apply -f - >/dev/null <<'EOF'
apiVersion: v1
kind: Namespace
metadata: {name: payments, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
---
apiVersion: v1
kind: Namespace
metadata: {name: development, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
---
apiVersion: v1
kind: Namespace
metadata: {name: platform, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
---
apiVersion: v1
kind: Namespace
metadata: {name: security, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: frontend, namespace: payments, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
spec:
  replicas: 1
  selector: {matchLabels: {app: frontend}}
  template:
    metadata: {labels: {app: frontend}}
    spec:
      containers:
      - name: frontend
        image: nginx:1.27
        command: [/bin/sh, -c]
        args: ["while :; do cat /etc/hostname >/dev/null; sleep 5; done"]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: api, namespace: payments, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
spec:
  replicas: 1
  selector: {matchLabels: {app: api}}
  template:
    metadata: {labels: {app: api}}
    spec:
      containers:
      - name: api
        image: nginx:1.27
        command: [/bin/sh, -c]
        args: ["while :; do date >/tmp/demo-api-heartbeat; sleep 7; done"]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: worker, namespace: development, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
spec:
  replicas: 1
  selector: {matchLabels: {app: worker}}
  template:
    metadata: {labels: {app: worker}}
    spec:
      containers:
      - name: worker
        image: nginx:1.27
        command: [/bin/sh, -c]
        args: ["while :; do printf worker >/dev/null; sleep 3; done"]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: auditor, namespace: security, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
spec:
  replicas: 1
  selector: {matchLabels: {app: auditor}}
  template:
    metadata: {labels: {app: auditor}}
    spec:
      containers:
      - name: auditor
        image: nginx:1.27
        command: [/bin/sh, -c]
        args: ["while :; do cat /etc/os-release >/dev/null; sleep 6; done"]
EOF

kubectl apply -f - >/dev/null <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata: {name: developer, namespace: payments, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
---
apiVersion: v1
kind: ServiceAccount
metadata: {name: security-reviewer, namespace: security, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
---
apiVersion: v1
kind: ServiceAccount
metadata: {name: restricted-user, namespace: payments, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: landlock-genprof-demo-cluster-identity, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
rules:
- apiGroups: [""]
  resources: [namespaces]
  resourceNames: [kube-system]
  verbs: [get]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: landlock-genprof-demo-developer-identity, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: landlock-genprof-demo-cluster-identity}
subjects: [{kind: ServiceAccount, name: developer, namespace: payments}]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: landlock-genprof-demo-reviewer-identity, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: landlock-genprof-demo-cluster-identity}
subjects: [{kind: ServiceAccount, name: security-reviewer, namespace: security}]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: landlock-genprof-demo-restricted-identity, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: landlock-genprof-demo-cluster-identity}
subjects: [{kind: ServiceAccount, name: restricted-user, namespace: payments}]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: landlock-genprof-demo-developer-user-identity, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: landlock-genprof-demo-cluster-identity}
subjects: [{kind: User, name: developer}]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: landlock-genprof-demo-reviewer-user-identity, labels: {app.kubernetes.io/part-of: landlock-genprof-demo}}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: landlock-genprof-demo-cluster-identity}
subjects: [{kind: User, name: security-reviewer}]
EOF

# These are namespace-local grants. No demo identity receives Namespace LIST.
for ns in payments development security; do
  kubectl -n "$ns" create role landlock-genprof-demo-workload-reader --verb=get,list \
    --resource=pods,deployments,statefulsets,daemonsets,replicasets --dry-run=client -o yaml | kubectl apply -f - >/dev/null
done
kubectl -n payments create role landlock-genprof-demo-observer --verb=get,list,create \
  --resource=observations,securityprofileproposals,traininghistories,observationcontributionreceipts --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create role landlock-genprof-demo-observer-status --verb=get,update,patch \
  --resource=observations/status,securityprofileproposals/status,observationcontributionreceipts/status --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create role landlock-genprof-demo-observer-history --verb=get,list,create,update,patch \
  --resource=traininghistories --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create rolebinding landlock-genprof-demo-developer-workloads --role=landlock-genprof-demo-workload-reader \
  --serviceaccount=payments:developer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create rolebinding landlock-genprof-demo-developer-observer --role=landlock-genprof-demo-observer \
  --serviceaccount=payments:developer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create rolebinding landlock-genprof-demo-developer-observer-status --role=landlock-genprof-demo-observer-status \
  --serviceaccount=payments:developer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create rolebinding landlock-genprof-demo-developer-history --role=landlock-genprof-demo-observer-history \
  --serviceaccount=payments:developer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
for role in landlock-genprof-demo-workload-reader landlock-genprof-demo-observer landlock-genprof-demo-observer-status landlock-genprof-demo-observer-history; do
  kubectl -n payments create rolebinding "landlock-genprof-demo-developer-${role##*-}" --role="$role" \
    --user=developer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
done
for role in landlock-genprof-demo-workload-reader landlock-genprof-demo-observer landlock-genprof-demo-observer-status landlock-genprof-demo-observer-history; do
  kubectl -n payments create rolebinding "landlock-genprof-demo-reviewer-${role##*-}" --role="$role" \
    --user=security-reviewer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
done
kubectl -n payments create role landlock-genprof-demo-reviewer-operations --verb=get,list,update,patch,create \
  --resource=observations,observations/status,securityprofileproposals,securityprofileproposals/status,traininghistories,observationcontributionreceipts,observationcontributionreceipts/status,applyattempts,rollbackattempts \
  --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create rolebinding landlock-genprof-demo-reviewer-operations --role=landlock-genprof-demo-reviewer-operations \
  --user=security-reviewer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl apply -f - >/dev/null <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: {name: landlock-genprof-demo-developer-development, namespace: development}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: Role, name: landlock-genprof-demo-workload-reader}
subjects: [{kind: ServiceAccount, name: developer, namespace: payments}]
EOF
kubectl -n payments create rolebinding landlock-genprof-demo-restricted-workloads --role=landlock-genprof-demo-workload-reader \
  --serviceaccount=payments:restricted-user --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n security create rolebinding landlock-genprof-demo-reviewer-workloads --role=landlock-genprof-demo-workload-reader \
  --serviceaccount=security:security-reviewer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n security create role landlock-genprof-demo-reviewer-operations --verb=get,list,update,patch,create \
  --resource=observations,observations/status,securityprofileproposals,securityprofileproposals/status,traininghistories,observationcontributionreceipts,applyattempts,rollbackattempts \
  --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n security create rolebinding landlock-genprof-demo-reviewer-operations --role=landlock-genprof-demo-reviewer-operations \
  --serviceaccount=security:security-reviewer --dry-run=client -o yaml | kubectl apply -f - >/dev/null

for item in payments/frontend payments/api development/worker security/auditor; do
  ns="${item%%/*}"; name="${item##*/}"
  kubectl -n "$ns" rollout status deployment/"$name" --timeout=180s >/dev/null
done

kubectl config view --raw >"$DEMO_KUBECONFIG"
chmod 600 "$DEMO_KUBECONFIG"
cluster_ref="$(kubectl config view --raw --minify -o jsonpath='{.contexts[0].context.cluster}')"
for item in "payments developer developer" "security security-reviewer security-reviewer" "payments restricted-user restricted-user"; do
  IFS=' ' read -r ns service_account context_name <<<"$item"
  token="$(kubectl -n "$ns" create token "$service_account" --duration=8h)"
  kubectl --kubeconfig "$DEMO_KUBECONFIG" config set-credentials "demo-${context_name}" --token="$token" >/dev/null
  kubectl --kubeconfig "$DEMO_KUBECONFIG" config set-context "$context_name" --cluster="$cluster_ref" \
    --user="demo-${context_name}" --namespace="$ns" >/dev/null
done
kubectl --kubeconfig "$DEMO_KUBECONFIG" config use-context developer >/dev/null

# Reuse the proven Linux executor/Gadget process. The backend receives only a
# private host-side kubeconfig path; the executor receives a separate private
# guest-side copy whose API endpoint is reachable through Lima.
WORK_DIR="$(mktemp -d -t landlock-genprof-operations-center-executor.XXXXXX)"
chmod 700 "$WORK_DIR"
executor_host_bin="$WORK_DIR/executor-linux"
executor_host_config="$WORK_DIR/executor-kubeconfig"
backend_executor_config="$WORK_DIR/backend-executor-kubeconfig"
backend_host_config="$WORK_DIR/backend-kubeconfig"
proxy_bin="$WORK_DIR/trustedproxy"
secret_file="$WORK_DIR/hmac.secret"
GUEST_EXECUTOR_BIN="/tmp/landlock-genprof-operations-center-executor-${$}"
GUEST_EXECUTOR_KUBECONFIG="/tmp/landlock-genprof-operations-center-kubeconfig-${$}"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$executor_host_bin" ./cmd/landlock-genprof
kubectl config view --raw --minify >"$executor_host_config"
chmod 600 "$executor_host_config"
cp "$executor_host_config" "$backend_executor_config"
chmod 600 "$backend_executor_config"
kubectl config view --raw >"$backend_host_config"
chmod 600 "$backend_host_config"
head -c 32 /dev/urandom | base64 | tr -d '\n' >"$secret_file"
chmod 600 "$secret_file"
api_server="$(kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.server}')"
guest_api_server="https://host.lima.internal:${api_server##*:}"
guest_cluster="$(kubectl config view --raw --minify -o jsonpath='{.contexts[0].context.cluster}')"
kubectl --kubeconfig "$executor_host_config" config set-cluster "$guest_cluster" --server="$guest_api_server" --tls-server-name=kubernetes >/dev/null
base64 <"$executor_host_bin" | limactl shell "$LIMA_VM" -- sh -c "base64 -d > '$GUEST_EXECUTOR_BIN' && chmod 700 '$GUEST_EXECUTOR_BIN'"
base64 <"$executor_host_config" | limactl shell "$LIMA_VM" -- sh -c "base64 -d > '$GUEST_EXECUTOR_KUBECONFIG' && chmod 600 '$GUEST_EXECUTOR_KUBECONFIG'"
limactl shell "$LIMA_VM" -- env KUBECONFIG="$GUEST_EXECUTOR_KUBECONFIG" "$GUEST_EXECUTOR_BIN" executor --namespace payments --namespace development --namespace security >"$WORK_DIR/executor.log" 2>&1 &
EXECUTOR_PID=$!
sleep 3
kill -0 "$EXECUTOR_PID" 2>/dev/null || { tail -80 "$WORK_DIR/executor.log" >&2 || true; die "Linux Observation executor exited during startup"; }
echo "EXECUTOR_READY=YES"

echo "DEMO_KUBECONFIG_READY=YES"
echo "DEMO_CONTEXTS=developer,security-reviewer,restricted-user"
echo "DEMO_NAMESPACES=payments,development,platform,security"
echo "DEMO_WORKLOADS=payments/frontend,payments/api,development/worker,security/auditor"
echo "DEMO_RBAC=REAL_KUBERNETES_RBAC"

go build -o "$proxy_bin" ./hack/trustedproxy

(
  cd "$ROOT_DIR"
  exec env KUBECONFIG="$backend_host_config" LANDLOCK_GENPROF_ENVIRONMENT_KUBECONFIG="$DEMO_KUBECONFIG" \
    LANDLOCK_GENPROF_DEPLOYMENT_MODE=production \
    LANDLOCK_GENPROF_TRUSTED_PROXY_HMAC_SECRET="$(<"$secret_file")" \
    LANDLOCK_GENPROF_ALLOWED_USERS="$DEMO_PROXY_USER" \
    LANDLOCK_GENPROF_ALLOWED_HOST="127.0.0.1:${DEMO_PROXY_PORT}" \
    LANDLOCK_GENPROF_OBSERVATION_EXECUTOR_KUBECONFIG="$backend_executor_config" \
    go run ./cmd/landlock-genprof ui --namespace payments --port "$DEMO_BACKEND_PORT"
) &
UI_PID=$!
for _ in $(seq 1 90); do
  if ! kill -0 "$UI_PID" 2>/dev/null; then die "Operations Center exited before readiness"; fi
  code="$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${DEMO_BACKEND_PORT}/" 2>/dev/null || true)"
  if [ "$code" = 200 ] || [ "$code" = 401 ] || [ "$code" = 403 ]; then
    break
  fi
  sleep 1
done
backend_code="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${DEMO_BACKEND_PORT}/" 2>/dev/null || true)"
[ "$backend_code" = 200 ] || [ "$backend_code" = 401 ] || [ "$backend_code" = 403 ] || die "Operations Center did not become ready"

(
  cd "$ROOT_DIR"
  exec "$proxy_bin" -listen "127.0.0.1:${DEMO_PROXY_PORT}" \
    -backend "http://127.0.0.1:${DEMO_BACKEND_PORT}" \
    -secret-file "$secret_file" \
    -user "$DEMO_PROXY_USER"
) >"$WORK_DIR/proxy.log" 2>&1 &
PROXY_PID=$!
for _ in $(seq 1 30); do
  if ! kill -0 "$PROXY_PID" 2>/dev/null; then tail -80 "$WORK_DIR/proxy.log" >&2 || true; die "Trusted Proxy fixture exited before readiness"; fi
  code="$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${DEMO_PROXY_PORT}/" 2>/dev/null || true)"
  [ "$code" = 200 ] && break
  sleep 1
done
[ "$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${DEMO_PROXY_PORT}/" 2>/dev/null || true)" = 200 ] || die "Trusted Proxy fixture did not become ready"
echo "OPERATIONS_CENTER_DEMO_READY=YES"
echo "URL=http://127.0.0.1:${DEMO_PROXY_PORT}"
echo "CLUSTER=${CLUSTER_NAME}"
echo "CONTEXTS=developer,security-reviewer,restricted-user"
echo "NAMESPACES=payments,development,platform,security"
echo "EXECUTOR_READY=YES"
echo "GADGET_READY=YES"
echo "Run the five-minute flow in docs/operations-center-demo.md. Press Ctrl-C to stop."
wait "$UI_PID"
