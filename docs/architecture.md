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

    subgraph host["Host kernel (Ubuntu 24.04 / 6.8)"]
        EBPF["eBPF gadgets — Inspektor Gadget<br/>trace_open · trace_exec"]
    end

    CLI["cmd/landlock-genprof<br/>✅ CLI trace (cobra, wired up)"]
    K8SPKG["internal/k8s<br/>✅ Resolve()"]
    TRACER["internal/tracer<br/>✅ Trace() (Linux only)"]
    POLICY["internal/policy<br/>✅ Synthesize()"]
    PODLOCKTYPES["pkg/podlock<br/>✅ LandlockProfile types"]
    YAML["profile.yaml"]
    HUMAN(["Human review — mandatory"])
    PODLOCKOP["PodLock operator<br/>(Kubewarden, external)"]

    CLI --> K8SPKG
    K8SPKG -. "resolves pod/namespace/container" .-> API
    CLI --> TRACER
    TRACER -. "attaches gadgets" .-> EBPF
    EBPF -. "observes syscalls" .-> POD
    EBPF -- "[]Event" --> TRACER
    TRACER -- "[]Event" --> POLICY
    POLICY -- "[]Rule + Confidence" --> PODLOCKTYPES
    PODLOCKTYPES --> YAML
    YAML --> HUMAN
    HUMAN -- "kubectl apply" --> PODLOCKOP
    PODLOCKOP -. "Landlock enforcement at runtime" .-> POD

    style EBPF fill:#f9d5a7,stroke:#333
    style HUMAN fill:#c8e6c9,stroke:#333
```

**Legend:** ✅ implemented · 🚧 types/signatures defined, logic = stub
(`panic("not implemented")`).

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
    participant FS as profile.yaml

    Dev->>CLI: trace --pod nginx-demo --duration 60s
    CLI->>K8s: Resolve(namespace, pod, container)
    K8s-->>CLI: TargetPod{...}
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
    end
    Tracer-->>CLI: []Event (merged)
    CLI->>Policy: Synthesize([]Event)
    Note over Policy: aggregation by directory<br/>+ Confidence calculation
    Policy-->>CLI: []Rule{Path, Access, Confidence}
    CLI->>FS: writes LandlockProfile (YAML, PodLock format)
    Dev->>FS: human review — checks `low`/`medium` rules
    Dev->>Dev: kubectl apply (deployment via PodLock, out of CLI scope)
```

The CLI **stops at writing the YAML** — it never calls `kubectl apply`
itself (see README §5, "mandatory human review").

Current scope: `Trace()` runs `trace_open` (file read/write access) and
`trace_exec` (file execute access) concurrently, merging both into a
single `[]Event`. Network gadgets (`trace_tcpconnect`/`trace_bind`) are
deliberately not implemented — PodLock's real CRD has no field to
represent network rights at all, see `docs/policy-synthesis.md`.

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
    podlock["pkg/podlock"]

    cmd --> k8s
    cmd --> tracer
    cmd --> policy
    policy --> tracer
    policy --> podlock
    tracer -. "Linux build only" .-> k8s
```

`internal/policy` imports `pkg/podlock` (`ToProfile`/`ToYAML`, see
`internal/policy/export.go`) for the bridge to `LandlockProfile`.
`cmd/landlock-genprof` only depends on `podlock` transitively (via the
value returned by `policy.ToProfile`): it never needs to import it
directly, since Go doesn't require importing a package to hold a value of
a type you never name explicitly.

`internal/tracer.Trace()` calls `k8s.RestConfig()` to get the same
in-cluster/kubeconfig resolution `cmd`'s own client uses (factored into
`internal/k8s/config.go` specifically to avoid duplicating that logic in
both places).

### `internal/tracer` is split by build tag — and that's deliberate

- `tracer.go`: `Event`/`Options` types only, zero external imports.
- `trace_linux.go` (`//go:build linux`): the real implementation, using
  the Inspektor Gadget Go SDK (`pkg/gadget-context`, `pkg/runtime/grpc`,
  ...) to run `trace_open:latest` and `trace_exec:latest` concurrently
  against the cluster's already-deployed Inspektor Gadget DaemonSet — the
  programmatic equivalent of running
  `kubectl gadget run trace_open:latest -n <ns> -c <container>` and
  `kubectl gadget run trace_exec:latest --paths -n <ns> -c <container>`
  side by side and merging their output.
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
