# Step 5 — Optional NetworkPolicy generation

PodLock's own CRD has no field for network rights, so `connect`/`bind`
observations get their own output format instead: pass `--network-out` to
also generate a Kubernetes `NetworkPolicy` from the same training run
(skipped if no network activity was observed). `--out`/`--network-out`
both default to a filename derived from the traced pod
(`<pod>-profile.yaml`, `<pod>-networkpolicy.yaml`) when passed with no
value — pass an explicit filename (`--network-out my-policy.yaml`) to
override:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nginx-demo
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: nginx        # copied from the traced pod's own labels
  policyTypes:
    - Egress
  egress:
    - ports:
        - protocol: TCP
          port: 443      # confidence: high
```

Only the observed port is encoded — no `from`/`to` peer restriction, since
the tracer knows a port was contacted, not a peer pod/service identity.
