# SeccompProfile resource output (`--seccomp-profile-out`)

`securityContext.seccompProfile.localhostProfile` can never carry a
seccomp profile's content inline — only a path Kubernetes resolves by
asking the **kubelet** to look on **that node's own local filesystem**,
never from any API object directly. That means neither the plain
`seccomp.json` (see [internal seccomp output](seccomp-profile.md)) nor a hand-rolled `ConfigMap` actually
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
apiVersion: security-profiles-operator.x-k8s.io/v1
kind: SeccompProfile
metadata:
  # Deterministic and cluster-unique: SeccompProfile is cluster-scoped from
  # SPO v0.9.0 on, so there is no namespace here and the name encodes
  # (namespace, pod, container). See docs/adr/0008.
  name: lg-v1-nginx-demo-<hash>
  annotations:
    landlockgenprof.io/managed-by: landlock-genprof
    landlockgenprof.io/seccomp-source: internal
spec:
  defaultAction: SCMP_ACT_ERRNO
  architectures: [SCMP_ARCH_X86_64]
  syscalls:
    - names: [accept4, capget, capset, chdir, epoll_wait, futex, openat, read, write]
      action: SCMP_ACT_ALLOW
```

(`capget`/`capset`/`chdir`/`futex` are explained in the internal seccomp page —
always included, none is something the traced binary itself calls.)

`spec.defaultAction`/`architectures`/`syscalls[].names`/`.action` mirror
`pkg/seccomp.Profile`'s own fields exactly (confirmed against SPO's own
Go source) — this is the same data as `seccomp.json`, just wrapped as a
directly appliable Kubernetes object instead of a file a human has to
copy by hand.

**Requires SPO actually installed in the cluster** — applying this
manifest alone does nothing without SPO's controller running to
reconcile it. Once it does, SPO writes the profile to
`/var/lib/kubelet/seccomp/operator/<name>.json` on every
node and exposes that same path as `status.localhostProfile` — the
`operator/<name>.json` value `--security-context-out`/
`--patched-manifest-out`/the `SecurityProfileProposal` all already
reference, computed ahead of time. During governed workload binding,
`apply-proposal --restart` waits for SPO reconciliation and rechecks the
realized identity as required by ADR-0007 — **confirmed live** against a
real reconciliation (`kubectl get seccompprofile <name> -o yaml` →
`status.localhostProfile`); the namespace segment used to be missing
here, which broke every target pod once its patched manifest was
actually applied (containerd refuses to start a container whose
referenced `localhostProfile` doesn't resolve to a real file — SPO never
writes to the un-namespaced path this tool used to assume). See
[`../enforcement-prerequisites.md`](../enforcement-prerequisites.md) for
installing SPO itself.

Applying this standalone file directly is not governed authorization. The
normal path includes it in `SecurityProfileProposal`, binds it into
`CandidateDigest`, requires explicit approval of that digest, and applies it
through `apply-proposal`.
