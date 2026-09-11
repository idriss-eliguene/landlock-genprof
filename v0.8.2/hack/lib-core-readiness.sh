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
  kubectl cluster-info >/dev/null 2>&1 || die "Kubernetes API is not reachable through context '${EXPECTED_CONTEXT}'"
  kubectl get --raw=/version >/dev/null 2>&1 || die "Kubernetes API version endpoint is not readable"
  echo "KUBERNETES_READY context=${current_context}"

  env_doctor_output="$(mktemp -t landlock-genprof-core-readiness.XXXXXX)"
  LIB_CORE_READINESS_ENV_DOCTOR_OUTPUT="$env_doctor_output"
  make -C "$ROOT_DIR" env-doctor | tee "$env_doctor_output"
  grep -q '^LOCAL_ENVIRONMENT_READY=true$' "$env_doctor_output" || die "canonical environment is not ready; see make env-doctor output"
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
  kubectl -n gadget rollout status daemonset/gadget --timeout=30s >/dev/null || die "Inspektor Gadget is not Ready"
  echo "TOPOLOGY_READY node=${LIMA_VM}-control-plane cilium=${CILIUM_VERSION} gadget=${IG_VERSION}"
}

lib_core_readiness_cleanup() {
  if [ -n "${LIB_CORE_READINESS_ENV_DOCTOR_OUTPUT:-}" ]; then
    rm -f "$LIB_CORE_READINESS_ENV_DOCTOR_OUTPUT"
  fi
}
