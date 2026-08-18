#!/usr/bin/env bash
# lib.sh — shared, deliberately dumb helpers for the v0.2 demo scripts.
#
# This file contains NO product logic. It resolves how to invoke the real
# CLI, prints section separators, and waits on Kubernetes state. It never
# computes a digest, never classifies confidence, never decides whether an
# approval is valid, and never predicts whether an apply will succeed —
# those are the product's job, and the product's exit status is the only
# authority this demo recognizes.
#
# Sourced by setup.sh, reset.sh, scenario.sh and record.sh.

# Shared configuration -----------------------------------------------------
# All overridable so the demo can run against a differently-named cluster
# without editing scripts.
DEMO_NAMESPACE="${DEMO_NAMESPACE:-landlock-genprof-e2e}"
DEMO_POD="${DEMO_POD:-nginx-demo}"
# The traced container is the tools sidecar and the traced binary is its
# curl, matching the Golden fixture the E2E already exercises: curl is
# what actually performs the deterministic filesystem and network actions,
# so it is the process whose behavior is worth observing.
DEMO_CONTAINER="${DEMO_CONTAINER:-tools}"
DEMO_BINARY="${DEMO_BINARY:-/usr/bin/curl}"
DEMO_DURATION="${DEMO_DURATION:-40s}"
DEMO_EXPECTED_CONTEXT="${DEMO_EXPECTED_CONTEXT-kind-landlock-genprof-e2e}"

DEMO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC2034  # consumed by the scripts that source this file
REPO_ROOT="$(cd "${DEMO_ROOT}/.." && pwd)"
DEMO_STATE="${DEMO_STATE:-${DEMO_ROOT}/.state}"

# Presentation -------------------------------------------------------------
demo_rule() { printf '%s\n' "────────────────────────────────────────────────────────────────────"; }

demo_stage() {
  printf '\n'
  demo_rule
  printf '  %s\n' "$*"
  demo_rule
}

demo_note() { printf '  %s\n' "$*"; }
demo_err()  { printf 'ERROR: %s\n' "$*" >&2; }

# CLI resolution -----------------------------------------------------------
# Canonical form is the kubectl plugin, which is what the docs, the E2E and
# review's own printed next-steps all use. LANDLOCK_GENPROF_BIN overrides it
# with an explicit binary (or wrapper) for environments where the plugin is
# not on PATH — same escape hatch the Makefile already honours.
demo_resolve_cli() {
  if [ -n "${LANDLOCK_GENPROF_BIN:-}" ]; then
    # Deliberately word-split: the override may carry arguments, e.g. a
    # container wrapper.
    # shellcheck disable=SC2206
    CLI_CMD=(${LANDLOCK_GENPROF_BIN})
    # shellcheck disable=SC2034  # consumed by the scripts that source this file
    CLI_MODE="binary-override"
  else
    CLI_CMD=(kubectl landlock-genprof)
    # shellcheck disable=SC2034  # consumed by the scripts that source this file
    CLI_MODE="kubectl-plugin"
    if ! command -v kubectl-landlock_genprof >/dev/null 2>&1; then
      demo_err "kubectl-landlock_genprof not found on PATH."
      demo_err "Install it with 'make install-plugin', or set LANDLOCK_GENPROF_BIN."
      return 1
    fi
  fi

  if ! "${CLI_CMD[@]}" --help >/dev/null 2>&1; then
    demo_err "'${CLI_CMD[*]} --help' failed — the CLI is not usable."
    return 1
  fi
  return 0
}

# Preconditions ------------------------------------------------------------
demo_require_cmd() {
  local missing=0 c
  for c in "$@"; do
    if ! command -v "$c" >/dev/null 2>&1; then
      demo_err "required command not found: $c"
      missing=1
    fi
  done
  return "$missing"
}

# Fail closed on an unexpected kubeconfig context: every script here
# mutates cluster state, and doing that against the wrong cluster is the
# one demo mistake with consequences outside the demo.
demo_check_context() {
  local current
  current="$(kubectl config current-context 2>/dev/null || true)"
  if [ -z "${DEMO_EXPECTED_CONTEXT}" ]; then
    demo_err "DEMO_EXPECTED_CONTEXT is empty; refusing to run"
    return 1
  fi
  if [ "$current" != "${DEMO_EXPECTED_CONTEXT}" ]; then
    demo_err "kubeconfig context is '${current}', expected '${DEMO_EXPECTED_CONTEXT}'."
    demo_err "Switch context, or set DEMO_EXPECTED_CONTEXT to override."
    return 1
  fi
  demo_note "context: ${current}"
  return 0
}

# Waiting ------------------------------------------------------------------
# Generic poll helper. Callers pass a command; this only decides *when to
# stop waiting*, never what the result means.
demo_wait_for() {
  local desc="$1" timeout="$2"; shift 2
  local waited=0
  while [ "$waited" -lt "$timeout" ]; do
    if "$@" >/dev/null 2>&1; then
      demo_note "ready: ${desc} (${waited}s)"
      return 0
    fi
    sleep 2
    waited=$((waited + 2))
  done
  demo_err "timed out after ${timeout}s waiting for ${desc}"
  return 1
}

demo_wait_pod_ready() {
  kubectl wait --for=condition=Ready "pod/${DEMO_POD}" \
    -n "${DEMO_NAMESPACE}" --timeout="${1:-180s}"
}

# The tools sidecar owns the curl binary every deterministic action runs
# through; exec'ing it before the first trace avoids racing container
# start against the first action.
demo_wait_tools_exec() {
  demo_wait_for "tools exec (${DEMO_BINARY})" "${1:-120}" \
    kubectl exec -n "${DEMO_NAMESPACE}" "${DEMO_POD}" -c "${DEMO_CONTAINER}" -- \
      "${DEMO_BINARY}" --version
}

demo_state_dir() {
  mkdir -p "${DEMO_STATE}"
}
