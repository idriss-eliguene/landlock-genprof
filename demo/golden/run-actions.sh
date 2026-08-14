#!/usr/bin/env bash
# run-actions.sh
# Helper to perform deterministic actions inside the nginx-demo pod
# Usage: ./run-actions.sh <action>
set -euo pipefail

NAMESPACE=${NAMESPACE:-landlock-genprof-e2e}
POD=${POD:-nginx-demo}
CONTAINER=${CONTAINER:-nginx}

kubectl() { command kubectl "$@"; }

# Try to run curl/wget/busybox in the container; if missing, attempt
# package-manager installs via direct execs (no shell). After install
# attempts, try curl again. This avoids using `sh -c` inside a shell-less
# container (distroless/scratch) which causes "sh: executable file not found".
_try_fetch() {
  # $1..$@ are the arguments passed to curl (e.g. -sS file://...)
  local args=("$@")

  # Try curl directly
  if kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl "${args[@]}" >/dev/null 2>&1; then
    return 0
  fi
  # Try wget
  if kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- wget -qO- "${args[@]}" >/dev/null 2>&1; then
    return 0
  fi
  # Try busybox wget
  if kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- busybox wget -qO- "${args[@]}" >/dev/null 2>&1; then
    return 0
  fi

  # Attempt package manager installs without shell-chaining
  # Try apk (Alpine)
  kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- apk add --no-cache curl >/dev/null 2>&1 || true
  # Try apt (Debian/Ubuntu) - run update then install as separate execs
  kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- apt-get update -y >/dev/null 2>&1 || true
  kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- apt-get install -y curl >/dev/null 2>&1 || true
  # Try dnf (Fedora/RHEL)
  kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- dnf install -y curl >/dev/null 2>&1 || true

  # After install attempts, try curl again
  if kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl "${args[@]}" >/dev/null 2>&1; then
    return 0
  fi

  return 1
}

case ${1:-} in
  fs_common)
    # read common file -> use curl to open /etc/hosts so proc.comm == curl
    _try_fetch file:///etc/hosts
    ;;
  fs_2_of_3)
    # write in a dedicated subdir under /var/tmp -> fetch locally and copy into pod
    tmpfile=$(mktemp)
    if command -v curl >/dev/null 2>&1; then
      curl -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o "$tmpfile" || true
    else
      wget -qO "$tmpfile" http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 >/dev/null 2>&1 || true
    fi
    # create target dir inside pod without using a shell by using kubectl exec to mkdir
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- mkdir -p /var/tmp/nginx-demo-2 >/dev/null 2>&1 || true
    kubectl cp "$tmpfile" "$NAMESPACE/$POD:/var/tmp/nginx-demo-2/marker" -c "$CONTAINER" || true
    rm -f "$tmpfile" || true
    ;;
  fs_1_of_3)
    # transient write under /srv/nginx/data -> fetch locally and copy into pod
    tmpfile=$(mktemp)
    if command -v curl >/dev/null 2>&1; then
      curl -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o "$tmpfile" || true
    else
      wget -qO "$tmpfile" http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 >/dev/null 2>&1 || true
    fi
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- mkdir -p /srv/nginx/data >/dev/null 2>&1 || true
    kubectl cp "$tmpfile" "$NAMESPACE/$POD:/srv/nginx/data/transient" -c "$CONTAINER" || true
    rm -f "$tmpfile" || true
    ;;
  net_common)
    # egress to echo service (port 8080) -> expected 3/3
    _try_fetch --max-time 2 -I http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 || true
    ;;
  net_2_of_3)
    # occasional egress to echo-8081 (expected 2/3)
    _try_fetch --max-time 2 -I http://echo-8081-svc.landlock-genprof-e2e.svc.cluster.local:8081 || true
    ;;
  net_1_of_3)
    # transient egress to echo-8082 (expected 1/3)
    _try_fetch --max-time 2 -I http://echo-8082-svc.landlock-genprof-e2e.svc.cluster.local:8082 || true
    ;;
  cap_common)
    # capability probe: best-effort attempt. Avoid relying on a shell in the target container.
    # Copy a small local file into the pod so the tracer observes open/write syscalls without requiring /bin/sh.
    tmpfile=$(mktemp)
    printf 'cap-probe' > "$tmpfile"
    kubectl cp "$tmpfile" "$NAMESPACE/$POD:/tmp/cap-probe" -c "$CONTAINER" || true
    rm -f "$tmpfile" || true
    ;;
  syscall_probe)
    # provoke some syscalls: create small files locally and copy them into the pod to avoid using a shell.
    for i in 1 2 3; do
      tmpfile=$(mktemp)
      printf "%s" "$i" > "$tmpfile"
      kubectl cp "$tmpfile" "$NAMESPACE/$POD:/tmp/x${i}" -c "$CONTAINER" || true
      rm -f "$tmpfile" || true
    done
    ;;
  *)
    echo "unknown action: $1" >&2
    exit 2
    ;;
esac
