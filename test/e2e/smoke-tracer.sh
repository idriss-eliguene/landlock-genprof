#!/usr/bin/env bash
set -euo pipefail

# Smoke tracer: deploy disposable pod, run landlock-genprof trace, assert >=1 event
NAMESPACE=${1:-landlock-genprof-smoke}
POD=${2:-nginx-smoke}
IMAGE=${3:-nginx:1.25}
DURATION=${4:-10s}

if [ -z "${LANDLOCK_GENPROF_BIN:-}" ]; then
  echo "ERROR: LANDLOCK_GENPROF_BIN must be set to the linux-native ./bin/landlock-genprof"
  exit 2
fi

echo "[smoke-tracer] namespace=$NAMESPACE pod=$POD image=$IMAGE"

kubectl create ns "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

cat <<EOF | kubectl apply -n "$NAMESPACE" -f -
apiVersion: v1
kind: Pod
metadata:
  name: $POD
spec:
  containers:
  - name: nginx
    image: $IMAGE
    command: ["/bin/sh","-c","sleep 300"]
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

OUT_FILE="/tmp/${POD}-events.json"
echo "[smoke-tracer] running tracer for $DURATION -> $OUT_FILE"
# run tracer; it will use the in-cluster Gadget via SDK
"${LANDLOCK_GENPROF_BIN}" trace --pod "$POD" -n "$NAMESPACE" --binary /usr/sbin/nginx --duration "$DURATION" --events-out "$OUT_FILE" || true

if [ ! -f "$OUT_FILE" ]; then
  echo "ERROR: events file not produced: $OUT_FILE"
  kubectl logs -n gadget -l app.kubernetes.io/name=gadget --all-containers || true
  kubectl get pods -n gadget -o wide || true
  kubectl describe pod "$POD" -n "$NAMESPACE" || true
  kubectl get events -n "$NAMESPACE" --sort-by=.lastTimestamp || true
  kubectl delete pod "$POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 4
fi

# crude check: ensure JSON array with at least one element
count=$(jq 'length' "$OUT_FILE" 2>/dev/null || echo 0)
if [ "$count" -lt 1 ]; then
  echo "ERROR: no events recorded (count=$count)"
  jq . "$OUT_FILE" || true
  kubectl logs -n gadget -l app.kubernetes.io/name=gadget --all-containers || true
  kubectl delete pod "$POD" -n "$NAMESPACE" --ignore-not-found
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 5
fi

echo "[smoke-tracer] success: events count=$count"

# cleanup
kubectl delete pod "$POD" -n "$NAMESPACE" --ignore-not-found
kubectl delete ns "$NAMESPACE" --ignore-not-found

exit 0
