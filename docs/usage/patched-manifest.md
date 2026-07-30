# Step 13 — Optional ready-to-apply patched manifest (`--patched-manifest-out`)

`--security-context-out`'s fragment (Step 10) still needs manual
pasting into a real spec. Pass `--patched-manifest-out` instead to get a
complete, ready-to-apply manifest with the generated `securityContext`
already merged in:

```bash
kubectl apply -f nginx-ds-patched.yaml
```

**Important nuance**: most container-spec fields, including
`securityContext`, are immutable on an already-running Pod — you can't
`kubectl apply` a modified one directly onto a live Pod. So for a pod
owned by a Deployment/StatefulSet/DaemonSet, this fetches and patches
the **owner's** manifest, not the pod's own — applying it triggers a
rollout, the real supported way to change this (same reasoning
`--restart` already applies for which identity to target). Only for a
bare pod is the pod's own manifest the right target, and even then,
applying it means delete+recreate.

**Merges, never replaces**: only `capabilities`/`seccompProfile` are
ever set on the target container's `securityContext` — every other
field the live manifest already has (`runAsUser`, `runAsNonRoot`,
`readOnlyRootFilesystem`, ...) is preserved exactly as-is. This tool
only ever contributes what it actually generated. Requires additional
RBAC (read-only — this never writes to the cluster, only fetches to
build a local file): [`../../deploy/rbac-patched-manifest.yaml`](../../deploy/rbac-patched-manifest.yaml).

The same content is embedded in `spec.patchedManifest` of the
`SecurityProfileProposal` (Step 12) on every run regardless of
whether `--patched-manifest-out` was passed — that flag only controls
whether it's *also* written as a local file.
