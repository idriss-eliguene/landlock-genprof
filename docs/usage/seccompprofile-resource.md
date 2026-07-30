# Step 14 — Optional SeccompProfile custom resource (`--seccomp-profile-out`)

`securityContext.seccompProfile.localhostProfile` can never carry a
seccomp profile's content inline — only a path Kubernetes resolves by
asking the **kubelet** to look on **that node's own local filesystem**,
never from any API object directly. That means neither the plain
`seccomp.json` (Step 8) nor a hand-rolled `ConfigMap` actually
closes the loop: something still has to copy the file onto every node.

[security-profiles-operator (SPO)](https://github.com/kubernetes-sigs/security-profiles-operator)
is the real, upstream Kubernetes-native answer: its own controller/
DaemonSet watches `SeccompProfile` objects and materializes them onto
every node's seccomp directory automatically. Pass `--seccomp-profile-out`
to generate one:

```bash
kubectl apply -f nginx-demo-seccompprofile.yaml
```

```yaml
apiVersion: security-profiles-operator.x-k8s.io/v1beta1
kind: SeccompProfile
metadata:
  name: nginx-demo
  namespace: default
spec:
  defaultAction: SCMP_ACT_ERRNO
  architectures: [SCMP_ARCH_X86_64]
  syscalls:
    - names: [accept4, capget, capset, chdir, epoll_wait, futex, openat, read, write]
      action: SCMP_ACT_ALLOW
```

(`capget`/`capset`/`chdir`/`futex` explained in Step 8 above —
always included, none is something the traced binary itself calls.)

`spec.defaultAction`/`architectures`/`syscalls[].names`/`.action` mirror
`pkg/seccomp.Profile`'s own fields exactly (confirmed against SPO's own
Go source) — this is the same data as `seccomp.json`, just wrapped as a
directly appliable Kubernetes object instead of a file a human has to
copy by hand.

**Requires SPO actually installed in the cluster** — applying this
manifest alone does nothing without SPO's controller running to
reconcile it. Once it does, SPO writes the profile to
`/var/lib/kubelet/seccomp/operator/<namespace>/<name>.json` on every
node and exposes that same path as `status.localhostProfile` — the
`operator/<namespace>/<pod>.json` value `--security-context-out`/
`--patched-manifest-out`/the `SecurityProfileProposal` all already
reference (Step 10), computed ahead of time since this tool never
waits for SPO's own reconciliation to run — **confirmed live** against a
real reconciliation (`kubectl get seccompprofile <name> -o yaml` →
`status.localhostProfile`); the namespace segment used to be missing
here, which broke every target pod once its patched manifest was
actually applied (containerd refuses to start a container whose
referenced `localhostProfile` doesn't resolve to a real file — SPO never
writes to the un-namespaced path this tool used to assume). See
[`../enforcement-prerequisites.md`](../enforcement-prerequisites.md) for
installing SPO itself.
