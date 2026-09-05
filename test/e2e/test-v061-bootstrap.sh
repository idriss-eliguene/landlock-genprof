#!/usr/bin/env bash
set -Eeuo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export ROOT_DIR
BOOTSTRAP="$ROOT_DIR/hack/bootstrap.sh"
export BOOTSTRAP
for f in "$BOOTSTRAP" "$ROOT_DIR/hack/env-doctor.sh" "$ROOT_DIR/hack/test-env.sh" "$ROOT_DIR/hack/test-env-clean.sh" "$ROOT_DIR/hack/bootstrap/readiness.sh"; do test -x "$f" || { echo "not executable: $f"; exit 1; }; done

# Bash prerequisite: mapfile in the project-layer Gadget installer requires
# Bash 4.0. Test the shared comparator with representative interpreter
# versions, including malformed input, without invoking the real environment.
source "$ROOT_DIR/hack/versions.env"
source "$ROOT_DIR/hack/bash-version.sh"
if check_bash_version 3 2 >/tmp/v061-bash-version.out 2>&1; then
  echo "unsupported Bash was accepted"; exit 1
fi
grep -q "Bash ${BASH_MIN_VERSION} or newer is required" /tmp/v061-bash-version.out || { echo "missing Bash prerequisite diagnostic"; exit 1; }
check_bash_version 4 0
check_bash_version 5 3
if check_bash_version x y >/tmp/v061-bash-version.out 2>&1; then
  echo "malformed Bash version was accepted"; exit 1
fi
rm -f /tmp/v061-bash-version.out
[ "$BASH_BIN" = "${BASH:-$(command -v bash)}" ] || { echo "selected Bash interpreter was not retained"; exit 1; }
grep -q 'require_modern_bash' "$BOOTSTRAP" || { echo "bootstrap lacks Bash gate"; exit 1; }
grep -q 'require_modern_bash' "$ROOT_DIR/hack/env-doctor.sh" || { echo "env-doctor lacks Bash gate"; exit 1; }
grep -q 'require_modern_bash' "$ROOT_DIR/hack/test-env.sh" || { echo "test-env lacks Bash gate"; exit 1; }
grep -q '"\$BASH_BIN".*install-crds.sh' "$ROOT_DIR/hack/test-env.sh" || { echo "test-env does not propagate selected Bash"; exit 1; }
grep -q '"\$BASH_BIN".*install-gadget.sh' "$ROOT_DIR/hack/test-env.sh" || { echo "test-env does not propagate selected Bash to Gadget"; exit 1; }
if output="$(bash "$BOOTSTRAP" --plan 2>&1)"; then
  echo "bootstrap proceeded with unsupported host Bash"; exit 1
fi
grep -q "Bash ${BASH_MIN_VERSION} or newer is required" <<<"$output" || { echo "bootstrap did not report its Bash prerequisite"; exit 1; }
if output="$(bash "$ROOT_DIR/hack/env-doctor.sh" 2>&1)"; then
  echo "env-doctor proceeded with unsupported host Bash"; exit 1
fi
grep -q "Bash ${BASH_MIN_VERSION} or newer is required" <<<"$output" || { echo "env-doctor did not report its Bash prerequisite"; exit 1; }
if output="$(bash "$ROOT_DIR/hack/test-env.sh" 2>&1)"; then
  echo "test-env proceeded with unsupported host Bash"; exit 1
fi
grep -q "Bash ${BASH_MIN_VERSION} or newer is required" <<<"$output" || { echo "test-env did not report its Bash prerequisite"; exit 1; }
! grep -q 'mapfile: command not found' <<<"$output" || { echo "test-env reached a late mapfile failure"; exit 1; }
echo "Bash prerequisite and interpreter propagation PASS"

# The remaining checks execute the bootstrap under the host interpreter. On
# macOS systems that expose only the unsupported system Bash 3.2, the
# prerequisite checks above are still runnable; defer the integration checks
# to a supported Bash rather than pretending the platform plan was exercised.
if ! require_modern_bash >/dev/null 2>&1; then
  echo "Bash 4.0+ unavailable; integration bootstrap checks deferred"
  exit 0
fi

for pair in 'Darwin arm64' 'Darwin x86_64' 'Linux arm64' 'Linux x86_64'; do
  read -r test_os test_arch <<< "$pair"
  output="$(BOOTSTRAP_OS="$test_os" BOOTSTRAP_ARCH="$test_arch" bash "$BOOTSTRAP" --plan)"
  grep -q PLATFORM_PLAN <<<"$output"
done
if BOOTSTRAP_OS=Plan9 BOOTSTRAP_ARCH=amd64 bash "$BOOTSTRAP" --plan >/dev/null 2>&1; then echo "unsupported OS accepted"; exit 1; fi
if BOOTSTRAP_OS=Linux BOOTSTRAP_ARCH=mips64 bash "$BOOTSTRAP" --plan >/dev/null 2>&1; then echo "unsupported arch accepted"; exit 1; fi
grep -q 'make test-env' "$BOOTSTRAP"; ! grep -q 'install-k3s\|k3s' "$BOOTSTRAP"
grep -q 'bootstrap.sh.*--lane core' "$ROOT_DIR/hack/init-vm.sh"
grep -q 'source.*hack/versions.env' "$ROOT_DIR/test/e2e/install-gadget.sh"
grep -q 'source.*hack/versions.env' "$ROOT_DIR/.github/workflows/core-e2e.yml"
grep -q 'checksum' "$ROOT_DIR/.github/workflows/core-e2e.yml"
grep -q 'API_READY' "$ROOT_DIR/hack/bootstrap/readiness.sh"
grep -q 'CILIUM_READY' "$ROOT_DIR/hack/bootstrap/readiness.sh"
grep -q 'COREDNS_READY' "$ROOT_DIR/hack/bootstrap/readiness.sh"
grep -q 'elapsed=' "$ROOT_DIR/hack/bootstrap/readiness.sh"
grep -q 'template://docker-rootful' "$BOOTSTRAP"
grep -q 'assert_rootful_runtime' "$BOOTSTRAP"
! grep -q 'template://docker"' "$BOOTSTRAP"
! grep -qE 'kind delete|limactl delete|helm .*(uninstall|delete)|delete (ns|namespace) gadget' "$ROOT_DIR/hack/test-env-clean.sh"
echo "v0.6.1 bootstrap static checks PASS"

# Runtime mode guard: rootless Docker is rejected, rootful Docker accepted.
RUNTIME_BIN="$(mktemp -d)"
cat > "$RUNTIME_BIN/docker" <<'EOF'
#!/usr/bin/env bash
if [ "${DOCKER_SECURITY_MODE:-rootful}" = rootless ]; then
  printf '["name=seccomp,profile=default","rootless"]\n'
else
  printf '["name=seccomp,profile=default"]\n'
fi
EOF
chmod +x "$RUNTIME_BIN/docker"
if ( PATH="$RUNTIME_BIN:$PATH" DOCKER_SECURITY_MODE=rootless BOOTSTRAP_OS=Darwin BOOTSTRAP_ARCH=arm64 bash -c 'source "$BOOTSTRAP"; assert_rootful_runtime' ) >/dev/null 2>&1; then
  echo "runtime guard accepted rootless Docker"; exit 1
fi
if ! ( PATH="$RUNTIME_BIN:$PATH" DOCKER_SECURITY_MODE=rootful BOOTSTRAP_OS=Darwin BOOTSTRAP_ARCH=arm64 bash -c 'source "$BOOTSTRAP"; assert_rootful_runtime' ) >/dev/null 2>&1; then
  echo "runtime guard rejected rootful Docker"; exit 1
fi
! grep -q 'desktop-linux' "$BOOTSTRAP"
rm -rf "$RUNTIME_BIN"
echo "rootful runtime guard PASS"

# Readiness sequencing: a registered but NotReady node cannot block Cilium.
READINESS_LOG="$(mktemp)"
if ! ( source "$ROOT_DIR/hack/bootstrap/readiness.sh"
  api_ready() { echo API_READY >> "$READINESS_LOG"; return 0; }
  node_registered() { echo NODE_REGISTERED >> "$READINESS_LOG"; return 0; }
  cilium_ready() { echo CILIUM_READY >> "$READINESS_LOG"; return 0; }
  kind_node_ready() { echo NODE_READY >> "$READINESS_LOG"; return 0; }
  coredns_ready() { echo COREDNS_READY >> "$READINESS_LOG"; return 0; }
  core_system_ready() { echo CORE_REQUIRED_SYSTEM_READY >> "$READINESS_LOG"; return 0; }
  wait_core_readiness 1 >/dev/null
); then
  echo "readiness sequence failed"; exit 1
fi
actual="$(tr '\n' ' ' < "$READINESS_LOG")"
[ "$actual" = 'API_READY NODE_REGISTERED CILIUM_READY NODE_READY COREDNS_READY CORE_REQUIRED_SYSTEM_READY ' ] || { echo "unexpected readiness order: $actual"; exit 1; }
rm -f "$READINESS_LOG"
grep -q 'wait_readiness NODE_REGISTERED' "$ROOT_DIR/hack/bootstrap/readiness.sh"
grep -q 'wait_readiness NODE_READY' "$ROOT_DIR/hack/bootstrap/readiness.sh"
echo "readiness dependency order PASS"

# Cilium readiness predicate: validate one structured record without shell
# integer parsing, including absent and malformed status values.
CILIUM_BIN="$(mktemp -d)"
cat > "$CILIUM_BIN/kubectl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${CILIUM_STATUS:-}"
EOF
chmod +x "$CILIUM_BIN/kubectl"
cilium_case() {
  local status="$1" expected="$2" output
  if output="$(PATH="$CILIUM_BIN:$PATH" CILIUM_STATUS="$status" bash -c 'source "$ROOT_DIR/hack/bootstrap/readiness.sh"; cilium_ready' 2>&1)"; then
    [ "$expected" = ready ] || { echo "unexpected Cilium readiness success for '$status'"; exit 1; }
  else
    [ "$expected" = not-ready ] || { echo "unexpected Cilium readiness failure for '$status': $output"; exit 1; }
  fi
  ! grep -qi 'integer expression expected' <<< "$output" || { echo "integer diagnostic for '$status'"; exit 1; }
}
cilium_case '1 1 1' ready
cilium_case '1 0 0' not-ready
cilium_case '0 0 0' not-ready
cilium_case '' not-ready
cilium_case 'x 1 1' not-ready
cilium_case '10 10 10' ready
cilium_case '10 9 9' not-ready
rm -rf "$CILIUM_BIN"
echo "Cilium readiness predicate PASS"

# CoreDNS readiness predicate: every selected pod must expose Ready=True.
COREDNS_BIN="$(mktemp -d)"
cat > "$COREDNS_BIN/kubectl" <<'EOF'
#!/usr/bin/env bash
if [ "${COREDNS_KUBECTL_FAIL:-0}" = 1 ]; then exit 1; fi
printf '%s\n' "${COREDNS_STATUS:-}"
EOF
chmod +x "$COREDNS_BIN/kubectl"
coredns_case() {
  local status="$1" expected="$2" output
  if output="$(PATH="$COREDNS_BIN:$PATH" COREDNS_STATUS="$status" bash -c 'source "$ROOT_DIR/hack/bootstrap/readiness.sh"; coredns_ready' 2>&1)"; then
    [ "$expected" = ready ] || { echo "unexpected CoreDNS readiness success for '$status'"; exit 1; }
  else
    [ "$expected" = not-ready ] || { echo "unexpected CoreDNS readiness failure for '$status': $output"; exit 1; }
  fi
  ! grep -qiE 'missing \]|integer expression expected|syntax error' <<< "$output" || { echo "CoreDNS diagnostic for '$status': $output"; exit 1; }
}
coredns_case 'coredns-a|True
coredns-b|True' ready
coredns_case 'coredns-a|True
coredns-b|False' not-ready
coredns_case 'coredns-a|False
coredns-b|False' not-ready
coredns_case 'coredns-a|True' ready
coredns_case '' not-ready
coredns_case 'coredns-a|' not-ready
coredns_case 'coredns-a|Ready' not-ready
rm -rf "$COREDNS_BIN"
echo "CoreDNS readiness predicate PASS"

# ============================================================
# M1 — pinned kubectl version convergence (behavioral, not grep)
# ============================================================
KUBECTL_PINNED="$(awk -F= '/^KUBECTL_VERSION=/ {print $2}' "$ROOT_DIR/hack/versions.env")"
M1_BIN="$(mktemp -d)"

cat > "$M1_BIN/kubectl" <<EOF
#!/usr/bin/env bash
printf '{\n  "clientVersion": {\n    "gitVersion": "%s"\n  }\n}\n' "${KUBECTL_PINNED}"
EOF
chmod +x "$M1_BIN/kubectl"
if ! ( PATH="$M1_BIN:$PATH" bash -c 'source "$BOOTSTRAP"; install_kubectl' ) >/dev/null 2>&1; then
  echo "M1: expected kubectl version (${KUBECTL_PINNED}) was rejected"; exit 1
fi

cat > "$M1_BIN/kubectl" <<'EOF'
#!/usr/bin/env bash
printf '{\n  "clientVersion": {\n    "gitVersion": "v1.30.0"\n  }\n}\n'
EOF
if ( PATH="$M1_BIN:$PATH" bash -c 'source "$BOOTSTRAP"; install_kubectl' ) >/dev/null 2>&1; then
  echo "M1: mismatched kubectl version (v1.30.0 vs ${KUBECTL_PINNED}) was accepted"; exit 1
fi
rm -rf "$M1_BIN"
echo "M1 kubectl version convergence PASS"

# ============================================================
# M2 — test-env target cluster safety (behavioral, logging kubectl stub)
# ============================================================
M2_BIN="$(mktemp -d)"
M2_LOG="$(mktemp)"
M2_STATE="$(mktemp -d)"
cat > "$M2_BIN/kubectl" <<EOF
#!/usr/bin/env bash
echo "\$@" >> "$M2_LOG"
case "\$1 \$2" in
  "cluster-info"*) exit 0 ;;
  "config current-context") echo "\${M2_CONTEXT:-kind-landlock-genprof-core}"; exit 0 ;;
esac
exit 1
EOF
chmod +x "$M2_BIN/kubectl"

# same-name context, no ownership record on this machine -> refused
: > "$M2_LOG"
if out="$(PATH="$M2_BIN:$PATH" XDG_STATE_HOME="$M2_STATE" bash "$ROOT_DIR/hack/test-env.sh" 2>&1)"; then
  echo "M2: same-name cluster without an ownership record was accepted"; exit 1
fi
grep -q 'no bootstrap ownership record' <<<"$out" || { echo "M2: expected ownership-record refusal, got: $out"; exit 1; }
grep -q '^apply' "$M2_LOG" && { echo "M2: mutation was attempted before the ownership gate refused"; exit 1; }

# arbitrary/unrelated current context -> refused before any apply
: > "$M2_LOG"
if out="$(PATH="$M2_BIN:$PATH" XDG_STATE_HOME="$M2_STATE" M2_CONTEXT=kind-unrelated-cluster bash "$ROOT_DIR/hack/test-env.sh" 2>&1)"; then
  echo "M2: an arbitrary reachable current context was accepted"; exit 1
fi
grep -q 'is not the bootstrap-owned Core context' <<<"$out" || { echo "M2: expected context-mismatch refusal, got: $out"; exit 1; }
grep -q '^apply' "$M2_LOG" && { echo "M2: mutation was attempted before the context gate refused"; exit 1; }

# owned intended Core target -> passes the gate (no refusal message emitted)
mkdir -p "$M2_STATE/landlock-genprof"
printf '{"cluster":"landlock-genprof-core","topology":"kind+cilium","owner":"landlock-genprof","createdBy":"hack/bootstrap.sh"}\n' \
  > "$M2_STATE/landlock-genprof/landlock-genprof-core.json"
out="$(PATH="$M2_BIN:$PATH" XDG_STATE_HOME="$M2_STATE" bash "$ROOT_DIR/hack/test-env.sh" 2>&1 || true)"
if grep -qE 'is not the bootstrap-owned Core context|no bootstrap ownership record|invalid ownership record' <<<"$out"; then
  echo "M2: an owned Core target was incorrectly refused: $out"; exit 1
fi
rm -rf "$M2_BIN" "$M2_LOG" "$M2_STATE"
echo "M2 test-env target cluster safety PASS"

# ============================================================
# M3 — test-env-clean ownership/cleanup symmetry (behavioral)
# ============================================================
M3_BIN="$(mktemp -d)"
M3_LOG="$(mktemp)"
M3_STATE="$(mktemp -d)"
cat > "$M3_BIN/kubectl" <<EOF
#!/usr/bin/env bash
echo "\$@" >> "$M3_LOG"
exit 0
EOF
chmod +x "$M3_BIN/kubectl"

# A: unowned environment -> refused, zero kubectl invocations (no destructive action)
: > "$M3_LOG"
if PATH="$M3_BIN:$PATH" XDG_STATE_HOME="$M3_STATE" bash "$ROOT_DIR/hack/test-env-clean.sh" >/dev/null 2>&1; then
  echo "M3: unowned environment cleanup was not refused"; exit 1
fi
[ -s "$M3_LOG" ] && { echo "M3: unowned environment cleanup invoked kubectl: $(cat "$M3_LOG")"; exit 1; }

# B/F: owned Core environment -> the resources test-env.sh actually installs are
# targeted; Cilium/Gadget (shared dependencies) are never mentioned.
mkdir -p "$M3_STATE/landlock-genprof"
printf '{"cluster":"landlock-genprof-core","topology":"kind+cilium","owner":"landlock-genprof","createdBy":"hack/bootstrap.sh"}\n' \
  > "$M3_STATE/landlock-genprof/landlock-genprof-core.json"
: > "$M3_LOG"
PATH="$M3_BIN:$PATH" XDG_STATE_HOME="$M3_STATE" bash "$ROOT_DIR/hack/test-env-clean.sh" >/dev/null
for crd in traininghistories.landlockgenprof.io securityprofileproposals.landlockgenprof.io applyattempts.landlockgenprof.io rollbackattempts.landlockgenprof.io; do
  grep -q "delete crd ${crd}" "$M3_LOG" || { echo "M3: owned cleanup did not target CRD ${crd}"; exit 1; }
done
grep -qi 'gadget' "$M3_LOG" && { echo "M3: cleanup touched shared Gadget resources: $(cat "$M3_LOG")"; exit 1; }
grep -qi 'cilium' "$M3_LOG" && { echo "M3: cleanup touched shared Cilium resources: $(cat "$M3_LOG")"; exit 1; }
rm -rf "$M3_BIN" "$M3_LOG" "$M3_STATE"
echo "M3 cleanup ownership/symmetry PASS"

# ============================================================
# M4 — readiness timeout budget exceeds the cited ~31-minute regression
# ============================================================
if ! ( bash -c 'source "$BOOTSTRAP"; [ "$READINESS_TIMEOUT" -ge 1860 ]' ); then
  echo "M4: default readiness timeout does not comfortably exceed 1860s (31 minutes)"; exit 1
fi
if ! ( LANDLOCK_CORE_READINESS_TIMEOUT=1861 bash -c 'source "$BOOTSTRAP"; [ "$READINESS_TIMEOUT" -eq 1861 ]' ); then
  echo "M4: LANDLOCK_CORE_READINESS_TIMEOUT override was not honored"; exit 1
fi
echo "M4 readiness timeout budget PASS"
