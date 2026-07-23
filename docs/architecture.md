# Architecture

This document describes the pipeline architecture (milestones M1-M4, see
[`roadmap.md`](roadmap.md)) — see each diagram's legend for what's actually
wired up vs still planned.

---

## 1. Data flow — components and trust boundary

```mermaid
flowchart TD
    subgraph cluster["Kubernetes cluster (kind)"]
        POD["Target pod<br/>(e.g. nginx-demo)"]
        API["kube-apiserver"]
    end

    subgraph host["Host kernel (Linux ≥ 6.8, tested on Ubuntu 24.04)"]
        EBPF["eBPF gadgets — Inspektor Gadget<br/>trace_open · trace_exec · trace_tcpconnect · trace_bind"]
    end

    CLI["cmd/landlock-genprof<br/>✅ CLI trace (cobra, wired up)"]
    K8SPKG["internal/k8s<br/>✅ Resolve()"]
    TRACER["internal/tracer<br/>✅ Trace() (Linux only)"]
    POLICY["internal/policy<br/>✅ Synthesize()"]
    IR["internal/profile<br/>✅ BehaviorProfile (IR)"]
    EXPORTER["internal/exporter/podlock<br/>✅ ToProfile()/ToYAML()"]
    PODLOCKTYPES["pkg/podlock<br/>✅ LandlockProfile types"]
    YAML["profile.yaml"]
    NETEXPORTER["internal/exporter/networkpolicy<br/>✅ ToPolicy()/ToYAML() — opt-in, --network-out"]
    NETYAML["networkpolicy.yaml"]
    HUMAN(["Human review — mandatory"])
    PODLOCKOP["PodLock operator<br/>(Kubewarden, external)"]

    CLI --> K8SPKG
    K8SPKG -. "resolves pod/namespace/container" .-> API
    CLI --> TRACER
    TRACER -. "attaches gadgets" .-> EBPF
    EBPF -. "observes syscalls" .-> POD
    EBPF -- "[]Event" --> TRACER
    TRACER -- "[]Event" --> POLICY
    POLICY -- "BehaviorProfile" --> IR
    IR -- "BehaviorProfile.Filesystem" --> EXPORTER
    EXPORTER --> PODLOCKTYPES
    PODLOCKTYPES --> YAML
    IR -- "BehaviorProfile.Network" --> NETEXPORTER
    NETEXPORTER --> NETYAML
    YAML --> HUMAN
    NETYAML --> HUMAN
    HUMAN -- "kubectl apply" --> PODLOCKOP
    PODLOCKOP -. "Landlock enforcement at runtime" .-> POD

    style EBPF fill:#f9d5a7,stroke:#333
    style HUMAN fill:#c8e6c9,stroke:#333
```

**Legend:** ✅ implemented · 🚧 types/signatures defined, logic = stub
(`panic("not implemented")`).

Note on `trace_tcpconnect`/`trace_bind`: their field names in
`internal/tracer/trace_linux.go` (`dst.port`, `addr.port` — both nested,
neither the flat name first guessed) are now confirmed against a live
cluster, the same way `trace_open`/`trace_exec`'s were (see
`docs/roadmap.md` M1). A wrong field name now fails with a clear error
(`requireField`) instead of a nil-pointer panic.

**Trust boundary worth noting** (details in
[`threat-model.md`](threat-model.md)): the tracer needs elevated
capabilities (`CAP_BPF`, `CAP_SYS_ADMIN` depending on the kernel) to attach
eBPF gadgets — it's the only piece of the pipeline that touches the host
kernel and the observed pod directly. Everything else (synthesis, YAML
generation) runs with the CLI process's normal privileges.

---

## 2. Sequence of a full training run

```mermaid
sequenceDiagram
    actor Dev
    participant CLI as cmd/landlock-genprof
    participant K8s as internal/k8s
    participant Tracer as internal/tracer
    participant IG as Inspektor Gadget (eBPF)
    participant Policy as internal/policy
    participant Exp as internal/exporter/podlock
    participant FS as profile.yaml
    participant NetExp as internal/exporter/networkpolicy
    participant NetFS as networkpolicy.yaml

    Dev->>CLI: trace --pod nginx-demo --duration 60s --network-out networkpolicy.yaml
    CLI->>K8s: Resolve(namespace, pod, container)
    K8s-->>CLI: TargetPod{..., Labels}
    CLI->>Tracer: Trace(Options{PodName, Duration, ...})
    par concurrently
        Tracer->>IG: kubectl gadget run trace_open:latest -n ... -c ...
        loop for Duration
            IG-->>Tracer: Event{Syscall: "openat", Path, Mode: read/write/read_write}
        end
    and
        Tracer->>IG: kubectl gadget run trace_exec:latest --paths -n ... -c ...
        loop for Duration
            IG-->>Tracer: Event{Syscall: "execve", Path, Mode: "exec"}
        end
    and
        Tracer->>IG: kubectl gadget run trace_tcpconnect:latest -n ... -c ...
        loop for Duration
            IG-->>Tracer: Event{Syscall: "connect", Port, Mode: "egress"}
        end
    and
        Tracer->>IG: kubectl gadget run trace_bind:latest -n ... -c ...
        loop for Duration
            IG-->>Tracer: Event{Syscall: "bind", Port, Mode: "ingress"}
        end
    end
    Tracer-->>CLI: []Event (merged)
    CLI->>Policy: Synthesize([]Event)
    Note over Policy: filesystem: aggregation by directory<br/>network: aggregation by (port, direction)<br/>+ Confidence calculation for both
    Policy-->>CLI: BehaviorProfile{Filesystem, Network}
    CLI->>Exp: ToProfile(meta, BehaviorProfile.Filesystem)
    Note over Exp: IR permission set -><br/>PodLock's 4 joint categories
    Exp-->>CLI: *podlock.LandlockProfile
    CLI->>Exp: ToYAML(LandlockProfile)
    Exp-->>CLI: []byte
    CLI->>FS: writes LandlockProfile (YAML, PodLock format)
    opt --network-out set and Network.Accesses non-empty
        CLI->>NetExp: ToPolicy(meta, BehaviorProfile.Network)
        Note over NetExp: one port per Ingress/Egress rule,<br/>podSelector from the traced pod's own labels
        NetExp-->>CLI: *networkingv1.NetworkPolicy
        CLI->>NetExp: ToYAML(NetworkPolicy)
        NetExp-->>CLI: []byte
        CLI->>NetFS: writes NetworkPolicy (YAML)
    end
    Dev->>FS: human review — checks `low`/`medium` rules
    Dev->>NetFS: human review — checks generated ports/podSelector
    Dev->>Dev: kubectl apply (deployment via PodLock and/or NetworkPolicy, out of CLI scope)
```

The CLI **stops at writing the YAML** — it never calls `kubectl apply`
itself (see README §5, "mandatory human review").

**`internal/policy` produces a Behavior IR, not a PodLock-shaped output**
(see §3 below and `docs/policy-synthesis.md`): `Synthesize()` returns an
`internal/profile.BehaviorProfile`, oblivious to PodLock. Converting that
IR into PodLock's specific YAML shape — including collapsing a
read/write/execute permission *set* into one of PodLock's four joint
categories (`readOnly`/`readWrite`/`readExec`/`readWriteExec`) — is
entirely `internal/exporter/podlock`'s job.

Current scope: `Trace()` runs `trace_open` (file read/write access),
`trace_exec` (file execute access), `trace_tcpconnect` (egress) and
`trace_bind` (ingress) concurrently, merging all four into a single
`[]Event`. PodLock's real CRD still has no field to represent network
rights (see `docs/policy-synthesis.md`) — that no longer blocks network
*tracing*, only the podlock exporter's own output, since
`internal/exporter/networkpolicy` gives the network half of the IR a
destination of its own.

Every one of the four gadgets is additionally scoped to the traced
binary's `comm` (`commFromBinaryPath`, `internal/tracer/trace_linux.go`),
not just the pod/namespace/container — Inspektor Gadget's own filter
can't distinguish the traced binary's own activity from a `kubectl exec`
session sharing the same namespaces. See `docs/e2e-demo.md` Finding 1 for
the real contamination this closes.

**Why two gadgets, not one:** `openat(2)` has no "exec" bit in its flags
(`O_ACCMODE` only distinguishes read/write/read_write — unlike FreeBSD,
Linux has no `O_EXEC`). `trace_open` alone can therefore never tell us a
path was *executed*; that signal only exists on `execve(2)`/`execveat(2)`,
which is what `trace_exec` hooks. This was found the hard way: an earlier
version of `Synthesize()` already had a `"exec"` `Mode` case and a
`readExec`/`readWriteExec` output category, exercised only by
hand-crafted unit test events — no real code path in `trace_linux.go`
could ever actually produce `Mode: "exec"` until `trace_exec` was wired
in. See `docs/policy-synthesis.md`.

---

## 3. Go package dependencies

```mermaid
flowchart LR
    cmd["cmd/landlock-genprof"]
    k8s["internal/k8s"]
    tracer["internal/tracer"]
    policy["internal/policy"]
    ir["internal/profile"]
    exporter["internal/exporter/podlock"]
    podlock["pkg/podlock"]
    netexporter["internal/exporter/networkpolicy"]
    netpolicyapi["k8s.io/api/networking/v1"]
    history["internal/history"]
    dynamicclient["k8s.io/client-go/dynamic"]

    cmd --> k8s
    cmd --> tracer
    cmd --> policy
    cmd --> exporter
    cmd --> netexporter
    cmd --> history
    policy --> tracer
    policy --> ir
    exporter --> ir
    exporter --> podlock
    netexporter --> ir
    netexporter --> netpolicyapi
    history --> ir
    history --> dynamicclient
    tracer -. "Linux build only" .-> k8s
```

**The Behavior IR (`internal/profile`) is the boundary between
observation and output format.** `internal/policy` turns raw
`tracer.Event`s into an `internal/profile.BehaviorProfile` and knows
nothing else — no `pkg/podlock`, no YAML, no Kubernetes types.
`internal/exporter/podlock` and `internal/exporter/networkpolicy` are the
only packages that depend on both `internal/profile` and an
output-specific type (`pkg/podlock`, or the already-vendored
`k8s.io/api/networking/v1` — no hand-rolled types package needed there,
unlike PodLock's CRD which isn't in any vendored library), and the
dependency only ever runs one way: exporter → IR. `internal/profile`
itself has zero knowledge that PodLock, `NetworkPolicy`, YAML, or
Kubernetes exist — enforced by a static import check in
`internal/profile/deps_test.go`, not just a convention. This is what let
`internal/exporter/networkpolicy` be added as a sibling of
`internal/exporter/podlock` without touching `internal/policy` or
`internal/profile`'s import graph at all — only their exported surface
grew (`BehaviorProfile.Network`). Cilium/`seccomp` remain unimplemented
future siblings of the same shape.

**`internal/history` is shaped like an exporter (depends on the IR, not
the other way — no changes needed to `internal/policy`/`internal/profile`
to add it), but it isn't one**: it reads back what it wrote on a previous
run (`history.Get`) as well as producing something new
(`history.Merge`/`Save`), and what it produces is consumed by nothing
downstream in this pipeline yet — `ApplyConfidence`'s output isn't wired
into either exporter (see `docs/policy-synthesis.md`). Its own
`k8s.io/client-go/dynamic` dependency is because `TrainingHistory` is
this project's own CRD with no generated typed client, unlike
`internal/k8s`'s typed `kubernetes.Interface` — the same reason
`internal/exporter/podlock` needed hand-rolled types for PodLock's CRD
but `internal/exporter/networkpolicy` didn't for the already-vendored
`NetworkPolicy` type.

`cmd/landlock-genprof` only depends on `pkg/podlock` transitively (via
the value returned by `podlock.ToProfile`, in `internal/exporter/podlock`):
it never needs to import `pkg/podlock` directly, since Go doesn't require
importing a package to hold a value of a type you never name explicitly.
Same reasoning for `internal/profile`: `cmd` holds a `BehaviorProfile`
value (returned by `policy.Synthesize`) without ever importing
`internal/profile` itself.

`internal/tracer.Trace()` calls `k8s.RestConfig()` to get the same
in-cluster/kubeconfig resolution `cmd`'s own client uses (factored into
`internal/k8s/config.go` specifically to avoid duplicating that logic in
both places).

### `internal/tracer` is split by build tag — and that's deliberate

- `tracer.go`: `Event`/`Options` types only, zero external imports.
- `trace_linux.go` (`//go:build linux`): the real implementation, using
  the Inspektor Gadget Go SDK (`pkg/gadget-context`, `pkg/runtime/grpc`,
  ...) to run `trace_open:latest`, `trace_exec:latest`,
  `trace_tcpconnect:latest`, and `trace_bind:latest` concurrently against
  the cluster's already-deployed Inspektor Gadget DaemonSet — the
  programmatic equivalent of running all four `kubectl gadget run ...`
  invocations side by side and merging their output.
- `trace_other.go` (`//go:build !linux`): returns a clear error instead of
  running anything.

This isn't cosmetic. The Inspektor Gadget SDK transitively pulls in
Linux-only syscall code (eBPF, cgroups, ...) that doesn't compile at all
on macOS/Windows — a plain `import` of it in a file with no build tag
would break `go build`/`go test` for **every** package that depends on
`internal/tracer`, which includes `internal/policy` (for the `Event`
type) and therefore `cmd` too. Splitting the file means only the real
capture logic is Linux-gated; the plain data types and anything built on
top of them keep compiling everywhere. This mirrors reality: Landlock and
eBPF only exist on Linux, so real tracer work only ever happens on the dev
VM (see `HOW_TO_START.md`) or in CI (`ubuntu-24.04`) — but that shouldn't
force every *other* package to become Linux-only along with it.
