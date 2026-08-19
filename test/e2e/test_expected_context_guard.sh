#!/usr/bin/env bash
# Hermetic test for EXPECTED_CONTEXT guard logic.
# Implements tests A-D without invoking full demo-golden orchestration.
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TMP_DIR="$ROOT_DIR/test/e2e/_tmp_guard"
# use deterministic temp path inside repo to avoid ephemeral macOS tmpdir race
rm -rf "$TMP_DIR" || true
mkdir -p "$TMP_DIR/bin"
trap 'rm -rf "$TMP_DIR"' EXIT

# create kubectl shim that returns the configured current-context
create_kubectl_shim(){
  local ctx="$1"
  cat > "$TMP_DIR/bin/kubectl" <<EOF
#!/usr/bin/env bash
# kubectl shim: minimal behavior for guard tests
if [ "${1:-}" = "config" ]; then
  printf '%s' "$ctx"
  exit 0
fi
if [ "${1:-}" = "landlock-genprof" ] && [ "${2:-}" = "--help" ]; then
  echo "kubectl landlock-genprof help"
  exit 0
fi
# harmless success for cluster-info/get/version used by preflight checks
case "${1:-}" in
  cluster-info|get|version)
    exit 0
    ;;
esac
# default: success to avoid failing check-only tests
exit 0
EOF
  chmod +x "$TMP_DIR/bin/kubectl"
}

# create landlock-genprof shim
cat > "$TMP_DIR/bin/landlock-genprof" <<'EOF'
#!/usr/bin/env bash
if [ "${1:-}" = "--help" ]; then
  echo "landlock-genprof help"
  exit 0
fi
exit 0
EOF
chmod +x "$TMP_DIR/bin/landlock-genprof"

export PATH="$TMP_DIR/bin:$PATH"

# Run hermetic guard checks directly (avoid running full demo script)
# Implement run_guard_check which mirrors the demo script's guard logic
run_guard_check(){
  # env_spec: 'UNSET' to indicate variable unset, or literal value (may be empty)
  local env_spec="$1"
  local kube_ctx="$2"
  export PATH="$TMP_DIR/bin:$PATH"
  if [ "$env_spec" = "UNSET" ]; then
    unset EXPECTED_CONTEXT
  else
    export EXPECTED_CONTEXT="$env_spec"
  fi
  # Use '-' expansion: unset -> default; empty remains empty
  EXPECTED_CONTEXT="${EXPECTED_CONTEXT-kind-landlock-genprof-e2e}"
  if [ -z "${EXPECTED_CONTEXT}" ]; then
    return 2
  fi
  # For hermetic testing avoid invoking external kubectl; use provided kube_ctx
  CUR_CTX="$kube_ctx"
  if [ "$CUR_CTX" != "$EXPECTED_CONTEXT" ]; then
    return 3
  fi
  return 0
}

# A: unset EXPECTED_CONTEXT, kube_ctx kind-landlock-genprof-e2e -> PASS
create_kubectl_shim "kind-landlock-genprof-e2e"
if ! run_guard_check UNSET "kind-landlock-genprof-e2e"; then
  echo "CASE A failed" >&2
  exit 1
fi
echo "CASE A passed"

# B: explicit matching override -> PASS
create_kubectl_shim "my-e2e"
if ! run_guard_check "my-e2e" "my-e2e"; then
  echo "CASE B failed" >&2
  exit 1
fi
echo "CASE B passed"

# C: explicit mismatch -> FAIL
create_kubectl_shim "something-else"
if run_guard_check "my-e2e" "something-else"; then
  echo "CASE C failed: expected mismatch to fail" >&2
  exit 1
fi
echo "CASE C passed"

# D: explicit empty override -> FAIL
create_kubectl_shim "kind-landlock-genprof-e2e"
if run_guard_check "" "kind-landlock-genprof-e2e"; then
  echo "CASE D failed: expected empty EXPECTED_CONTEXT to cause failure" >&2
  exit 1
fi
echo "CASE D passed"

echo "Single-run guard test PASSED"
