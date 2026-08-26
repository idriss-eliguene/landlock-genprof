#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NS="${NS:-landlock-genprof-podlock-spo-golden}"
POD="${POD:-podlock-spo-golden}"
IMAGE="${IMAGE:-landlock-genprof/fsprobe:podlock-spo-golden}"
BINARY=/probe/fsprobe
SECCOMP_PROBE=/probe/seccomp-probe
DENIED_PATH=/etc/shadow
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/podlock-spo-golden-artifacts}"
SPO_RECORDING_NAMESPACE="${SPO_RECORDING_NAMESPACE:-landlock-genprof-merged}"
SPO_RECORDING="${SPO_RECORDING:-lgmerged}"
SPO_PROFILE="${SPO_PROFILE:-lgmerged-tools}"
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
    command: ["/probe/fsprobe", "--loop"]
YAML
kubectl apply -f /tmp/podlock-spo.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/$POD -n "$NS" --timeout=180s
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" "$DENIED_PATH" | tee "$ARTIFACTS_DIR/control-denied.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/control-denied.txt" || fail "control access failed"
set +e
kubectl exec -n "$NS" "$POD" -c probe -- "$SECCOMP_PROBE" getpriority | tee "$ARTIFACTS_DIR/control-getpriority.txt"
CONTROL_SYSCALL_RC=${PIPESTATUS[0]}
set -e
[ "$CONTROL_SYSCALL_RC" -eq 0 ] || fail "control syscall access failed"
grep -F 'syscall=getpriority' "$ARTIFACTS_DIR/control-getpriority.txt" | grep -F 'errno=0 status=success' >/dev/null || fail "control syscall success contract failed"

kubectl landlock-genprof trace --pod "$POD" -n "$NS" --container probe --binary "$BINARY" --duration 30s --history --seccomp-source=spo --spo-import-mode=merged-provenance --spo-recording-namespace "$SPO_RECORDING_NAMESPACE" --spo-recording "$SPO_RECORDING" --spo-profile "$SPO_PROFILE" --out "$ARTIFACTS_DIR/generated-profile.yaml" --events-out "$ARTIFACTS_DIR/events.json" > "$ARTIFACTS_DIR/trace.txt" 2>&1 &
TRACE_PID=$!
sleep 5
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt | tee "$ARTIFACTS_DIR/observed-allowed.txt"
kubectl exec -n "$NS" "$POD" -c probe -- "$SECCOMP_PROBE" getpid | tee "$ARTIFACTS_DIR/observed-getpid.txt"
wait "$TRACE_PID"
grep -F 'SPO merged derived policy' "$ARTIFACTS_DIR/trace.txt" >/dev/null || fail "Seccomp source was not authoritative SPO"
kubectl get securityprofileproposal "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/proposal.yaml"
kubectl landlock-genprof review "$POD" -n "$NS" | tee "$ARTIFACTS_DIR/review.txt"
DIGEST="$(awk '/^Candidate digest: /{print $3; exit}' "$ARTIFACTS_DIR/review.txt")"
[ -n "$DIGEST" ] || fail "proposal digest missing"
kubectl landlock-genprof approve "$POD" -n "$NS" --expected-digest "$DIGEST" --reason podlock-spo-golden >/dev/null
set +e
kubectl landlock-genprof apply-proposal "$POD" -n "$NS" --yes --restart > "$ARTIFACTS_DIR/guard-apply.txt" 2>&1
APPLY_RC=$?
set -e
cat "$ARTIFACTS_DIR/guard-apply.txt"
[ "$APPLY_RC" -ne 0 ] || fail "production guard unexpectedly allowed pairwise composition"
grep -qi 'composition is unsupported\|runtime compatibility is unproven' "$ARTIFACTS_DIR/guard-apply.txt" || fail "production guard diagnostic missing"

LANDLOCK_CERTIFICATION_PROPOSAL="$POD" LANDLOCK_CERTIFICATION_NAMESPACE="$NS" LANDLOCK_CERTIFICATION_OUTPUT="$ARTIFACTS_DIR/apply.txt" \
  go test ./cmd/landlock-genprof -run '^TestCertificationApply$' -count=1
cat "$ARTIFACTS_DIR/apply.txt"
grep -F 'This will apply 3 artifact(s)' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "pairwise plan did not select exactly three artifacts"
grep -F '  - PodLock' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "PodLock missing from pairwise plan"
grep -F '  - SPO SeccompProfile' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "SeccompProfile missing from pairwise plan"
grep -F '  - Patched Manifest' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "Patched Manifest missing from pairwise plan"

# Keep startup as an explicit boundary so an unstarted container cannot be
# mistaken for a successful pairwise application. The always-run workflow
# diagnostics capture the runtime failure before cleanup.
kubectl wait --for=jsonpath='{.status.containerStatuses[0].state.running}' "pod/$POD" -n "$NS" --timeout=90s
echo "PAIRWISE_SPO_CERTIFICATION_APPLY PASS abi=$ABI"
