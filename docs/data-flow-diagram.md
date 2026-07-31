# Full data-flow diagram

Split out of [`architecture.md`](architecture.md) §1, which had grown
too long once the full implementation-level diagram sat right
alongside the overview one — this is every package, every generated
file, every RBAC boundary, for implementing or debugging the CLI
itself. For the high-level picture, [`architecture.md`](architecture.md)
§1's own first diagram is enough.

```mermaid
flowchart TD
    subgraph cluster["Kubernetes cluster (kind)"]
        POD["Target pod<br/>(e.g. nginx-demo)"]
        API["kube-apiserver"]
    end

    subgraph host["Host kernel (Linux ≥ 6.8, tested on Ubuntu 24.04)"]
        EBPF["eBPF gadgets — Inspektor Gadget<br/>trace_open · trace_exec · trace_tcpconnect · trace_bind ·<br/>advise_seccomp · trace_capabilities"]
    end

    CLI["cmd/landlock-genprof<br/>✅ CLI trace (cobra, wired up)"]
    K8SPKG["internal/k8s<br/>✅ Resolve() / PatchedManifest()"]
    TRACER["internal/tracer<br/>✅ Trace() (Linux only)"]
    POLICY["internal/policy<br/>✅ Synthesize()"]
    IR["internal/profile<br/>✅ BehaviorProfile (IR)"]

    subgraph exporters["internal/exporter/* — IR to local artifacts, one package per domain"]
        EXPORTER["podlock<br/>✅ filesystem"]
        NETEXPORTER["networkpolicy<br/>✅ network — opt-in, --network-out"]
        SECEXPORTER["seccomp<br/>✅ syscalls — opt-in, --seccomp-out"]
        SECCREXPORTER["spo<br/>✅ syscalls as SeccompProfile CR — opt-in, --seccomp-profile-out"]
        CAPEXPORTER["capabilities<br/>✅ capabilities — opt-in, --capabilities-out"]
        SCEXPORTER["securitycontext<br/>✅ capabilities + seccomp path reference —<br/>opt-in, --security-context-out"]
        REPEXPORTER["report<br/>✅ all 4 domains, always generated —<br/>opt-in file write, --report-out"]
    end

    YAML["profile.yaml"]
    NETYAML["networkpolicy.yaml"]
    SECJSON["seccomp.json"]
    SECCRYAML["{pod}-seccompprofile.yaml<br/>(SeccompProfile CR)"]
    CAPYAML["capabilities.yaml"]
    SCYAML["securitycontext.yaml"]
    REPMD["report.md"]
    PATCHFS["{identity}-patched.yaml<br/>live owner/bare-pod manifest,<br/>generated securityContext merged in"]

    PROP["internal/proposal<br/>✅ mandatory every run —<br/>re-renders each artifact independently"]
    K8SAPI["SecurityProfileProposal<br/>cluster object — spec.podLock / networkPolicy /<br/>patchedManifest / spoSeccompProfile"]

    HUMAN(["Human review — mandatory<br/>kubectl get securityprofileproposal / review command"])
    APPLYPROPOSAL["cmd/landlock-genprof apply-proposal<br/>✅ per-artifact create-or-update, retry on<br/>resourceVersion conflict — Patched Manifest<br/>opt-in only (--restart), --skip for the rest"]

    PODLOCKOP["PodLock operator<br/>(Kubewarden, external)"]
    CNI["CNI NetworkPolicy engine<br/>(e.g. Cilium, external)"]
    SPOOP["security-profiles-operator<br/>(external) — materializes localhostProfile<br/>on every node"]

    CLI --> K8SPKG
    K8SPKG -. "resolves pod/namespace/container" .-> API
    CLI --> TRACER
    TRACER -. "attaches gadgets" .-> EBPF
    EBPF -. "observes syscalls" .-> POD
    EBPF -- "[]Event" --> TRACER
    TRACER -- "[]Event" --> POLICY
    POLICY -- "BehaviorProfile" --> IR

    IR -- "Filesystem" --> EXPORTER --> YAML
    IR -- "Network" --> NETEXPORTER --> NETYAML
    IR -- "Syscalls" --> SECEXPORTER --> SECJSON
    SECEXPORTER -. "seccomp.Profile, CLI-mediated" .-> SECCREXPORTER
    SECCREXPORTER --> SECCRYAML
    IR -- "Capabilities" --> CAPEXPORTER --> CAPYAML
    CAPEXPORTER -. "ToProfile() reused directly —<br/>the one exporter-to-exporter dependency here" .-> SCEXPORTER
    SCEXPORTER --> SCYAML
    IR -- "all 4 domains" --> REPEXPORTER --> REPMD

    K8SPKG -- "live owner/bare-pod manifest" --> PATCHFS
    SCEXPORTER -. "generated securityContext" .-> PATCHFS

    EXPORTER -. "re-rendered, not read from YAML" .-> PROP
    NETEXPORTER -. "re-rendered, not read from YAML" .-> PROP
    SECCREXPORTER -. "re-rendered, not read from YAML" .-> PROP
    PATCHFS -. "same content embedded regardless of<br/>--patched-manifest-out" .-> PROP
    PROP -- "create-or-update" --> K8SAPI
    K8SAPI --> HUMAN
    REPMD -. "read alongside, if generated" .-> HUMAN

    HUMAN -- "kubectl landlock-genprof apply-proposal" --> APPLYPROPOSAL
    APPLYPROPOSAL -- "spec.podLock" --> PODLOCKOP
    APPLYPROPOSAL -- "spec.networkPolicy" --> CNI
    APPLYPROPOSAL -- "spec.spoSeccompProfile" --> SPOOP
    APPLYPROPOSAL -. "spec.patchedManifest, only with --restart<br/>(rollout for owned pods, delete+recreate for a bare pod)" .-> API

    PODLOCKOP -. "Landlock enforcement at runtime" .-> POD
    CNI -. "network enforcement" .-> POD
    SPOOP -. "writes localhostProfile,<br/>referenced by the patched manifest" .-> POD
    API -. "rolls out merged securityContext" .-> POD

    style EBPF fill:#f9d5a7,stroke:#333
    style HUMAN fill:#c8e6c9,stroke:#333
    style APPLYPROPOSAL fill:#c8e6c9,stroke:#333
```

**Legend:** ✅ implemented — every component below is (no stubs left as
of this writing; a future stub would use 🚧). Dotted arrows are indirect/out-of-process
relationships (network calls, reused-but-not-piped data, external
controllers reconciling); solid arrows are direct data flow within the
CLI process. `{pod}`/`{identity}` mean the same thing as `<pod>`/
`<identity>` used in prose elsewhere in this repo — mermaid's parser
doesn't accept angle brackets inside node/participant labels (confirmed:
they broke rendering on GitHub), so both diagrams in this doc use braces
instead.

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

**PodLock/CNI/SPO in this diagram are external and, except for the CNI,
not installed by this repo** — see
[`enforcement-prerequisites.md`](enforcement-prerequisites.md) for
what each actually needs, and in PodLock's case, why its own docs advise
against this project's `kind`-based reference environment entirely.
