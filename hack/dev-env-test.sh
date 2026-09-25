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
echo "dev environment safety tests: PASS"
