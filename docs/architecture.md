# Architecture

Ce document décrit l'architecture **cible** du pipeline (jalons M1-M4, voir
[`roadmap.md`](roadmap.md)). À date, seuls les types et les signatures de
fonctions existent dans le code (`panic("not implemented")` partout) — voir
la légende de chaque diagramme pour ce qui est réellement câblé.

---

## 1. Flux de données — composants et frontière de confiance

```mermaid
flowchart TD
    subgraph cluster["Cluster Kubernetes (kind)"]
        POD["Pod cible<br/>(ex: nginx-demo)"]
        API["kube-apiserver"]
    end

    subgraph host["Kernel hôte (Ubuntu 24.04 / 6.8)"]
        EBPF["Gadgets eBPF — Inspektor Gadget<br/>trace_open · trace_tcpconnect · trace_bind · trace_exec"]
    end

    CLI["cmd/landlock-genprof<br/>✅ CLI trace (cobra, câblée)"]
    K8SPKG["internal/k8s<br/>✅ Resolve()"]
    TRACER["internal/tracer<br/>🚧 Trace()"]
    POLICY["internal/policy<br/>✅ Synthesize()"]
    PODLOCKTYPES["pkg/podlock<br/>✅ types LandlockProfile"]
    YAML["profile.yaml"]
    HUMAN(["Revue humaine — obligatoire"])
    PODLOCKOP["PodLock operator<br/>(Kubewarden, externe)"]

    CLI --> K8SPKG
    K8SPKG -. "résout pod/namespace/container" .-> API
    CLI --> TRACER
    TRACER -. "attache les gadgets" .-> EBPF
    EBPF -. "observe les syscalls" .-> POD
    EBPF -- "[]Event" --> TRACER
    TRACER -- "[]Event" --> POLICY
    POLICY -- "[]Rule + Confidence" --> PODLOCKTYPES
    PODLOCKTYPES --> YAML
    YAML --> HUMAN
    HUMAN -- "kubectl apply" --> PODLOCKOP
    PODLOCKOP -. "enforcement Landlock au runtime" .-> POD

    style EBPF fill:#f9d5a7,stroke:#333
    style HUMAN fill:#c8e6c9,stroke:#333
```

**Légende :** ✅ implémenté · 🚧 types/signatures définis, logique = stub
(`panic("not implemented")`).

**Frontière de confiance à noter** (détail dans
[`threat-model.md`](threat-model.md)) : le tracer nécessite des capacités
élevées (`CAP_BPF`, `CAP_SYS_ADMIN` selon le kernel) pour attacher les
gadgets eBPF — c'est la seule brique du pipeline qui touche directement au
kernel hôte et au pod observé. Tout le reste (synthèse, génération YAML)
tourne avec les privilèges normaux du process CLI.

---

## 2. Séquence d'un training run complet

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
    Tracer->>IG: attache trace_open / trace_tcpconnect / trace_bind
    loop pendant Duration
        IG-->>Tracer: Event{Syscall, Path, Port, Mode}
    end
    Tracer-->>CLI: []Event
    CLI->>Policy: Synthesize([]Event)
    Note over Policy: agrégation par répertoire<br/>+ calcul de Confidence
    Policy-->>CLI: []Rule{Path, Access, Confidence}
    CLI->>FS: écrit LandlockProfile (YAML, format PodLock)
    Dev->>FS: revue humaine — vérifie les règles `low`/`medium`
    Dev->>Dev: kubectl apply (déploiement via PodLock, hors scope CLI)
```

Le CLI **s'arrête à l'écriture du YAML** — il n'appelle jamais `kubectl
apply` lui-même (voir README §5, "revue humaine obligatoire").

---

## 3. Dépendances entre packages Go

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

`internal/policy` importe désormais `pkg/podlock` (`ToProfile`/`ToYAML`,
voir `internal/policy/export.go`) — le pont vers `LandlockProfile` annoncé
comme "prévu M2" est câblé. `cmd/landlock-genprof` ne dépend de `podlock`
que transitivement (via la valeur retournée par `policy.ToProfile`) : il
n'a jamais besoin de l'importer directement, Go ne l'exige pas pour
manipuler une valeur d'un type qu'on ne nomme pas explicitement.

`Synthesize()` (agrégation événements → règles) et le CLI `trace` (voir
`cmd/landlock-genprof/trace.go`) sont implémentés — voir
[`docs/policy-synthesis.md`](policy-synthesis.md) pour le détail de
l'algorithme de synthèse et ses limites connues (heuristique de confiance
mono-run, profondeur d'agrégation empirique). Seul `internal/tracer.Trace()`
reste un stub : le CLI l'appelle et propage son `panic` tel quel, ce qui est
volontaire (voir docs/policy-synthesis.md et la note dans trace.go).
