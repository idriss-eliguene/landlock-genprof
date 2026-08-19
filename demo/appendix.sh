#!/usr/bin/env bash
# appendix.sh — the technical appendix to the canonical demo.
#
# Everything here is real and worth showing to an engineer, and none of it
# belongs in the 5-minute hero cut. The hero demo answers "why should this
# exist"; this answers "does it actually hold up".
#
# Reuses the same cluster, the same workload and the same product the hero
# demo uses. It is deliberately NOT a second demo implementation: it runs
# after scenario.sh, against the state scenario.sh left behind.
#
# Covered here:
#
#   doctor / abi              environment qualification
#   wrong-digest rejection    the adversarial approval case
#   policy / evidence         inspection surfaces
#   verify                    candidate verification
#   source-lifecycle proof    SPO's object was never adopted or mutated
#   scope limits              mergeStrategy None, coverage unknown
#
# Run: ./demo/appendix.sh

set -euo pipefail

# shellcheck source=demo/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

FAILURES=0
note_fail() { printf '  [FAIL] %s\n' "$*" >&2; FAILURES=$((FAILURES + 1)); }
note_ok()   { printf '  [ok]   %s\n' "$*"; }

demo_require_cmd kubectl jq || exit 1
demo_check_context || exit 1
demo_resolve_cli || exit 1

# --- environment qualification --------------------------------------------
# In the hero demo these are a setup still, not live commands: they qualify
# the environment, they do not advance the argument. Here they run for real.
demo_stage "APPENDIX 1 — environment qualification"

demo_note "doctor: operational prerequisites for this host/cluster"
set +e
"${CLI_CMD[@]}" doctor
DOCTOR_RC=$?
set -e
demo_note "doctor exit: ${DOCTOR_RC}"

demo_note "abi: the Landlock ABI this kernel reports"
set +e
"${CLI_CMD[@]}" abi
ABI_RC=$?
set -e
demo_note "abi exit: ${ABI_RC}"

# Neither is asserted to be 0: they legitimately report findings on hosts
# that cannot enforce Landlock, and the demo must not pretend otherwise.
demo_note "these qualify the environment; they do not prove host trust"

# --- adversarial approval --------------------------------------------------
demo_stage "APPENDIX 2 — wrong-digest approval is refused"

# Written to a file before parsing rather than piped: `awk ... {exit}` closes
# the pipe early, review takes SIGPIPE, and under `set -o pipefail` that ends
# the script with no output at all — which is exactly how this failed in run
# 32274599858.
demo_state_dir
"${CLI_CMD[@]}" review "${DEMO_POD}" -n "${DEMO_NAMESPACE}" > "${DEMO_STATE}/appendix-review.txt"
CURRENT_DIGEST="$(awk '/^Candidate digest: /{print $3; exit}' "${DEMO_STATE}/appendix-review.txt")"
BAD="sha256:0000000000000000000000000000000000000000000000000000000000000000"

set +e
"${CLI_CMD[@]}" approve "${DEMO_POD}" -n "${DEMO_NAMESPACE}" \
  --expected-digest "${BAD}" --reason "appendix: adversarial" 2>&1 | tail -3
BAD_RC="${PIPESTATUS[0]}"
set -e

if [ "${BAD_RC}" -ne 0 ]; then
  note_ok "approval with a wrong digest was refused (exit ${BAD_RC})"
else
  note_fail "approval succeeded with a wrong digest"
fi

STILL="$(demo_proposal_field .status.approvedCandidateDigest)"
if [ "${STILL}" = "${CURRENT_DIGEST}" ]; then
  note_ok "the stored approval was not replaced by the failed attempt"
else
  note_fail "the stored approval changed after a refused approval"
fi

# --- source lifecycle ------------------------------------------------------
# This is the section an SPO maintainer should read. The demo consumes their
# objects and must leave them exactly as it found them.
demo_stage "APPENDIX 3 — SPO's source objects were never adopted"

for src in "${DEMO_SOURCE_A}" "${DEMO_SOURCE_B}"; do
  state="$(kubectl get seccompprofile "${src}" -o jsonpath='{.spec.state}' 2>/dev/null || true)"
  owner="$(kubectl get seccompprofile "${src}" -o jsonpath='{.metadata.annotations.landlockgenprof\.io/managed-by}' 2>/dev/null || true)"
  rec="$(kubectl get seccompprofile "${src}" -o jsonpath='{.metadata.labels.spo\.x-k8s\.io/recording-id}' 2>/dev/null || true)"

  demo_panel "SPO SOURCE ${src}" \
    "spec.state                        ${state}" \
    "landlockgenprof managed-by        ${owner:-<absent, correct>}" \
    "spo recording-id                  ${rec}"

  if [ "${state}" = "Disabled" ]; then
    note_ok "${src} is still inert"
  else
    note_fail "${src} is ${state}"
  fi
  if [ -z "${owner}" ]; then
    note_ok "${src} was never marked as ours"
  else
    note_fail "${src} carries this project's ownership marker — it was adopted"
  fi
done

GOVERNED="$(kubectl get seccompprofile -o json \
  | jq -r '.items[] | select(.metadata.annotations["landlockgenprof.io/managed-by"]=="landlock-genprof") | .metadata.name' | head -1)"
demo_note "governed object this project does own: ${GOVERNED:-<none>}"

# --- inspection surfaces ---------------------------------------------------
demo_stage "APPENDIX 4 — inspection surfaces"

demo_note "policy: proposals visible in this namespace"
"${CLI_CMD[@]}" policy list -n "${DEMO_NAMESPACE}" 2>/dev/null || demo_note "(policy list unavailable)"

if [ -f "${DEMO_STATE}/candidate-b.json" ]; then
  demo_note "verify: check the recorded candidate against its own contract"
  set +e
  "${CLI_CMD[@]}" verify --candidate-file "${DEMO_STATE}/candidate-b.json" 2>&1 | tail -5
  demo_note "verify exit: ${PIPESTATUS[0]}"
  set -e
fi

# --- documented scope limits ----------------------------------------------
demo_stage "APPENDIX 5 — what v0.2 deliberately does not claim"

demo_panel "SCOPE" \
  "mergeStrategy      None only — SPO's merger drops container-id, and the" \
  "                   lineage contract requires it, so merged profiles are" \
  "                   refused rather than imported on partial lineage." \
  "syscall coverage   unknown — SPO v1.0.0 emits no coverage metadata, so" \
  "                   the value is recorded honestly instead of invented." \
  "seccomp diff       not implemented — diff compares Landlock candidates;" \
  "                   seccomp change is shown through provenance." \
  "behavioral denial  not proven — nothing here attempts a denied syscall." \
  "recorder topology  needs a real node; kind cannot resolve container pids."

if [ "${FAILURES}" -ne 0 ]; then
  demo_err "${FAILURES} appendix assertion(s) failed"
  exit 1
fi
demo_note "all appendix assertions passed"
