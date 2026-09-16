#!/usr/bin/env bash
# install-helm.sh — install the Helm version pinned by landlock-genprof.
#
# Idempotent: an already working Helm installation is reused.

set -euo pipefail

HELM_VERSION="${HELM_VERSION:-v4.2.3}"

if command -v helm >/dev/null 2>&1; then
    if helm version --short >/dev/null 2>&1; then
        echo "[ok] Helm already installed: $(helm version --short)"
        exit 0
    fi

    echo "ERROR: helm exists at $(command -v helm) but is not usable." >&2
    exit 1
fi

case "$(uname -m)" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo "ERROR: unsupported architecture for Helm: $(uname -m)" >&2
        exit 1
        ;;
esac

echo "[install] Helm ${HELM_VERSION} (${ARCH})"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

archive="helm-${HELM_VERSION}-linux-${ARCH}.tar.gz"
url="https://get.helm.sh/${archive}"

curl -fL --retry 3 \
    -o "${tmpdir}/${archive}" \
    "${url}"

tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}"

sudo install \
    -o root \
    -g root \
    -m 0755 \
    "${tmpdir}/linux-${ARCH}/helm" \
    /usr/local/bin/helm

if ! command -v helm >/dev/null 2>&1; then
    echo "ERROR: Helm installation completed but helm is not on PATH." >&2
    exit 1
fi

if ! helm version --short >/dev/null 2>&1; then
    echo "ERROR: Helm was installed but failed verification." >&2
    exit 1
fi

echo "[ok] Helm installed: $(helm version --short)"
