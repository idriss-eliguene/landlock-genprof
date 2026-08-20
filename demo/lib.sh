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
# The canonical demo now runs on a REAL-NODE cluster (k3s), not kind: it
# consumes real security-profiles-operator recordings, and SPO's eBPF
# recorder cannot resolve container pids when the node is itself a
# container (see test/e2e/install-k3s.sh). k3s names its context "default".
DEMO_EXPECTED_CONTEXT="${DEMO_EXPECTED_CONTEXT-default}"

# SPO source material, pre-baked by demo/spo-sources.sh. Two real
# recordings: A is the workload's original behavior, B is the same workload
# after it changed. Both are produced by the real operator; nothing here is
# hand-written.
DEMO_SPO_NAMESPACE="${DEMO_SPO_NAMESPACE:-security-profiles-operator}"
DEMO_RECORDING_A="${DEMO_RECORDING_A:-lgdemo-a}"
DEMO_RECORDING_B="${DEMO_RECORDING_B:-lgdemo-b}"
# SPO names a recorded profile "<recording>-<container>" under
# mergeStrategy: None. Derived, not guessed — see createProfileName.
DEMO_SOURCE_A="${DEMO_SOURCE_A:-${DEMO_RECORDING_A}-${DEMO_CONTAINER}}"
DEMO_SOURCE_B="${DEMO_SOURCE_B:-${DEMO_RECORDING_B}-${DEMO_CONTAINER}}"

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

# Display a product command exactly before executing it. This is deliberately
# presentation-only: it does not change arguments, exit status or product
# semantics. Internal kubectl/jq plumbing should continue to execute silently.
# Short presenter-facing explanation. It is visible in the canonical cast,
# but carries no product semantics.
demo_explain() {
  printf '\n'
  local line
  for line in "$@"; do
    printf '  %s\n' "$line"
  done
  printf '\n'
}

# Presentation input must never affect product command semantics.
# Read from the controlling terminal explicitly because scenario commands may
# run under pipes/redirections. A missing TTY degrades to no pause rather than
# turning a successful product command into a demo failure.
demo_wait_enter() {
  if [ "${DEMO_PACED:-0}" != "1" ]; then
    return 0
  fi

  if [ -r /dev/tty ]; then
    read -r _ </dev/tty || true
  fi

  return 0
}

demo_show_cmd() {
  printf '\n'
  printf '  $'
  printf ' %q' "$@"
  printf '\n'
}

# Presentation gate around a real product command.
#
# DEMO_FAST=1:
#   no presentation delay.
#
# DEMO_PACED=1:
#   presenter controls when the command runs and when the audience moves on.
demo_cmd() {
  demo_show_cmd "$@"

  if [ "${DEMO_PACED:-0}" = "1" ]; then
    printf '\n'
    printf '  Press Enter to run this command...'
    demo_wait_enter
    printf '\n\n'
  fi

  local rc=0
  if "$@"; then
    rc=0
  else
    rc=$?
  fi

  if [ "${DEMO_PACED:-0}" = "1" ]; then
    printf '\n'
    printf '  Press Enter to continue...'
    demo_wait_enter
    printf '\n'
  fi

  return "$rc"
}

# Some commands participate in a timed observation window. Pausing before
# execution would change the experiment by letting workload actions happen
# before the tracer is active. Display them immediately, execute them
# immediately, and pause only after their result.
demo_cmd_timed() {
  demo_show_cmd "$@"

  printf '\n'

  local rc=0
  if "$@"; then
    rc=0
  else
    rc=$?
  fi

  if [ "${DEMO_PACED:-0}" = "1" ]; then
    printf '\n'
    printf '  Press Enter to continue...'
    demo_wait_enter
    printf '\n'
  fi

  return "$rc"
}


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

# The canonical v0.2 scenario imports a real SPO-derived SeccompProfile.
# Checking only the top-level --help is insufficient: an older plugin may be
# perfectly executable while lacking the trace contract the demo requires.
#
# Probe the capability itself rather than trusting a version string.
demo_require_v02_cli() {
  local trace_help resolved version

  if ! trace_help="$("${CLI_CMD[@]}" trace --help 2>&1)"; then
    demo_err "cannot inspect the trace command of '${CLI_CMD[*]}'."
    return 1
  fi

  if ! grep -q -- '--seccomp-source' <<<"${trace_help}"; then
    resolved="${CLI_CMD[*]}"

    if [ "${CLI_MODE}" = "kubectl-plugin" ] \
      && command -v kubectl-landlock_genprof >/dev/null 2>&1; then
      resolved="$(command -v kubectl-landlock_genprof)"
    fi

    version="$("${CLI_CMD[@]}" version 2>&1 || true)"

    demo_err "the resolved landlock-genprof CLI is incompatible with the canonical v0.2 demo."
    demo_err "resolved executable: ${resolved}"
    [ -n "${version}" ] && demo_err "reported version: ${version}"
    demo_err "required capability is missing: trace --seccomp-source"
    demo_err "install the CLI from this checkout with 'make install-plugin',"
    demo_err "or set LANDLOCK_GENPROF_BIN to a compatible binary."
    return 1
  fi

  demo_note "CLI capability present: trace --seccomp-source"
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

# Presentation panels ------------------------------------------------------
# These format; they never compute. Every value printed under a heading is
# passed in by a caller that read it from real cluster or CLI state.
demo_panel() {
  local title="$1"; shift
  printf '\n  ┌─ %s\n' "${title}"
  local line
  for line in "$@"; do
    printf '  │  %s\n' "${line}"
  done
  printf '  └─\n\n'
}

# A deliberate pause so a viewer can read what just happened.
#
#   DEMO_FAST=1    no pause at all — CI should spend its minutes on the
#                  product, not on beats.
#   DEMO_PACED=1   wait for a keypress instead of a timer, so a presenter
#                  controls the tempo live rather than racing the script.
#
# Both are presentation only. Neither changes a single command the demo runs.
demo_beat() {
  [ "${DEMO_FAST:-0}" = "1" ] && return 0
  if [ "${DEMO_PACED:-0}" = "1" ] && [ -t 0 ]; then
    printf '  … press Enter to continue'
    read -r _ || true
    return 0
  fi
  sleep "${1:-2}"
}

# Reads a field off the live proposal. Returns empty when absent; callers
# decide what that means.
demo_proposal_field() {
  kubectl get securityprofileproposal "${DEMO_POD}" -n "${DEMO_NAMESPACE}" \
    -o jsonpath="{$1}" 2>/dev/null || true
}
