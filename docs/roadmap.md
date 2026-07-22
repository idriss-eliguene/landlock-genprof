# Roadmap

## Architecture decisions made

- Tracer based on the existing **Inspektor Gadget** gadgets (`trace_open`,
  `trace_tcpconnect`, ...) rather than an eBPF program written from
  scratch — greatly reduces failure risk for a team starting out with eBPF.
- Output in a **PodLock-compatible format** (`LandlockProfile` CRD,
  Kubewarden ecosystem) — the project is complementary, not a competitor.
- No automatic policy application: mandatory human review.

## Milestones

- [x] **M0 — Setup**: repo, license, GitHub Actions CI
      (`runs-on: ubuntu-24.04` to guarantee a kernel ≥ 6.8),
      `hack/check-kernel.sh` script, dev `kind` cluster — cluster + Inspektor
      Gadget deployed and verified working via `hack/init-vm.sh`
- [x] **⚠️ Hard checkpoint — week 3-4**: the tracer (Student A) must
      produce real events for at least one syscall type (e.g. `openat`),
      even minimal. **Cleared manually**: `kubectl gadget run trace_open:latest
      -n default -c nginx-demo` captures real `openat` events (confirmed
      `ls /etc` inside the container showing up live). This was done via the
      `kubectl gadget` CLI directly, not yet through `internal/tracer.Trace()`
      — the Go SDK integration is the remaining M1 work, but the fallback
      plan below is no longer needed: Inspektor Gadget works on this setup.
- [ ] **M1**: tracer functional on `openat`/`connect`, `trace` CLI
      working end to end on a test pod (nginx)
      - [x] `trace` CLI wired up with `cobra` (`cmd/landlock-genprof/trace.go`):
        `Resolve()` → `Trace()` → `Synthesize()` → `ToProfile`/`ToYAML` →
        writing the output file. `Trace()` is still a stub that panics
        — that's M1's remaining blocker, not the wiring.
      - [x] Manual proof that Inspektor Gadget captures real events on this
        cluster (see checkpoint above) — de-risks the actual SDK integration
      - [ ] Replace `panic("not implemented")` in `internal/tracer.Trace()`
        with a real call to the Inspektor Gadget Go SDK, equivalent to what
        `kubectl gadget run trace_open:latest -c <container>` just did manually
- [x] **M2**: policy synthesis (aggregation by directory, confidence
      levels), YAML export in PodLock format — `internal/policy.Synthesize`,
      `ToProfile`/`ToYAML` (see `docs/policy-synthesis.md`)
- [ ] **M3**: full K8s integration (target pod resolution, tracer's
      minimal RBAC — see `docs/threat-model.md`)
      - [x] `internal/k8s.Resolve`: checks that the pod exists, is
        `Running`, and that the target container exists (or is deduced if
        there's only one) — tested with client-go's `fake` clientset, no
        real cluster
      - [ ] Tracer's actual minimal RBAC (ServiceAccount/Role/RoleBinding) —
        see `docs/threat-model.md`
- [ ] **M4**: e2e demo on `kind` — profile generated for nginx, compared
      against a hand-written profile, gaps documented
- [ ] **M5 (stretch)**: post-deployment drift detection (Landlock denial
      logs → suggested policy adjustment)

## Fallback plan if the M0→M1 checkpoint fails

If the eBPF tracer (even via Inspektor Gadget) isn't working by the week
3-4 checkpoint: switch event capture to `strace -f` with output parsing.
Less elegant, but sufficient for a one-off training run (no production
performance constraint), and it lets Students B and C keep moving without
blocking on Student A.

## Task assignment

| Role | Component | Student |
|---|---|---|
| eBPF tracer | `internal/tracer/` | Student A |
| CLI + K8s integration | `cmd/`, `internal/k8s/`, `internal/policy/` | Student B |
| Methodology / security | `docs/threat-model.md`, adversarial tests | Student C |
