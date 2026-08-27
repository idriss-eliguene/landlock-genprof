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

# Resolve the stable Pod UID before treatment restart; CRI PID is not used as
# an identity source because it is commonly zero after exit.
kubectl get pod "$POD" -n "$NS" -o jsonpath='{.metadata.uid}' > "$ARTIFACTS_DIR/target-pod-uid.txt"

# Diagnostic-only tracing is deliberately started before the certification
# apply creates/restarts the treatment container.  The tracer emits an
# explicit readiness marker and retains both broad failed-syscall exits and
# high-fidelity clone/clone3 exits for retrospective PID correlation.
# bpftrace 0.20.2 does not provide a portable cgroupid builtin.  Keep the
# backend keyed by PID/TID; the host watcher supplies authoritative cgroups.
TRACE_EXPR='BEGIN { printf("TRACE_READY\n"); } tracepoint:raw_syscalls:sys_exit /args->ret < 0/ { printf("ts=%llu pid=%d tid=%d syscall=%d retval=%d comm=%s\n", nsecs, pid, tid, args->id, args->ret, comm); } tracepoint:syscalls:sys_exit_clone { printf("ts=%llu clone pid=%d tid=%d retval=%d comm=%s\n", nsecs, pid, tid, args->ret, comm); } tracepoint:syscalls:sys_exit_clone3 { printf("ts=%llu clone3 pid=%d tid=%d retval=%d comm=%s\n", nsecs, pid, tid, args->ret, comm); }'
date -u +%FT%TZ > "$ARTIFACTS_DIR/bpftrace-start.txt"
bpftrace --version > "$ARTIFACTS_DIR/bpftrace-version.txt" 2> "$ARTIFACTS_DIR/bpftrace-version.err" || true
sudo bpftrace -l 'tracepoint:syscalls:sys_exit_clone*' > "$ARTIFACTS_DIR/bpftrace-probes.txt" 2> "$ARTIFACTS_DIR/bpftrace-probes.err" || true
printf 'uid=%s\ncapabilities=\n' "$(id -u)" > "$ARTIFACTS_DIR/tracer-environment.txt"
id >> "$ARTIFACTS_DIR/tracer-environment.txt" 2>&1 || true
capsh --print >> "$ARTIFACTS_DIR/tracer-environment.txt" 2>&1 || true
TRACE_PID=""
TRACE_READY=0
TRACE_START_TS="$(date -u +%FT%TZ%N)"
TRACE_READY_TS=""
: > "$ARTIFACTS_DIR/bpftrace.stdout"
: > "$ARTIFACTS_DIR/bpftrace.stderr"
if command -v bpftrace >/dev/null 2>&1; then
  sudo stdbuf -oL -eL bpftrace -e "$TRACE_EXPR" > "$ARTIFACTS_DIR/bpftrace.stdout" 2> "$ARTIFACTS_DIR/bpftrace.stderr" &
  TRACE_PID=$!
  for _ in $(seq 1 40); do
    if grep -q '^TRACE_READY' "$ARTIFACTS_DIR/bpftrace.stdout" 2>/dev/null; then
      TRACE_READY=1
      TRACE_READY_TS="$(date -u +%FT%TZ%N)"
      break
    fi
    kill -0 "$TRACE_PID" 2>/dev/null || break
    sleep 0.25
  done
fi
printf 'ready=%s pid=%s start=%s ready_ts=%s\n' "$TRACE_READY" "${TRACE_PID:-}" "$TRACE_START_TS" "${TRACE_READY_TS:-}" > "$ARTIFACTS_DIR/bpftrace-readiness.txt"
if [ "$TRACE_READY" -ne 1 ]; then
  printf 'TRACE_SMOKE=NOT_RUN\n' > "$ARTIFACTS_DIR/trace-smoke.txt"
  if [ -n "$TRACE_PID" ]; then kill "$TRACE_PID" >/dev/null 2>&1 || true; fi
  fail "trace readiness marker not observed"
else
  SMOKE_START_TS="$(date -u +%FT%TZ%N)"
  sh -c 'sleep 0.2' & SMOKE_PID=$!
  wait "$SMOKE_PID" >/dev/null 2>&1 || true
  sleep 0.5
  SMOKE_END_TS="$(date -u +%FT%TZ%N)"
  if grep -Eq "(clone|clone3) pid=${SMOKE_PID} .*ret=" "$ARTIFACTS_DIR/bpftrace.stdout"; then
    printf 'TRACE_SMOKE=PASS pid=%s start=%s end=%s\n' "$SMOKE_PID" "$SMOKE_START_TS" "$SMOKE_END_TS" > "$ARTIFACTS_DIR/trace-smoke.txt"
  else
    printf 'TRACE_SMOKE=FAIL pid=%s start=%s end=%s\n' "$SMOKE_PID" "$SMOKE_START_TS" "$SMOKE_END_TS" > "$ARTIFACTS_DIR/trace-smoke.txt"
    kill "$TRACE_PID" >/dev/null 2>&1 || true
    fail "trace smoke event not correlated"
  fi
fi

: > "$ARTIFACTS_DIR/target-task-history.txt"
: > "$ARTIFACTS_DIR/target-incarnations.txt"
(
  deadline=$((SECONDS + 180))
  uid="$(sed -n '1p' "$ARTIFACTS_DIR/target-pod-uid.txt")"
  declare -A seen=()
  while [ "$SECONDS" -lt "$deadline" ]; do
    now="$(date -u +%FT%TZ%N)"
    while IFS= read -r procs; do
      cgroup="${procs%/cgroup.procs}"
      case "$cgroup" in *"$uid"*) ;; *) continue ;; esac
      while read -r pid; do
        [[ "$pid" =~ ^[1-9][0-9]*$ ]] || continue
        key="$cgroup:$pid"
        if [ -z "${seen[$key]+x}" ]; then
          seen[$key]=1
          comm="$(tr -d '\0' < "/proc/$pid/comm" 2>/dev/null || true)"
          exe="$(readlink "/proc/$pid/exe" 2>/dev/null || true)"
          cgid="$(stat -c %i "$cgroup" 2>/dev/null || true)"
          printf 'ts=%s pod_uid=%s cgroup=%s cgroup_id=%s pid=%s comm=%s exe=%s\n' "$now" "$uid" "$cgroup" "$cgid" "$pid" "$comm" "$exe" >> "$ARTIFACTS_DIR/target-task-history.txt"
          { echo "# ts=$now pid=$pid cgroup=$cgroup"; for f in status stat cgroup cmdline; do [ -r "/proc/$pid/$f" ] && { echo "## $f"; tr '\0' ' ' < "/proc/$pid/$f"; echo; }; done; } > "$ARTIFACTS_DIR/target-task-$pid.txt" 2>/dev/null || true
          printf 'ts=%s pod_uid=%s cgroup=%s cgroup_id=%s pid=%s comm=%s exe=%s\n' "$now" "$uid" "$cgroup" "$cgid" "$pid" "$comm" "$exe" >> "$ARTIFACTS_DIR/target-incarnations.txt"
        fi
      done < "$procs" 2>/dev/null
    done < <(find /sys/fs/cgroup -type f -name cgroup.procs 2>/dev/null)
    sleep 0.1
  done
) &
IDENTITY_PID=$!

LANDLOCK_CERTIFICATION_PROPOSAL="$POD" LANDLOCK_CERTIFICATION_NAMESPACE="$NS" LANDLOCK_CERTIFICATION_OUTPUT="$ARTIFACTS_DIR/apply.txt" \
  go test ./cmd/landlock-genprof -run '^TestCertificationApply$' -count=1
cat "$ARTIFACTS_DIR/apply.txt"
grep -F 'This will apply 3 artifact(s)' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "pairwise plan did not select exactly three artifacts"
grep -F '  - PodLock' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "PodLock missing from pairwise plan"
grep -F '  - SPO SeccompProfile' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "SeccompProfile missing from pairwise plan"
grep -F '  - Patched Manifest' "$ARTIFACTS_DIR/apply.txt" >/dev/null || fail "Patched Manifest missing from pairwise plan"

# Disposable differential control: identical workload/image with PodLock but
# without the governed SPO binding. This changes no approved artifact.
CONTROL_POD="${POD}-control"
sed "s/name: $POD/name: $CONTROL_POD/; /app: podlock-spo-target/a\\    podlock.kubewarden.io/profile: podlock-spo-golden" \
  /tmp/podlock-spo.yaml > /tmp/podlock-spo-control.yaml
kubectl apply -f /tmp/podlock-spo-control.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Running "pod/$CONTROL_POD" -n "$NS" --timeout=90s || true
kubectl get pod "$CONTROL_POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/control-pod.yaml" || true
kubectl describe pod "$CONTROL_POD" -n "$NS" > "$ARTIFACTS_DIR/control-pod.describe" || true
kubectl logs "$CONTROL_POD" -n "$NS" -c probe --timestamps > "$ARTIFACTS_DIR/control-logs.txt" 2>&1 || true
kubectl exec "$CONTROL_POD" -n "$NS" -c probe -- sh -c 'cat /proc/1/status; ulimit -u' > "$ARTIFACTS_DIR/control-proc-status.txt" 2>&1 || true
  kubectl get pod "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/treatment-pod-before-wait.yaml" || true
# Keep tracing through treatment startup/failure.  Do not let the readiness
# wait's status terminate the diagnostic collection before CRI evidence is
# persisted.
set +e
kubectl wait --for=jsonpath='{.status.containerStatuses[0].state.running}' "pod/$POD" -n "$NS" --timeout=90s
WAIT_RC=$?
set -e
sleep 30
if [ -n "${IDENTITY_PID:-}" ]; then
  kill "$IDENTITY_PID" >/dev/null 2>&1 || true
  wait "$IDENTITY_PID" >/dev/null 2>&1 || true
fi

if [ -n "$TRACE_PID" ]; then
  kill "$TRACE_PID" >/dev/null 2>&1 || true
  set +e
  wait "$TRACE_PID" >/dev/null 2>&1
  TRACE_RC=$?
  set -e
else
  TRACE_RC=127
fi
printf 'ready=%s exit=%s end=%s\n' "${TRACE_READY}" "$TRACE_RC" "$(date -u +%FT%TZ)" > "$ARTIFACTS_DIR/bpftrace.status"
cp "$ARTIFACTS_DIR/bpftrace.stdout" "$ARTIFACTS_DIR/trace-raw.txt" 2>/dev/null || true
cp "$ARTIFACTS_DIR/trace-raw.txt" "$ARTIFACTS_DIR/thread-syscalls.txt" 2>/dev/null || true
awk -F'pid=' '{if (NF > 1) {split($2,a," "); if (a[1] != "0") print a[1]}}' "$ARTIFACTS_DIR/target-task-history.txt" 2>/dev/null | sort -u > "$ARTIFACTS_DIR/target-pids.txt" || true
: > "$ARTIFACTS_DIR/target-cgroup-ids.txt"
awk -F'cgroup_id=' '{if (NF > 1) {split($2,a," "); if (a[1] != "") print a[1]}}' "$ARTIFACTS_DIR/target-task-history.txt" | sort -u > "$ARTIFACTS_DIR/target-cgroup-ids.txt" || true
: > "$ARTIFACTS_DIR/trace-target-correlated.txt"
while read -r TARGET_PID; do
  [ -n "$TARGET_PID" ] || continue
  grep -E "(pid=${TARGET_PID} |tid=${TARGET_PID} )" "$ARTIFACTS_DIR/trace-raw.txt" >> "$ARTIFACTS_DIR/trace-target-correlated.txt" 2>/dev/null || true
done < "$ARTIFACTS_DIR/target-pids.txt"
while read -r CGID; do
  [ -n "$CGID" ] || continue
  grep -E "cgroup=${CGID} " "$ARTIFACTS_DIR/trace-raw.txt" >> "$ARTIFACTS_DIR/trace-target-correlated.txt" 2>/dev/null || true
done < "$ARTIFACTS_DIR/target-cgroup-ids.txt"
sort -u "$ARTIFACTS_DIR/trace-target-correlated.txt" -o "$ARTIFACTS_DIR/trace-target-correlated.txt" 2>/dev/null || true
printf 'tracer_start=%s\ntrace_ready=%s\ntreatment_wait_rc=%s\ntreatment_create=%s\ntrace_stop=%s\n' "$TRACE_START_TS" "${TRACE_READY_TS:-}" "$WAIT_RC" "$(date -u +%FT%TZ%N)" "$(date -u +%FT%TZ%N)" > "$ARTIFACTS_DIR/timing.txt"
cp "$ARTIFACTS_DIR/target-task-history.txt" "$ARTIFACTS_DIR/target-identity.txt" 2>/dev/null || true

# Keep startup as an explicit boundary so an unstarted container cannot be
# mistaken for a successful pairwise application. The always-run workflow
# diagnostics capture the runtime failure before cleanup.
kubectl wait --for=jsonpath='{.status.containerStatuses[0].state.running}' "pod/$POD" -n "$NS" --timeout=90s
echo "PAIRWISE_SPO_CERTIFICATION_APPLY PASS abi=$ABI"
