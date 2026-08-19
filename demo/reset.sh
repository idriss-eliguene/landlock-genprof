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

# Governed SeccompProfiles are CLUSTER-scoped, so deleting the namespace's
# objects does not touch them. A survivor from an earlier take would make
# the scenario's "nothing was applied" assertion pass for the wrong reason,
# so they are removed by ownership marker rather than by guessing names.
#
# The SPO SOURCE profiles are deliberately NOT deleted: they are expensive
# pre-baked recordings (~127 s each) and are not product state. Selecting on
# this project's own managed-by annotation is what keeps them safe.
demo_note "removing governed SeccompProfiles from previous takes (cluster-scoped)"
for prof in $(kubectl get seccompprofile -o json 2>/dev/null \
  | jq -r '.items[] | select(.metadata.annotations["landlockgenprof.io/managed-by"]=="landlock-genprof") | .metadata.name'); do
  demo_note "  deleting governed profile ${prof}"
  kubectl delete seccompprofile "${prof}" --ignore-not-found >/dev/null
done

if [ "$RECREATE_POD" -eq 1 ]; then
  demo_note "recreating the workload pod"
  kubectl delete pod "${DEMO_POD}" -n "${DEMO_NAMESPACE}" --ignore-not-found --wait=true
  kubectl apply -n "${DEMO_NAMESPACE}" -f "${DEMO_ROOT}/golden/workload.yaml"
  demo_wait_pod_ready 240s || exit 1
  demo_wait_tools_exec 120 || exit 1
else
  # Clear the drifted file so the next run's "new path" really is new.
  # Only meaningful when the pod survives the reset.
  demo_note "clearing the drift path inside the running workload"
  NAMESPACE="${DEMO_NAMESPACE}" POD="${DEMO_POD}" \
    ACTION_CONTAINER="${DEMO_CONTAINER}" CURL_BIN="${DEMO_BINARY}" \
    bash "${DEMO_ROOT}/drift-action.sh" clean
  demo_note "note: the path itself still exists in the container filesystem."
  demo_note "      Use --recreate-pod for a fully clean take."
fi

demo_note "removing local runtime state"
rm -rf "${DEMO_STATE}"

demo_stage "Reset complete"
demo_note "no proposal, no approval, no accumulated history in ${DEMO_NAMESPACE}"
demo_note "no governed SeccompProfile; SPO source recordings are preserved"
demo_note "Next: ./demo/scenario.sh"
