#!/usr/bin/env bash
# preflight-spo-recorder.sh — prove the pid-namespace blocker is gone before
# spending a full D-MIN run on it.
#
# The D-MIN scenario previously failed on kind for one reason: SPO's eBPF
# recorder could not turn a BPF-reported pid into a container ID, because BPF
# reports pids in the host kernel's namespace and kind's node is a container
# with its own. Run 32255710044: 156 failed lookups, zero associations.
#
# That is an infrastructure property, so it is checked as infrastructure —
# separately from, and before, any product assertion. This script deliberately
# does NOT create a ProfileRecording or import anything; it only answers "can
# this cluster's recorder resolve a pid to a container at all". The product
# proof stays in spo-dmin.sh.
#
# Log text is the evidence here on purpose: the association step has no API
# surface, and this is preflight rather than the certification of a product
# claim.

set -euo pipefail
IFS=$'\n\t'

SPO_NAMESPACE="${SPO_NAMESPACE:-security-profiles-operator}"
PROBE_NS="${PROBE_NS:-spo-recorder-preflight}"
PROBE_POD="${PROBE_POD:-recorder-probe}"
PROBE_SECONDS="${PROBE_SECONDS:-45}"

fail() { echo "ERROR: $*" >&2; exit 1; }
stage() { printf '\n[preflight] %s\n' "$*"; }

cleanup() {
  kubectl delete namespace "${PROBE_NS}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT

# --- enable the recorder ---------------------------------------------------
stage "enabling SPO's eBPF recorder"

# spec.enricher.enableBpfRecorder, NOT spec.enableBpfRecorder — SPO v1.0.0
# groups the recorder and enricher toggles under `enricher`. verbosity 1 is
# required for this script to see anything: the association path logs at
# VerboseLevel while per-pid noise logs at Info.
kubectl -n "${SPO_NAMESPACE}" patch spod spod --type=merge \
  -p '{"spec":{"verbosity":1,"enricher":{"enableBpfRecorder":true}}}'

# Read back: a merge patch with a wrong path only WARNS and then reports
# "patched (no change)".
ENABLED="$(kubectl -n "${SPO_NAMESPACE}" get spod spod -o jsonpath='{.spec.enricher.enableBpfRecorder}')"
[ "${ENABLED}" = "true" ] \
  || fail "spec.enricher.enableBpfRecorder is '${ENABLED}' after patching; the field path does not match this SPO version"

# Wait for the operator to rewrite the DaemonSet before waiting on a rollout:
# the previous generation is already rolled out, so rollout status would
# return instantly and prove nothing.
has_bpf_container() {
  kubectl -n "${SPO_NAMESPACE}" get daemonset spod -o json 2>/dev/null \
    | jq -e '[.spec.template.spec.containers[].name] | index("bpf-recorder")' >/dev/null 2>&1
}
for _ in $(seq 1 40); do
  has_bpf_container && break
  sleep 5
done
has_bpf_container || fail "the operator never added a bpf-recorder container to spod"

kubectl -n "${SPO_NAMESPACE}" rollout status daemonset/spod --timeout=420s \
  || fail "spod did not roll out with the bpf recorder enabled"

READY="$(kubectl -n "${SPO_NAMESPACE}" get pods -o json \
  | jq '[.items[].status.containerStatuses[]? | select(.name=="bpf-recorder") | select(.ready)] | length')"
[ "${READY}" -gt 0 ] \
  || { kubectl -n "${SPO_NAMESPACE}" logs daemonset/spod -c bpf-recorder --tail=100 >&2 || true
       fail "no bpf-recorder container is ready"; }
echo "[ok] recorder is running (${READY} ready)"

# --- generate pid activity -------------------------------------------------
stage "generating container process activity"

kubectl create namespace "${PROBE_NS}" --dry-run=client -o yaml | kubectl apply -f -

# A short-lived exec loop produces a steady stream of new pids inside a
# container, which is exactly the event the recorder must resolve.
kubectl apply -f - <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: ${PROBE_POD}
  namespace: ${PROBE_NS}
spec:
  terminationGracePeriodSeconds: 1
  restartPolicy: Never
  containers:
    - name: probe
      image: busybox:1.36.1
      command: ["sh", "-c", "while true; do /bin/true; sleep 1; done"]
YAML

kubectl wait --for=condition=Ready "pod/${PROBE_POD}" -n "${PROBE_NS}" --timeout=180s \
  || { kubectl describe pod "${PROBE_POD}" -n "${PROBE_NS}" >&2; fail "the probe pod never became Ready"; }

echo "[info] letting the recorder observe for ${PROBE_SECONDS}s"
sleep "${PROBE_SECONDS}"

# --- verdict ---------------------------------------------------------------
stage "checking pid to container association"

LOG="$(kubectl -n "${SPO_NAMESPACE}" logs daemonset/spod -c bpf-recorder --tail=100000 2>/dev/null || true)"

RESOLVED="$(printf '%s\n' "${LOG}" | grep -c "Found container ID for PID" || true)"
UNRESOLVED="$(printf '%s\n' "${LOG}" | grep -c "No container ID found for PID" || true)"

echo "[info] resolved pid->container: ${RESOLVED}"
echo "[info] unresolved pid lookups:  ${UNRESOLVED}"

if [ "${RESOLVED}" -eq 0 ]; then
  echo "==== sample of unresolved lookups ====" >&2
  printf '%s\n' "${LOG}" | grep "No container ID found for PID" | tail -5 >&2 || true
  echo >&2
  echo "The recorder observed BPF events but resolved no pid to a container." >&2
  echo "This is the kind pid-namespace failure. The node must BE the Linux" >&2
  echo "host; a containerized node cannot satisfy SPO's recorder." >&2
  fail "PID NAMESPACE BLOCKER STILL PRESENT — this cluster cannot host D-MIN"
fi

echo "[ok] PID NAMESPACE OK — the recorder resolved ${RESOLVED} pid(s) to containers"
echo "[ok] this cluster can host the D-MIN recorder scenario"
