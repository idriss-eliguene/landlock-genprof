# landlock-genprof

[![CI](https://github.com/idriss-eliguene/landlock-genprof/actions/workflows/ci.yml/badge.svg)](https://github.com/idriss-eliguene/landlock-genprof/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/idriss-eliguene/landlock-genprof)](https://goreportcard.com/report/github.com/idriss-eliguene/landlock-genprof)
[![License](https://img.shields.io/badge/license-Apache--2.0%20OR%20MIT-blue.svg)](COPYRIGHT)

> Version française pour les étudiants : [`README.etudiants.md`](README.etudiants.md).
> Student onboarding guide (French): [`HOW_TO_START.md`](HOW_TO_START.md).
> Installing against a cluster you already have? [`INSTALL.md`](INSTALL.md).

Automatic Kubernetes security profile generator — [Landlock](https://landlock.io/),
`NetworkPolicy`, seccomp, and Linux capabilities — built on **observation** of a
running pod (a "training run") rather than manual rule authoring.

```
Container runs with broad, hand-guessed permissions
                    │
                    ▼
   landlock-genprof trace --pod nginx --duration 60s
                    │
                    ▼
    Observed runtime behavior — filesystem, network, syscalls
                    │
                    ▼
     Generated least-privilege profiles, confidence-annotated

  ✓ Filesystem  → PodLock LandlockProfile
  ✓ Network     → Kubernetes NetworkPolicy
  ✓ Syscalls    → seccomp (security-profiles-operator)
  ✓ Hardening   → securityContext fragment
```

See [§8 — Example output](#8-example-output) for a real generated profile.

The name is a deliberate nod to `aa-genprof` / `aa-logprof` — the AppArmor
profile generation tools. Landlock had no equivalent when this started,
and filling that gap is where the name comes from — the tool itself has
since grown to cover network, syscalls, and capabilities from the same
training run, not just Landlock's own filesystem/network rights.

> **Status:** the observe → synthesize → export pipeline is built and
> confirmed end to end on a live cluster (filesystem, network, seccomp,
> capabilities, cross-run confidence via `--history`), tagged `v0.1.0`.
> [`docs/roadmap.md`](docs/roadmap.md) tracks what's actually built,
> milestone by milestone — the source of truth over anything below.
> [`docs/product-definition-v1.md`](docs/product-definition-v1.md) for
> where this is headed as a product. This started as a 3-student course
> project; that context (team, original risk plan, original milestone
> plan) moved to [`docs/pedagogy.md`](docs/pedagogy.md) — real, but not
> what a reader evaluating the tool itself needs first.

## Quick links

| | |
|---|---|
| **Install** | [`INSTALL.md`](INSTALL.md) — already have a cluster? Start here. |
| **Full usage reference** | [`docs/usage.md`](docs/usage.md) — every `trace` flag, one section each. |
| **Architecture** | [`docs/architecture.md`](docs/architecture.md) — data flow, [sequence diagram](docs/sequence-diagram.md), [package deps](docs/packages.md). |
| **Demo** | [`demo/script.md`](demo/script.md) — a 75s walkthrough script. |
| **Contributing** | [`CONTRIBUTING.md`](CONTRIBUTING.md) · [`GOVERNANCE.md`](GOVERNANCE.md) · [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) |
| **Enforcement prerequisites** | [`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md) — what PodLock/SPO/a NetworkPolicy-capable CNI each need, including PodLock's real limitation on this project's own `kind` setup. |

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

Generating a correct `LandlockProfile` doesn't require PodLock's operator
to be installed anywhere — but seeing it actually enforced does. See
[`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md)
before assuming this repo's own `kind`-based dev setup can demonstrate
that end to end; short version: it can't, per PodLock's own docs.

---

## 3. How it works

Five core steps — full detail, every optional flag, in
[`docs/usage.md`](docs/usage.md):

1. **Training run** — the target pod runs normally for a set duration
   (`landlock-genprof trace --pod ... --duration 60s`).
2. **Capture** — [Inspektor Gadget](https://www.inspektor-gadget.io/)
   gadgets observe filesystem, network, syscall, and capability activity
   via eBPF.
3. **Synthesis** — events are aggregated by directory, each rule gets a
   `Confidence` level (`high`/`medium`/`low`) based on how consistently
   it was observed.
4. **Export** — one package per output: PodLock `LandlockProfile`
   (always), `NetworkPolicy`, seccomp (plain JSON and/or an SPO
   `SeccompProfile` CR), a capabilities fragment, a composed
   `securityContext`, a Markdown review report — plus a
   `SecurityProfileProposal` cluster object combining the four
   directly-appliable artifacts, published on every run (mandatory, not
   opt-in). See [§8](#8-example-output) for real examples of each.
5. **Human review** — `landlock-genprof` never applies anything itself.
   Every artifact is a starting point for review, not a final result.

---

## 4. Technical stack

| Component | Choice | Rationale |
|---|---|---|
| Language | **Go 1.26** | Native K8s ecosystem (client-go); Inspektor Gadget Go SDK |
| Tracer | **[Inspektor Gadget](https://www.inspektor-gadget.io/)** | Pre-written, CNCF-maintained eBPF gadgets — avoids writing eBPF from scratch |
| Output formats | **PodLock** `LandlockProfile`, `NetworkPolicy`, seccomp (JSON + SPO CR), capabilities, `securityContext` | Existing, upstream formats — complementary, not competing |
| Dev cluster | **[kind](https://kind.sigs.k8s.io/)** + **[Cilium](https://cilium.io/)** | kind shares the host kernel (required for Landlock/eBPF); Cilium replaces kindnet so generated `NetworkPolicy` is actually enforceable |
| CI | **GitHub Actions** (`ubuntu-24.04`) | Kernel 6.8 — covers both FS and network Landlock; `build-and-test` + `security` (gosec, Trivy) both required checks |
| License | **Apache-2.0 OR MIT** | Dual license, recipient's choice — compatible with PodLock and the CNCF ecosystem |

**Key Go dependencies** (all pinned to exact versions in `go.mod`, never `@latest`):

```
github.com/inspektor-gadget/inspektor-gadget  # tracer SDK (Linux-only, see internal/tracer)
sigs.k8s.io/yaml                               # YAML serialization
k8s.io/client-go                               # pod resolution
github.com/spf13/cobra                         # CLI
```

---

## 5. Repository layout

> Full data-flow/sequence/package-dependency diagrams:
> [`docs/architecture.md`](docs/architecture.md),
> [`docs/sequence-diagram.md`](docs/sequence-diagram.md),
> [`docs/packages.md`](docs/packages.md) — the ASCII tree below is
> deliberately shallow; a deep hand-maintained one goes stale (this
> project's own past experience — see `docs/roadmap.md`).

```
landlock-genprof/
├── cmd/landlock-genprof/    CLI entry point — trace, review, version
├── internal/
│   ├── tracer/              Syscall event capture (Inspektor Gadget)
│   ├── policy/               Event aggregation → Behavior IR
│   ├── profile/              Behavior IR — independent of any output format
│   ├── exporter/             One package per output format (6 total)
│   ├── history/              TrainingHistory CRD (multi-run Confidence)
│   ├── proposal/             SecurityProfileProposal CRD
│   └── k8s/                  Pod resolution, --restart, --patched-manifest-out
├── pkg/                     Go types for PodLock/seccomp/SPO CRDs
├── examples/                Illustrative + real generated artifacts
├── docs/                    Architecture, usage, threat model, roadmap, ...
├── deploy/                  RBAC/CRD manifests + the Helm chart
├── demo/                    Demo script
├── hack/                    Dev VM/kernel-check scripts
└── .github/workflows/       CI (build-and-test, security)
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
helm            # Installing this project's own chart, and Cilium below
```

### Setting up kind and the dev cluster

```bash
# Install kind (pinned version, not @latest)
go install sigs.k8s.io/kind@v0.32.0

# Create cluster — CNI disabled here on purpose, see the note below
cat <<EOF | kind create cluster --name landlock-dev --config -
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
EOF

# Verify
kubectl cluster-info --context kind-landlock-dev
```

> `./hack/init-vm.sh` (or `make init-vm`) automates this plus Helm,
> Cilium, kubectl, Inspektor Gadget, and a test pod in one idempotent
> script — see `HOW_TO_START.md` §2 for a detailed walkthrough of what
> it does. Cilium replaces kind's default CNI (kindnet) because kindnet
> doesn't implement `NetworkPolicy` at all — skip that step only if you
> don't care whether `--network-out`'s output is actually enforceable.
> This gets you `trace` and profile *generation* end to end; actually
> enforcing what's generated needs more — see
> [`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md).

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

# Apply required CRDs/RBAC before the first trace run
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/crd-securityprofileproposal.yaml
kubectl apply -f deploy/rbac-proposal.yaml
# Required whenever this run composes securityContext data
# (commonly true in practice when syscalls are observed)
kubectl apply -f deploy/rbac-patched-manifest.yaml

# CLI (Trace() captures openat via Inspektor Gadget — Linux + a real
# cluster with Inspektor Gadget deployed required, see HOW_TO_START.md)
go run ./cmd/landlock-genprof trace --pod nginx --namespace default --binary /usr/sbin/nginx --duration 60s --out profile.yaml
```

This is the fastest path to a first result, on the disposable dev
cluster from §6. For a Helm-based install, the kubectl-plugin build, or
installing against a cluster you already have (not one you just spun up
for this), see [`INSTALL.md`](INSTALL.md) instead of repeating all of
that here.

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

The other five artifacts each get their own example too — same
`nginx-demo` scenario, one file per domain:

| Domain | Example |
|---|---|
| Network (`--network-out`) | [`examples/nginx-generated-networkpolicy.yaml`](examples/nginx-generated-networkpolicy.yaml) |
| Syscalls, plain JSON (`--seccomp-out`) | [`examples/nginx-generated-seccomp.json`](examples/nginx-generated-seccomp.json) |
| Syscalls, SPO custom resource (`--seccomp-profile-out`) | [`examples/nginx-generated-seccompprofile.yaml`](examples/nginx-generated-seccompprofile.yaml) |
| Capabilities (`--capabilities-out`) | [`examples/nginx-generated-capabilities.yaml`](examples/nginx-generated-capabilities.yaml) |
| Composed securityContext (`--security-context-out`) | [`examples/nginx-generated-securitycontext.yaml`](examples/nginx-generated-securitycontext.yaml) |
| Unified review report (`--report-out`) | [`examples/nginx-generated-report.md`](examples/nginx-generated-report.md) |

Unlike `nginx-generated-profile.yaml` above (real M4 milestone output),
these five are illustrative — adapted from
[`docs/usage.md`](docs/usage.md)'s own Step 4* sections rather than
freshly captured from a live run. Their shape and
field names are accurate; regenerating them from an actual `trace` run
is tracked as [good first issue #94](https://github.com/idriss-eliguene/landlock-genprof/issues/94).

### The `SecurityProfileProposal` — the actual primary artifact

Every `trace` run publishes **all four applyable artifacts together**
as one cluster object (`docs/usage.md` Step 4nonies) — this, not the
separate local files, is the artifact this tool is really built around:
reviewable via `kubectl`/GitOps, one `kubectl get -o yaml` away instead
of five separate files to track down.

See [`examples/nginx-generated-proposal.yaml`](examples/nginx-generated-proposal.yaml)
for the complete object — `spec.podLock`/`networkPolicy`/
`patchedManifest`/`spoSeccompProfile` each hold the exact rendered YAML
of the corresponding artifact as a plain string, copy-pasteable
straight out of `kubectl get securityprofileproposal nginx-demo -o
yaml` into a `kubectl apply -f -`.

---

## 9. Threat model

The tracer needs elevated capabilities (`CAP_BPF`, `CAP_SYS_ADMIN`
depending on kernel version) to observe a pod's syscalls — a real
attack surface, documented and analyzed, not just flagged as a
to-do. [`docs/threat-model.md`](docs/threat-model.md) covers:

1. **Tracer attack surface** — exact capabilities required, RBAC scope,
   whether the tracer should run permanently (it shouldn't).
2. **Completeness of generated profiles** — the false-negative risk a
   short training run carries, and how `Confidence` surfaces it.
3. **Pentesting the operator / the generated profile** — evasion: can a
   traced pod detect it's being observed and behave differently?
4. **CI hardening** — `gosec`/Trivy as required checks, not advisory.

---

## 10. Contributing

External contributions are welcome. See [`CONTRIBUTING.md`](CONTRIBUTING.md)
for the development setup, code conventions, and what to check before
opening a PR — also [`GOVERNANCE.md`](GOVERNANCE.md) for how decisions get
made and [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) for the expected
conduct. For where the product is headed, see
[`docs/product-definition-v1.md`](docs/product-definition-v1.md),
[`docs/product-design-v1.md`](docs/product-design-v1.md), and
[`docs/product-roadmap-v1.md`](docs/product-roadmap-v1.md).

---

## 11. License

Dual-licensed, recipient's choice: [Apache-2.0](LICENSE-APACHE) **or**
[MIT](LICENSE-MIT) — see [`COPYRIGHT`](COPYRIGHT). Compatible with
PodLock and the CNCF ecosystem.
