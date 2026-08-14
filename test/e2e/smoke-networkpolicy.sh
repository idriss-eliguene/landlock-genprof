#!/usr/bin/env bash
set -euo pipefail

# Smoke test: verify NetworkPolicy deny-ingress blocks pod-to-pod traffic.
# Exit codes:
# 2 - pod readiness/timeouts
# 3 - could not determine server IP
# 4 - initial connectivity failed
# 5 - connectivity still succeeds after deny (POLICY_NOT_ENFORCED)
# 6 - infrastructure unhealthy during denial check

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

# Wait for policy propagation: ensure policy exists and targets server pod
wait_for_policy() {
  timeout=${1:-30}
  for i in $(seq 1 $timeout); do
    if kubectl get networkpolicy deny-client-ingress -n "$NAMESPACE" >/dev/null 2>&1; then
      # verify server pod has label expected by policy
      if kubectl get pod "$SERVER_POD" -n "$NAMESPACE" -o jsonpath='{.metadata.labels.app}' 2>/dev/null | grep -q "net-server"; then
        return 0
      fi
    fi
    sleep 1
  done
  return 1
}

# Wait for cilium to reload endpoint BPF program for server pod (best-effort)
wait_for_cilium_reload() {
  timeout=${1:-30}
  for i in $(seq 1 $timeout); do
    if kubectl -n kube-system logs -l k8s-app=cilium --tail=200 2>/dev/null | grep -q "Reloaded endpoint BPF program.*k8sPodName=landlock-genprof-net/net-server"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

if ! wait_for_policy 30; then
  echo "ERROR: policy not observed or server label mismatch" >&2
  kubectl get networkpolicy -n "$NAMESPACE" -o wide || true
  kubectl get pod -n "$NAMESPACE" -o wide || true
  exit 6
fi

# only attempt to wait for cilium reload when cilium appears present
if kubectl -n kube-system get ds cilium >/dev/null 2>&1; then
  if ! wait_for_cilium_reload 30; then
    echo "WARN: did not observe cilium endpoint BPF reload for server pod (continuing)" >&2
  fi
fi

# try_no_connect: bounded retry where success = connectivity becomes denied while infra healthy
try_no_connect() {
  timeout=${1:-120}
  interval=2
  for i in $(seq 1 $timeout); do
    if kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "curl -sS --max-time 2 -o /dev/null -w '%{http_code}' http://$SERVER_IP:8080" >/dev/null 2>&1; then
      # still reachable (HTTP code printed), wait and retry
      sleep $interval
      continue
    else
      # network blocked (curl failed); verify infra health
      srv_ready=$(kubectl get pod "$SERVER_POD" -n "$NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
      cli_ready=$(kubectl get pod "$CLIENT_POD" -n "$NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
      if [ "$srv_ready" != "True" ] || [ "$cli_ready" != "True" ]; then
        echo "ERROR: infra unhealthy during denial check (srv_ready=$srv_ready cli_ready=$cli_ready)" >&2
        kubectl describe pod -n "$NAMESPACE" "$SERVER_POD" || true
        kubectl describe pod -n "$NAMESPACE" "$CLIENT_POD" || true
        return 2
      fi
      if ! kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "command -v curl" >/dev/null 2>&1; then
        echo "ERROR: client missing curl binary — cannot attribute" >&2
        return 2
      fi
      # Confirm Cilium has processed policies for this endpoint if cilium is present
      if kubectl -n kube-system get ds cilium >/dev/null 2>&1; then
        # attempt to view recent cilium agent logs (best-effort)
        kubectl -n kube-system logs -l k8s-app=cilium --tail=100 || true
      fi
      return 0
    fi
  done
  return 1
}

res=$(try_no_connect 120 || true)
if [ "$res" = "0" ]; then
  echo "[smoke-net] connectivity blocked as expected"
elif [ "$res" = "2" ]; then
  echo "ERROR: infrastructure unhealthy during denial check" >&2
  kubectl get pods -n "$NAMESPACE" -o wide || true
  kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  exit 6
else
  echo "ERROR: connectivity still succeeds after deny (POLICY_NOT_ENFORCED)" >&2
  echo "[diag] pods in namespace:"; kubectl get pods -n "$NAMESPACE" -o wide || true
  echo "[diag] networkpolicies in namespace:"; kubectl get networkpolicy -n "$NAMESPACE" -o wide || true
  echo "[diag] networkpolicy yaml:"; kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  echo "[diag] cilium endpoints (brief):"; kubectl -n kube-system logs -l k8s-app=cilium --tail=200 || true
  kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
  exit 5
fi

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
