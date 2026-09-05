#!/usr/bin/env bash
set -Eeuo pipefail

echo "WARNING: hack/init-vm.sh is deprecated; delegating to hack/bootstrap.sh --lane core" >&2
exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/bootstrap.sh" --lane core "$@"
