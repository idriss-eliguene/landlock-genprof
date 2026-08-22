#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/seccomp-differential-artifacts}"
NS="${NS:-podlock-seccomp-differential}"
IMAGE="landlock-genprof/fsprobe:governed-e2e"
mkdir -p "$ARTIFACTS_DIR"
fail() { echo "ERROR: $*" >&2; exit 1; }

cat > "$ARTIFACTS_DIR/landlockprofile.yaml" <<'YAML'
apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: differential-profile
  namespace: podlock-seccomp-differential
spec:
  profilesByContainer:
    probe:
      /probe/fsprobe:
        readExec: [/probe]
        readOnly: [/data]
YAML

cat > "$ARTIFACTS_DIR/seccompprofile.yaml" <<'YAML'
apiVersion: security-profiles-operator.x-k8s.io/v1
kind: SeccompProfile
metadata:
  name: differential-seccomp
  annotations:
    landlockgenprof.io/managed-by: landlock-genprof
    landlockgenprof.io/seccomp-origin: observed
    landlockgenprof.io/seccomp-source: internal
spec:
  defaultAction: SCMP_ACT_ERRNO
  architectures: [SCMP_ARCH_X86_64, SCMP_ARCH_X86, SCMP_ARCH_X32]
  syscalls:
  - action: SCMP_ACT_ALLOW
    names: [capget, capset, chdir, clock_nanosleep, futex]
YAML

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl apply -f "$ARTIFACTS_DIR/landlockprofile.yaml" >/dev/null
kubectl apply -f "$ARTIFACTS_DIR/seccompprofile.yaml" >/dev/null
kubectl wait --for=jsonpath='{.status.status}'=Installed seccompprofile/differential-seccomp --timeout=180s

for condition in a b c d; do
  pod="condition-$condition"
  mkdir -p "$ARTIFACTS_DIR/condition-$condition"
  security=""
  label=""
  case "$condition" in
    b) label='    podlock.kubewarden.io/profile: differential-profile' ;;
    c) security='      seccompProfile:\n        type: Localhost\n        localhostProfile: operator/differential-seccomp.json' ;;
    d) label='    podlock.kubewarden.io/profile: differential-profile'; security='      seccompProfile:\n        type: Localhost\n        localhostProfile: operator/differential-seccomp.json' ;;
  esac
  cat > "/tmp/$pod.yaml" <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: $pod
  namespace: $NS
  labels:
$label
spec:
  restartPolicy: Always
  containers:
  - name: probe
    image: $IMAGE
    imagePullPolicy: Never
    command: ["sh", "-c", "while true; do sleep 2; done"]
$(printf '%b\n' "$security")
YAML
  date -Ins > "$ARTIFACTS_DIR/condition-$condition/timestamps.txt"
  kubectl apply -f "/tmp/$pod.yaml" >/dev/null
  kubectl get pod "$pod" -n "$NS" -o json > "$ARTIFACTS_DIR/condition-$condition/pod.json" || true
  deadline=$((SECONDS + 90))
  started=false
  while [ "$SECONDS" -lt "$deadline" ]; do
    if json="$(kubectl get pod "$pod" -n "$NS" -o json 2>/dev/null)" && jq -e '[.status.containerStatuses[]? | select(.name == "probe" and .started == true and .state.running != null and .containerID != "")] | length == 1' <<<"$json" >/dev/null; then
      started=true
      break
    fi
    sleep 1
  done
  date -Ins >> "$ARTIFACTS_DIR/condition-$condition/timestamps.txt"
  kubectl get pod "$pod" -n "$NS" -o json > "$ARTIFACTS_DIR/condition-$condition/pod.json" || true
  kubectl get pod "$pod" -n "$NS" -o yaml > "$ARTIFACTS_DIR/condition-$condition/pod.yaml" || true
  kubectl describe pod "$pod" -n "$NS" > "$ARTIFACTS_DIR/condition-$condition/pod-describe.txt" 2>&1 || true
  kubectl get events -n "$NS" --field-selector involvedObject.name="$pod" --sort-by='.lastTimestamp' > "$ARTIFACTS_DIR/condition-$condition/events.txt" 2>&1 || true
  sudo k3s crictl ps -a 2>&1 | tee "$ARTIFACTS_DIR/condition-$condition/cri-containers.txt" >/dev/null || true
  sudo k3s crictl pods 2>&1 | tee "$ARTIFACTS_DIR/condition-$condition/cri-pods.txt" >/dev/null || true
  jq '{phase:.status.phase,containerStatuses:(.status.containerStatuses // []),initContainerStatuses:(.status.initContainerStatuses // [])}' "$ARTIFACTS_DIR/condition-$condition/pod.json" > "$ARTIFACTS_DIR/condition-$condition/security-context.txt" 2>/dev/null || true
  printf 'condition=%s\nstarted=%s\n' "$condition" "$started" > "$ARTIFACTS_DIR/condition-$condition/startup-result.txt"
  if [ "$condition" = b ] || [ "$condition" = d ]; then
    kubectl get landlockprofile differential-profile -n "$NS" -o yaml > "$ARTIFACTS_DIR/condition-$condition/podlock-profile.yaml" || true
    kubectl logs daemonset/podlock-nri-plugin -n podlock -c nri --tail=2000 > "$ARTIFACTS_DIR/condition-$condition/nri.log" 2>&1 || true
    sudo find /var/run/podlock -type f -name profile.json -exec cp {} "$ARTIFACTS_DIR/condition-$condition/profile.json" \; 2>/dev/null || true
  fi
  if [ "$condition" = c ] || [ "$condition" = d ]; then
    kubectl get seccompprofile differential-seccomp -o yaml > "$ARTIFACTS_DIR/condition-$condition/seccomp-profile.yaml" || true
    kubectl get seccompprofile differential-seccomp -o jsonpath='{.status}' > "$ARTIFACTS_DIR/condition-$condition/spo-status.txt" || true
    kubectl -n security-profiles-operator logs deployment/security-profiles-operator --all-containers --tail=1000 > "$ARTIFACTS_DIR/condition-$condition/spo-manager.log" 2>&1 || true
    kubectl -n security-profiles-operator logs daemonset/spod -c security-profiles-operator --tail=1000 > "$ARTIFACTS_DIR/condition-$condition/spod.log" 2>&1 || true
  fi
done

kubectl get pods -n "$NS" -o json > "$ARTIFACTS_DIR/all-pods.json" || true
for condition in a b c d; do
  jq 'del(.metadata.labels["podlock.kubewarden.io/profile"], .spec.containers[].securityContext.seccompProfile)' "$ARTIFACTS_DIR/condition-$condition/pod.json" > "$ARTIFACTS_DIR/condition-$condition/equivalence.json" 2>/dev/null || true
done
if cmp -s "$ARTIFACTS_DIR/condition-a/equivalence.json" "$ARTIFACTS_DIR/condition-b/equivalence.json" && cmp -s "$ARTIFACTS_DIR/condition-a/equivalence.json" "$ARTIFACTS_DIR/condition-c/equivalence.json" && cmp -s "$ARTIFACTS_DIR/condition-a/equivalence.json" "$ARTIFACTS_DIR/condition-d/equivalence.json"; then
  echo PASS > "$ARTIFACTS_DIR/equivalence-report.txt"
else
  echo FAIL > "$ARTIFACTS_DIR/equivalence-report.txt"
fi

results=()
for condition in a b c d; do results+=("$condition=$(awk -F= '/^started=/{print $2}' "$ARTIFACTS_DIR/condition-$condition/startup-result.txt")"); done
printf '%s\n' "${results[@]}" > "$ARTIFACTS_DIR/summary.txt"
if grep -q '^a=true$' "$ARTIFACTS_DIR/summary.txt" && grep -q '^b=true$' "$ARTIFACTS_DIR/summary.txt" && grep -q '^c=false$' "$ARTIFACTS_DIR/summary.txt" && grep -q '^d=false$' "$ARTIFACTS_DIR/summary.txt"; then
  echo SECCOMP_STARTUP_CAUSE_PROVEN >> "$ARTIFACTS_DIR/summary.txt"
else
  echo STARTUP_CAUSE_STILL_INCONCLUSIVE >> "$ARTIFACTS_DIR/summary.txt"
fi

kubectl delete namespace "$NS" --ignore-not-found >/dev/null || true
