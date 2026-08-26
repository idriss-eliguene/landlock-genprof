#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/podlock-golden-artifacts}"
NS="${NS:-landlock-genprof-podlock-golden}"
POD="${POD:-podlock-golden}"
IMAGE="${IMAGE:-landlock-genprof/fsprobe:governed-e2e}"
BINARY=/probe/fsprobe
# PodLock/landlock records the parent directory needed for an observed file.
# Keep the negative target outside the learned /data tree so the generated
# profile cannot authorize it as a consequence of allowing /data.
DENIED_PATH=/etc/shadow
mkdir -p "$ARTIFACTS_DIR"
fail() { echo "ERROR: $*" >&2; exit 1; }

echo "[preflight] node and Landlock"
kubectl wait --for=condition=Ready node --all --timeout=300s
kubectl get crd landlockprofiles.podlock.kubewarden.io >/dev/null \
  || fail "PodLock LandlockProfile CRD is unavailable"
kubectl -n podlock rollout status deployment/podlock-controller --timeout=300s
kubectl -n podlock rollout status daemonset/podlock-nri-plugin --timeout=300s
gcc -O2 -Wall -Wextra "$ROOT_DIR/test/e2e/landlock-abi-probe.c" -o /tmp/landlock-abi-probe
/tmp/landlock-abi-probe | tee "$ARTIFACTS_DIR/landlock-abi.txt"
ABI="$(sed -n 's/^LANDLOCK_ABI=//p' "$ARTIFACTS_DIR/landlock-abi.txt")"
[ -n "$ABI" ] && [ "$ABI" -ge 3 ] || fail "Landlock ABI < 3"

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
cat > /tmp/podlock-golden.yaml <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: $POD
  namespace: $NS
spec:
  restartPolicy: Always
  containers:
  - name: probe
    image: $IMAGE
    imagePullPolicy: Never
    command: ["sh", "-c", "while true; do sleep 2; done"]
YAML
kubectl apply -f /tmp/podlock-golden.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/$POD -n "$NS" --timeout=180s

echo "[control] forbidden candidate is accessible without PodLock"
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" "$DENIED_PATH" \
  | tee "$ARTIFACTS_DIR/control-denied.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/control-denied.txt" \
  || fail "unconfined control cannot access forbidden candidate"

echo "[product] trace -> generated PodLock proposal"
kubectl landlock-genprof trace --pod "$POD" -n "$NS" --container probe --binary "$BINARY" \
  --duration 30s --history --out "$ARTIFACTS_DIR/generated-profile.yaml" \
  --events-out "$ARTIFACTS_DIR/events.json" > "$ARTIFACTS_DIR/trace.txt" 2>&1 &
TRACE_PID=$!
sleep 5
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt \
  | tee "$ARTIFACTS_DIR/observed-allowed.txt"
wait "$TRACE_PID"
kubectl get securityprofileproposal "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/proposal.yaml"
kubectl landlock-genprof review "$POD" -n "$NS" | tee "$ARTIFACTS_DIR/review.txt"
DIGEST="$(awk '/^Candidate digest: /{print $3; exit}' "$ARTIFACTS_DIR/review.txt")"
[ -n "$DIGEST" ] || fail "generated proposal has no digest"
kubectl landlock-genprof approve "$POD" -n "$NS" --expected-digest "$DIGEST" \
  --reason podlock-golden | tee "$ARTIFACTS_DIR/approve.txt"
kubectl get securityprofileproposal "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/proposal-approved.yaml"
APPROVED_DIGEST="$(kubectl get securityprofileproposal "$POD" -n "$NS" -o jsonpath='{.status.approvedCandidateDigest}')"
[ "$APPROVED_DIGEST" = "$DIGEST" ] || fail "approved proposal digest mismatch"
kubectl landlock-genprof apply-proposal "$POD" -n "$NS" --yes --restart \
  --skip=spo-seccompprofile \
  | tee "$ARTIFACTS_DIR/apply.txt"

kubectl get landlockprofile "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/generated-podlock.yaml"
kubectl get landlockprofile "$POD" -n "$NS" -o jsonpath='{.metadata.uid}' \
  | tee "$ARTIFACTS_DIR/podlock-uid.txt"
kubectl get securityprofileproposal "$POD" -n "$NS" -o jsonpath='{.spec.podLock}' \
  > "$ARTIFACTS_DIR/approved-podlock.yaml"
kubectl create --dry-run=client -f "$ARTIFACTS_DIR/generated-profile.yaml" -o json \
  | jq -S 'del(.metadata)' > "$ARTIFACTS_DIR/traced-profile-normalized.json"
kubectl create --dry-run=client -f "$ARTIFACTS_DIR/approved-podlock.yaml" -o json \
  | jq -S 'del(.metadata)' > "$ARTIFACTS_DIR/approved-profile-normalized.json"
cmp -s "$ARTIFACTS_DIR/traced-profile-normalized.json" "$ARTIFACTS_DIR/approved-profile-normalized.json" \
  || fail "approved proposal PodLock differs from traced generated profile"
kubectl get landlockprofile "$POD" -n "$NS" -o json \
  | jq -S 'del(.metadata)' > "$ARTIFACTS_DIR/live-profile-normalized.json"
cmp -s "$ARTIFACTS_DIR/approved-profile-normalized.json" "$ARTIFACTS_DIR/live-profile-normalized.json" \
  || fail "live LandlockProfile differs from approved proposal artifact"
jq -e --arg path /data/allowed.txt \
  '.spec.profilesByContainer.probe["/probe/fsprobe"].readOnly | index($path) != null' \
  "$ARTIFACTS_DIR/live-profile-normalized.json" >/dev/null \
  || fail "generated profile does not allow the observed path"
jq -e --arg path "$DENIED_PATH" \
  '([.spec.profilesByContainer.probe["/probe/fsprobe"][]?[]?] | index($path)) == null' \
  "$ARTIFACTS_DIR/live-profile-normalized.json" >/dev/null \
  || fail "generated profile unexpectedly allows the forbidden path"
kubectl get pod "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/protected-pod.yaml"
grep -F "podlock.kubewarden.io/profile: $POD" "$ARTIFACTS_DIR/protected-pod.yaml" \
  || fail "protected workload is not bound to generated PodLock"
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/$POD -n "$NS" --timeout=240s

echo "[protected] allowed path succeeds"
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt \
  | tee "$ARTIFACTS_DIR/protected-allowed.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/protected-allowed.txt" \
  || fail "generated PodLock denied observed allowed path"

echo "[protected] forbidden candidate is denied by Landlock"
set +e
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" "$DENIED_PATH" \
  > "$ARTIFACTS_DIR/protected-denied.txt" 2>&1
DENIED_RC=$?
set -e
cat "$ARTIFACTS_DIR/protected-denied.txt"
grep -F 'result=failure errno=13' "$ARTIFACTS_DIR/protected-denied.txt" \
  || fail "protected forbidden path was not denied with EACCES"

kubectl logs daemonset/podlock-nri-plugin -n podlock -c nri --tail=2000 \
  > "$ARTIFACTS_DIR/nri.log" 2>&1 || true
grep -qi 'landlock profile applied' "$ARTIFACTS_DIR/protected-denied.txt" \
  || grep -qi 'mutation requested' "$ARTIFACTS_DIR/nri.log" \
  || fail "no PodLock controller/NRI activation evidence"

echo "[substitution] wrong profile reference fails closed"
kubectl label pod "$POD" -n "$NS" podlock.kubewarden.io/profile=does-not-exist --overwrite >/dev/null
kubectl delete pod "$POD" -n "$NS" --wait=true --timeout=120s >/dev/null
awk '/^spec:/{print "  labels:"; print "    podlock.kubewarden.io/profile: does-not-exist"} {print}' \
  /tmp/podlock-golden.yaml > /tmp/podlock-golden-wrong.yaml
kubectl apply -f /tmp/podlock-golden-wrong.yaml >/dev/null
if kubectl wait --for=jsonpath='{.status.phase}'=Running pod/$POD -n "$NS" --timeout=30s; then
  fail "workload ran after substitution to missing PodLock"
fi

kubectl delete namespace "$NS" --ignore-not-found >/dev/null || true
echo "REAL_PODLOCK_GOLDEN PASS abi=$ABI profile=$POD denied_rc=$DENIED_RC"
