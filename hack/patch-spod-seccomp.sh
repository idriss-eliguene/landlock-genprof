#!/usr/bin/env bash
# Adds `clock_gettime` to security-profiles-operator's own seccomp
# allow-list, then restarts spod to pick it up.
#
# Root cause (confirmed live, VirtualBox/Ubuntu ARM64 guest -> Docker ->
# kind node, 2026-07-30): spod's main container applies a Localhost
# seccomp profile to *itself* (ConfigMap security-profiles-operator-
# profile, key security-profiles-operator.json — not the same thing as
# the spod CR's spec.allowedSyscalls, which only gates SeccompProfile
# objects *other* workloads submit, confirmed against SPO's own source,
# internal/pkg/daemon/seccompprofile/seccompprofile.go). That profile's
# default syscalls[0].names list is missing clock_gettime. In most
# environments this is harmless because glibc/Go serve clock_gettime via
# vDSO (no real syscall, seccomp never sees it) — but in nested
# virtualization (VirtualBox -> Docker -> kind, seen on ARM64 here) the
# vDSO fast path isn't reliably available, so the runtime falls back to
# the real syscall, which SCMP_ACT_ERRNO then silently blocks (no dmesg
# entry — confirmed absent). controller-runtime's manager.New() needs
# working timers/deadlines for its first API-discovery call, so spod's
# main container crash-loops with:
#   "Failed to get API Group-Resources" err="...: context deadline exceeded"
# every time, immediately, not intermittently.
#
# This is a reproduction of kubernetes-sigs/security-profiles-operator#2121
# ("Fixed issue with crashing SPOD daemon by allowing clock_gettime
# syscall") — that fix apparently didn't stick, or doesn't cover every
# environment this syscall's vDSO fast path can be unreliable on.
#
# No Helm value or spod CR field covers this (checked: v0.7.1's
# values.yaml has no seccomp/syscall key at all; spec.allowedSyscalls is
# the wrong knob, see above) — hence a script, not a --set flag. Run this
# once after installing SPO (see docs/enforcement-prerequisites.md);
# re-running is safe; a `helm upgrade`/reinstall will reset the
# ConfigMap and need this run again.
#
# Idempotent: checks whether clock_gettime is already present before
# doing anything, and only restarts spod if the ConfigMap actually
# changed.
set -euo pipefail

NAMESPACE="security-profiles-operator"
CONFIGMAP="security-profiles-operator-profile"

if ! command -v jq >/dev/null 2>&1; then
	echo "jq is required (apt install jq / brew install jq) — used to edit the ConfigMap's embedded JSON in place."
	exit 1
fi

if ! kubectl get ns "$NAMESPACE" >/dev/null 2>&1; then
	echo "Namespace $NAMESPACE not found — is security-profiles-operator installed? See docs/enforcement-prerequisites.md."
	exit 1
fi

current_syscalls="$(kubectl -n "$NAMESPACE" get cm "$CONFIGMAP" -o jsonpath='{.data.security-profiles-operator\.json}' | jq -r '.syscalls[0].names[]')"
if echo "$current_syscalls" | grep -qx "clock_gettime"; then
	echo "clock_gettime already present in ${CONFIGMAP}'s syscall allow-list — nothing to do."
	exit 0
fi

echo "Adding clock_gettime to ${CONFIGMAP}'s syscall allow-list..."
kubectl -n "$NAMESPACE" get cm "$CONFIGMAP" -o json \
	| jq '.data."security-profiles-operator.json" |= (fromjson | .syscalls[0].names += ["clock_gettime"] | tojson)' \
	| kubectl apply -f -

echo "Waiting ~90s for the kubelet ConfigMap-volume sync before restarting spod..."
sleep 90

echo "Restarting spod to pick up the corrected profile..."
kubectl -n "$NAMESPACE" delete pod -l name=spod

echo
echo "Done. Check: kubectl get pods -n $NAMESPACE"
echo "Expect the spod pod's security-profiles-operator container to reach Running with 0 restarts within ~1min."
