#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/podlock-upstream-smoke-artifacts}"
NS="${NS:-podlock-upstream-smoke}"
IMAGE="${IMAGE:-landlock-genprof/fsprobe:podlock-smoke}"
mkdir -p "$ARTIFACTS_DIR"
fail() { echo "ERROR: $*" >&2; exit 1; }

kubectl wait --for=condition=Ready node --all --timeout=300s
kubectl get nodes -o wide | tee "$ARTIFACTS_DIR/nodes.txt"
kubectl get crd landlockprofiles.podlock.kubewarden.io > "$ARTIFACTS_DIR/crd.txt"
kubectl -n podlock rollout status deployment/podlock-controller --timeout=300s
kubectl -n podlock rollout status daemonset/podlock-nri-plugin --timeout=300s
kubectl get mutatingwebhookconfiguration podlock-controller-webhook > "$ARTIFACTS_DIR/mutating-webhook.txt"
kubectl get validatingwebhookconfiguration podlock-controller-webhook-validating > "$ARTIFACTS_DIR/validating-webhook.txt"
kubectl -n podlock get service podlock-controller-webhook > "$ARTIFACTS_DIR/webhook-service.txt"
kubectl -n podlock get secret podlock-controller-webhook-tls -o json > "$ARTIFACTS_DIR/webhook-tls.json"

gcc -O2 -Wall -Wextra "$ROOT_DIR/test/e2e/landlock-abi-probe.c" -o /tmp/landlock-abi-probe
/tmp/landlock-abi-probe | tee "$ARTIFACTS_DIR/landlock-abi.txt"
ABI="$(sed -n 's/^LANDLOCK_ABI=//p' "$ARTIFACTS_DIR/landlock-abi.txt")"
[ -n "$ABI" ] && [ "$ABI" -ge 3 ] || fail "Landlock ABI < 3"

kubectl get node -o jsonpath='{.items[0].status.nodeInfo.containerRuntimeVersion}' \
  | tee "$ARTIFACTS_DIR/container-runtime.txt"
grep -q '^containerd://' "$ARTIFACTS_DIR/container-runtime.txt" \
  || fail "unsupported container runtime"
sudo k3s ctr plugins ls | tee "$ARTIFACTS_DIR/containerd-plugins.txt"
grep -qi nri "$ARTIFACTS_DIR/containerd-plugins.txt" \
  || fail "containerd NRI plugin is not present"
if command -v runc >/dev/null 2>&1; then command -v runc > "$ARTIFACTS_DIR/runc.txt"; \
elif sudo test -x /var/lib/rancher/k3s/data/current/bin/runc; then echo /var/lib/rancher/k3s/data/current/bin/runc > "$ARTIFACTS_DIR/runc.txt"; \
else fail "runC unavailable"; fi
sudo test -S /var/run/nri/nri.sock || fail "NRI socket unavailable"

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
cat > /tmp/podlock-smoke-profile.yaml <<YAML
apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: smoke-profile
  namespace: $NS
spec:
  profilesByContainer:
    probe:
      /probe/fsprobe:
        readOnly:
        - /data/allowed.txt
YAML
kubectl apply -f /tmp/podlock-smoke-profile.yaml >/dev/null
kubectl get landlockprofile smoke-profile -n "$NS" -o yaml > "$ARTIFACTS_DIR/profile.yaml"

cat > /tmp/podlock-smoke-control.yaml <<YAML
apiVersion: v1
kind: Pod
metadata: {name: control, namespace: $NS}
spec:
  restartPolicy: Always
  containers:
  - name: probe
    image: $IMAGE
    imagePullPolicy: Never
    command: ["sh", "-c", "while true; do sleep 2; done"]
YAML
kubectl apply -f /tmp/podlock-smoke-control.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/control -n "$NS" --timeout=180s
kubectl exec -n "$NS" control -c probe -- /probe/fsprobe /data/denied.txt \
  | tee "$ARTIFACTS_DIR/control.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/control.txt" || fail "control access failed"

cat > /tmp/podlock-smoke-protected.yaml <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: protected
  namespace: $NS
  labels:
    podlock.kubewarden.io/profile: smoke-profile
spec:
  restartPolicy: Always
  containers:
  - name: probe
    image: $IMAGE
    imagePullPolicy: Never
    command: ["sh", "-c", "while true; do sleep 2; done"]
YAML
kubectl apply -f /tmp/podlock-smoke-protected.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/protected -n "$NS" --timeout=180s
kubectl get pod protected -n "$NS" -o yaml > "$ARTIFACTS_DIR/protected-pod.yaml"
kubectl exec -n "$NS" protected -c probe -- /probe/fsprobe /data/allowed.txt \
  | tee "$ARTIFACTS_DIR/protected-allowed.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/protected-allowed.txt" || fail "allowed access failed"
set +e
kubectl exec -n "$NS" protected -c probe -- /probe/fsprobe /data/denied.txt \
  > "$ARTIFACTS_DIR/protected-denied.txt" 2>&1
DENIED_RC=$?
set -e
cat "$ARTIFACTS_DIR/protected-denied.txt"
grep -F 'result=failure errno=13' "$ARTIFACTS_DIR/protected-denied.txt" || fail "Landlock denial not observed"
kubectl logs protected -n "$NS" > "$ARTIFACTS_DIR/protected-logs.txt" 2>&1 || true
kubectl -n podlock logs daemonset/podlock-nri-plugin -c nri --tail=2000 > "$ARTIFACTS_DIR/nri.log" 2>&1 || true
grep -qiE 'landlock profile applied|mutation requested' "$ARTIFACTS_DIR/protected-logs.txt" "$ARTIFACTS_DIR/nri.log" \
  || fail "no PodLock application evidence"

cat > /tmp/podlock-smoke-missing.yaml <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: missing
  namespace: $NS
  labels:
    podlock.kubewarden.io/profile: does-not-exist
spec:
  restartPolicy: Always
  containers:
  - name: probe
    image: $IMAGE
    imagePullPolicy: Never
    command: ["sh", "-c", "while true; do sleep 2; done"]
YAML
kubectl apply -f /tmp/podlock-smoke-missing.yaml >/dev/null
set +e
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/missing -n "$NS" --timeout=30s
MISSING_WAIT_RC=$?
set -e
kubectl get pod missing -n "$NS" -o yaml > "$ARTIFACTS_DIR/missing-pod.yaml" || true
[ "$MISSING_WAIT_RC" -ne 0 ] || fail "missing profile unexpectedly reached Running"
echo "PODLOCK_UPSTREAM_SMOKE PASS abi=$ABI denied_rc=$DENIED_RC missing_wait_rc=$MISSING_WAIT_RC"
