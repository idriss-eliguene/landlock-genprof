#!/usr/bin/env bash
# scenario.sh — the canonical v0.2 demo: from observed behavior to
# governed authority.
#
#   OBSERVED != APPROVED
#
# This orchestrator is deliberately dumb. It sequences real commands and
# prints what they really said. It does not compute a candidate digest,
# classify confidence, synthesize a profile, revalidate an approval, or
# predict whether an apply will succeed. Where it needs a digest, it
# passes through the string the product itself printed. The only thing it
# ever branches on is a real command's exit status — which is the
# product's own verdict, not a reimplementation of one.
#
# Prerequisite: ./demo/setup.sh, then ./demo/reset.sh.
#
#   ./demo/scenario.sh            # full canonical scenario
#   ./demo/scenario.sh --paced    # pause between stages (live presenting)

set -euo pipefail

# shellcheck source=demo/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

PACED=0
if [ "${1:-}" = "--paced" ]; then
  PACED=1
fi

pause() {
  [ "$PACED" -eq 1 ] || return 0
  printf '\n  [enter to continue] '
  read -r _ || true
}

demo_require_cmd kubectl || exit 1
demo_check_context || exit 1
demo_resolve_cli || exit 1
demo_state_dir

NS="${DEMO_NAMESPACE}"
POD="${DEMO_POD}"
S="${DEMO_STATE}"

action() {
  NAMESPACE="$NS" POD="$POD" CONTAINER="${DEMO_CONTAINER}" \
    ACTION_CONTAINER="${DEMO_CONTAINER}" CURL_BIN="${DEMO_BINARY}" \
    bash "${DEMO_ROOT}/golden/run-actions.sh" "$1"
}

drift() {
  NAMESPACE="$NS" POD="$POD" ACTION_CONTAINER="${DEMO_CONTAINER}" \
    CURL_BIN="${DEMO_BINARY}" bash "${DEMO_ROOT}/drift-action.sh" "$1"
}

# run_trace <run-label> <candidate-out> [extra action ...]
# Starts a real training run and drives deterministic workload behavior
# while it observes. The trace command is the product; this only decides
# when to poke the workload.
run_trace() {
  local label="$1" candidate_out="$2"; shift 2
  local log="${S}/trace-${label}.log"

  "${CLI_CMD[@]}" trace \
    --pod "$POD" -n "$NS" \
    --container "${DEMO_CONTAINER}" --binary "${DEMO_BINARY}" \
    --duration "${DEMO_DURATION}" --history \
    --out "${S}/${POD}-profile.yaml" \
    "--network-out=${S}/${POD}-networkpolicy.yaml" \
    "--seccomp-profile-out=${S}/${POD}-seccompprofile.yaml" \
    "--patched-manifest-out=${S}/${POD}-patched.yaml" \
    "--candidate-out=${candidate_out}" \
    "--events-out=${S}/${POD}-events-${label}.json" \
    >"${log}" 2>&1 &
  local trace_pid=$!

  # Give the tracer a moment to attach before generating behavior.
  sleep 3
  local a
  for a in "$@"; do
    # Actions are real work inside the real workload; their curl progress
    # meters are not. Suppress on success, show everything on failure.
    if [[ "$a" == drift:* ]]; then
      drift "${a#drift:}" >>"${S}/actions.log" 2>&1 || { demo_err "action ${a} failed"; tail -20 "${S}/actions.log" >&2; exit 1; }
    else
      action "$a" >>"${S}/actions.log" 2>&1 || { demo_err "action ${a} failed"; tail -20 "${S}/actions.log" >&2; exit 1; }
    fi
  done

  wait "$trace_pid"
  local rc=$?
  if [ "$rc" -ne 0 ]; then
    demo_err "training run '${label}' failed (exit ${rc}); log follows"
    cat "${log}" >&2
    exit "$rc"
  fi
  # Show the product's own summary, not a paraphrase of it.
  sed -n '/WORKLOAD SECURITY ANALYSIS/,$p' "${log}" || cat "${log}"
}

# ---------------------------------------------------------------------------
demo_stage "STAGE 1 — Baseline: an unrestricted workload, no proposal yet"

kubectl get pod "$POD" -n "$NS" \
  -o jsonpath='{.spec.containers[0].securityContext}{"\n"}'
demo_note "^ the workload's own securityContext"
printf '\n'
kubectl get securityprofileproposal -n "$NS" 2>&1 | sed 's/^/  /'
kubectl get traininghistory -n "$NS" 2>&1 | sed 's/^/  /'
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 2 — Observation: three training runs on the real workload"

demo_note "Each run drives deterministic behavior:"
demo_note "  every run  : read /etc/hosts, egress :8080"
demo_note "  runs 1-2   : write /var/tmp/... , egress :8081"
demo_note "  run 1 only : write /srv/nginx/data/transient, egress :8082"
printf '\n'

demo_note "--- training run 1/3 ---"
run_trace run1 "${S}/candidate-run1.json" \
  fs_common net_common fs_2_of_3 net_2_of_3 fs_1_of_3 net_1_of_3
pause

demo_note "--- training run 2/3 ---"
run_trace run2 "${S}/candidate-run2.json" \
  fs_common net_common fs_2_of_3 net_2_of_3
pause

demo_note "--- training run 3/3 ---"
run_trace run3 "${S}/candidate-a.json" \
  fs_common net_common
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 3 — Evidence accumulated across runs"

kubectl get traininghistory -n "$NS" \
  -o custom-columns='NAME:.metadata.name,RUNS:.spec.runsRecorded' 2>&1 | sed 's/^/  /'
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 4 — Explain: why each rule exists, and how well it is known"

"${CLI_CMD[@]}" explain --candidate-file "${S}/candidate-a.json" \
  | tee "${S}/explain-a.txt"
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 5 — Review candidate A"

"${CLI_CMD[@]}" review "$POD" -n "$NS" | tee "${S}/review-a.txt"

# Capture the digest the product printed, purely to pass it back as a
# command-line argument. The demo never computes this value.
DIGEST_A="$(awk '/^Candidate digest: /{print $3; exit}' "${S}/review-a.txt")"
if [ -z "$DIGEST_A" ]; then
  demo_err "no 'Candidate digest:' line in review output — cannot continue"
  exit 1
fi
printf '\n'
demo_note "captured from review output: ${DIGEST_A}"
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 6 — Approve exactly candidate A"

"${CLI_CMD[@]}" approve "$POD" -n "$NS" \
  --expected-digest "$DIGEST_A" \
  --reason "reviewed with the platform team" | tee "${S}/approve-a.txt"

printf '\n'
demo_note "the approval, read back from the cluster:"
kubectl get securityprofileproposal "$POD" -n "$NS" -o json \
  | python3 -c 'import json,sys; s=json.load(sys.stdin).get("status",{}); [print(f"  {k}: {s[k]}") for k in ("approvalState","approvedCandidateDigest","approvalMechanismVersion") if k in s]'
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 7 — The workload's behavior changes"

DRIFT_PATH="$(drift path)"
demo_note "the workload starts writing a path it has never written before:"
demo_note "  ${DRIFT_PATH}"
printf '\n'
demo_note "nobody re-approves anything. This is the whole point."
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 8 — Trace again (routine: the workload changed)"

run_trace drift "${S}/candidate-b.json" \
  fs_common net_common drift:fs_new_path
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 9 — The proposal moved on; the approval did not"

demo_note "approval status still recorded on the proposal:"
kubectl get securityprofileproposal "$POD" -n "$NS" -o json \
  | python3 -c 'import json,sys; s=json.load(sys.stdin).get("status",{}); [print(f"  {k}: {s[k]}") for k in ("approvalState","approvedCandidateDigest","approvalMechanismVersion") if k in s]'
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 10 — Before the apply attempt: is anything already applied?"

kubectl get networkpolicy -n "$NS" 2>&1 | sed 's/^/  /'
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 11 — Governed apply, against a stale approval"

set +e
"${CLI_CMD[@]}" apply-proposal "$POD" -n "$NS" --yes \
  --skip=podlock,spo-seccompprofile \
  >"${S}/apply-stale.out" 2>"${S}/apply-stale.err"
APPLY_RC=$?
set -e

cat "${S}/apply-stale.out"
printf '\n'
# Verbatim. The product's wording is the message; the demo does not
# restate, summarize or dramatize it.
cat "${S}/apply-stale.err"
printf '\n'
demo_note "exit status: ${APPLY_RC}"

if [ "$APPLY_RC" -eq 0 ]; then
  demo_err "the governed apply SUCCEEDED against the stale approval."
  demo_err "That is not the expected demo state. Stopping rather than narrating it."
  exit 1
fi
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 12 — After the refusal: still nothing applied"

kubectl get networkpolicy -n "$NS" 2>&1 | sed 's/^/  /'
printf '\n'
demo_note "the stale candidate was rejected before the first API application."
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 13 — What actually changed?"

# diff's exit-code contract is 0 = identical, 1 = differences found,
# 3 = usage/IO error (see `diff --help`). Exit 1 therefore also makes
# main() print its generic non-zero-exit line to stderr, which arrives
# before the diff itself and reads like a failure. Keep stderr in the
# state dir so the rule lines land in order; the exit status is still
# shown, unmodified.
set +e
"${CLI_CMD[@]}" diff "${S}/candidate-a.json" "${S}/candidate-b.json" \
  2>"${S}/diff-a-b.err" | tee "${S}/diff-a-b.txt"
DIFF_RC=${PIPESTATUS[0]}
set -e
printf '\n'
demo_note "diff exit status: ${DIFF_RC}  (0 = identical, 1 = differences found)"
demo_note "^ the authority that changed, rule by rule"
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 14 — Review candidate B"

"${CLI_CMD[@]}" review "$POD" -n "$NS" | tee "${S}/review-b.txt"
DIGEST_B="$(awk '/^Candidate digest: /{print $3; exit}' "${S}/review-b.txt")"
if [ -z "$DIGEST_B" ]; then
  demo_err "no 'Candidate digest:' line in review output — cannot continue"
  exit 1
fi
printf '\n'
demo_note "candidate A: ${DIGEST_A}"
demo_note "candidate B: ${DIGEST_B}"
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 15 — Approve candidate B"

"${CLI_CMD[@]}" approve "$POD" -n "$NS" \
  --expected-digest "$DIGEST_B" \
  --reason "re-reviewed after the workload changed" | tee "${S}/approve-b.txt"
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 16 — Governed apply, against the current approval"

"${CLI_CMD[@]}" apply-proposal "$POD" -n "$NS" --yes \
  --skip=podlock,spo-seccompprofile | tee "${S}/apply-b.out"
pause

# ---------------------------------------------------------------------------
demo_stage "STAGE 17 — Final state"

demo_note "approval on the proposal:"
kubectl get securityprofileproposal "$POD" -n "$NS" -o json \
  | python3 -c 'import json,sys; s=json.load(sys.stdin).get("status",{}); [print(f"  {k}: {s[k]}") for k in ("approvalState","approvedCandidateDigest","approvalMechanismVersion") if k in s]'
printf '\n'
demo_note "applied to the Kubernetes API:"
kubectl get networkpolicy -n "$NS" 2>&1 | sed 's/^/  /'

printf '\n'
demo_rule
printf '  OBSERVED != APPROVED\n'
printf '  Observation proposes authority. A human authorizes it.\n'
printf '  The apply path revalidates before it changes anything.\n'
demo_rule
printf '\n'
demo_note "raw outputs from this run: ${S}"
