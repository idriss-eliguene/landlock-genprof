#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/governed-artifacts}"
NS="${NS:-podlock-governed}"
POD="${POD:-governed-probe}"
IMAGE="${IMAGE:-landlock-genprof/fsprobe:governed-e2e}"
BINARY="/probe/fsprobe"
mkdir -p "$ARTIFACTS_DIR"
fail() { echo "ERROR: $*" >&2; exit 1; }

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

cat > /tmp/control.yaml <<EOF
apiVersion: v1
kind: Pod
metadata: {name: control-probe, namespace: $NS}
spec:
  restartPolicy: Never
  containers:
  - name: probe
    image: $IMAGE
    command: ["$BINARY", "/data/allowed.txt"]
    imagePullPolicy: Never
EOF
kubectl apply -f /tmp/control.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Succeeded pod/control-probe -n "$NS" --timeout=180s
kubectl logs pod/control-probe -n "$NS" | tee "$ARTIFACTS_DIR/control-allowed.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/control-allowed.txt" || fail "control allowed read failed"

sed 's/control-probe/control-denied/g; s#/data/allowed.txt#/data/denied.txt#g' /tmp/control.yaml > /tmp/control-denied.yaml
kubectl apply -f /tmp/control-denied.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Succeeded pod/control-denied -n "$NS" --timeout=180s
kubectl logs pod/control-denied -n "$NS" | tee "$ARTIFACTS_DIR/control-denied.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/control-denied.txt" || fail "control denied-candidate read failed"

kubectl get pod control-probe -n "$NS" -o yaml > "$ARTIFACTS_DIR/control-pod.yaml"
sed "s/control-probe/$POD/g; s#/data/allowed.txt#/data/allowed.txt#g" /tmp/control.yaml > /tmp/governed.yaml
kubectl delete pod control-probe -n "$NS" --ignore-not-found >/dev/null
kubectl delete pod control-denied -n "$NS" --ignore-not-found >/dev/null
kubectl apply -f /tmp/governed.yaml >/dev/null

CLI="kubectl landlock-genprof"
$CLI trace --pod "$POD" -n "$NS" --container probe --binary "$BINARY" --duration 30s \
  --events-out "$ARTIFACTS_DIR/events.json" --out "$ARTIFACTS_DIR/profile.yaml" \
  > "$ARTIFACTS_DIR/trace.txt" 2>&1 &
TRACE_PID=$!
sleep 5
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt > "$ARTIFACTS_DIR/trace-read.txt"
wait "$TRACE_PID"

kubectl get securityprofileproposal "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/proposal-before.yaml"
jq -n --arg d "$(awk '/^Candidate digest: /{print $3; exit}' "$ARTIFACTS_DIR/review.txt" 2>/dev/null || true)" '{digest:$d}' > /dev/null
$CLI review "$POD" -n "$NS" | tee "$ARTIFACTS_DIR/review.txt"
DIGEST="$(awk '/^Candidate digest: /{print $3; exit}' "$ARTIFACTS_DIR/review.txt")"
[ -n "$DIGEST" ] || fail "review produced no CandidateDigest"
$CLI approve "$POD" -n "$NS" --expected-digest "$DIGEST" --reason governed-podlock | tee "$ARTIFACTS_DIR/approve.txt"
APPROVED="$(kubectl get securityprofileproposal "$POD" -n "$NS" -o jsonpath='{.status.approvedCandidateDigest}')"
[ "$APPROVED" = "$DIGEST" ] || fail "approved digest mismatch"
kubectl get securityprofileproposal "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/proposal-approved.yaml"

$CLI apply-proposal "$POD" -n "$NS" --yes --restart | tee "$ARTIFACTS_DIR/apply.txt"
kubectl get landlockprofile "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/live-profile.yaml"
kubectl get pod "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/governed-pod.yaml"
grep -F "podlock.kubewarden.io/profile: $POD" "$ARTIFACTS_DIR/governed-pod.yaml" || fail "binding label missing"
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/"$POD" -n "$NS" --timeout=240s

kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt | tee "$ARTIFACTS_DIR/governed-allowed.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/governed-allowed.txt" || fail "governed allowed read failed"
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/denied.txt | tee "$ARTIFACTS_DIR/governed-denied.txt" || true
grep -F 'result=failure errno=13' "$ARTIFACTS_DIR/governed-denied.txt" || fail "governed denial was not EACCES"

kubectl get pods -n "$NS" -o yaml > "$ARTIFACTS_DIR/pods.yaml"
kubectl logs daemonset/podlock-nri-plugin -n podlock -c nri > "$ARTIFACTS_DIR/nri.log" 2>&1 || true
sudo find /var/run/podlock -type f -name profile.json -print -exec cp {} "$ARTIFACTS_DIR/profile.json" \; || true
grep -R "landlock profile applied" "$ARTIFACTS_DIR/nri.log" || fail "seal activation evidence missing"
