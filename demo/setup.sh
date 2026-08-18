#!/usr/bin/env bash
# setup.sh — bring the demo environment to a recordable state.
#
# This composes the cluster/dependency targets the E2E already owns rather
# than reimplementing them, deploys the existing Golden fixture, and waits
# until everything the scenario needs is actually ready. It creates no
# product state: no trace, no proposal, no approval. scenario.sh does that.
#
# Idempotent: safe to re-run against an already-provisioned cluster.
#
#   ./demo/setup.sh              # assume a cluster exists, provision into it
#   ./demo/setup.sh --with-cluster   # also create the kind cluster + deps

set -euo pipefail

# shellcheck source=demo/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

WITH_CLUSTER=0
if [ "${1:-}" = "--with-cluster" ]; then
  WITH_CLUSTER=1
fi

demo_stage "Demo setup"

demo_require_cmd kubectl || exit 1

if [ "$WITH_CLUSTER" -eq 1 ]; then
  demo_require_cmd kind || exit 1
  demo_note "creating cluster and installing dependencies (existing Makefile targets)"
  # These are the same targets the authoritative Core E2E runs; the demo
  # deliberately does not own cluster lifecycle.
  make -C "${REPO_ROOT}" e2e-cluster-create
  make -C "${REPO_ROOT}" e2e-install
fi

demo_check_context || exit 1

if ! demo_resolve_cli; then
  exit 1
fi
demo_note "CLI mode: ${CLI_MODE} (${CLI_CMD[*]})"
"${CLI_CMD[@]}" version || true

demo_stage "Prerequisites"

# Inspektor Gadget supplies the observation path; without a Ready DaemonSet
# a trace produces nothing and the demo silently becomes meaningless.
if ! kubectl -n gadget get daemonset gadget >/dev/null 2>&1; then
  demo_err "Inspektor Gadget DaemonSet not found in namespace 'gadget'."
  demo_err "Run './demo/setup.sh --with-cluster', or 'make e2e-install' against this cluster."
  exit 1
fi
# shellcheck disable=SC2016  # deliberately deferred: re-evaluated on each poll
demo_wait_for "gadget DaemonSet" 300 \
  bash -c 'test "$(kubectl -n gadget get daemonset gadget -o jsonpath="{.status.numberReady}")" = "$(kubectl -n gadget get daemonset gadget -o jsonpath="{.status.desiredNumberScheduled}")"' \
  || exit 1

for crd in securityprofileproposals.landlockgenprof.io traininghistories.landlockgenprof.io; do
  if ! kubectl get crd "$crd" >/dev/null 2>&1; then
    demo_err "CRD missing: $crd — run 'make e2e-install' against this cluster."
    exit 1
  fi
  demo_note "CRD present: $crd"
done

demo_stage "Golden workload"

kubectl create namespace "${DEMO_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Reuse the E2E-owned fixtures verbatim. The demo does not keep its own
# copy of the workload: a second copy would drift from the one CI proves.
kubectl apply -n "${DEMO_NAMESPACE}" -f "${DEMO_ROOT}/golden/echo-service.yaml"

# The workload is a bare Pod, whose spec is immutable: if one is already
# running from an older revision of the fixture, `apply` fails rather than
# updating it. Recreate in that case so the demo always runs against the
# fixture in this checkout, not whatever a previous session left behind.
if ! kubectl apply -n "${DEMO_NAMESPACE}" -f "${DEMO_ROOT}/golden/workload.yaml" 2>/dev/null; then
  demo_note "existing pod differs from the current fixture — recreating it"
  kubectl delete pod "${DEMO_POD}" -n "${DEMO_NAMESPACE}" --ignore-not-found --wait=true
  kubectl apply -n "${DEMO_NAMESPACE}" -f "${DEMO_ROOT}/golden/workload.yaml"
fi

demo_note "waiting for the workload to become Ready"
demo_wait_pod_ready 240s || exit 1
demo_wait_tools_exec 120 || exit 1

demo_state_dir

demo_stage "Demo environment ready"
demo_note "namespace : ${DEMO_NAMESPACE}"
demo_note "pod       : ${DEMO_POD} (container: ${DEMO_CONTAINER}, binary: ${DEMO_BINARY})"
demo_note "duration  : ${DEMO_DURATION} per training run"
demo_note "state dir : ${DEMO_STATE}"
printf '\n'
demo_note "Next: ./demo/reset.sh   (clear any product state from a previous take)"
demo_note "Then: ./demo/scenario.sh"
