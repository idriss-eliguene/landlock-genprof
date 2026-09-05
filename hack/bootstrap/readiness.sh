#!/usr/bin/env bash
set -Eeuo pipefail

wait_readiness() {
  local label="$1" timeout="$2"; shift 2
  local started now elapsed
  started="$(date +%s)"
  while true; do
    now="$(date +%s)"
    elapsed=$((now - started))
    if "$@"; then
      echo "[ready] ${label} elapsed=${elapsed}s"
      return 0
    fi
    if [ "$elapsed" -ge "$timeout" ]; then
      echo "[error] ${label} not ready after ${elapsed}s" >&2
      return 1
    fi
    echo "[wait] ${label} elapsed=${elapsed}s; current state is not ready"
    sleep 5
  done
}

api_ready() { kubectl version --request-timeout=5s >/dev/null 2>&1; }
kind_node_ready() { kubectl get nodes --no-headers 2>/dev/null | awk '$2 == "Ready" { n++ } END { exit !(n >= 1) }'; }
cilium_ready() {
  local values
  values="$(kubectl -n kube-system get ds cilium -o jsonpath='{.status.desiredNumberScheduled} {.status.numberReady} {.status.numberAvailable}' 2>/dev/null || true)"
  local desired ready available
  read -r desired ready available <<< "$values"
  [ "${desired:-0}" -gt 0 ] && [ "${ready:-0}" = "$desired" ] && [ "${available:-0}" = "$desired" ]
}
coredns_ready() {
  [ "$(kubectl -n kube-system get pods -l k8s-app=kube-dns -o jsonpath='{range .items[*]}{range .status.conditions[?(@.type=="Ready")]}{.status}{"\n"}{end}{end}' 2>/dev/null | grep -cx True || true)" -ge 1
}
core_system_ready() { kubectl -n kube-system get pods --no-headers 2>/dev/null | awk '$3 == "Running" { n++ } END { exit !(n >= 1) }'; }

dump_core_diagnostics() {
  echo "==== cluster info ====" >&2; kubectl cluster-info >&2 || true
  echo "==== nodes ====" >&2; kubectl get nodes -o wide >&2 || true
  echo "==== kube-system pods ====" >&2; kubectl get pods -n kube-system -o wide >&2 || true
  echo "==== cilium ====" >&2; kubectl -n kube-system get ds cilium -o wide >&2 || true
  kubectl -n kube-system get deploy cilium-operator -o wide >&2 || true
  echo "==== events ====" >&2; kubectl get events -A --sort-by=.lastTimestamp >&2 || true
}

wait_core_readiness() {
  local timeout="${1:-900}"
  wait_readiness API_READY "$timeout" api_ready || { dump_core_diagnostics; return 1; }
  wait_readiness KIND_NODE_READY "$timeout" kind_node_ready || { dump_core_diagnostics; return 1; }
  wait_readiness CILIUM_READY "$timeout" cilium_ready || { dump_core_diagnostics; return 1; }
  wait_readiness COREDNS_READY "$timeout" coredns_ready || { dump_core_diagnostics; return 1; }
  wait_readiness CORE_REQUIRED_SYSTEM_READY "$timeout" core_system_ready || { dump_core_diagnostics; return 1; }
  echo "PLATFORM_READY topology=kind+cilium"
}
