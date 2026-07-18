#!/usr/bin/env bash
# Vérifie que le kernel hôte supporte Landlock (FS + réseau) et eBPF,
# prérequis pour développer/tester landlock-genprof.
set -euo pipefail

echo "== Vérification du kernel =="
KERNEL_VERSION=$(uname -r)
echo "Kernel: ${KERNEL_VERSION}"

# Landlock FS support depuis 5.13, réseau depuis 6.4
MAJOR=$(echo "$KERNEL_VERSION" | cut -d. -f1)
MINOR=$(echo "$KERNEL_VERSION" | cut -d. -f2)

if [ "$MAJOR" -lt 5 ] || { [ "$MAJOR" -eq 5 ] && [ "$MINOR" -lt 13 ]; }; then
  echo "❌ Kernel trop ancien pour Landlock FS (besoin >= 5.13)"
  exit 1
fi
echo "✅ Landlock FS supporté"

if [ "$MAJOR" -gt 6 ] || { [ "$MAJOR" -eq 6 ] && [ "$MINOR" -ge 4 ]; }; then
  echo "✅ Landlock réseau supporté (>= 6.4)"
else
  echo "⚠️  Landlock réseau non supporté sur ce kernel (besoin >= 6.4) — FS uniquement"
fi

echo ""
echo "== Vérification Landlock actif =="
if dmesg 2>/dev/null | grep -qw landlock; then
  echo "✅ Landlock actif (trouvé dans dmesg)"
else
  echo "⚠️  Impossible de confirmer via dmesg (permissions ou buffer vidé) — non bloquant"
fi

echo ""
echo "== Vérification eBPF =="
if [ -d /sys/fs/bpf ]; then
  echo "✅ bpffs monté"
else
  echo "⚠️  /sys/fs/bpf absent — vérifier que bpffs est monté"
fi

echo ""
echo "Prérequis de base OK. Voir README.md pour la suite (cluster kind, Inspektor Gadget)."
