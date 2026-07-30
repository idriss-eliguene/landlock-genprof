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

## Why

{{#include ../../docs/product-definition-v1.md:9:19}}

See the [full product definition](docs/product-definition-v1.md) for
positioning against PodLock/SPO/static compliance scanners, MVP scope,
and where this is headed.

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
| **No cluster yet** | [Set up a test environment](docs/test-environment.md) — disposable `kind` cluster, from nothing. |
| **Already have a cluster** | [Install](INSTALL.md) — get the CLI, apply the RBAC/CRDs. |
| **Usage reference** | [Usage reference](docs/usage.md) — every `trace` flag, one section each. |
| **Architecture** | [Overview](docs/architecture.md) — components and their interactions, at a glance. Deep dives (full data flow, sequence diagram, package deps, policy synthesis) are nested underneath, for contributors. |
| **Contributing to the code** | [Quickstart (English)](HOW_TO_START.md) / [Guide FR](COMMENT_COMMENCER.md) — git workflow, code walkthrough, first tasks. |

Source repository: [github.com/idriss-eliguene/landlock-genprof](https://github.com/idriss-eliguene/landlock-genprof).
