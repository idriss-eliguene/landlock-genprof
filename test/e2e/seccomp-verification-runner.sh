#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KIND_NAME="landlock-genprof-e2e"
PROFILE_NAME="seccomp-verification-e2e"
PROFILE_PATH="operator/${PROFILE_NAME}.json"
VERIFY_NS="landlock-genprof-seccomp-verifier"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-${ROOT_DIR}/seccomp-verification-artifacts}"
TMP_DIR="$(mktemp -d)"
ADMIN_KUBECONFIG="${TMP_DIR}/kind-admin.kubeconfig"
VERIFIER_KUBECONFIG="${TMP_DIR}/verifier.kubeconfig"
mkdir -p "${ARTIFACTS_DIR}"

collect_diagnostics() {
  set +e
  kubectl --kubeconfig "${ADMIN_KUBECONFIG}" -n "${VERIFY_NS}" get pods -o yaml >"${ARTIFACTS_DIR}/verifier-pods.yaml" 2>&1
  kubectl --kubeconfig "${ADMIN_KUBECONFIG}" -n "${VERIFY_NS}" describe pods >"${ARTIFACTS_DIR}/verifier-pods.txt" 2>&1
  kubectl --kubeconfig "${ADMIN_KUBECONFIG}" -n "${VERIFY_NS}" logs -l landlockgenprof.io/seccomp-verifier --all-containers --tail=100 >"${ARTIFACTS_DIR}/verifier-logs.txt" 2>&1
  kubectl --kubeconfig "${ADMIN_KUBECONFIG}" get seccompprofile "${PROFILE_NAME}" -o yaml >"${ARTIFACTS_DIR}/profile.yaml" 2>&1
  kubectl --kubeconfig "${ADMIN_KUBECONFIG}" -n "${VERIFY_NS}" get events --sort-by=.lastTimestamp >"${ARTIFACTS_DIR}/events.txt" 2>&1
  kind export logs --name "${KIND_NAME}" "${ARTIFACTS_DIR}/kind-logs" >/dev/null 2>&1
  set -e
}
trap 'status=$?; if [ "$status" -ne 0 ]; then collect_diagnostics; fi; rm -rf "${TMP_DIR}"; exit "$status"' EXIT

kubectl config view --raw --minify >"${ADMIN_KUBECONFIG}"
test "$(kind get clusters | grep -cx "${KIND_NAME}")" -eq 1
if kubectl --kubeconfig "${ADMIN_KUBECONFIG}" get seccompprofile "${PROFILE_NAME}" >/dev/null 2>&1; then
  echo "refusing to replace existing test profile ${PROFILE_NAME}" >&2
  exit 2
fi

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "${TMP_DIR}/seccomp-verifier-probe" "${ROOT_DIR}/cmd/seccomp-verifier-probe"
docker buildx build --platform linux/amd64 --load \
  --tag localhost/landlock-genprof/seccomp-verifier:ci \
  --metadata-file "${TMP_DIR}/image-metadata.json" \
  -f "${ROOT_DIR}/Dockerfile.seccomp-verifier" "${TMP_DIR}"
IMAGE_DIGEST="$(jq -r '.["containerimage.digest"] // empty' "${TMP_DIR}/image-metadata.json")"
[[ "${IMAGE_DIGEST}" =~ ^sha256:[a-f0-9]{64}$ ]]
IMAGE_REF="localhost/landlock-genprof/seccomp-verifier@${IMAGE_DIGEST}"
printf 'probe_image=%s\n' "${IMAGE_REF}" >"${ARTIFACTS_DIR}/image.txt"
kind load docker-image localhost/landlock-genprof/seccomp-verifier:ci --name "${KIND_NAME}"
NODE="$(kubectl --kubeconfig "${ADMIN_KUBECONFIG}" get nodes -o jsonpath='{.items[0].metadata.name}')"
NODE_CONTAINER="$(kind get nodes --name "${KIND_NAME}" | head -n 1)"
docker exec "${NODE_CONTAINER}" ctr -n k8s.io images tag \
  localhost/landlock-genprof/seccomp-verifier:ci "${IMAGE_REF}"

cat >"${TMP_DIR}/profile.yaml" <<EOF
apiVersion: security-profiles-operator.x-k8s.io/v1
kind: SeccompProfile
metadata:
  name: ${PROFILE_NAME}
spec:
  defaultAction: SCMP_ACT_ALLOW
  architectures: [SCMP_ARCH_X86_64]
  syscalls:
    - names: [getpriority]
      action: SCMP_ACT_ERRNO
      errnoRet: 1
EOF
kubectl --kubeconfig "${ADMIN_KUBECONFIG}" apply -f "${TMP_DIR}/profile.yaml"
installed=""
for _ in $(seq 1 90); do
  installed="$(kubectl --kubeconfig "${ADMIN_KUBECONFIG}" get seccompprofile "${PROFILE_NAME}" -o jsonpath='{.status.localhostProfile}' 2>/dev/null || true)"
  [ "${installed}" = "${PROFILE_PATH}" ] && break
  sleep 2
done
[ "${installed}" = "${PROFILE_PATH}" ] || { echo "SPO did not materialize ${PROFILE_PATH}" >&2; exit 1; }
docker exec "${NODE_CONTAINER}" test -r "/var/lib/kubelet/seccomp/${PROFILE_PATH}"

kubectl --kubeconfig "${ADMIN_KUBECONFIG}" apply -f "${ROOT_DIR}/deploy/rbac-seccomp-verifier.yaml"
TOKEN="$(kubectl --kubeconfig "${ADMIN_KUBECONFIG}" -n "${VERIFY_NS}" create token landlock-genprof-seccomp-verifier --duration=10m)"
CLUSTER_NAME="$(kubectl --kubeconfig "${ADMIN_KUBECONFIG}" config view --minify -o jsonpath='{.clusters[0].name}')"
CONTEXT_NAME="seccomp-verifier-${KIND_NAME}"
CA_DATA="$(kubectl --kubeconfig "${ADMIN_KUBECONFIG}" config view --raw --minify -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')"
test -n "${CA_DATA}"
printf '%s' "${CA_DATA}" | base64 --decode >"${TMP_DIR}/cluster-ca.crt"
kubectl --kubeconfig "${VERIFIER_KUBECONFIG}" config set-cluster "${CLUSTER_NAME}" \
  --server="$(kubectl --kubeconfig "${ADMIN_KUBECONFIG}" config view --minify -o jsonpath='{.clusters[0].cluster.server}')" \
  --certificate-authority="${TMP_DIR}/cluster-ca.crt" --embed-certs=true >/dev/null
kubectl --kubeconfig "${VERIFIER_KUBECONFIG}" config set-credentials seccomp-verifier --token="${TOKEN}" >/dev/null
kubectl --kubeconfig "${VERIFIER_KUBECONFIG}" config set-context "${CONTEXT_NAME}" --cluster="${CLUSTER_NAME}" --user=seccomp-verifier --namespace="${VERIFY_NS}" >/dev/null
kubectl --kubeconfig "${VERIFIER_KUBECONFIG}" config use-context "${CONTEXT_NAME}" >/dev/null
unset TOKEN

export LANDLOCK_GENPROF_LIVE_KIND_VERIFICATION=1
export LANDLOCK_GENPROF_VERIFIER_KUBECONFIG="${VERIFIER_KUBECONFIG}"
export LANDLOCK_GENPROF_VERIFIER_NAMESPACE="${VERIFY_NS}"
export LANDLOCK_GENPROF_VERIFIER_NODE="${NODE}"
export LANDLOCK_GENPROF_VERIFIER_IMAGE="${IMAGE_REF}"
export LANDLOCK_GENPROF_VERIFIER_PROFILE="${PROFILE_PATH}"
go test ./internal/seccompverification -run '^TestLiveKindTwinPodRuntimeVerification$' -count=1 -v
collect_diagnostics
