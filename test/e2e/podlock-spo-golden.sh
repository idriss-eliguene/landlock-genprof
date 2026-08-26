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
set +e
kubectl exec -n "$NS" "$POD" -c probe -- /usr/local/bin/seccomp-probe getpriority | tee "$ARTIFACTS_DIR/control-getpriority.txt"
CONTROL_SYSCALL_RC=${PIPESTATUS[0]}
set -e
[ "$CONTROL_SYSCALL_RC" -eq 0 ] || fail "control syscall access failed"
grep -F 'syscall=getpriority' "$ARTIFACTS_DIR/control-getpriority.txt" | grep -F 'errno=0 status=success' >/dev/null || fail "control syscall success contract failed"

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
set +e
kubectl landlock-genprof apply-proposal "$POD" -n "$NS" --yes --restart > "$ARTIFACTS_DIR/apply.txt" 2>&1
APPLY_RC=$?
set -e
cat "$ARTIFACTS_DIR/apply.txt"
[ "$APPLY_RC" -ne 0 ] || fail "unsupported PodLock+Seccomp composition was applied"
grep -F 'This will apply 3 artifact(s)' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "pairwise plan did not select exactly three artifacts"
grep -F '  - PodLock' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "PodLock missing from pairwise plan"
grep -F '  - SPO SeccompProfile' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "SeccompProfile missing from pairwise plan"
grep -F '  - Patched Manifest' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "Patched Manifest missing from pairwise plan"
grep -qi 'composition is unsupported\|runtime compatibility is unproven' "$ARTIFACTS_DIR/apply.txt" || fail "missing fail-closed composition diagnostic"
if kubectl get landlockprofile "$POD" -n "$NS" >/dev/null 2>&1; then fail "LandlockProfile mutated before composition rejection"; fi
if kubectl get pod "$POD" -n "$NS" >/dev/null 2>&1 && kubectl get pod "$POD" -n "$NS" -o json | jq -e '.metadata.labels["podlock.kubewarden.io/profile"]' >/dev/null; then fail "workload binding mutated before composition rejection"; fi
echo "PAIRWISE_COMPOSITION_FAIL_CLOSED PASS abi=$ABI"
