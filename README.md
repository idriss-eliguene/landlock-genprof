# landlock-genprof

> Version française pour les étudiants : [`README.etudiants.md`](README.etudiants.md).
> Student onboarding guide (French): [`HOW_TO_START.md`](HOW_TO_START.md).

Automatic [Landlock](https://landlock.io/) security profile generator for
Kubernetes, built on **observation** of a running pod (a "training run") rather
than manual rule authoring.

The name is a deliberate nod to `aa-genprof` / `aa-logprof` — the AppArmor
profile generation tools. Landlock has no equivalent yet.
`landlock-genprof` fills that gap.

> **Status:** initial scaffolding — implementation in progress with students.
> See [`docs/roadmap.md`](docs/roadmap.md) for milestones and task assignments.

---

## Table of contents

1. [The problem](#1-the-problem)
2. [Positioning — PodLock and the Kubewarden ecosystem](#2-positioning--podlock-and-the-kubewarden-ecosystem)
3. [How it works](#3-how-it-works)
4. [Technical stack](#4-technical-stack)
5. [Repository architecture](#5-repository-architecture)
6. [Prerequisites](#6-prerequisites)
7. [Quick start](#7-quick-start)
8. [Example output](#8-example-output)
9. [Team and task assignment](#9-team-and-task-assignment)
10. [Risk management](#10-risk-management)
11. [Milestones](#11-milestones)
12. [Threat model](#12-threat-model)
13. [Contributing](#13-contributing)
14. [License](#14-license)

---

## 1. The problem

**Landlock** is a Linux Security Module (LSM) introduced in kernel 5.13 that
allows processes to confine themselves to a subset of the filesystem and network,
**without requiring root privileges**. This is a rare and valuable property:
whereas AppArmor, SELinux, or seccomp require system-wide configuration by an
administrator, a process can arm Landlock itself.

### Why it is hard to use in practice

Writing a Landlock policy by hand requires **guessing in advance** every path,
directory, and port an application will ever need:

- **Too permissive** → the policy protects nothing (everything is allowed to
  avoid breaking the app)
- **Too restrictive** → the application breaks in production on a rare code path

The problem is compounded in a Kubernetes context:

- Landlock has **no native integration in containerd/runc**, so there is no
  standard K8s support (`securityContext` cannot arm Landlock)
- There is **no equivalent of `aa-genprof`** for Landlock, neither in the
  [Security Profiles Operator](https://github.com/kubernetes-sigs/security-profiles-operator)
  nor in [PodLock](https://github.com/flavio/podlock)

`landlock-genprof` addresses this gap: observe first, write the policy after.

---

## 2. Positioning — PodLock and the Kubewarden ecosystem

[PodLock](https://github.com/flavio/podlock) (part of the
[Kubewarden](https://www.kubewarden.io/) ecosystem) is the closest existing project.
It provides:

- A `LandlockProfile` CRD to describe pod restrictions
- A K8s operator that enforces the policy at container startup

**What PodLock does not do:** generate the profiles. The user must author them by
hand — which is precisely the problem addressed here.

```
                           ┌─────────────────────────────────┐
  landlock-genprof         │  PodLock (Kubewarden)            │
  ──────────────────       │  ─────────────────────────────── │
  observes the pod  ──────►│  LandlockProfile CRD             │
  generates YAML           │  K8s operator                    │
  (human review)    ──────►│  Runtime enforcement             │
                           └─────────────────────────────────┘
```

`landlock-genprof` is **complementary to PodLock**, not a competitor. It generates
profiles in the format expected by PodLock, upstream in the chain.

---

## 3. How it works

The full workflow runs in five steps:

### Step 1 — Training run

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

### Step 2 — Syscall capture (Tracer)

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

### Step 3 — Policy synthesis

Events are aggregated by directory (to avoid per-file overfitting) and a
**confidence level** is calculated for each rule based on how consistently it
was observed across multiple runs:

| Level | Meaning |
|---|---|
| `high` | Observed consistently on every run — reliable rule |
| `medium` | Observed on multiple runs, but with inconsistencies |
| `low` | Observed only once — must be reviewed before deployment |

### Step 4 — YAML generation

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

### Step 4bis — Optional NetworkPolicy generation

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

### Step 4ter — Optional target restart (`--restart`)

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
[`deploy/rbac-restart.yaml`](deploy/rbac-restart.yaml) first.

### Step 4quater — Optional multi-run history (`--history`)

`Confidence` is meant to reflect how many separate training runs
observed an access ("seen on every run" vs "seen once out of 5 runs"),
but a single `trace` run has no way to know that — it can only measure
how many times something was seen *within* that one run. Pass
`--history` to persist a `TrainingHistory` custom resource
(`internal/history`, no controller — the CLI reads/writes it directly)
that accumulates across every `--history` run for the same
container/binary, so `Confidence` can finally be computed from the real
ratio. Requires the CRD and additional RBAC, applied once:
[`deploy/crd-traininghistory.yaml`](deploy/crd-traininghistory.yaml),
[`deploy/rbac-history.yaml`](deploy/rbac-history.yaml). Query the result
directly: `kubectl get traininghistory <container>-<binary-basename> -o
yaml`. `profile.yaml`/`networkpolicy.yaml`/`capabilities.yaml` themselves
show it too — every path/port/capability gets a trailing `# confidence:
...` comment (see Step 4), and with `--history` that comment reflects the
real cross-run ratio instead of the single-run estimate used without it.
`seccomp.json` (Step 4quinquies) can't carry a comment — its confidence
is printed to stdout instead.

### Step 4quinquies — Optional seccomp profile generation (`--seccomp-out`)

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

### Step 4sexies — Optional Linux capabilities fragment (`--capabilities-out`)

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
`docs/e2e-demo.md` Finding 5): privilege-related capability checks
(dropping root via `setuid`/`setgid`, binding a privileged port,
`chown`ing files during init) cluster heavily at container startup.
Tracing a container that's already been running for a while will often
come back with nothing observed at all — not wrong, just nothing left to
see — the same startup blind spot `--restart` already exists to close
for filesystem access (Finding 2), applying here too.

### Step 4septies — Optional composed securityContext (`--security-context-out`)

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
  localhostProfile: operator/default/nginx-demo.json
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
`operator/<namespace>/<pod>.json` naming convention — see Step 4undecies
for why, and for the flag that actually generates the object at that
path.

**Deliberately does not infer** `privileged`, `allowPrivilegeEscalation`,
`runAsNonRoot`, `readOnlyRootFilesystem`, or `runAsUser` — nothing in
this project observes any of them today, and guessing "safe defaults"
regardless of what was actually seen would contradict the project's own
positioning: observe, don't guess.

### Step 4octies — Optional unified review report (`--report-out`)

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
`docs/e2e-demo.md` Finding 5 and re-run with `--restart`.

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

### Step 4nonies — Proposal publishing (mandatory)

Every `trace` run publishes its generated multi-domain profile as a
`SecurityProfileProposal` custom resource — stored as a cluster object
instead of only local files, reviewable via `kubectl`/GitOps. This isn't
an opt-in flag: it's the primary reviewable artifact this tool produces,
so a run fails outright if it can't publish (missing CRD or RBAC below)
rather than silently degrading to local files only.

```bash
kubectl get securityprofileproposal nginx-demo -o yaml
```

Each field is the **exact rendered content** of the corresponding local
file — `spec.podLock` is the full, real `profile.yaml`
(`apiVersion`/`kind`/`metadata`/`spec` included), `spec.networkPolicy`
the full `networkpolicy.yaml`, `spec.seccomp` the real `seccomp.json`
text, `spec.patchedManifest` the full `<identity>-patched.yaml` (Step
4decies below) — the live owner's (or bare pod's) complete manifest with
the generated `securityContext` already merged in, not the bare fragment
`--security-context-out` produces, `spec.spoSeccompProfile` the full
`<pod>-seccompprofile.yaml` (Step 4undecies below) — a
security-profiles-operator SeccompProfile custom resource. Copy any of
them directly out of `kubectl get -o yaml` and use as-is (`kubectl apply
-f -` for all five).

`spec.patchedManifest`'s `securityContext.seccompProfile.localhostProfile`
always references SPO's own `operator/<namespace>/<pod>.json` naming
convention whenever `spec.seccomp` is non-empty — see Step 4undecies for
why a plain filename isn't enough and what applying
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
[`deploy/crd-securityprofileproposal.yaml`](deploy/crd-securityprofileproposal.yaml),
[`deploy/rbac-proposal.yaml`](deploy/rbac-proposal.yaml).

### Step 4decies — Optional ready-to-apply patched manifest (`--patched-manifest-out`)

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
build a local file): [`deploy/rbac-patched-manifest.yaml`](deploy/rbac-patched-manifest.yaml).

The same content is embedded in `spec.patchedManifest` of the
`SecurityProfileProposal` (Step 4nonies) on every run regardless of
whether `--patched-manifest-out` was passed — that flag only controls
whether it's *also* written as a local file.

### Step 4undecies — Optional SeccompProfile custom resource (`--seccomp-profile-out`)

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
`/var/lib/kubelet/seccomp/operator/<namespace>/<name>.json` on every
node and exposes that same path as `status.localhostProfile` — the
`operator/<namespace>/<pod>.json` value `--security-context-out`/
`--patched-manifest-out`/the `SecurityProfileProposal` all already
reference (Step 4septies), computed ahead of time since this tool never
waits for SPO's own reconciliation to run.

### Step 5 — Mandatory human review

**`landlock-genprof` never deploys a profile automatically.**
The generated YAML is a starting point for human review, not a final result.
The `Confidence` field per rule makes explicit what is reliable and what requires
attention. See [`docs/threat-model.md`](docs/threat-model.md) for the recommended
validation methodology.

**Applying a `LandlockProfile` alone has no effect.** PodLock's admission
webhook matches a running pod to a `LandlockProfile` object via a label
on the *pod* — `podlock.kubewarden.io/profile: <profile-name>` — not by
anything embedded in the CRD itself. `landlock-genprof trace` prints the
exact `kubectl label` command to run after `kubectl apply`-ing the
generated profile.

---

## 4. Technical stack

| Component | Choice | Rationale |
|---|---|---|
| Language | **Go 1.26** | Native K8s ecosystem (client-go, controller-runtime); Inspektor Gadget Go SDK |
| Tracer | **[Inspektor Gadget](https://www.inspektor-gadget.io/)** | Pre-written, CNCF-maintained eBPF gadgets — avoids writing eBPF from scratch (high risk for beginners) |
| Output format | **LandlockProfile CRD** ([PodLock](https://github.com/flavio/podlock)) | Existing format, Kubewarden ecosystem — complementary, not competing |
| Dev cluster | **[kind](https://kind.sigs.k8s.io/)** | Shares host kernel — required for Landlock and eBPF to work |
| CI | **GitHub Actions** (`ubuntu-24.04`) | Kernel 6.8 — covers both FS and network Landlock |
| License | **Apache-2.0 OR MIT** | Dual license, recipient's choice (convention from `landlock-lsm/island`) — compatible with PodLock and the CNCF ecosystem |

**Key Go dependencies** (all pinned to exact versions in `go.mod`, never `@latest`):

```
github.com/inspektor-gadget/inspektor-gadget  # tracer SDK (Linux-only, see internal/tracer)
sigs.k8s.io/yaml                               # YAML serialization
k8s.io/client-go                               # pod resolution
github.com/spf13/cobra                         # CLI
```

---

## 5. Repository architecture

> Flow diagrams (components, training-run sequence, package dependencies):
> see [`docs/architecture.md`](docs/architecture.md).

```
landlock-genprof/
│
├── cmd/landlock-genprof/      CLI entry point
│   └── main.go                Sub-command dispatch (trace, version)
│
├── internal/
│   ├── tracer/                Syscall event capture
│   │   └── tracer.go          Event, Options types — Inspektor Gadget integration
│   ├── policy/                Event aggregation → Behavior IR
│   │   └── synthesize.go      Synthesize() — aggregation algorithm (technology-neutral)
│   ├── profile/                Behavior IR — independent of any output format
│   │   └── profile.go         BehaviorProfile, FilesystemProfile, FileAccess, Confidence
│   ├── exporter/
│   │   ├── podlock/           IR → PodLock conversion (only package depending on both)
│   │   │   └── export.go      ToProfile(), ToYAML()
│   │   ├── networkpolicy/     IR → Kubernetes NetworkPolicy conversion
│   │   │   └── export.go      ToPolicy(), ToYAML()
│   │   ├── seccomp/           IR → seccomp profile conversion
│   │   │   └── export.go      ToProfile(), ToJSON()
│   │   ├── capabilities/      IR → Linux capabilities fragment conversion
│   │   │   └── export.go      ToProfile(), ToYAML()
│   │   ├── securitycontext/   Composes capabilities + a seccomp reference
│   │   │   └── export.go      ToSecurityContext(), ToYAML()
│   │   └── report/            Unified Markdown review report
│   │       └── export.go      ToMarkdown()
│   ├── history/                TrainingHistory CRD (multi-run Confidence)
│   │   └── record.go          Record, Merge(), ApplyConfidence()
│   ├── proposal/                SecurityProfileProposal CRD (publishable snapshot)
│   │   └── store.go            Spec, Save(), Get()
│   └── k8s/                   Pod target orchestration
│       ├── target.go          Namespace/pod/container resolution via client-go
│       ├── restart.go         --restart: DetectOwner(), Restart(), PodSelectorFor()
│       └── patch.go           --patched-manifest-out: PatchedManifest()
│
├── pkg/
│   ├── podlock/                Go types for the LandlockProfile CRD (PodLock)
│   │   └── types.go           LandlockProfile, Profile, Metadata
│   └── seccomp/                Go types for the seccomp profile JSON format
│       └── types.go           Profile, SyscallRule
│
├── examples/
│   └── nginx-generated-profile.yaml   Illustrative example of a generated profile
│
├── docs/
│   ├── roadmap.md             Milestones, task assignments, fallback plan
│   └── threat-model.md        Attack surface, validation methodology
│
├── hack/
│   └── check-kernel.sh        Kernel prerequisite check (Landlock + eBPF)
│
├── .github/workflows/
│   └── ci.yml                 Build, test, vet (ubuntu-24.04 / kernel 6.8)
│
├── Makefile                   build/test/vet/docker-* targets (see `make help`)
├── Dockerfile.dev             Build/test in a Linux container without the VM
├── go.mod
├── LICENSE-APACHE             Full Apache-2.0 text
├── LICENSE-MIT                Full MIT text
├── COPYRIGHT                  Explains the "either, your choice" dual license
├── README.md                  This document
└── README.etudiants.md        French version for students
```

---

## 6. Prerequisites

### Linux kernel

landlock-genprof's only real requirement is the **kernel version** — not
a specific distro. Nothing under `hack/` calls a distro-specific package
manager (`apt`/`dnf`/`yum`, ...); `check-kernel.sh`/`init-vm.sh` only use
`uname`, `curl`, `tar`, and generic Linux tooling. Any distro shipping a
kernel meeting the versions below should work.

| Feature | Minimum kernel version | Notes |
|---|---|---|
| Landlock FS | **≥ 5.13** | File/directory confinement |
| Landlock network | **≥ 6.4** | TCP confinement (connect/bind) |
| eBPF (Inspektor Gadget) | **≥ 5.8** recommended | BPF ring buffer |

**Actually tested** (this is a "known to work" list, not a restriction —
see above):

| Distro | Kernel | Status |
|---|---|---|
| Ubuntu 24.04 | 6.8 | ✅ validated |
| Ubuntu 26.04 | 7.0 | ✅ validated |

Check host prerequisites:

```bash
./hack/check-kernel.sh
```

### Tools

```bash
go 1.26+        # Build and tests
kind            # Local K8s cluster (shares host kernel)
kubectl         # Cluster interaction
```

### Setting up kind and the dev cluster

```bash
# Install kind (pinned version, not @latest)
go install sigs.k8s.io/kind@v0.32.0

# Create cluster
kind create cluster --name landlock-dev

# Verify
kubectl cluster-info --context kind-landlock-dev
```

> `./hack/init-vm.sh` (or `make init-vm`) automates this plus kubectl,
> Inspektor Gadget, and a test pod in one idempotent script — see
> `HOW_TO_START.md` §2 for a detailed walkthrough of what it does.

---

## 7. Quick start

```bash
# Clone the repo
git clone git@github.com:idriss-eliguene/landlock-genprof.git
cd landlock-genprof

# Check kernel prerequisites
./hack/check-kernel.sh

# Build
go build ./...

# Tests (unit — no cluster required)
go test ./...

# CLI (Trace() captures openat via Inspektor Gadget — Linux + a real
# cluster with Inspektor Gadget deployed required, see HOW_TO_START.md)
go run ./cmd/landlock-genprof trace --pod nginx --namespace default --binary /usr/sbin/nginx --duration 60s --out profile.yaml
```

### Installing as a kubectl plugin

`landlock-genprof` works standalone (as above), but also installs as a
`kubectl` plugin: a plugin is nothing more than an executable named
`kubectl-<name>` somewhere on `$PATH` — `kubectl <name>` finds and runs
it. The tool already resolves the kubeconfig the same way `kubectl`
itself does (`internal/k8s.RestConfig()`), so no code changes were needed
for this, only a differently-named build:

```bash
make install-plugin   # builds kubectl-landlock-genprof and installs it into $(go env GOPATH)/bin
kubectl plugin list   # confirms kubectl sees it
kubectl landlock-genprof trace --pod nginx --namespace default --binary /usr/sbin/nginx --duration 60s
```

One kubectl-plugin quirk worth knowing: global `kubectl` flags placed
*before* the plugin name (`kubectl -n foo landlock-genprof ...`) are
**not** forwarded to the plugin — kubectl only passes through the
arguments that come *after* the plugin name. Use `landlock-genprof`'s own
`-n`/`--namespace` flag instead: `kubectl landlock-genprof trace -n foo ...`.

---

## 8. Example output

Profile generated for an nginx pod after a 60 s training run.
See [`examples/nginx-generated-profile.yaml`](examples/nginx-generated-profile.yaml).

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
          - /usr/share/nginx        # confidence: high — seen on every run
        readWrite:
          - /tmp                    # confidence: high — seen on every run
          - /var/cache/nginx/proxy  # confidence: low — seen 1 out of 5 runs
```

The `confidence` annotation makes **explicit** what is reliable and what needs
verification before production deployment.

---

## 9. Team and task assignment

Three-student project. Each role is independent to allow parallel progress from day one.

| Student | Component | Technical focus |
|---|---|---|
| **Student A** | `internal/tracer/` | Inspektor Gadget SDK integration, syscall → Landlock right mapping, event formats |
| **Student B** | `cmd/`, `internal/k8s/`, `internal/policy/` | CLI (cobra), K8s orchestration via client-go, synthesis algorithm and directory-level aggregation |
| **Student C** | `docs/threat-model.md`, adversarial tests, CI | Profile validation methodology, tracer attack surface, pentesting (evasion, RBAC), CI hardening (gosec, Trivy) |

### How to work in parallel from week 1

Students B and C **do not need a working tracer** to make progress. Mock trace data
(a static `[]Event` slice hard-coded in tests) allows developing the synthesis
algorithm and the threat model independently. Real integration with Student A's
tracer happens at M1.

---

## 10. Risk management

### Primary risk: eBPF is hard for beginners

eBPF is notoriously complex (kernel verifier, CO-RE, bpftool). Two mitigations
were established at design time:

**Mitigation 1 — Do not write eBPF from scratch**

We consume existing **Inspektor Gadget** gadgets via their Go SDK
(`trace_open`, `trace_tcpconnect`, etc.). These gadgets are authored, tested, and
maintained by the CNCF community. Student A does not write eBPF programs —
they call a Go API that returns `Event` objects.

**Mitigation 2 — Hard checkpoint at week 3-4**

If the tracer does not produce real events (at minimum `openat`) by week 3-4,
**immediately switch to the fallback plan**: capture events using `strace -f` and
parse its output. Less elegant than eBPF, but:

- Sufficient for a one-shot training run (no production performance requirement)
- Students B and C are not blocked
- The rest of the pipeline (synthesis, YAML generation, CLI) is unchanged

```
Plan A (nominal)          Plan B (fallback week 3-4)
─────────────────         ──────────────────────────
Inspektor Gadget    →     strace -f + parsing
  Go SDK                  (same Event{} interface)
  eBPF kernel             no eBPF kernel requirement
```

### Secondary risk: completeness of generated profiles

A short training run does not cover all code paths (error handling, edge cases,
rarely-triggered behaviour). An incomplete profile may break the application in
production on an unobserved path. Mitigation: the `Confidence` field per rule makes
this risk **visible** in the YAML rather than giving a false impression of
completeness. See [`docs/threat-model.md`](docs/threat-model.md).

---

## 11. Milestones

| Milestone | Content | Owner |
|---|---|---|
| **M0 — Setup** | Repo, CI, `go.mod` with real dependencies, `hack/check-kernel.sh`, kind cluster | All |
| ⚠️ **Checkpoint week 3-4** | Tracer produces real events on at least `openat`. Otherwise: switch to `strace` fallback | Student A |
| **M1** | Working tracer (`openat` + `connect`), end-to-end `trace` CLI on an nginx pod | A + B |
| **M2** | Policy synthesis (directory-level aggregation, confidence levels), PodLock YAML export | B + C |
| **M3** | Full K8s integration (pod resolution via client-go, minimal tracer RBAC) | B + C |
| **M4** | End-to-end demo on kind — generated profile for nginx, comparison with a hand-written profile | All |
| **M5 _(stretch)_** | Post-deployment drift detection: Landlock denial logs → policy adjustment suggestions | All |

---

## 12. Threat model

The tracer itself introduces an attack surface: it requires elevated capabilities
(`CAP_BPF`, `CAP_SYS_ADMIN` depending on kernel version) to observe a pod's
syscalls. Open questions to document in [`docs/threat-model.md`](docs/threat-model.md):

- Which exact capabilities are required, and can they be reduced?
- Should the tracer run permanently or only during training runs (preferred)?
- What is the minimal RBAC for the tracer's service account (dedicated namespace,
  no cluster-wide rights beyond what is strictly needed)?
- Can an observed pod **detect it is being traced** and modify its behaviour to
  generate an artificial profile (evasion)?
- Can the human review workflow be bypassed in practice?

---

## 13. Contributing

This is a teaching project. External contributions are welcome after the semester
ends. Active development currently happens in student branches:

```
master        → stable scaffolding, architecture decisions
feat/tracer   → Student A (internal/tracer)
feat/policy   → Student B (internal/policy + k8s + cmd)
feat/threat   → Student C (docs + CI)
```

---

## 14. License

Dual-licensed, recipient's choice: [Apache-2.0](LICENSE-APACHE) **or**
[MIT](LICENSE-MIT) — see [`COPYRIGHT`](COPYRIGHT). Convention borrowed from
[`landlock-lsm/island`](https://github.com/landlock-lsm/island), the official
Landlock sandboxing tool. Compatible with PodLock and the CNCF ecosystem.
