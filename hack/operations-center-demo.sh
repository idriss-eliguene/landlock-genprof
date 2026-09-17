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
UI_PID=""

die() { echo "ERROR: $*" >&2; exit 1; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [ -n "$UI_PID" ] && kill -0 "$UI_PID" 2>/dev/null; then
    kill -TERM "$UI_PID" 2>/dev/null || true
    wait "$UI_PID" 2>/dev/null || true
  fi
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
EOF

# These are namespace-local grants. No demo identity receives Namespace LIST.
for ns in payments development security; do
  kubectl -n "$ns" create role landlock-genprof-demo-workload-reader --verb=get,list \
    --resource=pods,deployments,statefulsets,daemonsets,replicasets --dry-run=client -o yaml | kubectl apply -f - >/dev/null
done
kubectl -n payments create role landlock-genprof-demo-observer --verb=get,list,create \
  --resource=observations,securityprofileproposals,traininghistories,observationcontributionreceipts --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create role landlock-genprof-demo-observer-status --verb=get,update,patch \
  --resource=observations/status,securityprofileproposals/status --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create rolebinding landlock-genprof-demo-developer-workloads --role=landlock-genprof-demo-workload-reader \
  --serviceaccount=payments:developer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create rolebinding landlock-genprof-demo-developer-observer --role=landlock-genprof-demo-observer \
  --serviceaccount=payments:developer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n payments create rolebinding landlock-genprof-demo-developer-observer-status --role=landlock-genprof-demo-observer-status \
  --serviceaccount=payments:developer --dry-run=client -o yaml | kubectl apply -f - >/dev/null
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

echo "DEMO_KUBECONFIG_READY=YES"
echo "DEMO_CONTEXTS=developer,security-reviewer,restricted-user"
echo "DEMO_NAMESPACES=payments,development,platform,security"
echo "DEMO_WORKLOADS=payments/frontend,payments/api,development/worker,security/auditor"
echo "DEMO_RBAC=REAL_KUBERNETES_RBAC"

(
  cd "$ROOT_DIR"
  exec env KUBECONFIG="$DEMO_KUBECONFIG" LANDLOCK_GENPROF_ENVIRONMENT_KUBECONFIG="$DEMO_KUBECONFIG" \
    go run ./cmd/landlock-genprof ui --namespace payments --port "$DEMO_BACKEND_PORT"
) &
UI_PID=$!
for _ in $(seq 1 90); do
  if ! kill -0 "$UI_PID" 2>/dev/null; then die "Operations Center exited before readiness"; fi
  code="$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${DEMO_BACKEND_PORT}/" 2>/dev/null || true)"
  [ "$code" = 200 ] && break
  sleep 1
done
[ "$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${DEMO_BACKEND_PORT}/" 2>/dev/null || true)" = 200 ] || die "Operations Center did not become ready"
echo "OPERATIONS_CENTER_DEMO_READY=YES"
echo "URL=http://127.0.0.1:${DEMO_BACKEND_PORT}"
echo "CLUSTER=${CLUSTER_NAME}"
echo "CONTEXTS=developer,security-reviewer,restricted-user"
echo "NAMESPACES=payments,development,platform,security"
echo "Run the five-minute flow in docs/operations-center-demo.md. Press Ctrl-C to stop."
wait "$UI_PID"
