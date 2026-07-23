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
- [x] **M1**: tracer functional on `openat` (`connect`/`bind` descoped, see
      below — not a PodLock-representable output), `trace` CLI working end
      to end on a test pod (nginx)
      - [x] `trace` CLI wired up with `cobra` (`cmd/landlock-genprof/trace.go`):
        `Resolve()` → `Trace()` → `Synthesize()` → `ToProfile`/`ToYAML` →
        writing the output file.
      - [x] Manual proof that Inspektor Gadget captures real events on this
        cluster (see checkpoint above) — de-risked the actual SDK integration
      - [x] `internal/tracer.Trace()` implemented for `openat` (`trace_open`
        gadget) via the Inspektor Gadget Go SDK (gRPC runtime against the
        cluster's DaemonSet) — see `trace_linux.go` and
        `docs/architecture.md` §3 for the build-tag split (Linux-only, by
        necessity: the SDK doesn't compile on macOS/Windows)
      - [x] `trace_exec` gadget added alongside `trace_open` (run
        concurrently, merged into a single `[]Event`): `openat(2)` has no
        "exec" bit in its flags, so `trace_open` alone could never
        produce a `Mode: "exec"` event — found by trying to actually
        trigger a `readWriteExec` rule on the live cluster after the
        schema-alignment fix below, which exposed that `readExec`/
        `readWriteExec` had never been reachable from real data, only
        from hand-crafted test fixtures. See `docs/policy-synthesis.md`.
      - [x] `connect`/`bind` (network) via `trace_tcpconnect`/`trace_bind`
        — initially deferred: PodLock's real CRD schema
        (`github.com/flavio/podlock`) has no field to represent Landlock
        network rights at all, verified directly against its source
        rather than assumed, so capturing network events would have
        produced data with nowhere to go. That blocker was specific to
        the PodLock exporter, not to network support in general — see
        M2's `internal/exporter/networkpolicy` entry below, which gives
        this data its own destination. **Field names confirmed against
        the live cluster**: `trace_tcpconnect`'s destination port
        (`dst.port`, a nested field — first guessed as flat `dport`,
        wrong) and `trace_bind`'s bound port (`addr.port` — first guessed
        as flat `port`, which doesn't exist and crashed with a nil
        pointer dereference before `requireField` was added to fail
        cleanly instead) were both confirmed via `kubectl gadget run
        ... -o json` and a real end-to-end `trace` run producing a
        correct `NetworkPolicy`. Along the way, found and fixed a real
        false positive: an outbound `nc` connection produced a spurious
        `bind` event on its own ephemeral source port, indistinguishable
        from a real listener at the syscall level — filtered by port
        range in `internal/policy.Synthesize` (`ephemeralPortStart`, see
        `docs/policy-synthesis.md`).
      - [x] **First full pipeline run validated on the live cluster**:
        `go run ./cmd/landlock-genprof trace --pod nginx-demo --binary
        /usr/sbin/nginx` against real activity (`kubectl exec nginx-demo --
        ls /etc`) produced a correct `profile.yaml` end to end. Surfaced
        and fixed a real bug in the process (directory-open aggregated to
        its own parent, producing `readOnly: [/]` — see
        `docs/policy-synthesis.md`), which no hand-crafted unit test had
        caught.
- [x] **M2**: policy synthesis (aggregation by directory, confidence
      levels), YAML export in PodLock format — `internal/policy.Synthesize`,
      `ToProfile`/`ToYAML` (see `docs/policy-synthesis.md`)
      - [x] **Architecture evolution — Behavior IR**: `Synthesize` used to
        return a PodLock-shaped `[]Rule` directly, coupling the
        observation/aggregation layer to one specific output format.
        Introduced `internal/profile` (a technology-neutral
        `BehaviorProfile`/`FilesystemProfile`/`FileAccess` IR, statically
        checked to have zero dependency on PodLock/YAML/Kubernetes — see
        `internal/profile/deps_test.go`) and isolated all PodLock-specific
        conversion in `internal/exporter/podlock`. `Synthesize` now
        produces the IR; the exporter alone decides how a permission set
        maps to PodLock's four joint categories. Prepared the ground for
        future exporters without having implemented any of them yet.
      - [x] **First real second exporter — `internal/exporter/networkpolicy`**:
        `BehaviorProfile` gained a `Network` field (`NetworkProfile`/
        `NetworkAccess`, one entry per observed `(port, direction)` pair)
        and `Synthesize` now aggregates `connect`/`bind` events into it,
        alongside the filesystem half — the network tracing this M1 note
        used to say was "out of scope" is back in scope now that this
        exporter gives it a destination PodLock never had (see M1 above).
        `internal/exporter/networkpolicy.ToPolicy` maps it to a
        Kubernetes `NetworkPolicy` (`podSelector` from the traced pod's
        own labels, one port per observed access, no `From`/`To` peer
        restriction since only a port was ever observed, not a peer
        identity). Wired into the CLI behind an opt-in `--network-out`
        flag (`cmd/landlock-genprof/trace.go`) — unlike the PodLock
        profile, a `NetworkPolicy` is something a cluster admin has to
        choose to apply, not something generated unconditionally. Cilium/
        `seccomp` remain unimplemented future siblings. See
        `docs/architecture.md` §3 and `docs/policy-synthesis.md`.
- [x] **M3**: full K8s integration (target pod resolution, tracer's
      minimal RBAC — see `docs/threat-model.md`)
      - [x] `internal/k8s.Resolve`: checks that the pod exists, is
        `Running`, and that the target container exists (or is deduced if
        there's only one) — tested with client-go's `fake` clientset, no
        real cluster
      - [x] Tracer's actual minimal RBAC (ServiceAccount/Role/RoleBinding) —
        [`deploy/rbac.yaml`](../deploy/rbac.yaml), each rule traced back
        to a specific API call in the code (not "grant broadly to be
        safe") — see `docs/threat-model.md` §1. Applied to the live
        cluster and verified with `kubectl auth can-i --as=...`: all 6
        checks (3 positive, 3 negative) match the manifest exactly —
        including confirming `pods/portforward` needs
        `--subresource=portforward` in `can-i`, not the `pods/portforward`
        slash form, which is a `kubectl auth can-i` CLI quirk, not a gap
        in the Role itself.
      - [x] Full functional test of `trace` using *only* this restricted
        ServiceAccount's token (`kubectl create token` + a scoped
        kubeconfig, no admin access): the full pipeline ran without a
        single permission error. **M3 complete.**
- [x] **M4**: e2e demo on `kind` — profile generated for nginx, compared
      against a hand-written profile, gaps documented — see
      `docs/e2e-demo.md`. Two real findings: `kubectl exec` activity
      during a training run leaks into the traced binary's profile as
      false-positive `readExec` rules (the tracer scopes events by
      pod/container, not by process — see `internal/tracer/trace_linux.go`),
      and resources opened once at container startup (pid file, log fd)
      are invisible to a trace that attaches after the container is
      already running. Both logged as methodology risks in
      `docs/threat-model.md` §2.
      - [x] **Finding 1 fixed at the tracer level, verified live**: all
        four `run*Tracer` functions (`internal/tracer/trace_linux.go`)
        now additionally scope capture to the traced binary's `comm`
        (`commFromBinaryPath`, field `proc.comm` — confirmed via
        `kubectl gadget run trace_open:latest -o json`), closing the
        `kubectl exec` contamination for both the PodLock and
        `NetworkPolicy` outputs. Re-running M4's exact scenario against
        the live cluster confirmed the fix and, unexpectedly, exposed a
        deeper flaw in the original methodology itself: `ls`/`cat` via
        `kubectl exec` never actually exercised nginx at all (empty
        profile once correctly excluded) — real traffic
        (`wget` to nginx) was needed to produce a genuine,
        correctly-attributed `readOnly: [/usr/share/nginx]`. See
        `docs/e2e-demo.md`'s Finding 1 update. Finding 2 (startup blind
        spot) remains open.
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
