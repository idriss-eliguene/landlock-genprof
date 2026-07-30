# Step 9 — Optional Linux capabilities fragment (`--capabilities-out`)

Pass `--capabilities-out` to also generate a Linux capabilities fragment
from observed capability checks (skipped if none were observed), via
Inspektor Gadget's `trace_capabilities` gadget (see Step 2's gadget
table):

```yaml
add:
  - NET_BIND_SERVICE   # confidence: high
drop:
  - ALL
```

Unlike the other three outputs, this isn't a complete, standalone
artifact: Linux capabilities only ever live inside a container's own
`securityContext.capabilities` field, there's no equivalent of a
`NetworkPolicy` or seccomp profile to generate on their own. This file is
a bare fragment for you to paste directly under that key — `drop: [ALL]`
always, `add` listing every capability observed (Kubernetes' own
short-name convention, `CAP_` prefix stripped). Since this is meant for
manual pasting, not something the kubelet loads directly, it keeps the
same `# confidence: ...` comment style as `profile.yaml`/
`networkpolicy.yaml`.

**Combine with `--restart` on an already-running container** (see
`e2e-demo.md` Finding 5): privilege-related capability checks
(dropping root via `setuid`/`setgid`, binding a privileged port,
`chown`ing files during init) cluster heavily at container startup.
Tracing a container that's already been running for a while will often
come back with nothing observed at all — not wrong, just nothing left to
see — the same startup blind spot `--restart` already exists to close
for filesystem access (Finding 2), applying here too.
