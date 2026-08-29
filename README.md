# landlock-genprof

[![CI](https://github.com/idriss-eliguene/landlock-genprof/actions/workflows/ci.yml/badge.svg)](https://github.com/idriss-eliguene/landlock-genprof/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/idriss-eliguene/landlock-genprof)](https://goreportcard.com/report/github.com/idriss-eliguene/landlock-genprof)
[![License](https://img.shields.io/badge/license-Apache--2.0%20OR%20MIT-blue.svg)](COPYRIGHT)

**Govern runtime-derived Kubernetes security policy.**

Observe or import what a workload learned. Review one mixed-origin candidate.
Authorize its exact digest. Apply only what remains approved.

> Version française pour les étudiants : [`README.etudiants.md`](README.etudiants.md).
> Student onboarding guide: [`HOW_TO_START.md`](HOW_TO_START.md) (French
> version: [`COMMENT_COMMENCER.md`](COMMENT_COMMENCER.md)).
> Installing against a cluster you already have? [`INSTALL.md`](INSTALL.md).

landlock-genprof turns runtime evidence and externally derived policy into a
`SecurityProfileProposal`: one reviewable candidate with deterministic content
identity and explicit human authority. It governs filesystem, network, seccomp,
and capability artifacts toward external enforcement and verification.

```
landlock-genprof observations       SPO-derived SeccompProfile
   filesystem / network                  syscalls
             \                              /
              └──── governed candidate ────┘
                         │
                  CandidateDigest
                         │
                review → approve exact digest
                         │
                   governed apply
                         │
             PodLock · CNI · SPO/runtime
                         │
                       verify
```

[![landlock-genprof: trace, review, and apply-proposal against a real cluster](demo/demo.gif)](https://asciinema.org/a/Y0IHrGK0zYcDbgaw)

Click the GIF for the interactive recording, or see
[`demo/script.md`](demo/script.md) for the full shot list to run it yourself.

Direct observations can carry cross-run confidence. SPO-derived syscalls do
not enter landlock-genprof `TrainingHistory` and receive no invented
confidence: they enter at the artifact layer as derived policy with provenance.

**Why:** Kubernetes already provides strong least-privilege controls, but
teams struggle to configure them correctly — policy authoring is manual,
error-prone, and demands deep platform expertise. See
[`docs/architecture.md`](docs/architecture.md) for the current source,
authority, and enforcement boundaries.

The name is a deliberate nod to `aa-genprof` / `aa-logprof` — the AppArmor
profile generation tools. Landlock had no equivalent when this started,
and filling that gap is where the name comes from — the tool itself has
since grown to cover network, syscalls, and capabilities from the same
training run, not just Landlock's own filesystem/network rights.

> **Status:** proposal generation, deterministic digest identity,
> digest-bound approval, stale-authority rejection, and governed apply are
> implemented, tagged `v0.3.0`. <!-- x-release-please-version --> NetworkPolicy
> denial is demonstrated on Cilium; the SPO/Seccomp path has a real-node
> merged-provenance and tested behavioral-denial boundary; PodLock/Landlock
> kernel denial is not demonstrated. [`docs/PROGRESS.md`](docs/PROGRESS.md)
> is authoritative.

## Govern a candidate

No cluster yet? [`hack/init-vm.sh`](hack/init-vm.sh) builds a disposable
one (`kind` + Cilium + Inspektor Gadget + a test pod), see
[`docs/test-environment.md`](docs/test-environment.md) for the
step-by-step version. Already have a cluster? See
[`INSTALL.md`](INSTALL.md) instead — same three commands below, once
the CLI and its RBAC/CRDs are in place.

```bash
kubectl landlock-genprof doctor

kubectl landlock-genprof trace --pod nginx-demo --namespace default \
  --binary /usr/sbin/nginx --duration 60s

kubectl landlock-genprof review nginx-demo

# Use the Candidate digest printed by review to bind explicit approval.
kubectl landlock-genprof approve nginx-demo \
  --expected-digest sha256:<candidate-digest-from-review>

kubectl landlock-genprof apply-proposal nginx-demo
```

Diagnose, acquire, review, approve the reviewed digest, then apply through
`apply-proposal`. A changed candidate cannot inherit the old approval. Full lifecycle:
[`docs/usage.md`](docs/usage.md); every command's own options/examples:
[CLI reference](https://idriss-eliguene.github.io/landlock-genprof/).

## Quick links

| | |
|---|---|
| **No cluster yet** | [`docs/test-environment.md`](docs/test-environment.md) — disposable `kind` cluster, from nothing. |
| **Install** | [`INSTALL.md`](INSTALL.md) — already have a cluster? Start here. |
| **Full usage reference** | [`docs/usage.md`](docs/usage.md) — every `trace` flag, one section each. |
| **Architecture** | [`docs/architecture.md`](docs/architecture.md) — data flow, [sequence diagram](docs/sequence-diagram.md), [package deps](docs/packages.md). |
| **Founder-assisted pilot** | [`docs/pilot/README.md`](docs/pilot/README.md) — prerequisites, workflow, recovery, data handling, and evidence handoff. |
| **Demo** | [`demo/script.md`](demo/script.md) — a 75s walkthrough script. |
| **Contributing** | [`CONTRIBUTING.md`](CONTRIBUTING.md) · [`GOVERNANCE.md`](GOVERNANCE.md) · [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) |
| **Enforcement prerequisites** | [`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md) — what PodLock/SPO/a NetworkPolicy-capable CNI each need, including PodLock's real limitation on this project's own `kind` setup. |

---

## 1. The problem

Kubernetes has several real least-privilege mechanisms — Landlock,
seccomp, `NetworkPolicy`, Linux capabilities — but every one of them
requires **guessing in advance** what an application actually needs,
hand-authored, before anyone has observed it running:

- **Too permissive** → the policy protects nothing (everything is allowed to
  avoid breaking the app)
- **Too restrictive** → the application breaks in production on a rare code path

### Landlock: the flagship example, and where the name comes from

**Landlock** is a Linux Security Module (LSM) introduced in kernel 5.13 that
allows processes to confine themselves to a subset of the filesystem and network,
**without requiring root privileges**. This is a rare and valuable property:
whereas AppArmor, SELinux, or seccomp require system-wide configuration by an
administrator, a process can arm Landlock itself.

The problem above is compounded for Landlock specifically, in a Kubernetes context:

- Landlock has **no native integration in containerd/runc**, so there is no
  standard K8s support (`securityContext` cannot arm Landlock)
- There is **no equivalent of `aa-genprof`** for Landlock, neither in the
  [Security Profiles Operator](https://github.com/kubernetes-sigs/security-profiles-operator)
  nor in [PodLock](https://github.com/flavio/podlock)

That gap is why this project exists and where its name comes from — but
the same "guess it by hand" problem is just as real for seccomp and
`NetworkPolicy`, which is why `landlock-genprof` addresses all of them
from the same training run, not Landlock alone: observe first, write the
policy after.

### The second problem: learning is not authorization

Observation solves the guessing problem, and other systems already solve it
well — [security-profiles-operator](https://github.com/kubernetes-sigs/security-profiles-operator)
records syscalls with a production eBPF recorder, generates a
`SeccompProfile`, installs it on every node and enforces it.

What no learner provides is a **decision**. A recorded profile describes what
a workload *did*. Enforcing it is a statement about what it is *allowed* to
do, and those are not the same claim.

> **LEARNED ≠ AUTHORIZED**

`landlock-genprof` v0.2 is the authorization boundary between the two. What
was learned — by SPO, or by this project's own tracer — becomes one
reviewable candidate with one deterministic identity; a human's approval is
bound to **that exact content**; and when the workload changes, the previous
approval stops authorizing anything until someone reviews the change.

This is proven end to end against a real operator, not asserted:

```
real SPO ProfileRecording → derived policy → lineage + semantics validated
  → governed snapshot → CandidateDigest → human approval
  → stale authority refused → backend readiness → enforcement identity
  → workload binding LAST
```

That evidence proves SPO reconciliation and workload binding, not syscall
denial; the latter remains an open verification gate.

Filesystem (PodLock/Landlock), network (`NetworkPolicy`) and syscalls
(SPO `SeccompProfile`) travel as **one candidate, one digest, one decision**.
SPO records none of the first two.

---

## 2. Positioning — PodLock, SPO, and native Kubernetes enforcement

`landlock-genprof` doesn't enforce anything itself — it feeds three
existing, independent enforcement mechanisms, one per domain:

| Domain | Enforced by | This project generates |
|---|---|---|
| Filesystem (Landlock) | [PodLock](https://github.com/flavio/podlock) ([Kubewarden](https://www.kubewarden.io/) ecosystem) | `LandlockProfile` CRD |
| Syscalls (seccomp) | [security-profiles-operator](https://github.com/kubernetes-sigs/security-profiles-operator) (SPO) | `SeccompProfile` CRD |
| Network | Any CNI implementing `NetworkPolicy` (e.g. Cilium) | Kubernetes `NetworkPolicy` |

[PodLock](https://github.com/flavio/podlock) is the closest existing
project overall — it provides the `LandlockProfile` CRD and the operator
that enforces it at container startup, but **doesn't generate the
profiles**: the user still has to author them by hand, precisely the
problem addressed here.

```
                           ┌─────────────────────────────────┐
  landlock-genprof         │  PodLock (Kubewarden)            │
  ──────────────────       │  ─────────────────────────────── │
  observes the pod  ──────►│  LandlockProfile CRD             │
  generates YAML           │  K8s operator                    │
  (human review)    ──────►│  Runtime enforcement             │
                           └─────────────────────────────────┘
```

`landlock-genprof` is **complementary to PodLock, SPO, and your CNI** —
not a competitor to any of them. It generates profiles in the formats
each one already expects, upstream in the chain, for whichever domains
a given training run actually observed.

Generating a correct `LandlockProfile` doesn't require PodLock's operator
to be installed anywhere — but seeing it actually enforced does. See
[`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md)
before assuming this repo's own `kind`-based dev setup can demonstrate
that end to end; short version: it can't, per PodLock's own docs — and
the same doc covers SPO's and the CNI's own prerequisites too.

---

## 3. How it works

Seven lifecycle stages — with artifact exports kept secondary — in
[`docs/usage.md`](docs/usage.md):

1. **Diagnose and select sources** — check host prerequisites and explicitly
   choose internal or SPO-derived seccomp policy.
2. **Acquire** — landlock-genprof observes filesystem/network behavior. In
   SPO mode, SPO separately observes syscalls and derives the real
   `SeccompProfile`.
3. **Assemble** — direct evidence and derived artifacts retain provenance and
   form one `SecurityProfileProposal`. SPO syscalls do not enter
   `TrainingHistory` or receive invented confidence.
4. **Identify and review** — `CandidateDigest` gives the proposal deterministic
   content identity; `review` exposes that exact candidate.
5. **Authorize** — explicit approval binds human authority to the reviewed
   digest, and changed content makes the earlier approval stale.
6. **Governed apply** — `apply-proposal` revalidates authority, orders
   external artifacts, checks supported readiness, and binds the workload last.
7. **External enforcement and verification** — PodLock, the CNI, and
   SPO/runtime enforce; separate checks establish what was actually realized.

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
> deliberately shallow; a deep hand-maintained one goes stale.

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

# Build + install as a kubectl plugin (recommended invocation below —
# see INSTALL.md for the plain `go build`/`go run` alternative)
go test ./...
make install-plugin
kubectl plugin list   # confirms kubectl sees it

# Apply required CRDs/RBAC before the first trace run
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/crd-securityprofileproposal.yaml
kubectl apply -f deploy/rbac-proposal.yaml
# Required whenever this run composes securityContext data
# (commonly true in practice when syscalls are observed)
kubectl apply -f deploy/rbac-patched-manifest.yaml

# CLI (Trace() captures openat via Inspektor Gadget — Linux + a real
# cluster with Inspektor Gadget deployed required, see HOW_TO_START.md)
kubectl landlock-genprof trace --pod nginx --namespace default --binary /usr/sbin/nginx --duration 60s --out profile.yaml
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

Unlike `nginx-generated-profile.yaml` above (captured live output),
these five are illustrative — adapted from
[`docs/usage.md`](docs/usage.md)'s own Step 4* sections rather than
freshly captured from a live run. Their shape and
field names are accurate; regenerating them from an actual `trace` run
is tracked as [good first issue #94](https://github.com/idriss-eliguene/landlock-genprof/issues/94).

### The `SecurityProfileProposal` — the actual primary artifact

Every `trace` run publishes **all four applyable artifacts together**
as one cluster object ([proposal publishing](docs/usage/proposal-publishing.md)) — this, not the
separate local files, is the artifact this tool is really built around:
reviewable via `kubectl`/GitOps, one `kubectl get -o yaml` away instead
of five separate files to track down.

See [`examples/nginx-generated-proposal.yaml`](examples/nginx-generated-proposal.yaml)
for the complete object — `spec.podLock`/`networkPolicy`/
`patchedManifest`/`spoSeccompProfile` each hold the exact rendered YAML
of the corresponding artifact as a plain string for review and inspection.
Those strings are not independently authorized rollout artifacts. For the
governed proposal workflow, obtain the Candidate digest from `review`, approve
that digest explicitly, then apply with `apply-proposal`.

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

## 11. Upgrading to v0.2

**Proposals generated before v0.2 must be regenerated.** They embed the SPO
v0.8.4 shape — a namespaced `v1beta1` `SeccompProfile` and an
`operator/<namespace>/<name>.json` localhost profile path. `SeccompProfile`
became cluster-scoped at SPO v0.9.0, and the governed apply path refuses to
reinterpret the old path rather than bind a workload to a profile whose
readiness was never established. Such a proposal now fails closed.

There is **no rewrite or re-approval migration**, deliberately: rewriting a
stored proposal would change the candidate and therefore invalidate its
approved digest, which is the mechanism working, not a defect. Re-run
`trace` to produce a fresh candidate and approve it.

### What v0.2 does not claim

- **Universal merged-profile least privilege** — not claimed. Merged
  `Containers` provenance is recording-level, contributor lineage is
  unavailable, and widening remains review-visible.
- **Syscall coverage as lineage, confidence, or authority** — not claimed.
  Coverage is optional informational provenance and does not become
  `TrainingHistory` evidence.
- **Seccomp semantic diff** — not implemented. `diff` compares Landlock
  candidates; seccomp changes are shown through provenance.
- **Universal Seccomp enforcement** — not claimed. A bounded real-node
  experiment demonstrates `getpid` allowed and naturally absent `getpriority`
  rejected with `EPERM` under one approved candidate.
- **SPO recording on kind** — not supported. SPO's eBPF recorder resolves
  container pids through `/proc`, which cannot work when the node is itself
  a container. The D-MIN and demo suites use a real-node cluster.

---

## 12. License

Dual-licensed, recipient's choice: [Apache-2.0](LICENSE-APACHE) **or**
[MIT](LICENSE-MIT) — see [`COPYRIGHT`](COPYRIGHT). Compatible with
PodLock and the CNCF ecosystem.
