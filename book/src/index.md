# landlock-genprof

Automatic Kubernetes security profile generator — [Landlock](https://landlock.io/),
`NetworkPolicy`, seccomp, and Linux capabilities — built on **observation** of a
running pod (a "training run") rather than manual rule authoring.

```text
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

The name is a deliberate nod to `aa-genprof` / `aa-logprof` — the AppArmor
profile generation tools. Landlock had no equivalent when this started,
and filling that gap is where the name comes from — the tool itself has
since grown to cover network, syscalls, and capabilities from the same
training run, not just Landlock's own filesystem/network rights.

> **Status:** the observe → synthesize → export pipeline is built and
> confirmed end to end on a live cluster (filesystem, network, seccomp,
> capabilities, cross-run confidence via `--history`), tagged `v0.1.0`.
> [Roadmap](docs/roadmap.md) tracks what's actually built, milestone by
> milestone — the source of truth over anything else on this site.

## Where to start

| | |
|---|---|
| **Install** | [Install](INSTALL.md) — already have a cluster? Start here. |
| **Quickstart** | [Quickstart](HOW_TO_START.md) — no cluster yet, walks through provisioning one. |
| **Usage reference** | [Usage reference](docs/usage.md) — every `trace` flag, one section each. |
| **Architecture** | [Overview](docs/architecture.md) — data flow, [sequence diagram](docs/sequence-diagram.md), [package deps](docs/packages.md). |
| **Demo** | [Demo script](demo/script.md) — a 75s walkthrough, plus the [full end-to-end run](docs/e2e-demo.md). |

Source repository: [github.com/idriss-eliguene/landlock-genprof](https://github.com/idriss-eliguene/landlock-genprof).
