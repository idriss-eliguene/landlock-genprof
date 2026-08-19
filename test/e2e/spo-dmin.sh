#!/usr/bin/env bash
# spo-dmin.sh — prove the UPSTREAM half of the SPO architecture against a
# real security-profiles-operator: D-MIN.
#
#   real ProfileRecording → real SPO observation
#     → real SPO-generated SeccompProfile (inert, lineage-labelled)
#     → landlock-genprof import (ADR-0008 gates)
#     → governed snapshot → proposal → review
#     → digest-bound approval → governed apply
#     → real SPO reconciliation → ADR-0007 readiness → identity
#     → workload binding → Running
#
# spo-interop.sh proves the downstream half with syscalls this project
# observed itself. This proves the half that crosses the project boundary:
# SPO learns the syscall authority, a human authorizes exactly that
# content, and SPO enforces exactly what was authorized.
#
# No fake SPO objects anywhere. Every object under test is produced by the
# real operator.
#
# What this does NOT prove:
#
#   * behavioral syscall enforcement — nothing attempts a denied syscall.
#   * mergeStrategy: Containers — deliberately out of scope for v0.2. SPO's
#     merger drops the container-id label, so a merged profile cannot
#     satisfy the lineage contract and is refused (docs/adr/0008, "Merge
#     strategy scope"). This scenario uses mergeStrategy: None.
#   * syscall coverage — SPO v1.0.0 emits no coverage metadata at all, so
#     the authoritative expectation here is the token "unknown".
#
# Requires SPO installed (test/e2e/install-spo.sh).

set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

NAMESPACE="${NAMESPACE:-landlock-genprof-dmin}"
SPO_NAMESPACE="${SPO_NAMESPACE:-security-profiles-operator}"
RECORDING="${RECORDING:-lgdmin}"
RECORDER_POD="${RECORDER_POD:-dmin-recorder}"
POD="${POD:-nginx-demo}"
CONTAINER="${CONTAINER:-tools}"
BINARY="${BINARY:-/usr/bin/curl}"
DURATION="${DURATION:-40s}"
IMAGE="${IMAGE:-curlimages/curl:8.3.0}"
RECORD_SECONDS="${RECORD_SECONDS:-45}"
EXPECTED_CONTEXT="${EXPECTED_CONTEXT-kind-landlock-genprof-e2e}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/artifacts}"

# SPO names a recorded profile "<recording>-<container>" when
# mergeStrategy is None (createProfileName, no replica suffix).
SOURCE_PROFILE="${RECORDING}-${CONTAINER}"

# The workload command is identical in the recorded and the governed pod.
# That is deliberate and load-bearing: the recorded profile has
# defaultAction SCMP_ACT_ERRNO, so the governed container can only boot
# under it if it makes the same syscalls. Different commands would turn a
# governance proof into a syscall-coverage lottery.
WORKLOAD_CMD='while true; do curl -sS file:///etc/hosts -o /dev/null 2>/dev/null || true; sleep 2; done'

mkdir -p "${ARTIFACTS_DIR}"

fail() { echo "ERROR: $*" >&2; exit 1; }
stage() { printf '\n[stage] %s\n' "$*"; }

# --- preflight -------------------------------------------------------------
stage "preflight"

command -v kubectl >/dev/null 2>&1 || fail "kubectl not found"
command -v jq >/dev/null 2>&1 || fail "jq not found"

if [ -n "${EXPECTED_CONTEXT}" ]; then
  CUR_CTX="$(kubectl config current-context 2>/dev/null || true)"
  [ "${CUR_CTX}" = "${EXPECTED_CONTEXT}" ] || fail "context is ${CUR_CTX}, expected ${EXPECTED_CONTEXT}"
fi

CLI_CMD=(kubectl landlock-genprof)
if [ -n "${LANDLOCK_GENPROF_BIN:-}" ]; then
  # shellcheck disable=SC2206
  CLI_CMD=(${LANDLOCK_GENPROF_BIN})
fi
"${CLI_CMD[@]}" --help >/dev/null 2>&1 || fail "${CLI_CMD[*]} is not usable"

kubectl get crd profilerecordings.security-profiles-operator.x-k8s.io >/dev/null 2>&1 \
  || fail "SPO ProfileRecording CRD is absent — run test/e2e/install-spo.sh first"

# --- 0. recorder enablement ------------------------------------------------
stage "0. RECORDER — enable SPO's eBPF recorder"

# Off by default. Patched here rather than in install-spo.sh so the already
# certified spo-interop.sh scenario runs against an unmodified installation.
kubectl -n "${SPO_NAMESPACE}" patch spod spod --type=merge \
  -p '{"spec":{"enableBpfRecorder":true}}'

# The operator rewrites the spod DaemonSet in response; wait for the new
# generation to roll out rather than racing the old pods.
sleep 10
kubectl -n "${SPO_NAMESPACE}" rollout status daemonset/spod --timeout=420s \
  || { kubectl -n "${SPO_NAMESPACE}" get pods -o wide >&2; fail "spod did not roll out with the bpf recorder enabled"; }

# Fail here, loudly, rather than three stages later with an empty profile.
for _ in $(seq 1 30); do
  if kubectl -n "${SPO_NAMESPACE}" get daemonset spod -o json \
    | jq -e '[.spec.template.spec.containers[].name] | index("bpf-recorder")' >/dev/null 2>&1; then
    break
  fi
  sleep 5
done
kubectl -n "${SPO_NAMESPACE}" get daemonset spod -o json \
  | jq -e '[.spec.template.spec.containers[].name] | index("bpf-recorder")' >/dev/null 2>&1 \
  || fail "spod has no bpf-recorder container after enabling it; this cluster's kernel may not support SPO's recorder"

echo "[ok] RECORDER — spod is running the bpf recorder"

# --- 1. recording ----------------------------------------------------------
stage "1. PROFILE RECORDING — real ProfileRecording, mergeStrategy None"

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# SPO's recording webhook is namespace-selected. Without this label the
# recorder annotation is never injected and the pod is simply not recorded.
kubectl label namespace "${NAMESPACE}" spo.x-k8s.io/enable-recording=true --overwrite

kubectl apply -f - <<YAML
apiVersion: security-profiles-operator.x-k8s.io/v1
kind: ProfileRecording
metadata:
  name: ${RECORDING}
  namespace: ${NAMESPACE}
spec:
  kind: SeccompProfile
  recorder: Bpf
  # v0.2 supports None only: the merger drops the container-id label, so a
  # merged profile cannot satisfy ADR-0008's lineage tuple.
  mergeStrategy: None
  # Required by the import. Makes SPO leave the generated profile in
  # spec.state: Disabled, so recorded authority is never enforced on any
  # node before a human has approved it.
  disableProfileAfterRecording: true
  podSelector:
    matchLabels:
      app: lg-dmin-record
YAML

# The webhook mutates pods at admission, so the recording must exist first.
kubectl apply -f - <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: ${RECORDER_POD}
  namespace: ${NAMESPACE}
  labels:
    app: lg-dmin-record
spec:
  terminationGracePeriodSeconds: 5
  restartPolicy: Never
  containers:
    - name: ${CONTAINER}
      image: ${IMAGE}
      command: ["sh", "-c", "${WORKLOAD_CMD}"]
      securityContext:
        runAsUser: 0
YAML

kubectl wait --for=condition=Ready "pod/${RECORDER_POD}" -n "${NAMESPACE}" --timeout=240s \
  || { kubectl describe pod "${RECORDER_POD}" -n "${NAMESPACE}" >&2; fail "the recorded pod never became Ready"; }

# Proof the webhook actually engaged. Without this the scenario could
# "pass" recording nothing and fail confusingly much later.
# SPO's recorder annotation prefix is "io.containers.trace-bpf/<container>"
# for the Bpf recorder and "io.containers.trace-logs/" for Logs
# (internal/pkg/config/config.go). Matching the common prefix keeps this
# assertion true if the recorder is switched.
RECORD_ANN="$(kubectl get pod "${RECORDER_POD}" -n "${NAMESPACE}" -o json \
  | jq -r '.metadata.annotations | to_entries[] | select(.key|startswith("io.containers.trace-")) | .key' | head -1)"
[ -n "${RECORD_ANN}" ] \
  || fail "SPO's webhook did not annotate the pod for recording; is the namespace labelled and the webhook running?"
echo "[info] recording annotation: ${RECORD_ANN}"

echo "[info] letting the workload run for ${RECORD_SECONDS}s under the recorder"
sleep "${RECORD_SECONDS}"

# SPO writes the profile when the recorded container stops.
kubectl delete pod "${RECORDER_POD}" -n "${NAMESPACE}" --wait=true --timeout=120s

echo "[info] waiting for SPO to generate ${SOURCE_PROFILE}"
for _ in $(seq 1 60); do
  if kubectl get seccompprofile "${SOURCE_PROFILE}" >/dev/null 2>&1; then
    break
  fi
  sleep 5
done
kubectl get seccompprofile "${SOURCE_PROFILE}" >/dev/null 2>&1 || {
  kubectl -n "${SPO_NAMESPACE}" logs daemonset/spod -c bpf-recorder --tail=200 >&2 || true
  kubectl get seccompprofile -o wide >&2 || true
  fail "SPO never generated ${SOURCE_PROFILE}"
}
echo "[ok] PROFILE RECORDING — real SPO produced ${SOURCE_PROFILE}"

# --- 2. derived profile ----------------------------------------------------
stage "2. DERIVED PROFILE — real observation, inert, lineage-labelled"

kubectl get seccompprofile "${SOURCE_PROFILE}" -o json > "${ARTIFACTS_DIR}/dmin-source-profile.json"

SRC_API="$(jq -r '.apiVersion' "${ARTIFACTS_DIR}/dmin-source-profile.json")"
SRC_STATE="$(jq -r '.spec.state // ""' "${ARTIFACTS_DIR}/dmin-source-profile.json")"
SRC_DEFAULT="$(jq -r '.spec.defaultAction // ""' "${ARTIFACTS_DIR}/dmin-source-profile.json")"
SRC_SYSCALLS="$(jq '[.spec.syscalls[]?.names[]?] | length' "${ARTIFACTS_DIR}/dmin-source-profile.json")"
SRC_PARTIAL="$(jq -r '.metadata.labels["spo.x-k8s.io/partial"] // "<absent>"' "${ARTIFACTS_DIR}/dmin-source-profile.json")"
SRC_REC="$(jq -r '.metadata.labels["spo.x-k8s.io/recording-id"] // ""' "${ARTIFACTS_DIR}/dmin-source-profile.json")"
SRC_REC_NS="$(jq -r '.metadata.labels["spo.x-k8s.io/recording-namespace"] // ""' "${ARTIFACTS_DIR}/dmin-source-profile.json")"
SRC_CTR="$(jq -r '.metadata.labels["spo.x-k8s.io/container-id"] // ""' "${ARTIFACTS_DIR}/dmin-source-profile.json")"
SRC_COVERAGE="$(jq -r '.metadata.annotations["spo.x-k8s.io/syscall-coverage"] // "<absent>"' "${ARTIFACTS_DIR}/dmin-source-profile.json")"

echo "[info] apiVersion=${SRC_API} state=${SRC_STATE} defaultAction=${SRC_DEFAULT} syscalls=${SRC_SYSCALLS}"
echo "[info] labels: recording-id=${SRC_REC} recording-namespace=${SRC_REC_NS} container-id=${SRC_CTR} partial=${SRC_PARTIAL}"
echo "[info] annotation syscall-coverage=${SRC_COVERAGE}"

[ "${SRC_API}" = "security-profiles-operator.x-k8s.io/v1" ] || fail "source apiVersion is ${SRC_API}"
[ "${SRC_SYSCALLS}" -gt 0 ] || fail "SPO recorded no syscalls; the recorder produced an empty profile"

# INERTNESS — a required security assertion. SPO-derived policy must never
# become authority merely because SPO generated it.
[ "${SRC_STATE}" = "Disabled" ] \
  || fail "source profile state is '${SRC_STATE}', expected Disabled — disableProfileAfterRecording did not take effect, so recorded authority is live before review"
echo "[ok] INERTNESS — source profile is spec.state: Disabled"

# COMPLETENESS — mergeStrategy None must not produce a partial profile.
[ "${SRC_PARTIAL}" = "<absent>" ] \
  || fail "source profile carries the partial label (${SRC_PARTIAL}); mergeStrategy None should not produce partials"

# LINEAGE — this is one of the primary purposes of this scenario. If real
# SPO does not emit the tuple ADR-0008 requires, that is an architectural
# finding, not something to work around.
LINEAGE_OK=1
[ "${SRC_REC}" = "${RECORDING}" ] || { echo "MISMATCH recording-id: ${SRC_REC} != ${RECORDING}" >&2; LINEAGE_OK=0; }
[ "${SRC_REC_NS}" = "${NAMESPACE}" ] || { echo "MISMATCH recording-namespace: ${SRC_REC_NS} != ${NAMESPACE}" >&2; LINEAGE_OK=0; }
[ "${SRC_CTR}" = "${CONTAINER}" ] || { echo "MISMATCH container-id: ${SRC_CTR} != ${CONTAINER}" >&2; LINEAGE_OK=0; }
[ "${LINEAGE_OK}" -eq 1 ] || fail "ARCHITECTURAL ASSUMPTION INVALID — real SPO did not emit the lineage tuple docs/adr/0008 requires. Do not weaken validation to make this pass."
echo "[ok] LINEAGE — real SPO emitted recording-id, recording-namespace and container-id exactly as required"

# Snapshot the source for the immutability assertion later.
jq -S '{spec: .spec, labels: .metadata.labels}' "${ARTIFACTS_DIR}/dmin-source-profile.json" \
  > "${ARTIFACTS_DIR}/dmin-source-before.json"

# --- 3. governed workload --------------------------------------------------
stage "3. WORKLOAD — deploy the governed pod (not recorded)"

# Deliberately does NOT carry app=lg-dmin-record: the ProfileRecording still
# exists (the import requires it), and a matching label would make SPO
# record this pod too and overwrite the source profile mid-scenario.
kubectl apply -f - <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: ${POD}
  namespace: ${NAMESPACE}
  labels:
    app: lg-dmin-governed
spec:
  terminationGracePeriodSeconds: 5
  containers:
    - name: ${CONTAINER}
      image: ${IMAGE}
      command: ["sh", "-c", "${WORKLOAD_CMD}"]
      securityContext:
        runAsUser: 0
YAML

kubectl wait --for=condition=Ready "pod/${POD}" -n "${NAMESPACE}" --timeout=240s

# --- 4. import -------------------------------------------------------------
stage "4. IMPORT — landlock-genprof imports the SPO-derived policy"

# --history so the TrainingHistory assertion below has something to assert
# against: filesystem and network must still accumulate, syscalls must not.
"${CLI_CMD[@]}" trace \
  --pod "${POD}" -n "${NAMESPACE}" \
  --container "${CONTAINER}" --binary "${BINARY}" \
  --duration "${DURATION}" --history \
  --seccomp-source=spo \
  --spo-recording "${RECORDING}" \
  --spo-profile "${SOURCE_PROFILE}" \
  --out "${ARTIFACTS_DIR}/${POD}-profile.yaml" \
  >"${ARTIFACTS_DIR}/dmin-trace.log" 2>&1 &
TRACE_PID=$!

sleep 8
for _ in 1 2 3; do
  kubectl exec -n "${NAMESPACE}" "${POD}" -c "${CONTAINER}" -- \
    "${BINARY}" -sS file:///etc/hosts -o /dev/null || true
done
wait "${TRACE_PID}" || { cat "${ARTIFACTS_DIR}/dmin-trace.log" >&2; fail "trace with --seccomp-source=spo failed"; }

grep -q "Seccomp source: SPO derived policy" "${ARTIFACTS_DIR}/dmin-trace.log" \
  || { cat "${ARTIFACTS_DIR}/dmin-trace.log" >&2; fail "trace did not report an SPO import"; }
grep "Seccomp source: SPO derived policy" "${ARTIFACTS_DIR}/dmin-trace.log"
echo "[ok] IMPORT — the ADR-0008 gates accepted a real SPO-generated profile"

# --- 5. snapshot -----------------------------------------------------------
stage "5. SNAPSHOT — the source is unchanged and the governed copy is distinct"

kubectl get seccompprofile "${SOURCE_PROFILE}" -o json > "${ARTIFACTS_DIR}/dmin-source-after-raw.json"
jq -S '{spec: .spec, labels: .metadata.labels}' "${ARTIFACTS_DIR}/dmin-source-after-raw.json" \
  > "${ARTIFACTS_DIR}/dmin-source-after.json"

if ! diff -u "${ARTIFACTS_DIR}/dmin-source-before.json" "${ARTIFACTS_DIR}/dmin-source-after.json"; then
  fail "landlock-genprof mutated the SPO source object; import must be read-only"
fi
echo "[ok] SNAPSHOT — the SPO source object is byte-identical after import"

kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" \
  -o jsonpath='{.spec.spoSeccompProfile}' > "${ARTIFACTS_DIR}/dmin-governed.yaml"
[ -s "${ARTIFACTS_DIR}/dmin-governed.yaml" ] || fail "the proposal carries no seccomp artifact"

kubectl create --dry-run=client -o json -f "${ARTIFACTS_DIR}/dmin-governed.yaml" \
  > "${ARTIFACTS_DIR}/dmin-governed.json" 2>/dev/null \
  || fail "the governed SeccompProfile is not a valid manifest"

PROFILE_NAME="$(jq -r '.metadata.name' "${ARTIFACTS_DIR}/dmin-governed.json")"
echo "[info] source=${SOURCE_PROFILE} governed=${PROFILE_NAME}"
[ "${PROFILE_NAME}" != "${SOURCE_PROFILE}" ] \
  || fail "the governed profile reuses the SPO source name; Model B requires a distinct owned object"
case "${PROFILE_NAME}" in
  lg-v1-*) ;;
  *) fail "governed name ${PROFILE_NAME} does not follow the lg-v1-* contract" ;;
esac
EXPECTED_PATH="operator/${PROFILE_NAME}.json"
echo "[ok] SNAPSHOT — governed copy ${PROFILE_NAME} is distinct from the source"

# Enforcement content must have survived the copy exactly.
jq -S '{defaultAction: .spec.defaultAction, architectures: .spec.architectures, syscalls: .spec.syscalls}' \
  "${ARTIFACTS_DIR}/dmin-source-profile.json" > "${ARTIFACTS_DIR}/dmin-content-source.json"
jq -S '{defaultAction: .spec.defaultAction, architectures: .spec.architectures, syscalls: .spec.syscalls}' \
  "${ARTIFACTS_DIR}/dmin-governed.json" > "${ARTIFACTS_DIR}/dmin-content-governed.json"
if ! diff -u "${ARTIFACTS_DIR}/dmin-content-source.json" "${ARTIFACTS_DIR}/dmin-content-governed.json"; then
  fail "the governed copy's enforcement content differs from the source; the snapshot lost or altered semantics"
fi
echo "[ok] SEMANTICS — defaultAction/architectures/syscalls copied exactly"

# --- 6. provenance ---------------------------------------------------------
stage "6. PROVENANCE — the governed artifact records what was verified"

prov() { jq -r --arg k "$1" '.metadata.annotations[$k] // "<absent>"' "${ARTIFACTS_DIR}/dmin-governed.json"; }

P_SOURCE="$(prov landlockgenprof.io/seccomp-source)"
P_ORIGIN="$(prov landlockgenprof.io/seccomp-origin)"
P_PROFILE="$(prov landlockgenprof.io/spo-source-profile)"
P_REC_NS="$(prov landlockgenprof.io/spo-recording-namespace)"
P_REC="$(prov landlockgenprof.io/spo-recording-name)"
P_CTR="$(prov landlockgenprof.io/spo-container-id)"
P_COV="$(prov landlockgenprof.io/spo-syscall-coverage)"

printf '[info] source=%s origin=%s profile=%s recording=%s/%s container=%s coverage=%s\n' \
  "${P_SOURCE}" "${P_ORIGIN}" "${P_PROFILE}" "${P_REC_NS}" "${P_REC}" "${P_CTR}" "${P_COV}"

[ "${P_SOURCE}" = "spo" ]                 || fail "seccomp-source=${P_SOURCE}, expected spo"
[ "${P_ORIGIN}" = "derived" ]             || fail "seccomp-origin=${P_ORIGIN}, expected derived"
[ "${P_PROFILE}" = "${SOURCE_PROFILE}" ]  || fail "spo-source-profile=${P_PROFILE}, expected ${SOURCE_PROFILE}"
[ "${P_REC_NS}" = "${NAMESPACE}" ]        || fail "spo-recording-namespace=${P_REC_NS}"
[ "${P_REC}" = "${RECORDING}" ]           || fail "spo-recording-name=${P_REC}"
[ "${P_CTR}" = "${CONTAINER}" ]           || fail "spo-container-id=${P_CTR}"

# Upstream SPO v1.0.0 emits no coverage metadata. "unknown" is the correct
# recorded fact — it must never be fabricated into a number or a tier.
[ "${P_COV}" = "unknown" ] \
  || fail "coverage=${P_COV}; SPO v1.0.0 provides none, so the only honest value is 'unknown'"
case "${P_COV}" in
  0|full|low|medium|high) fail "coverage was fabricated as ${P_COV}" ;;
esac
echo "[ok] PROVENANCE — source/origin/lineage recorded, coverage honestly unknown"

# --- 7. TrainingHistory boundary -------------------------------------------
stage "7. NO TRAININGHISTORY CONTAMINATION — derived policy is not observation"

kubectl get traininghistory "${POD}" -n "${NAMESPACE}" -o json > "${ARTIFACTS_DIR}/dmin-history.json" 2>/dev/null \
  || fail "no TrainingHistory was recorded; --history did not take effect, so this assertion would be vacuous"

TH_SYSCALLS="$(jq '(.spec.syscallAccesses // []) | length' "${ARTIFACTS_DIR}/dmin-history.json")"
TH_FS="$(jq '(.spec.filesystemAccesses // []) | length' "${ARTIFACTS_DIR}/dmin-history.json")"
TH_RUNS="$(jq '.spec.runsRecorded // 0' "${ARTIFACTS_DIR}/dmin-history.json")"
echo "[info] TrainingHistory: runsRecorded=${TH_RUNS} filesystemAccesses=${TH_FS} syscallAccesses=${TH_SYSCALLS}"

# The assertion that matters: SPO-derived syscall policy did not become
# syscall evidence. Filesystem accesses SHOULD be present — they are still
# ours, and their presence is what proves this history is real rather than
# an empty object that would satisfy the syscall check trivially.
[ "${TH_SYSCALLS}" -eq 0 ] \
  || fail "TrainingHistory carries ${TH_SYSCALLS} syscallAccesses in SPO mode; derived policy became observation evidence"
[ "${TH_FS}" -gt 0 ] \
  || fail "TrainingHistory carries no filesystemAccesses, so the syscall assertion above proves nothing — filesystem observation must still work in SPO mode"
echo "[ok] NO CONTAMINATION — ${TH_FS} filesystem accesses recorded, 0 syscall accesses"

# --- 8. review -------------------------------------------------------------
stage "8. REVIEW — the reviewer is told the truth about the source"

"${CLI_CMD[@]}" review "${POD}" -n "${NAMESPACE}" | tee "${ARTIFACTS_DIR}/dmin-review.txt"

grep -q "Source: security-profiles-operator" "${ARTIFACTS_DIR}/dmin-review.txt" \
  || fail "review does not name SPO as the seccomp source"
grep -q "Origin: derived policy" "${ARTIFACTS_DIR}/dmin-review.txt" \
  || fail "review does not identify the seccomp domain as derived policy"
grep -q "Coverage: unknown" "${ARTIFACTS_DIR}/dmin-review.txt" \
  || fail "review does not report coverage as unknown"
grep -q "Confidence: not applicable" "${ARTIFACTS_DIR}/dmin-review.txt" \
  || fail "review does not state that confidence is not applicable for derived policy"
# `grep && fail` would abort the script under set -e on the GOOD path,
# because a non-matching grep makes the whole list return non-zero.
if grep -q "Source: landlock-genprof observation" "${ARTIFACTS_DIR}/dmin-review.txt"; then
  fail "review claims landlock-genprof observed the imported syscalls"
fi
echo "[ok] REVIEW — SPO source, derived origin, unknown coverage, no invented confidence"

# --- 9. digest-bound approval ----------------------------------------------
stage "9. DIGEST — approval is bound to this exact candidate"

DIGEST="$(awk '/^Candidate digest: /{print $3; exit}' "${ARTIFACTS_DIR}/dmin-review.txt")"
[ -n "${DIGEST}" ] || fail "review produced no CandidateDigest"

BAD_DIGEST="sha256:0000000000000000000000000000000000000000000000000000000000000000"
set +e
"${CLI_CMD[@]}" approve "${POD}" -n "${NAMESPACE}" --expected-digest "${BAD_DIGEST}" --reason "negative" \
  >"${ARTIFACTS_DIR}/dmin-approve-wrong.txt" 2>&1
BAD_RC=$?
set -e
[ "${BAD_RC}" -ne 0 ] || fail "approve succeeded with a wrong digest"
echo "[ok] NEGATIVE — wrong-digest approval refused"

"${CLI_CMD[@]}" approve "${POD}" -n "${NAMESPACE}" --expected-digest "${DIGEST}" --reason "spo d-min"
APPROVED_DIGEST="$(kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" -o jsonpath='{.status.approvedCandidateDigest}')"
[ "${APPROVED_DIGEST}" = "${DIGEST}" ] || fail "approved digest ${APPROVED_DIGEST} != reviewed ${DIGEST}"
echo "[ok] APPROVAL — bound to ${DIGEST}"

if kubectl get seccompprofile "${PROFILE_NAME}" >/dev/null 2>&1; then
  fail "governed SeccompProfile ${PROFILE_NAME} exists before apply-proposal"
fi

# --- 10. governed apply ----------------------------------------------------
stage "10. GOVERNED APPLY — the governed copy is applied, not the source"

set +e
"${CLI_CMD[@]}" apply-proposal "${POD}" -n "${NAMESPACE}" --yes --restart \
  --skip=podlock --readiness-timeout=180s \
  >"${ARTIFACTS_DIR}/dmin-apply.log" 2>&1
APPLY_RC=$?
set -e
cat "${ARTIFACTS_DIR}/dmin-apply.log"
[ "${APPLY_RC}" -eq 0 ] || fail "governed apply failed (exit ${APPLY_RC})"

kubectl get seccompprofile "${PROFILE_NAME}" >/dev/null 2>&1 \
  || fail "the governed SeccompProfile ${PROFILE_NAME} was not applied"

# The source must still be inert. Applying the governed copy must not have
# enabled, adopted, or otherwise touched SPO's object.
SRC_STATE_AFTER="$(kubectl get seccompprofile "${SOURCE_PROFILE}" -o jsonpath='{.spec.state}')"
[ "${SRC_STATE_AFTER}" = "Disabled" ] \
  || fail "the SPO source profile is now ${SRC_STATE_AFTER}; governed apply must never enable the source"
echo "[ok] GOVERNED APPLY — ${PROFILE_NAME} applied, source still Disabled"

# --- 11. reconciliation ----------------------------------------------------
stage "11. SPO RECONCILIATION — real SPO materialized the governed profile"

INSTALLED="$(kubectl get seccompprofile "${PROFILE_NAME}" -o jsonpath='{.status.localhostProfile}')"
echo "[info] status.localhostProfile=${INSTALLED}"
[ "${INSTALLED}" = "${EXPECTED_PATH}" ] || fail "SPO materialized ${INSTALLED}, expected ${EXPECTED_PATH}"
echo "[ok] RECONCILED — ${INSTALLED}"

# --- 12. identity ----------------------------------------------------------
stage "12. IDENTITY — live enforcement content equals the approved content"

kubectl get seccompprofile "${PROFILE_NAME}" -o json > "${ARTIFACTS_DIR}/dmin-live.json"
IDENTITY_FIELDS='{defaultAction: .spec.defaultAction, architectures: .spec.architectures, syscalls: .spec.syscalls}'
jq -S "${IDENTITY_FIELDS}" "${ARTIFACTS_DIR}/dmin-governed.json" > "${ARTIFACTS_DIR}/dmin-identity-approved.json"
jq -S "${IDENTITY_FIELDS}" "${ARTIFACTS_DIR}/dmin-live.json"     > "${ARTIFACTS_DIR}/dmin-identity-live.json"

if ! diff -u "${ARTIFACTS_DIR}/dmin-identity-approved.json" "${ARTIFACTS_DIR}/dmin-identity-live.json"; then
  fail "the live profile's enforcement content differs from the approved content"
fi
echo "[ok] IDENTITY — what SPO enforces is what was approved"

# --- 13. binding -----------------------------------------------------------
stage "13. BINDING — the workload references the governed profile, never the source"

BOUND_PATH="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o json \
  | jq -r --arg c "${CONTAINER}" '.spec.containers[] | select(.name==$c) | .securityContext.seccompProfile.localhostProfile // ""')"
echo "[info] container ${CONTAINER} localhostProfile=${BOUND_PATH}"
[ "${BOUND_PATH}" = "${EXPECTED_PATH}" ] || fail "container references ${BOUND_PATH}, expected ${EXPECTED_PATH}"
case "${BOUND_PATH}" in
  *"${SOURCE_PROFILE}"*) fail "the workload is bound to the SPO source profile rather than the governed copy" ;;
esac
echo "[ok] BOUND — ${CONTAINER} -> ${EXPECTED_PATH}"

# --- 14. running -----------------------------------------------------------
stage "14. RUNNING — the workload starts under SPO-recorded, human-approved authority"

if ! kubectl wait --for=condition=Ready "pod/${POD}" -n "${NAMESPACE}" --timeout=180s; then
  echo "==== pod did not become Ready ====" >&2
  kubectl describe pod "${POD}" -n "${NAMESPACE}" >&2 || true
  if kubectl describe pod "${POD}" -n "${NAMESPACE}" 2>/dev/null | grep -qiE "cannot load seccomp profile|no such file or directory"; then
    fail "the workload could not resolve its seccomp profile — an ADR-0007 readiness failure"
  fi
  fail "the workload did not start under the SPO-recorded profile; likely recording coverage, not the governed path"
fi

PHASE="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o jsonpath='{.status.phase}')"
[ "${PHASE}" = "Running" ] || fail "pod phase is ${PHASE}, expected Running"
RESTARTS="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o json | jq '[.status.containerStatuses[].restartCount] | add')"
WAITING="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o json | jq -r '[.status.containerStatuses[].state.waiting.reason // empty] | join(",")')"
echo "[info] phase=${PHASE} restarts=${RESTARTS} waiting=${WAITING:-none}"
case "${WAITING}" in
  *CrashLoopBackOff*|*CreateContainerError*) fail "container is in ${WAITING}" ;;
esac
[ "${RESTARTS}" -eq 0 ] || fail "container restarted ${RESTARTS} times under the imported profile"
echo "[ok] RUNNING — bound to the approved profile, ${RESTARTS} restarts"

# --- summary ---------------------------------------------------------------
stage "D-MIN proven"
cat <<SUMMARY
  PROFILE RECORDING   real ProfileRecording ${NAMESPACE}/${RECORDING} (mergeStrategy None)
  REAL OBSERVATION    SPO's eBPF recorder captured ${SRC_SYSCALLS} syscall names
  DERIVED PROFILE     ${SOURCE_PROFILE}
  INERTNESS           source spec.state: Disabled, before and after apply
  LINEAGE             recording-id / recording-namespace / container-id all emitted and matched
  IMPORT              ADR-0008 gates accepted real SPO output
  SNAPSHOT            source byte-identical after import; governed copy ${PROFILE_NAME}
  PROVENANCE          source=spo origin=derived coverage=unknown
  NO CONTAMINATION    ${TH_FS} filesystem accesses, 0 syscall accesses in TrainingHistory
  REVIEW              SPO / derived / unknown / confidence not applicable
  DIGEST              ${DIGEST}
  APPROVAL            wrong digest refused, correct digest bound
  GOVERNED APPLY      ${PROFILE_NAME} applied; source untouched
  RECONCILIATION      SPO reported ${INSTALLED}
  IDENTITY            live == approved
  BINDING             ${CONTAINER} -> ${EXPECTED_PATH}
  RUNNING             ${PHASE}, restarts=${RESTARTS}

  NOT proven here: behavioral syscall denial; mergeStrategy Containers
  (out of scope for v0.2 — SPO's merger drops container-id); syscall
  coverage (upstream SPO v1.0.0 emits none).
SUMMARY
