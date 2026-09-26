#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

bash -n "$ROOT_DIR/hack/dev-env.sh"

if VERSION=HEAD bash "$ROOT_DIR/hack/dev-env.sh" status >/tmp/landlock-genprof-dev-env-test.out 2>&1; then
  echo "mutable HEAD selector was accepted" >&2
  exit 1
fi
grep -F "must be an immutable tag or commit" /tmp/landlock-genprof-dev-env-test.out >/dev/null

if bash "$ROOT_DIR/hack/dev-env.sh" status >/tmp/landlock-genprof-dev-env-test.out 2>&1; then
  echo "missing VERSION was accepted" >&2
  exit 1
fi
grep -F "VERSION is required" /tmp/landlock-genprof-dev-env-test.out >/dev/null

if bash "$ROOT_DIR/hack/dev-env.sh" unsupported >/tmp/landlock-genprof-dev-env-test.out 2>&1; then
  echo "unknown command was accepted" >&2
  exit 1
fi
grep -F "unknown command" /tmp/landlock-genprof-dev-env-test.out >/dev/null

grep -F 'KUBECONFIG="$KUBECONFIG_PATH"' "$ROOT_DIR/hack/dev-env.sh" >/dev/null
grep -F 'verify_ownership' "$ROOT_DIR/hack/dev-env.sh" >/dev/null
grep -F 'DEV_INSTALL_SPO' "$ROOT_DIR/hack/dev-env.sh" >/dev/null
grep -F 'DEV_INSTALL_PODLOCK' "$ROOT_DIR/hack/dev-env.sh" >/dev/null
grep -F 'TARGET_INSTANCE' "$ROOT_DIR/hack/dev-env.sh" >/dev/null

rm -f /tmp/landlock-genprof-dev-env-test.out

# Exercise state cleanup without Docker or Kubernetes. The fixture uses the
# real command with stubs that report no cluster, so this covers interrupted
# cleanup recovery and fail-closed custody checks without touching a cluster.
FIXTURE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/landlock-genprof-dev-cleanup.XXXXXX")"
STUB_BIN="$FIXTURE_ROOT/bin"
TEST_SHA="$(git -C "$ROOT_DIR" rev-parse HEAD)"
TEST_STATE_ROOT="$FIXTURE_ROOT/state/landlock-genprof/dev/$TEST_SHA"
TEST_CP_ID="0000000000000000000000000000000000000000000000000000000000000000"
TEST_DAEMON_ID="fixture-daemon"
trap 'chmod -R u+rw "$FIXTURE_ROOT" 2>/dev/null || true; rm -rf "$FIXTURE_ROOT"' EXIT
mkdir -p "$STUB_BIN"
cat > "$STUB_BIN/docker" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  context)
    case "${2:-}" in
      show) echo "${DOCKER_TEST_CONTEXT:-lima-landlock-genprof-core}" ;;
      inspect) echo unix:///tmp/landlock-genprof-test.sock ;;
    esac
    ;;
  info)
    case " $* " in
      *"{{.ID}}"*) echo "${DOCKER_TEST_DAEMON_ID:-fixture-daemon}" ;;
      *" --format "*) echo '[]' ;;
    esac
    ;;
  ps)
    printf '%s\n' "${DOCKER_TEST_OUTPUT:-}"
    printf '%s\n' "$@" > "${DOCKER_TEST_ARGS_FILE:-/dev/null}"
    ;;
  inspect)
    printf '%s\n' "${DOCKER_TEST_INSPECT:-}"
    ;;
esac
EOF
cat > "$STUB_BIN/kind" <<'EOF'
#!/usr/bin/env bash
if [ "${1:-}" = get ] && [ "${2:-}" = clusters ]; then exit 0; fi
exit 1
EOF
cat > "$STUB_BIN/limactl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$STUB_BIN/sysctl" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *hw.ncpu*) echo 4 ;;
  *hw.memsize*) echo 6442450944 ;;
  *) exit 1 ;;
esac
EOF
chmod 755 "$STUB_BIN/docker" "$STUB_BIN/kind" "$STUB_BIN/limactl" "$STUB_BIN/sysctl"

make_fixture() {
  local mode="${1:-writable}"
  local instance="${2:-}"
  if [ -n "$instance" ]; then
    TEST_STATE_ROOT="$FIXTURE_ROOT/state/landlock-genprof/dev/$TEST_SHA/$instance"
  else
    TEST_STATE_ROOT="$FIXTURE_ROOT/state/landlock-genprof/dev/$TEST_SHA"
  fi
  rm -rf "$TEST_STATE_ROOT"
  mkdir -p "$TEST_STATE_ROOT/source/hack" "$TEST_STATE_ROOT/gopath/pkg/mod/read-only"
  cp "$ROOT_DIR/go.mod" "$TEST_STATE_ROOT/source/go.mod"
  cp "$ROOT_DIR/hack/versions.env" "$TEST_STATE_ROOT/source/hack/versions.env"
  cp "$ROOT_DIR/hack/bootstrap.sh" "$TEST_STATE_ROOT/source/hack/bootstrap.sh"
  printf '%s\n' "$TEST_SHA" > "$TEST_STATE_ROOT/source/.landlock-genprof-source-sha"
  local cluster="landlock-genprof-dev-${TEST_SHA:0:12}"
  if [ -n "$instance" ]; then cluster="${cluster}-${instance}"; fi
  printf '{"cluster":"%s","context":"kind-%s","instance":"%s","owner":"landlock-genprof","sourceSHA":"%s","dockerContext":"lima-landlock-genprof-core","dockerDaemonID":"%s","controlPlaneID":"%s","createdBy":"hack/dev-env.sh"}\n' \
    "$cluster" "$cluster" "$instance" "$TEST_SHA" "$TEST_DAEMON_ID" "$TEST_CP_ID" > "$TEST_STATE_ROOT/ownership.json"
  printf 'generated module cache\n' > "$TEST_STATE_ROOT/gopath/pkg/mod/read-only/file.go"
  if [ "$mode" = readonly ]; then
    chmod 555 "$TEST_STATE_ROOT/gopath/pkg/mod/read-only"
    chmod 444 "$TEST_STATE_ROOT/gopath/pkg/mod/read-only/file.go"
  fi
}

run_down_fixture() {
  local instance="${1:-}"
  VERSION="$TEST_SHA" INSTANCE="$instance" XDG_STATE_HOME="$FIXTURE_ROOT/state" \
    DOCKER_TEST_DAEMON_ID="${DOCKER_TEST_DAEMON_ID:-$TEST_DAEMON_ID}" PATH="$STUB_BIN:$PATH" \
    bash "$ROOT_DIR/hack/dev-env.sh" down
}

run_resolve_fixture() {
  VERSION="$TEST_SHA" INSTANCE="$1" XDG_STATE_HOME="$FIXTURE_ROOT/state" bash -c \
    'source "$1"; resolve_source; printf "%s\n%s\n%s\n" "$CLUSTER_NAME" "$STATE_ROOT" "$EXPECTED_CONTEXT"' \
    bash "$ROOT_DIR/hack/dev-env.sh"
}

# The omitted instance preserves the original names and state path; distinct
# valid instances derive distinct names and state roots for the same SHA.
legacy_resolution="$(run_resolve_fixture '')"
printf '%s\n' "$legacy_resolution" | grep -Fx "landlock-genprof-dev-${TEST_SHA:0:12}" >/dev/null
printf '%s\n' "$legacy_resolution" | grep -Fx "$FIXTURE_ROOT/state/landlock-genprof/dev/$TEST_SHA" >/dev/null
alpha_resolution="$(run_resolve_fixture alpha)"
beta_resolution="$(run_resolve_fixture beta)"
[ "$alpha_resolution" != "$beta_resolution" ]
printf '%s\n' "$alpha_resolution" | grep -Fx "landlock-genprof-dev-${TEST_SHA:0:12}-alpha" >/dev/null
printf '%s\n' "$beta_resolution" | grep -Fx "landlock-genprof-dev-${TEST_SHA:0:12}-beta" >/dev/null
printf '%s\n' "$alpha_resolution" | grep -Fx "$FIXTURE_ROOT/state/landlock-genprof/dev/$TEST_SHA/alpha" >/dev/null
printf '%s\n' "$beta_resolution" | grep -Fx "$FIXTURE_ROOT/state/landlock-genprof/dev/$TEST_SHA/beta" >/dev/null

for invalid_instance in A bad_value '-leading' 'trailing-' "$(printf 'x%.0s' {1..25})"; do
  if run_resolve_fixture "$invalid_instance" >/tmp/landlock-genprof-dev-instance.out 2>&1; then
    echo "invalid instance was accepted: $invalid_instance" >&2
    exit 1
  fi
done
rm -f /tmp/landlock-genprof-dev-instance.out

run_control_plane_fixture() {
  DOCKER_TEST_OUTPUT="$1" DOCKER_TEST_INSPECT="${2:-}" \
    DOCKER_TEST_ARGS_FILE="$FIXTURE_ROOT/docker-args" PATH="$STUB_BIN:$PATH" \
    bash -c 'source "$1"; CLUSTER_NAME=landlock-genprof-dev-test; control_plane_id' \
    bash "$ROOT_DIR/hack/dev-env.sh"
}

# Normal stale-state cleanup.
make_fixture writable
run_down_fixture | grep -Fx 'DEV_DOWN=STALE_STATE_CLEANED' >/dev/null
[ ! -e "$TEST_STATE_ROOT" ]

# Recovery after the cluster deletion phase was interrupted, including the
# read-only module-cache permissions that caused the qualification failure.
make_fixture readonly
run_down_fixture | grep -Fx 'DEV_DOWN=STALE_STATE_CLEANED' >/dev/null
[ ! -e "$TEST_STATE_ROOT" ]

# A mismatched ownership record must prevent deletion.
make_fixture writable
sed -i.bak "s/sourceSHA\\\":\\\"$TEST_SHA/sourceSHA\\\":\\\"$(printf 'f%.0s' {1..40})/" "$TEST_STATE_ROOT/ownership.json"
rm -f "$TEST_STATE_ROOT/ownership.json.bak"
if run_down_fixture >/tmp/landlock-genprof-dev-cleanup.out 2>&1; then
  echo "mismatched ownership was accepted" >&2
  exit 1
fi
[ -e "$TEST_STATE_ROOT" ]
grep -F "ownership record does not exactly match" /tmp/landlock-genprof-dev-cleanup.out >/dev/null

# A Docker daemon change must also fail closed.
make_fixture writable
if DOCKER_TEST_DAEMON_ID=other-daemon run_down_fixture >/tmp/landlock-genprof-dev-cleanup.out 2>&1; then
  echo "Docker daemon mismatch was accepted" >&2
  exit 1
fi
[ -e "$TEST_STATE_ROOT" ]
grep -F "ownership record does not exactly match" /tmp/landlock-genprof-dev-cleanup.out >/dev/null

# Symlinks inside the tree must fail closed and leave the sentinel intact.
make_fixture writable
printf 'protected sentinel\n' > "$FIXTURE_ROOT/sentinel"
ln -s "$FIXTURE_ROOT/sentinel" "$TEST_STATE_ROOT/unsafe-link"
if run_down_fixture >/tmp/landlock-genprof-dev-cleanup.out 2>&1; then
  echo "symlinked state was accepted" >&2
  exit 1
fi
[ -e "$TEST_STATE_ROOT" ]
[ "$(cat "$FIXTURE_ROOT/sentinel")" = "protected sentinel" ]
grep -F "unsafe entry in state directory" /tmp/landlock-genprof-dev-cleanup.out >/dev/null
rm -f /tmp/landlock-genprof-dev-cleanup.out

# Cleanup of one instance must not remove another instance's state.
make_fixture writable alpha
ALPHA_STATE_ROOT="$TEST_STATE_ROOT"
make_fixture writable beta
BETA_STATE_ROOT="$TEST_STATE_ROOT"
run_down_fixture alpha | grep -Fx 'DEV_DOWN=STALE_STATE_CLEANED' >/dev/null
[ ! -e "$ALPHA_STATE_ROOT" ]
[ -e "$BETA_STATE_ROOT" ]
run_down_fixture beta | grep -Fx 'DEV_DOWN=STALE_STATE_CLEANED' >/dev/null
[ ! -e "$BETA_STATE_ROOT" ]

# Control-plane lookup must use formatted output without Docker's quiet mode,
# require one exact Kind control-plane name, and verify the full identity.
MATCH_ID="$(printf 'a%.0s' {1..64})"
SECOND_ID="$(printf 'b%.0s' {1..64})"
MATCH_NAME="landlock-genprof-dev-test-control-plane"
result="$(run_control_plane_fixture "$MATCH_ID $MATCH_NAME" "$MATCH_ID")"
[ "$result" = "$MATCH_ID" ]
grep -Fx -- '--no-trunc' "$FIXTURE_ROOT/docker-args" >/dev/null
if grep -Fx -- '-q' "$FIXTURE_ROOT/docker-args" >/dev/null; then
  echo "control-plane lookup used Docker quiet mode" >&2
  exit 1
fi

if run_control_plane_fixture "" "$MATCH_ID" >/tmp/landlock-genprof-dev-control-plane.out 2>&1; then
  echo "missing control-plane container was accepted" >&2
  exit 1
fi
grep -F "found 0" /tmp/landlock-genprof-dev-control-plane.out >/dev/null

MULTI_OUTPUT="$MATCH_ID $MATCH_NAME"$'\n'"$SECOND_ID $MATCH_NAME"
if run_control_plane_fixture "$MULTI_OUTPUT" "$MATCH_ID" \
  >/tmp/landlock-genprof-dev-control-plane.out 2>&1; then
  echo "multiple control-plane containers were accepted" >&2
  exit 1
fi
grep -F "found 2" /tmp/landlock-genprof-dev-control-plane.out >/dev/null

if run_control_plane_fixture "$MATCH_ID ${MATCH_NAME}-extra" "$MATCH_ID" \
  >/tmp/landlock-genprof-dev-control-plane.out 2>&1; then
  echo "similar control-plane name was accepted" >&2
  exit 1
fi
grep -F "found 0" /tmp/landlock-genprof-dev-control-plane.out >/dev/null

if run_control_plane_fixture "not-a-container-id $MATCH_NAME" "$MATCH_ID" \
  >/tmp/landlock-genprof-dev-control-plane.out 2>&1; then
  echo "unexpected Docker ID output was accepted" >&2
  exit 1
fi
grep -F "unexpected Docker container ID output" /tmp/landlock-genprof-dev-control-plane.out >/dev/null

if run_control_plane_fixture "$MATCH_ID $MATCH_NAME extra" "$MATCH_ID" \
  >/tmp/landlock-genprof-dev-control-plane.out 2>&1; then
  echo "unexpected Docker row output was accepted" >&2
  exit 1
fi
grep -F "unexpected Docker container output" /tmp/landlock-genprof-dev-control-plane.out >/dev/null

# The runtime guard must reject a Docker context other than the documented
# macOS/Lima daemon before any cluster operation is attempted.
if [ "$(uname -s)" = Darwin ]; then
  if VERSION="$TEST_SHA" DOCKER_TEST_CONTEXT=unexpected XDG_STATE_HOME="$FIXTURE_ROOT/context-state" \
    PATH="$STUB_BIN:$PATH" bash "$ROOT_DIR/hack/dev-env.sh" doctor \
    >/tmp/landlock-genprof-dev-control-plane.out 2>&1; then
    echo "Docker context mismatch was accepted" >&2
    exit 1
  fi
  grep -F "macOS development requires Docker context" \
    /tmp/landlock-genprof-dev-control-plane.out >/dev/null
fi
rm -f /tmp/landlock-genprof-dev-control-plane.out

# Versioned doctor must resolve Git metadata without creating persistent state,
# and repeated runs must remain non-mutating.
DOCTOR_STATE_HOME="$FIXTURE_ROOT/doctor-state"
DOCTOR_STATE_ROOT="$DOCTOR_STATE_HOME/landlock-genprof/dev/$TEST_SHA/alpha"
for _ in 1 2; do
  VERSION="$TEST_SHA" INSTANCE=alpha XDG_STATE_HOME="$DOCTOR_STATE_HOME" \
    PATH="$STUB_BIN:$PATH" bash "$ROOT_DIR/hack/dev-env.sh" doctor >/dev/null
done
[ ! -e "$DOCTOR_STATE_ROOT" ]

# dev-test uses an ephemeral SHA-scoped state root so containment validation
# remains active without creating persistent contributor state.
DEV_TEST_BIN="$FIXTURE_ROOT/dev-test-bin"
mkdir -p "$DEV_TEST_BIN"
FAKE_GO_VERSION="$(awk '$1 == "toolchain" { print $2; exit }' "$ROOT_DIR/go.mod")"
FAKE_GO_VERSION="${FAKE_GO_VERSION#go}"
cat > "$DEV_TEST_BIN/go" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = version ]; then
  echo "go version go${FAKE_GO_VERSION} fixture/amd64"
  exit 0
fi
exit 1
EOF
cat > "$DEV_TEST_BIN/make" <<'EOF'
#!/usr/bin/env bash
[ "${1:-}" = -C ] && [ -d "${2:-}" ] && [ "${3:-}" = test-unit ]
EOF
chmod 755 "$DEV_TEST_BIN/go" "$DEV_TEST_BIN/make"
DEV_TEST_STATE_HOME="$FIXTURE_ROOT/dev-test-state"
VERSION="$TEST_SHA" INSTANCE=test XDG_STATE_HOME="$DEV_TEST_STATE_HOME" \
  PATH="$DEV_TEST_BIN:$STUB_BIN:$PATH" bash "$ROOT_DIR/hack/dev-env.sh" test
[ ! -e "$DEV_TEST_STATE_HOME" ]

# Git archives may contain contained symlinks. They are valid source content,
# while links outside the source snapshot and dangling links remain rejected.
SNAPSHOT_STATE_HOME="$FIXTURE_ROOT/snapshot-state"
SNAPSHOT_STATE_ROOT="$SNAPSHOT_STATE_HOME/landlock-genprof/dev/$TEST_SHA/links"
VERSION="$TEST_SHA" INSTANCE=links XDG_STATE_HOME="$SNAPSHOT_STATE_HOME" \
  PATH="$STUB_BIN:$PATH" bash -c \
  'source "$1"; resolve_source; materialize_source; validate_state_root' \
  bash "$ROOT_DIR/hack/dev-env.sh"
[ "$(find -P "$SNAPSHOT_STATE_ROOT/source" -type l | wc -l | tr -d ' ')" -eq 3 ]

printf 'protected sentinel\n' > "$FIXTURE_ROOT/sentinel"
ln -s "$FIXTURE_ROOT/sentinel" "$SNAPSHOT_STATE_ROOT/source/escape-link"
if VERSION="$TEST_SHA" INSTANCE=links XDG_STATE_HOME="$SNAPSHOT_STATE_HOME" \
  PATH="$STUB_BIN:$PATH" bash -c \
  'source "$1"; resolve_source; validate_state_root' \
  bash "$ROOT_DIR/hack/dev-env.sh" >/tmp/landlock-genprof-dev-symlink.out 2>&1; then
  echo "escaping source symlink was accepted" >&2
  exit 1
fi
grep -F "symlink escapes immutable source snapshot" /tmp/landlock-genprof-dev-symlink.out >/dev/null
rm -f "$SNAPSHOT_STATE_ROOT/source/escape-link"
ln -s missing-target "$SNAPSHOT_STATE_ROOT/source/dangling-link"
if VERSION="$TEST_SHA" INSTANCE=links XDG_STATE_HOME="$SNAPSHOT_STATE_HOME" \
  PATH="$STUB_BIN:$PATH" bash -c \
  'source "$1"; resolve_source; validate_state_root' \
  bash "$ROOT_DIR/hack/dev-env.sh" >/tmp/landlock-genprof-dev-symlink.out 2>&1; then
  echo "dangling source symlink was accepted" >&2
  exit 1
fi
grep -F "dangling or looping symlink" /tmp/landlock-genprof-dev-symlink.out >/dev/null
rm -f /tmp/landlock-genprof-dev-symlink.out

# A malformed ownership record is inspected but never adopted.
make_fixture writable alpha
sed -i.bak 's/"controlPlaneID":"[^"]*"/"controlPlaneID":""/' "$TEST_STATE_ROOT/ownership.json"
rm -f "$TEST_STATE_ROOT/ownership.json.bak"
if VERSION="$TEST_SHA" INSTANCE=alpha XDG_STATE_HOME="$FIXTURE_ROOT/state" \
  PATH="$STUB_BIN:$PATH" bash "$ROOT_DIR/hack/dev-env.sh" doctor \
  >/tmp/landlock-genprof-dev-incomplete.out 2>&1; then
  echo "incomplete ownership was accepted" >&2
  exit 1
fi
grep -F "ownership record has no unambiguous control-plane identity" \
  /tmp/landlock-genprof-dev-incomplete.out >/dev/null
rm -f /tmp/landlock-genprof-dev-incomplete.out

echo "dev environment safety tests: PASS"
