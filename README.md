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

| Gadget | Observed syscall | Landlock rights |
|---|---|---|
| `trace_open` | `openat`, `open` | `LANDLOCK_ACCESS_FS_READ_FILE`, `WRITE_FILE`, `EXECUTE` |
| `trace_tcpconnect` | `connect` | `LANDLOCK_ACCESS_NET_CONNECT_TCP` (kernel ≥ 6.4) |
| `trace_bind` | `bind` | `LANDLOCK_ACCESS_NET_BIND_TCP` (kernel ≥ 6.4) |
| `trace_exec` | `execve`, `execveat` | `LANDLOCK_ACCESS_FS_EXECUTE` |

Each captured event produces an `Event` object:

```go
type Event struct {
    Timestamp time.Time
    Syscall   string // "openat", "connect", "bind", "execve"
    Path      string // file path, if applicable
    Port      int    // network port, if applicable
    Mode      string // "read", "write", "read_write", "exec"
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
│   ├── exporter/podlock/      IR → PodLock conversion (only package depending on both)
│   │   └── export.go          ToProfile(), ToYAML()
│   └── k8s/                   Pod target orchestration
│       └── target.go          Namespace/pod/container resolution via client-go
│
├── pkg/
│   └── podlock/               Go types for the LandlockProfile CRD (PodLock)
│       └── types.go           LandlockProfile, Profile, Metadata
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
