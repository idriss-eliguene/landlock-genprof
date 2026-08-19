#!/usr/bin/env bash
set -euo pipefail

# Smoke test: verify NetworkPolicy deny-ingress blocks pod-to-pod traffic.
#
# This test is deliberately black-box: it never inspects CNI-internal policy state
# (CiliumEndpoint CRD, cilium-dbg endpoint revisions, agent logs). Those introspection
# paths proved to be version- and image-specific and repeatedly broke in ways unrelated
# to the behaviour under test. What this test asserts is the thing that actually matters:
# after the NetworkPolicy is applied, client->server traffic stops being allowed.
#
# Enforcement is therefore detected by polling the same connectivity probe used for the
# initial "connectivity OK" assertion, and waiting for it to start failing.
#
# Exit codes:
# 2 - pod readiness/timeouts
# 3 - could not determine server IP
# 4 - initial connectivity failed
# 5 - connectivity still succeeds after deny (POLICY_NOT_ENFORCED)
# 6 - infrastructure unhealthy during connectivity probing

NAMESPACE=${1:-landlock-genprof-net}
SERVER_POD=net-server
CLIENT_POD=net-client

cleanup() {
  kubectl delete ns "$NAMESPACE" --ignore-not-found || true
}
trap cleanup EXIT

kubectl create ns "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# server: deterministic HTTP listener using hashicorp/http-echo
cat <<'EOF' | kubectl apply -n "$NAMESPACE" -f -
apiVersion: v1
kind: Pod
metadata:
  name: net-server
  labels:
    app: net-server
spec:
  containers:
  - name: server
    image: hashicorp/http-echo:0.2.3
    args: ["-text=hello","-listen=:8080"]
    ports:
    - containerPort: 8080
    imagePullPolicy: IfNotPresent
  restartPolicy: Always
EOF

# client pod used to test connectivity (curl image)
cat <<'EOF' | kubectl apply -n "$NAMESPACE" -f -
apiVersion: v1
kind: Pod
metadata:
  name: net-client
spec:
  containers:
  - name: client
    image: curlimages/curl:8.2.1
    command: ["/bin/sh","-c","sleep 300"]
    imagePullPolicy: IfNotPresent
  restartPolicy: Always
EOF

# Wait for pods to be Ready (bounded, generous timeout)
if ! kubectl wait --for=condition=Ready pod/$SERVER_POD -n "$NAMESPACE" --timeout=120s >/dev/null 2>&1; then
  echo "ERROR: server pod did not become Ready within timeout" >&2
  kubectl describe pod -n "$NAMESPACE" "$SERVER_POD" || true
  exit 2
fi
if ! kubectl wait --for=condition=Ready pod/$CLIENT_POD -n "$NAMESPACE" --timeout=120s >/dev/null 2>&1; then
  echo "ERROR: client pod did not become Ready within timeout" >&2
  kubectl describe pod -n "$NAMESPACE" "$CLIENT_POD" || true
  exit 2
fi

# resolve server pod IP and test connectivity from client
SERVER_IP=$(kubectl get pod "$SERVER_POD" -n "$NAMESPACE" -o jsonpath='{.status.podIP}')
if [ -z "$SERVER_IP" ]; then
  echo "ERROR: could not get server pod IP"
  exit 3
fi

echo "[smoke-net] serverIP=$SERVER_IP:8080; testing connectivity"

PROBE_ERR=$(mktemp)
trap 'cleanup; rm -f "$PROBE_ERR"' EXIT

# Remote probe: run curl in the client pod and report curl's own exit status
# explicitly. curl's stderr is folded into stdout so that $PROBE_ERR only ever
# holds errors produced by kubectl exec itself. Because the remote shell ends
# with printf, `kubectl exec` exits non-zero only on infrastructure failure --
# never merely because the connection was refused. That distinction is what
# keeps a broken exec from being silently misread as "traffic blocked".
REMOTE_PROBE="curl -sS --connect-timeout 2 --max-time 4 -o /dev/null -w '%{http_code}' http://$SERVER_IP:8080 2>&1; printf '|rc=%s' \$?"

# probe_connectivity: single client->server connection attempt.
# Returns 0 = connected, 1 = blocked/refused, 2 = infrastructure failure.
# Sets PROBE_DETAIL with the full context of what was observed.
PROBE_DETAIL=''
probe_connectivity() {
  local out rc curl_rc
  rc=0
  out=$(kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "$REMOTE_PROBE" 2>"$PROBE_ERR") || rc=$?
  if [ "$rc" -ne 0 ]; then
    PROBE_DETAIL="kubectl exec failed (exit=$rc): $(tr '\n' ' ' <"$PROBE_ERR")"
    return 2
  fi
  case "$out" in
    *'|rc='*) curl_rc=${out##*'|rc='} ;;
    *)
      PROBE_DETAIL="malformed probe output (no rc marker): '$out'"
      return 2
      ;;
  esac
  if ! printf '%s' "$curl_rc" | grep -Eq '^[0-9]+$'; then
    PROBE_DETAIL="non-numeric curl exit status '$curl_rc' in probe output: '$out'"
    return 2
  fi
  PROBE_DETAIL="curl_rc=$curl_rc response='${out%%'|rc='*}'"
  [ "$curl_rc" -eq 0 ] && return 0
  return 1
}

# bounded retry until the connection succeeds
try_connect() {
  local timeout=${1:-30} interval=1 waited=0 status
  while [ "$waited" -lt "$timeout" ]; do
    status=0; probe_connectivity || status=$?
    case "$status" in
      0) return 0 ;;
      2) echo "[smoke-net] probe infrastructure error: $PROBE_DETAIL" >&2 ;;
    esac
    sleep "$interval"
    waited=$((waited + interval))
  done
  return 1
}

if ! try_connect 30; then
  echo "ERROR: initial connectivity failed; last probe: ${PROBE_DETAIL:-none}" >&2
  kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl logs -n "$NAMESPACE" "$CLIENT_POD" || true
  exit 4
fi

echo "[smoke-net] initial connectivity OK ($PROBE_DETAIL)"

# apply a deny ingress policy to block client->server
cat <<'EOF' | kubectl apply -n "$NAMESPACE" -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-client-ingress
spec:
  podSelector:
    matchLabels:
      app: net-server
  policyTypes:
  - Ingress
  ingress: []
EOF

# Ensure the NetworkPolicy resource actually exists before asserting on its effect.
if ! kubectl get networkpolicy deny-client-ingress -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "ERROR: networkpolicy resource missing after apply" >&2
  kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  exit 6
fi

# Black-box enforcement check.
#
# Poll the same connectivity probe used above and wait for it to start failing.
# No CNI-internal state is consulted: whether the policy has been "computed" or
# "realized" is an implementation detail, and the only observable that matters is
# that the traffic stops flowing.
#
# A single blocked probe is not sufficient to declare success -- one flaky curl
# would otherwise turn into a false pass, which is the worst possible failure mode
# for this test. Two consecutive blocked observations are required.
DENY_WINDOW=40           # seconds to wait for enforcement to take effect
DENY_INTERVAL=2          # seconds between probes
REQUIRED_CONSECUTIVE_BLOCKED=2

echo "[smoke-net] policy applied; waiting up to ${DENY_WINDOW}s for traffic to be blocked"

waited=0
consecutive_blocked=0
consecutive_infra_errors=0
last_connected_detail=''
blocked=0

while [ "$waited" -lt "$DENY_WINDOW" ]; do
  status=0; probe_connectivity || status=$?
  case "$status" in
    0)
      consecutive_blocked=0
      consecutive_infra_errors=0
      last_connected_detail="$PROBE_DETAIL"
      echo "[smoke-net] t=${waited}s still reachable ($PROBE_DETAIL)"
      ;;
    1)
      consecutive_infra_errors=0
      consecutive_blocked=$((consecutive_blocked + 1))
      echo "[smoke-net] t=${waited}s blocked ($PROBE_DETAIL) [$consecutive_blocked/$REQUIRED_CONSECUTIVE_BLOCKED]"
      if [ "$consecutive_blocked" -ge "$REQUIRED_CONSECUTIVE_BLOCKED" ]; then
        blocked=1
        break
      fi
      ;;
    2)
      # Infrastructure failure is never counted as "blocked".
      consecutive_blocked=0
      consecutive_infra_errors=$((consecutive_infra_errors + 1))
      echo "[smoke-net] t=${waited}s probe infrastructure error: $PROBE_DETAIL" >&2
      if [ "$consecutive_infra_errors" -ge 5 ]; then
        echo "ERROR: connectivity probe could not be executed ($consecutive_infra_errors consecutive failures)" >&2
        echo "Last probe detail: $PROBE_DETAIL" >&2
        kubectl get pods -n "$NAMESPACE" -o wide || true
        kubectl describe pod -n "$NAMESPACE" "$CLIENT_POD" || true
        exit 6
      fi
      ;;
  esac
  sleep "$DENY_INTERVAL"
  waited=$((waited + DENY_INTERVAL))
done

if [ "$blocked" -ne 1 ]; then
  echo "ERROR: connectivity still succeeds ${DENY_WINDOW}s after applying deny-ingress policy (POLICY_NOT_ENFORCED)" >&2
  echo "Last successful probe: ${last_connected_detail:-none}" >&2
  echo "Last probe detail: ${PROBE_DETAIL:-none}" >&2
  echo "--- pods ---" >&2
  kubectl get pods -n "$NAMESPACE" -o wide || true
  echo "--- networkpolicy ---" >&2
  kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  echo "--- server logs ---" >&2
  kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
  if kubectl -n kube-system get ds cilium >/dev/null 2>&1; then
    echo "--- cilium agent logs (diagnostic only) ---" >&2
    kubectl -n kube-system logs -l k8s-app=cilium --tail=200 || true
  fi
  exit 5
fi

echo "[smoke-net] connectivity blocked as expected after ${waited}s"

# cleanup
kubectl delete ns "$NAMESPACE" --ignore-not-found
exit 0

