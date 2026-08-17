# Usage — the full training-run workflow

Split out of `README.md`, which now keeps only a short summary and links
here — this is the complete step-by-step reference for every `trace`
flag, moved out for the same reason `sequence-diagram.md`/`packages.md`
were: a "what is this and why would I use it" README shouldn't also be
the full flag reference.

**Assumes:** a Kubernetes cluster already up, `landlock-genprof`
installed (as `kubectl landlock-genprof` or standalone), and the
RBAC/CRDs applied — see [`../INSTALL.md`](../INSTALL.md) (or
[`test-environment.md`](test-environment.md) if you don't have a
cluster yet) if any of that isn't true yet.

## TL;DR

```bash
# Observe: trains on the target pod, publishes a reviewable proposal
kubectl landlock-genprof trace --pod nginx-demo --namespace default \
  --binary /usr/sbin/nginx --duration 60s

# Review: prints what was observed and what's available to apply
kubectl landlock-genprof review nginx-demo

# Apply: prompts for confirmation before touching the cluster
kubectl landlock-genprof apply-proposal nginx-demo
```

That's the whole loop — observe, review, apply. Everything else on this
page is reference material for it: every optional artifact `trace` can
also generate (Steps 5-14, see the table below), and the full 15-step
breakdown of what each command above actually does under the hood.

The full workflow runs in 15 steps: four mandatory (Steps 1-4), several
optional ones after that (one per `--*-out` flag, see the table below),
Step 12 (proposal publishing) mandatory again, and Step 15 (mandatory
human review) closing it out:

## Step 1 — Training run

The target pod runs normally for a defined duration (e.g. 60 s or longer, depending
on application complexity). The goal is to cover the most frequent code paths.

```
kubectl landlock-genprof trace \
  --pod nginx-demo \
  --namespace default \
  --binary /usr/sbin/nginx \
  --duration 60s \
  --out profile.yaml
```

**Why `--binary` is required, not auto-detected:** it's not just a label
on the output — the tracer uses it to filter observed events down to
that specific process's own `comm`
(`internal/tracer/trace_linux.go`'s `commFromBinaryPath`), so a
`kubectl exec`/sidecar/debug session sharing the pod's namespaces
doesn't contaminate the profile (see `e2e-demo.md` Finding 1, the real
bug this filter was added for). Auto-detecting it (e.g. via
`/proc/<pid>/exe`) was considered and set aside: it would need either a
new `pods/exec` RBAC grant (nothing in this project needs cluster
`exec` access today) or assuming PID 1 is the right process, which
breaks for the common case of an entrypoint script or `tini`/`dumb-init`
wrapper that then execs the real binary.

## Step 2 — Syscall capture (Tracer)

During the training run, `landlock-genprof` captures the pod's system calls via
**[Inspektor Gadget](https://www.inspektor-gadget.io/) gadgets**:

| Gadget | Observed syscall | Output |
|---|---|---|
| `trace_open` | `openat`, `open` | `LANDLOCK_ACCESS_FS_READ_FILE`, `WRITE_FILE`, `EXECUTE` |
| `trace_tcp` (connect-only mode) | `connect` | `LANDLOCK_ACCESS_NET_CONNECT_TCP` (kernel ≥ 6.4) |
| `trace_bind` | `bind` | `LANDLOCK_ACCESS_NET_BIND_TCP` (kernel ≥ 6.4) |
| `trace_exec` | `execve`, `execveat` | `LANDLOCK_ACCESS_FS_EXECUTE` |
| `advise_seccomp` | every syscall issued by the container | seccomp profile (`--seccomp-out`, see step 8) |
| `trace_capabilities` | `cap_capable()` checks | Linux capabilities fragment (`--capabilities-out`, see step 9) |

`advise_seccomp` is Inspektor Gadget's own seccomp-profile advisor, reused
as-is rather than reimplemented — it already records a container's
syscalls and formats them into the target seccomp JSON shape. One
difference from the other four: it observes every process on the node
during the run, not just the target container (its own probe can't
filter earlier without losing the target's own startup syscalls) —
filtering to the target container happens at its own output stage.
`trace_capabilities` doesn't share this quirk: it filters in-kernel by
container the normal way, just like `trace_open`/etc.

Each captured event produces an `Event` object:

```go
type Event struct {
    Timestamp time.Time
    Syscall   string // "openat", "connect", "bind", "execve", or a bare syscall/capability name
    Path      string // file path, if applicable
    Port      int    // network port, if applicable
    Mode      string // "read", "write", "read_write", "exec", "egress", "ingress", "syscall", "capability"
}
```

## Step 3 — Policy synthesis

Events are aggregated by directory (to avoid per-file overfitting) and a
**confidence level** is calculated for each rule based on how consistently it
was observed across multiple runs:

| Level | Meaning |
|---|---|
| `high` | Observed consistently on every run — reliable rule |
| `medium` | Observed on multiple runs, but with inconsistencies |
| `low` | Observed only once — must be reviewed before deployment |

## Step 4 — YAML generation

The profile is exported in PodLock's `LandlockProfile` CRD format:

```yaml
apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
  namespace: default
spec:
  profilesByContainer:
    nginx:
      "/usr/sbin/nginx":
        readExec:
          - /lib
          - /lib64
        readOnly:
          - /usr/share/nginx        # confidence: high
        readWrite:
          - /tmp                    # confidence: high
          - /var/cache/nginx/proxy  # confidence: low — review before prod
```

## Optional steps

The core workflow above (Steps 1–4) is enough to get a usable profile.
Each row below adds one more artifact from the same training run —
pick only what you need.

| Flag | What it adds | Details |
|---|---|---|
| `--network-out` | Kubernetes `NetworkPolicy` from observed connections | [Network policy generation](usage/network-policy.md) |
| `--restart` | Restarts target to capture startup-only activity | [Target restart](usage/target-restart.md) |
| `--history` | Cross-run confidence via persisted training history | [Multi-run history](usage/multi-run-history.md) |
| `--seccomp-out` | Plain seccomp JSON profile | [Seccomp profile](usage/seccomp-profile.md) |
| `--capabilities-out` | Linux capabilities fragment | [Capabilities fragment](usage/capabilities-fragment.md) |
| `--security-context-out` | Composed `securityContext` (capabilities + seccomp ref) | [Composed securityContext](usage/composed-security-context.md) |
| `--report-out` | Single Markdown review report, all domains combined | [Unified report](usage/unified-report.md) |
| *(always on)* | Publishes `SecurityProfileProposal` to the cluster | [Proposal publishing](usage/proposal-publishing.md) |
| `--patched-manifest-out` | Ready-to-apply manifest with `securityContext` merged | [Patched manifest](usage/patched-manifest.md) |
| `--seccomp-profile-out` | `SeccompProfile` CR for security-profiles-operator | [SeccompProfile resource](usage/seccompprofile-resource.md) |

## Step 15 — Mandatory human review

**`landlock-genprof` never deploys a profile automatically.**
The generated YAML is a starting point for human review, not a final result.
The `Confidence` field per rule makes explicit what is reliable and what requires
attention. See [`threat-model.md`](threat-model.md) for the recommended
validation methodology.

**Applying a `LandlockProfile` alone has no effect.** PodLock's admission
webhook matches a running pod to a `LandlockProfile` object via a label
on the *pod* — `podlock.kubewarden.io/profile: <profile-name>` — not by
anything embedded in the CRD itself. `landlock-genprof trace` prints the
exact `kubectl label` command to run after `kubectl apply`-ing the
generated profile.

### Reviewing and applying, with a confirmation step

```bash
kubectl landlock-genprof apply-proposal nginx-demo -n default
```

Fetches the `SecurityProfileProposal` and prints the same full
`WORKLOAD SECURITY REVIEW` block the standalone `review` command shows
(container, binary, generated-at, history-used, per-artifact
availability, PodLock label status) — not just an artifact name list —
then lists exactly which artifacts it's about to apply and asks
**`Apply these to the cluster? [y/N]`** before touching anything. The
CLI-native form of the "mandatory human review" above: a decision made
with the same context a standalone `review` would give, not just the
YAML skimmed by hand before running `kubectl apply`. `--yes`/`-y` skips
the prompt for CI/scripted use (still prints the summary and what it
applied). `internal/k8s.Apply` creates-or-updates each artifact directly
via the Kubernetes API (not a `kubectl apply -f` subprocess) — same
create-then-fallback-to-update logic as `SecurityProfileProposal`
publishing itself (Step 12 above).

**One artifact failing doesn't stop the others.** PodLock is first in
apply order, so on a cluster that only has some of PodLock/CNI/SPO
installed — this project's own `kind` reference environment has none of
them — stopping at the first failure would mean artifacts that *would*
succeed (e.g. a plain builtin `NetworkPolicy`) never even get attempted.
Each artifact is applied independently; failures are printed per artifact
as they happen, and the command exits non-zero if any failed, but only
after trying every one.

**Patched Manifest is opt-in — `--restart`.** Applying it recreates the
target pod (a bare pod's securityContext is immutable in place, see
below) referencing whatever the enforcement side is supposed to provide
— a `localhostProfile` path only SPO ever writes, a
`podlock.kubewarden.io/profile` label only PodLock's admission webhook
ever acts on. If that operator isn't actually ready yet, applying the
patched manifest breaks the pod outright (`cannot load seccomp profile
...: no such file or directory`), confirmed live — and unlike the other
three artifacts (inert until their own operator reconciles them, or a
live-updatable resource), this is the one that force-restarts a
possibly-still-working pod. So it's the only artifact left out of an
apply by default; pass `--restart` to include it if it's available:

```bash
kubectl landlock-genprof apply-proposal nginx-demo -n default --restart
```

Without `--restart`, a proposal with a Patched Manifest available prints
`Leaving out 1 artifact(s) that would restart the target pod — pass
--restart to include:` and applies everything else. Confirmed live: the
version of this where restarting was the default and skipping it opt-out
(`--skip=patched-manifest`) let repeated `apply-proposal` runs
force-restart a pod whose enforcement side wasn't ready yet, with no
single moment where restarting it was an actual decision — a 73-minute,
15-restart `CrashLoopBackOff`.

**Leaving other artifacts out — `--skip`.** For the three artifacts that
*don't* restart anything, `--skip` leaves specific ones out of a given
apply instead:

```bash
kubectl landlock-genprof apply-proposal nginx-demo -n default --skip=podlock
# or several, comma-separated or repeated:
kubectl landlock-genprof apply-proposal nginx-demo -n default --skip=podlock,networkpolicy
```

Valid values: `podlock`, `networkpolicy`, `patched-manifest`,
`spo-seccompprofile` — `patched-manifest` is accepted for
completeness/explicitness but redundant, since it's already left out
unless `--restart` is passed. An unrecognized value is rejected before
the command ever connects to the cluster, rather than silently applying
everything.

**Prerequisites — different from everything else in this file.** Every
other command on this page runs under whatever RBAC `deploy/rbac*.yaml`
grants the tracer's ServiceAccount, deliberately read-only/generation-only
(see `docs/threat-model.md`). `apply-proposal` is the one exception: it
runs under **your own** `kubectl` identity (`internal/k8s.RestConfig()`
falls back to your local kubeconfig whenever it isn't running in-cluster,
which is the normal case for a human running this from their terminal),
and needs whatever create/update permissions *you* already have for the
resource kinds a given proposal actually contains:

- `NetworkPolicy` (`networking.k8s.io`) — builtin, no extra operator needed.
- `LandlockProfile` (`podlock.kubewarden.io`) — needs
  [PodLock](https://github.com/flavio/podlock) installed in the cluster;
  applying fails with a clear "could not find the requested resource"
  error otherwise. **Not installed by `hack/init-vm.sh`** — and PodLock's
  own docs advise against this project's `kind`-based reference cluster
  entirely, see [`enforcement-prerequisites.md`](enforcement-prerequisites.md).
- `SeccompProfile` (`security-profiles-operator.x-k8s.io`) — needs
  security-profiles-operator installed, same failure mode if it isn't.
  **Not installed by `hack/init-vm.sh` either** — opt-in on purpose (see
  [`enforcement-prerequisites.md`](enforcement-prerequisites.md) for the
  concrete Helm install commands actually tested against this project's
  reference cluster, not just a link to SPO's own upstream docs).
- The patched manifest — `Pod`/`Deployment`/`StatefulSet`/`DaemonSet`,
  builtin, whichever the target actually is. A bare `Pod` (no owner) is
  deleted and recreated, not patched in place — confirmed live:
  `kubectl apply`/a generic `Update` both hit
  `Forbidden: pod updates may not change fields other than ...` on most
  Pod fields, including `securityContext`. Deployment/StatefulSet/DaemonSet
  don't have this restriction — those update in place and roll out
  normally.

This project deliberately doesn't grant the tracer's ServiceAccount write
access to any of these — doing so would meaningfully widen its blast
radius for a capability only a human approving changes needs, not the
tracer itself. See [`../deploy/rbac.yaml`](../deploy/rbac.yaml) and
siblings for the RBAC this project *does* provision, and why each grant
stops where it does.

From a local clone instead: `make export-proposal PROPOSAL=<name>` then
`make apply-proposal PROPOSAL=<name>` do the same export-then-`kubectl
apply -f` — no preview, no prompt. Kept for contributors and local
testing; `apply-proposal` above is the reviewed path for actually using
the tool.
