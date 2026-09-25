#!/usr/bin/env bash
# Version-targeted, ownership-gated contributor environment.
#
# This wrapper never changes the contributor's checkout. It materializes the
# selected Git object into private state and executes the selected source's
# bootstrap/test scripts from there.
set -Eeuo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_BASE="${XDG_STATE_HOME:-${HOME}/.local/state}/landlock-genprof/dev"
TARGET_VERSION="${VERSION:-}"
SOURCE_SHA=""
SOURCE_SHORT_SHA=""
SOURCE_DIR=""
STATE_ROOT=""
KUBECONFIG_PATH=""
EXPECTED_CONTEXT=""
OWNERSHIP_FILE=""
DOCKER_CONTEXT_NAME=""
DOCKER_ENDPOINT=""
GO_TOOLCHAIN=""
KUBECTL_VERSION=""
KIND_VERSION=""
KIND_NODE_IMAGE=""
HELM_VERSION=""
CILIUM_VERSION=""
IG_VERSION=""

die() { echo "ERROR: $*" >&2; exit 1; }
log() { echo "[dev-env] $*"; }

usage() {
  cat <<'EOF'
Usage: hack/dev-env.sh <doctor|up|status|test|e2e|down>

VERSION is required for up, status, test, e2e and down. It must identify a
local Git tag or commit; this command never falls back to HEAD and never
changes the current checkout.

Optional dependency installation for `up` is explicit:
  DEV_INSTALL_SPO=1 DEV_SPO_VERSION=v1.0.0 \
  DEV_CERT_MANAGER_VERSION=v1.17.2 hack/dev-env.sh up
  DEV_INSTALL_PODLOCK=1 DEV_PODLOCK_VERSION=0.1.1 \
  DEV_CERT_MANAGER_VERSION=v1.18.2 hack/dev-env.sh up
EOF
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required; install it explicitly and retry"
}

resolve_source() {
  [ -n "$TARGET_VERSION" ] || die "VERSION is required; use a tag or commit (no implicit HEAD fallback)"
  case "$TARGET_VERSION" in
    HEAD|refs/heads/*|origin/*|main|master) die "VERSION must be an immutable tag or commit, not '$TARGET_VERSION'" ;;
  esac
  SOURCE_SHA="$(git -C "$ROOT_DIR" rev-parse --verify --end-of-options "${TARGET_VERSION}^{commit}" 2>/dev/null || true)"
  [ -n "$SOURCE_SHA" ] || die "unknown VERSION '$TARGET_VERSION'; fetch the tag/commit explicitly and retry"
  SOURCE_SHORT_SHA="${SOURCE_SHA:0:12}"
  STATE_ROOT="$STATE_BASE/$SOURCE_SHA"
  SOURCE_DIR="$STATE_ROOT/source"
  KUBECONFIG_PATH="$STATE_ROOT/kubeconfig"
  EXPECTED_CONTEXT="kind-landlock-genprof-dev-${SOURCE_SHORT_SHA}"
  OWNERSHIP_FILE="$STATE_ROOT/ownership.json"
}

materialize_source() {
  mkdir -p "$STATE_ROOT"
  if [ -e "$SOURCE_DIR/.landlock-genprof-source-sha" ]; then
    [ "$(cat "$SOURCE_DIR/.landlock-genprof-source-sha")" = "$SOURCE_SHA" ] ||
      die "state source directory does not match requested source SHA: $SOURCE_DIR"
  elif [ -e "$SOURCE_DIR" ]; then
    die "refusing to overwrite existing non-source state directory: $SOURCE_DIR"
  else
    mkdir -p "$SOURCE_DIR"
    git -C "$ROOT_DIR" archive --format=tar "$SOURCE_SHA" | tar -xf - -C "$SOURCE_DIR"
    printf '%s\n' "$SOURCE_SHA" > "$SOURCE_DIR/.landlock-genprof-source-sha"
  fi
  [ -f "$SOURCE_DIR/hack/versions.env" ] || die "selected source lacks hack/versions.env"
  [ -f "$SOURCE_DIR/hack/bootstrap.sh" ] || die "selected source lacks hack/bootstrap.sh"
  [ -f "$SOURCE_DIR/go.mod" ] || die "selected source lacks go.mod"
}

source_pin() {
  local name="$1" value
  value="$(sed -n "s/^${name}=//p" "$SOURCE_DIR/hack/versions.env" | head -n 1)"
  [ -n "$value" ] || die "selected source has no ${name} pin"
  printf '%s\n' "$value"
}

load_source_metadata() {
  local toolchain_line go_line
  toolchain_line="$(awk '$1 == "toolchain" { print $2; exit }' "$SOURCE_DIR/go.mod")"
  go_line="$(awk '$1 == "go" { print $2; exit }' "$SOURCE_DIR/go.mod")"
  GO_TOOLCHAIN="${toolchain_line:-$go_line}"
  GO_TOOLCHAIN="${GO_TOOLCHAIN#go}"
  [ -n "$GO_TOOLCHAIN" ] || die "selected source has no Go version in go.mod"
  KUBECTL_VERSION="$(source_pin KUBECTL_VERSION)"
  KIND_VERSION="$(source_pin KIND_VERSION)"
  KIND_NODE_IMAGE="$(source_pin KIND_NODE_IMAGE)"
  HELM_VERSION="$(source_pin HELM_VERSION)"
  CILIUM_VERSION="$(source_pin CILIUM_VERSION)"
  IG_VERSION="$(source_pin IG_VERSION)"
  [[ "$KIND_NODE_IMAGE" == *@sha256:* ]] || die "selected KIND_NODE_IMAGE is not digest pinned"
}

check_go_toolchain() {
  require_command go
  local actual
  actual="$(go version | sed -n 's/.*go\([0-9][0-9.]*\).*/\1/p')"
  [ "$actual" = "$GO_TOOLCHAIN" ] ||
    die "Go toolchain mismatch: selected source requires ${GO_TOOLCHAIN}, found ${actual:-unknown}; install/select the required toolchain"
  echo "GO_TOOLCHAIN=${actual}"
}

check_resources() {
  local cpus memory_bytes memory_mib disk_kib disk_gib
  if [ "$(uname -s)" = Darwin ]; then
    cpus="$(sysctl -n hw.ncpu 2>/dev/null || echo 0)"
    memory_bytes="$(sysctl -n hw.memsize 2>/dev/null || echo 0)"
  else
    cpus="$(nproc 2>/dev/null || echo 0)"
    memory_bytes="$(awk '/MemTotal:/ {print $2 * 1024; exit}' /proc/meminfo 2>/dev/null || echo 0)"
  fi
  memory_mib=$((memory_bytes / 1024 / 1024))
  local disk_path="$STATE_BASE"
  [ -d "$disk_path" ] || disk_path="$HOME"
  disk_kib="$(df -Pk "$disk_path" | awk 'NR == 2 {print $4}')"
  disk_gib=$((disk_kib / 1024 / 1024))
  echo "RESOURCES cpus=${cpus} memoryMiB=${memory_mib} freeDiskGiB=${disk_gib}"
  [ "$cpus" -ge 4 ] || die "at least 4 CPUs are required for the disposable environment"
  [ "$memory_mib" -ge 6144 ] || die "at least 6144 MiB memory is required for the disposable environment"
  [ "$disk_gib" -ge 20 ] || die "at least 20 GiB free disk is required for the disposable environment"
}

prepare_runtime() {
  require_command docker
  DOCKER_CONTEXT_NAME="$(docker context show 2>/dev/null || true)"
  [ -n "$DOCKER_CONTEXT_NAME" ] || die "could not determine the active Docker context"
  if [ "$(uname -s)" = Darwin ]; then
    [ "$DOCKER_CONTEXT_NAME" = "lima-landlock-genprof-core" ] ||
      die "macOS development requires Docker context lima-landlock-genprof-core; refusing to switch contexts"
    require_command limactl
  fi
  DOCKER_ENDPOINT="$(docker context inspect "$DOCKER_CONTEXT_NAME" --format '{{.Endpoints.docker.Host}}' 2>/dev/null || true)"
  [ -n "$DOCKER_ENDPOINT" ] || die "could not resolve Docker endpoint for context $DOCKER_CONTEXT_NAME"
  export DOCKER_HOST="$DOCKER_ENDPOINT"
  docker info >/dev/null 2>&1 || die "Docker runtime is not reachable through $DOCKER_CONTEXT_NAME"
  if docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -Eiq '(^|[^[:alnum:]])rootless([^[:alnum:]]|$)'; then
    die "rootless Docker is not supported for the Cilium development cluster"
  fi
  echo "DOCKER_CONTEXT=${DOCKER_CONTEXT_NAME}"
  echo "DOCKER_ENDPOINT=${DOCKER_ENDPOINT}"
}

check_host_tools() {
  local command_name
  for command_name in bash git go kubectl kind helm tar awk; do require_command "$command_name"; done
  if [ "$(uname -s)" = Darwin ]; then require_command limactl; fi
}

write_metadata() {
  local kubectl_actual kind_actual helm_actual node_image cilium_image gadget_image cilium_image_id gadget_image_id
  kubectl_actual="$(kubectl version --client -o json 2>/dev/null | tr '\n' ' ' || true)"
  kind_actual="$(kind version 2>/dev/null | tr '\n' ' ' || true)"
  helm_actual="$(helm version --short 2>/dev/null | tr '\n' ' ' || true)"
  node_image="$(kubectl get node -o jsonpath='{.status.nodeInfo.osImage} {.status.nodeInfo.architecture}' 2>/dev/null || true)"
  cilium_image="$(kubectl -n kube-system get daemonset cilium -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)"
  gadget_image="$(kubectl -n gadget get daemonset gadget -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)"
  cilium_image_id="$(kubectl -n kube-system get pods -l k8s-app=cilium -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' 2>/dev/null || true)"
  gadget_image_id="$(kubectl -n gadget get pods -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' 2>/dev/null || true)"
  umask 077
  {
    printf 'source_sha=%s\n' "$SOURCE_SHA"
    printf 'requested_version=%s\n' "$TARGET_VERSION"
    printf 'cluster=%s\n' "${CLUSTER_NAME:-landlock-genprof-dev-${SOURCE_SHORT_SHA}}"
    printf 'context=%s\n' "$EXPECTED_CONTEXT"
    printf 'kubeconfig=%s\n' "$KUBECONFIG_PATH"
    printf 'docker_context=%s\n' "$DOCKER_CONTEXT_NAME"
    printf 'docker_endpoint=%s\n' "$DOCKER_ENDPOINT"
    printf 'go_toolchain=%s\n' "$GO_TOOLCHAIN"
    printf 'kubectl=%s\n' "$KUBECTL_VERSION"
    printf 'kind=%s\n' "$KIND_VERSION"
    printf 'kind_node_image=%s\n' "$KIND_NODE_IMAGE"
    printf 'helm=%s\n' "$HELM_VERSION"
    printf 'cilium=%s\n' "$CILIUM_VERSION"
    printf 'inspektor_gadget=%s\n' "$IG_VERSION"
    printf 'kubectl_effective=%s\n' "$kubectl_actual"
    printf 'kind_effective=%s\n' "$kind_actual"
    printf 'helm_effective=%s\n' "$helm_actual"
    printf 'node_runtime=%s\n' "$node_image"
    printf 'cilium_image=%s\n' "$cilium_image"
    printf 'gadget_image=%s\n' "$gadget_image"
    printf 'cilium_image_id=%s\n' "$cilium_image_id"
    printf 'gadget_image_id=%s\n' "$gadget_image_id"
    printf 'spo=%s\n' "${DEV_INSTALL_SPO:-0}"
    printf 'podlock=%s\n' "${DEV_INSTALL_PODLOCK:-0}"
  } > "$STATE_ROOT/metadata.env"
}

export_isolated_environment() {
  mkdir -p "$STATE_ROOT" "$STATE_ROOT/gopath/bin"
  chmod 700 "$STATE_ROOT"
  export KUBECONFIG="$KUBECONFIG_PATH"
  export GOPATH="$STATE_ROOT/gopath"
  export PATH="$STATE_ROOT/gopath/bin:$PATH"
  export XDG_STATE_HOME="$STATE_ROOT/xstate"
  export LANDLOCK_CORE_CLUSTER="landlock-genprof-dev-${SOURCE_SHORT_SHA}"
  CLUSTER_NAME="$LANDLOCK_CORE_CLUSTER"
  mkdir -p "$XDG_STATE_HOME/landlock-genprof"
}

verify_ownership() {
  [ -f "$OWNERSHIP_FILE" ] || die "cluster ownership record missing for source ${SOURCE_SHA}; refusing to adopt or delete a cluster"
  grep -Fq '"owner":"landlock-genprof"' "$OWNERSHIP_FILE" || die "invalid ownership record: $OWNERSHIP_FILE"
  grep -Fq "\"sourceSHA\":\"${SOURCE_SHA}\"" "$OWNERSHIP_FILE" || die "ownership record source SHA mismatch"
}

cluster_exists() { kind get clusters 2>/dev/null | grep -Fxq "$CLUSTER_NAME"; }

install_optional_backends() {
  if [ "${DEV_INSTALL_SPO:-0}" = 1 ]; then
    [ -x "$SOURCE_DIR/test/e2e/install-spo.sh" ] || die "selected source has no SPO installer"
    [ -n "${DEV_SPO_VERSION:-}" ] || die "DEV_SPO_VERSION is required with DEV_INSTALL_SPO=1"
    [ -n "${DEV_CERT_MANAGER_VERSION:-}" ] || die "DEV_CERT_MANAGER_VERSION is required with DEV_INSTALL_SPO=1"
    SPO_VERSION="$DEV_SPO_VERSION" CERT_MANAGER_VERSION="$DEV_CERT_MANAGER_VERSION" \
      bash "$SOURCE_DIR/test/e2e/install-spo.sh"
  fi
  if [ "${DEV_INSTALL_PODLOCK:-0}" = 1 ]; then
    [ -n "${DEV_PODLOCK_VERSION:-}" ] || die "DEV_PODLOCK_VERSION is required with DEV_INSTALL_PODLOCK=1"
    [ -n "${DEV_CERT_MANAGER_VERSION:-}" ] || die "DEV_CERT_MANAGER_VERSION is required with DEV_INSTALL_PODLOCK=1"
    helm repo add jetstack https://charts.jetstack.io >/dev/null
    helm repo add podlock https://flavio.github.io/podlock >/dev/null
    helm repo update jetstack podlock >/dev/null
    helm upgrade --install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace \
      --version "$DEV_CERT_MANAGER_VERSION" --set crds.enabled=true --wait --timeout 10m
    helm pull podlock/podlock --version "$DEV_PODLOCK_VERSION" --destination "$STATE_ROOT"
    helm upgrade --install podlock "$STATE_ROOT/podlock-${DEV_PODLOCK_VERSION}.tgz" \
      --namespace podlock --create-namespace --wait --timeout 10m
  fi
}

doctor() {
  check_host_tools
  prepare_runtime
  check_resources
  if [ -n "$TARGET_VERSION" ]; then
    resolve_source; materialize_source; load_source_metadata
    echo "SOURCE_SHA=${SOURCE_SHA}"
    echo "SOURCE_TOOLCHAIN=${GO_TOOLCHAIN}"
    printf 'SOURCE_PINS kubectl=%s kind=%s helm=%s cilium=%s gadget=%s\n' \
      "$KUBECTL_VERSION" "$KIND_VERSION" "$HELM_VERSION" "$CILIUM_VERSION" "$IG_VERSION"
  else
    echo "VERSION_TARGET=not-selected"
  fi
  echo "DEV_DOCTOR=PASS"
}

up() {
  resolve_source; materialize_source; load_source_metadata
  check_host_tools; prepare_runtime; check_resources; check_go_toolchain
  export_isolated_environment
  if cluster_exists; then
    verify_ownership
    [ -f "$KUBECONFIG_PATH" ] || die "owned cluster exists but isolated kubeconfig is missing: $KUBECONFIG_PATH"
    log "reusing owned cluster $CLUSTER_NAME for source $SOURCE_SHA"
  else
    if [ -e "$OWNERSHIP_FILE" ]; then die "ownership record exists but cluster is absent; refusing ambiguous recovery"; fi
    bash "$SOURCE_DIR/hack/bootstrap.sh" --lane core
    [ -f "$XDG_STATE_HOME/landlock-genprof/${CLUSTER_NAME}.json" ] || die "bootstrap did not create its ownership record"
    cp "$XDG_STATE_HOME/landlock-genprof/${CLUSTER_NAME}.json" "$OWNERSHIP_FILE"
    # Add immutable source custody to the selected bootstrap's ownership fact.
    printf '{"cluster":"%s","context":"%s","owner":"landlock-genprof","sourceSHA":"%s","createdBy":"hack/dev-env.sh"}\n' \
      "$CLUSTER_NAME" "$EXPECTED_CONTEXT" "$SOURCE_SHA" > "$OWNERSHIP_FILE"
  fi
  kubectl config current-context | grep -Fx "$EXPECTED_CONTEXT" >/dev/null || die "bootstrap selected an unexpected Kubernetes context"
  make -C "$SOURCE_DIR" test-env
  install_optional_backends
  write_metadata
  echo "DEV_UP=PASS"
  echo "SOURCE_SHA=${SOURCE_SHA}"
  echo "KUBECONFIG=${KUBECONFIG_PATH}"
  echo "CONTEXT=${EXPECTED_CONTEXT}"
}

status() {
  resolve_source; materialize_source; load_source_metadata
  export_isolated_environment; prepare_runtime
  if ! cluster_exists; then echo "DEV_STATUS=ABSENT"; exit 0; fi
  verify_ownership
  [ -f "$KUBECONFIG_PATH" ] || die "owned cluster exists but isolated kubeconfig is missing: $KUBECONFIG_PATH"
  kubectl --context "$EXPECTED_CONTEXT" cluster-info >/dev/null
  kubectl --context "$EXPECTED_CONTEXT" get nodes -o wide
  kubectl --context "$EXPECTED_CONTEXT" get pods -A --no-headers | sed -n '1,80p'
  [ -f "$STATE_ROOT/metadata.env" ] && cat "$STATE_ROOT/metadata.env"
  echo "DEV_STATUS=READY"
}

test_source() {
  resolve_source; materialize_source; load_source_metadata
  for command_name in bash git go tar awk; do require_command "$command_name"; done
  check_go_toolchain
  export_isolated_environment
  make -C "$SOURCE_DIR" test-unit
}

e2e() {
  resolve_source; materialize_source; load_source_metadata; check_host_tools; prepare_runtime
  export_isolated_environment; verify_ownership
  [ "$(uname -s)" = Linux ] || die "dev-e2e requires a Linux execution host; macOS/Lima provisioning is supported by dev-up, but the tracer must run in a Linux guest executor"
  kubectl --context "$EXPECTED_CONTEXT" cluster-info >/dev/null || die "isolated development cluster is not reachable"
  [ -x "$GOPATH/bin/kubectl-landlock_genprof" ] || die "selected source plugin is not installed; run make dev-up VERSION=$TARGET_VERSION"
  EXPECTED_CONTEXT="$EXPECTED_CONTEXT" GOLDEN_ARTIFACTS_DIR="$STATE_ROOT/evidence" \
    PATH="$GOPATH/bin:$PATH" bash "$SOURCE_DIR/hack/demo-golden.sh"
}

down() {
  resolve_source; materialize_source; load_source_metadata; require_command kind; prepare_runtime; export_isolated_environment
  cluster_exists || { echo "DEV_DOWN=ABSENT"; exit 0; }
  verify_ownership
  [ -f "$KUBECONFIG_PATH" ] || die "owned cluster exists but isolated kubeconfig is missing: $KUBECONFIG_PATH"
  kubectl config current-context | grep -Fx "$EXPECTED_CONTEXT" >/dev/null || die "isolated kubeconfig is not on the owned context"
  kind delete cluster --name "$CLUSTER_NAME"
  rm -rf "$STATE_ROOT"
  echo "DEV_DOWN=PASS"
}

command_name="${1:-}"
case "$command_name" in
  doctor) shift; doctor "$@" ;;
  up) shift; up "$@" ;;
  status) shift; status "$@" ;;
  test) shift; test_source "$@" ;;
  e2e) shift; e2e "$@" ;;
  down) shift; down "$@" ;;
  -h|--help|"") usage; [ -n "$command_name" ] || exit 2 ;;
  *) die "unknown command '$command_name'" ;;
esac
