#!/usr/bin/env bash
set -Eeuo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bash-version.sh"
# Check this before probing the project layer so an unsupported interpreter
# cannot reach a later Bash 4-only command such as mapfile.
ensure_bash_interpreter 0 "$0" "$@" || exit 2
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bootstrap/readiness.sh"
failures=0
check() { if "$@" >/dev/null 2>&1; then echo "READY $*"; else echo "MISSING $*"; failures=$((failures + 1)); fi; }
os="$(uname -s)"; arch="$(uname -m)"
case "$os:$arch" in Linux:x86_64|Linux:amd64|Linux:aarch64|Linux:arm64|Darwin:x86_64|Darwin:amd64|Darwin:arm64) echo "HOST_SUPPORTED os=$os arch=$arch" ;; *) echo "HOST_UNSUPPORTED os=$os arch=$arch"; exit 2 ;; esac
echo "READY bash ${BASH_MIN_VERSION}+ (using ${BASH_BIN})"; check curl --version; check kubectl version --client; check kind version; check helm version
if [ "$os" = Linux ]; then check docker info; else check limactl list; check docker info; fi
if kubectl cluster-info >/dev/null 2>&1; then
  echo "API_READY context=$(kubectl config current-context)"
  kubectl get nodes -o wide || failures=$((failures + 1))
  if cilium_ready; then echo "CILIUM_READY version=${CILIUM_VERSION}"; else echo "CILIUM_NOT_READY expected=${CILIUM_VERSION}"; failures=$((failures + 1)); fi
  if coredns_ready; then echo "COREDNS_READY"; else echo "COREDNS_NOT_READY"; failures=$((failures + 1)); fi
else echo "API_NOT_REACHABLE"; failures=$((failures + 1)); fi
if kubectl get crd securityprofileproposals.landlockgenprof.io >/dev/null 2>&1 && kubectl get crd applyattempts.landlockgenprof.io >/dev/null 2>&1; then echo "PROJECT_BOOTSTRAP_READY"; else echo "PROJECT_BOOTSTRAP_REQUIRED"; fi
echo "LOCAL_ENVIRONMENT_READY=$([ "$failures" -eq 0 ] && echo true || echo false)"
exit "$failures"
