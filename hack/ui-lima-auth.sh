#!/usr/bin/env bash
set -Eeuo pipefail

# Reproducible qualification harness for the production-like, authenticated
# Operations Center path. Unlike `make ui-lima` (UI_MODE=local, no trusted
# proxy, no auth), this starts the real backend in
# LANDLOCK_GENPROF_DEPLOYMENT_MODE=production and puts it behind
# hack/trustedproxy, a bounded TEST FIXTURE that faithfully exercises the
# production header/signature contract (via internal/authn) without being a
# production reverse proxy itself. It automates exactly the previously
# manual procedure in docs/test-environment.md.
#
# It creates only disposable, qualification-scoped material: a random HMAC
# secret file under a private temp directory, and (if requested) a
# disposable Kubernetes Secret/allowlist. It never touches the canonical
# CRDs, the canonical cluster/VM, or any historical/durable specimen, and it
# never prints the HMAC secret or signature material.

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
UI_NAMESPACE="${UI_NAMESPACE:-default}"
BACKEND_PORT="${BACKEND_PORT:-8081}"
PROXY_PORT="${PROXY_PORT:-8090}"
BACKEND_HOST=127.0.0.1
PROXY_HOST=127.0.0.1
PROXY_URL="http://${PROXY_HOST}:${PROXY_PORT}"
QUALIFICATION_USER="${QUALIFICATION_USER:-qualification-operator}"
QUALIFICATION_GROUPS="${QUALIFICATION_GROUPS:-}"
QUALIFICATION_ROLE_NAME="landlock-genprof-ui-lima-auth-viewer"
QUALIFICATION_BINDING_NAME="landlock-genprof-ui-lima-auth-viewer-binding"

BACKEND_PID=""
PROXY_PID=""
WORK_DIR=""
QUALIFICATION_RBAC_CREATED=0

die() {
  echo "ERROR: $*" >&2
  exit 1
}

stop_all() {
  if [ -n "$PROXY_PID" ] && kill -0 "$PROXY_PID" 2>/dev/null; then
    kill "$PROXY_PID" 2>/dev/null || true
    wait "$PROXY_PID" 2>/dev/null || true
  fi
  if [ -n "$BACKEND_PID" ] && kill -0 "$BACKEND_PID" 2>/dev/null; then
    kill "$BACKEND_PID" 2>/dev/null || true
    wait "$BACKEND_PID" 2>/dev/null || true
  fi
  if [ "$QUALIFICATION_RBAC_CREATED" -eq 1 ]; then
    kubectl delete clusterrolebinding "$QUALIFICATION_BINDING_NAME" --ignore-not-found >/dev/null 2>&1 || true
    kubectl delete clusterrole "$QUALIFICATION_ROLE_NAME" --ignore-not-found >/dev/null 2>&1 || true
    echo "DISPOSABLE_RBAC_REMOVED name=${QUALIFICATION_ROLE_NAME}"
  fi
  if [ -n "$WORK_DIR" ]; then
    # Contains only the disposable HMAC secret and a copy of the current
    # kubeconfig used as the temporary v0.8 executor-kubeconfig compatibility
    # input (see docs/v08-governance-operations.md, G9-D1). Neither is
    # canonical/durable state.
    rm -rf "$WORK_DIR"
  fi
  lib_core_readiness_cleanup
}

cleanup() {
  local status=$?
  echo
  echo "CLEANUP_STARTING"
  stop_all
  echo "CLEANUP_DONE"
  exit "$status"
}
handle_signal() {
  trap - INT TERM
  # Do not call cleanup() directly here: this handler runs inside the same
  # shell as the EXIT trap below, so falling through to `exit` lets that
  # trap run cleanup exactly once instead of invoking it twice.
  exit 130
}
trap cleanup EXIT
trap handle_signal INT TERM

lib_core_readiness_require_commands curl docker go kubectl limactl make
lib_core_readiness_check

for port in "$BACKEND_PORT" "$PROXY_PORT"; do
  case "$port" in
    ''|*[!0-9]*) die "ports must be numeric (BACKEND_PORT/PROXY_PORT)" ;;
  esac
done
if command -v lsof >/dev/null 2>&1; then
  for port in "$BACKEND_PORT" "$PROXY_PORT"; do
    lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | tail -n +2 | grep -q . &&
      die "port ${port} is already in use; retry with BACKEND_PORT=/PROXY_PORT= set to a free pair"
  done
fi

# Disposable, uniquely-named read-only RBAC for the qualification identity.
# Its rules are a verbatim copy of the chart's own reusable
# landlock-genprof-team-viewer ClusterRole
# (deploy/helm/landlock-genprof/templates/rbac-operations-center-team.yaml)
# under a different, ui-lima-auth-scoped name, so this harness does not
# invent a new permission set and does not collide with that object if the
# real chart is ever installed alongside it in this cluster.
kubectl apply -f - >/dev/null <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${QUALIFICATION_ROLE_NAME}
  labels:
    app.kubernetes.io/managed-by: landlock-genprof-ui-lima-auth
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list"]
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
    verbs: ["get", "list"]
  - apiGroups: ["landlockgenprof.io"]
    resources: ["observations", "observationcontributionreceipts", "traininghistories", "securityprofileproposals", "applyattempts", "rollbackattempts"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${QUALIFICATION_BINDING_NAME}
  labels:
    app.kubernetes.io/managed-by: landlock-genprof-ui-lima-auth
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ${QUALIFICATION_ROLE_NAME}
subjects:
  - kind: User
    name: ${QUALIFICATION_USER}
    apiGroup: rbac.authorization.k8s.io
EOF
QUALIFICATION_RBAC_CREATED=1
echo "DISPOSABLE_RBAC_CREATED name=${QUALIFICATION_ROLE_NAME} subject=${QUALIFICATION_USER}"

WORK_DIR="$(mktemp -d -t landlock-genprof-ui-lima-auth.XXXXXX)"
chmod 700 "$WORK_DIR"
secret_file="${WORK_DIR}/hmac.secret"
# Text-encode the random bytes: the production code reads this secret as an
# environment variable (os.Getenv), which cannot carry embedded NUL bytes,
# so the on-disk secret material must be safe text, not raw binary.
head -c 32 /dev/urandom | base64 | tr -d '\n' > "$secret_file"
chmod 600 "$secret_file"
echo "DISPOSABLE_HMAC_SECRET_CREATED path=${secret_file} length=$(wc -c < "$secret_file" | tr -d ' ')"

# The current v0.8 startup contract requires a kubeconfig path for the
# executor client even when qualifying only the read/auth path
# (docs/v08-governance-operations.md, "G9-D1"). Use a private copy of the
# operator's own current kubeconfig rather than the real file, so cleanup
# can remove it without touching anything the operator manages.
executor_kubeconfig="${WORK_DIR}/executor-kubeconfig"
kubectl config view --raw --minify > "$executor_kubeconfig"
chmod 600 "$executor_kubeconfig"

echo "BACKEND_STARTING address=${BACKEND_HOST}:${BACKEND_PORT} namespace=${UI_NAMESPACE} mode=production"
(
  cd "$ROOT_DIR"
  exec env \
    LANDLOCK_GENPROF_DEPLOYMENT_MODE=production \
    LANDLOCK_GENPROF_TRUSTED_PROXY_HMAC_SECRET="$(cat "$secret_file")" \
    LANDLOCK_GENPROF_ALLOWED_USERS="$QUALIFICATION_USER" \
    LANDLOCK_GENPROF_OBSERVATION_EXECUTOR_KUBECONFIG="$executor_kubeconfig" \
    LANDLOCK_GENPROF_ALLOWED_HOST="${PROXY_HOST}:${PROXY_PORT}" \
    go run ./cmd/landlock-genprof ui --namespace "$UI_NAMESPACE" --port "$BACKEND_PORT"
) >"${WORK_DIR}/backend.log" 2>&1 &
BACKEND_PID=$!

backend_ready=0
for _ in $(seq 1 60); do
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    die "backend exited before becoming ready; see ${WORK_DIR}/backend.log"
  fi
  code="$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' -H "Host: ${PROXY_HOST}:${PROXY_PORT}" "http://${BACKEND_HOST}:${BACKEND_PORT}/" 2>/dev/null || true)"
  if [ "$code" = 401 ] || [ "$code" = 200 ]; then
    backend_ready=1
    break
  fi
  sleep 1
done
[ "$backend_ready" -eq 1 ] || die "backend did not become ready within 60 seconds; see ${WORK_DIR}/backend.log"
echo "BACKEND_READY"

echo "AUTH_CONTRACT_CHECK_STARTING"

unsigned_code="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' -H "Host: ${PROXY_HOST}:${PROXY_PORT}" "http://${BACKEND_HOST}:${BACKEND_PORT}/")"
[ "$unsigned_code" = 401 ] || die "unsigned request expected 401, got ${unsigned_code}"
echo "UNSIGNED_REQUEST=401 (expected)"

# authfixture's "env" format (NAME=value, one per line) is parsed into a
# curl -H argument array here, one array entry per header, so no header
# value (or the whole assertion) is ever exposed to shell word-splitting or
# re-quoting the way capturing "-H \"...\"" text into a variable would be.
# HTTP header names are case-insensitive, so translating underscores to
# hyphens is all that is needed — no case round-trip is required for the
# server to recognize X_OPERATIONS_CENTER_USER as X-Operations-Center-User.
build_header_args() {
  local -n out_array=$1
  shift
  out_array=()
  while IFS='=' read -r name value; do
    [ -z "$name" ] && continue
    if [ -z "$value" ]; then
      # curl silently drops "-H 'Name: '" (empty value) instead of sending
      # an empty header. The production Verifier requires the Groups header
      # to be present exactly once even for a groupless identity
      # (internal/authn/identity.go), so an empty value must still be sent;
      # curl's "-H 'Name;'" form is the documented way to force that.
      out_array+=(-H "${name//_/-};")
    else
      out_array+=(-H "${name//_/-}: ${value}")
    fi
  done < <(go run "$ROOT_DIR/hack/authfixture" -secret-file "$secret_file" -user "$QUALIFICATION_USER" -groups "$QUALIFICATION_GROUPS" -format env "$@")
}

declare -a valid_header_args
build_header_args valid_header_args
valid_code="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' -H "Host: ${PROXY_HOST}:${PROXY_PORT}" "${valid_header_args[@]}" "http://${BACKEND_HOST}:${BACKEND_PORT}/")"
[ "$valid_code" = 200 ] || die "validly signed request expected 200, got ${valid_code}"
echo "VALID_SIGNED_REQUEST=200 (expected)"

declare -a stale_header_args
build_header_args stale_header_args -age 10m
stale_code="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' -H "Host: ${PROXY_HOST}:${PROXY_PORT}" "${stale_header_args[@]}" "http://${BACKEND_HOST}:${BACKEND_PORT}/")"
[ "$stale_code" = 401 ] || die "10-minute-stale signed request expected 401, got ${stale_code}"
echo "STALE_SIGNED_REQUEST=401 (expected)"

echo "AUTH_CONTRACT_CHECK_PASSED unsigned=401 valid=200 stale=401"

echo "TRUSTED_PROXY_FIXTURE_STARTING listen=${PROXY_URL} identity=${QUALIFICATION_USER}"
echo "TEST FIXTURE. NOT A PRODUCTION PROXY. See hack/trustedproxy/main.go."
(
  cd "$ROOT_DIR"
  exec go run ./hack/trustedproxy \
    -listen "${PROXY_HOST}:${PROXY_PORT}" \
    -backend "http://${BACKEND_HOST}:${BACKEND_PORT}" \
    -secret-file "$secret_file" \
    -user "$QUALIFICATION_USER" \
    -groups "$QUALIFICATION_GROUPS"
) >"${WORK_DIR}/proxy.log" 2>&1 &
PROXY_PID=$!

proxy_ready=0
for _ in $(seq 1 30); do
  if ! kill -0 "$PROXY_PID" 2>/dev/null; then
    die "trusted-proxy fixture exited before becoming ready; see ${WORK_DIR}/proxy.log"
  fi
  if [ "$(curl -sS --max-time 2 -o /dev/null -w '%{http_code}' "${PROXY_URL}/")" = 200 ]; then
    proxy_ready=1
    break
  fi
  sleep 1
done
[ "$proxy_ready" -eq 1 ] || die "trusted-proxy fixture did not become ready within 30 seconds; see ${WORK_DIR}/proxy.log"
echo "TRUSTED_PROXY_FIXTURE_READY"

echo "UI_MODE=PRODUCTION_LIKE_TRUSTED_PROXY"
echo "AUTH_MODE=SIGNED_TRUSTED_PROXY_ASSERTION"
echo
echo "Open in your browser (through the fixture proxy only; the backend"
echo "itself is not meant to be reached directly):"
echo
echo "$PROXY_URL"
echo
echo "Every request through this URL is asserted as identity"
echo "'${QUALIFICATION_USER}'. Press Ctrl-C to stop and clean up."
wait "$BACKEND_PID"
