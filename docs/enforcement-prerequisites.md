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
| `{pod}-networkpolicy.yaml` | Any CNI that implements NetworkPolicy | **Yes**, since this session — `hack/init-vm.sh` installs Cilium |
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

Requires cert-manager first:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml
kubectl --namespace cert-manager wait --for condition=ready pod -l app.kubernetes.io/instance=cert-manager
```

Then SPO itself:

```bash
kubectl create ns security-profiles-operator
kubectl label ns security-profiles-operator \
  app=security-profiles-operator \
  pod-security.kubernetes.io/audit=privileged \
  pod-security.kubernetes.io/enforce=privileged \
  pod-security.kubernetes.io/warn=privileged \
  app.kubernetes.io/managed-by=Helm \
  --overwrite=true
kubectl annotate ns security-profiles-operator \
  "meta.helm.sh/release-name"="security-profiles-operator" \
  "meta.helm.sh/release-namespace"="security-profiles-operator" \
  --overwrite

helm install security-profiles-operator \
  --namespace security-profiles-operator \
  https://github.com/kubernetes-sigs/security-profiles-operator/releases/download/v0.7.1/security-profiles-operator-0.7.1.tgz

kubectl get pods -n security-profiles-operator
```

**Confirmed live on this project's reference VM (VirtualBox + `kind`,
2026-07-30) — the chart above needs two fixes before any of its pods come
up healthy. Both are bugs in the v0.7.1 chart release itself, not this
project's setup:**

1. **`spoImage.tag` defaults to `latest` against the `k8s-staging-sp-operator`
   *staging* registry**, not a pinned release matching the chart version.
   The resulting operator crashes on startup (`cannot get resource
   "configmaps" ... is forbidden` — the staging image's RBAC expectations
   don't match what the v0.7.1 chart's templates grant). Fix: pin to the
   real release image, per v0.7.1's own GitHub release notes —

   ```bash
   helm upgrade security-profiles-operator \
     --namespace security-profiles-operator \
     https://github.com/kubernetes-sigs/security-profiles-operator/releases/download/v0.7.1/security-profiles-operator-0.7.1.tgz \
     --set spoImage.registry=registry.k8s.io \
     --set spoImage.repository=security-profiles-operator/security-profiles-operator \
     --set spoImage.tag=v0.7.1
   ```

2. **The `spod` DaemonSet's `metrics` sidecar is hardcoded (not a Helm
   value — an env var baked into `templates/deployment.yaml`) to
   `gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1`**, a registry path
   kubebuilder has since discontinued (`ImagePullBackOff`,
   `not found`). Fix: point it at the actively-maintained upstream
   instead (`quay.io/brancz/kube-rbac-proxy`) via `kubectl set env` —
   there's no chart value for this, so `helm upgrade` alone can't fix it:

   ```bash
   kubectl set env deployment/security-profiles-operator -n security-profiles-operator \
     RELATED_IMAGE_RBAC_PROXY=quay.io/brancz/kube-rbac-proxy:v0.22.1
   kubectl delete pod -n security-profiles-operator -l name=spod
   ```

   Version jump v0.13.1 → v0.22.1 (nothing in between still exists to pin
   to) worked without a flag-compatibility issue — the sidecar's own flags
   (`--secure-listen-address`, `--upstream`, `--tls-cert-file`, etc.) are
   unchanged across that range.

**After both fixes:** the three `security-profiles-operator` manager
pods and the three `security-profiles-operator-webhook` pods reach
`Running`/`1/1` reliably. The `spod` DaemonSet pod's main container did
**not** stabilize on this particular VM — it repeatedly failed its
startup API-group-resources discovery call
(`Get "https://10.96.0.1:443/api?timeout=32s": context deadline
exceeded"`) while every other pod on the same cluster (including SPO's
own manager/webhook pods) reaches the API server without issue. Not
resolved; suspected environment-specific (nested virtualization —
VirtualBox → Docker → `kind` — rather than a config problem), not chased
further since it doesn't block `apply-proposal`'s actual correctness
(already confirmed live for `NetworkPolicy`/the patched manifest). If you
hit this and resolve it, this is the place to record what worked.

**Known `kind`-specific caveat, still not independently verified beyond
the above:** nested container environments like `kind` may separately
need a custom `/proc` path for SPO's daemon to read host process info
correctly, per SPO's own docs:

```bash
kubectl -n security-profiles-operator patch spod spod --type=merge -p '{"spec":{"procPath":"/proc"}}'
```

Treat this one command as unverified — check
[SPO's own `installation-usage.md`](https://github.com/kubernetes-sigs/security-profiles-operator/blob/main/installation-usage.md)
for the current, exact guidance before relying on it.

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
