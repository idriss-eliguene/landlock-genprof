#!/usr/bin/env bash
set -euo pipefail

# Smoke tracer: deploy disposable pod, run landlock-genprof trace, assert >=1 event
NAMESPACE=${1:-landlock-genprof-smoke}
POD=${2:-nginx-smoke}
IMAGE=${3:-nginx:1.25}
DURATION=${4:-10s}

if ! command -v kubectl >/dev/null 2>&1; then
  echo "ERROR: kubectl not found" >&2
  exit 2
fi
if ! command -v kubectl-landlock_genprof >/dev/null 2>&1; then
  echo "ERROR: kubectl-landlock_genprof not found on PATH (canonical plugin required)" >&2
  exit 2
fi
if ! kubectl landlock-genprof --help >/dev/null 2>&1; then
  echo "ERROR: kubectl landlock-genprof --help failed (canonical plugin required)" >&2
  exit 2
fi

echo "[smoke-tracer] namespace=$NAMESPACE pod=$POD image=$IMAGE"

kubectl create ns "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# Deploy server pod using default nginx process (listening)
cat <<EOF | kubectl apply -n "$NAMESPACE" -f -
apiVersion: v1
kind: Pod
metadata:
  name: $POD
spec:
  containers:
  - name: nginx
    image: $IMAGE
    imagePullPolicy: IfNotPresent
EOF

# wait for Running
for i in $(seq 1 60); do
  phase=$(kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
  echo "[smoke-tracer] pod phase=$phase"
  if [ "$phase" = "Running" ]; then break; fi
  sleep 1
done

if [ "$(kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}')" != "Running" ]; then
  kubectl describe pod "$POD" -n "$NAMESPACE" || true
  kubectl get events -n "$NAMESPACE" --sort-by=.lastTimestamp || true
  echo "ERROR: pod did not reach Running"
  kubectl delete pod "$POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 3
fi

# Ensure Inspektor Gadget is present and ready
if ! kubectl get ns gadget >/dev/null 2>&1; then
  echo "ERROR: gadget namespace not found" >&2
  kubectl get pods -n gadget || true
  exit 4
fi
# ensure daemonset(s) are ready
# safely enumerate gadget daemonsets
mapfile -t GADGET_DS_NAMES < <(
  kubectl get daemonset -n gadget -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null
)
# require exactly one Gadget DaemonSet (fail-closed)
if [ "${#GADGET_DS_NAMES[@]}" -eq 0 ]; then
  echo "ERROR: no Gadget DaemonSet found in namespace 'gadget'" >&2
  kubectl -n gadget get daemonset -o wide || true
  exit 4
fi
if [ "${#GADGET_DS_NAMES[@]}" -gt 1 ]; then
  echo "ERROR: multiple Gadget DaemonSets found in namespace 'gadget': ${GADGET_DS_NAMES[*]}" >&2
  kubectl -n gadget get daemonset -o wide || true
  exit 4
fi
# single name assigned
ds="${GADGET_DS_NAMES[0]}"
# validate name: no whitespace, slash, quote, backslash
# validate name: accept only valid Kubernetes DNS-1123 label for resource names
if ! [[ "$ds" =~ ^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$ ]]; then
  echo "ERROR: invalid Gadget DaemonSet name: [$ds]" >&2
  kubectl -n gadget get daemonset -o wide || true
  exit 4
fi

desired=$(kubectl get daemonset -n gadget "$ds" -o jsonpath='{.status.desiredNumberScheduled}')
ready=$(kubectl get daemonset -n gadget "$ds" -o jsonpath='{.status.numberReady}')
echo "[smoke-tracer] gadget daemonset $ds desired=$desired ready=$ready"
if [ -z "$desired" ] || [ "$desired" -eq 0 ]; then
  echo "ERROR: gadget daemonset $ds has no desired pods" >&2
  exit 4
fi
if [ "$ready" -lt "$desired" ]; then
  echo "waiting for gadget daemonset $ds to rollout"
  if ! kubectl rollout status daemonset/"$ds" -n gadget --timeout=120s; then
    echo "ERROR: gadget daemonset $ds failed to become ready" >&2
    kubectl -n gadget get pods -o wide || true
    exit 4
  fi
fi

OUT_FILE="/tmp/${POD}-events.json"
echo "[smoke-tracer] will run tracer for $DURATION -> $OUT_FILE"

# create a client pod to generate traffic while tracing
CLIENT_POD=${POD}-client
cat <<EOF | kubectl apply -n "$NAMESPACE" -f -
apiVersion: v1
kind: Pod
metadata:
  name: $CLIENT_POD
spec:
  containers:
  - name: client
    image: curlimages/curl:8.2.1
    command: ["/bin/sh","-c","sleep 300"]
  restartPolicy: Always
EOF
kubectl wait --for=condition=Ready pod/"$CLIENT_POD" -n "$NAMESPACE" --timeout=120s

# get server IP for client to hit
SERVER_IP=$(kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.status.podIP}')
if [ -z "$SERVER_IP" ]; then
  echo "ERROR: could not resolve server pod IP" >&2
  kubectl delete pod "$CLIENT_POD" -n "$NAMESPACE" --ignore-not-found
  exit 4
fi

# Validate OUT_FILE is absolute and non-empty to avoid cobra NoOptDefVal ambiguity
if [ -z "${OUT_FILE:-}" ]; then
  echo "ERROR: OUT_FILE is empty" >&2
  exit 1
fi
case "$OUT_FILE" in
  /*) ;;
  *)
    echo "ERROR: OUT_FILE must be absolute: [$OUT_FILE]" >&2
    exit 1
    ;;
esac

# remove any stale output file
rm -f -- "$OUT_FILE"

# Canonical E2E CLI contract: kubectl plugin consumption only.
TRACE_LOG="/tmp/${POD}-trace.log"
CLI_EXEC=(kubectl landlock-genprof)

TRACE_CMD=(
  "${CLI_EXEC[@]}"
  trace
  --pod "$POD"
  --namespace "$NAMESPACE"
  --binary "/usr/sbin/nginx"
  --duration "$DURATION"
  "--events-out=$OUT_FILE"
)
# Print argv safely for audit
printf 'trace argv:'
for arg in "${TRACE_CMD[@]}"; do printf ' %q' "$arg"; done
printf '\n'

# start tracer and capture output to TRACE_LOG
"${TRACE_CMD[@]}" >"$TRACE_LOG" 2>&1 &
TRACE_PID=$!

# give trace a moment to attach
sleep 2

# during the trace window, generate traffic from client to server (best-effort)
# try a few times to ensure activity
for i in $(seq 1 20); do
  kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "curl -sS --max-time 2 http://$SERVER_IP:80 || true" >/dev/null 2>&1 || true
  sleep 1
done

# wait for tracer to finish and capture exit status
set +e
wait "$TRACE_PID"
TRACE_RC=$?
set -e

echo "trace exit code: $TRACE_RC"
if [ "$TRACE_RC" -ne 0 ]; then
  echo "ERROR: tracer exited with non-zero status $TRACE_RC" >&2
  echo "--- tracer log ($TRACE_LOG) ---" >&2
  sed -n '1,200p' "$TRACE_LOG" >&2 || true
  kubectl logs -n gadget -l app.kubernetes.io/name=gadget --all-containers || true
  kubectl get pods -n gadget -o wide || true
  kubectl delete pod "$POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete pod "$CLIENT_POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 4
fi

# verify output file exists
if [ ! -f "$OUT_FILE" ]; then
  echo "ERROR: events file not produced: $OUT_FILE"
  kubectl logs -n gadget -l app.kubernetes.io/name=gadget --all-containers || true
  kubectl get pods -n gadget -o wide || true
  kubectl describe pod "$POD" -n "$NAMESPACE" || true
  kubectl get events -n "$NAMESPACE" --sort-by=.lastTimestamp || true
  kubectl delete pod "$POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete pod "$CLIENT_POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 4
fi

# count events robustly for v1 (array) or v2 ({Events: [...]}) formats
# Validate v2 evidence schema: object with .version=="v2" and .events array non-empty
if jq -e 'if type=="array" then (length>0) else ((.version == "v2") and (.events | type == "array") and (.events | length > 0)) end' "$OUT_FILE" >/dev/null 2>&1; then
  if jq -e 'type=="array"' "$OUT_FILE" >/dev/null 2>&1; then
    count=$(jq 'length' "$OUT_FILE")
  else
    count=$(jq '.events|length' "$OUT_FILE")
  fi
else
  echo "ERROR: no events recorded or malformed events file"
  jq . "$OUT_FILE" || true
  kubectl logs -n gadget -l app.kubernetes.io/name=gadget --all-containers || true
  kubectl delete pod "$POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete pod "$CLIENT_POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 5
fi

echo "[smoke-tracer] success: events count=$count"

# cleanup
kubectl delete pod "$POD" -n "$NAMESPACE" --ignore-not-found
kubectl delete pod "$CLIENT_POD" -n "$NAMESPACE" --ignore-not-found
kubectl delete ns "$NAMESPACE" --ignore-not-found

exit 0
