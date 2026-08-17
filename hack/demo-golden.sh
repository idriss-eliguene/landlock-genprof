#!/usr/bin/env bash
# demo-golden.sh
# Orchestrates a non-destructive Golden E2E using the landlock-genprof CLI
# Requirements: kubectl configured to the target cluster, kubectl landlock-genprof plugin
# This script FAILS EARLY when required prerequisites are missing.

set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Default expected kubeconfig context (unset -> default; empty -> fail-closed)
# Use '-' expansion so an explicit empty override is preserved (must fail-closed).
EXPECTED_CONTEXT="${EXPECTED_CONTEXT-kind-landlock-genprof-e2e}"
if [ -z "${EXPECTED_CONTEXT}" ]; then
  echo "ERROR: EXPECTED_CONTEXT is empty; refusing to run" >&2
  exit 1
fi

echo "[stage] demo-golden starting"

# Prechecks ---------------------------------------------------------------
# support a --check-only mode which performs zero cluster mutations
CHECK_ONLY=0
if [ "${1:-}" = "--check-only" ]; then
  CHECK_ONLY=1
  echo "[mode] running in CHECK-ONLY (no cluster mutations)"
fi

echo "[check] kubectl..."
if ! command -v kubectl >/dev/null 2>&1; then
  echo "ERROR: kubectl not found" >&2
  exit 1
fi

# Canonical E2E CLI contract: kubectl plugin consumption only.
CLI_CMD=(kubectl landlock-genprof)
CLI_MODE="kubectl-plugin"
if ! command -v kubectl-landlock_genprof >/dev/null 2>&1; then
  echo "GOLDEN_KUBECTL_PLUGIN_NOT_AVAILABLE: kubectl-landlock_genprof not found on PATH" >&2
  exit 1
fi
PLUGIN_PATH="$(command -v kubectl-landlock_genprof)"
# Normalize PATH to existing unique dirs so kubectl plugin list is deterministic.
PATH_CLEAN=""
IFS=':' read -r -a PATH_PARTS <<< "$PATH"
for p in "${PATH_PARTS[@]}"; do
  [ -d "$p" ] || continue
  case ":$PATH_CLEAN:" in
    *":$p:"*) ;;
    *) PATH_CLEAN="${PATH_CLEAN:+$PATH_CLEAN:}$p" ;;
  esac
done
if ! kubectl landlock-genprof --help >/dev/null 2>&1; then
  echo "GOLDEN_KUBECTL_PLUGIN_NOT_AVAILABLE: kubectl landlock-genprof --help failed" >&2
  exit 1
fi
set +e
PATH="$PATH_CLEAN" kubectl plugin list >/dev/null 2>&1
_plugin_list_rc=$?
set -e
if [ "$_plugin_list_rc" -ne 0 ]; then
  echo "[warn] kubectl plugin list returned rc=$_plugin_list_rc; continuing because canonical plugin execution is checked separately"
fi
printf '[check] demo CLI mode=%s\n' "$CLI_MODE"
printf '[check] plugin path=%q\n' "$PLUGIN_PATH"

# cluster info
CUR_CTX=$(kubectl config current-context 2>/dev/null || true)
echo "[info] current-context: $CUR_CTX"
echo "[info] cluster-info:"; kubectl cluster-info || true
echo "[info] nodes:"; kubectl get nodes -o wide || true

# Safety: must run against E2E kind context only
if [ "$CUR_CTX" != "$EXPECTED_CONTEXT" ]; then
  echo "ERROR: current context '$CUR_CTX' != expected '$EXPECTED_CONTEXT' — refusing to run against an unexpected cluster" >&2
  exit 1
fi

# CRDs required for generation
echo "[check] CRDs: SecurityProfileProposal and TrainingHistory"
if ! kubectl get crd securityprofileproposals.landlockgenprof.io >/dev/null 2>&1; then
  echo "ERROR: SecurityProfileProposal CRD not found; apply deploy/crd-securityprofileproposal.yaml" >&2
  exit 1
fi
if ! kubectl get crd traininghistories.landlockgenprof.io >/dev/null 2>&1; then
  echo "ERROR: TrainingHistory CRD not found; apply deploy/crd-traininghistory.yaml" >&2
  exit 1
fi

# Inspektor Gadget presence (required for trace acquisition)
echo "[check] Inspektor Gadget presence"
if kubectl get ns gadget >/dev/null 2>&1 && kubectl get daemonset -n gadget gadget >/dev/null 2>&1; then
  echo "Inspektor Gadget appears deployed"
else
  echo "ERROR: Inspektor Gadget not deployed in cluster (kubectl gadget deploy). This demo requires it." >&2
  exit 1
fi

# Optional enforcement backends
SPO_PRESENT=0
PODLOCK_PRESENT=0
if kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io >/dev/null 2>&1; then
  SPO_PRESENT=1
  echo "Optional SPO detected: Seccomp enforcement tests will be attempted"
else
  echo "Optional SPO NOT detected: Seccomp enforcement tests will be SKIPPED"
fi
if kubectl get crd landlockprofiles.podlock.kubewarden.io >/dev/null 2>&1; then
  PODLOCK_PRESENT=1
  echo "Optional PodLock detected: Landlock enforcement tests will be attempted"
else
  echo "Optional PodLock NOT detected: Landlock enforcement tests will be SKIPPED"
fi

if [ "$CHECK_ONLY" -eq 1 ]; then
  echo "[check-only] performed non-mutating preflight checks"
  echo "[check-only] SPO_PRESENT=$SPO_PRESENT PODLOCK_PRESENT=$PODLOCK_PRESENT"
  echo "[check-only] done"
  exit 0
fi

# Deploy demo workloads --------------------------------------------------
echo "[stage] deploying workload and echo service"
# Ensure the demo namespace exists (create-if-not)
kubectl create ns landlock-genprof-e2e --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f "$ROOT_DIR/demo/golden/workload.yaml"
kubectl apply -f "$ROOT_DIR/demo/golden/echo-service.yaml"

echo "[wait] for nginx-demo pod readiness"
kubectl wait --for=condition=Ready pod/nginx-demo -n landlock-genprof-e2e --timeout=120s
kubectl wait --for=condition=Available deployment/echo-8080-deploy -n landlock-genprof-e2e --timeout=120s
kubectl wait --for=condition=Available deployment/echo-8081-deploy -n landlock-genprof-e2e --timeout=120s
kubectl wait --for=condition=Available deployment/echo-8082-deploy -n landlock-genprof-e2e --timeout=120s

# determine identities used by trace and record naming
NAMESPACE=landlock-genprof-e2e
EXPECTED_CONTEXT="kind-landlock-genprof-e2e"
POD=nginx-demo
# Trace and actions now target an auxiliary container with userland tools
CONTAINER=tools
# ACTION_CONTAINER is the sidecar that performs harness actions; same as CONTAINER
ACTION_CONTAINER=${CONTAINER}
BINARY=/usr/bin/curl
PROPOSAL_NAME=${POD}

# Compute TrainingHistory names per internal/history/store.go:
# v2: <container>-<basename(binary)>-<short-hash>
BASENAME=$(basename "$BINARY")
SHORT_HASH=$(printf '%s' "$BINARY" | sha256sum | awk '{print $1}' | cut -c1-8)
TH_V2_NAME="${CONTAINER}-${BASENAME}-${SHORT_HASH}"
TH_LEGACY_NAME="${CONTAINER}-${BASENAME}"

echo "[info] computed TrainingHistory names: v2=$TH_V2_NAME legacy=$TH_LEGACY_NAME"

# helper to read runsRecorded from whichever TrainingHistory object exists
get_runs_recorded() {
  if kubectl get traininghistory "$TH_V2_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
    kubectl get traininghistory "$TH_V2_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.runsRecorded}'
  elif kubectl get traininghistory "$TH_LEGACY_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
    kubectl get traininghistory "$TH_LEGACY_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.runsRecorded}'
  else
    echo "0"
  fi
}

# RUNS: three iterations -------------------------------------------------
# We run the tracer with --history so SaveWithMerge is exercised
# Increase duration to avoid attach/action races; three runs remain short but reliable.
DURATION=40s
# ensure artifacts dir exists so CI artifact collector can pick up trace logs
ARTIFACTS_DIR="$ROOT_DIR/artifacts"
mkdir -p "$ARTIFACTS_DIR"

# Tools container executable readiness gate: ensure $BINARY is exec-able inside the tools container
wait_for_tools_exec() {
  local attempt=0
  local max=60
  while [ $attempt -lt $max ]; do
    if kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- test -x "$BINARY" >/dev/null 2>&1; then
      # verify version and basic file:// read
      if kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- "$BINARY" --version >/dev/null 2>&1 && kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- "$BINARY" -sS file:///etc/hosts -o /dev/null >/dev/null 2>&1; then
        echo "[check] tools executable ready: $BINARY"
        return 0
      fi
    fi
    attempt=$((attempt+1))
    sleep 1
  done
  echo "GOLDEN_TOOLS_EXEC_NOT_READY: $BINARY not exec-able in $POD:$CONTAINER" >&2
  echo "Pod describe:" >&2
  kubectl describe pod "$POD" -n "$NAMESPACE" >&2 || true
  kubectl get events -n "$NAMESPACE" --sort-by='.lastTimestamp' | sed -n '1,200p' >&2 || true
  return 1
}

# Run loop: runs 1..3 with robust action wrapper and finalizer
# Ensure tools executable is ready before starting the first run
if ! wait_for_tools_exec; then
  echo "Aborting: tools executable not ready" >&2
  exit 1
fi

for run in 1 2 3; do
  echo "[run] starting trace run #$run"

  TRACE_LOG="/tmp/trace-run-${run}.log"
  TRACE_EVENTS="/tmp/${POD}-events.json"
  echo "[trace] starting trace, logging to ${TRACE_LOG}"
  TRACE_CMD=("${CLI_CMD[@]}" trace --pod "$POD" -n "$NAMESPACE" --container "$CONTAINER" --binary "$BINARY" --duration "$DURATION" --history --out "${POD}-profile.yaml" --network-out "${POD}-networkpolicy.yaml" --seccomp-out "${POD}-seccomp.json" --capabilities-out "${POD}-capabilities.yaml" --security-context-out "${POD}-securitycontext.yaml" --patched-manifest-out "${POD}-patched.yaml" --seccomp-profile-out "${POD}-seccompprofile.yaml" --report-out "${POD}-report.md" "--events-out=${TRACE_EVENTS}")
  printf '[trace] argv:'; printf ' %q' "${TRACE_CMD[@]}"; printf '\n'
  "${TRACE_CMD[@]}" >"${TRACE_LOG}" 2>&1 &
  TRACE_PID=$!

  # idempotent finalizer: ensure TRACE_LOG and events preserved and TRACE_PID reaped
  preserve_trace_artifacts() {
    # avoid clobbering artifacts dir if not set
    ARTDIR=${ARTIFACTS_DIR:-"$ROOT_DIR/artifacts"}
    mkdir -p "$ARTDIR" || true
    if [ -n "${TRACE_PID:-}" ]; then
      if kill -0 "$TRACE_PID" >/dev/null 2>&1; then
        kill "$TRACE_PID" >/dev/null 2>&1 || true
        wait "$TRACE_PID" 2>/dev/null || true
      fi
    fi
    if [ -n "${TRACE_LOG:-}" ] && [ -f "$TRACE_LOG" ]; then
      cp "$TRACE_LOG" "$ARTDIR/trace-run-${run}.log" || true
    fi
    if [ -n "${TRACE_EVENTS:-}" ] && [ -f "$TRACE_EVENTS" ]; then
      cp "$TRACE_EVENTS" "$ARTDIR/${POD}-events-run-${run}.json" || true
    fi
    # Preserve current CR state for failure diagnosis.
    kubectl get traininghistory -n "$NAMESPACE" -o yaml >"$ARTDIR/traininghistory-current.yaml" 2>/dev/null || true
    kubectl get securityprofileproposal -n "$NAMESPACE" -o yaml >"$ARTDIR/securityprofileproposal-current.yaml" 2>/dev/null || true
  }
  # ensure finalizer runs on any exit
  trap preserve_trace_artifacts EXIT

  # action wrapper: runs action command, captures stdout/stderr/rc and enforces preservation on failure
  run_action() {
    local name="$1"; shift
    echo "[action] run=${run} name=${name} cmd=$*"
    set +e
    out="$("$@" 2>&1)"
    rc=$?
    set -e
    echo "[action] run=${run} name=${name} rc=${rc}"
    if [ $rc -ne 0 ]; then
      echo "ERROR: action ${name} failed rc=${rc}" >&2
      echo "==== ACTION ${name} OUTPUT START ====" >&2
      printf '%s
' "$out" >&2 || true
      echo "==== ACTION ${name} OUTPUT END ====" >&2
      # preserve trace artifacts and then exit non-zero
      preserve_trace_artifacts
      exit $rc
    fi
  }

  # give the trace a moment to attach
  sleep 3

  # perform controlled actions via wrapper
  run_action fs_common env NAMESPACE="$NAMESPACE" POD="$POD" CONTAINER="$CONTAINER" ACTION_CONTAINER="$ACTION_CONTAINER" CURL_BIN="$BINARY" bash "$ROOT_DIR/demo/golden/run-actions.sh" fs_common
  run_action net_common env NAMESPACE="$NAMESPACE" POD="$POD" CONTAINER="$CONTAINER" ACTION_CONTAINER="$ACTION_CONTAINER" CURL_BIN="$BINARY" bash "$ROOT_DIR/demo/golden/run-actions.sh" net_common
  # cap_common is best-effort
  run_action cap_common env NAMESPACE="$NAMESPACE" POD="$POD" CONTAINER="$CONTAINER" ACTION_CONTAINER="$ACTION_CONTAINER" CURL_BIN="$BINARY" bash "$ROOT_DIR/demo/golden/run-actions.sh" cap_common || true

  if [ "$run" -le 2 ]; then
    run_action fs_2_of_3 env NAMESPACE="$NAMESPACE" POD="$POD" CONTAINER="$CONTAINER" ACTION_CONTAINER="$ACTION_CONTAINER" CURL_BIN="$BINARY" bash "$ROOT_DIR/demo/golden/run-actions.sh" fs_2_of_3
    run_action net_2_of_3 env NAMESPACE="$NAMESPACE" POD="$POD" CONTAINER="$CONTAINER" ACTION_CONTAINER="$ACTION_CONTAINER" CURL_BIN="$BINARY" bash "$ROOT_DIR/demo/golden/run-actions.sh" net_2_of_3
  fi

  if [ "$run" -eq 1 ]; then
    run_action fs_1_of_3 env NAMESPACE="$NAMESPACE" POD="$POD" CONTAINER="$CONTAINER" ACTION_CONTAINER="$ACTION_CONTAINER" CURL_BIN="$BINARY" bash "$ROOT_DIR/demo/golden/run-actions.sh" fs_1_of_3
    run_action net_1_of_3 env NAMESPACE="$NAMESPACE" POD="$POD" CONTAINER="$CONTAINER" ACTION_CONTAINER="$ACTION_CONTAINER" CURL_BIN="$BINARY" bash "$ROOT_DIR/demo/golden/run-actions.sh" net_1_of_3
  fi

  run_action syscall_probe env NAMESPACE="$NAMESPACE" POD="$POD" CONTAINER="$CONTAINER" ACTION_CONTAINER="$ACTION_CONTAINER" CURL_BIN="$BINARY" bash "$ROOT_DIR/demo/golden/run-actions.sh" syscall_probe || true

  echo "[run] waiting for trace to finish (pid $TRACE_PID)"
  wait "$TRACE_PID"
  TRACE_RC=$?

  # preserve artifacts (trap also does this) — copy again to be safe
  preserve_trace_artifacts

  if [ "$TRACE_RC" -ne 0 ]; then
    echo "ERROR: trace run #$run failed with exit code $TRACE_RC" >&2
    echo "==== TRACE LOG ${TRACE_LOG} START ====" >&2
    if [ -f "$TRACE_LOG" ]; then
      sed -n '1,400p' "$TRACE_LOG" >&2 || true
    else
      echo "(trace log not found at $TRACE_LOG)" >&2
    fi
    echo "==== TRACE LOG ${TRACE_LOG} END ====" >&2
    exit $TRACE_RC
  fi

  # validate events JSON exists and is v2 and has events
  if [ ! -f "$TRACE_EVENTS" ]; then
    echo "ERROR: events file $TRACE_EVENTS missing" >&2
    preserve_trace_artifacts
    exit 1
  fi
  # quick JSON sanity check
  if ! jq -e '.version=="v2" and (.events|type=="array") and (.events|length>0)' "$TRACE_EVENTS" >/dev/null 2>&1; then
    echo "ERROR: events JSON $TRACE_EVENTS failed validation" >&2
    echo "==== EVENTS JSON ${TRACE_EVENTS} START ====" >&2
    sed -n '1,400p' "$TRACE_EVENTS" >&2 || true
    echo "==== EVENTS JSON ${TRACE_EVENTS} END ====" >&2
    preserve_trace_artifacts
    exit 1
  fi

  # verify TrainingHistory increment
  runs=$(get_runs_recorded)
  echo "[info] after run $run: TrainingHistory.runsRecorded = $runs"
  if [ "$runs" -ne "$run" ]; then
    echo "ERROR: expected runsRecorded == $run but got $runs" >&2
    echo "DEBUG: dumping TrainingHistory objects for diagnostics"
    kubectl get traininghistory -n "$NAMESPACE" -o yaml || true
    preserve_trace_artifacts
    exit 1
  fi

  # remove per-run trap, but preserve artifacts one last time; continue loop
  trap - EXIT
  preserve_trace_artifacts
done

# After three runs: fetch the TrainingHistory object and inspect controlled observations
TH_NAME_TO_USE=""
if kubectl get traininghistory "$TH_V2_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
  TH_NAME_TO_USE="$TH_V2_NAME"
else
  TH_NAME_TO_USE="$TH_LEGACY_NAME"
fi

echo "[stage] TrainingHistory persisted object: $TH_NAME_TO_USE"
kubectl get traininghistory "$TH_NAME_TO_USE" -n "$NAMESPACE" -o json > /tmp/th.json

# Print summary of seenInRuns for example paths/ports/capabilities/syscalls
echo "[inspect] filesystem seenInRuns entries:"
jq -r '.spec.filesystemAccesses[] | "path="+.path+" seenInRuns="+(.seenInRuns|tostring)' /tmp/th.json || true

echo "[inspect] network seenInRuns entries:"
jq -r '.spec.networkAccesses[] | "port="+(.port|tostring)+" dir="+.direction+" seenInRuns="+(.seenInRuns|tostring)' /tmp/th.json || true

echo "[inspect] syscall seenInRuns entries:"
jq -r '.spec.syscallAccesses[] | "name="+.name+" seenInRuns="+(.seenInRuns|tostring)' /tmp/th.json || true

echo "[inspect] capability seenInRuns entries:"
jq -r '.spec.capabilityAccesses[] | "name="+.name+" seenInRuns="+(.seenInRuns|tostring)' /tmp/th.json || true

# FAIL-CLOSED ASSERTIONS for controlled identities (deterministic fixture expectations)
# Filesystem identities
#
# NOTE: filesystemAccesses are aggregated per-directory, not per-file (see
# internal/landlock's aggregationDir / maxAggregationDepth=3 doc comment) —
# a file access collapses to filepath.Dir(path), truncated to 3 segments.
# So /etc/hosts -> /etc and /var/tmp/nginx-demo-2/marker ->
# /var/tmp/nginx-demo-2. These assertions must match the aggregated
# directory, not the original leaf file path (previously stale after the
# directory-aggregation feature landed — see internal/landlock/kernel.go).
FS_COMMON_SEEN=$(jq -r '.spec.filesystemAccesses[] | select(.path=="/etc") | .seenInRuns' /tmp/th.json || true)
FS_MED_SEEN=$(jq -r '.spec.filesystemAccesses[] | select(.path=="/var/tmp/nginx-demo-2") | .seenInRuns' /tmp/th.json || true)
FS_LOW_SEEN=$(jq -r '.spec.filesystemAccesses[] | select(.path|startswith("/srv/nginx/data")) | .seenInRuns' /tmp/th.json || true)

# Network identities
NET_COMMON_SEEN=$(jq -r '.spec.networkAccesses[] | select(.port==8080 and .direction=="egress") | .seenInRuns' /tmp/th.json || true)
NET_MED_SEEN=$(jq -r '.spec.networkAccesses[] | select(.port==8081 and .direction=="egress") | .seenInRuns' /tmp/th.json || true)
NET_LOW_SEEN=$(jq -r '.spec.networkAccesses[] | select(.port==8082 and .direction=="egress") | .seenInRuns' /tmp/th.json || true)

echo "[assert] FS common seenInRuns=$FS_COMMON_SEEN (expect 3)"
echo "[assert] FS med seenInRuns=$FS_MED_SEEN (expect 2)"
echo "[assert] FS low seenInRuns=$FS_LOW_SEEN (expect 1)"

echo "[assert] NET common seenInRuns=$NET_COMMON_SEEN (expect 3)"
echo "[assert] NET med seenInRuns=$NET_MED_SEEN (expect 2)"
echo "[assert] NET low seenInRuns=$NET_LOW_SEEN (expect 1)"

if [ "$FS_COMMON_SEEN" != "3" ]; then echo "ERROR: FS common seenInRuns != 3" >&2; exit 1; fi
if [ "$FS_MED_SEEN" != "2" ]; then echo "ERROR: FS med seenInRuns != 2" >&2; exit 1; fi
if [ "$FS_LOW_SEEN" != "1" ]; then echo "ERROR: FS low seenInRuns != 1" >&2; exit 1; fi

if [ "$NET_COMMON_SEEN" != "3" ]; then echo "ERROR: NET common seenInRuns != 3" >&2; exit 1; fi
if [ "$NET_MED_SEEN" != "2" ]; then echo "ERROR: NET med seenInRuns != 2" >&2; exit 1; fi
if [ "$NET_LOW_SEEN" != "1" ]; then echo "ERROR: NET low seenInRuns != 1" >&2; exit 1; fi

# CONFIDENCE assertions: check proposal.networkPolicy YAML contains trailing comments with expected confidence
jq -r '.spec.networkPolicy' /tmp/proposal.json > /tmp/proposal-network.yaml

echo "[inspect] network policy YAML snippet (for confidence):"
sed -n '1,200p' /tmp/proposal-network.yaml || true

grep -P "port: 8080.*#\s*confidence: high" /tmp/proposal-network.yaml >/dev/null || { echo "ERROR: expected port 8080 comment '# confidence: high' not found" >&2; exit 1; }
grep -P "port: 8081.*#\s*confidence: medium" /tmp/proposal-network.yaml >/dev/null || { echo "ERROR: expected port 8081 comment '# confidence: medium' not found" >&2; exit 1; }
grep -P "port: 8082.*#\s*confidence: low" /tmp/proposal-network.yaml >/dev/null || { echo "ERROR: expected port 8082 comment '# confidence: low' not found" >&2; exit 1; }

echo "[ok] Confidence annotations appear in proposal.networkPolicy for controlled ports"

# Fetch the SecurityProfileProposal persisted by trace
echo "[stage] fetching SecurityProfileProposal: $PROPOSAL_NAME"
kubectl get securityprofileproposal "$PROPOSAL_NAME" -n "$NAMESPACE" -o json > /tmp/proposal.json

# Assert four artifacts present in persisted Spec
for field in podLock networkPolicy spoSeccompProfile patchedManifest; do
  val=$(jq -r --arg f "$field" '.spec[$f] // ""' /tmp/proposal.json)
  if [ -z "$val" ] || [ "$val" = "null" ]; then
    echo "ERROR: proposal.spec.$field empty — acquisition domain may be missing" >&2
    exit 1
  else
    echo "[ok] proposal.spec.$field present"
  fi
done

# REVIEW: capture CandidateDigest from review command output
echo "[stage] review"
REVIEW_OUT=$("${CLI_CMD[@]}" review "$PROPOSAL_NAME" -n "$NAMESPACE" 2>&1 || true)
echo "$REVIEW_OUT" | sed -n '1,120p'
printf '%s\n' "$REVIEW_OUT" > "${ARTIFACTS_DIR}/review-output.txt"
# extract line 'Candidate digest: sha256:...'
CAND_DIGEST=$(printf '%s' "$REVIEW_OUT" | awk '/Candidate digest: /{print $3; exit}')
if [ -z "$CAND_DIGEST" ]; then
  echo "ERROR: could not extract CandidateDigest from review output" >&2
  exit 1
fi
# validate digest format (sha256:64hex)
if ! [[ "$CAND_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  echo "ERROR: candidate digest has unexpected format: $CAND_DIGEST" >&2
  exit 1
fi

echo "[info] CandidateDigest = $CAND_DIGEST"

# Negative test: attempt to approve with wrong digest (expect failure)
BAD_DIGEST="sha256:0000000000000000000000000000000000000000000000000000000000000000"
set +e
BAD_APPROVE_OUT=$("${CLI_CMD[@]}" approve "$PROPOSAL_NAME" -n "$NAMESPACE" --expected-digest "$BAD_DIGEST" --reason "negative-test" 2>&1)
BAD_APPROVE_RC=$?
set -e
printf '%s\n' "$BAD_APPROVE_OUT" > "${ARTIFACTS_DIR}/approve-wrong-digest.out"
if [ "$BAD_APPROVE_RC" -eq 0 ]; then
  echo "ERROR: approve succeeded with wrong digest (should have failed)" >&2
  exit 1
else
  echo "[ok] approve with wrong digest failed as expected"
fi

# Approve with correct digest
echo "[stage] approving with correct digest"
set +e
GOOD_APPROVE_OUT=$("${CLI_CMD[@]}" approve "$PROPOSAL_NAME" -n "$NAMESPACE" --expected-digest "$CAND_DIGEST" --reason "demo approval" 2>&1)
GOOD_APPROVE_RC=$?
set -e
printf '%s\n' "$GOOD_APPROVE_OUT" > "${ARTIFACTS_DIR}/approve-correct-digest.out"
if [ "$GOOD_APPROVE_RC" -ne 0 ]; then
  echo "ERROR: approve with correct digest failed" >&2
  printf '%s\n' "$GOOD_APPROVE_OUT" >&2
  exit "$GOOD_APPROVE_RC"
fi
printf '%s\n' "$GOOD_APPROVE_OUT"

# verify status fields persisted
APPR_STATE=$(kubectl get securityprofileproposal "$PROPOSAL_NAME" -n "$NAMESPACE" -o jsonpath='{.status.approvalState}')
APPR_DIGEST=$(kubectl get securityprofileproposal "$PROPOSAL_NAME" -n "$NAMESPACE" -o jsonpath='{.status.approvedCandidateDigest}')
APPR_VER=$(kubectl get securityprofileproposal "$PROPOSAL_NAME" -n "$NAMESPACE" -o jsonpath='{.status.approvalMechanismVersion}')

echo "[info] status.approvalState=$APPR_STATE"
echo "[info] status.approvedCandidateDigest=$APPR_DIGEST"
echo "[info] status.approvalMechanismVersion=$APPR_VER"

if [ "$APPR_STATE" != "Approved" ]; then
  echo "ERROR: approval state not Approved" >&2
  exit 1
fi
if [ "$APPR_DIGEST" != "$CAND_DIGEST" ]; then
  echo "ERROR: approvedCandidateDigest mismatch" >&2
  exit 1
fi
if [ -z "$APPR_VER" ]; then
  echo "ERROR: approvalMechanismVersion missing" >&2
  exit 1
fi

# APPLY: use hardened apply-proposal (non-interactive)
echo "[stage] apply-proposal"
SKIP_ARGS=""
if [ "$SPO_PRESENT" -eq 0 ]; then
  SKIP_ARGS="$SKIP_ARGS --skip=spo-seccompprofile"
fi
if [ "$PODLOCK_PRESENT" -eq 0 ]; then
  SKIP_ARGS="$SKIP_ARGS --skip=podlock"
fi
set +e
APPLY_OUT=$("${CLI_CMD[@]}" apply-proposal "$PROPOSAL_NAME" -n "$NAMESPACE" --restart --yes $SKIP_ARGS 2>&1)
APPLY_RC=$?
set -e
printf '%s\n' "$APPLY_OUT" > "${ARTIFACTS_DIR}/apply-proposal.out"
if [ "$APPLY_RC" -ne 0 ]; then
  echo "ERROR: apply-proposal failed" >&2
  printf '%s\n' "$APPLY_OUT" >&2
  exit "$APPLY_RC"
fi
printf '%s\n' "$APPLY_OUT"

# verify applied resources existence
# LandlockProfile (PodLock CR) name uses proposal name per exporters
echo "[verify] resources existence"
if kubectl get landlockprofile "$PROPOSAL_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "LandlockProfile: APPLIED"
else
  echo "LandlockProfile: NOT FOUND or operator missing"
fi
if kubectl get networkpolicy "$PROPOSAL_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "NetworkPolicy: APPLIED"
else
  echo "NetworkPolicy: NOT FOUND"
fi
if kubectl get seccompprofile "$PROPOSAL_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "SeccompProfile: APPLIED"
else
  echo "SeccompProfile: NOT FOUND or SPO missing"
fi

# inspect patched manifest applied (workload restart effect)
# For a pod it is delete+create; check pod existence
if kubectl get pod "$POD" -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "workload pod exists after apply"
  kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.spec.containers[0].securityContext}' || true
else
  echo "workload pod not found after apply"
fi

# Optional enforcement checks (only when operators present)
if [ "$SPO_PRESENT" -eq 1 ]; then
  echo "[enforce-check] SPO present — attempting seccomp enforcement probe (best-effort)"
  echo "(manual verification recommended)"
else
  echo "[enforce-check] SPO not present — skipping seccomp enforcement checks"
fi
if [ "$PODLOCK_PRESENT" -eq 1 ]; then
  echo "[enforce-check] PodLock present — attempting Landlock enforcement probe (best-effort)"
  echo "(manual verification recommended)"
else
  echo "[enforce-check] PodLock not present — skipping Landlock enforcement checks"
fi

# Final summary
echo "[done] Golden demo completed (resources may be operator-dependent)."

echo "Cleanup hints: kubectl delete -f demo/golden/workload.yaml; kubectl delete -f demo/golden/echo-service.yaml; kubectl delete securityprofileproposal $PROPOSAL_NAME -n $NAMESPACE; kubectl delete traininghistory $TH_V2_NAME -n $NAMESPACE || true"
