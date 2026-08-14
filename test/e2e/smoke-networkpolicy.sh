#!/usr/bin/env bash
set -euo pipefail

# Smoke test: verify NetworkPolicy deny-ingress blocks pod-to-pod traffic.
# This version waits authoritatively for Cilium endpoint policy state (CiliumEndpoint CRD
# or cilium-dbg) instead of grepping logs.
# Exit codes:
# 2 - pod readiness/timeouts
# 3 - could not determine server IP
# 4 - initial connectivity failed
# 5 - connectivity still succeeds after deny (POLICY_NOT_ENFORCED)
# 6 - infrastructure unhealthy during denial check
# 7 - policy not computed for endpoint
# 8 - policy not realized in datapath

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

# bounded retry until success
try_connect() {
  timeout=${1:-30}
  interval=1
  for i in $(seq 1 $timeout); do
    if kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "curl -sS --max-time 2 http://$SERVER_IP:8080 | grep -q hello" >/dev/null 2>&1; then
      return 0
    fi
    sleep $interval
  done
  return 1
}

if ! try_connect 30; then
  echo "ERROR: initial connectivity failed"
  kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl logs -n "$NAMESPACE" "$CLIENT_POD" || true
  exit 4
fi

echo "[smoke-net] initial connectivity OK"

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

# Authoritative Cilium endpoint inspection functions
get_ciliumendpoint_json() {
  # prefer CRD if available
  if kubectl api-resources | grep -qi '^ciliumendpoints'; then
    kubectl -n "$NAMESPACE" get ciliumendpoint "$SERVER_POD" -o json 2>/dev/null || true
  else
    CILIUM_POD=$(kubectl -n kube-system get pods -l k8s-app=cilium -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    if [ -z "$CILIUM_POD" ]; then
      echo ""; return 1
    fi
    kubectl -n kube-system exec "$CILIUM_POD" -- cilium-dbg endpoint get pod-name:"$NAMESPACE":"$SERVER_POD" -o json 2>/dev/null || true
  fi
}

parse_endpoint_policy() {
  json="$1"
  # normalize common key variants; use jq to safely extract
  spec_rev=$(printf '%s' "$json" | jq -r '.status.policy.spec["policy-revision"] // .status.policy.spec["policyRevision"] // empty')
  realized_rev=$(printf '%s' "$json" | jq -r '.status.policy.realized["policy-revision"] // .status.policy.realized["policyRevision"] // empty')
  realized_enabled=$(printf '%s' "$json" | jq -r '.status.policy.realized["policy-enabled"] // .status.policy.realized["policyEnabled"] // empty')
  state=$(printf '%s' "$json" | jq -r '.status.state // .status.Status // empty')
  endpoint_id=$(printf '%s' "$json" | jq -r '.status.endpointID // .status.ID // .status.identity // .status.identity.labels // empty')
  printf '%s|%s|%s|%s|%s' "$spec_rev" "$realized_rev" "$realized_enabled" "$state" "$endpoint_id"
}

# baseline capture
base_spec_rev=0
base_realized_rev=0
if kubectl -n kube-system get ds cilium >/dev/null 2>&1; then
  cep_json=$(get_ciliumendpoint_json) || true
  if [ -n "$cep_json" ]; then
    parsed=$(parse_endpoint_policy "$cep_json")
    base_spec_rev=$(printf '%s' "$parsed" | cut -d'|' -f1)
    base_realized_rev=$(printf '%s' "$parsed" | cut -d'|' -f2)
    base_state=$(printf '%s' "$parsed" | cut -d'|' -f4)
    [ -n "$base_spec_rev" ] || base_spec_rev=0
    [ -n "$base_realized_rev" ] || base_realized_rev=0
    echo "[smoke-net] cilium endpoint baseline: specRev=$base_spec_rev realizedRev=$base_realized_rev state=$base_state"
  else
    echo "[smoke-net] Cilium present but could not read endpoint CRD/CLI; will attempt authoritative checks and fall back if needed" >&2
  fi
else
  echo "[smoke-net] cilium not detected; will fall back to best-effort connectivity polling" >&2
fi

# Ensure NetworkPolicy resource exists
if ! kubectl get networkpolicy deny-client-ingress -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "ERROR: networkpolicy resource missing after apply" >&2
  kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  exit 6
fi

# Wait for spec.policy-revision to advance (policy computed for endpoint)
if kubectl -n kube-system get ds cilium >/dev/null 2>&1; then
  waited=0; timeout=60; interval=1
  NEW_SPEC_REV=''
  while [ $waited -lt $timeout ]; do
    cep_json=$(get_ciliumendpoint_json) || true
    if [ -n "$cep_json" ]; then
      parsed=$(parse_endpoint_policy "$cep_json")
      spec_rev=$(printf '%s' "$parsed" | cut -d'|' -f1)
      realized_rev=$(printf '%s' "$parsed" | cut -d'|' -f2)
      state=$(printf '%s' "$parsed" | cut -d'|' -f4)
      echo "[smoke-net] observed endpoint: state=$state specRev=${spec_rev:-0} realizedRev=${realized_rev:-0}"
      if [ -n "$spec_rev" ] && [ "$spec_rev" -gt "$base_spec_rev" ]; then
        NEW_SPEC_REV=$spec_rev
        break
      fi
    fi
    sleep $interval
    waited=$((waited+interval))
  done
  if [ -z "${NEW_SPEC_REV:-}" ]; then
    echo "ERROR: POLICY_NOT_COMPUTED_FOR_ENDPOINT (spec revision did not advance)" >&2
    kubectl get ciliumendpoint -n "$NAMESPACE" -o yaml || true
    kubectl -n kube-system logs -l k8s-app=cilium --tail=200 || true
    exit 7
  fi

  # Wait for datapath realization: realized.policy-revision == spec.policy-revision and state==ready
  waited=0; timeout=120; interval=2
  while [ $waited -lt $timeout ]; do
    cep_json=$(get_ciliumendpoint_json) || true
    parsed=$(parse_endpoint_policy "$cep_json")
    spec_rev=$(printf '%s' "$parsed" | cut -d'|' -f1)
    realized_rev=$(printf '%s' "$parsed" | cut -d'|' -f2)
    realized_enabled=$(printf '%s' "$parsed" | cut -d'|' -f3)
    state=$(printf '%s' "$parsed" | cut -d'|' -f4)
    echo "[smoke-net] endpoint policy: state=${state:-unknown} specRev=${spec_rev:-0} realizedRev=${realized_rev:-0} enabled=${realized_enabled:-}" 
    if [ -n "$realized_rev" ] && [ -n "$spec_rev" ] && [ "$realized_rev" -ge "$spec_rev" ] && [ "$state" = "ready" ]; then
      if printf '%s' "$realized_enabled" | grep -qi ingress; then
        echo "[smoke-net] endpoint datapath realized policyRev=$realized_rev"
        break
      fi
    fi
    sleep $interval
    waited=$((waited+interval))
  done
  if ! ( [ -n "$realized_rev" ] && [ -n "$spec_rev" ] && [ "$realized_rev" -ge "$spec_rev" ] && [ "$state" = "ready" ] ); then
    echo "ERROR: POLICY_NOT_REALIZED_IN_DATAPATH" >&2
    kubectl get ciliumendpoint -n "$NAMESPACE" -o yaml || true
    kubectl -n kube-system logs -l k8s-app=cilium --tail=400 || true
    exit 8
  fi

  # Now perform denial attempts (fresh connections)
  attempts=3
  success_count=0
  for i in $(seq 1 $attempts); do
    if kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "curl -sS --connect-timeout 2 --max-time 4 -o /dev/null -w '%{http_code}' http://$SERVER_IP:8080" >/dev/null 2>&1; then
      echo "denial attempt $i: still reachable" >&2
      success_count=$((success_count+1))
    else
      echo "denial attempt $i: failed as expected"
    fi
  done
  if [ $success_count -gt 0 ]; then
    echo "ERROR: connectivity still succeeds after datapath realization (POLICY_NOT_ENFORCED)" >&2
    kubectl get pods -n "$NAMESPACE" -o wide || true
    kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
    kubectl -n kube-system logs -l k8s-app=cilium --tail=200 || true
    exit 5
  fi
  echo "[smoke-net] connectivity blocked as expected"
else
  # fallback: best-effort polling (no cilium present)
  try_no_connect() {
    timeout=${1:-120}
    interval=2
    for i in $(seq 1 $timeout); do
      if kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "curl -sS --max-time 2 -o /dev/null -w '%{http_code}' http://$SERVER_IP:8080" >/dev/null 2>&1; then
        sleep $interval
        continue
      else
        return 0
      fi
    done
    return 1
  }

  if ! try_no_connect 120; then
    echo "ERROR: connectivity still succeeds after deny" >&2
    kubectl get pods -n "$NAMESPACE" -o wide || true
    kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
    kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
    exit 5
  fi
  echo "[smoke-net] connectivity blocked as expected (best-effort)"
fi
" != "Running" ]; then
  echo "ERROR: pods not running"
  kubectl describe pod -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl describe pod -n "$NAMESPACE" "$CLIENT_POD" || true
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 2
fi

# resolve server cluster IP and test connectivity from client
SERVER_IP=$(kubectl get pod "$SERVER_POD" -n "$NAMESPACE" -o jsonpath='{.status.podIP}')
if [ -z "$SERVER_IP" ]; then
  echo "ERROR: could not get server pod IP"
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 3
fi

echo "[smoke-net] serverIP=$SERVER_IP:8080; testing connectivity"
# bounded retry until success
try_connect() {
  timeout=${1:-20}
  interval=1
  for i in $(seq 1 $timeout); do
    if kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "curl -sS --max-time 2 http://$SERVER_IP:8080 | grep -q hello" >/dev/null 2>&1; then
      return 0
    fi
    sleep $interval
  done
  return 1
}

if ! try_connect 20; then
  echo "ERROR: initial connectivity failed"
  kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 4
fi

echo "[smoke-net] initial connectivity OK"

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

# wait until connectivity fails (bounded)
try_no_connect() {
  timeout=${1:-20}
  interval=1
  for i in $(seq 1 $timeout); do
    if kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "curl -sS --max-time 2 http://$SERVER_IP:8080" >/dev/null 2>&1; then
      sleep $interval
      continue
    else
      return 0
    fi
  done
  return 1
}

if ! try_no_connect 20; then
  echo "ERROR: connectivity still succeeds after deny"
  kubectl get pods -n "$NAMESPACE" -o wide || true
  kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 5
fi

echo "[smoke-net] connectivity blocked as expected"

# cleanup
kubectl delete ns "$NAMESPACE" --ignore-not-found
exit 0
" != "Running" ] || [ "$(kubectl get pod "$CLIENT_POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}')" != "Running" ]; then
  echo "ERROR: pods not running"
  kubectl describe pod -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl describe pod -n "$NAMESPACE" "$CLIENT_POD" || true
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 2
fi

# resolve server cluster IP and test connectivity from client
SERVER_IP=$(kubectl get pod "$SERVER_POD" -n "$NAMESPACE" -o jsonpath='{.status.podIP}')
if [ -z "$SERVER_IP" ]; then
  echo "ERROR: could not get server pod IP"
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 3
fi

echo "[smoke-net] serverIP=$SERVER_IP:8080; testing connectivity"
kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "echo ping | nc -w 2 $SERVER_IP 8080" || { echo "ERROR: initial connectivity failed"; kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true; kubectl delete ns "$NAMESPACE" --ignore-not-found; exit 4; }

echo "[smoke-net] initial connectivity OK"

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

# wait a moment for policy to apply
sleep 3

echo "[smoke-net] testing connectivity after deny"
if kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "echo ping | nc -w 2 $SERVER_IP 8080"; then
  echo "ERROR: connectivity still succeeds after deny"
  kubectl get pods -n "$NAMESPACE" -o wide || true
  kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 5
fi

echo "[smoke-net] connectivity blocked as expected"

# cleanup
kubectl delete ns "$NAMESPACE" --ignore-not-found
exit 0
