# Usage — the full training-run workflow

Split out of `README.md`, which now keeps only a short summary and links
here — this is the complete step-by-step reference for every `trace`
flag, moved out for the same reason `sequence-diagram.md`/`packages.md`
were: a "what is this and why would I use it" README shouldn't also be
the full flag reference.

The full workflow runs in five core steps, with several optional
sub-steps at Step 4 (one per `--*-out` flag):

## Step 1 — Training run

The target pod runs normally for a defined duration (e.g. 60 s or longer, depending
on application complexity). The goal is to cover the most frequent code paths.

```
landlock-genprof trace \
  --pod nginx-demo \
  --namespace default \
  --binary /usr/sbin/nginx \
  --duration 60s \
  --out profile.yaml
```

## Step 2 — Syscall capture (Tracer)

During the training run, `landlock-genprof` captures the pod's system calls via
**[Inspektor Gadget](https://www.inspektor-gadget.io/) gadgets**:

| Gadget | Observed syscall | Output |
|---|---|---|
| `trace_open` | `openat`, `open` | `LANDLOCK_ACCESS_FS_READ_FILE`, `WRITE_FILE`, `EXECUTE` |
| `trace_tcpconnect` | `connect` | `LANDLOCK_ACCESS_NET_CONNECT_TCP` (kernel ≥ 6.4) |
| `trace_bind` | `bind` | `LANDLOCK_ACCESS_NET_BIND_TCP` (kernel ≥ 6.4) |
| `trace_exec` | `execve`, `execveat` | `LANDLOCK_ACCESS_FS_EXECUTE` |
| `advise_seccomp` | every syscall issued by the container | seccomp profile (`--seccomp-out`, see step 4) |
| `trace_capabilities` | `cap_capable()` checks | Linux capabilities fragment (`--capabilities-out`, see step 4) |

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

## Step 4bis — Optional NetworkPolicy generation

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

## Step 4ter — Optional target restart (`--restart`)

Resources a process opens once at startup (a pid file, a log fd) and
keeps writing to are invisible to a trace attached to an already-running
container — `trace_open` only observes `openat()`, not later `write()`s
on an already-open fd. Pass `--restart` to have the CLI restart the
target right before observing it, attaching the tracer *first* in every
case so the restart's startup activity is actually captured — delete
+recreate for a bare pod, or the same rollout-restart mechanism `kubectl
rollout restart` uses for a Deployment/StatefulSet/DaemonSet-owned one.

The tracer is pre-targeted differently depending on whether the owner
keeps a stable pod name: a bare pod or StatefulSet keeps its name across
the restart, so the tracer is pre-attached by that name directly. A
Deployment/DaemonSet's replacement gets an unpredictable new name, so
the tracer is instead pre-attached by the **workload's own label
selector** — which also means the generated profile is identified by the
*workload's* name (e.g. `nginx-ds`), not one ephemeral pod, and the
PodLock guidance patches the pod template (`kubectl patch deployment`/
`daemonset`) instead of labeling a single pod that a future rollout would
replace anyway.

Opt-in: it's disruptive to the running workload, and needs additional
RBAC beyond the base manifest — apply
[`../deploy/rbac-restart.yaml`](../deploy/rbac-restart.yaml) first.

## Step 4quater — Optional multi-run history (`--history`)

`Confidence` is meant to reflect how many separate training runs
observed an access ("seen on every run" vs "seen once out of 5 runs"),
but a single `trace` run has no way to know that — it can only measure
how many times something was seen *within* that one run. Pass
`--history` to persist a `TrainingHistory` custom resource
(`internal/history`, no controller — the CLI reads/writes it directly)
that accumulates across every `--history` run for the same
container/binary, so `Confidence` can finally be computed from the real
ratio. Requires the CRD and additional RBAC, applied once:
[`../deploy/crd-traininghistory.yaml`](../deploy/crd-traininghistory.yaml),
[`../deploy/rbac-history.yaml`](../deploy/rbac-history.yaml). Query the result
directly: `kubectl get traininghistory <container>-<binary-basename> -o
yaml`. `profile.yaml`/`networkpolicy.yaml`/`capabilities.yaml` themselves
show it too — every path/port/capability gets a trailing `# confidence:
...` comment (see Step 4), and with `--history` that comment reflects the
real cross-run ratio instead of the single-run estimate used without it.
`seccomp.json` (Step 4quinquies) can't carry a comment — its confidence
is printed to stdout instead.

## Step 4quinquies — Optional seccomp profile generation (`--seccomp-out`)

Pass `--seccomp-out` to also generate a seccomp profile from the same
training run (skipped if no syscalls were observed), via Inspektor
Gadget's own `advise_seccomp` gadget (see Step 2's gadget table):

```json
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": ["SCMP_ARCH_X86_64"],
  "syscalls": [
    {
      "names": ["accept4", "epoll_wait", "openat", "read", "write"],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
```

Deliberately plain JSON, not YAML with a `# confidence: ...` comment like
the other two outputs: this file is loaded directly by the kubelet/
container runtime (referenced via a pod's
`securityContext.seccompProfile.localhostProfile`, never `kubectl
apply`d), so it has to stay valid, comment-free JSON. Instead, the CLI
prints the syscalls not yet confirmed across multiple `--history` runs to
stdout after writing the file — on a single run without `--history`, that
means every syscall, since `advise_seccomp` reports one deduplicated set
per run rather than a per-occurrence count, so `Confidence` can only ever
be `low` until `--history` accumulates more runs.

This one is worth taking seriously before enforcing: a missing syscall
doesn't just narrow access like an overly-strict `NetworkPolicy` would —
it breaks the container outright. Prefer `--history` over a single run
before deploying it.

## Step 4sexies — Optional Linux capabilities fragment (`--capabilities-out`)

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

## Step 4septies — Optional composed securityContext (`--security-context-out`)

Pass `--security-context-out` to also generate a composed
`securityContext` fragment combining the same capabilities data from
Step 4sexies with a *reference* to the seccomp profile — generated
whenever syscalls were observed, independent of whether `--seccomp-out`/
`--seccomp-profile-out` (Step 4quinquies/4undecies) were also passed
this run:

```yaml
capabilities:
  add:
    - NET_BIND_SERVICE   # confidence: high
  drop:
    - ALL
seccompProfile:
  type: Localhost
  localhostProfile: operator/nginx-demo.json
```

This is **not** a merge of the seccomp and capabilities exporters —
`seccomp.json`/`capabilities.yaml` are still generated exactly as
before, independently. A seccomp profile has to ship as its own file for
the kubelet to load (`localhostProfile` only ever takes a path
reference, never inline content), so merging the files themselves
wouldn't actually reduce anything — it'd just add indirection. This flag
adds a third, composed *view* on top, for the common case of wanting
both in one place to paste under a container's `securityContext:` key.
`localhostProfile` always follows security-profiles-operator (SPO)'s own
`operator/<pod>.json` naming convention — see Step 4undecies
for why, and for the flag that actually generates the object at that
path.

**Deliberately does not infer** `privileged`, `allowPrivilegeEscalation`,
`runAsNonRoot`, `readOnlyRootFilesystem`, or `runAsUser` — nothing in
this project observes any of them today, and guessing "safe defaults"
regardless of what was actually seen would contradict the project's own
positioning: observe, don't guess.

## Step 4octies — Optional unified review report (`--report-out`)

Pass `--report-out` to also generate one Markdown report combining all
four observed domains — filesystem, network, syscalls, capabilities —
for a single review pass, instead of up to five separate files:

```markdown
# Security Profile Review — nginx-demo

- **Generated:** 2026-07-24T10:00:00Z
- **Namespace/Container:** default/nginx
- **Binary:** /usr/sbin/nginx
- **Training duration:** 1m0s
- **--history used:** no — Confidence below is internal/policy's single-run proxy, not a real cross-run ratio

## Filesystem
| Path | Permissions | Confidence |
|---|---|---|
| `/etc/nginx` | read | high |

## Capabilities
No capability checks observed. Capability checks cluster heavily at
container startup — if this container was already running before this
trace started, there may be nothing left to observe — see
`e2e-demo.md` Finding 5 and re-run with `--restart`.

## Review checklist
- [ ] Re-run with `--history` a few times before trusting any `low`/`medium` entry above.
- [ ] Re-run with `--restart` — capabilities and/or syscalls came back empty...
```

Unlike every other `--*-out` flag, this one is **never skipped** when
passed, even if a domain observed nothing at all — an empty domain is
itself useful review content (usually the startup blind spot from Step
4ter/Finding 5, worth surfacing directly rather than leaving the reader
to rediscover it). It also works **standalone**, independent of the
other `--*-out` flags: `internal/policy.Synthesize` already populates
all four IR domains every run regardless of which flags were passed
(all six gadgets always run), so the report shows the real data
directly — and additionally links to any of the other files that were
also generated this same run.

## Step 4nonies — Proposal publishing (mandatory)

Every `trace` run publishes its generated multi-domain profile as a
`SecurityProfileProposal` custom resource — stored as a cluster object
instead of only local files, reviewable via `kubectl`/GitOps. This isn't
an opt-in flag: it's the primary reviewable artifact this tool produces,
so a run fails outright if it can't publish (missing CRD or RBAC below)
rather than silently degrading to local files only. See
[`../examples/nginx-generated-proposal.yaml`](../examples/nginx-generated-proposal.yaml)
for a complete example.

```bash
kubectl get securityprofileproposal nginx-demo -o yaml

# Product-facing review summary
go run ./cmd/landlock-genprof review nginx-demo
```

Each field is the **exact rendered content** of the corresponding local
file — `spec.podLock` is the full, real `profile.yaml`
(`apiVersion`/`kind`/`metadata`/`spec` included), `spec.networkPolicy`
the full `networkpolicy.yaml`, `spec.patchedManifest` the full
`<identity>-patched.yaml` (Step 4decies below) — the live owner's (or
bare pod's) complete manifest with the generated `securityContext`
already merged in, not the bare fragment `--security-context-out`
produces, `spec.spoSeccompProfile` the full `<pod>-seccompprofile.yaml`
(Step 4undecies below) — a security-profiles-operator SeccompProfile
custom resource, the sole seccomp-related field (its own `spec.syscalls`
already carries the same data a raw `spec.seccomp` field would, so
there's no separate copy to keep in sync). Copy any of them directly out
of `kubectl get -o yaml` and use as-is (`kubectl apply -f -` for all
four).

`spec.patchedManifest`'s `securityContext.seccompProfile.localhostProfile`
always references SPO's own `operator/<pod>.json` naming
convention whenever `spec.spoSeccompProfile` is non-empty — see Step
4undecies for why a plain filename isn't enough and what applying
`spec.spoSeccompProfile` actually does.

This is the **first slice of a larger evidence/proposal/approved-policy
model**: `TrainingHistory` (`--history`, Step 4quater) is the evidence
stage, `SecurityProfileProposal` is the proposal stage — both are plain
CRUD, no controller. An eventual approved-policy stage
(`WorkloadSecurityProfile`) and an enforcement operator to keep it from
drifting are **not** part of this — that's real controller-runtime work,
deliberately out of scope for now. The object's name is the target pod
(overwritten on every re-run, not accumulated — a proposal is the
*latest* recommendation, same as the local files). Requires the CRD and
additional RBAC, applied once:
[`../deploy/crd-securityprofileproposal.yaml`](../deploy/crd-securityprofileproposal.yaml),
[`../deploy/rbac-proposal.yaml`](../deploy/rbac-proposal.yaml).

## Step 4decies — Optional ready-to-apply patched manifest (`--patched-manifest-out`)

`--security-context-out`'s fragment (Step 4septies) still needs manual
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
build a local file): [`../deploy/rbac-patched-manifest.yaml`](../deploy/rbac-patched-manifest.yaml).

The same content is embedded in `spec.patchedManifest` of the
`SecurityProfileProposal` (Step 4nonies) on every run regardless of
whether `--patched-manifest-out` was passed — that flag only controls
whether it's *also* written as a local file.

## Step 4undecies — Optional SeccompProfile custom resource (`--seccomp-profile-out`)

`securityContext.seccompProfile.localhostProfile` can never carry a
seccomp profile's content inline — only a path Kubernetes resolves by
asking the **kubelet** to look on **that node's own local filesystem**,
never from any API object directly. That means neither the plain
`seccomp.json` (Step 4quinquies) nor a hand-rolled `ConfigMap` actually
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
  name: nginx-demo
  namespace: default
spec:
  defaultAction: SCMP_ACT_ERRNO
  architectures: [SCMP_ARCH_X86_64]
  syscalls:
    - names: [accept4, epoll_wait, openat, read, write]
      action: SCMP_ACT_ALLOW
```

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
`operator/<pod>.json` value `--security-context-out`/
`--patched-manifest-out`/the `SecurityProfileProposal` all already
reference (Step 4septies), computed ahead of time since this tool never
waits for SPO's own reconciliation to run. See
[`enforcement-prerequisites.md`](enforcement-prerequisites.md) for
installing SPO itself.

## Step 5 — Mandatory human review

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
