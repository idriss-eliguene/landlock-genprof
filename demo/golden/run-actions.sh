#!/usr/bin/env bash
# run-actions.sh
# Helper to perform deterministic actions inside the nginx-demo pod
# Usage: ./run-actions.sh <action>
set -euo pipefail

NAMESPACE=${NAMESPACE:-landlock-genprof-e2e}
POD=${POD:-nginx-demo}
CONTAINER=${CONTAINER:-nginx}
# ACTION_CONTAINER is the sidecar used to perform harness actions; defaults to the primary CONTAINER
ACTION_CONTAINER=${ACTION_CONTAINER:-${CONTAINER}}
# CURL_BIN is the authoritative executable path for deterministic required actions.
# Keep this fixed (no fallback probing) to preserve fixture determinism.
CURL_BIN=${CURL_BIN:-/usr/bin/curl}

kubectl() { command kubectl "$@"; }

# Strict _try_fetch: REQUIRED Golden network/fs observations MUST be
# performed by CURL_BIN inside the tools container. No fallbacks,
# no package installs. If curl unavailable, return non-zero immediately.
_try_fetch() {
  local args=("$@")
  # Attempt in-container curl only. Do not swallow stderr; let caller
  # capture stdout/stderr via the wrapper.
  if kubectl exec -n "$NAMESPACE" "$POD" -c "$ACTION_CONTAINER" -- "$CURL_BIN" "${args[@]}"; then
    return 0
  fi
  return 1
}

case ${1:-} in
  fs_common)
    # read common file -> use in-container curl to open /etc/hosts so proc.comm == curl
    # Use file:// scheme; rely on CURL_BIN inside the tools container
    if ! kubectl exec -n "$NAMESPACE" "$POD" -c "$ACTION_CONTAINER" -- "$CURL_BIN" -sS file:///etc/hosts -o /dev/null; then
      echo "ERROR: fs_common in-container curl failed" >&2
      exit 1
    fi
    ;;
  fs_2_of_3)
    # write into /var/tmp/nginx-demo-2/marker using in-container curl
    # Use curl --create-dirs to create parent directories in one step
    if ! kubectl exec -n "$NAMESPACE" "$POD" -c "$ACTION_CONTAINER" -- "$CURL_BIN" -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o /var/tmp/nginx-demo-2/marker; then
      echo "ERROR: fs_2_of_3 in-container curl failed" >&2
      exit 1
    fi
    ;;
  fs_1_of_3)
    # write into /srv/nginx/data/transient using in-container curl
    if ! kubectl exec -n "$NAMESPACE" "$POD" -c "$ACTION_CONTAINER" -- "$CURL_BIN" -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o /srv/nginx/data/transient; then
      echo "ERROR: fs_1_of_3 in-container curl failed" >&2
      exit 1
    fi
    ;;
  net_common)
    # egress to echo service (port 8080) -> expected 3/3
    if ! _try_fetch --max-time 2 -I http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080; then
      echo "ERROR: net_common fetch failed" >&2
      exit 1
    fi
    ;;
  net_2_of_3)
    # occasional egress to echo-8081 (expected 2/3)
    if ! _try_fetch --max-time 2 -I http://echo-8081-svc.landlock-genprof-e2e.svc.cluster.local:8081; then
      echo "ERROR: net_2_of_3 fetch failed" >&2
      exit 1
    fi
    ;;
  net_1_of_3)
    # transient egress to echo-8082 (expected 1/3)
    if ! _try_fetch --max-time 2 -I http://echo-8082-svc.landlock-genprof-e2e.svc.cluster.local:8082; then
      echo "ERROR: net_1_of_3 fetch failed" >&2
      exit 1
    fi
    ;;
  cap_common)
    # capability probe: best-effort. This remains best-effort because
    # deterministic capability/syscall stimuli are not part of the
    # strict curl-based contract. Keep as non-gating.
    tmpfile=$(mktemp)
    printf 'cap-probe' > "$tmpfile"
    kubectl cp "$tmpfile" "$NAMESPACE/$POD:/tmp/cap-probe" -c "$ACTION_CONTAINER" || true
    rm -f "$tmpfile" || true
    ;;
  syscall_probe)
    # syscall probe remains best-effort. Use kubectl cp for simplicity
    # but do not assert these events in Golden's deterministic checks.
    for i in 1 2 3; do
      tmpfile=$(mktemp)
      printf "%s" "$i" > "$tmpfile"
      kubectl cp "$tmpfile" "$NAMESPACE/$POD:/tmp/x${i}" -c "$ACTION_CONTAINER" || true
      rm -f "$tmpfile" || true
    done
    ;;
  *)
    echo "unknown action: $1" >&2
    exit 2
    ;;
esac
