#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/governed-artifacts}"
NS="${NS:-podlock-governed}"
POD="${POD:-governed-probe}"
IMAGE="${IMAGE:-landlock-genprof/fsprobe:governed-e2e}"
BINARY="/probe/fsprobe"
mkdir -p "$ARTIFACTS_DIR"
fail() { echo "ERROR: $*" >&2; exit 1; }

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

cat > /tmp/control.yaml <<EOF
apiVersion: v1
kind: Pod
metadata: {name: control-probe, namespace: $NS}
spec:
  restartPolicy: Always
  containers:
  - name: probe
    image: $IMAGE
    command: ["sh", "-c", "while true; do sleep 2; done"]
    imagePullPolicy: Never
EOF
kubectl apply -f /tmp/control.yaml >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/control-probe -n "$NS" --timeout=180s
kubectl exec -n "$NS" control-probe -c probe -- "$BINARY" /data/allowed.txt | tee "$ARTIFACTS_DIR/control-allowed.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/control-allowed.txt" || fail "control allowed read failed"

kubectl exec -n "$NS" control-probe -c probe -- "$BINARY" /data/denied.txt | tee "$ARTIFACTS_DIR/control-denied.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/control-denied.txt" || fail "control denied-candidate read failed"

kubectl get pod control-probe -n "$NS" -o yaml > "$ARTIFACTS_DIR/control-pod.yaml"
sed "s/control-probe/$POD/g; s#/data/allowed.txt#/data/allowed.txt#g" /tmp/control.yaml > /tmp/governed.yaml
kubectl delete pod control-probe -n "$NS" --ignore-not-found >/dev/null
kubectl apply -f /tmp/governed.yaml >/dev/null
kubectl wait \
  --for=condition=Ready \
  pod/"$POD" \
  -n "$NS" \
  --timeout=180s
command -v kubectl-landlock_genprof >/dev/null || fail "kubectl-landlock_genprof plugin is not on PATH"
kubectl landlock-genprof --help >/dev/null || fail "kubectl cannot discover landlock-genprof plugin"
CLI=(kubectl landlock-genprof)
"${CLI[@]}" trace --pod "$POD" -n "$NS" --container probe --binary "$BINARY" --duration 30s \
  --events-out "$ARTIFACTS_DIR/events.json" --out "$ARTIFACTS_DIR/profile.yaml" \
  > "$ARTIFACTS_DIR/trace.txt" 2>&1 &
TRACE_PID=$!
sleep 5
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt > "$ARTIFACTS_DIR/trace-read.txt"
wait "$TRACE_PID"

kubectl get securityprofileproposal "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/proposal-before.yaml"
"${CLI[@]}" review "$POD" -n "$NS" | tee "$ARTIFACTS_DIR/review.txt"
DIGEST="$(awk '/^Candidate digest: /{print $3; exit}' "$ARTIFACTS_DIR/review.txt")"
[ -n "$DIGEST" ] || fail "review produced no CandidateDigest"
"${CLI[@]}" approve "$POD" -n "$NS" --expected-digest "$DIGEST" --reason governed-podlock | tee "$ARTIFACTS_DIR/approve.txt"
APPROVED="$(kubectl get securityprofileproposal "$POD" -n "$NS" -o jsonpath='{.status.approvedCandidateDigest}')"
[ "$APPROVED" = "$DIGEST" ] || fail "approved digest mismatch"
kubectl get securityprofileproposal "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/proposal-approved.yaml"

kubectl get pod "$POD" -n "$NS" -o json > "$ARTIFACTS_DIR/pre-delete-pod.json"
date -Ins > "$ARTIFACTS_DIR/apply-start.txt"
kubectl get events -n "$NS" \
  --field-selector involvedObject.kind=Pod,involvedObject.name="$POD" \
  --sort-by='.lastTimestamp' > "$ARTIFACTS_DIR/events-before-delete.txt" 2>&1 || true

observe_deletion() {
  local out="$ARTIFACTS_DIR/deletion-observer.txt"
  while :; do
    local now json
    now="$(date -Ins)"
    if json="$(kubectl get pod "$POD" -n "$NS" -o json 2>&1)"; then
      jq -c --arg now "$now" '{time:$now,result:"present",uid:.metadata.uid,resourceVersion:.metadata.resourceVersion,deletionTimestamp:(.metadata.deletionTimestamp // null),deletionGracePeriodSeconds:(.metadata.deletionGracePeriodSeconds // null),phase:(.status.phase // null),finalizers:(.metadata.finalizers // []),reason:(.status.reason // null),message:(.status.message // null),containerStatuses:(.status.containerStatuses // []),initContainerStatuses:(.status.initContainerStatuses // [])}' <<<"$json" >> "$out"
    else
      jq -cn --arg now "$now" --arg error "$json" '{time:$now,result:"get-error",error:$error}' >> "$out"
      if grep -q 'NotFound\|not found' <<<"$json"; then
        jq -cn --arg now "$now" '{time:$now,result:"not-found"}' >> "$out"
      fi
    fi
    sleep 1
  done
}
observe_deletion &
OBSERVER_PID=$!
set +e
"${CLI[@]}" apply-proposal "$POD" -n "$NS" --yes --restart > "$ARTIFACTS_DIR/apply.txt" 2>&1
APPLY_STATUS=$?
set -e
date -Ins > "$ARTIFACTS_DIR/apply-end.txt"
sleep 5
kill "$OBSERVER_PID" 2>/dev/null || true
wait "$OBSERVER_PID" 2>/dev/null || true
kubectl get events -n "$NS" \
  --field-selector involvedObject.kind=Pod,involvedObject.name="$POD" \
  --sort-by='.lastTimestamp' > "$ARTIFACTS_DIR/events-after-delete.txt" 2>&1 || true
cat "$ARTIFACTS_DIR/apply.txt"
if [ "$APPLY_STATUS" -ne 0 ]; then
  fail "apply-proposal failed (exit $APPLY_STATUS)"
fi

# Capture replacement state before the first exec. Phase=Running does not
# prove that the target container has a runtime ID or running state.
kubectl get pod "$POD" -n "$NS" -o json > "$ARTIFACTS_DIR/replacement-pod.json"
ORIGINAL_UID="$(jq -r '.metadata.uid // ""' "$ARTIFACTS_DIR/pre-delete-pod.json")"
REPLACEMENT_UID="$(jq -r '.metadata.uid // ""' "$ARTIFACTS_DIR/replacement-pod.json")"
{
  printf 'original_uid=%s\nreplacement_uid=%s\nuid_changed=%s\n' \
    "$ORIGINAL_UID" "$REPLACEMENT_UID" "$([ "$ORIGINAL_UID" != "$REPLACEMENT_UID" ] && echo true || echo false)"
  jq '{metadata:{uid:.metadata.uid,resourceVersion:.metadata.resourceVersion,creationTimestamp:.metadata.creationTimestamp,deletionTimestamp:(.metadata.deletionTimestamp // null),labels:.metadata.labels,annotations:.metadata.annotations},spec:{nodeName:.spec.nodeName,restartPolicy:.spec.restartPolicy,containers:.spec.containers,initContainers:(.spec.initContainers // [])},status:{phase:.status.phase,conditions:(.status.conditions // []),containerStatuses:(.status.containerStatuses // []),initContainerStatuses:(.status.initContainerStatuses // [])}}' "$ARTIFACTS_DIR/replacement-pod.json"
} > "$ARTIFACTS_DIR/replacement-pod-state.txt"

kubectl describe pod "$POD" -n "$NS" > "$ARTIFACTS_DIR/replacement-pod-describe.txt" 2>&1 || true
kubectl get events -n "$NS" --field-selector involvedObject.name="$POD" --sort-by='.lastTimestamp' > "$ARTIFACTS_DIR/replacement-pod-events.txt" 2>&1 || true
kubectl get seccompprofile -o yaml > "$ARTIFACTS_DIR/replacement-seccomp-profiles.yaml" 2>&1 || true
kubectl -n security-profiles-operator logs daemonset/spod -c security-profiles-operator --tail=1000 > "$ARTIFACTS_DIR/replacement-spod.log" 2>&1 || true
kubectl -n security-profiles-operator logs deployment/security-profiles-operator --all-containers --tail=1000 > "$ARTIFACTS_DIR/replacement-spo-manager.log" 2>&1 || true
kubectl logs daemonset/podlock-nri-plugin -n podlock -c nri --tail=2000 > "$ARTIFACTS_DIR/replacement-nri.log" 2>&1 || true
sudo k3s crictl ps -a 2>&1 | tee "$ARTIFACTS_DIR/replacement-cri-containers.txt" >/dev/null || true
sudo k3s crictl pods 2>&1 | tee "$ARTIFACTS_DIR/replacement-cri-pods.txt" >/dev/null || true

observe_replacement_container() {
  local out="$ARTIFACTS_DIR/replacement-readiness-timeline.txt"
  local deadline=$((SECONDS + 15))
  while [ "$SECONDS" -lt "$deadline" ]; do
    local now json statuses
    now="$(date -Ins)"
    if json="$(kubectl get pod "$POD" -n "$NS" -o json 2>&1)"; then
      statuses="$(jq -c '[.status.containerStatuses[]? | select(.name == "probe") | {name,ready,started,restartCount,containerID,image,imageID,state,lastState}]' <<<"$json")"
      jq -cn --arg time "$now" --arg uid "$(jq -r '.metadata.uid // ""' <<<"$json")" --arg phase "$(jq -r '.status.phase // ""' <<<"$json")" --argjson statuses "$statuses" '{time:$time,uid:$uid,phase:$phase,probe_status_present:($statuses|length>0),probe_ready:([$statuses[]? | select(.ready == true)]|length>0),probe_started:([$statuses[]? | select(.started == true)]|length>0),probe_containerID:([$statuses[]?.containerID // empty]|first // ""),probe_state:([$statuses[]?.state // {}]|first)}' >> "$out"
      if jq -e '[.status.containerStatuses[]? | select(.name == "probe" and .state.running != null and .containerID != "")] | length == 1' <<<"$json" >/dev/null; then
        break
      fi
    else
      jq -cn --arg time "$now" --arg error "$json" '{time:$time,result:"get-error",error:$error}' >> "$out"
    fi
    sleep 1
  done
}
observe_replacement_container &
READINESS_OBSERVER_PID=$!

kubectl get landlockprofile "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/live-profile.yaml"
kubectl get pod "$POD" -n "$NS" -o yaml > "$ARTIFACTS_DIR/governed-pod.yaml"
grep -F "podlock.kubewarden.io/profile: $POD" "$ARTIFACTS_DIR/governed-pod.yaml" || fail "binding label missing"
kubectl wait --for=jsonpath='{.status.phase}'=Running pod/"$POD" -n "$NS" --timeout=240s

kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/allowed.txt | tee "$ARTIFACTS_DIR/governed-allowed.txt"
grep -F 'result=success errno=0' "$ARTIFACTS_DIR/governed-allowed.txt" || fail "governed allowed read failed"
kubectl exec -n "$NS" "$POD" -c probe -- "$BINARY" /data/denied.txt | tee "$ARTIFACTS_DIR/governed-denied.txt" || true
grep -F 'result=failure errno=13' "$ARTIFACTS_DIR/governed-denied.txt" || fail "governed denial was not EACCES"
wait "$READINESS_OBSERVER_PID" 2>/dev/null || true

kubectl get pods -n "$NS" -o yaml > "$ARTIFACTS_DIR/pods.yaml"
kubectl logs daemonset/podlock-nri-plugin -n podlock -c nri > "$ARTIFACTS_DIR/nri.log" 2>&1 || true
sudo find /var/run/podlock -type f -name profile.json -print -exec cp {} "$ARTIFACTS_DIR/profile.json" \; || true
grep -R "landlock profile applied" "$ARTIFACTS_DIR/nri.log" || fail "seal activation evidence missing"
