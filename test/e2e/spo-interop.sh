#!/usr/bin/env bash
# spo-interop.sh — prove the downstream half of the SPO architecture
# against a real security-profiles-operator.
#
#   generated SeccompProfile (v1, cluster-scoped)
#     → governed proposal → digest-bound approval
#     → governed apply → real SPO reconciliation
#     → ADR-0007 readiness → identity → Gate 3
#     → workload binding → Pod Running
#
# What this does NOT prove, and must not be described as proving:
#
#   * SPO observation feeding landlock-genprof. ProfileRecording import is
#     docs/adr/0008 and is not implemented; every syscall here came from
#     this project's own tracer.
#   * behavioral syscall enforcement. Nothing attempts a denied syscall.
#     The proof level ends at "the workload is bound to the approved
#     profile and starts".
#
# Requires SPO installed (test/e2e/install-spo.sh). The Core E2E does not
# install SPO and carries the fail-closed proof for its absence instead.

set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

NAMESPACE="${NAMESPACE:-landlock-genprof-spo}"
POD="${POD:-nginx-demo}"
CONTAINER="${CONTAINER:-tools}"
BINARY="${BINARY:-/bin/sh}"
DURATION="${DURATION:-40s}"
EXPECTED_CONTEXT="${EXPECTED_CONTEXT-kind-landlock-genprof-e2e}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/artifacts}"

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

kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io >/dev/null 2>&1 \
  || fail "SPO is not installed — run test/e2e/install-spo.sh first"

# --- workload --------------------------------------------------------------
stage "deploying the workload"

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
# Reuse the Golden fixtures rather than inventing a second workload; they
# are the ones CI already exercises. Namespace is overridden so this runs
# alongside the Core E2E's own namespace without interfering.
sed "s/namespace: landlock-genprof-e2e/namespace: ${NAMESPACE}/g" \
  "${ROOT_DIR}/demo/golden/echo-service.yaml" | kubectl apply -n "${NAMESPACE}" -f -
sed "s/namespace: landlock-genprof-e2e/namespace: ${NAMESPACE}/g" \
  "${ROOT_DIR}/demo/golden/workload.yaml" | kubectl apply -n "${NAMESPACE}" -f -

kubectl wait --for=condition=Ready "pod/${POD}" -n "${NAMESPACE}" --timeout=240s

# --- observation -----------------------------------------------------------
stage "training run"

# --restart deliberately: the generated profile has defaultAction
# SCMP_ACT_ERRNO, so the container has to be able to start under whatever
# was observed. Attaching to an already-running container misses its
# startup syscalls entirely (docs/usage/target-restart.md), which is the
# difference between a profile the workload can boot under and one it cannot.
# The observed executable is the same shell command used by the governed
# container; keeping this identity aligned is required for a meaningful
# startup proof.
"${CLI_CMD[@]}" trace \
  --pod "${POD}" -n "${NAMESPACE}" \
  --container "${CONTAINER}" --binary "${BINARY}" \
  --duration "${DURATION}" --restart --history \
  --out "${ARTIFACTS_DIR}/${POD}-profile.yaml" \
  "--seccomp-profile-out=${ARTIFACTS_DIR}/${POD}-seccompprofile.yaml" \
  "--patched-manifest-out=${ARTIFACTS_DIR}/${POD}-patched.yaml" \
  >"${ARTIFACTS_DIR}/spo-interop-trace.log" 2>&1 &
TRACE_PID=$!

sleep 8
for _ in 1 2 3; do
  kubectl exec -n "${NAMESPACE}" "${POD}" -c "${CONTAINER}" -- \
    "${BINARY}" -sS file:///etc/hosts -o /dev/null || true
  kubectl exec -n "${NAMESPACE}" "${POD}" -c "${CONTAINER}" -- \
    "${BINARY}" -sS --max-time 2 -I "http://echo-8080-svc.${NAMESPACE}.svc.cluster.local:8080" || true
done
wait "${TRACE_PID}" || { cat "${ARTIFACTS_DIR}/spo-interop-trace.log" >&2; fail "trace failed"; }
sed -n '/WORKLOAD SECURITY ANALYSIS/,$p' "${ARTIFACTS_DIR}/spo-interop-trace.log"

# --- 1. GENERATED ----------------------------------------------------------
stage "1. GENERATED — proposal carries a modern cluster-scoped SeccompProfile"

kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" >/dev/null 2>&1 \
  || fail "no SecurityProfileProposal was published"

kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" \
  -o jsonpath='{.spec.spoSeccompProfile}' > "${ARTIFACTS_DIR}/approved-seccompprofile.yaml"
[ -s "${ARTIFACTS_DIR}/approved-seccompprofile.yaml" ] \
  || fail "the proposal carries no SPO SeccompProfile artifact"

# Convert once to JSON so every assertion below is structural rather than
# a grep over YAML text.
kubectl create --dry-run=client -o json -f "${ARTIFACTS_DIR}/approved-seccompprofile.yaml" \
  > "${ARTIFACTS_DIR}/approved-seccompprofile.json" 2>/dev/null \
  || fail "the generated SeccompProfile is not a valid manifest"

APPROVED_API="$(jq -r '.apiVersion' "${ARTIFACTS_DIR}/approved-seccompprofile.json")"
APPROVED_KIND="$(jq -r '.kind' "${ARTIFACTS_DIR}/approved-seccompprofile.json")"
PROFILE_NAME="$(jq -r '.metadata.name' "${ARTIFACTS_DIR}/approved-seccompprofile.json")"
APPROVED_NS="$(jq -r '.metadata.namespace // ""' "${ARTIFACTS_DIR}/approved-seccompprofile.json")"

echo "[info] apiVersion=${APPROVED_API} kind=${APPROVED_KIND} name=${PROFILE_NAME}"

[ "${APPROVED_API}" = "security-profiles-operator.x-k8s.io/v1" ] \
  || fail "apiVersion is ${APPROVED_API}, expected security-profiles-operator.x-k8s.io/v1"
[ "${APPROVED_KIND}" = "SeccompProfile" ] || fail "kind is ${APPROVED_KIND}"
[ -z "${APPROVED_NS}" ] \
  || fail "the generated SeccompProfile carries metadata.namespace=${APPROVED_NS}; it is cluster-scoped"
case "${PROFILE_NAME}" in
  lg-v1-*) ;;
  *) fail "profile name ${PROFILE_NAME} does not follow the lg-v1-* governed naming contract" ;;
esac

EXPECTED_PATH="operator/${PROFILE_NAME}.json"
echo "[ok] GENERATED — cluster-scoped v1 profile ${PROFILE_NAME}"

# --- 2. GOVERNED -----------------------------------------------------------
stage "2. GOVERNED — digest-bound approval"

"${CLI_CMD[@]}" review "${POD}" -n "${NAMESPACE}" | tee "${ARTIFACTS_DIR}/spo-interop-review.txt"
DIGEST="$(awk '/^Candidate digest: /{print $3; exit}' "${ARTIFACTS_DIR}/spo-interop-review.txt")"
[ -n "${DIGEST}" ] || fail "review produced no CandidateDigest"

# Cheap adversarial assertion, kept because it costs one command.
BAD_DIGEST="sha256:0000000000000000000000000000000000000000000000000000000000000000"
set +e
"${CLI_CMD[@]}" approve "${POD}" -n "${NAMESPACE}" --expected-digest "${BAD_DIGEST}" --reason "negative" \
  >"${ARTIFACTS_DIR}/spo-interop-approve-wrong.txt" 2>&1
BAD_RC=$?
set -e
[ "${BAD_RC}" -ne 0 ] || fail "approve succeeded with a wrong digest"
echo "[ok] wrong-digest approval refused"

"${CLI_CMD[@]}" approve "${POD}" -n "${NAMESPACE}" --expected-digest "${DIGEST}" --reason "spo interop"
APPROVED_DIGEST="$(kubectl get securityprofileproposal "${POD}" -n "${NAMESPACE}" -o jsonpath='{.status.approvedCandidateDigest}')"
[ "${APPROVED_DIGEST}" = "${DIGEST}" ] || fail "approved digest ${APPROVED_DIGEST} != reviewed ${DIGEST}"
echo "[ok] GOVERNED — approval bound to ${DIGEST}"

# The governed profile must not exist before the governed apply creates it.
if kubectl get seccompprofile "${PROFILE_NAME}" >/dev/null 2>&1; then
  fail "SeccompProfile ${PROFILE_NAME} already exists before apply-proposal"
fi

# --- 3-7. governed apply ---------------------------------------------------
stage "3. APPLIED — governed apply (this is the path under test)"

# PodLock is absent from this cluster and is skipped explicitly; the
# SeccompProfile is deliberately NOT skipped — it is the artifact under
# test. --restart opts into the workload binding.
set +e
"${CLI_CMD[@]}" apply-proposal "${POD}" -n "${NAMESPACE}" --yes --restart \
  --skip=podlock --readiness-timeout=180s \
  >"${ARTIFACTS_DIR}/spo-interop-apply.log" 2>&1
APPLY_RC=$?
set -e
cat "${ARTIFACTS_DIR}/spo-interop-apply.log"
[ "${APPLY_RC}" -eq 0 ] || fail "governed apply failed (exit ${APPLY_RC})"

kubectl get seccompprofile "${PROFILE_NAME}" >/dev/null 2>&1 \
  || fail "the cluster-scoped SeccompProfile ${PROFILE_NAME} was not applied"
echo "[ok] APPLIED — cluster-scoped ${PROFILE_NAME} exists"

# --- 4. RECONCILED ---------------------------------------------------------
stage "4. RECONCILED — real SPO populated status.localhostProfile"

# apply-proposal already waited for this; asserting it independently is
# what makes the E2E a proof rather than a restatement of the CLI's own
# behavior.
INSTALLED="$(kubectl get seccompprofile "${PROFILE_NAME}" -o jsonpath='{.status.localhostProfile}')"
echo "[info] status.localhostProfile=${INSTALLED}"
[ "${INSTALLED}" = "${EXPECTED_PATH}" ] \
  || fail "SPO materialized ${INSTALLED}, expected ${EXPECTED_PATH}"
echo "[ok] RECONCILED — SPO reports ${INSTALLED}"

# --- 5. IDENTITY -----------------------------------------------------------
stage "5. IDENTITY — live enforcement content equals the approved content"

kubectl get seccompprofile "${PROFILE_NAME}" -o json > "${ARTIFACTS_DIR}/live-seccompprofile.json"
IDENTITY_FIELDS='{defaultAction: .spec.defaultAction, architectures: .spec.architectures, syscalls: .spec.syscalls}'
jq -S "${IDENTITY_FIELDS}" "${ARTIFACTS_DIR}/approved-seccompprofile.json" > "${ARTIFACTS_DIR}/identity-approved.json"
jq -S "${IDENTITY_FIELDS}" "${ARTIFACTS_DIR}/live-seccompprofile.json"     > "${ARTIFACTS_DIR}/identity-live.json"

if ! diff -u "${ARTIFACTS_DIR}/identity-approved.json" "${ARTIFACTS_DIR}/identity-live.json"; then
  fail "the live profile's enforcement content differs from the approved content"
fi
echo "[ok] IDENTITY — defaultAction/architectures/syscalls match"

# --- 6. BOUND --------------------------------------------------------------
stage "6. BOUND — the workload references the governed profile"

BOUND_PATH="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o json \
  | jq -r --arg c "${CONTAINER}" '.spec.containers[] | select(.name==$c) | .securityContext.seccompProfile.localhostProfile // ""')"
echo "[info] container ${CONTAINER} localhostProfile=${BOUND_PATH}"
[ "${BOUND_PATH}" = "${EXPECTED_PATH}" ] \
  || fail "container references ${BOUND_PATH}, expected ${EXPECTED_PATH}"
echo "[ok] BOUND — ${CONTAINER} -> ${EXPECTED_PATH}"

# --- 7. RUNNING ------------------------------------------------------------
stage "7. RUNNING — the bound workload starts"

if ! kubectl wait --for=condition=Ready "pod/${POD}" -n "${NAMESPACE}" --timeout=180s; then
  echo "==== pod did not become Ready ====" >&2
  kubectl describe pod "${POD}" -n "${NAMESPACE}" >&2 || true

  # Distinguish the two failure causes, because they mean different
  # things. An unresolved localhostProfile would be an ADR-0007 failure —
  # the binding happened before the profile was usable. Anything else is
  # most likely profile completeness: defaultAction is SCMP_ACT_ERRNO, so
  # a syscall the training run never observed is denied at startup, which
  # is a synthesis-coverage problem and not a governed-apply problem.
  if kubectl describe pod "${POD}" -n "${NAMESPACE}" 2>/dev/null | grep -qiE "cannot load seccomp profile|no such file or directory"; then
    fail "the workload could not resolve its seccomp profile — this is an ADR-0007 readiness failure"
  fi
  fail "the workload did not start under the generated profile; see diagnostics above (likely profile coverage, not the governed apply path)"
fi

PHASE="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o jsonpath='{.status.phase}')"
[ "${PHASE}" = "Running" ] || fail "pod phase is ${PHASE}, expected Running"

RESTARTS="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o json | jq '[.status.containerStatuses[].restartCount] | add')"
WAITING="$(kubectl get pod "${POD}" -n "${NAMESPACE}" -o json | jq -r '[.status.containerStatuses[].state.waiting.reason // empty] | join(",")')"
echo "[info] phase=${PHASE} restarts=${RESTARTS} waiting=${WAITING:-none}"
case "${WAITING}" in
  *CrashLoopBackOff*|*CreateContainerError*)
    fail "container is in ${WAITING}"
    ;;
esac
echo "[ok] RUNNING — the workload is bound to the approved profile and running"

# --- summary ---------------------------------------------------------------
stage "SPO interoperability proven"
cat <<SUMMARY
  GENERATED   cluster-scoped v1 SeccompProfile ${PROFILE_NAME}
  GOVERNED    approval bound to ${DIGEST}
  APPLIED     submitted through the governed apply path
  RECONCILED  real SPO reported ${INSTALLED}
  IDENTITY    live enforcement content == approved content
  BOUND       ${CONTAINER} -> ${EXPECTED_PATH}
  RUNNING     pod ${PHASE}, restarts=${RESTARTS}

  NOT proven here: SPO observation ingestion (docs/adr/0008, not
  implemented) and behavioral syscall enforcement (no denial attempted).
SUMMARY
