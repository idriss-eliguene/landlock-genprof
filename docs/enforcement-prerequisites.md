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

## security-profiles-operator (SPO) — opt-in

Only needed if you want a generated `{pod}-seccompprofile.yaml` to
actually be materialized as a `localhostProfile` on the node — not
needed to generate it, review it, or even `kubectl apply` it (the object
is created either way, just inert without SPO's controller).

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

**Known `kind`-specific caveat, not yet independently verified on this
project's own VM setup:** nested container environments like `kind` may
need a custom `/proc` path for SPO's daemon to read host process info
correctly. SPO's own docs mention patching this via:

```bash
kubectl -n security-profiles-operator patch spod spod --type=merge -p '{"spec":{"procPath":"/proc"}}'
```

Treat this one command as unverified — check
[SPO's own `installation-usage.md`](https://github.com/kubernetes-sigs/security-profiles-operator/blob/main/installation-usage.md)
for the current, exact guidance before relying on it, and update this
doc once someone has actually confirmed it live (see this repo's own
"confirmed live" convention in `docs/roadmap.md` — don't claim it here
until it's true).

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

- Generating `profile.yaml` and `kubectl apply`-ing it (creating the
  `LandlockProfile` object) works fine on `kind` — the object exists,
  is inspectable, is exactly what a human reviews.
- Actually seeing PodLock's operator arm Landlock on the target pod at
  runtime does not work reliably here. Don't stage or claim that in a
  demo (see `demo/script.md`) unless you've set up a real VM-per-node
  environment and verified it live.

**If you actually need to verify live PodLock enforcement:** follow
PodLock's own quickstart with [Lima](https://lima-vm.io/) instead of
`kind` — out of scope for this repo's own setup scripts, since it would
mean maintaining a second, incompatible reference environment alongside
the `kind`-based one everything else here assumes.
