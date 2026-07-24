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
        choose to apply, not something generated unconditionally. Cilium
        remains an unimplemented future sibling. See
        `docs/architecture.md` §3 and `docs/policy-synthesis.md`.
      - [x] **Third exporter — `internal/exporter/seccomp`**: reuses
        Inspektor Gadget's own purpose-built `advise_seccomp` gadget
        (confirmed against its vendored source, not reimplemented from
        scratch) instead of raw syscall tracing — the same underlying
        method security-profiles-operator's own recording flow uses.
        `BehaviorProfile` gained a `Syscalls` field (`SyscallProfile`/
        `SyscallAccess`, one entry per observed syscall name, plus the
        node's reported `Architectures`); `Synthesize` now aggregates
        `Mode: "syscall"` events into it, and `tracer.Trace()` grew a
        second `[]string` return value carrying the architecture list (a
        per-run, not per-event, fact). Genuine departure from the other
        two exporters: `advise_seccomp` reports one deduplicated syscall
        set per run rather than per-occurrence events, so `SeenCount` is
        always 1 and Confidence is always `low` without `--history` —
        intentional, not a bug (a single run can't prove a syscall is
        safe to omit). `internal/exporter/seccomp.ToJSON` stays plain
        JSON (no `# confidence: ...` comment — the file must stay
        loadable as-is by the kubelet/container runtime); the CLI prints
        not-yet-confirmed syscalls to stdout instead
        (`writeSeccompProfile`). Wired in behind an opt-in `--seccomp-out`
        flag, same `NoOptDefVal` pattern as `--network-out`. `internal/history`
        extended with a third `SyscallAccesses` domain, same shape as
        `NetworkAccesses`; `deploy/crd-traininghistory.yaml`'s schema
        updated to match (additive, re-`kubectl apply` is enough). No new
        RBAC. One real caveat, confirmed from `advise_seccomp`'s own
        source, not this project's code: its eBPF probe observes every
        process on the node during the run, not just the target container
        — see `docs/threat-model.md` §1. See `docs/architecture.md` §3.
      - [x] **Fourth exporter — `internal/exporter/capabilities`**:
        the product-vision discussion that led to the seccomp exporter
        named four observation dimensions, not three — filesystem,
        network, syscalls, and Linux capabilities. Reuses Inspektor
        Gadget's `trace_capabilities` gadget (confirmed against its
        vendored source), a normal streaming, in-kernel container-filtered
        gadget — much closer in shape to `trace_open` than to
        `advise_seccomp`'s flush-on-stop exception, so `tracer.Trace()`'s
        signature needed no further change: capabilities ride the
        existing `[]Event` stream (`Mode: "capability"`), unlike
        `advise_seccomp`'s architecture list. `BehaviorProfile` gained a
        `Capabilities` field (`CapabilityProfile`/`CapabilityAccess`);
        `Synthesize` aggregates by capability name with real
        per-occurrence `SeenCount` (not the seccomp exception — a
        capability check can genuinely repeat within one run). Reuses the
        already-vendored `k8s.io/api/core/v1.Capabilities` type rather
        than hand-rolling one. Real structural difference from the other
        three: Linux capabilities aren't a standalone Kubernetes object,
        only ever a sub-field of a container's own securityContext — so
        `internal/exporter/capabilities.ToYAML` produces a bare
        `add`/`drop` fragment (`drop: [ALL]` always) for a human to paste
        directly under `securityContext.capabilities:`, not a complete
        applyable resource (confirmed this shape with the project owner,
        the alternative considered being a ready-to-run `kubectl patch`
        command instead). Since this fragment is human-pasted rather than
        loaded by the kubelet/runtime, it keeps the same
        `# confidence: ...` YAML comment mechanism podlock/networkpolicy
        use — no seccomp-style JSON constraint. Wired in behind an opt-in
        `--capabilities-out` flag. `internal/history` extended with a
        fourth `CapabilityAccesses` domain, same shape as
        `SyscallAccesses`/`NetworkAccesses`; CRD schema updated to match.
        No new RBAC. See `docs/architecture.md` §3,
        `docs/policy-synthesis.md`.
      - [x] **Composed view, not a fifth backend — `internal/exporter/securitycontext`**:
        a follow-up proposal suggested merging the seccomp and
        capabilities exporters into one "ContainerSecurityContext"
        backend, optionally also inferring static hardening fields
        (`privileged`, `allowPrivilegeEscalation`, `runAsNonRoot`,
        `readOnlyRootFilesystem`) from safe-default heuristics.
        Deliberately **not** done that way: a seccomp profile still has
        to ship as its own file for the kubelet to load
        (`corev1.SeccompProfile.LocalhostProfile` only ever takes a path
        reference, confirmed via its own doc comment), so a true merge
        would still produce two files, just with more indirection —
        `internal/exporter/seccomp` and `internal/exporter/capabilities`
        stay exactly as they are. Instead, `securitycontext` is a third,
        additive view composing the *already-computed* capabilities
        fragment with a *reference* to the seccomp file, only when one
        was genuinely written this same run (never a dangling
        reference) — this project's first exporter-to-exporter
        dependency (reuses `capabilities.ToProfile` directly rather than
        duplicating the `CAP_`-prefix-stripping logic). Deliberately
        does **not** infer the static hardening fields either: nothing
        in this codebase observes them, and stamping in "safe defaults"
        regardless of what was actually seen would contradict this
        project's own positioning (observe, don't guess). `RunAsUser`
        might be legitimately derivable later from process credentials
        Inspektor Gadget's `gadget_process` struct already carries —
        worth a look another time, but that's new tracer work. Wired in
        behind an opt-in `--security-context-out` flag. No tracer/IR/
        history changes. See `docs/architecture.md` §2–3.
      - [x] **Unified review report — `internal/exporter/report`**: five
        separate files per run, each needing its own look, was the
        natural next friction point once the observation stack (four
        domains) and the composed securityContext view were both done.
        Chosen over jumping straight to the enforcement operator — it
        extends this project's "mandatory human review" principle
        instead of replacing it, and doesn't foreclose the operator
        decision either way. Renders one `<pod>-report.md` combining
        all four IR domains directly (not just links to the other
        files — `internal/policy.Synthesize` already populates all four
        every run, regardless of which individual `--*-out` flags were
        passed, since all six gadgets always run), with a review
        checklist grounded in what's actually true this run
        (`--history` status, whether Capabilities/Syscalls came back
        suspiciously empty). The simplest exporter shape yet: depends
        only on `internal/profile`, no output-specific type at all
        (Markdown text has none to convert into), so no reuse from any
        sibling exporter the way `securitycontext` does. Also the only
        output never gated on non-empty data — an empty domain is
        itself useful review content, most often the startup blind spot
        (Findings 2/5) baked directly into the report's own wording
        rather than left for the reader to rediscover. Wired in behind
        an opt-in `--report-out` flag, works standalone. No tracer/IR/
        history/RBAC changes. See `docs/architecture.md` §2–3.
      - [x] **First slice of an evidence/proposal/approved-policy model
        — `internal/proposal`, `SecurityProfileProposal`**: a follow-up
        architecture discussion proposed a three-stage pipeline
        (`WorkloadTrainingRecord` → `SecurityProfileProposal` →
        `WorkloadSecurityProfile`, with an operator only ever
        reconciling *approved* state, never deciding what to apply).
        `WorkloadTrainingRecord` turned out to already exist —
        `TrainingHistory`, not a new CRD. This entry is the second
        stage only: `SecurityProfileProposal`, a new Kind under the
        existing `landlockgenprof.io` API group (not a new group),
        publishing rendered artifacts as one cluster object, reviewable
        via kubectl/GitOps instead of only local files. Still no
        controller — same reasoning that kept `TrainingHistory`
        controller-free: publishing a snapshot is simple CRUD
        (`internal/proposal/store.go`'s `Save`, a plain create-or-update,
        overwriting on every re-run rather than accumulating). Built via
        `runtime.DefaultUnstructuredConverter` (confirmed in the
        vendored `k8s.io/apimachinery` source) rather than
        `internal/history/store.go`'s hand-rolled field-by-field map
        construction.
        **Reworked same day after live testing**: first shipped with
        `Spec`'s four fields as structured sub-specs
        (`podlock.LandlockProfileSpec`/`networkingv1.NetworkPolicySpec`/
        `seccomp.Profile`/`corev1.SecurityContext`, CRD schema loosened
        with `x-kubernetes-preserve-unknown-fields: true`). Live testing
        showed the real problem: none of those include
        `apiVersion`/`kind`/`metadata`, so nothing was directly
        copy-pasteable or `kubectl apply -f`-able — defeating the point
        of a *reviewable* artifact. Reworked to store the exact rendered
        text (YAML/JSON) each exporter's own `ToYAML`/`ToJSON` already
        produces for the local files instead — simpler in every way:
        plain `string` fields (no pointers), plain `type: string` CRD
        schema (no more preserve-unknown-fields hack), and
        `internal/proposal` no longer depends on
        `pkg/podlock`/`k8s.io/api/...`/`pkg/seccomp` at all, only
        `k8s.io/client-go/dynamic`.
        **The approved-policy stage (`WorkloadSecurityProfile`), the
        approval mechanism itself, and the enforcement operator are
        deliberately NOT part of this change** — that's the one stage
        that actually needs a reconciliation loop (keeping applied
        resources from drifting), unlike the two evidence/proposal
        stages before it, and is a much larger, separate undertaking
        (controller-runtime, RBAC to apply resources on users' behalf).
        **Made mandatory, not opt-in, after live feedback**: the
        `--publish-proposal` flag has been removed — every `trace` run
        now publishes the `SecurityProfileProposal` unconditionally, and
        fails outright (not a silent skip) if it can't (missing CRD or
        `deploy/rbac-proposal.yaml`). It's the primary reviewable
        artifact this tool produces, not an optional extra. See
        `docs/architecture.md` §2–3.
      - [x] **Ready-to-apply patched manifest — `internal/k8s.PatchedManifest`,
        `--patched-manifest-out`**: `--security-context-out`'s bare
        fragment still needed manual pasting into a real spec. Confirmed
        the key nuance before building this: most container-spec fields,
        including `securityContext`, are immutable on an already-running
        Pod, so for an owned pod the useful artifact is the *owner's*
        manifest (`Deployment`/`StatefulSet`/`DaemonSet`, patched on
        `spec.template.spec.containers[].securityContext`) — applying
        that triggers a rollout, the real supported way to change this —
        not the ephemeral pod's own YAML. Reuses `internal/k8s`'s
        existing `DetectOwner`/`OwnerKind` from `--restart` directly
        rather than reinventing the same distinction. Merges, never
        replaces: only `Capabilities`/`SeccompProfile` are ever set on
        the target container, every other existing `securityContext`
        field (`RunAsUser`, `RunAsNonRoot`, ...) is preserved untouched
        — verified by a test that deliberately caught a real bug this
        way: a naive re-marshal of the live fetched object still
        serialized `status: {}` (no `omitempty` on that field in the
        real API types), fixed with a dedicated minimal manifest type
        that omits the field entirely rather than trying to zero it out.
        New, self-sufficient, read-only RBAC manifest
        (`deploy/rbac-patched-manifest.yaml`) — see
        `docs/threat-model.md` §1 for why it's deliberately not folded
        into `deploy/rbac-restart.yaml`. No tracer/IR/history changes.
        **`SecurityProfileProposal`'s `securityContext` field replaced by
        `patchedManifest`**: live feedback pointed out the inconsistency
        this left — `podLock`/`networkPolicy` in the proposal were
        already full, directly-appliable manifests, but the proposal's
        `securityContext` field stayed the bare
        capabilities/seccompProfile fragment `--security-context-out`
        produces, no `apiVersion`/`kind`/`metadata`. `publishProposal`
        now calls the same `PatchedManifest`/`PatchedManifestForOwner`
        functions `--patched-manifest-out` uses, so `spec.patchedManifest`
        is always the full manifest, generated on every run regardless of
        whether `--patched-manifest-out` was also passed (that flag only
        controls the *local file* copy). Needs
        `deploy/rbac-patched-manifest.yaml` unconditionally now, not just
        when that flag is used — see `docs/threat-model.md` §1.
        **Second bug, confirmed live**: `--patched-manifest-out` combined
        with `--restart` against a Deployment/DaemonSet failed
        (`pods "..." not found`) — the pod name captured before the
        restart no longer existed by the time the manifest step ran,
        since `--restart`'s rollout replaces that exact pod under a new
        `generateName` for these two owner kinds (the same unstable-name
        problem `PodSelectorFor` already exists to solve for the
        tracer). Fixed by adding `k8s.PatchedManifestForOwner`, which
        takes the owner/name `--restart` already determined instead of
        re-fetching a pod that may already be gone — used automatically
        whenever the owner is a Deployment or DaemonSet, see
        `cmd/landlock-genprof/trace.go`'s `writePatchedManifest`.
        **Third bug, confirmed live**: for a bare pod, `cleanPod` was
        dumping the live pod's raw `spec` as-is — `nodeName` (pinning any
        recreated pod to that one node, unlike `restartBarePod`, which
        already clears it for the same reason) and the ServiceAccount
        admission controller's injected `kube-api-access-*` token
        volume/volumeMount, neither of which a human ever writes into a
        pod manifest by hand. `cleanDeployment`/`cleanStatefulSet`/
        `cleanDaemonSet` don't have this problem — they read an owner's
        own *template*, which never gets these live-scheduling-time
        injections. Fixed by clearing `NodeName` and stripping the
        injected volume/mount pair in `cleanPod` before marshaling.
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
        `docs/e2e-demo.md`'s Finding 1 update.
      - [x] **Finding 2 fixed, opt-in**: `trace --restart`
        (`internal/k8s/restart.go`) restarts the target pod — delete
        +recreate for a bare pod, or the same rollout-restart annotation
        patch `kubectl rollout restart` uses for a Deployment-owned one
        — and re-targets the tracer at the replacement before the
        observation window starts, so startup-time opens (pid file, log
        fd) are actually captured. Opt-in (disruptive to the running
        workload, needs additional RBAC — see
        `deploy/rbac-restart.yaml`). See
        `docs/e2e-demo.md`/`docs/threat-model.md`.
        - [x] **First live attempt was itself broken, caught and fixed**:
          restart-then-trace produced a fully empty profile — gadget
          attachment (a real gRPC handshake per gadget) is reliably
          slower than an already-cached image's container start, so the
          tracer was still attaching after nginx had already finished its
          startup opens. Fixed by adding an `onReady` callback to
          `tracer.Trace` (`internal/tracer/trace_linux.go`, fired once
          all four gadgets confirm attachment) and reordering the
          bare-pod case in `traceWithRestart`
          (`cmd/landlock-genprof/trace.go`) to attach first, then
          restart — relying on Inspektor Gadget's KubeManager filter to
          dynamically re-attach to the replacement container under the
          same pod name. Deployment-owned pods still restart first (the
          replacement's name isn't known in advance).
          - [x] **Confirmed live**: `trace --restart` on `nginx-demo`
            produced `readWrite: [/run, /var/log/nginx]` — exactly the
            gap Finding 2 named — plus `readExec: [/usr/sbin]` (nginx's
            own startup `execve`, never previously observed) and a
            richer, correctly-attributed `readOnly` set. See
            `docs/e2e-demo.md`'s Finding 2 update.
        - [x] **Extended to StatefulSet/DaemonSet, both confirmed live**
          (`internal/k8s.DetectOwner`/`Restart`, `deploy/rbac-restart.yaml`'s
          extra `ClusterRole` rules): the split isn't "bare pod vs.
          everything else" but **stable name vs. unstable name**
          (`internal/k8s.KeepsStableName`). StatefulSet pods keep their
          deterministic `<name>-<ordinal>` identity across a rolling
          restart, joining the bare-pod attach-tracer-first bucket —
          confirmed live: `trace --restart` on `nginx-sts-0` produced the
          same `readWrite: [/run, /var/log/nginx]` signature as the
          bare-pod case, with no "Tracing replacement pod" line (proof
          the attach-first sequence ran, since that line only exists in
          the other bucket).
        - [x] **Deployment/DaemonSet: found broken live, fixed with
          label-selector pre-targeting.** DaemonSet pods get a new
          `generateName`-assigned suffix every recreation, so they
          couldn't be pre-targeted by name — left on the older
          restart-then-discover order, which **live testing immediately
          confirmed was actually broken**, not just theoretically
          imperfect: a fully empty profile (`{}`) for a real DaemonSet
          restart, same root cause as the original bare-pod bug, never
          fixed for this path because there was no stable name to
          pre-target with. Fixed by discovering Inspektor Gadget's
          `KubeManager` operator supports filtering by **label
          selector**, not just exact pod name (confirmed in the vendored
          SDK, `pkg/operators/common/container-selector.go`'s
          `ParamSelector` — same confidence as the already-proven
          `podname`/`namespace`/`containername` params). A Deployment/
          DaemonSet's own `spec.selector` (`internal/k8s.PodSelectorFor`,
          fetched *before* the restart) lets `traceWithRestart`
          pre-attach the tracer the same way as the stable-name cases —
          every owner kind now shares one attach-first sequence, only
          differing in *what* to pre-target with. Side effect: the
          generated profile's identity becomes the **workload's own
          name** (e.g. `nginx-ds`), not an ephemeral pod's — more honest
          about what capturing "any pod matching this selector" actually
          means, and the PodLock label hint now patches the pod
          *template* for these two kinds instead of labeling a
          soon-to-be-replaced pod. **Confirmed live**: re-running the
          exact DaemonSet scenario that produced the empty profile
          produced a correct one instead — `readWrite: [/run,
          /var/log/nginx]`, `metadata.name: nginx-ds`, and the
          `kubectl patch daemonset` hint — proving
          `operator.KubeManager.selector` re-attaches to the replacement
          pod in time, the same way `podname`-based re-matching already
          did for bare pods. See `docs/e2e-demo.md`. `internal/k8s.Restart` simplified
          alongside this — no owner kind needs to discover or report
          back a replacement's identity anymore, so it dropped its
          `*TargetPod` return down to just `error`, and `waitForNewPod`
          (only ever needed for the old discover-the-name approach) was
          deleted.
- [ ] **M5 (stretch)**: post-deployment drift detection (Landlock denial
      logs → suggested policy adjustment)
      - [x] **Persistence prerequisite done**: `trace --history`
        (`internal/history`, opt-in — `deploy/crd-traininghistory.yaml`
        + `deploy/rbac-history.yaml`) persists a `TrainingHistory`
        custom resource per container/binary, accumulating
        `RunsRecorded` and each access's `SeenInRuns` across every run —
        no controller, the CLI reads/writes it directly via the dynamic
        client. This is exactly the missing piece
        `docs/policy-synthesis.md`'s "Confidence: a deliberately
        provisional heuristic" section named: `Confidence` can now be
        computed from a real cross-run ratio
        (`internal/history.ApplyConfidence`) instead of
        `confidenceFor`'s single-run proxy. An access not observed in a
        run keeps its `SeenInRuns` while `RunsRecorded` grows, so its
        ratio decays on its own — the actual drift *signal* M5 needs,
        though not drift *detection* (alerting, or consuming Landlock
        denial logs) itself, which remains the open stretch goal.
      - [x] **Confirmed live**: four `trace --history` runs against
        `nginx-demo` — two idle, two with real `wget` traffic in
        parallel — produced `runsRecorded: 4` and
        `filesystemAccesses: [{path: /usr/share/nginx, permissions:
        [read], seenInRuns: 2}]`, exactly the two runs that actually hit
        nginx. Demonstrates the ratio-decay property working as
        designed: 2/4 computes `ConfidenceMedium` via
        `confidenceForHistory`, not `ConfidenceHigh` — the two idle runs
        genuinely diluted the ratio. One naming gotcha, not a bug:
        `nginx-demo`'s container is itself named `nginx-demo` (`kubectl
        run` sets container name = pod name), so the record is
        `nginx-demo-nginx`, not `nginx-nginx` as the illustrative
        examples elsewhere assume.
      - [x] **Done: `Confidence` now surfaced in the generated YAML** —
        `internal/exporter/podlock`/`internal/exporter/networkpolicy`'s
        `ToYAML` functions attach a trailing `# confidence: ...` comment
        per path/port (`annotateConfidence`, re-parsing the already-
        correct `sigs.k8s.io/yaml` output into a `gopkg.in/yaml.v3`
        `Node` tree rather than encoding the struct directly, which
        keeps PodLock's exact camelCase keys intact — `yaml.v3` has no
        `json`-tag support and would otherwise guess wrong). Not a
        schema change: comments are stripped by any real YAML parser
        before `kubectl apply` ever sees them, only visible to the human
        doing the mandatory review. **Confirmed live**: `trace --history`
        on `nginx-demo` produced `- /usr/share/nginx # confidence:
        medium` directly in the generated `profile.yaml`, the diluted
        ratio from earlier accumulated runs. See
        `docs/policy-synthesis.md`.

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
