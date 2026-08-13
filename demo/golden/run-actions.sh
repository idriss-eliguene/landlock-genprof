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
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS file:///etc/hosts >/dev/null
    ;;
  fs_2_of_3)
    # write in a dedicated subdir under /var/tmp -> use curl to fetch echo and write file
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o /var/tmp/nginx-demo-2/marker
    ;;
  fs_1_of_3)
    # transient write under /srv/nginx/data -> use curl to fetch echo and write transient file
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --create-dirs http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 -o /srv/nginx/data/transient
    ;;
  net_common)
    # egress to echo service (port 8080) -> expected 3/3
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --max-time 2 -I http://echo-8080-svc.landlock-genprof-e2e.svc.cluster.local:8080 >/dev/null
    ;;
  net_2_of_3)
    # occasional egress to echo-8081 (expected 2/3)
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --max-time 2 -I http://echo-8081-svc.landlock-genprof-e2e.svc.cluster.local:8081 >/dev/null
    ;;
  net_1_of_3)
    # transient egress to echo-8082 (expected 1/3)
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- curl -sS --max-time 2 -I http://echo-8082-svc.landlock-genprof-e2e.svc.cluster.local:8082 >/dev/null
    ;;
  cap_common)
    # capability probe: best-effort attempt; deterministic capture cannot be guaranteed in all environments.
    # Here we attempt a benign probe that may trigger cap_capable() checks in some runtimes; treat capability as CONDITIONAL.
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- sh -c 'echo "cap-probe" >/tmp/cap-probe || true' || true
    ;;
  syscall_probe)
    # provoke some syscalls: open a temp file repeatedly
    kubectl exec -n "$NAMESPACE" "$POD" -c "$CONTAINER" -- sh -c 'for i in 1 2 3; do echo $i > /tmp/x$i; done' || true
    ;;
  *)
    echo "unknown action: $1" >&2
    exit 2
    ;;
esac
