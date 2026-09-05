#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/versions.env"
# shellcheck disable=SC1091
source "$ROOT_DIR/hack/bootstrap/readiness.sh"

LANE=core
PLAN=0
HOST_OS="${BOOTSTRAP_OS:-$(uname -s)}"
HOST_ARCH="${BOOTSTRAP_ARCH:-$(uname -m)}"
CLUSTER_NAME="${LANDLOCK_CORE_CLUSTER:-landlock-genprof-core}"
OWNERSHIP_DIR="${XDG_STATE_HOME:-${HOME}/.local/state}/landlock-genprof"
OWNERSHIP_FILE="$OWNERSHIP_DIR/${CLUSTER_NAME}.json"
# Default readiness budget: comfortably exceeds the ~31-minute (1860s) slow
# CoreDNS image pull observed in practice — see hack/bootstrap/readiness.sh.
# Override with LANDLOCK_CORE_READINESS_TIMEOUT if a slower environment needs it.
READINESS_TIMEOUT="${LANDLOCK_CORE_READINESS_TIMEOUT:-2700}"

die() { echo "ERROR: $*" >&2; exit 1; }
log() { echo "[bootstrap] $*"; }

usage() {
  cat <<'EOF'
Usage: hack/bootstrap.sh [--lane core] [--plan]

Creates or selects the contributor Core topology: kind + Cilium.
macOS uses a native-architecture Lima Linux guest; Linux uses its native
container runtime. Project CRDs, RBAC, Gadget, SPO, and PodLock belong to
make test-env or explicit optional setup, not this layer.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --lane) [ "${2:-}" = core ] || die "only --lane core is supported"; shift 2 ;;
    --plan) PLAN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

case "$HOST_OS:$HOST_ARCH" in
  Linux:x86_64|Linux:amd64|Linux:aarch64|Linux:arm64|Darwin:x86_64|Darwin:amd64|Darwin:arm64) ;;
  *) die "unsupported host OS/architecture: ${HOST_OS}/${HOST_ARCH}; supported Linux amd64/arm64 and Darwin amd64/arm64" ;;
esac

case "$HOST_ARCH" in
  x86_64|amd64) DOWNLOAD_ARCH=amd64; LIMA_ARCH=x86_64 ;;
  aarch64|arm64) DOWNLOAD_ARCH=arm64; LIMA_ARCH=aarch64 ;;
esac

if [ "$PLAN" -eq 1 ]; then
  case "$HOST_OS" in
    Linux) echo "PLATFORM_PLAN=linux-native-runtime kind+cilium" ;;
    Darwin) echo "PLATFORM_PLAN=macos-lima-native-${HOST_ARCH} kind+cilium" ;;
  esac
  echo "VERSIONS kubectl=${KUBECTL_VERSION} kind=${KIND_VERSION} helm=${HELM_VERSION} cilium=${CILIUM_VERSION} gadget=${IG_VERSION}"
  echo "PROJECT_LAYER=make-test-env"
  exit 0
fi

require_cmd() { command -v "$1" >/dev/null 2>&1 || die "$1 is required; install it explicitly and rerun"; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "sha256sum or shasum is required for download verification"
  fi
}

kubectl_client_version() {
  kubectl version --client -o json 2>/dev/null | awk -F'"' '/gitVersion/ {print $4; exit}'
}

install_kubectl() {
  local os arch url sum tmp expected actual bin_dir current
  if command -v kubectl >/dev/null 2>&1; then
    current="$(kubectl_client_version)"
    [ -n "$current" ] || die "installed kubectl is unusable"
    [ "$current" = "$KUBECTL_VERSION" ] || die "installed kubectl version mismatch: expected ${KUBECTL_VERSION}, found ${current}; install ${KUBECTL_VERSION} explicitly (e.g. remove or replace the kubectl on PATH) and rerun — bootstrap does not silently replace an unrelated system kubectl"
    return
  fi
  os=linux; [ "$HOST_OS" = Darwin ] && os=darwin
  arch="$DOWNLOAD_ARCH"
  bin_dir="${XDG_BIN_HOME:-${HOME}/.local/bin}"; mkdir -p "$bin_dir"
  tmp="$(mktemp)"; url="https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${os}/${arch}/kubectl"; sum="${url}.sha256"
  curl -fsSL "$url" -o "$tmp"; expected="$(curl -fsSL "$sum" | tr -d '[:space:]')"; actual="$(sha256_file "$tmp")"
  [ "$expected" = "$actual" ] || die "kubectl checksum mismatch"
  install -m 0755 "$tmp" "$bin_dir/kubectl"; rm -f "$tmp"; export PATH="$bin_dir:$PATH"
}

install_kind() {
  local os arch url sum tmp expected actual bin_dir
  command -v kind >/dev/null 2>&1 && { kind version | grep -q "${KIND_VERSION#v}" || die "installed kind version mismatch"; return; }
  os=linux; [ "$HOST_OS" = Darwin ] && os=darwin
  arch="$DOWNLOAD_ARCH"
  bin_dir="${XDG_BIN_HOME:-${HOME}/.local/bin}"; mkdir -p "$bin_dir"
  tmp="$(mktemp)"; url="https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-${os}-${arch}"; sum="${url}.sha256sum"
  curl -fsSL "$url" -o "$tmp"; expected="$(curl -fsSL "$sum" | awk '{for(i=1;i<=NF;i++) if ($i ~ /^[[:xdigit:]]{64}$/) {print tolower($i); exit}}')"; actual="$(sha256_file "$tmp")"
  [ "$expected" = "$actual" ] || die "kind checksum mismatch"
  install -m 0755 "$tmp" "$bin_dir/kind"; rm -f "$tmp"; export PATH="$bin_dir:$PATH"
}

install_helm() {
  local os arch url sum tmp expected actual extract bin_dir
  command -v helm >/dev/null 2>&1 && { helm version --short | grep -q "${HELM_VERSION#v}" || die "installed Helm version mismatch"; return; }
  os=linux; [ "$HOST_OS" = Darwin ] && os=darwin
  arch="$DOWNLOAD_ARCH"
  bin_dir="${XDG_BIN_HOME:-${HOME}/.local/bin}"; mkdir -p "$bin_dir"; tmp="$(mktemp)"
  url="https://get.helm.sh/helm-${HELM_VERSION}-${os}-${arch}.tar.gz"; sum="${url}.sha256sum"
  curl -fsSL "$url" -o "$tmp"; expected="$(curl -fsSL "$sum" | awk '{for(i=1;i<=NF;i++) if ($i ~ /^[[:xdigit:]]{64}$/) {print tolower($i); exit}}')"; actual="$(sha256_file "$tmp")"
  [ "$expected" = "$actual" ] || die "Helm checksum mismatch"
  extract="$(mktemp -d)"; tar -xzf "$tmp" -C "$extract"; install -m 0755 "$extract/${os}-${arch}/helm" "$bin_dir/helm"; rm -rf "$tmp" "$extract"; export PATH="$bin_dir:$PATH"
}

setup_lima() {
  local vm="landlock-genprof-core" context endpoint
  require_cmd limactl
  if ! limactl list --format '{{.Name}}' | grep -qx "$vm"; then
    log "creating native-architecture Lima VM ${vm}"
    limactl start --name "$vm" --arch "$LIMA_ARCH" template://docker-rootful
  else
    log "reusing Lima VM ${vm}"
    limactl start "$vm" >/dev/null
  fi
  context="lima-${vm}"
  if ! docker context inspect "$context" >/dev/null 2>&1; then
    endpoint="unix://${HOME}/.lima/${vm}/sock/docker.sock"
    docker context create "$context" --docker "host=$endpoint" >/dev/null
  fi
  export DOCKER_HOST="$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')"
  docker info >/dev/null 2>&1 || die "Lima Docker runtime is unreachable"
  assert_rootful_runtime
}

runtime_is_rootless() {
  docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -Eiq '(^|[^[:alnum:]])rootless([^[:alnum:]]|$)'
}

assert_rootful_runtime() {
  if runtime_is_rootless; then
    die "Core kind+Cilium requires a rootful container runtime; selected Docker runtime reports rootless. Recreate the owned Lima VM from template://docker-rootful and rerun"
  fi
  log "container runtime is rootful"
}

setup_runtime() {
  if [ "$HOST_OS" = Darwin ]; then
    setup_lima
  else
    require_cmd docker
    docker info >/dev/null 2>&1 || die "Docker/container runtime is unreachable"
    assert_rootful_runtime
  fi
}

setup_cluster() {
  local cfg
  if kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
    [ -f "$OWNERSHIP_FILE" ] || die "cluster ${CLUSTER_NAME} exists without landlock-genprof ownership; refusing to adopt/delete a shared cluster"
    log "reusing owned kind cluster ${CLUSTER_NAME}"
  else
    cfg="$(mktemp)"
    cat > "$cfg" <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: ${KIND_NODE_IMAGE}
networking:
  disableDefaultCNI: true
EOF
    kind create cluster --name "$CLUSTER_NAME" --config "$cfg"
    rm -f "$cfg"
    mkdir -p "$OWNERSHIP_DIR"
    printf '{"cluster":"%s","topology":"kind+cilium","owner":"landlock-genprof","createdBy":"hack/bootstrap.sh"}\n' "$CLUSTER_NAME" > "$OWNERSHIP_FILE"
  fi
  kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null
  kubectl cluster-info >/dev/null 2>&1 || die "kind API is not reachable"
}

setup_cilium() {
  helm repo add cilium https://helm.cilium.io/ >/dev/null
  helm repo update cilium >/dev/null
  helm upgrade --install cilium cilium/cilium --version "$CILIUM_VERSION" --namespace kube-system --create-namespace --set image.pullPolicy=IfNotPresent --set ipam.mode=kubernetes --set operator.replicas=1
}

# Guarded so tests can `source` this file (e.g. to call install_kubectl or
# inspect READINESS_TIMEOUT directly) without triggering real provisioning.
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  log "host=${HOST_OS} arch=${HOST_ARCH} lane=${LANE}"
  install_kubectl; install_kind; install_helm; setup_runtime; setup_cluster; setup_cilium
  wait_core_readiness "$READINESS_TIMEOUT"
  echo "PLATFORM_READY=true"
  echo "Project layer is intentionally separate: run 'make env-doctor' and then 'make test-env'."
fi
