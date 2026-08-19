#!/usr/bin/env bash
# spo-sources.sh — pre-bake the two REAL security-profiles-operator
# recordings the hero demo consumes.
#
# Why this is setup and not part of the scenario: SPO's record-and-generate
# cycle measured 127 s per recording in CI. Two of them is over four minutes
# of an audience watching nothing happen, for a step that demonstrates SPO
# rather than landlock-genprof. The recordings are therefore produced before
# the camera rolls; the scenario consumes the resulting real cluster state.
#
# What this deliberately does NOT do: write a SeccompProfile by hand. Every
# byte of enforcement content the demo later imports was produced by SPO's
# own eBPF recorder observing a real container. A fabricated source would
# make the entire demo a lie, because the one thing it claims is that a
# legitimate upstream learner produced valid policy.
#
# Two recordings, one workload, two behaviors:
#
#   A — the workload reads a local file.
#   B — the workload also calls a service over the network.
#
# B is a superset: same container, same image, one added behavior. That is
# what makes the syscall delta real and explainable in one sentence rather
# than an artifact of two unrelated workloads.
#
# Requires: a real-node cluster with SPO installed and its eBPF recorder
# enabled (demo/setup.sh --with-cluster does this).

set -euo pipefail

# shellcheck source=demo/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

RECORD_SECONDS="${RECORD_SECONDS:-45}"
IMAGE="${DEMO_IMAGE:-curlimages/curl:8.3.0}"
ECHO_URL="http://echo-8080-svc.${DEMO_NAMESPACE}.svc.cluster.local:8080"

# Behavior A: read a local file, repeatedly. Deterministic, no network.
BEHAVIOR_A="while true; do curl -sS file:///etc/hosts -o /dev/null 2>/dev/null || true; sleep 2; done"
# Behavior B: the same, plus a real network call. The added syscalls
# (socket/connect and friends) are a consequence of the workload doing more,
# not of the recorder being configured differently.
BEHAVIOR_B="while true; do curl -sS file:///etc/hosts -o /dev/null 2>/dev/null || true; curl -sS --max-time 2 -o /dev/null ${ECHO_URL} 2>/dev/null || true; sleep 2; done"

# record_one <recording-name> <recorder-pod> <behavior>
#
# Creates a real ProfileRecording, runs a pod that matches its selector,
# lets SPO observe, then deletes the pod — which is what makes SPO write the
# profile. Waits for the generated object to exist.
record_one() {
  local recording="$1" pod="$2" behavior="$3"
  local profile="${recording}-${DEMO_CONTAINER}"

  if kubectl get seccompprofile "${profile}" >/dev/null 2>&1; then
    demo_note "source profile ${profile} already exists — skipping (idempotent)"
    return 0
  fi

  demo_stage "Recording ${recording} (real SPO, ~${RECORD_SECONDS}s + generation)"

  kubectl apply -f - >/dev/null <<YAML
apiVersion: security-profiles-operator.x-k8s.io/v1
kind: ProfileRecording
metadata:
  name: ${recording}
  namespace: ${DEMO_NAMESPACE}
spec:
  kind: SeccompProfile
  recorder: Bpf
  # v0.2 supports None only: SPO's merger drops the container-id label that
  # the import's lineage contract requires (docs/adr/0008).
  mergeStrategy: None
  # Makes SPO leave the generated profile in spec.state: Disabled. The demo's
  # entire cold open depends on this being real: the source is inert because
  # SPO left it inert, not because we disabled it.
  disableProfileAfterRecording: true
  podSelector:
    matchLabels:
      app: ${recording}
YAML

  # The recording must exist before the pod: SPO's webhook annotates pods at
  # admission, so a pod created first is simply never recorded.
  kubectl apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: ${pod}
  namespace: ${DEMO_NAMESPACE}
  labels:
    app: ${recording}
spec:
  terminationGracePeriodSeconds: 5
  restartPolicy: Never
  containers:
    - name: ${DEMO_CONTAINER}
      image: ${IMAGE}
      command: ["sh", "-c", "${behavior}"]
      securityContext:
        runAsUser: 0
YAML

  kubectl wait --for=condition=Ready "pod/${pod}" -n "${DEMO_NAMESPACE}" --timeout=240s \
    || { kubectl describe pod "${pod}" -n "${DEMO_NAMESPACE}" >&2; demo_err "recorder pod ${pod} never became Ready"; return 1; }

  # Proof the webhook engaged. Without it the recording silently observes
  # nothing and the failure only surfaces minutes later as a missing profile.
  local ann
  ann="$(kubectl get pod "${pod}" -n "${DEMO_NAMESPACE}" -o json \
    | jq -r '.metadata.annotations | to_entries[] | select(.key|startswith("io.containers.trace-")) | .key' | head -1)"
  [ -n "${ann}" ] || { demo_err "SPO did not annotate ${pod} for recording"; return 1; }
  demo_note "recording annotation: ${ann}"

  # The recorder maps containers to recordings via pod status; wait for the
  # container ID to be visible rather than racing that lookup.
  local cid=""
  local i
  for i in $(seq 1 30); do
    cid="$(kubectl get pod "${pod}" -n "${DEMO_NAMESPACE}" -o jsonpath='{.status.containerStatuses[0].containerID}' 2>/dev/null || true)"
    [ -n "${cid}" ] && break
    sleep 2
  done
  [ -n "${cid}" ] || { demo_err "${pod} never reported a containerID"; return 1; }

  demo_note "observing for ${RECORD_SECONDS}s"
  sleep "${RECORD_SECONDS}"

  # SPO writes the profile when the recorded container stops.
  kubectl delete pod "${pod}" -n "${DEMO_NAMESPACE}" --wait=true --timeout=120s >/dev/null

  demo_note "waiting for SPO to generate ${profile}"
  for i in $(seq 1 60); do
    kubectl get seccompprofile "${profile}" >/dev/null 2>&1 && break
    sleep 5
  done
  kubectl get seccompprofile "${profile}" >/dev/null 2>&1 || {
    kubectl -n "${DEMO_SPO_NAMESPACE}" logs daemonset/spod -c bpf-recorder --tail=100000 2>/dev/null \
      | grep -iE "container ID|profile|Unable to" | grep -viE "Received new pid|record pid exit" | tail -30 >&2 || true
    demo_err "SPO never generated ${profile}"
    return 1
  }

  local count state
  count="$(kubectl get seccompprofile "${profile}" -o json | jq '[.spec.syscalls[]?.names[]?] | length')"
  state="$(kubectl get seccompprofile "${profile}" -o jsonpath='{.spec.state}')"
  demo_note "generated ${profile}: ${count} syscalls, state=${state}"

  # Fail here rather than in the scenario: an enforcing source would break
  # the demo's central claim that learned policy starts inert.
  [ "${state}" = "Disabled" ] \
    || { demo_err "${profile} is ${state}, expected Disabled"; return 1; }
  return 0
}

demo_require_cmd kubectl jq || exit 1
demo_check_context || exit 1

kubectl get crd profilerecordings.security-profiles-operator.x-k8s.io >/dev/null 2>&1 \
  || { demo_err "SPO is not installed — run ./demo/setup.sh --with-cluster"; exit 1; }

record_one "${DEMO_RECORDING_A}" "rec-a" "${BEHAVIOR_A}" || exit 1
record_one "${DEMO_RECORDING_B}" "rec-b" "${BEHAVIOR_B}" || exit 1

# The delta is the demo's premise, so it is verified here rather than
# assumed on stage. If SPO ever produces identical profiles for these two
# behaviors, the drift scene has nothing to show and the demo should fail
# during setup, not in front of an audience.
A_COUNT="$(kubectl get seccompprofile "${DEMO_SOURCE_A}" -o json | jq '[.spec.syscalls[]?.names[]?] | length')"
B_COUNT="$(kubectl get seccompprofile "${DEMO_SOURCE_B}" -o json | jq '[.spec.syscalls[]?.names[]?] | length')"

demo_stage "SPO source material ready"
demo_panel "REAL SPO RECORDINGS" \
  "${DEMO_SOURCE_A}   ${A_COUNT} syscalls   state=Disabled" \
  "${DEMO_SOURCE_B}   ${B_COUNT} syscalls   state=Disabled"

if [ "${A_COUNT}" = "${B_COUNT}" ]; then
  demo_err "recordings A and B captured the same number of syscalls (${A_COUNT})."
  demo_err "Behavior B must observably differ, or the drift scene proves nothing."
  exit 1
fi
demo_note "syscall authority differs between A and B — the drift is real"
