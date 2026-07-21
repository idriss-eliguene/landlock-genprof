# Architecture

This document describes the **target** pipeline architecture (milestones M1-M4,
see [`roadmap.md`](roadmap.md)). As of now, only types and function signatures
exist in the code (`panic("not implemented")` everywhere) — see each diagram's
legend for what's actually wired up.

---

## 1. Data flow — components and trust boundary

```mermaid
flowchart TD
    subgraph cluster["Kubernetes cluster (kind)"]
        POD["Target pod<br/>(e.g. nginx-demo)"]
        API["kube-apiserver"]
    end

    subgraph host["Host kernel (Ubuntu 24.04 / 6.8)"]
        EBPF["eBPF gadgets — Inspektor Gadget<br/>trace_open · trace_tcpconnect · trace_bind · trace_exec"]
    end

    CLI["cmd/landlock-genprof<br/>✅ CLI trace (cobra, wired up)"]
    K8SPKG["internal/k8s<br/>✅ Resolve()"]
    TRACER["internal/tracer<br/>🚧 Trace()"]
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
    Tracer->>IG: attaches trace_open / trace_tcpconnect / trace_bind
    loop for Duration
        IG-->>Tracer: Event{Syscall, Path, Port, Mode}
    end
    Tracer-->>CLI: []Event
    CLI->>Policy: Synthesize([]Event)
    Note over Policy: aggregation by directory<br/>+ Confidence calculation
    Policy-->>CLI: []Rule{Path, Access, Confidence}
    CLI->>FS: writes LandlockProfile (YAML, PodLock format)
    Dev->>FS: human review — checks `low`/`medium` rules
    Dev->>Dev: kubectl apply (deployment via PodLock, out of CLI scope)
```

The CLI **stops at writing the YAML** — it never calls `kubectl apply`
itself (see README §5, "mandatory human review").

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
```

`internal/policy` now imports `pkg/podlock` (`ToProfile`/`ToYAML`, see
`internal/policy/export.go`) — the bridge to `LandlockProfile` previously
flagged as "planned M2" is wired up. `cmd/landlock-genprof` only depends
on `podlock` transitively (via the value returned by `policy.ToProfile`):
it never needs to import it directly, since Go doesn't require importing
a package to hold a value of a type you never name explicitly.

`Synthesize()` (event aggregation → rules) and the `trace` CLI (see
`cmd/landlock-genprof/trace.go`) are implemented — see
[`docs/policy-synthesis.md`](policy-synthesis.md) for the synthesis
algorithm's details and known limitations (single-run confidence
heuristic, empirical aggregation depth). Only `internal/tracer.Trace()`
remains a stub: the CLI calls it and propagates its `panic` as-is, which
is intentional (see docs/policy-synthesis.md and the note in trace.go).
