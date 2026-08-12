#!/usr/bin/env bash
set -euo pipefail

NAMESPACE=${1:-landlock-genprof-net}
SERVER_POD=net-server
CLIENT_POD=net-client

kubectl create ns "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# server: simple TCP listener using busybox nc
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
    image: busybox
    command: ["/bin/sh","-c","while true; do nc -l -p 8080 -e echo; done"]
    ports:
    - containerPort: 8080
    imagePullPolicy: IfNotPresent
  restartPolicy: Always
EOF

# client pod used to test connectivity
cat <<'EOF' | kubectl apply -n "$NAMESPACE" -f -
apiVersion: v1
kind: Pod
metadata:
  name: net-client
spec:
  containers:
  - name: client
    image: busybox
    command: ["/bin/sh","-c","sleep 300"]
    imagePullPolicy: IfNotPresent
  restartPolicy: Always
EOF

# wait for pods
for i in $(seq 1 60); do
  s1=$(kubectl get pod "$SERVER_POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
  s2=$(kubectl get pod "$CLIENT_POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
  echo "[smoke-net] server=$s1 client=$s2"
  if [ "$s1" = "Running" -a "$s2" = "Running" ]; then break; fi
  sleep 1
done

if [ "$(kubectl get pod "$SERVER_POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}')" != "Running" ] || [ "$(kubectl get pod "$CLIENT_POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}')" != "Running" ]; then
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
