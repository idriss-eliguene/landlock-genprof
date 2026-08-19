# ADR-0007: Governed apply ordering and enforcement readiness

Status: Proposed

Date: 2026-08-18

Revised: 2026-08-19 — SPO backend section retargeted to the modern API
(cluster-scoped `v1`). The ordering, readiness, identity and Gate 3
contract below is unchanged; only the SPO-specific adapter detail moved.

## Context

`apply-proposal` applies an approved `SecurityProfileProposal`'s artifacts
to the cluster. Four artifact kinds exist today: a PodLock
`LandlockProfile`, a `NetworkPolicy`, an SPO `SeccompProfile`, and a
Patched Manifest (the target workload with the generated `securityContext`
merged in).

Three of those artifacts are *enforcement resources* consumed by an
external controller. The fourth — the Patched Manifest — is a **binding
artifact**: it is the thing that makes the workload actually reference the
others. `cmd/landlock-genprof/trace.go` computes
`seccompLocalhostProfile` from `internal/exporter/spo.LocalhostProfilePath`
and threads it into the patched
manifest's `securityContext.seccompProfile.localhostProfile`
(`internal/k8s/patch.go:161`); the same manifest carries the
`podlock.kubewarden.io/profile` label (`trace.go:53`). The binding
artifact therefore names both the SPO profile and the PodLock profile.

The current application order is wrong. `proposalArtifacts`
(`cmd/landlock-genprof/review.go:41-46`) returns:

    PodLock → NetworkPolicy → Patched Manifest → SPO SeccompProfile

The plan is built in that order (`apply_proposal.go:183-188`) and executed
in that order (`apply_proposal.go:252`). So the Patched Manifest — which
under `--restart` **deletes and recreates the pod** (`internal/k8s/apply.go`
`applyPod`) — is applied *before* the `SeccompProfile` CR it references has
even been submitted, let alone materialized on the node by SPO.

The repository already documents both the mechanism and the symptom.
`internal/exporter/spo/export.go:110-118` records that containerd refuses
to start a container whose `localhostProfile` does not resolve to a real
file on the node ("cannot load seccomp profile ...: no such file or
directory"), and that this tool "never waits for SPO's own reconciliation
to actually run". `demo/golden/workload.yaml` records the operational
consequence: a 73-minute, 15-restart `CrashLoopBackOff` from repeatedly
force-restarting a pod "whose enforcement side wasn't actually ready yet
(SPO/PodLock)".

The defect is invisible in authoritative CI only because SPO is absent
from the E2E cluster, so the `SeccompProfile` artifact is always excluded
via `--skip=spo-seccompprofile`. Introducing SPO — the v0.2 direction —
activates it on the first run.

A second, subtler problem appears once a wait exists at all. Submitting a
resource and waiting for a controller to reconcile it opens a window in
which the live resource can be mutated by another actor. If the workload
is then bound, the enforced content may not be the content that was
approved — which would breach the project's central invariant
(ADR-0006: approval authorizes exact candidate content).

## Decision

Adopt **Option C — ordering plus backend readiness plus identity
recheck**, with a scoped third authority gate immediately before the
binding artifact.

`apply-proposal` MUST apply artifacts in dependency order, MUST NOT apply
the binding artifact until every enforcement artifact that the binding
artifact references has reached a backend-declared readiness state whose
content still matches the approved plan, and MUST re-verify approval
immediately before the binding artifact. Any failure of those conditions
fails closed: the binding artifact is not applied, and the command exits
`2` (blocking failure, ADR-0001).

Readiness is an **execution precondition**, never authority. It never
recomputes, mutates, or influences `CandidateDigest` or approval state.

## Invariants

- **INV-APPLY-ORDER-01** — A binding artifact MUST NOT be applied until
  every enforcement artifact it references has been applied and has
  reached its backend's declared readiness state.
- **INV-APPLY-ORDER-02** — Readiness MUST be established for the exact
  approved content, not merely for a resource of the same name and
  namespace. A backend reporting "ready" for content that differs from the
  planned artifact is NOT ready for the purposes of this contract.
- **INV-APPLY-ORDER-03** — Readiness timeout, readiness failure, identity
  mismatch, or any error while establishing readiness fails closed: the
  binding artifact is not applied.
- **INV-APPLY-ORDER-04** — Runtime readiness state does not alter
  `CandidateDigest`, `approvedCandidateDigest`, `approvalState`, or the
  planned payload. Authority is content; readiness is execution.
- **INV-APPLY-ORDER-05** — A failure applying any artifact stops the
  sequence. Artifacts already applied remain; no further artifact is
  applied, and in particular the binding artifact is not.
- **INV-APPLY-ORDER-06** — The apply path remains retry-safe to the extent
  the underlying apply semantics are idempotent. Retrying after a
  readiness timeout MUST NOT require manual cleanup of artifacts already
  applied.
- **INV-APPLY-ORDER-07** — Absence of an external backend MUST NOT
  silently change the enforcement source or silently drop an approved
  artifact. Excluding an artifact remains an explicit operator act
  (`--skip`).
- **INV-APPLY-ORDER-08** — Approval MUST be re-validated immediately
  before the binding artifact is applied, after any readiness wait.

## Artifact dependency model

| Artifact | Class | External reconciler | Node-local materialization | Referenced by binding artifact | Live-applicable | Readiness signal |
|---|---|---|---|---|---|---|
| SPO `SeccompProfile` | Enforcement resource | SPO daemon | **Yes** — writes `operator/<name>.json` | **Yes** (`securityContext.seccompProfile.localhostProfile`) | Yes | **Yes** — see below |
| PodLock `LandlockProfile` | Enforcement resource | PodLock operator | Operator-defined | **Yes** (`podlock.kubewarden.io/profile` label) | Yes | **Unspecified** — no backend contract exists yet |
| `NetworkPolicy` | Live policy resource | CNI datapath | No | No | Yes | Not required — see below |
| Patched Manifest | **Binding artifact** | None (kubelet) | No | n/a | **No** — `applyPod` deletes and recreates the pod | n/a |

The Patched Manifest is the only artifact that is both destructive and
workload-affecting, and the only one that references the others. That
asymmetry, not symmetry, is what the ordering rule follows.

## Ordering

Dependency-derived, not positional:

1. **Enforcement resources referenced by the binding artifact** — SPO
   `SeccompProfile`, PodLock `LandlockProfile`.
2. **Independent live policy resources** — `NetworkPolicy`. Applied before
   the binding artifact so a recreated pod is covered from the moment it
   starts rather than being briefly unprotected. It has no ordering
   *dependency*; this is a defensive preference, not a requirement.
3. **Readiness gate** — block until every artifact from (1) that declares
   readiness is ready *for the planned content*.
4. **Authority re-validation** — re-read the proposal, re-run
   `ValidateApprovedCandidate`, and confirm the candidate digest still
   equals the digest captured when the plan was built.
5. **Binding artifact** — Patched Manifest, always last.

Within (1) and (2), relative order is unconstrained and MUST remain
deterministic.

## Backend readiness

### SPO — specified

**Target API.** `security-profiles-operator.x-k8s.io/v1`,
kind `SeccompProfile`, **scope `Cluster`**. Confirmed by reading SPO's own
CRD manifests per tag: v0.8.4 is `Namespaced`/`v1beta1`; **v0.9.0 changed
the scope to `Cluster`** while still serving only `v1beta1`; v1.0.0 serves
`v1` (storage) with `v1beta1` still served. A CRD's scope is a property of
the whole CRD, not of a version, so `v1beta1` at v1.0.0 is cluster-scoped
too and a conversion webhook cannot bridge that — conversion maps between
versions, never between scopes. The breaking change for this project is
therefore the **scope** change at v0.9.0, not the version change at v1.0.0.

**Namespace is not part of `SeccompProfile` identity under this API.** A
governed profile is identified by name alone, cluster-wide. Readiness
lookup is a cluster-scoped Get; the artifact carries no
`metadata.namespace`; and the materialized path is
`operator/<name>.json`, with no namespace segment.

An applied `SeccompProfile` is **READY** when both hold:

- `status.localhostProfile` is populated and equals the path the planned
  patched manifest references — `operator/<name>.json` for the governed
  profile's name; **and**
- the live object's enforcement-relevant spec — `defaultAction`,
  `architectures`, `syscalls` — still equals the planned artifact's spec.

`status.localhostProfile` is preferred over a generic `status` phase
because it is the exact string the binding depends on. Checking the thing
the workload actually needs beats checking a proxy for it.

- **NOT READY** — the resource exists but `status.localhostProfile` is
  absent or does not yet match. Keep waiting until the timeout.
- **FAILED** — SPO reports a terminal error state for the object, or the
  object has been deleted since it was applied. Stop waiting; fail closed.
- **UNKNOWN** — the status cannot be read (API error, RBAC). Treated as
  NOT READY until the timeout, then fail closed. UNKNOWN is never treated
  as READY.

Comparison is on enforcement-relevant spec fields only. Whole-object
comparison would produce false mismatches, because SPO legitimately writes
`status` and may add its own metadata.

**Cluster-scoped collision safety.** Because the governed
`SeccompProfile` occupies a cluster-wide name space, applying one can
overwrite an object this project did not create — a namespaced resource
could never do that. Before applying a cluster-scoped enforcement
artifact, the apply path MUST read any existing object of that name and
refuse, fail-closed, unless it carries this project's own ownership
marker. An object we own may be updated to the approved content (that is
what makes a retry safe); an unowned object is never overwritten. See
ADR-0008 for the deterministic naming scheme that makes such a collision
unlikely, and this rule for what happens when one occurs anyway.

**Backend contract, not scattered assumptions.** API version, resource
scope, GVR, `localhostProfile` construction and parsing, lookup
semantics and readiness interpretation are all backend-specific. They
MUST live behind a single internal backend adapter. Generic governance
and apply code MUST NOT encode `v1`/`v1beta1`, namespaced/cluster-scoped,
or the segment count of a profile path. v0.2 targets modern SPO only;
this boundary exists so that supporting another version later is a change
in one place rather than nine (see "Deferred work").

### PodLock — unspecified

**UNSPECIFIED UNTIL A BACKEND CONTRACT EXISTS.** The LandlockProfile is
referenced by the binding artifact and therefore ordered before it, but no
readiness predicate is defined today because PodLock publishes no contract
this project has verified. Until one exists, PodLock declares no readiness
and the gate does not wait on it. This is a known gap, recorded rather
than papered over: ordering alone does not prevent a PodLock analogue of
the seccomp CrashLoop.

### NetworkPolicy — no readiness required

A `NetworkPolicy` is not referenced by the pod, is not materialized as a
node-local file, and is selected by label rather than by name. Nothing in
the pod's startup path can fail because a NetworkPolicy has not yet been
realized in the CNI datapath. Datapath realization latency affects when
traffic is filtered, not whether the workload starts. No readiness wait is
defined, and none should be invented for symmetry.

## Revalidation

Three gates, all retained or added:

- **Gate 1 — preflight** (existing): `ValidateApprovedCandidate` before
  any planning or mutation.
- **Gate 2 — pre-apply** (existing): after the confirmation prompt,
  re-read spec and status, re-validate, and confirm the digest has not
  changed since the plan was built.
- **Gate 3 — pre-binding** (**new**): after any readiness wait and
  immediately before the binding artifact only, re-read spec and status,
  re-run `ValidateApprovedCandidate`, and confirm the digest still equals
  the digest captured at plan time.

Gate 3 exists because a readiness wait is a new window of unbounded
duration between Gate 2 and the destructive step. Plan immutability
already guarantees that the *payload* submitted is the reviewed bytes
(`TestRunApplyProposal_UsesPlannedPayloadEvenIfProposalMutatesAtApplyTime`),
but it does not detect that approval was **revoked** while we waited.
Binding a workload on a revoked approval would be a fail-open path, so
plan immutability alone is insufficient.

Gate 3 is deliberately scoped to the binding artifact rather than run
before every artifact: the binding artifact is the only destructive,
workload-affecting step, and re-validating before each of three inert
artifact applies would add cost without adding a guarantee.

## Failure semantics

| Condition | Behavior |
|---|---|
| Readiness timeout | **FAIL-CLOSED** — binding not applied, exit `2` |
| SPO CRD absent | **FAIL-CLOSED** — the `SeccompProfile` apply itself fails ("the server could not find the requested resource"); sequence stops before binding |
| SPO controller absent (CRD present) | **FAIL-CLOSED** via readiness timeout — status is never populated |
| `SeccompProfile` rejected by the API | **FAIL-CLOSED** — apply fails; sequence stops |
| `status.localhostProfile` never populated | **FAIL-CLOSED** via timeout |
| `status.localhostProfile` populated but different from the referenced path | **FAIL-CLOSED** immediately — no waiting improves a mismatch |
| Live spec drifted from the planned artifact | **FAIL-CLOSED** immediately (INV-APPLY-ORDER-02) |
| Approval revoked during the wait | **FAIL-CLOSED** at Gate 3 |
| Candidate digest changed during the wait | **FAIL-CLOSED** at Gate 3 |
| Transient API error while polling | **RETRY** until the timeout, then fail closed |
| Context cancellation (Ctrl-C) | **FAIL-CLOSED** — stop; artifacts already applied remain |
| Backend declares no readiness (PodLock today) | **DEGRADED** — ordered but not gated; recorded as a known gap |

Silent fallback is prohibited. In particular, once a proposal has been
approved, apply MUST NOT substitute a different enforcement source or
silently omit an approved artifact because a backend is unavailable.

## Partial apply semantics

The apply loop is **not transactional and this ADR does not make it so.**
On failure, artifacts already applied remain in the cluster. The contract
is **fail-stop, not rollback**: no further artifact is applied, and the
binding artifact in particular is not.

This is a safe asymmetry. Every artifact that can be left behind is inert
with respect to the running workload: a `SeccompProfile` on a node that
nothing references enforces nothing, a `LandlockProfile` with no labelled
pod enforces nothing, and a `NetworkPolicy` is a live policy the operator
can inspect and remove. The only artifact whose partial application would
be dangerous is the binding artifact, and it is the one artifact this
contract guarantees is never reached on failure.

Automatic rollback is **NOT DESIRED** for v0.2: it would require the apply
path to delete resources it did not create in this run, which is a larger
authority than "apply what was approved" and introduces its own failure
modes. Cleanup remains an operator action. Revisit only if operational
evidence shows the residue causes real harm.

## Time budget

`apply-proposal` becomes a potentially tens-of-seconds operation. That is
accepted. Governed application is synchronous today and the readiness wait
MUST NOT be moved to a background process to preserve CLI responsiveness —
the synchronous shape is what makes "nothing was bound" observable at the
call site.

The wait MUST be bounded and configurable, with a default chosen to
accommodate a loaded CI cluster. Timeout is a CLI/operational concern, not
part of candidate authority, and therefore not digested.

## Observability

Waiting silently is not acceptable. The apply path MUST report progress
through the sequence — which artifact is being applied, that it is waiting
for a specific backend to become ready, that readiness was reached, and
which artifact is being applied next. On failure it MUST state which
condition failed (timeout, path mismatch, spec drift, approval change) so
an operator can act without reading logs. Exact strings are an
implementation concern.

## Consequences

**Easier.** The workload is never bound to an enforcement resource that is
not materialized, which removes the documented CrashLoop class. The
"approved content is the enforced content" invariant extends across the
external controller boundary rather than stopping at API submission. SPO
integration becomes possible without a known-broken apply path. A backend
that later declares readiness (PodLock) plugs into an existing contract
instead of forcing a redesign.

**Costs.** `apply-proposal` now depends on an external controller reaching
a lifecycle state, so its latency and its failure modes are no longer
purely a function of the Kubernetes API. A new gate and a polling loop add
code to the most safety-critical path in the project. Gate 3 adds two API
reads before binding. The contract makes the non-transactional nature of
apply explicit and therefore harder to quietly change later. Retrying
after a timeout re-applies already-applied artifacts, which is safe but
not free.

**Foreclosed.** Applying the binding artifact optimistically and letting
the kubelet retry is no longer acceptable. Treating "resource created" as
"enforcement ready" is no longer acceptable anywhere in the apply path.

## Alternatives considered

**A — Ordering only, no wait.** Apply the `SeccompProfile` before the
Patched Manifest but do not wait. Rejected: submission is not
materialization. SPO reconciliation is asynchronous, so a fast apply
followed by an immediate pod recreate still races the daemon writing the
node-local file. It narrows the window without closing it, which is the
worst outcome — an intermittent CrashLoop is harder to diagnose than a
deterministic one.

**B — Ordering plus readiness, no identity recheck.** Rejected: readiness
proves a resource of that name is materialized, not that *the approved
content* is materialized. Another actor mutating the live CR during the
wait would cause the workload to be bound to content nobody approved,
breaching ADR-0006's invariant at exactly the point this project claims to
protect.

**D — Asynchronous controller model.** Persist desired state and let a
controller coordinate readiness and binding. Rejected for v0.2: this
project ships no controller, `docs/cli-design.md` records runtime
enforcement as permanently out of scope, and introducing a reconciler
would relocate the governed decision boundary into a component with no
human in the loop. Revisit only if the CLI-driven model proves
operationally insufficient.

**E — Make apply transactional.** Rejected: rollback requires deleting
resources the run did not create and cannot distinguish from pre-existing
state, and the fail-stop contract already prevents the only dangerous
partial state.

## Security considerations

- **TOCTOU across the wait.** Closed by Gate 3 (approval revocation,
  digest change) and by INV-APPLY-ORDER-02 (live content drift).
- **Live CR mutation during reconciliation.** Closed by the identity
  recheck. Note the residual limit: this ADR protects the window *up to
  binding*. Mutation of the enforcement resource *after* binding is not
  detected — post-apply drift monitoring is deferred work.
- **Ready-but-wrong object.** A same-named resource reporting ready is not
  sufficient; content equality is required.
- **Stale approval.** A long wait cannot launder a revoked approval into a
  binding.
- **Silent fallback.** Prohibited (INV-APPLY-ORDER-07). A missing backend
  fails closed rather than quietly changing what enforces the workload.
- **Partial application.** Bounded by construction: everything that can
  remain is inert with respect to the workload.
- **Authority/execution separation.** Readiness never feeds back into the
  digest or approval state, so operational state cannot be used to
  manufacture authority.

## Testing requirements

To be implemented with the decision, not before:

1. **Ordering** — the SPO `SeccompProfile` is applied before the Patched
   Manifest.
2. **Readiness gate** — the Patched Manifest is not applied while the
   backend reports NOT READY.
3. **Timeout** — readiness never reached: no binding applied, exit `2`.
4. **Enforcement-artifact apply failure** — no binding applied.
5. **Path mismatch** — `status.localhostProfile` differs from the
   referenced path: no binding applied.
6. **Identity drift** — live spec differs from the planned artifact: no
   binding applied.
7. **Approval revoked during the wait** — Gate 3 rejects; no binding
   applied.
8. **Digest changed during the wait** — Gate 3 rejects; no binding
   applied.
9. **NetworkPolicy failure before binding** — no binding applied.
10. **Retry after timeout** — a second run with the backend healthy
    completes without manual cleanup.
11. **Standalone unchanged** — with SPO skipped or absent, behavior is
    identical to the current certified path.
12. **Authoritative E2E** — profile applied, reconciled, readiness
    observed, workload bound, pod **Running not CrashLooping**, and — if
    behavioral enforcement enters v0.2 — a denied syscall.

## Migration and compatibility

Standalone behavior (no SPO installed, or `--skip=spo-seccompprofile`) is
unchanged: with no artifact declaring readiness, the gate is a no-op and
the sequence reduces to today's behavior with a corrected order.

Existing proposals are unaffected. This ADR changes no schema, no digest
payload, and no approval mechanism version: `candidate-v1` is untouched,
and existing `approvedCandidateDigest` values remain valid.

CLI compatibility: existing invocations continue to work. A new bounded
timeout flag may be added; its default must not make previously fast
applies appear hung without progress output.

## Deferred work

- **PodLock readiness** — unspecified until a backend contract exists.
- **Multi-version SPO support** — v0.2 targets modern SPO only. Supporting
  namespaced `v1beta1` alongside cluster-scoped `v1` would make the
  rendered artifact, and therefore `CandidateDigest`, depend on which SPO
  happens to be installed. See ADR-0008's "CandidateDigest" section.
- **Rollback / transactional apply** — not desired for v0.2; revisit only
  on operational evidence.
- **Asynchronous controller model** — out of scope.
- **Post-apply drift monitoring** — detecting mutation of an enforcement
  resource after binding.
- **Approval granularity** — per-domain or composite digests; separate
  decision, not required by this contract.
- **Derived-policy import boundary** — how an externally produced
  `SeccompProfile` becomes a governed artifact is a separate ADR. This
  contract applies identically to internally generated and externally
  derived `SeccompProfile` artifacts, because it constrains application
  order and readiness, not artifact provenance.

## Why this blocks v0.2

The defect is latent today only because SPO is absent from the
authoritative E2E, so the artifact that triggers it is always skipped. The
v0.2 SPO integration removes that mask. Shipping SPO without this decision
would give the governed apply path the ability to bind a workload to an
enforcement resource that is not ready — reproducing the documented
CrashLoop, and doing so at the exact moment the project claims that
approved authority is what gets enforced.

Sequence: this ADR → implementation → tests → SPO integration → new RC
certification.

## References

- `cmd/landlock-genprof/review.go:40-47` — `proposalArtifacts` order.
- `cmd/landlock-genprof/apply_proposal.go` — plan construction, existing
  Gates 1 and 2, sequential non-transactional apply loop.
- `internal/k8s/apply.go` — `applyPod` delete-and-recreate semantics.
- `internal/k8s/patch.go:161` — `securityContext.seccompProfile` merge.
- `internal/exporter/spo/export.go:100-121` — `LocalhostProfilePath`, the
  containerd failure mode, and the explicit note that this tool never
  waits for SPO reconciliation.
- `cmd/landlock-genprof/trace.go:53` — `podlock.kubewarden.io/profile`.
- `demo/golden/workload.yaml` — the recorded CrashLoopBackOff.
- [ADR-0001](0001-exit-code-contract.md) — exit-code contract (`2` =
  blocking failure).
- [ADR-0006](0006-security-profile-proposal-approval-binding.md) —
  approval binds exact candidate content.
- [`docs/enforcement-prerequisites.md`](../enforcement-prerequisites.md) —
  which controller enforces which artifact.
- [`docs/PROGRESS.md`](../PROGRESS.md) — demonstrated-capability ledger.
