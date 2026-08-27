# Enforcement prerequisites

`landlock-genprof` generates profiles; it never enforces them itself (see
[`architecture.md`](architecture.md) §1 — "this tool's job ends" at
`kubectl apply`). Three different external controllers are what actually
enforce what gets generated. This doc exists because none of that was
documented anywhere before — someone could get the CLI itself fully
working and still hit a wall trying to see real enforcement, with no
pointer to why.

| Generated artifact | Enforced by | Set up by this repo? |
|---|---|---|
| `profile.yaml` (LandlockProfile) | PodLock operator (Kubewarden) | No — see the limitation below |
| `{pod}-networkpolicy.yaml` | Any CNI that implements NetworkPolicy | **Yes** — `hack/init-vm.sh` installs Cilium |
| `{pod}-seccompprofile.yaml` (SeccompProfile CR) | security-profiles-operator (SPO) | No — opt-in, see below |

## CNI (NetworkPolicy enforcement)

Already handled: `hack/init-vm.sh` creates the `kind` cluster with the
default CNI (kindnet) disabled and installs
[Cilium](https://cilium.io/) instead — kindnet does not implement
NetworkPolicy at all, so a generated `networkpolicy.yaml` would apply
successfully and enforce nothing, silently. Nothing extra to do if you
ran the current version of that script.

## security-profiles-operator (SPO) — opt-in, and not installed by default

Only needed if you want a generated `{pod}-seccompprofile.yaml` to
actually be materialized as a `localhostProfile` on the node. **Not**
opt-in for `kubectl apply`/`apply-proposal` themselves, though — confirmed
live: with nothing SPO-related installed (this project's reference `kind`
cluster's default state), applying the `SeccompProfile` artifact fails
outright with `the server could not find the requested resource` — no
CRD registered, not a graceful "created but inert" fallback. That only
holds if SPO's CRD specifically has been installed without its
controller, an unusual, deliberate scenario — not what "SPO not set up"
means for anyone following this doc from scratch.

This project targets **SPO v1.0.0**, which serves `SeccompProfile` at
`security-profiles-operator.x-k8s.io/v1` and, since v0.9.0, **cluster-scoped**.
That API shape lives in `internal/spobackend`, which is the single
authority on it — the exporter, the generic apply path and ADR-0007's
readiness gate all ask that package rather than restating any of it.

`test/e2e/install-spo.sh` performs exactly the steps below and is what the
SPO Interop E2E workflow runs.

Requires cert-manager first — SPO's CRDs carry
`cert-manager.io/inject-ca-from` annotations, so its webhook never becomes
ready without it:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml
kubectl -n cert-manager wait --for=condition=Available deployment --all --timeout=300s
```

Then SPO itself. The v1.0.0 manifest is self-contained: it creates the
`security-profiles-operator` namespace **with the privileged
`pod-security.kubernetes.io/*` labels already set**, and pins its image to
`registry.k8s.io/security-profiles-operator/security-profiles-operator:v1.0.0`.
No namespace labelling step and no image overrides are needed.

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/security-profiles-operator/v1.0.0/deploy/operator.yaml
kubectl -n security-profiles-operator rollout status deployment/security-profiles-operator --timeout=300s
```

The `spod` DaemonSet is **not** in that manifest — the operator creates it
from a `SecurityProfilesOperatorDaemon` resource after it starts, so wait
for it separately once it appears:

```bash
kubectl -n security-profiles-operator rollout status daemonset/spod --timeout=300s
```

Verify the API this project actually targets, rather than assuming:

```bash
kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io \
  -o jsonpath='{.spec.scope}{" "}{.spec.versions[*].name}'
# expect: Cluster v1
```

### Historical: the v0.8.4 workarounds are obsolete

Earlier revisions of this document pinned SPO **v0.8.4** — the last release
serving a *namespaced* `SeccompProfile` — and carried two workarounds for
bugs in that release's Helm chart. Both were verified against the v1.0.0
manifest and **no longer apply**:

1. The chart's `spoImage.tag` defaulted to `latest` against a *staging*
   registry, so the operator crashed on startup. `deploy/operator.yaml`
   pins a real `registry.k8s.io` image at the release version.
2. The `spod` metrics sidecar was hardcoded to
   `gcr.io/kubebuilder/kube-rbac-proxy`, a discontinued registry path,
   producing `ImagePullBackOff` with no chart value to override it. v1.0.0
   contains no `kube-rbac-proxy` or `RELATED_IMAGE_RBAC_PROXY` reference
   at all.

A third historical note, `clock_gettime` missing from v0.7.1's own
self-applied profile, was fixed upstream in v0.8.3 and is likewise not a
concern here.

Do not reintroduce any of these against v1.0.0.

### What SPO interoperability currently demonstrates

Demonstrated (SPO Interop E2E, `test/e2e/spo-interop.sh`): landlock-genprof
generates a native cluster-scoped `SeccompProfile` v1, governs it through
review and digest-bound approval, applies it via the governed apply path,
waits for **real SPO reconciliation** (`status.localhostProfile`), verifies
the live enforcement content still equals the approved content, and binds
the workload only after the profile is ready.

**Current demonstrated boundary:**

- **SPO observation feeding landlock-genprof.** A real `ProfileRecording` is
  imported explicitly as derived policy; merged-provenance and optional v1
  coverage behavior are demonstrated by the real-node run documented in
  [`PROGRESS.md`](PROGRESS.md).
- **Behavioral syscall enforcement.** Run `32561123023` proved the bounded
  candidate experiment: `getpid` succeeded under the approved profile, while
  naturally absent `getpriority` succeeded in the unconfined control and
  returned `EPERM` after governed application. This does not claim universal
  least privilege or complete Seccomp verification.

## PodLock — not supported on this project's reference environment

**PodLock's own documentation explicitly advises against this project's
entire `kind`-based setup:**

> "It is not recommended to run PodLock with clusters spawned by kind,
> the nodes should be running inside a VM or physical machine with
> Landlock support."
> — [PodLock quickstart](https://flavio.github.io/podlock/podlock-docs/v0.1.0/quickstart.html)

`kind` nodes are Docker containers, not separate VMs — exactly the setup
PodLock's docs warn about. (`minikube` is explicitly ruled out too, for
the same underlying reason: its VM doesn't support Landlock.) This is
specific to **PodLock's own operator** — it is not a limitation of
`landlock-genprof` or of Landlock itself: the tracer's own use of
Landlock-adjacent kernel features works fine on this project's `kind`
setup (confirmed repeatedly, see `docs/roadmap.md`/`docs/e2e-demo.md`),
because those syscalls hit the VM's real host kernel directly. It's
specifically PodLock's controller that has trouble with nodes that are
themselves containers.

**What this means in practice:**

- Generating `profile.yaml` always works on `kind` — it's a local file,
  no cluster object involved.
- `kubectl apply`/`apply-proposal` applying it as a `LandlockProfile`
  object does **not** work on this project's reference `kind` setup by
  default — confirmed live: fails with `the server could not find the
  requested resource`, since nothing here installs PodLock's CRD at all.
  It would only succeed if you'd separately installed PodLock's CRD
  (even without its controller/webhook) — not something this repo or its
  docs currently walk through, on purpose, given the limitation below.
- Actually seeing PodLock's operator arm Landlock on the target pod at
  runtime does not work reliably on `kind` even with PodLock installed.
  Don't stage or claim either of the above in a demo (see
  `demo/script.md`) unless you've set up a real VM-per-node environment
  and verified it live.

**If you actually need to verify live PodLock enforcement:** follow
PodLock's own quickstart with [Lima](https://lima-vm.io/) instead of
`kind` — out of scope for this repo's own setup scripts, since it would
mean maintaining a second, incompatible reference environment alongside
the `kind`-based one everything else here assumes.
