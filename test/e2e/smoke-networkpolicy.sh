#!/usr/bin/env bash
set -euo pipefail

NAMESPACE=${1:-landlock-genprof-net}
SERVER_POD=net-server
CLIENT_POD=net-client

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

# Wait for pods to be Ready (use kubectl wait for correct condition)
if ! kubectl wait --for=condition=Ready pod/$SERVER_POD -n "$NAMESPACE" --timeout=60s >/dev/null 2>&1; then
  echo "ERROR: server pod did not become Ready within timeout" >&2
  kubectl describe pod -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 2
fi
if ! kubectl wait --for=condition=Ready pod/$CLIENT_POD -n "$NAMESPACE" --timeout=60s >/dev/null 2>&1; then
  echo "ERROR: client pod did not become Ready within timeout" >&2
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

# wait until connectivity fails (bounded), with infrastructure checks before attributing to policy
try_no_connect() {
  timeout=${1:-60}
  interval=1
  for i in $(seq 1 $timeout); do
    if kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "curl -sS --max-time 2 http://$SERVER_IP:8080" >/dev/null 2>&1; then
      sleep $interval
      continue
    else
      # verify infrastructure health before accepting policy effect
      srv_ready=$(kubectl get pod "$SERVER_POD" -n "$NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
      cli_ready=$(kubectl get pod "$CLIENT_POD" -n "$NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
      if [ "$srv_ready" != "True" ]; then
        echo "ERROR: server pod not Ready (srv_ready=$srv_ready) — cannot attribute to policy" >&2
        kubectl describe pod -n "$NAMESPACE" "$SERVER_POD" || true
        return 2
      fi
      if [ "$cli_ready" != "True" ]; then
        echo "ERROR: client pod not Ready (cli_ready=$cli_ready) — cannot attribute to policy" >&2
        kubectl describe pod -n "$NAMESPACE" "$CLIENT_POD" || true
        return 2
      fi
      if ! kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- sh -c "command -v curl" >/dev/null 2>&1; then
        echo "ERROR: client missing curl binary — cannot attribute" >&2
        return 2
      fi
      # success: connectivity denied and infrastructure healthy
      return 0
    fi
  done
  return 1
}

res=$(try_no_connect 60 || true)
if [ "$res" = "0" ]; then
  echo "[smoke-net] connectivity blocked as expected"
elif [ "$res" = "2" ]; then
  echo "ERROR: infrastructure unhealthy during denial check" >&2
  kubectl get pods -n "$NAMESPACE" -o wide || true
  kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 6
else
  echo "ERROR: connectivity still succeeds after deny" >&2
  echo "[diag] pods in namespace:"; kubectl get pods -n "$NAMESPACE" -o wide || true
  echo "[diag] networkpolicies in namespace:"; kubectl get networkpolicy -n "$NAMESPACE" -o wide || true
  echo "[diag] networkpolicy yaml:"; kubectl get networkpolicy -n "$NAMESPACE" -o yaml || true
  echo "[diag] cilium endpoints (brief):"; kubectl -n kube-system logs -l k8s-app=cilium --tail=200 || true
  kubectl logs -n "$NAMESPACE" "$SERVER_POD" || true
  kubectl delete ns "$NAMESPACE" --ignore-not-found
  exit 5
fi

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
