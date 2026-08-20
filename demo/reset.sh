#!/usr/bin/env bash
# reset.sh — return the demo to a known initial state.
#
# Deletes only demo-owned product state so the scenario can be re-run from
# the top: the SecurityProfileProposal (and with it the approval status),
# the TrainingHistory that carries cross-run confidence, the NetworkPolicy
# the governed apply created, and the local runtime files.
#
# It deliberately does NOT delete the cluster, the Gadget, the CRDs, the
# echo services, or anything outside ${DEMO_NAMESPACE}. Safe to run
# repeatedly, and safe to run when nothing exists yet.
#
#   ./demo/reset.sh                # reset product state, keep the pod
#   ./demo/reset.sh --recreate-pod # also recreate the workload pod

set -euo pipefail

# shellcheck source=demo/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

RECREATE_POD=0
if [ "${1:-}" = "--recreate-pod" ]; then
  RECREATE_POD=1
fi

demo_stage "Demo reset"

demo_require_cmd kubectl jq || exit 1
demo_check_context || exit 1

# --ignore-not-found so a reset on a clean cluster is a no-op, not an error.
demo_note "removing SecurityProfileProposal (this also removes its approval status)"
kubectl delete securityprofileproposal "${DEMO_POD}" \
  -n "${DEMO_NAMESPACE}" --ignore-not-found

demo_note "removing TrainingHistory (cross-run evidence)"
# The history object name is derived by the product from container/binary,
# so delete by label-free bulk within the demo namespace rather than
# guessing the computed name.
kubectl delete traininghistory --all -n "${DEMO_NAMESPACE}" --ignore-not-found

demo_note "removing the NetworkPolicy a previous governed apply created"
kubectl delete networkpolicy "${DEMO_POD}" \
  -n "${DEMO_NAMESPACE}" --ignore-not-found

# Governed SeccompProfiles are CLUSTER-scoped, so deleting namespace objects
# does not remove them. SPO may keep a governed profile alive while a workload
# still references its localhostProfile. Remove the consumer BEFORE deleting
# governed authority.
#
# SPO SOURCE profiles are deliberately preserved: they are learned evidence,
# not governed product state.
demo_note "discovering governed SeccompProfiles from previous takes"

mapfile -t governed_profiles < <(
  kubectl get seccompprofile -o json 2>/dev/null \
    | jq -r '.items[]
      | select(.metadata.annotations["landlockgenprof.io/managed-by"]=="landlock-genprof")
      | .metadata.name'
)

need_recreate="$RECREATE_POD"

if [ "${#governed_profiles[@]}" -gt 0 ]; then
  need_recreate=1

  demo_note "governed authority exists; removing the workload consumer first"

  kubectl delete pod "${DEMO_POD}" \
    -n "${DEMO_NAMESPACE}" \
    --ignore-not-found \
    --wait=true \
    --timeout=60s

  for prof in "${governed_profiles[@]}"; do
    demo_note "  deleting governed profile ${prof}"

    if ! kubectl delete seccompprofile "${prof}" \
      --ignore-not-found \
      --wait=true \
      --timeout=60s >/dev/null; then

      demo_err "timed out deleting governed SeccompProfile ${prof}"
      kubectl get seccompprofile "${prof}" \
        -o custom-columns='NAME:.metadata.name,STATE:.spec.state,DELETING:.metadata.deletionTimestamp,FINALIZERS:.metadata.finalizers' \
        2>/dev/null || true
      exit 1
    fi
  done
else
  demo_note "no governed SeccompProfile found"
fi

if [ "$need_recreate" -eq 1 ]; then
  demo_note "recreating the clean workload pod"

  # Idempotent when no pod exists.
  kubectl delete pod "${DEMO_POD}" \
    -n "${DEMO_NAMESPACE}" \
    --ignore-not-found \
    --wait=true \
    --timeout=60s

  kubectl apply \
    -n "${DEMO_NAMESPACE}" \
    -f "${DEMO_ROOT}/golden/workload.yaml"

  demo_wait_pod_ready 240s || exit 1
  demo_wait_tools_exec 120 || exit 1
else
  # No governed authority existed, so keeping the current pod is safe.
  # Clear only the deliberate drift mutation.
  demo_note "clearing the drift path inside the running workload"

  NAMESPACE="${DEMO_NAMESPACE}" POD="${DEMO_POD}" \
    ACTION_CONTAINER="${DEMO_CONTAINER}" CURL_BIN="${DEMO_BINARY}" \
    bash "${DEMO_ROOT}/drift-action.sh" clean
fi

demo_note "removing local runtime state"
rm -rf "${DEMO_STATE}"

demo_stage "Reset complete"
demo_note "no proposal, no approval, no accumulated history in ${DEMO_NAMESPACE}"
demo_note "no governed SeccompProfile; SPO source recordings are preserved"
demo_note "Next: ./demo/scenario.sh"
