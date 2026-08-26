#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NS="${NS:-landlock-genprof-podlock-spo-golden}"
POD="${POD:-podlock-spo-golden}"
IMAGE="${IMAGE:-landlock-genprof/fsprobe:podlock-spo-golden}"
BINARY=/probe/fsprobe
DENIED_PATH=/etc/shadow
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/podlock-spo-golden-artifacts}"
mkdir -p "$ARTIFACTS_DIR"
fail() { echo "ERROR: $*" >&2; exit 1; }

kubectl wait --for=condition=Ready node --all --timeout=300s
kubectl get crd landlockprofiles.podlock.kubewarden.io >/dev/null || fail "PodLock CRD unavailable"
kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io >/dev/null || fail "SPO CRD unavailable"
kubectl -n podlock rollout status deployment/podlock-controller --timeout=300s
kubectl -n podlock rollout status daemonset/podlock-nri-plugin --timeout=300s
kubectl -n security-profiles-operator rollout status deployment/security-profiles-operator --timeout=300s
kubectl -n security-profiles-operator patch spod spod --type=merge -p '{"spec":{"verbosity":1,"enricher":{"enableBpfRecorder":true}}}' >/dev/null
kubectl -n security-profiles-operator rollout status daemonset/spod --timeout=420s

gcc -O2 -Wall -Wextra "$ROOT_DIR/test/e2e/landlock-abi-probe.c" -o /tmp/landlock-abi-probe
/tmp/landlock-abi-probe | tee "$ARTIFACTS_DIR/landlock-abi.txt"
ABI="$(sed -n 's/^LANDLOCK_ABI=//p' "$ARTIFACTS_DIR/landlock-abi.txt")"
[ -n "$ABI" ] && [ "$ABI" -ge 3 ] || fail "Landlock ABI < 3"

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl label namespace "$NS" spo.x-k8s.io/enable-recording=true --overwrite >/dev/null
cat > /tmp/podlock-spo.yaml <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: $POD
  namespace: $NS
  labels:
    app: podlock-spo-target
spec:
  restartPolicy: Always
  containers:
  - name: probe
    image: $IMAGE
    imagePullPolicy: Never
    command: ["sh", "-c", "while true; do /probe/fsprobe /data/allowed.txt >/dev/null; /usr/local/bin/seccomp-probe getpid >/dev/null; sleep 2; done"]
YAML
kubectl apply -f /tmp/podlock-spo.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/$POD -n "$NS" --timeout=180s
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" "$DENIED_PATH" | tee "$ARTIFACTS_DIR/control-denied.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/control-denied.txt" || fail "control access failed"
kubectl exec -n "$NS" "$POD" -c probe -- /usr/local/bin/seccomp-probe getpriority | tee "$ARTIFACTS_DIR/control-getpriority.txt"
grep -F 'result=0 errno=0' "$ARTIFACTS_DIR/control-getpriority.txt" || fail "control syscall access failed"

kubectl landlock-genprof trace --pod "$POD" -n "$NS" --container probe --binary "$BINARY" --duration 30s --history --out "$ARTIFACTS_DIR/generated-profile.yaml" --events-out "$ARTIFACTS_DIR/events.json" > "$ARTIFACTS_DIR/trace.txt" 2>&1 &
TRACE_PID=$!
sleep 5
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt | tee "$ARTIFACTS_DIR/observed-allowed.txt"
kubectl exec -n "$NS" "$POD" -c probe -- /usr/local/bin/seccomp-probe getpid | tee "$ARTIFACTS_DIR/observed-getpid.txt"
wait "$TRACE_PID"
kubectl get securityprofileproposal "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/proposal.yaml"
kubectl landlock-genprof review "$POD" -n "$NS" | tee "$ARTIFACTS_DIR/review.txt"
DIGEST="$(awk '/^Candidate digest: /{print $3; exit}' "$ARTIFACTS_DIR/review.txt")"
[ -n "$DIGEST" ] || fail "proposal digest missing"
kubectl landlock-genprof approve "$POD" -n "$NS" --expected-digest "$DIGEST" --reason podlock-spo-golden >/dev/null
kubectl landlock-genprof apply-proposal "$POD" -n "$NS" --yes --restart | tee "$ARTIFACTS_DIR/apply.txt"
grep -F 'This will apply 3 artifact(s)' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "pairwise plan did not select exactly three artifacts"
grep -F '  - PodLock' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "PodLock missing from pairwise plan"
grep -F '  - SPO SeccompProfile' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "SeccompProfile missing from pairwise plan"
grep -F '  - Patched Manifest' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "Patched Manifest missing from pairwise plan"

kubectl get landlockprofile "$POD" -n "$NS" -o json > "$ARTIFACTS_DIR/live-podlock.json"
kubectl get seccompprofile "$(kubectl get securityprofileproposal "$POD" -n "$NS" -o jsonpath='{.spec.spoSeccompProfile}' | kubectl create --dry-run=client -f - -o jsonpath='{.metadata.name}')" -o json > "$ARTIFACTS_DIR/live-seccomp.json"
kubectl get pod "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/protected-pod.yaml"
grep -F "podlock.kubewarden.io/profile: $POD" "$ARTIFACTS_DIR/protected-pod.yaml" >/dev/null || fail "PodLock binding missing"
grep -F 'localhostProfile:' "$ARTIFACTS_DIR/protected-pod.yaml" >/dev/null || fail "Seccomp binding missing"
SECcomp_NAME="$(kubectl get seccompprofile -o json | jq -r --arg ns "$NS" --arg pod "$POD" '.items[] | select(.metadata.annotations["landlockgenprof.io/target-namespace"]==$ns and .metadata.annotations["landlockgenprof.io/target-pod"]==$pod) | .metadata.name' | head -1)"
[ -n "$SECcomp_NAME" ] || fail "cannot identify generated SeccompProfile provenance"
kubectl get seccompprofile "$SECcomp_NAME" -o jsonpath='{.status.localhostProfile}' | grep -F "operator/" >/dev/null || fail "SPO SeccompProfile is not reconciled"
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/$POD -n "$NS" --timeout=240s

kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt | tee "$ARTIFACTS_DIR/protected-allowed.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/protected-allowed.txt" || fail "Landlock allowed access failed"
set +e
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" "$DENIED_PATH" > "$ARTIFACTS_DIR/protected-denied.txt" 2>&1
set -e
grep -F 'result=failure errno=13' "$ARTIFACTS_DIR/protected-denied.txt" || fail "Landlock denial missing"
kubectl exec -n "$NS" "$POD" -c probe -- /usr/local/bin/seccomp-probe getpriority > "$ARTIFACTS_DIR/protected-getpriority.txt" 2>&1 || true
grep -Eq 'errno=13|Operation not permitted|result=-1' "$ARTIFACTS_DIR/protected-getpriority.txt" || fail "Seccomp denial not proven"
kubectl logs daemonset/podlock-nri-plugin -n podlock -c nri --tail=2000 > "$ARTIFACTS_DIR/nri.log" 2>&1 || true
grep -qi 'landlock profile applied\|mutation requested' "$ARTIFACTS_DIR/nri.log" || fail "PodLock application evidence missing"
set +e
SUBSTITUTION_OUTPUT="$(kubectl label pod "$POD" -n "$NS" podlock.kubewarden.io/profile=does-not-exist --overwrite 2>&1)"
SUBSTITUTION_RC=$?
set -e
[ "$SUBSTITUTION_RC" -ne 0 ] || fail "PodLock substitution unexpectedly succeeded"
echo "$SUBSTITUTION_OUTPUT" | grep -qi 'immutable' || fail "PodLock substitution lacked fail-closed evidence"
echo "REAL_PODLOCK_SPO_GOLDEN PASS abi=$ABI"
