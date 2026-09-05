#!/usr/bin/env bash
set -Eeuo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BOOTSTRAP="$ROOT_DIR/hack/bootstrap.sh"
for f in "$BOOTSTRAP" "$ROOT_DIR/hack/env-doctor.sh" "$ROOT_DIR/hack/test-env.sh" "$ROOT_DIR/hack/test-env-clean.sh" "$ROOT_DIR/hack/bootstrap/readiness.sh"; do test -x "$f" || { echo "not executable: $f"; exit 1; }; done
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
! grep -q 'kind delete\|limactl delete' "$ROOT_DIR/hack/test-env-clean.sh"
echo "v0.6.1 bootstrap static checks PASS"
