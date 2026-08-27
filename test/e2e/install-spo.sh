#!/usr/bin/env bash
# install-spo.sh — install security-profiles-operator into the E2E cluster.
#
# Only needed by the SPO interoperability workflow. The Core E2E does not
# install SPO and must not start doing so: making the authoritative release
# gate depend on an external operator's install reliability is a cost this
# project deliberately does not take (see docs/adr/0007 and the Core E2E's
# own fail-closed proof).
#
# Everything is pinned. Both workarounds this repository documented for
# SPO v0.8.4's Helm chart are obsolete at v1.0.0 and are deliberately NOT
# carried forward — verified against the v1.0.0 manifest:
#
#   * the chart's spoImage.tag defaulted to `latest` against a staging
#     registry; deploy/operator.yaml pins
#     registry.k8s.io/security-profiles-operator/security-profiles-operator:v1.0.0
#   * the spod metrics sidecar was hardcoded to a discontinued
#     gcr.io/kubebuilder path; no RELATED_IMAGE_RBAC_PROXY or
#     kube-rbac-proxy reference exists in v1.0.0 at all
#
# The manifest also creates the security-profiles-operator namespace with
# the privileged pod-security labels already set, so no manual labelling
# step is needed either.

set -euo pipefail

# Pinned; never `latest`. SPO v1.0.0 is the first release serving
# SeccompProfile v1, and this project targets that API only
# (internal/spobackend).
SPO_VERSION="${SPO_VERSION:-v1.0.0}"
SPO_IMAGE="${SPO_IMAGE:-}"
SPO_IMAGE_PULL_POLICY="${SPO_IMAGE_PULL_POLICY:-Always}"
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.17.2}"

SPO_MANIFEST="${SPO_MANIFEST:-https://raw.githubusercontent.com/kubernetes-sigs/security-profiles-operator/${SPO_VERSION}/deploy/operator.yaml}"
CERT_MANAGER_MANIFEST="https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"

SPO_NAMESPACE="security-profiles-operator"

echo "[spo] installing cert-manager ${CERT_MANAGER_VERSION}"
# Required for SPO's webhook certificate injection — the CRDs carry
# cert-manager.io/inject-ca-from annotations, so the operator's webhook
# never becomes ready without it.
kubectl apply -f "${CERT_MANAGER_MANIFEST}"
kubectl -n cert-manager wait --for=condition=Available deployment --all --timeout=300s

echo "[spo] installing security-profiles-operator ${SPO_VERSION}"
if [ -n "${SPO_IMAGE}" ]; then
  echo "[spo] pinning operator image ${SPO_IMAGE}"
  {
    if [[ "${SPO_MANIFEST}" =~ ^https?:// ]]; then
      curl -fsSL "${SPO_MANIFEST}"
    else
      cat "${SPO_MANIFEST}"
    fi
  } \
    | sed \
      -e "s#image: gcr.io/k8s-staging-sp-operator/security-profiles-operator:latest#image: ${SPO_IMAGE}#" \
      -e "s#imagePullPolicy: Always#imagePullPolicy: ${SPO_IMAGE_PULL_POLICY}#" \
    | kubectl apply -f -
else
  kubectl apply -f "${SPO_MANIFEST}"
fi

echo "[spo] waiting for the operator deployment"
# The manifest asks for 3 replicas; a single-node kind cluster schedules
# them all on one node, which is fine.
kubectl -n "${SPO_NAMESPACE}" rollout status deployment/security-profiles-operator --timeout=300s
ACTUAL_IMAGE="$(kubectl -n "${SPO_NAMESPACE}" get deployment/security-profiles-operator -o jsonpath='{.spec.template.spec.containers[?(@.name=="security-profiles-operator")].image}')"
ACTUAL_PULL_POLICY="$(kubectl -n "${SPO_NAMESPACE}" get deployment/security-profiles-operator -o jsonpath='{.spec.template.spec.containers[?(@.name=="security-profiles-operator")].imagePullPolicy}')"
echo "[spo] deployed image ${ACTUAL_IMAGE}"
if [ -n "${SPO_IMAGE}" ] && [ "${ACTUAL_IMAGE}" != "${SPO_IMAGE}" ]; then
  echo "ERROR: deployed SPO image ${ACTUAL_IMAGE}, expected pinned ${SPO_IMAGE}" >&2
  exit 1
fi
if [ -n "${SPO_IMAGE}" ] && [ "${ACTUAL_PULL_POLICY}" != "${SPO_IMAGE_PULL_POLICY}" ]; then
  echo "ERROR: deployed SPO image pull policy ${ACTUAL_PULL_POLICY}, expected ${SPO_IMAGE_PULL_POLICY}" >&2
  exit 1
fi

echo "[spo] waiting for the spod DaemonSet"
# spod is not in the manifest: the operator creates it from a
# SecurityProfilesOperatorDaemon resource once it starts, so it has to be
# waited for in two steps — existence, then readiness.
for _ in $(seq 1 60); do
  if kubectl -n "${SPO_NAMESPACE}" get daemonset spod >/dev/null 2>&1; then
    break
  fi
  sleep 5
done
if ! kubectl -n "${SPO_NAMESPACE}" get daemonset spod >/dev/null 2>&1; then
  echo "ERROR: spod DaemonSet was never created by the operator" >&2
  kubectl -n "${SPO_NAMESPACE}" get pods -o wide >&2 || true
  exit 1
fi
kubectl -n "${SPO_NAMESPACE}" rollout status daemonset/spod --timeout=300s

echo "[spo] verifying the SeccompProfile API this project targets"
# Fail here rather than deep inside the interop scenario if upstream ever
# changes shape again: the adapter targets v1, cluster-scoped, and both
# facts are load-bearing (docs/adr/0007, docs/adr/0008).
SCOPE="$(kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io -o jsonpath='{.spec.scope}')"
VERSIONS="$(kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io -o jsonpath='{.spec.versions[*].name}')"
echo "[spo] SeccompProfile scope=${SCOPE} versions=${VERSIONS}"

if [ "${SCOPE}" != "Cluster" ]; then
  echo "ERROR: SeccompProfile scope is ${SCOPE}, expected Cluster — internal/spobackend assumes cluster-scoped" >&2
  exit 1
fi
case " ${VERSIONS} " in
  *" v1 "*) ;;
  *)
    echo "ERROR: SeccompProfile does not serve v1 (serves: ${VERSIONS}) — internal/spobackend targets v1" >&2
    exit 1
    ;;
esac

echo "[spo] ready"
