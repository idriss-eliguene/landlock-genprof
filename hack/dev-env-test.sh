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

rm -f /tmp/landlock-genprof-dev-env-test.out

# Exercise state cleanup without Docker or Kubernetes. The fixture uses the
# real command with stubs that report no cluster, so this covers interrupted
# cleanup recovery and fail-closed custody checks without touching a cluster.
FIXTURE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/landlock-genprof-dev-cleanup.XXXXXX")"
STUB_BIN="$FIXTURE_ROOT/bin"
TEST_SHA="$(git -C "$ROOT_DIR" rev-parse HEAD)"
TEST_STATE_ROOT="$FIXTURE_ROOT/state/landlock-genprof/dev/$TEST_SHA"
TEST_CP_ID="0000000000000000000000000000000000000000000000000000000000000000"
trap 'chmod -R u+rw "$FIXTURE_ROOT" 2>/dev/null || true; rm -rf "$FIXTURE_ROOT"' EXIT
mkdir -p "$STUB_BIN"
cat > "$STUB_BIN/docker" <<'EOF'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  "context show") echo lima-landlock-genprof-core ;;
  "context inspect") echo unix:///tmp/landlock-genprof-test.sock ;;
  "info")
    case " $* " in *" --format "*) echo '[]' ;; esac
    ;;
  "ps") ;;
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
chmod 755 "$STUB_BIN/docker" "$STUB_BIN/kind" "$STUB_BIN/limactl"

make_fixture() {
  local mode="${1:-writable}"
  rm -rf "$TEST_STATE_ROOT"
  mkdir -p "$TEST_STATE_ROOT/source/hack" "$TEST_STATE_ROOT/gopath/pkg/mod/read-only"
  cp "$ROOT_DIR/go.mod" "$TEST_STATE_ROOT/source/go.mod"
  cp "$ROOT_DIR/hack/versions.env" "$TEST_STATE_ROOT/source/hack/versions.env"
  cp "$ROOT_DIR/hack/bootstrap.sh" "$TEST_STATE_ROOT/source/hack/bootstrap.sh"
  printf '%s\n' "$TEST_SHA" > "$TEST_STATE_ROOT/source/.landlock-genprof-source-sha"
  printf '{"cluster":"landlock-genprof-dev-%s","context":"kind-landlock-genprof-dev-%s","owner":"landlock-genprof","sourceSHA":"%s","controlPlaneID":"%s","createdBy":"hack/dev-env.sh"}\n' \
    "${TEST_SHA:0:12}" "${TEST_SHA:0:12}" "$TEST_SHA" "$TEST_CP_ID" > "$TEST_STATE_ROOT/ownership.json"
  printf 'generated module cache\n' > "$TEST_STATE_ROOT/gopath/pkg/mod/read-only/file.go"
  if [ "$mode" = readonly ]; then
    chmod 555 "$TEST_STATE_ROOT/gopath/pkg/mod/read-only"
    chmod 444 "$TEST_STATE_ROOT/gopath/pkg/mod/read-only/file.go"
  fi
}

run_down_fixture() {
  VERSION="$TEST_SHA" XDG_STATE_HOME="$FIXTURE_ROOT/state" PATH="$STUB_BIN:$PATH" \
    bash "$ROOT_DIR/hack/dev-env.sh" down
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

echo "dev environment safety tests: PASS"
