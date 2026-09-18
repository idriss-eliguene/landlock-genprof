# Shared canonical Core (Lima/Docker/Kubernetes/CRD/topology) readiness
# checks, factored out of hack/ui-lima.sh so hack/ui-lima-auth.sh does not
# duplicate them. Source this after setting LIMA_VM and EXPECTED_CONTEXT,
# and after defining a die() function. It never mutates the canonical
# cluster/VM beyond starting the VM itself if it is merely Stopped; it never
# recreates the VM, cluster, or CRDs.

lib_core_readiness_require_commands() {
  for command_name in "$@"; do
    command -v "$command_name" >/dev/null 2>&1 || die "$command_name is required"
  done
}

lib_core_readiness_check() {
  local vm_record vm_status vm_arch docker_context current_context env_doctor_output

  vm_record="$(limactl list --format '{{.Name}} {{.Status}} {{.Arch}}' | awk -v vm="$LIMA_VM" '$1 == vm { print; exit }')"
  [ -n "$vm_record" ] || die "Lima VM '${LIMA_VM}' was not found"
  vm_status="$(printf '%s\n' "$vm_record" | awk '{print $2}')"
  vm_arch="$(printf '%s\n' "$vm_record" | awk '{print $3}')"
  if [ "$vm_status" = Stopped ]; then
    echo "LIMA_STARTING vm=${LIMA_VM}"
    limactl start "$LIMA_VM" >/dev/null
    vm_record="$(limactl list --format '{{.Name}} {{.Status}} {{.Arch}}' | awk -v vm="$LIMA_VM" '$1 == vm { print; exit }')"
    vm_status="$(printf '%s\n' "$vm_record" | awk '{print $2}')"
  fi
  [ "$vm_status" = Running ] || die "Lima VM '${LIMA_VM}' is ${vm_status}; refusing to recreate the canonical VM"
  case "$vm_arch" in
    aarch64|arm64) ;;
    *) die "Lima VM '${LIMA_VM}' has unsupported architecture '${vm_arch}'" ;;
  esac
  echo "LIMA_READY vm=${LIMA_VM} arch=${vm_arch}"

  docker_context="$(docker context show 2>/dev/null || true)"
  [ "$docker_context" = "lima-${LIMA_VM}" ] || die "Docker context is '${docker_context}', expected 'lima-${LIMA_VM}'; refusing to switch daemons"
  docker info >/dev/null 2>&1 || die "Docker is not reachable through context '${docker_context}'"
  echo "DOCKER_READY context=${docker_context}"

  current_context="$(kubectl config current-context 2>/dev/null || true)"
  [ "$current_context" = "$EXPECTED_CONTEXT" ] || die "Kubernetes context is '${current_context}', expected '${EXPECTED_CONTEXT}'"
  if ! kubectl cluster-info >/dev/null 2>&1; then
    # A stopped VM can leave the preserved kind control-plane container
    # stopped even though Docker and the kubeconfig are healthy. Recover only
    # this exact canonical node; never recreate the cluster or touch another
    # context. If it is merely booting, the bounded API wait below handles it.
    local control_plane="${LIMA_VM}-control-plane"
    local control_state
    control_state="$(docker ps -a --filter "name=^/${control_plane}$" --format '{{.Status}}' 2>/dev/null || true)"
    case "$control_state" in
      Exited\ *|Created\ *)
        echo "KIND_CONTROL_PLANE_STARTING name=${control_plane}"
        docker start "$control_plane" >/dev/null || die "failed to start preserved kind control-plane '${control_plane}'"
        ;;
      "")
        docker ps -a --filter "name=^/${control_plane}$" --format 'table {{.Names}}\t{{.Status}}' >&2 || true
        die "preserved kind control-plane '${control_plane}' was not found"
        ;;
    esac
  fi
  local api_deadline=$(( $(date +%s) + 120 ))
  while ! kubectl cluster-info >/dev/null 2>&1; do
    [ "$(date +%s)" -lt "$api_deadline" ] || die "Kubernetes API is not reachable through context '${EXPECTED_CONTEXT}' after bounded control-plane recovery"
    sleep 2
  done
  kubectl get --raw=/version >/dev/null 2>&1 || die "Kubernetes API version endpoint is not readable"
  echo "KUBERNETES_READY context=${current_context}"

  env_doctor_output="$(mktemp -t landlock-genprof-core-readiness.XXXXXX)"
  LIB_CORE_READINESS_ENV_DOCTOR_OUTPUT="$env_doctor_output"
  local env_deadline=$(( $(date +%s) + 120 ))
  while true; do
    if make -C "$ROOT_DIR" env-doctor | tee "$env_doctor_output" && grep -q '^LOCAL_ENVIRONMENT_READY=true$' "$env_doctor_output"; then
      break
    fi
    [ "$(date +%s)" -lt "$env_deadline" ] || die "canonical environment is not ready after bounded convergence; see make env-doctor output"
    echo "ENVIRONMENT_CONVERGING retrying readiness checks"
    sleep 5
  done
  echo "ENVIRONMENT_READY"

  local required_crds=(
    traininghistories.landlockgenprof.io
    securityprofileproposals.landlockgenprof.io
    applyattempts.landlockgenprof.io
    rollbackattempts.landlockgenprof.io
    observations.landlockgenprof.io
    observationcontributionreceipts.landlockgenprof.io
  )
  for crd in "${required_crds[@]}"; do
    kubectl wait --for=condition=Established "crd/${crd}" --timeout=30s >/dev/null || die "CRD ${crd} is not Established"
  done
  kubectl wait --for=condition=Ready "node/${LIMA_VM}-control-plane" --timeout=30s >/dev/null || die "canonical Core node is not Ready"
  kubectl -n kube-system rollout status daemonset/cilium --timeout=30s >/dev/null || die "Cilium is not Ready"
  kubectl -n kube-system rollout status deployment/coredns --timeout=30s >/dev/null || die "CoreDNS is not Ready"
  # The upstream Gadget DaemonSet may leave observedGeneration one revision
  # behind after a probe-only template patch even while its availability
  # fields are authoritative. Use the bounded readiness predicate directly;
  # never treat a merely existing Pod as healthy.
  local gadget_deadline=$(( $(date +%s) + 60 ))
  while true; do
    local gadget_status
    gadget_status="$(kubectl -n gadget get daemonset gadget -o jsonpath='{.status.desiredNumberScheduled} {.status.currentNumberScheduled} {.status.numberReady} {.status.updatedNumberScheduled} {.status.numberAvailable}' 2>/dev/null || true)"
    if [ "$gadget_status" = "1 1 1 1 1" ]; then
      break
    fi
    [ "$(date +%s)" -lt "$gadget_deadline" ] || die "Inspektor Gadget is not Ready (status: ${gadget_status:-unavailable})"
    sleep 2
  done
  echo "TOPOLOGY_READY node=${LIMA_VM}-control-plane cilium=${CILIUM_VERSION} gadget=${IG_VERSION}"
}

lib_core_readiness_cleanup() {
  if [ -n "${LIB_CORE_READINESS_ENV_DOCTOR_OUTPUT:-}" ]; then
    rm -f "$LIB_CORE_READINESS_ENV_DOCTOR_OUTPUT"
  fi
}
