#!/usr/bin/env bash
# Real-node evidence for ADR-0009 and SPO syscall coverage (#3355).

set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAMESPACE="${MERGED_NAMESPACE:-landlock-genprof-merged}"
SPO_NAMESPACE="${SPO_NAMESPACE:-security-profiles-operator}"
RECORDING="${MERGED_RECORDING:-lgmerged}"
RECORDER_POD_A="${MERGED_RECORDER_POD_A:-merged-file}"
RECORDER_POD_B="${MERGED_RECORDER_POD_B:-merged-network}"
POD="${MERGED_POD:-merged-target}"
CONTAINER="${MERGED_CONTAINER:-tools}"
BINARY="${MERGED_BINARY:-/usr/bin/curl}"
IMAGE="${MERGED_IMAGE:-curlimages/curl:8.3.0}"
PROBE="/usr/local/bin/seccomp-probe"
DURATION="${MERGED_DURATION:-12s}"
RECORD_SECONDS="${MERGED_RECORD_SECONDS:-45}"
EXPECTED_CONTEXT="${EXPECTED_CONTEXT:-default}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/artifacts}"
COVERAGE_KEY="spo.x-k8s.io/syscall-coverage"
WORKLOAD_CMD="while true; do curl -sS file:///etc/hosts -o /dev/null 2>/dev/null || true; ${PROBE} getpid >/dev/null; sleep 2; done"
NETWORK_WORKLOAD_CMD="while true; do curl -sS --connect-timeout 1 http://127.0.0.1:9 -o /dev/null 2>/dev/null || true; ${PROBE} getpid >/dev/null; sleep 2; done"

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
  name: ${RECORDER_POD_A}
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
---
apiVersion: v1
kind: Pod
metadata:
  name: ${RECORDER_POD_B}
  namespace: ${NAMESPACE}
  labels:
    app: lg-merged-record
spec:
  terminationGracePeriodSeconds: 5
  restartPolicy: Never
  containers:
    - name: ${CONTAINER}
      image: ${IMAGE}
      command: ["sh", "-c", "${NETWORK_WORKLOAD_CMD}"]
      securityContext:
        runAsUser: 0
YAML

kubectl wait --for=condition=Ready pod -l app=lg-merged-record -n "${NAMESPACE}" --timeout=240s
for _ in $(seq 1 30); do
  if kubectl get pod -l app=lg-merged-record -n "${NAMESPACE}" -o json \
    | jq -e '.items | length == 2 and all(.[]; .status.containerStatuses[0].containerID != null)' >/dev/null; then
    break
  fi
  sleep 2
done
kubectl get pod -l app=lg-merged-record -n "${NAMESPACE}" -o json \
  | jq -e '.items | length == 2 and all(.[]; .status.containerStatuses[0].containerID != null)' >/dev/null \
  || fail "recorded pods never both reported container IDs"
sleep "${RECORD_SECONDS}"
kubectl delete pod "${RECORDER_POD_A}" "${RECORDER_POD_B}" -n "${NAMESPACE}" --wait=true --timeout=120s

# Wait for both real partials before deleting the recording, which is
# the normal SPO trigger for final merging.
PARTIALS=0
for _ in $(seq 1 60); do
  PARTIALS="$(kubectl get seccompprofile -l "spo.x-k8s.io/recording-id=${RECORDING},spo.x-k8s.io/partial" -o json 2>/dev/null | jq '.items | length')"
  [ "${PARTIALS}" -ge 2 ] && break
  sleep 5
done
[ "${PARTIALS}" -ge 2 ] || fail "SPO produced fewer than two partial SeccompProfiles"
EXPECTED_PARTIAL_A="${RECORDING}-${CONTAINER}-${RECORDER_POD_A}"
EXPECTED_PARTIAL_B="${RECORDING}-${CONTAINER}-${RECORDER_POD_B}"
EXPECTED_MERGED="${RECORDING}-${CONTAINER}"
kubectl get seccompprofile -l "spo.x-k8s.io/recording-id=${RECORDING},spo.x-k8s.io/partial" -o json \
  > "${ARTIFACTS_DIR}/merged-real-partials.json"
PARTIAL_NAMES="$(jq -r '.items[].metadata.name' "${ARTIFACTS_DIR}/merged-real-partials.json")"
for expected in "${EXPECTED_PARTIAL_A}" "${EXPECTED_PARTIAL_B}"; do
  printf '%s\n' "${PARTIAL_NAMES}" | grep -qx "${expected}" \
    || fail "real partial identity does not contain expected fixed-Pod suffix ${expected}: ${PARTIAL_NAMES}"
done
if printf '%s\n' "${PARTIAL_NAMES}" | grep -qx "${EXPECTED_MERGED}"; then
  fail "real partial still collides with final merged identity ${EXPECTED_MERGED}"
fi
jq -e --arg container "${CONTAINER}" \
  'all(.items[]; .metadata.labels["spo.x-k8s.io/container-id"] == $container)' \
  "${ARTIFACTS_DIR}/merged-real-partials.json" >/dev/null \
  || fail "real partials do not share the expected container merge group"

PARTIAL_A_SYSCALLS="$(jq -c --arg name "${EXPECTED_PARTIAL_A}" \
  '.items[] | select(.metadata.name == $name) | [.spec.syscalls[]?.names[]] | unique | sort' \
  "${ARTIFACTS_DIR}/merged-real-partials.json")"
PARTIAL_B_SYSCALLS="$(jq -c --arg name "${EXPECTED_PARTIAL_B}" \
  '.items[] | select(.metadata.name == $name) | [.spec.syscalls[]?.names[]] | unique | sort' \
  "${ARTIFACTS_DIR}/merged-real-partials.json")"
[ -n "${PARTIAL_A_SYSCALLS}" ] && [ -n "${PARTIAL_B_SYSCALLS}" ] \
  || fail "could not extract both real partial syscall sets"
[ "${PARTIAL_A_SYSCALLS}" != "${PARTIAL_B_SYSCALLS}" ] \
  || fail "real contributors produced identical syscall sets; widening was not demonstrated"

EXPECTED_UNION="$(jq -c '[.items[].spec.syscalls[]?.names[]] | unique | sort' \
  "${ARTIFACTS_DIR}/merged-real-partials.json")"
EXPECTED_COVERAGE="$(jq -cS \
  '[.items[] | ([.spec.syscalls[]?.names[]] | unique)[]] | group_by(.) | map({key: .[0], value: length}) | from_entries' \
  "${ARTIFACTS_DIR}/merged-real-partials.json")"
jq '{partials: [.items[] | {name: .metadata.name, container: .metadata.labels["spo.x-k8s.io/container-id"], syscalls: ([.spec.syscalls[]?.names[]] | unique | sort)}]}' \
  "${ARTIFACTS_DIR}/merged-real-partials.json" > "${ARTIFACTS_DIR}/merged-partial-syscalls.json"
echo "[evidence] real partial profiles before merge: ${PARTIAL_NAMES}"

kubectl get profilerecording "${RECORDING}" -n "${NAMESPACE}" -o yaml > "${ARTIFACTS_DIR}/merged-profilerecording.yaml"
# Deletion is the Containers merge trigger.  Do not wait for the
# ProfileRecording finalizer: the merged SeccompProfile is the completion
# signal, while cleanup may legitimately lag or remain finalizer-blocked.
kubectl delete profilerecording "${RECORDING}" -n "${NAMESPACE}" --wait=false

SOURCE_PROFILE=""
for _ in $(seq 1 60); do
  SOURCE_PROFILE="$(kubectl get seccompprofile -o json 2>/dev/null | jq -r --arg r "${RECORDING}" --arg n "${NAMESPACE}" \
    '.items[] | select(.metadata.labels["spo.x-k8s.io/recording-id"]==$r and .metadata.labels["spo.x-k8s.io/recording-namespace"]==$n and (.metadata.labels|has("spo.x-k8s.io/partial")|not)) | .metadata.name' | head -1)"
  [ -n "${SOURCE_PROFILE}" ] && break
  sleep 5
done
[ -n "${SOURCE_PROFILE}" ] || fail "SPO produced no final merged SeccompProfile"
[ "${SOURCE_PROFILE}" = "${EXPECTED_MERGED}" ] \
  || fail "final merged identity ${SOURCE_PROFILE}, expected ${EXPECTED_MERGED}"
kubectl get seccompprofile "${SOURCE_PROFILE}" -o json > "${ARTIFACTS_DIR}/merged-real-source.json"
jq -e '
  .apiVersion and .kind == "SeccompProfile" and
  (.metadata.name | type == "string" and length > 0) and
  (.spec | type == "object") and
  (.spec.syscalls | type == "array" and length > 0) and
  all(.spec.syscalls[]; (.names | type == "array" and length > 0))
' "${ARTIFACTS_DIR}/merged-real-source.json" >/dev/null \
  || fail "final merged SeccompProfile is structurally invalid"

# Preserve finalizer diagnostics, but never make recording disappearance a
# prerequisite for accepting the already-observed merged profile.
if kubectl get profilerecording "${RECORDING}" -n "${NAMESPACE}" -o json \
  > "${ARTIFACTS_DIR}/merged-profilerecording-after-merge.json" 2>/dev/null; then
  echo "[diagnostic] ProfileRecording remains while final profile is authoritative"
fi

ACTUAL_UNION="$(jq -c '[.spec.syscalls[]?.names[]] | unique | sort' "${ARTIFACTS_DIR}/merged-real-source.json")"
[ "${ACTUAL_UNION}" = "${EXPECTED_UNION}" ] \
  || fail "merged SeccompProfile syscall set is not the union of the real partial inputs"

REAL_COVERAGE="$(jq -r --arg k "${COVERAGE_KEY}" '.metadata.annotations[$k] // empty' "${ARTIFACTS_DIR}/merged-real-source.json")"
[ -n "${REAL_COVERAGE}" ] || fail "REAL SPO OUTPUT has no ${COVERAGE_KEY}; deployed SPO does not contain #3355"
printf '%s\n' "${REAL_COVERAGE}" | jq -e '.total as $total | .version=="v1" and ($total|type=="number" and .>=2) and (.syscalls|type=="object") and all(.syscalls[]; type=="number" and .>=1 and .<=$total)' >/dev/null \
  || fail "real coverage annotation violates supported v1 semantics: ${REAL_COVERAGE}"
COVERAGE_TOTAL="$(printf '%s' "${REAL_COVERAGE}" | jq '.total')"
[ "${COVERAGE_TOTAL}" -eq "${PARTIALS}" ] \
  || fail "coverage total ${COVERAGE_TOTAL} does not match real partial count ${PARTIALS}"
ACTUAL_COVERAGE="$(printf '%s' "${REAL_COVERAGE}" | jq -cS '.syscalls')"
[ "${ACTUAL_COVERAGE}" = "${EXPECTED_COVERAGE}" ] \
  || fail "coverage counts do not match presence across the real partial inputs"
ALL_SYSCALLS="$(printf '%s' "${REAL_COVERAGE}" | jq -r '.total as $total | .syscalls | to_entries[] | select(.value == $total) | .key')"
SUBSET_SYSCALLS="$(printf '%s' "${REAL_COVERAGE}" | jq -r '.total as $total | .syscalls | to_entries[] | select(.value < $total) | .key')"
[ -n "${ALL_SYSCALLS}" ] || fail "no syscall was present in all real contributors"
[ -n "${SUBSET_SYSCALLS}" ] || fail "all syscall counts equal total; actual widening was not demonstrated"
REC_LABEL="$(jq -r '.metadata.labels["spo.x-k8s.io/recording-id"] // empty' "${ARTIFACTS_DIR}/merged-real-source.json")"
REC_NS_LABEL="$(jq -r '.metadata.labels["spo.x-k8s.io/recording-namespace"] // empty' "${ARTIFACTS_DIR}/merged-real-source.json")"
PARTIAL_LABEL="$(jq -r '.metadata.labels["spo.x-k8s.io/partial"] // "<absent>"' "${ARTIFACTS_DIR}/merged-real-source.json")"
CONTAINER_LABEL="$(jq -r '.metadata.labels["spo.x-k8s.io/container-id"] // "<absent>"' "${ARTIFACTS_DIR}/merged-real-source.json")"
[ "${REC_LABEL}" = "${RECORDING}" ] || fail "merged profile recording-id mismatch"
[ "${REC_NS_LABEL}" = "${NAMESPACE}" ] || fail "merged profile recording-namespace mismatch"
[ "${PARTIAL_LABEL}" = "<absent>" ] || fail "final merged profile is still partial"
[ "${CONTAINER_LABEL}" = "<absent>" ] || fail "merged output unexpectedly claims unique container lineage"
echo "[ok] REAL SPO OUTPUT profile=${SOURCE_PROFILE} coverage.version=v1 total=${COVERAGE_TOTAL}"
echo "[evidence] syscalls present in all contributors: ${ALL_SYSCALLS}"
echo "[evidence] syscalls present in a subset of contributors: ${SUBSET_SYSCALLS}"

stage "authoritative coverage -> P2.6-B -> P3"
SPO_GOLDEN_LIVE=1 \
SPO_GOLDEN_NAMESPACE="${NAMESPACE}" \
SPO_GOLDEN_RECORDING="${RECORDING}" \
SPO_GOLDEN_PROFILE="${SOURCE_PROFILE}" \
SPO_GOLDEN_POD="${POD}" \
SPO_GOLDEN_CONTAINER="${CONTAINER}" \
SPO_GOLDEN_SUBJECT="${NAMESPACE}/${POD}:${CONTAINER}" \
SPO_GOLDEN_ATTEMPT="spo:${NAMESPACE}:${RECORDING}:${SOURCE_PROFILE}" \
SPO_GOLDEN_VERSION="${SPO_SOURCE_SHA:-305ee9fc8b3128f0ede4459b11f29e09ce61d5ce}" \
SPO_GOLDEN_SPO_NAMESPACE="${SPO_NAMESPACE}" \
SPO_GOLDEN_IMAGE="${SPO_IMAGE:-localhost/security-profiles-operator:305ee9fc8b3128f0}" \
GOCACHE="${GOCACHE:-/tmp/landlock-gocache}" \
go test ./internal/authority -run '^TestGoldenRealSPOCoverageEligibility$' -count=1 -v \
  | tee "${ARTIFACTS_DIR}/merged-authority-eligibility.txt"
grep -q 'p3=ELIGIBLE forged=REJECTED' "${ARTIFACTS_DIR}/merged-authority-eligibility.txt" \
  || fail "real SPO coverage did not traverse the authoritative P2.6-B/P3 path"

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

POSITIVE_SYSCALL="getpid"
printf '%s' "${ACTUAL_UNION}" | jq -e --arg syscall "${POSITIVE_SYSCALL}" 'index($syscall) != null' >/dev/null \
  || fail "positive probe ${POSITIVE_SYSCALL} is absent from the exact real merged profile"
NEGATIVE_SYSCALL=""
for candidate in getpriority sysinfo uname sched_yield; do
  if printf '%s' "${ACTUAL_UNION}" | jq -e --arg syscall "${candidate}" 'index($syscall) == null' >/dev/null; then
    NEGATIVE_SYSCALL="${candidate}"
    break
  fi
done
[ -n "${NEGATIVE_SYSCALL}" ] \
  || fail "NO_SAFE_NEGATIVE_PROBE: every supported safe probe is present in the exact real merged profile"
echo "[evidence] approved-policy probe membership: ${POSITIVE_SYSCALL}=present ${NEGATIVE_SYSCALL}=absent"

stage "control — same target without governed SeccompProfile"
kubectl exec -n "${NAMESPACE}" "${POD}" -c "${CONTAINER}" -- "${PROBE}" "${POSITIVE_SYSCALL}" \
  | tee "${ARTIFACTS_DIR}/merged-control-positive.txt"
grep -q "syscall=${POSITIVE_SYSCALL} .*errno=0" "${ARTIFACTS_DIR}/merged-control-positive.txt" \
  || fail "positive control did not report ${POSITIVE_SYSCALL} success"
kubectl exec -n "${NAMESPACE}" "${POD}" -c "${CONTAINER}" -- "${PROBE}" "${NEGATIVE_SYSCALL}" \
  | tee "${ARTIFACTS_DIR}/merged-control-negative.txt"
grep -q "syscall=${NEGATIVE_SYSCALL} .*errno=0" "${ARTIFACTS_DIR}/merged-control-negative.txt" \
  || fail "negative syscall ${NEGATIVE_SYSCALL} does not succeed in the unconfined control"

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
FIRST_SUBSET_SYSCALL="$(printf '%s\n' "${SUBSET_SYSCALLS}" | head -1)"
grep -q "${FIRST_SUBSET_SYSCALL}: present in .* contributing partial profiles" "${ARTIFACTS_DIR}/merged-review-known.txt" \
  || fail "review does not expose observed subset coverage for ${FIRST_SUBSET_SYSCALL}"

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

stage "governed runtime enforcement"
kubectl annotate seccompprofile "${SOURCE_PROFILE}" "${COVERAGE_KEY}=${REAL_COVERAGE}" --overwrite >/dev/null
D_ENFORCE="$(trace_import enforcement)"
[ "${D_ENFORCE}" = "${D1}" ] \
  || fail "restoring the real SPO coverage did not restore approved candidate D1: ${D_ENFORCE} != ${D1}"
assert_merged_review "${ARTIFACTS_DIR}/merged-review-enforcement.txt"
"${CLI_CMD[@]}" approve "${POD}" -n "${NAMESPACE}" --expected-digest "${D1}" --reason "governed seccomp enforcement"

kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" -o jsonpath='{.spec.spoSeccompProfile}' \
  > "${ARTIFACTS_DIR}/merged-approved-seccomp.yaml"
kubectl create --dry-run=client -f "${ARTIFACTS_DIR}/merged-approved-seccomp.yaml" -o json \
  > "${ARTIFACTS_DIR}/merged-approved-seccomp.json"
GOVERNED_PROFILE="$(jq -r '.metadata.name' "${ARTIFACTS_DIR}/merged-approved-seccomp.json")"
EXPECTED_LOCALHOST_PROFILE="operator/${GOVERNED_PROFILE}.json"
APPROVED_SYSCALLS="$(jq -c '[.spec.syscalls[]?.names[]] | unique | sort' "${ARTIFACTS_DIR}/merged-approved-seccomp.json")"
printf '%s' "${APPROVED_SYSCALLS}" | jq -e --arg syscall "${POSITIVE_SYSCALL}" 'index($syscall) != null' >/dev/null \
  || fail "positive probe is absent from the exact approved governed artifact"
printf '%s' "${APPROVED_SYSCALLS}" | jq -e --arg syscall "${NEGATIVE_SYSCALL}" 'index($syscall) == null' >/dev/null \
  || fail "negative probe is present in the exact approved governed artifact"

"${CLI_CMD[@]}" apply-proposal "${POD}" -n "${NAMESPACE}" --yes --restart \
  --skip=podlock --readiness-timeout=180s \
  | tee "${ARTIFACTS_DIR}/merged-enforcement-apply.txt"
INSTALLED_PROFILE="$(kubectl get seccompprofile "${GOVERNED_PROFILE}" -o jsonpath='{.status.localhostProfile}')"
[ "${INSTALLED_PROFILE}" = "${EXPECTED_LOCALHOST_PROFILE}" ] \
  || fail "SPO installed ${INSTALLED_PROFILE}, expected ${EXPECTED_LOCALHOST_PROFILE}"
kubectl wait --for=condition=Ready "pod/${POD}" -n "${NAMESPACE}" --timeout=180s
BOUND_PROFILE="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o json \
  | jq -r --arg container "${CONTAINER}" '.spec.containers[] | select(.name==$container) | .securityContext.seccompProfile.localhostProfile // empty')"
[ "${BOUND_PROFILE}" = "${EXPECTED_LOCALHOST_PROFILE}" ] \
  || fail "governed target references ${BOUND_PROFILE}, expected ${EXPECTED_LOCALHOST_PROFILE}"

kubectl exec -n "${NAMESPACE}" "${POD}" -c "${CONTAINER}" -- "${PROBE}" "${POSITIVE_SYSCALL}" \
  | tee "${ARTIFACTS_DIR}/merged-enforced-positive.txt"
grep -q "syscall=${POSITIVE_SYSCALL} .*errno=0" "${ARTIFACTS_DIR}/merged-enforced-positive.txt" \
  || fail "allowed syscall ${POSITIVE_SYSCALL} failed under the governed profile"
set +e
kubectl exec -n "${NAMESPACE}" "${POD}" -c "${CONTAINER}" -- "${PROBE}" "${NEGATIVE_SYSCALL}" \
  > "${ARTIFACTS_DIR}/merged-enforced-negative.txt" 2>&1
DENIED_RC=$?
set -e
cat "${ARTIFACTS_DIR}/merged-enforced-negative.txt"
[ "${DENIED_RC}" -ne 0 ] || fail "absent syscall ${NEGATIVE_SYSCALL} unexpectedly succeeded under the governed profile"
grep -q "syscall=${NEGATIVE_SYSCALL} .*errno=1" "${ARTIFACTS_DIR}/merged-enforced-negative.txt" \
  || fail "absent syscall ${NEGATIVE_SYSCALL} was not rejected with EPERM"

cat > "${ARTIFACTS_DIR}/merged-digests.txt" <<EVIDENCE
real-known-d1=${D1}
equivalent=${DEQ}
semantic-mutation-d2=${D2}
stale-apply-exit=${STALE_RC}
enforcement-digest=${D_ENFORCE}
positive-syscall=${POSITIVE_SYSCALL}
negative-syscall=${NEGATIVE_SYSCALL}
governed-profile=${GOVERNED_PROFILE}
localhost-profile=${BOUND_PROFILE}
negative-errno=1
EVIDENCE

stage "proven"
cat <<SUMMARY
  REAL SPO OUTPUT       ${SOURCE_PROFILE}
  MERGE STRATEGY        Containers
  REAL COVERAGE         v1 total=${COVERAGE_TOTAL}
  REAL UNION            merged set equals union of ${PARTIALS} partials
  WIDENING SIGNAL       syscall presence count below total
  D1                    ${D1}
  EQUIVALENT DIGEST     ${DEQ}
  MUTATED D2            ${D2}
  STALE AUTHORITY       rejected before backend mutation
  COVERAGE STATES       KNOWN / ABSENT / MALFORMED / UNSUPPORTED
  TRAININGHISTORY       ${TH_FS} filesystem accesses / 0 syscall accesses
  RUNTIME PROFILE       ${CONTAINER} -> ${BOUND_PROFILE}
  POSITIVE CONTROL      ${POSITIVE_SYSCALL} succeeded without and with governed seccomp
  NEGATIVE CONTROL      ${NEGATIVE_SYSCALL} succeeded without governed seccomp
  BEHAVIORAL DENIAL     ${NEGATIVE_SYSCALL} rejected with EPERM under governed seccomp
  CLAIM BOUNDARY        tested syscall boundary only; no universal least-privilege claim
SUMMARY
