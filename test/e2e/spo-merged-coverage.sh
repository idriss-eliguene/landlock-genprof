#!/usr/bin/env bash
# Real-node evidence for ADR-0009 and SPO syscall coverage (#3355).

set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAMESPACE="${MERGED_NAMESPACE:-landlock-genprof-merged}"
SPO_NAMESPACE="${SPO_NAMESPACE:-security-profiles-operator}"
RECORDING="${MERGED_RECORDING:-lgmerged}"
RECORDER_POD="${MERGED_RECORDER_POD:-merged-recorder}"
POD="${MERGED_POD:-merged-target}"
CONTAINER="${MERGED_CONTAINER:-tools}"
BINARY="${MERGED_BINARY:-/usr/bin/curl}"
IMAGE="${MERGED_IMAGE:-curlimages/curl:8.3.0}"
DURATION="${MERGED_DURATION:-12s}"
RECORD_SECONDS="${MERGED_RECORD_SECONDS:-45}"
EXPECTED_CONTEXT="${EXPECTED_CONTEXT:-default}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/artifacts}"
COVERAGE_KEY="spo.x-k8s.io/syscall-coverage"
WORKLOAD_CMD='while true; do curl -sS file:///etc/hosts -o /dev/null 2>/dev/null || true; sleep 2; done'

mkdir -p "${ARTIFACTS_DIR}"
fail() { echo "ERROR: $*" >&2; exit 1; }
stage() { printf '\n[merged] %s\n' "$*"; }

CLI_CMD=(kubectl landlock-genprof)
if [ -n "${LANDLOCK_GENPROF_BIN:-}" ]; then
  # shellcheck disable=SC2206
  CLI_CMD=(${LANDLOCK_GENPROF_BIN})
fi

stage "preflight"
[ "$(kubectl config current-context)" = "${EXPECTED_CONTEXT}" ] || fail "unexpected kubectl context"
kubectl get crd profilerecordings.security-profiles-operator.x-k8s.io >/dev/null
kubectl -n "${SPO_NAMESPACE}" rollout status deployment/security-profiles-operator --timeout=180s

stage "REAL SPO OUTPUT — mergeStrategy Containers"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
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
  mergeStrategy: Containers
  disableProfileAfterRecording: true
  podSelector:
    matchLabels:
      app: lg-merged-record
YAML

kubectl apply -f - <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: ${RECORDER_POD}
  namespace: ${NAMESPACE}
  labels:
    app: lg-merged-record
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

kubectl wait --for=condition=Ready "pod/${RECORDER_POD}" -n "${NAMESPACE}" --timeout=240s
for _ in $(seq 1 30); do
  CID="$(kubectl get pod "${RECORDER_POD}" -n "${NAMESPACE}" -o jsonpath='{.status.containerStatuses[0].containerID}')"
  [ -n "${CID}" ] && break
  sleep 2
done
[ -n "${CID:-}" ] || fail "recorded pod never reported a container ID"
sleep "${RECORD_SECONDS}"
kubectl delete pod "${RECORDER_POD}" -n "${NAMESPACE}" --wait=true --timeout=120s

# Wait for at least one real partial before deleting the recording, which is
# the normal SPO trigger for final merging.
PARTIALS=0
for _ in $(seq 1 60); do
  PARTIALS="$(kubectl get seccompprofile -l "spo.x-k8s.io/recording-id=${RECORDING},spo.x-k8s.io/partial" -o json 2>/dev/null | jq '.items | length')"
  [ "${PARTIALS}" -gt 0 ] && break
  sleep 5
done
[ "${PARTIALS}" -gt 0 ] || fail "SPO produced no partial SeccompProfiles"
echo "[evidence] real partial profiles before merge: ${PARTIALS}"

kubectl get profilerecording "${RECORDING}" -n "${NAMESPACE}" -o yaml > "${ARTIFACTS_DIR}/merged-profilerecording.yaml"
kubectl delete profilerecording "${RECORDING}" -n "${NAMESPACE}" --wait=true --timeout=240s

SOURCE_PROFILE=""
for _ in $(seq 1 60); do
  SOURCE_PROFILE="$(kubectl get seccompprofile -o json 2>/dev/null | jq -r --arg r "${RECORDING}" --arg n "${NAMESPACE}" \
    '.items[] | select(.metadata.labels["spo.x-k8s.io/recording-id"]==$r and .metadata.labels["spo.x-k8s.io/recording-namespace"]==$n and (.metadata.labels|has("spo.x-k8s.io/partial")|not)) | .metadata.name' | head -1)"
  [ -n "${SOURCE_PROFILE}" ] && break
  sleep 5
done
[ -n "${SOURCE_PROFILE}" ] || fail "SPO produced no final merged SeccompProfile"
kubectl get seccompprofile "${SOURCE_PROFILE}" -o json > "${ARTIFACTS_DIR}/merged-real-source.json"

REAL_COVERAGE="$(jq -r --arg k "${COVERAGE_KEY}" '.metadata.annotations[$k] // empty' "${ARTIFACTS_DIR}/merged-real-source.json")"
[ -n "${REAL_COVERAGE}" ] || fail "REAL SPO OUTPUT has no ${COVERAGE_KEY}; deployed SPO does not contain #3355"
printf '%s\n' "${REAL_COVERAGE}" | jq -e '.total as $total | .version=="v1" and ($total|type=="number" and .>0) and (.syscalls|type=="object") and ([.syscalls[] | . <= $total]|all)' >/dev/null \
  || fail "real coverage annotation violates supported v1 semantics: ${REAL_COVERAGE}"
COVERAGE_TOTAL="$(printf '%s' "${REAL_COVERAGE}" | jq '.total')"
REC_LABEL="$(jq -r '.metadata.labels["spo.x-k8s.io/recording-id"] // empty' "${ARTIFACTS_DIR}/merged-real-source.json")"
REC_NS_LABEL="$(jq -r '.metadata.labels["spo.x-k8s.io/recording-namespace"] // empty' "${ARTIFACTS_DIR}/merged-real-source.json")"
PARTIAL_LABEL="$(jq -r '.metadata.labels["spo.x-k8s.io/partial"] // "<absent>"' "${ARTIFACTS_DIR}/merged-real-source.json")"
CONTAINER_LABEL="$(jq -r '.metadata.labels["spo.x-k8s.io/container-id"] // "<absent>"' "${ARTIFACTS_DIR}/merged-real-source.json")"
[ "${REC_LABEL}" = "${RECORDING}" ] || fail "merged profile recording-id mismatch"
[ "${REC_NS_LABEL}" = "${NAMESPACE}" ] || fail "merged profile recording-namespace mismatch"
[ "${PARTIAL_LABEL}" = "<absent>" ] || fail "final merged profile is still partial"
[ "${CONTAINER_LABEL}" = "<absent>" ] || fail "merged output unexpectedly claims unique container lineage"
echo "[ok] REAL SPO OUTPUT profile=${SOURCE_PROFILE} coverage.version=v1 total=${COVERAGE_TOTAL}"

stage "independent application target"
kubectl apply -f - <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: ${POD}
  namespace: ${NAMESPACE}
  labels:
    app: lg-merged-target
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

trace_import() {
  local label="$1"
  "${CLI_CMD[@]}" trace --pod "${POD}" -n "${NAMESPACE}" --container "${CONTAINER}" \
    --binary "${BINARY}" --duration "${DURATION}" --history \
    --seccomp-source=spo --spo-import-mode=merged-provenance \
    --spo-recording-namespace "${NAMESPACE}" --spo-recording "${RECORDING}" \
    --spo-profile "${SOURCE_PROFILE}" --out "${ARTIFACTS_DIR}/merged-profile.yaml" \
    >"${ARTIFACTS_DIR}/merged-trace-${label}.log" 2>&1 &
  local trace_pid=$!
  sleep 3
  kubectl exec -n "${NAMESPACE}" "${POD}" -c "${CONTAINER}" -- "${BINARY}" -sS file:///etc/hosts -o /dev/null || true
  wait "${trace_pid}" || { cat "${ARTIFACTS_DIR}/merged-trace-${label}.log" >&2; fail "merged import ${label} failed"; }
  grep -q "SPO merged derived policy" "${ARTIFACTS_DIR}/merged-trace-${label}.log" || fail "${label} did not use explicit merged mode"

  # After the real KNOWN candidate is captured, compatibility derivatives
  # restore every unrelated candidate field to that baseline. The imported
  # SeccompProfile remains the one produced by the real import path; this
  # prevents run-to-run filesystem timing from masquerading as a coverage
  # digest change.
  if [ -s "${ARTIFACTS_DIR}/merged-baseline-spec.json" ]; then
    kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" -o json \
      | jq '.spec' > "${ARTIFACTS_DIR}/merged-current-spec.json"
    jq -s '{spec:(.[0] + {spoSeccompProfile: .[1].spoSeccompProfile})}' \
      "${ARTIFACTS_DIR}/merged-baseline-spec.json" "${ARTIFACTS_DIR}/merged-current-spec.json" \
      > "${ARTIFACTS_DIR}/merged-spec-patch.json"
    kubectl patch securityprofileproposal "${POD}" -n "${NAMESPACE}" --type=merge \
      --patch-file "${ARTIFACTS_DIR}/merged-spec-patch.json" >/dev/null
  fi
  "${CLI_CMD[@]}" review "${POD}" -n "${NAMESPACE}" | tee "${ARTIFACTS_DIR}/merged-review-${label}.txt" >&2
  awk '/^Candidate digest: /{print $3; exit}' "${ARTIFACTS_DIR}/merged-review-${label}.txt"
}

assert_merged_review() {
  local file="$1"
  grep -q "Source: security-profiles-operator" "${file}" || fail "review omits SPO source"
  grep -q "Origin: derived policy" "${file}" || fail "review omits derived origin"
  grep -q "Recording: ${NAMESPACE}/${RECORDING}" "${file}" || fail "review omits recording"
  grep -q "Derivation: merged" "${file}" || fail "review omits merged derivation"
  grep -q "Merge strategy: Containers" "${file}" || fail "review omits merge strategy"
  grep -q "Contributor lineage: unavailable" "${file}" || fail "review invents contributor lineage"
  grep -q "Application target: ${NAMESPACE}/${POD} container ${CONTAINER}" "${file}" || fail "review omits independent target"
  grep -q "Widening warning: this profile is a union" "${file}" || fail "review omits widening warning"
  grep -q "Confidence: not applicable" "${file}" || fail "review inferred confidence"
}

stage "govern real merged output"
D1="$(trace_import known)"
kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" -o json \
  | jq '.spec' > "${ARTIFACTS_DIR}/merged-baseline-spec.json"
assert_merged_review "${ARTIFACTS_DIR}/merged-review-known.txt"
grep -q "Syscall coverage: schema v1; ${COVERAGE_TOTAL} contributing partial profiles" "${ARTIFACTS_DIR}/merged-review-known.txt" \
  || fail "review does not expose normalized KNOWN coverage"

kubectl get traininghistory -n "${NAMESPACE}" -o json > "${ARTIFACTS_DIR}/merged-traininghistory.json"
TH_SYSCALLS="$(jq '[.items[].spec.syscallAccesses // [] | length] | add // 0' "${ARTIFACTS_DIR}/merged-traininghistory.json")"
TH_FS="$(jq '[.items[].spec.filesystemAccesses // [] | length] | add // 0' "${ARTIFACTS_DIR}/merged-traininghistory.json")"
[ "${TH_SYSCALLS}" -eq 0 ] || fail "coverage became TrainingHistory syscall evidence"
[ "${TH_FS}" -gt 0 ] || fail "TrainingHistory isolation assertion is vacuous"

"${CLI_CMD[@]}" approve "${POD}" -n "${NAMESPACE}" --expected-digest "${D1}" --reason "real SPO merged coverage"
APPROVED="$(kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" -o jsonpath='{.status.approvedCandidateDigest}')"
[ "${APPROVED}" = "${D1}" ] || fail "approval did not bind D1"

stage "CONTROLLED TEST DERIVATIVE — representation equivalence"
EQUIVALENT="$(printf '%s' "${REAL_COVERAGE}" | jq -S .)"
kubectl annotate seccompprofile "${SOURCE_PROFILE}" "${COVERAGE_KEY}=${EQUIVALENT}" --overwrite >/dev/null
DEQ="$(trace_import equivalent)"
[ "${DEQ}" = "${D1}" ] || fail "equivalent JSON changed CandidateDigest: ${D1} != ${DEQ}"

stage "CONTROLLED TEST DERIVATIVE — semantic mutation and stale authority"
MUTATED="$(printf '%s' "${REAL_COVERAGE}" | jq -c '.total += 1')"
kubectl annotate seccompprofile "${SOURCE_PROFILE}" "${COVERAGE_KEY}=${MUTATED}" --overwrite >/dev/null
D2="$(trace_import mutated)"
[ "${D2}" != "${D1}" ] || fail "semantic coverage mutation did not change CandidateDigest"

backend_state() {
  {
    kubectl get seccompprofile -o json | jq -S '[.items[]|{kind:"SeccompProfile",name:.metadata.name,rv:.metadata.resourceVersion}]'
    kubectl get networkpolicy -A -o json | jq -S '[.items[]|{kind:"NetworkPolicy",namespace:.metadata.namespace,name:.metadata.name,rv:.metadata.resourceVersion}]'
    kubectl get pod "${POD}" -n "${NAMESPACE}" -o json | jq -S '{kind:"Pod",namespace:.metadata.namespace,name:.metadata.name,rv:.metadata.resourceVersion}'
  }
}
BEFORE="$(backend_state)"
set +e
"${CLI_CMD[@]}" apply-proposal "${POD}" -n "${NAMESPACE}" --yes --skip=podlock \
  >"${ARTIFACTS_DIR}/merged-stale-apply.txt" 2>&1
STALE_RC=$?
set -e
[ "${STALE_RC}" -ne 0 ] || fail "old approval authorized semantically changed coverage"
grep -qi "digest mismatch" "${ARTIFACTS_DIR}/merged-stale-apply.txt" || { cat "${ARTIFACTS_DIR}/merged-stale-apply.txt" >&2; fail "stale rejection did not identify digest mismatch"; }
AFTER="$(backend_state)"
[ "${BEFORE}" = "${AFTER}" ] || fail "backend SeccompProfiles mutated before stale approval rejection"

stage "CONTROLLED TEST DERIVATIVES — optional coverage states"
kubectl annotate seccompprofile "${SOURCE_PROFILE}" "${COVERAGE_KEY}-" >/dev/null
trace_import absent >/dev/null
assert_merged_review "${ARTIFACTS_DIR}/merged-review-absent.txt"
grep -q "Syscall coverage: unavailable" "${ARTIFACTS_DIR}/merged-review-absent.txt" || fail "ABSENT not review-visible"

kubectl annotate seccompprofile "${SOURCE_PROFILE}" "${COVERAGE_KEY}={" --overwrite >/dev/null
trace_import malformed >/dev/null
assert_merged_review "${ARTIFACTS_DIR}/merged-review-malformed.txt"
grep -q "Syscall coverage: malformed metadata (no coverage value or confidence inferred)" "${ARTIFACTS_DIR}/merged-review-malformed.txt" || fail "MALFORMED not review-visible"
if grep -q "present in" "${ARTIFACTS_DIR}/merged-review-malformed.txt"; then fail "MALFORMED interpreted counts"; fi

kubectl annotate seccompprofile "${SOURCE_PROFILE}" "${COVERAGE_KEY}={\"version\":\"v2\"}" --overwrite >/dev/null
trace_import unsupported >/dev/null
assert_merged_review "${ARTIFACTS_DIR}/merged-review-unsupported.txt"
grep -q "Syscall coverage: unsupported schema v2" "${ARTIFACTS_DIR}/merged-review-unsupported.txt" || fail "UNSUPPORTED not review-visible"
if grep -q "present in" "${ARTIFACTS_DIR}/merged-review-unsupported.txt"; then fail "UNSUPPORTED interpreted counts"; fi

cat > "${ARTIFACTS_DIR}/merged-digests.txt" <<EVIDENCE
real-known-d1=${D1}
equivalent=${DEQ}
semantic-mutation-d2=${D2}
stale-apply-exit=${STALE_RC}
EVIDENCE

stage "proven"
cat <<SUMMARY
  REAL SPO OUTPUT       ${SOURCE_PROFILE}
  MERGE STRATEGY        Containers
  REAL COVERAGE         v1 total=${COVERAGE_TOTAL}
  D1                    ${D1}
  EQUIVALENT DIGEST     ${DEQ}
  MUTATED D2            ${D2}
  STALE AUTHORITY       rejected before backend mutation
  COVERAGE STATES       KNOWN / ABSENT / MALFORMED / UNSUPPORTED
  TRAININGHISTORY       ${TH_FS} filesystem accesses / 0 syscall accesses
  CLAIM BOUNDARY        governance evidence only; no behavioral denial claim
SUMMARY
