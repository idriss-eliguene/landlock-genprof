#!/usr/bin/env bash
# scenario.sh — the canonical v0.2 hero demo.
#
#   "SPO learns more, and still cannot self-authorize."
#
# Thesis: landlock-genprof is the authorization boundary between runtime
# learning and runtime enforcement.
#
#   LEARNED != AUTHORIZED
#
# The narrative deliberately uses security-profiles-operator as the learner,
# because SPO is a strong, legitimate, upstream system that observes syscalls
# better than this project does. That is the point. If a weak learner's
# output were refused, nobody would be surprised. When SPO's output — valid,
# freshly recorded, better-informed — still cannot enforce itself, the
# boundary is the only thing that explains why.
#
# What this script is allowed to do: create workloads, run the real CLI, run
# kubectl, wait, select and format output, assert externally-visible facts,
# and clean up. What it must never do: compute a digest, decide whether an
# approval is valid, classify confidence, validate lineage, or predict
# whether an apply will succeed. Those are the product's job, and the
# product's exit status is the only authority recognized here.
#
# Every assertion below exists so the demo FAILS if its thesis is false.
# A demo that passes when the security property is broken is worse than no
# demo at all.
#
# Prerequisites: ./demo/setup.sh --with-cluster (real-node cluster, SPO
# installed, recordings A and B pre-baked), then ./demo/reset.sh.

set -euo pipefail

if [ "${1:-}" = "--paced" ]; then
  # Presenting live: each beat waits for a keypress instead of a timer.
  export DEMO_PACED=1
fi

# shellcheck source=demo/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

FAILURES=0
assert() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    printf '  [ok]   %s\n' "${desc}"
  else
    printf '  [FAIL] %s\n' "${desc}" >&2
    FAILURES=$((FAILURES + 1))
  fi
}
assert_eq() {
  local desc="$1" got="$2" want="$3"
  if [ "${got}" = "${want}" ]; then
    printf '  [ok]   %s\n' "${desc}"
  else
    printf '  [FAIL] %s (got %q, want %q)\n' "${desc}" "${got}" "${want}" >&2
    FAILURES=$((FAILURES + 1))
  fi
}
assert_ne() {
  local desc="$1" a="$2" b="$3"
  if [ "${a}" != "${b}" ]; then
    printf '  [ok]   %s\n' "${desc}"
  else
    printf '  [FAIL] %s (both %q)\n' "${desc}" "${a}" >&2
    FAILURES=$((FAILURES + 1))
  fi
}

# --- preconditions ---------------------------------------------------------

demo_require_cmd kubectl jq || exit 1
demo_check_context || exit 1
demo_resolve_cli || exit 1
demo_require_v02_cli || exit 1
demo_state_dir

for profile in "${DEMO_SOURCE_A}" "${DEMO_SOURCE_B}"; do
  kubectl get seccompprofile "${profile}" >/dev/null 2>&1 \
    || { demo_err "missing SPO source ${profile} — run ./demo/setup.sh first"; exit 1; }
done

# Reads the governed profile's name out of the rendered artifact. kubectl
# does the parsing (same trick test/e2e/spo-interop.sh uses) rather than
# adding a YAML library dependency to the demo host — and reading the name
# from the artifact, rather than recomputing it, keeps naming logic where it
# belongs: in the product.
governed_profile_name() {
  demo_proposal_field .spec.spoSeccompProfile \
    | kubectl create --dry-run=client -o json -f - 2>/dev/null \
    | jq -r '.metadata.name' 2>/dev/null || true
}

action_file() {
  kubectl exec -n "${DEMO_NAMESPACE}" "${DEMO_POD}" -c "${DEMO_CONTAINER}" -- \
    "${DEMO_BINARY}" -sS file:///etc/hosts -o /dev/null >/dev/null 2>&1 || true
}
# The workload's new behavior, driven through drift-action.sh so the demo
# keeps no second copy of it.
action_drift() {
  NAMESPACE="${DEMO_NAMESPACE}" POD="${DEMO_POD}" ACTION_CONTAINER="${DEMO_CONTAINER}" \
    CURL_BIN="${DEMO_BINARY}" bash "${DEMO_ROOT}/drift-action.sh" fs_new_path >/dev/null 2>&1 || true
}

action_network() {
  kubectl exec -n "${DEMO_NAMESPACE}" "${DEMO_POD}" -c "${DEMO_CONTAINER}" -- \
    "${DEMO_BINARY}" -sS --max-time 2 -o /dev/null \
    "http://echo-8080-svc.${DEMO_NAMESPACE}.svc.cluster.local:8080" >/dev/null 2>&1 || true
}

# trace_with <recording> <source-profile> <log> <tag> [with-drift]
#
# The real product path. Filesystem and network authority come from this
# project's own observation of the live workload; syscall authority is
# imported from the named SPO recording. Both land in one candidate.
#
# The workload's actions run INSIDE the trace window, which is the only way
# they are observed at all. That includes the drifted behavior: performing it
# before the window would change the container's filesystem without the
# tracer ever seeing it, and the diff would then have nothing to show — which
# is exactly what happened on the first run of this scenario.
trace_with() {
  local recording="$1" source_profile="$2" log="$3" tag="$4" with_drift="${5:-no}"
  "${CLI_CMD[@]}" trace \
    --pod "${DEMO_POD}" -n "${DEMO_NAMESPACE}" \
    --container "${DEMO_CONTAINER}" --binary "${DEMO_BINARY}" \
    --duration "${DEMO_DURATION}" --history \
    --seccomp-source=spo \
    --spo-recording "${recording}" \
    --spo-profile "${source_profile}" \
    --out "${DEMO_STATE}/${DEMO_POD}-profile.yaml" \
    "--candidate-out=${DEMO_STATE}/candidate-${tag}.json" \
    >"${log}" 2>&1 &
  local pid=$!
  sleep 8
  local i
  for i in 1 2 3; do
    action_file
    action_network
    if [ "${with_drift}" = "with-drift" ]; then
      action_drift
    fi
  done
  wait "${pid}" || { cat "${log}" >&2; demo_err "trace failed"; exit 1; }
}

# ===========================================================================
# 0:00 — COLD OPEN
# ===========================================================================
demo_stage "LEARNED"

A_COUNT="$(kubectl get seccompprofile "${DEMO_SOURCE_A}" -o json | jq '[.spec.syscalls[]?.names[]?] | length')"
A_STATE="$(kubectl get seccompprofile "${DEMO_SOURCE_A}" -o jsonpath='{.spec.state}')"
PROPOSAL_STATE="$(demo_proposal_field .status.approvalState)"

demo_panel "security-profiles-operator observed this workload" \
  "SeccompProfile   ${DEMO_SOURCE_A}" \
  "syscalls         ${A_COUNT}" \
  "spec.state       ${A_STATE}"
demo_panel "landlock-genprof" \
  "SecurityProfileProposal   ${PROPOSAL_STATE:-<none>}"

demo_note "SPO observed this workload and produced a valid policy."
demo_note "Nothing is enforcing it. That is not a bug."
printf '\n'
demo_note "LEARNED != AUTHORIZED"

assert_eq "the learned source profile starts inert" "${A_STATE}" "Disabled"
demo_beat 4

# ===========================================================================
# 0:25 — IMPORT / CANDIDATE A
# ===========================================================================
demo_stage "Import: SPO's derived policy enters as a candidate, not as authority"

trace_with "${DEMO_RECORDING_A}" "${DEMO_SOURCE_A}" "${DEMO_STATE}/trace-a.log" a
grep "Seccomp source: SPO derived policy" "${DEMO_STATE}/trace-a.log" || true

GOVERNED_NAME="$(governed_profile_name)"

demo_panel "SOURCE vs GOVERNED" \
  "SPO source profile   ${DEMO_SOURCE_A}   (state: ${A_STATE}, untouched)" \
  "governed copy        ${GOVERNED_NAME}"

assert "the governed copy has its own identity" test -n "${GOVERNED_NAME}"
assert_ne "governed identity differs from the SPO source" "${GOVERNED_NAME}" "${DEMO_SOURCE_A}"
assert_eq "SPO's source object is still inert after import" \
  "$(kubectl get seccompprofile "${DEMO_SOURCE_A}" -o jsonpath='{.spec.state}')" "Disabled"
demo_beat 3

# ===========================================================================
# ONE WORKLOAD, THREE DOMAINS
# ===========================================================================
demo_stage "One workload, three authority domains, one decision"

HAS_PODLOCK="$([ -n "$(demo_proposal_field .spec.podLock)" ] && echo yes || echo no)"
HAS_NETPOL="$([ -n "$(demo_proposal_field .spec.networkPolicy)" ] && echo yes || echo no)"
HAS_SECCOMP="$([ -n "$(demo_proposal_field .spec.spoSeccompProfile)" ] && echo yes || echo no)"

demo_panel "CANDIDATE A" \
  "filesystem   PodLock LandlockProfile        ${HAS_PODLOCK}   (observed here)" \
  "network      NetworkPolicy                  ${HAS_NETPOL}   (observed here)" \
  "syscalls     SPO SeccompProfile             ${HAS_SECCOMP}   (derived by SPO)" \
  "" \
  "                 ONE CANDIDATE / ONE DIGEST / ONE DECISION"

demo_note "SPO did not observe the filesystem authority. It does not record it."
assert_eq "the candidate carries filesystem authority" "${HAS_PODLOCK}" "yes"
assert_eq "the candidate carries syscall authority" "${HAS_SECCOMP}" "yes"
demo_beat 4

# ===========================================================================
# 1:05 — REVIEW A
# ===========================================================================
demo_stage "Review candidate A"

"${CLI_CMD[@]}" review "${DEMO_POD}" -n "${DEMO_NAMESPACE}" | tee "${DEMO_STATE}/review-a.txt"

DIGEST_A="$(awk '/^Candidate digest: /{print $3; exit}' "${DEMO_STATE}/review-a.txt")"
assert "review produced a candidate digest" test -n "${DIGEST_A}"
assert "review names SPO as the seccomp source" \
  grep -q "Source: security-profiles-operator" "${DEMO_STATE}/review-a.txt"
assert "review reports derived policy, not observation" \
  grep -q "Origin: derived policy" "${DEMO_STATE}/review-a.txt"
assert "review reports coverage as unknown (SPO v1.0.0 emits none)" \
  grep -q "Coverage: unknown" "${DEMO_STATE}/review-a.txt"
assert "review refuses to invent a confidence tier for derived policy" \
  grep -q "Confidence: not applicable" "${DEMO_STATE}/review-a.txt"
demo_beat 4

# ===========================================================================
# 1:55 — APPROVE A
# ===========================================================================
demo_stage "HUMAN DECISION — approve exactly this content"

"${CLI_CMD[@]}" approve "${DEMO_POD}" -n "${DEMO_NAMESPACE}" \
  --expected-digest "${DIGEST_A}" --reason "reviewed: candidate A"

APPROVED_A="$(demo_proposal_field .status.approvedCandidateDigest)"
demo_panel "APPROVED" \
  "reviewed digest   ${DIGEST_A}" \
  "approved digest   ${APPROVED_A}"
assert_eq "the approval is bound to exactly the reviewed content" "${APPROVED_A}" "${DIGEST_A}"
demo_beat 3

# ===========================================================================
# 2:15 — THE WORKLOAD CHANGES
# ===========================================================================
demo_stage "The workload changes"

DRIFT_PATH="$(bash "${DEMO_ROOT}/drift-action.sh" path)"
NAMESPACE="${DEMO_NAMESPACE}" POD="${DEMO_POD}" ACTION_CONTAINER="${DEMO_CONTAINER}" \
  CURL_BIN="${DEMO_BINARY}" bash "${DEMO_ROOT}/drift-action.sh" fs_new_path

demo_note "the workload now writes ${DRIFT_PATH}"
demo_note "and the learner recorded it calling a service it never called before"
demo_beat 3

# ===========================================================================
# 2:40 — CANDIDATE B
# ===========================================================================
demo_stage "The learner learned more"

trace_with "${DEMO_RECORDING_B}" "${DEMO_SOURCE_B}" "${DEMO_STATE}/trace-b.log" b with-drift

"${CLI_CMD[@]}" review "${DEMO_POD}" -n "${DEMO_NAMESPACE}" > "${DEMO_STATE}/review-b.txt"
DIGEST_B="$(awk '/^Candidate digest: /{print $3; exit}' "${DEMO_STATE}/review-b.txt")"

demo_panel "THE PROPOSAL MOVED ON. THE APPROVAL DID NOT." \
  "candidate now   ${DIGEST_B}" \
  "approved        ${APPROVED_A}"

assert "review produced a digest for candidate B" test -n "${DIGEST_B}"
assert_ne "candidate B is not candidate A" "${DIGEST_B}" "${DIGEST_A}"
assert_eq "the stored approval still points at candidate A" \
  "$(demo_proposal_field .status.approvedCandidateDigest)" "${DIGEST_A}"
demo_beat 3

# ===========================================================================
# 3:10 — THE MONEY SHOT
# ===========================================================================
demo_stage "NOT AUTHORIZED"

set +e
"${CLI_CMD[@]}" apply-proposal "${DEMO_POD}" -n "${DEMO_NAMESPACE}" --yes \
  >"${DEMO_STATE}/apply-stale.log" 2>&1
STALE_RC=$?
set -e
cat "${DEMO_STATE}/apply-stale.log"
printf '\n  exit status: %d\n' "${STALE_RC}"

# Exit 1 is the contract for a refused approval (ADR-0001: non-blocking
# finding); ADR-0007's readiness refusal is exit 2. Asserted, not assumed.
assert_eq "governed apply refused a stale approval with the contract's exit code" "${STALE_RC}" "1"
assert "the refusal names the digest mismatch" \
  grep -q "approved candidate digest mismatch" "${DEMO_STATE}/apply-stale.log"

demo_beat 5
demo_note "SPO learned a better policy."
demo_note "That still did not give it authority."
demo_beat 4

# ===========================================================================
# 3:30 — NOTHING WAS APPLIED
# ===========================================================================
demo_stage "Nothing was applied"

GOVERNED_B="$(governed_profile_name)"
BOUND_NOW="$(kubectl get pod "${DEMO_POD}" -n "${DEMO_NAMESPACE}" -o json \
  | jq -r --arg c "${DEMO_CONTAINER}" '.spec.containers[] | select(.name==$c) | .securityContext.seccompProfile.localhostProfile // ""')"

demo_panel "CLUSTER STATE AFTER THE REFUSAL" \
  "governed profile in cluster   $(kubectl get seccompprofile "${GOVERNED_B}" >/dev/null 2>&1 && echo present || echo absent)" \
  "workload seccomp binding      ${BOUND_NOW:-<none>}" \
  "SPO source ${DEMO_SOURCE_B}   state=$(kubectl get seccompprofile "${DEMO_SOURCE_B}" -o jsonpath='{.spec.state}')"

assert "unauthorized candidate B was not applied" \
  bash -c "! kubectl get seccompprofile '${GOVERNED_B}' >/dev/null 2>&1"
assert_eq "the workload was not rebound under unauthorized authority" "${BOUND_NOW}" ""
assert_eq "SPO's source profile is still inert" \
  "$(kubectl get seccompprofile "${DEMO_SOURCE_B}" -o jsonpath='{.spec.state}')" "Disabled"
demo_beat 3

# ===========================================================================
# 3:45 — WHAT CHANGED?
# ===========================================================================
demo_stage "WHAT CHANGED?"

demo_note "filesystem — rule by rule, from this project's own observation:"
set +e
"${CLI_CMD[@]}" diff "${DEMO_STATE}/candidate-a.json" "${DEMO_STATE}/candidate-b.json" \
  | tee "${DEMO_STATE}/diff.txt"
set -e

assert "the filesystem diff shows the newly observed path" \
  grep -q "$(dirname "${DRIFT_PATH}")" "${DEMO_STATE}/diff.txt"

# There is no seccomp semantic diff in v0.2 — `diff` compares Landlock
# candidates. Rather than pretend otherwise, show what genuinely changed
# about the seccomp domain: which recording it came from, and how much
# authority each carried.
B_COUNT="$(kubectl get seccompprofile "${DEMO_SOURCE_B}" -o json | jq '[.spec.syscalls[]?.names[]?] | length')"
demo_panel "SECCOMP — provenance changed" \
  "candidate A   recording ${DEMO_RECORDING_A}   source ${DEMO_SOURCE_A}   ${A_COUNT} syscalls" \
  "candidate B   recording ${DEMO_RECORDING_B}   source ${DEMO_SOURCE_B}   ${B_COUNT} syscalls" \
  "" \
  "v0.2 diffs Landlock candidates; seccomp changes are shown by provenance."

assert "review B records the new recording as the seccomp source" \
  grep -q "${DEMO_RECORDING_B}" "${DEMO_STATE}/review-b.txt"
demo_beat 4

# ===========================================================================
# TRAININGHISTORY PROOF PANEL
# ===========================================================================
demo_stage "Derived policy never became our evidence"

kubectl get traininghistory -n "${DEMO_NAMESPACE}" -o json > "${DEMO_STATE}/history.json"
TH_NAME="$(jq -r '[.items[].metadata.name] | join(", ")' "${DEMO_STATE}/history.json")"
TH_FS="$(jq '[.items[].spec.filesystemAccesses // [] | length] | add // 0' "${DEMO_STATE}/history.json")"
TH_SYS="$(jq '[.items[].spec.syscallAccesses // [] | length] | add // 0' "${DEMO_STATE}/history.json")"

demo_panel "TrainingHistory ${TH_NAME}" \
  "filesystemAccesses   ${TH_FS}" \
  "syscallAccesses      ${TH_SYS}"
demo_note "SPO's derived syscalls never became our observation evidence."

assert "filesystem observation is real" test "${TH_FS}" -gt 0
assert_eq "no SPO-derived syscall became training evidence" "${TH_SYS}" "0"
demo_beat 4

# ===========================================================================
# 4:30 — APPROVE B
# ===========================================================================
demo_stage "HUMAN DECISION — approve the change that actually happened"

"${CLI_CMD[@]}" approve "${DEMO_POD}" -n "${DEMO_NAMESPACE}" \
  --expected-digest "${DIGEST_B}" --reason "reviewed: candidate B"

APPROVED_B="$(demo_proposal_field .status.approvedCandidateDigest)"
assert_eq "the new approval is bound to candidate B" "${APPROVED_B}" "${DIGEST_B}"
demo_beat 2

# ===========================================================================
# 4:50 — GOVERNED APPLY
# ===========================================================================
demo_stage "AUTHORIZED — digest, approval, readiness, identity, then binding"

set +e
"${CLI_CMD[@]}" apply-proposal "${DEMO_POD}" -n "${DEMO_NAMESPACE}" --yes --restart \
  --skip=podlock --readiness-timeout=180s \
  2>&1 | tee "${DEMO_STATE}/apply-b.log"
APPLY_RC="${PIPESTATUS[0]}"
set -e
assert_eq "governed apply succeeded against a current approval" "${APPLY_RC}" "0"

kubectl wait --for=condition=Ready "pod/${DEMO_POD}" -n "${DEMO_NAMESPACE}" --timeout=180s >/dev/null 2>&1 || true

EXPECTED_PATH="operator/${GOVERNED_B}.json"
INSTALLED="$(kubectl get seccompprofile "${GOVERNED_B}" -o jsonpath='{.status.localhostProfile}' 2>/dev/null || true)"
BOUND="$(kubectl get pod "${DEMO_POD}" -n "${DEMO_NAMESPACE}" -o json \
  | jq -r --arg c "${DEMO_CONTAINER}" '.spec.containers[] | select(.name==$c) | .securityContext.seccompProfile.localhostProfile // ""')"
PHASE="$(kubectl get pod "${DEMO_POD}" -n "${DEMO_NAMESPACE}" -o jsonpath='{.status.phase}')"
RESTARTS="$(kubectl get pod "${DEMO_POD}" -n "${DEMO_NAMESPACE}" -o json | jq '[.status.containerStatuses[].restartCount] | add')"
SRC_FINAL="$(kubectl get seccompprofile "${DEMO_SOURCE_B}" -o jsonpath='{.spec.state}')"

assert_eq "SPO reconciled the governed profile onto the node" "${INSTALLED}" "${EXPECTED_PATH}"
assert_eq "the workload is bound to the governed profile" "${BOUND}" "${EXPECTED_PATH}"
assert "the workload is never bound to the SPO source" \
  bash -c "case '${BOUND}' in *${DEMO_SOURCE_B}*) exit 1;; *) exit 0;; esac"
assert_eq "the workload is Running" "${PHASE}" "Running"
assert_eq "no unexpected restarts" "${RESTARTS}" "0"
assert_eq "SPO's source profile is still inert at the end" "${SRC_FINAL}" "Disabled"

# ===========================================================================
# 5:20 — FINAL PROOF
# ===========================================================================
demo_stage "AUTHORIZED"

demo_panel "WHAT REACHED ENFORCEMENT" \
  "SPO OBSERVED        ✓   ${DEMO_SOURCE_B} (${B_COUNT} syscalls)" \
  "POLICY DERIVED      ✓   imported as derived policy, not evidence" \
  "HUMAN APPROVED      ✓   ${APPROVED_B}" \
  "BACKEND READY       ✓   ${INSTALLED}" \
  "IDENTITY VERIFIED   ✓   approved content == enforced content" \
  "WORKLOAD BOUND      ✓   ${BOUND}"
demo_panel "FINAL STATE" \
  "SPO source profile   ${DEMO_SOURCE_B}   ${SRC_FINAL}" \
  "governed profile     ${GOVERNED_B}   reconciled" \
  "workload             ${DEMO_POD}   ${PHASE}, restarts=${RESTARTS}"

printf '\n'
demo_note "Learning is automatic. Authority is not."
printf '\n'

if [ "${FAILURES}" -ne 0 ]; then
  demo_err "${FAILURES} demo assertion(s) failed — the narrative is not supported by the cluster"
  exit 1
fi
demo_note "all demo assertions passed"
