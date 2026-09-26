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
TARGET_INSTANCE="${INSTANCE:-}"
SOURCE_SHA=""
SOURCE_SHORT_SHA=""
SOURCE_DIR=""
STATE_ROOT=""
EPHEMERAL_ROOT=""
KUBECONFIG_PATH=""
EXPECTED_CONTEXT=""
OWNERSHIP_FILE=""
DOCKER_CONTEXT_NAME=""
DOCKER_ENDPOINT=""
DOCKER_DAEMON_ID=""
GO_TOOLCHAIN=""
KUBECTL_VERSION=""
KIND_VERSION=""
KIND_NODE_IMAGE=""
HELM_VERSION=""
CILIUM_VERSION=""
IG_VERSION=""
CONTROL_PLANE_ID=""

die() { echo "ERROR: $*" >&2; exit 1; }
log() { echo "[dev-env] $*"; }

usage() {
  cat <<'EOF'
Usage: hack/dev-env.sh <doctor|up|status|test|e2e|down>

VERSION is required for up, status, test, e2e and down. It must identify a
local Git tag or commit; this command never falls back to HEAD and never
changes the current checkout.

INSTANCE is optional. When set, it must be 1-24 characters using lowercase
letters, digits and internal hyphens, and gives the environment a distinct
cluster and state directory for the same VERSION.

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
  validate_instance
  SOURCE_SHA="$(git -C "$ROOT_DIR" rev-parse --verify --end-of-options "${TARGET_VERSION}^{commit}" 2>/dev/null || true)"
  [ -n "$SOURCE_SHA" ] || die "unknown VERSION '$TARGET_VERSION'; fetch the tag/commit explicitly and retry"
  SOURCE_SHORT_SHA="${SOURCE_SHA:0:12}"
  if [ -n "$TARGET_INSTANCE" ]; then
    STATE_ROOT="$STATE_BASE/$SOURCE_SHA/$TARGET_INSTANCE"
  else
    STATE_ROOT="$STATE_BASE/$SOURCE_SHA"
  fi
  SOURCE_DIR="$STATE_ROOT/source"
  KUBECONFIG_PATH="$STATE_ROOT/kubeconfig"
  if [ -n "$TARGET_INSTANCE" ]; then
    CLUSTER_NAME="landlock-genprof-dev-${SOURCE_SHORT_SHA}-${TARGET_INSTANCE}"
  else
    CLUSTER_NAME="landlock-genprof-dev-${SOURCE_SHORT_SHA}"
  fi
  EXPECTED_CONTEXT="kind-${CLUSTER_NAME}"
  OWNERSHIP_FILE="$STATE_ROOT/ownership.json"
}

validate_instance() {
  [ -z "$TARGET_INSTANCE" ] && return 0
  [ "${#TARGET_INSTANCE}" -le 24 ] || die "INSTANCE must be at most 24 characters"
  [[ "$TARGET_INSTANCE" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] ||
    die "INSTANCE must contain only lowercase letters, digits and internal hyphens"
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
  validate_state_root
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

source_pin_from_git() {
  local name="$1" value
  value="$(git -C "$ROOT_DIR" show "$SOURCE_SHA:hack/versions.env" | sed -n "s/^${name}=//p" | head -n 1)"
  [ -n "$value" ] || die "selected source has no ${name} pin"
  printf '%s\n' "$value"
}

load_source_metadata_from_git() {
  local toolchain_line go_line
  git -C "$ROOT_DIR" cat-file -e "$SOURCE_SHA:hack/versions.env" || die "selected source lacks hack/versions.env"
  git -C "$ROOT_DIR" cat-file -e "$SOURCE_SHA:hack/bootstrap.sh" || die "selected source lacks hack/bootstrap.sh"
  git -C "$ROOT_DIR" cat-file -e "$SOURCE_SHA:go.mod" || die "selected source lacks go.mod"
  toolchain_line="$(git -C "$ROOT_DIR" show "$SOURCE_SHA:go.mod" | awk '$1 == "toolchain" { print $2; exit }')"
  go_line="$(git -C "$ROOT_DIR" show "$SOURCE_SHA:go.mod" | awk '$1 == "go" { print $2; exit }')"
  GO_TOOLCHAIN="${toolchain_line:-$go_line}"
  GO_TOOLCHAIN="${GO_TOOLCHAIN#go}"
  [ -n "$GO_TOOLCHAIN" ] || die "selected source has no Go version in go.mod"
  KUBECTL_VERSION="$(source_pin_from_git KUBECTL_VERSION)"
  KIND_VERSION="$(source_pin_from_git KIND_VERSION)"
  KIND_NODE_IMAGE="$(source_pin_from_git KIND_NODE_IMAGE)"
  HELM_VERSION="$(source_pin_from_git HELM_VERSION)"
  CILIUM_VERSION="$(source_pin_from_git CILIUM_VERSION)"
  IG_VERSION="$(source_pin_from_git IG_VERSION)"
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
  DOCKER_DAEMON_ID="$(docker info --format '{{.ID}}' 2>/dev/null || true)"
  [[ "$DOCKER_DAEMON_ID" =~ ^[A-Za-z0-9._:-]+$ ]] ||
    die "could not determine a stable Docker daemon identity"
  if docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -Eiq '(^|[^[:alnum:]])rootless([^[:alnum:]]|$)'; then
    die "rootless Docker is not supported for the Cilium development cluster"
  fi
  echo "DOCKER_CONTEXT=${DOCKER_CONTEXT_NAME}"
  echo "DOCKER_ENDPOINT=${DOCKER_ENDPOINT}"
}

check_host_tools() {
  local command_name
  for command_name in bash git go kubectl kind helm tar awk realpath; do require_command "$command_name"; done
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
    printf 'instance=%s\n' "$TARGET_INSTANCE"
    printf 'kubeconfig=%s\n' "$KUBECONFIG_PATH"
    printf 'docker_context=%s\n' "$DOCKER_CONTEXT_NAME"
    printf 'docker_endpoint=%s\n' "$DOCKER_ENDPOINT"
    printf 'docker_daemon_id=%s\n' "$DOCKER_DAEMON_ID"
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
  export LANDLOCK_CORE_CLUSTER="$CLUSTER_NAME"
  mkdir -p "$XDG_STATE_HOME/landlock-genprof"
}

export_ephemeral_environment() {
  export KUBECONFIG="$EPHEMERAL_ROOT/kubeconfig"
  export GOPATH="$EPHEMERAL_ROOT/gopath"
  export PATH="$GOPATH/bin:$PATH"
  export XDG_STATE_HOME="$EPHEMERAL_ROOT/xstate"
  export LANDLOCK_CORE_CLUSTER="$CLUSTER_NAME"
  mkdir -p "$GOPATH/bin" "$XDG_STATE_HOME/landlock-genprof"
}

validate_state_root() {
  case "$STATE_BASE" in
    /*) ;;
    *) die "state base must be an absolute path: $STATE_BASE" ;;
  esac
  local expected_state_root="$STATE_BASE/$SOURCE_SHA"
  if [ -n "$TARGET_INSTANCE" ]; then expected_state_root="$expected_state_root/$TARGET_INSTANCE"; fi
  [ "$STATE_ROOT" = "$expected_state_root" ] || die "unsafe state path: $STATE_ROOT"
  [ -d "$STATE_ROOT" ] || die "state directory is missing: $STATE_ROOT"
  [ ! -L "$STATE_BASE" ] || die "state base is a symlink: $STATE_BASE"
  [ ! -L "$STATE_ROOT" ] || die "state directory is a symlink: $STATE_ROOT"
  [ -d "$SOURCE_DIR" ] || die "source snapshot is missing: $SOURCE_DIR"
  [ ! -L "$SOURCE_DIR" ] || die "source snapshot is a symlink: $SOURCE_DIR"
  local source_real link_path link_real
  source_real="$(realpath "$SOURCE_DIR" 2>/dev/null || true)"
  [ -n "$source_real" ] || die "source snapshot cannot be resolved: $SOURCE_DIR"
  local unsafe_path
  unsafe_path="$(find -P "$STATE_ROOT" ! -type l ! \( -type f -o -type d \) -print -quit)"
  [ -z "$unsafe_path" ] || die "unsafe entry in state directory: $unsafe_path"
  while IFS= read -r link_path; do
    [ -n "$link_path" ] || continue
    case "$link_path" in
      "$SOURCE_DIR"/*) ;;
      *) die "unsafe entry in state directory: $link_path" ;;
    esac
    link_real="$(realpath "$link_path" 2>/dev/null || true)"
    [ -n "$link_real" ] || die "dangling or looping symlink: $link_path"
    case "$link_real" in
      "$source_real"|"$source_real"/*) ;;
      *) die "symlink escapes immutable source snapshot: $link_path -> $link_real" ;;
    esac
  done < <(find -P "$STATE_ROOT" -type l -print)
}

control_plane_id() {
  local rows id name extra match_count=0 matched_id inspected_id
  rows="$(docker ps -a --no-trunc --filter "label=io.x-k8s.kind.cluster=${CLUSTER_NAME}" --format '{{.ID}} {{.Names}}')"
  if [ -n "$rows" ]; then
    while IFS=$' \t' read -r id name extra; do
      [[ "$id" =~ ^[0-9a-fA-F]{64}$ ]] || die "unexpected Docker container ID output"
      [ -n "$name" ] || die "unexpected Docker container name output"
      [ -z "$extra" ] || die "unexpected Docker container output"
      if [ "$name" = "${CLUSTER_NAME}-control-plane" ]; then
        match_count=$((match_count + 1))
        matched_id="$id"
      fi
    done <<< "$rows"
  fi
  [ "$match_count" -eq 1 ] || die "expected exactly one control-plane container for ${CLUSTER_NAME}; found ${match_count}"
  inspected_id="$(docker inspect "$matched_id" --format '{{.Id}}')"
  [[ "$inspected_id" =~ ^[0-9a-fA-F]{64}$ ]] || die "unexpected Docker inspect identity output"
  [ "$inspected_id" = "$matched_id" ] || die "Docker container identity changed during lookup"
  printf '%s\n' "$inspected_id"
}

verify_ownership() {
  validate_state_root
  [ -f "$OWNERSHIP_FILE" ] || die "cluster ownership record missing for source ${SOURCE_SHA}; refusing to adopt or delete a cluster"
  [ ! -L "$OWNERSHIP_FILE" ] || die "ownership record is a symlink: $OWNERSHIP_FILE"
  local record expected_record recorded_control_plane_id actual_control_plane_id
  record="$(cat "$OWNERSHIP_FILE")"
  recorded_control_plane_id="$(printf '%s\n' "$record" | sed -n 's/.*"controlPlaneID":"\([0-9a-fA-F][0-9a-fA-F]*\)".*/\1/p')"
  [ "${#recorded_control_plane_id}" -eq 64 ] || die "ownership record has no unambiguous control-plane identity"
  expected_record="{\"cluster\":\"${CLUSTER_NAME}\",\"context\":\"${EXPECTED_CONTEXT}\",\"instance\":\"${TARGET_INSTANCE}\",\"owner\":\"landlock-genprof\",\"sourceSHA\":\"${SOURCE_SHA}\",\"dockerContext\":\"${DOCKER_CONTEXT_NAME}\",\"dockerDaemonID\":\"${DOCKER_DAEMON_ID}\",\"controlPlaneID\":\"${recorded_control_plane_id}\",\"createdBy\":\"hack/dev-env.sh\"}"
  [ "$record" = "$expected_record" ] || die "ownership record does not exactly match the selected cluster and source"
  if cluster_exists; then
    actual_control_plane_id="$(control_plane_id)"
    [ "$actual_control_plane_id" = "$recorded_control_plane_id" ] ||
      die "control-plane identity does not match ownership record; refusing adoption or deletion"
  fi
}

prepare_state_cleanup() {
  validate_state_root
  local owner unsafe_owner
  owner="$(id -un)"
  unsafe_owner="$(find -P "$STATE_ROOT" ! -user "$owner" -print -quit)"
  [ -z "$unsafe_owner" ] || die "state entry has ambiguous ownership: $unsafe_owner"
  # Go module caches are intentionally read-only. Make only this verified,
  # disposable tree removable; never relax permissions before custody checks.
  chmod -R u+rw "$STATE_ROOT" || die "cannot make owned state directory removable: $STATE_ROOT"
}

remove_state_root() {
  rm -rf "$STATE_ROOT" || die "state cleanup interrupted: $STATE_ROOT"
  [ ! -e "$STATE_ROOT" ] || die "state cleanup incomplete: $STATE_ROOT"
}

has_unowned_runtime_state() {
  [ -e "$KUBECONFIG_PATH" ] || [ -e "$STATE_ROOT/gopath" ] ||
    [ -e "$STATE_ROOT/xstate" ] || [ -e "$STATE_ROOT/metadata.env" ] ||
    [ -e "$STATE_ROOT/evidence" ]
}

inspect_existing_state() {
  [ -e "$STATE_ROOT" ] || return 0
  validate_state_root
  if [ -e "$OWNERSHIP_FILE" ]; then
    verify_ownership
  elif has_unowned_runtime_state; then
    die "state directory contains unowned runtime state; refusing to reuse: $STATE_ROOT"
  else
    echo "STATE_EXISTING_UNOWNED_SOURCE=$STATE_ROOT"
  fi
}

cleanup_state() {
  verify_ownership
  prepare_state_cleanup
  remove_state_root
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
  validate_instance
  check_host_tools
  prepare_runtime
  check_resources
  if [ -n "$TARGET_VERSION" ]; then
    resolve_source; load_source_metadata_from_git
    inspect_existing_state
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
  local state_preexisted=0
  resolve_source
  check_host_tools
  if [ -e "$STATE_ROOT" ] || [ -L "$STATE_ROOT" ]; then
    state_preexisted=1
    validate_state_root
  fi
  materialize_source; load_source_metadata
  prepare_runtime; check_resources; check_go_toolchain
  export_isolated_environment
  if cluster_exists; then
    verify_ownership
    [ -f "$KUBECONFIG_PATH" ] || die "owned cluster exists but isolated kubeconfig is missing: $KUBECONFIG_PATH"
    log "reusing owned cluster $CLUSTER_NAME for source $SOURCE_SHA"
  else
    if [ "$state_preexisted" -eq 1 ] && [ ! -f "$OWNERSHIP_FILE" ] && has_unowned_runtime_state; then
      die "state directory contains unowned runtime state; refusing to reuse: $STATE_ROOT"
    fi
    if [ -e "$OWNERSHIP_FILE" ]; then die "ownership record exists but cluster is absent; refusing ambiguous recovery"; fi
    bash "$SOURCE_DIR/hack/bootstrap.sh" --lane core
    [ -f "$XDG_STATE_HOME/landlock-genprof/${CLUSTER_NAME}.json" ] || die "bootstrap did not create its ownership record"
    cp "$XDG_STATE_HOME/landlock-genprof/${CLUSTER_NAME}.json" "$OWNERSHIP_FILE"
    # Add immutable source custody to the selected bootstrap's ownership fact.
    CONTROL_PLANE_ID="$(control_plane_id)"
    printf '{"cluster":"%s","context":"%s","instance":"%s","owner":"landlock-genprof","sourceSHA":"%s","dockerContext":"%s","dockerDaemonID":"%s","controlPlaneID":"%s","createdBy":"hack/dev-env.sh"}\n' \
      "$CLUSTER_NAME" "$EXPECTED_CONTEXT" "$TARGET_INSTANCE" "$SOURCE_SHA" "$DOCKER_CONTEXT_NAME" "$DOCKER_DAEMON_ID" "$CONTROL_PLANE_ID" > "$OWNERSHIP_FILE"
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
  resolve_source; load_source_metadata_from_git; prepare_runtime
  if ! cluster_exists; then
    inspect_existing_state
    echo "DEV_STATUS=ABSENT"
    exit 0
  fi
  verify_ownership
  [ -f "$KUBECONFIG_PATH" ] || die "owned cluster exists but isolated kubeconfig is missing: $KUBECONFIG_PATH"
  export_isolated_environment
  materialize_source
  kubectl --context "$EXPECTED_CONTEXT" cluster-info >/dev/null
  kubectl --context "$EXPECTED_CONTEXT" get nodes -o wide
  kubectl --context "$EXPECTED_CONTEXT" get pods -A --no-headers | sed -n '1,80p'
  [ -f "$STATE_ROOT/metadata.env" ] && cat "$STATE_ROOT/metadata.env"
  echo "DEV_STATUS=READY"
}

test_source() {
  resolve_source; load_source_metadata_from_git
  for command_name in bash git go tar awk; do require_command "$command_name"; done
  check_go_toolchain
  EPHEMERAL_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/landlock-genprof-dev-test.XXXXXX")"
  trap 'rm -rf "$EPHEMERAL_ROOT"' EXIT
  STATE_BASE="$EPHEMERAL_ROOT"
  STATE_ROOT="$EPHEMERAL_ROOT/$SOURCE_SHA"
  if [ -n "$TARGET_INSTANCE" ]; then STATE_ROOT="$STATE_ROOT/$TARGET_INSTANCE"; fi
  SOURCE_DIR="$STATE_ROOT/source"
  KUBECONFIG_PATH="$STATE_ROOT/kubeconfig"
  OWNERSHIP_FILE="$STATE_ROOT/ownership.json"
  materialize_source
  export_ephemeral_environment
  make -C "$SOURCE_DIR" test-unit
  trap - EXIT
  rm -rf "$EPHEMERAL_ROOT"
}

e2e() {
  resolve_source; load_source_metadata_from_git; check_host_tools; prepare_runtime
  cluster_exists || die "isolated development cluster is absent: $CLUSTER_NAME"
  verify_ownership
  [ -f "$KUBECONFIG_PATH" ] || die "owned cluster is missing its isolated kubeconfig: $KUBECONFIG_PATH"
  export_isolated_environment; materialize_source
  [ "$(uname -s)" = Linux ] || die "dev-e2e requires a Linux execution host; macOS/Lima provisioning is supported by dev-up, but the tracer must run in a Linux guest executor"
  kubectl --context "$EXPECTED_CONTEXT" cluster-info >/dev/null || die "isolated development cluster is not reachable"
  [ -x "$GOPATH/bin/kubectl-landlock_genprof" ] || die "selected source plugin is not installed; run make dev-up VERSION=$TARGET_VERSION"
  EXPECTED_CONTEXT="$EXPECTED_CONTEXT" GOLDEN_ARTIFACTS_DIR="$STATE_ROOT/evidence" \
    PATH="$GOPATH/bin:$PATH" bash "$SOURCE_DIR/hack/demo-golden.sh"
}

down() {
  resolve_source; load_source_metadata_from_git; require_command kind; prepare_runtime
  if cluster_exists; then
    verify_ownership
    [ -f "$KUBECONFIG_PATH" ] || die "owned cluster exists but isolated kubeconfig is missing: $KUBECONFIG_PATH"
    [ ! -L "$KUBECONFIG_PATH" ] || die "isolated kubeconfig is a symlink: $KUBECONFIG_PATH"
    export_isolated_environment
    kubectl config current-context | grep -Fx "$EXPECTED_CONTEXT" >/dev/null || die "isolated kubeconfig is not on the owned context"
    prepare_state_cleanup
    kind delete cluster --name "$CLUSTER_NAME"
    remove_state_root
    echo "DEV_DOWN=PASS"
  elif [ -e "$STATE_ROOT" ]; then
    # Recover state left by an interrupted cleanup, but only after the same
    # exact ownership and path checks used while a cluster is present.
    cleanup_state
    echo "DEV_DOWN=STALE_STATE_CLEANED"
  else
    echo "DEV_DOWN=ABSENT"
  fi
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
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
fi
