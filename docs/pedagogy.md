# Course context — team, original risk plan, original milestones

Split out of `README.md`. This project started as a 3-student course
project (see [`HOW_TO_START.md`](../HOW_TO_START.md) for their onboarding)
before also taking on a product/CNCF trajectory (see
[`product-definition-v1.md`](product-definition-v1.md)) — this document is
the pedagogical planning record from that original context. It doesn't
belong in the main product-facing README, but the content itself is real
and still useful for the students actually on this project.

**For current, up-to-date status, see [`roadmap.md`](roadmap.md)** — the
milestone table below is the *original* week-1 planning snapshot, kept
as a historical record, not a live status page. Nearly everything in it
is done and has been for a while; `roadmap.md` is what actually tracks
that.

## Team and task assignment

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

## Risk management (original plan)

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

This risk didn't materialize — the tracer has worked via Inspektor
Gadget since M1, confirmed live repeatedly (see `roadmap.md`). Kept here
as a planning record, not a live concern.

### Secondary risk: completeness of generated profiles

A short training run does not cover all code paths (error handling, edge cases,
rarely-triggered behaviour). An incomplete profile may break the application in
production on an unobserved path. Mitigation: the `Confidence` field per rule makes
this risk **visible** in the YAML rather than giving a false impression of
completeness. See [`threat-model.md`](threat-model.md).

This one is still live, not just original planning — it's a fundamental
property of observation-based profile generation, not something a
checkpoint resolves.

## Milestones (original week-1 planning snapshot)

| Milestone | Content | Owner |
|---|---|---|
| **M0 — Setup** | Repo, CI, `go.mod` with real dependencies, `hack/check-kernel.sh`, kind cluster | All |
| ⚠️ **Checkpoint week 3-4** | Tracer produces real events on at least `openat`. Otherwise: switch to `strace` fallback | Student A |
| **M1** | Working tracer (`openat` + `connect`), end-to-end `trace` CLI on an nginx pod | A + B |
| **M2** | Policy synthesis (directory-level aggregation, confidence levels), PodLock YAML export | B + C |
| **M3** | Full K8s integration (pod resolution via client-go, minimal tracer RBAC) | B + C |
| **M4** | End-to-end demo on kind — generated profile for nginx, comparison with a hand-written profile | All |
| **M5 _(stretch)_** | Post-deployment drift detection: Landlock denial logs → policy adjustment suggestions | All |

All of M0-M4 is done. See [`roadmap.md`](roadmap.md) for what was
actually built at each stage (considerably more than this original
snapshot anticipated — network/seccomp/capabilities exporters, the
Helm chart, `--history`/`--restart`, the `SecurityProfileProposal`
model, none of which were planned at this level of detail up front).
M5 remains the one open item, still genuinely a stretch goal.
