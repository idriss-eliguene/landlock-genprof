#!/usr/bin/env bash
# run-actions.sh
# Helper to perform deterministic actions inside the nginx-demo pod
# Usage: ./run-actions.sh <action>
set -euo pipefail

NAMESPACE=${NAMESPACE:-landlock-genprof-e2e}
POD=${POD:-nginx-demo}
CONTAINER=${CONTAINER:-nginx}

kubectl() { command kubectl "$@"; }

case ${1:-} in
  fs_common)
    # read common file -> use curl to open /etc/hosts so proc.comm == curl
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS file:///etc/hosts >/dev/null 2>&1 || kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- sh -c 'command -v curl >/dev/null 2>&1 || (if command -v apk >/dev/null 2>&1; then apk add --no-cache curl >/dev/null; elif command -v apt-get >/dev/null 2>&1; then apt-get update >/dev/null && apt-get install -y curl >/dev/null; elif command -v dnf >/dev/null 2>&1; then dnf install -y curl >/dev/null; else echo "curl not found and no supported package manager" >&2; exit 2; fi); exec curl -sS file:///etc/hosts'
    ;;
  fs_2_of_3)
    # write in a dedicated subdir under /var/tmp -> use curl to fetch echo and write file
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o /var/tmp/nginx-demo-2/marker >/dev/null 2>&1 || kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- sh -c 'command -v curl >/dev/null 2>&1 || (if command -v apk >/dev/null 2>&1; then apk add --no-cache curl >/dev/null; elif command -v apt-get >/dev/null 2>&1; then apt-get update >/dev/null && apt-get install -y curl >/dev/null; elif command -v dnf >/dev/null 2>&1; then dnf install -y curl >/dev/null; else echo "curl not found and no supported package manager" >&2; exit 2; fi); exec curl -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o /var/tmp/nginx-demo-2/marker'
    ;;
  fs_1_of_3)
    # transient write under /srv/nginx/data -> use curl to fetch echo and write transient file
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o /srv/nginx/data/transient >/dev/null 2>&1 || kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- sh -c 'command -v curl >/dev/null 2>&1 || (if command -v apk >/dev/null 2>&1; then apk add --no-cache curl >/dev/null; elif command -v apt-get >/dev/null 2>&1; then apt-get update >/dev/null && apt-get install -y curl >/dev/null; elif command -v dnf >/dev/null 2>&1; then dnf install -y curl >/dev/null; else echo "curl not found and no supported package manager" >&2; exit 2; fi); exec curl -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o /srv/nginx/data/transient'
    ;;
  net_common)
    # egress to echo service (port 8080) -> expected 3/3
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --max-time 2 -I http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 >/dev/null 2>&1 || kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- sh -c 'command -v curl >/dev/null 2>&1 || (if command -v apk >/dev/null 2>&1; then apk add --no-cache curl >/dev/null; elif command -v apt-get >/dev/null 2>&1; then apt-get update >/dev/null && apt-get install -y curl >/dev/null; elif command -v dnf >/dev/null 2>&1; then dnf install -y curl >/dev/null; else echo "curl not found and no supported package manager" >&2; exit 2; fi); exec curl -sS --max-time 2 -I http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 >/dev/null'
    ;;
  net_2_of_3)
    # occasional egress to echo-8081 (expected 2/3)
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- sh -c 'command -v curl >/dev/null 2>&1 || (if command -v apk >/dev/null 2>&1; then apk add --no-cache curl >/dev/null; elif command -v apt-get >/dev/null 2>&1; then apt-get update >/dev/null && apt-get install -y curl >/dev/null; elif command -v dnf >/dev/null 2>&1; then dnf install -y curl >/dev/null; else echo "curl not found and no supported package manager" >&2; exit 2; fi); exec curl -sS --max-time 2 -I http://echo-8081-svc.landlock-genprof-e2e.svc.cluster.local:8081 >/dev/null'
    ;;
  net_1_of_3)
    # transient egress to echo-8082 (expected 1/3)
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --max-time 2 -I http://echo-8082-svc.landlock-genprof-e2e.svc.cluster.local:8082 >/dev/null 2>&1 || kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- sh -c 'command -v curl >/dev/null 2>&1 || (if command -v apk >/dev/null 2>&1; then apk add --no-cache curl >/dev/null; elif command -v apt-get >/dev/null 2>&1; then apt-get update >/dev/null && apt-get install -y curl >/dev/null; elif command -v dnf >/dev/null 2>&1; then dnf install -y curl >/dev/null; else echo "curl not found and no supported package manager" >&2; exit 2; fi); exec curl -sS --max-time 2 -I http://echo-8082-svc.landlock-genprof-e2e.svc.cluster.local:8082 >/dev/null'
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
