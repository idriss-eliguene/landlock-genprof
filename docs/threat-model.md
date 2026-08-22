# Threat model and validation methodology

**In short:** the tracer itself needs zero elevated privileges — it's an
ordinary API client, same category as `kubectl`. The real elevated
access belongs to Inspektor Gadget's own DaemonSet, a separate
component this project depends on but doesn't control. What follows:

1. [Tracer attack surface](#1-tracer-attack-surface) — exact RBAC, and
   the one flag (`--restart`) that genuinely widens the blast radius.
2. [Completeness of generated profiles](#2-completeness-of-generated-profiles-false-negative-risk) —
   what a short training run can miss, and the two gaps already fixed
   (startup blind spot, cross-process contamination).
3. [Candidate authority and governed apply](#3-candidate-authority-and-governed-apply) —
   substitution, stale authority, source provenance, readiness, and partial apply.
4. [Runtime validation](#4-runtime-validation) — enforcement and bypass questions.
5. [CI hardening](#5-ci-hardening) — SAST/SCA status.

## 1. Tracer attack surface

**`landlock-genprof` itself needs zero elevated Linux capabilities —
confirmed directly in `internal/tracer/trace_linux.go`: no `cilium/ebpf`,
no raw `bpf()`/netlink syscalls, `golang.org/x/sys/unix` is used only for
an `O_DIRECTORY` flag constant.** It's an ordinary Kubernetes API client,
same category as `kubectl`/`helm` — it never runs on a node and is never
privileged/`hostNetwork`/`hostPID`. All syscall observation happens via
Inspektor Gadget's own gRPC runtime
(`grpcruntime.WithConnectUsingK8SProxy`), tunneled through a
`pods/portforward` subresource — the same K8s API mechanism `kubectl
port-forward` uses to reach a pod that's already running.

The actual elevated privileges (`CAP_BPF`, `CAP_SYS_ADMIN` depending on
kernel version) belong entirely to **Inspektor Gadget's own DaemonSet**
— a separate component this project depends on but doesn't deploy or
control (`kubectl gadget deploy`, out of scope here; see
`docs/enforcement-prerequisites.md`'s sibling docs for what this project
does and doesn't set up). That DaemonSet, always running on every node
once deployed, is the real attack surface worth scrutinizing — not
`landlock-genprof`'s own process, which never runs permanently and never
touches a node directly. This project's own contribution to the attack
surface is much narrower: the RBAC below, granting API-level (not
kernel-level) access to reach that already-running daemon.

- [x] What's the tracer's service account's minimal RBAC? See
  [`deploy/rbac.yaml`](../deploy/rbac.yaml): `get` on `pods` cluster-wide
  (target pod resolution, namespace chosen dynamically at runtime) +
  `list` on `pods` and `create` on `pods/portforward` scoped to the
  `gadget` namespace only (reaching Inspektor Gadget's daemon via the K8s
  proxy). Every rule traces back to a specific API call in the code —
  see the manifest's own comments. **Confirmed generic, not per-gadget**:
  nothing in the manifest names `trace_open`/`trace_exec` specifically —
  it's daemon-reachability access only, so adding `trace_tcp`/
  `trace_bind` (`internal/tracer/trace_linux.go`) required no new RBAC
  rule. Same for `advise_seccomp` (`runSeccompTracer`, added for the
  seccomp exporter) and `trace_capabilities` (`runCapabilitiesTracer`,
  added for the capabilities exporter): no new RBAC either.
- **`advise_seccomp` observes every process on the node during the
  training run, not just the target container** — confirmed directly in
  its own upstream source (`program.bpf.c`'s `sys_enter` probe comment):
  container filtering can't happen in-kernel without losing the target
  container's own startup syscalls (executed by `runc` before the
  container's filter is installed), so this specific gadget deliberately
  records node-wide and only filters down to the target container
  afterwards, at its own formatting stage. This is an upstream design
  choice in a gadget this project reuses as-is (see
  `internal/tracer/trace_linux.go`'s `runSeccompTracer`), not something
  introduced by this codebase — but it does mean a training run using
  `--seccomp-out` briefly observes syscall activity from every other
  workload on the same node, a wider blast radius than the other four
  gadgets (which scope in-kernel via the standard mount-namespace filter).
  Worth knowing before running `--seccomp-out` on a shared/multi-tenant
  node. **`trace_capabilities` does not share this caveat** — confirmed
  via its own source (`program.bpf.c` includes `<gadget/filter.h>` and
  calls `gadget_should_discard_data_current()`, the same in-kernel
  container-filtering mechanism `trace_open`/etc. use), so
  `--capabilities-out` scopes to the target container the normal way.
- What's the blast radius if the tracer itself is compromised?
  **`--restart` (`internal/k8s/restart.go`) genuinely widens this** —
  unlike everything above, it's not read-only. It needs `delete`/
  `create` on `pods` and `patch` on `deployments`/`statefulsets`/
  `daemonsets` (see
  [`deploy/rbac-restart.yaml`](../deploy/rbac-restart.yaml)), meaning a
  compromised tracer ServiceAccount with this manifest applied could
  kill and recreate the pods it's pointed at, or force a rollout restart
  on their owning Deployment/StatefulSet/DaemonSet — not just read one
  pod. Deliberately kept
  in a **separate, opt-in manifest**, not folded into the base
  `deploy/rbac.yaml`: deploying the base manifest alone keeps today's
  read-only posture unchanged; `--restart` (and the extra blast radius
  that comes with it) is a choice an operator makes explicitly by also
  applying the second manifest.
- **`internal/k8s/patch.go`'s `PatchedManifest`/`PatchedManifestForOwner`
  need `get` on `deployments`/`statefulsets`/`daemonsets`, but stay
  read-only** — meaningfully smaller blast radius than `--restart`'s
  manifest: they only ever fetch objects to build a manifest, never
  patch or delete anything in the cluster. **No longer tied to an opt-in
  flag**: since `SecurityProfileProposal` publishing became mandatory
  every `trace` run needs this RBAC whenever
  there's a `securityContext` to compose, whether or not
  `--patched-manifest-out` was also passed to additionally write a local
  file. Deliberately its own manifest
  ([`deploy/rbac-patched-manifest.yaml`](../deploy/rbac-patched-manifest.yaml)),
  not folded into `deploy/rbac-restart.yaml` even though it overlaps two
  of its three `get` grants: it doesn't require opting into `--restart`'s
  disruptive delete/patch capabilities too, and this project's own RBAC
  principle is one self-sufficient manifest per capability, even for one
  that's now baseline-required rather than optional. Two `ClusterRole`s
  granting `get` on the same resource (if both manifests are applied) is
  harmless, standard RBAC composition.

## 2. Completeness of generated profiles (false-negative risk)

A short training run doesn't cover every possible code path (errors, edge
cases, rarely triggered behavior). A profile generated by observation can
therefore be:
- too restrictive if it's missing legitimate rules (the app breaks in prod)
- silently incomplete if nobody knows what wasn't observed

**Startup blind spot — fixed, opt-in, via `trace --restart`.**
`docs/e2e-demo.md` Finding 2 documented that resources opened once at
process startup (a pid file, a log fd) are invisible to a trace attached
to an already-running container. `internal/k8s.Restart` closes this by
restarting the target (delete+recreate for a bare pod, a rollout-restart
annotation patch for a Deployment-owned one) right before the
observation window starts — opt-in via `--restart` because it's
disruptive to the running workload and needs the additional RBAC noted
in §1.

Recommended validation protocol:

- Exercise normal, startup, error, and infrequent paths across multiple runs;
  no fixed duration or run count proves completeness.
- Treat `Confidence` as cross-run occurrence evidence, not correctness,
  completeness, authorization, enforcement, or verification.
- Exercise the target with *real* traffic (e.g. an actual HTTP request to
  nginx), not `kubectl exec` debug commands that only incidentally touch similar
  paths — see `docs/e2e-demo.md` Finding 1's live re-verification, where
  `ls`/`cat` via `kubectl exec` produced a fully empty profile once
  correctly excluded from nginx's own attribution, because nginx itself
  never did anything observable during that window.

**Contamination risk — fixed at the tracer level for all four gadgets.**
`docs/e2e-demo.md` Finding 1 documented that the tracer's Inspektor Gadget
filter scopes events by `namespace`/`podname`/`containername` only, never
by process — any process sharing the container's namespaces during the
training window got attributed to the traced binary, which produced
false `readExec: /bin, /usr/bin` rules from a `kubectl exec` debugging
session. The same gap affected `trace_tcp`/`trace_bind`: a
`connect`/`bind` made by anything sharing the pod's network namespace
during training — a debugging session, a sidecar, an attacker — would be
attributed to the traced workload and broaden its generated
`NetworkPolicy` the same way. `internal/tracer/trace_linux.go` now scopes
every one of the four `run*Tracer` functions to the traced binary's
`comm` (`commFromBinaryPath`), closing this for both PodLock and
`NetworkPolicy` output. Residual risk, deliberately accepted: a
legitimate child process spawned under a different `comm` (e.g. a CGI
script) is filtered out too — a false negative traded for closing this
false positive, see `commFromBinaryPath`'s own comment.

## 3. Candidate authority and governed apply

The authority model assumes an authenticated human reviewer is permitted to
approve policy for the target namespace and protects that decision with
content identity. Kubernetes authentication, RBAC, and admission policy still
decide who may write proposal status or enforcement resources; a compromised
authorized reviewer can intentionally approve dangerous content.

| Threat | Implemented control | Residual risk or limit |
|---|---|---|
| Candidate substituted before approval | `approve --expected-digest` compares the reviewed digest with current content and records authority for that digest | A reviewer can still approve the wrong digest intentionally or without adequate review |
| Candidate mutated after review or approval | Content mutation changes `CandidateDigest`; stale or mismatched approval is rejected | Candidate-wide digest forces full re-review even for an isolated domain change |
| Stale approval reused for a newer candidate | Approved and current digests must match under `candidate-v1`; mismatch fails closed | None within the current digest field set; schema evolution must update digest vectors |
| Review/apply TOCTOU | `apply-proposal` validates before planning, applies an immutable planned payload, and revalidates immediately before workload binding | External resources applied before a later failure are not rolled back |
| Approval recorded for the wrong content | Expected-digest comparison and retry-on-conflict prevent accidental transfer across concurrent updates | Human identity and decision quality depend on cluster authentication, RBAC, and review process |
| Seccomp source silently substituted | Source mode is explicit; there is no internal/SPO fallback; provenance is included in digested content | Switching sources deliberately requires full re-review |
| SPO provenance spoofed or cross-workload policy imported | ADR-0008 requires recording namespace, recording ID, container lineage, completion, inertness, ownership, and supported-semantics checks | Labels are structural lineage under RBAC, not cryptographic authentication; stronger UID-bound lineage needs upstream support |
| SPO source mutated after import | Import is a copied governed snapshot, not a live reference; the source object is never mutated by landlock-genprof | Re-importing changed source content creates a new candidate and requires review |
| SPO-derived data laundered as observation/confidence | SPO policy enters at the artifact layer; SPO syscalls have no route to `TrainingHistory` and receive no landlock-genprof confidence | Coverage is optional provenance (absent in official v1.0.0, normalized when supported); it remains neither lineage nor authority |
| Governed enforcement object mutated before binding | ADR-0007 readiness checks exact planned identity and content before binding | Only implemented backend adapters can provide readiness semantics |
| Backend reconciliation fails or times out | Readiness failure, timeout, or identity mismatch stops binding and exits as a blocking failure | Enforcement resources applied earlier can remain in the cluster |
| Partial multi-artifact application | Application is ordered; the first failure stops the sequence and binding stays last | Apply is sequential, not transactional, and does not roll back earlier resources |
| Applied mistaken for enforced | Documentation and status keep API application separate from backend realization | Generic Kubernetes API success cannot prove kernel or datapath behavior |
| Enforced mistaken for verified | Behavioral verification is a separate evidence gate | `verify` currently checks Landlock ABI compatibility; it is not general runtime verification |

These controls implement fail-closed authority and binding behavior: ambiguity,
missing approval, stale content, unsupported SPO semantics, lineage failure,
readiness timeout, or governed identity mismatch prevents the affected governed
transition. They do not make the whole apply transaction atomic, authenticate
human intent beyond Kubernetes controls, or prove external enforcement.

## 4. Runtime validation

Once a profile is deployed (via PodLock), try to bypass it:
- Can a pod escape its Landlock policy?
- Can the traced process detect that it's being observed and change its
  behavior during the training run (evasion)?
- Can the human review workflow be bypassed in practice?

## 5. CI hardening

- [x] Integrate a SAST/SCA scan (`gosec`, Trivy) on the project's Go code,
  in `.github/workflows/ci.yml` — `security` job, separate from
  `build-and-test` (not yet a required status check: first results need
  triaging before making it blocking).
