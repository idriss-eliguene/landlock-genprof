# landlock-genprof Demo

## The idea

> **LEARNED ≠ AUTHORIZED**

Runtime learning is a solved problem, and other systems do it better than we
do. security-profiles-operator records syscalls with a production eBPF
recorder, merges across replicas, generates a `SeccompProfile`, installs it
on every node, and enforces it.

What no learner provides is a decision. A recorded profile is a description
of what a workload *did*. Enforcing it is a statement about what it is
*allowed* to do — and only a human can turn the first into the second.

landlock-genprof is the authorization boundary between runtime learning and
runtime enforcement. It takes what was learned — by SPO, or by its own
tracer — turns it into one reviewable candidate with one deterministic
identity, binds a human's approval to **that exact identity**, and refuses to
enforce anything else.

**Approve exactly what you reviewed.**

## What you will see

```
SPO recorded this workload and produced a valid SeccompProfile
    → the source profile is Disabled. Nothing enforces it.
    → import: SPO's policy enters as a CANDIDATE, not as authority
    → one candidate: filesystem + network + syscalls, one digest
    → review  → CandidateDigest A
    → approve exactly A
    → the workload changes, and SPO records it again
    → the proposal becomes candidate B
    → governed apply  → REFUSED: approved digest ≠ current digest
    → nothing was applied
    → what changed: filesystem diff + seccomp provenance
    → TrainingHistory: syscallAccesses = 0
    → review B → approve B → governed apply succeeds
    → SPO reconciles, identity verified, workload bound LAST
```

The learner did nothing wrong. SPO produced a legitimate, better-informed
profile. It still did not acquire the authority to enforce it.

**Learning is automatic. Authority is not.**

## Why the learner is SPO

The previous cut proved the same invariant using drift our own tracer
produced. That works, but invites one objection: *your tool changed its own
output, so of course the digest moved.*

Sourcing the drift from security-profiles-operator removes it. SPO is a
legitimate upstream system with a better syscall instrument than this
project's own. Nobody attacks anything, nothing malfunctions, and the learner
is **right** — its second profile is a better description of the workload
than its first. The refusal is the product.

This is also why the demo is not an SPO demo. SPO does the observing, the
generating, the reconciling and the enforcing. What it has no mechanism for
is a decision, and that is the only thing this project adds.

## What this proves

Each of these is exercised by a real command in `scenario.sh`, against a
real cluster:

- Behavior is observed with eBPF and accumulated across runs in a
  `TrainingHistory` resource (`runsRecorded` climbs 1 → 2 → 3).
- Each synthesized rule carries a confidence level derived from how many
  runs saw it — `low` = 1, `medium` = 2, `high` = 3 or more — plus the
  Landlock ABI level and minimum kernel each right requires.
- A `SecurityProfileProposal` is published to the cluster with four
  artifacts, and reduces to one `sha256:` **CandidateDigest**.
- `approve` refuses any digest that does not match the current candidate.
- The approval is persisted in the resource's status as
  `approvalState`, `approvedCandidateDigest` and `approvalMechanismVersion`.
- Re-tracing updates the proposal spec **and preserves the previous
  approval status** — which is exactly how a stale approval arises.
- `apply-proposal` recomputes the digest over the current spec and fails
  closed when it does not match the approved one. **The rejection happens
  before the first API application**: the run below shows the target
  NetworkPolicy absent before the attempt and still absent after.
- `diff` shows which rule changed, so a digest mismatch is legible as a
  privilege change rather than a hash comparison.
- Re-reviewing and re-approving the new candidate makes the governed apply
  succeed, and the artifact is created through the Kubernetes API.

## What this does NOT prove

Be precise about this. The demo deliberately stops at the Kubernetes API
boundary.

- **No PodLock consumption, and no Landlock kernel enforcement.** The
  LandlockProfile artifact is generated and can be applied, but no PodLock
  operator consumes it here. `docs/PROGRESS.md` records this as `BLOCKED`.
- **No SPO behavioral seccomp enforcement.** The SeccompProfile artifact is
  generated; no syscall is ever denied in this demo. Also `BLOCKED`.
- **No behavioral enforcement of the generated NetworkPolicy.** It is
  applied through the API and confirmed present. Whether traffic is then
  blocked is the CNI's job and is not demonstrated here. (The project has
  demonstrated NetworkPolicy blocking under Cilium separately, using a
  fixture policy in `test/e2e/smoke-networkpolicy.sh` — that is a different
  artifact and a different claim.)
- **No external evidence ingestion.** Observation comes from this project's
  own tracer.
- **No controller or operator reconciliation.** Everything here is
  CLI-driven; there is no control loop.
- **No transactional apply.** The rejection path applies nothing at all,
  which is what this demo shows. But a *successful* apply is sequential and
  continues past a failed artifact, so "nothing is ever partially applied"
  is not a property this product has and is not claimed.

`docs/PROGRESS.md` is the canonical, capability-by-capability record of
what is demonstrated.

## Architecture

```
  demo/scenario.sh          disposable orchestration
        │                   sequences commands, prints separators,
        │                   captures output, waits on readiness
        │  invokes
        ▼
  kubectl landlock-genprof  the real CLI — every security decision
        │                   (synthesis, digest, approval validation,
        │                   pre-apply revalidation) happens here
        ▼
  real Kubernetes cluster   real CRs, real API application
        │
        ▼
  real product state        TrainingHistory, SecurityProfileProposal,
                            approval status, applied artifacts
```

The orchestration never computes a digest, never classifies confidence,
never decides whether an approval is valid and never predicts an apply
outcome. Where it needs a digest, it passes back the string the product
printed. The only thing it branches on is a real command's exit status.

## Requirements

- A Kubernetes cluster with **Inspektor Gadget** installed and Ready
  (`make e2e-install` provides it), and the project's CRDs applied.
- `kubectl`, and the CLI installed as a kubectl plugin
  (`make install-plugin`). Set `LANDLOCK_GENPROF_BIN` to override with an
  explicit binary.
- **Linux** for the observation stages: the tracer uses eBPF and is
  Linux-only by design (`internal/tracer/trace_other.go` returns a clear
  error elsewhere). See `HOW_TO_START.md` for the dev VM.
- `python3`, used only to pretty-print status fields from `kubectl -o json`.
- `bash` 4+.

## Quick Start

```bash
./demo/setup.sh --with-cluster   # first time: real-node k3s + deps + SPO + recordings
./demo/setup.sh                  # subsequent: provision into an existing cluster
./demo/reset.sh                  # clear product state from a previous take
./demo/scenario.sh               # the canonical hero demo, start to finish
./demo/appendix.sh               # the technical appendix, same cluster
```

`--with-cluster` provisions a **real-node k3s cluster**, not kind, and
pre-bakes two real SPO recordings (~127 s each). That is the slow part and it
happens once, before the camera rolls.

Set `DEMO_FAST=1` to collapse the presentation pauses — useful in CI, wrong
for a live audience:

```bash
DEMO_FAST=1 ./demo/scenario.sh
```

## Full Step-by-Step

`scenario.sh` runs seventeen stages:

| Stage | What happens |
|---|---|
| 1 | Baseline: the workload's `securityContext`, no proposal, no history |
| 2 | Three real training runs, each driving deterministic behavior |
| 3 | `TrainingHistory` shows `runsRecorded` = 3 |
| 4 | `explain` renders per-rule confidence, rights, ABI, evidence |
| 5 | `review` prints the WORKLOAD SECURITY REVIEW and CandidateDigest A |
| 6 | `approve --expected-digest <A>`; status read back from the cluster |
| 7 | The workload starts writing a new path |
| 8 | A fourth training run; the proposal becomes candidate B |
| 9 | The approval status is still the one bound to A |
| 10 | Confirm the target NetworkPolicy does not exist yet |
| 11 | `apply-proposal` against the stale approval → refused, exit 1 |
| 12 | The NetworkPolicy still does not exist |
| 13 | `diff` shows which rule changed |
| 14 | `review` again → CandidateDigest B |
| 15 | `approve --expected-digest <B>` |
| 16 | `apply-proposal` → applies the NetworkPolicy |
| 17 | Final approval status and applied resources |

The demo drives behavior through two scripts. `demo/golden/run-actions.sh`
is the Golden fixture the authoritative E2E already owns and asserts on —
it produces the 3-of-3 / 2-of-3 / 1-of-3 frequencies that make confidence
tiers visible. `demo/drift-action.sh` is demo-only and adds the one new
behavior after approval, so the E2E fixture never has to change to serve a
presentation.

## Expected Outputs

Real excerpts from a full run. Digests and timestamps differ per run;
the shapes do not.

Approval bound to candidate A (stage 6):

```
landlock-genprof-e2e/nginx-demo: Approved
  Reason: reviewed with the platform team

  the approval, read back from the cluster:
  approvalState: Approved
  approvedCandidateDigest: sha256:306eac30863a2c51299d755504d5a69a9eb4cccf512a815ab9e626efef7c75cc
  approvalMechanismVersion: candidate-v1
```

The refusal (stage 11) — this is the product's own wording, verbatim:

```
apply preflight failed: approved candidate digest mismatch: approved=sha256:306eac30863a2c51299d755504d5a69a9eb4cccf512a815ab9e626efef7c75cc computed=sha256:58614d8cf24d261197ca61e851828197ff2f064e275ebe696ca819d180a36643

  exit status: 1
```

Nothing applied, before and after the attempt (stages 10 and 12):

```
  No resources found in landlock-genprof-e2e namespace.
```

What actually changed (stage 13):

```
+ /srv/nginx/data: [write_file truncate]

  diff exit status: 1  (0 = identical, 1 = differences found)
```

Governed apply after re-approval (stage 16):

```
This will apply 1 artifact(s):
  - NetworkPolicy

Planned artifacts:
  - NetworkPolicy: networking.k8s.io/v1, Kind=NetworkPolicy landlock-genprof-e2e/nginx-demo

applied: NetworkPolicy

Done.
```

## Recording

See [`recording.md`](recording.md) for the full procedure.

```bash
./demo/record.sh signature   # full canonical scenario
./demo/record.sh hero        # short cut
```

## Live Presentation

See [`live-checklist.md`](live-checklist.md). The short version: pre-provision
everything, keep a recording of the identical run one keystroke away, and
**never debug live** — when the climax of your demo is a refusal, an
audience cannot tell "the demo broke" from "the product refused."

## Troubleshooting

**`kubectl-landlock_genprof not found on PATH`** — run `make install-plugin`,
or set `LANDLOCK_GENPROF_BIN` to a binary path.

**`kubeconfig context is '…', expected 'kind-landlock-genprof-e2e'`** —
every script fails closed on this deliberately. Switch context or set
`DEMO_EXPECTED_CONTEXT`.

**`Inspektor Gadget DaemonSet not found`** — run `./demo/setup.sh
--with-cluster`, or `make e2e-install` against the cluster.

**Trace reports `0 item(s)` for every domain** — the tracer attached but saw
nothing. Either the actions did not run (check `demo/.state/actions.log`),
or the traced binary does not match the process doing the work. The demo
traces `/usr/bin/curl` in the `tools` sidecar because curl performs every
deterministic action; tracing the `nginx` container instead observes an
idle server.

**Trace fails on macOS/Windows with a platform error** — expected. The
tracer is Linux-only. Use the dev VM (`HOW_TO_START.md`).

**`runsRecorded` is empty or not incrementing** — the field is on `.spec`,
not `.status`. If it is genuinely not incrementing, the `--history` flag or
its extra RBAC is missing (`docs/usage.md`).

**`This gadget was built with ig vX and it's being run with vY`** — a
warning, not an error: the cluster's Gadget DaemonSet is a different version
from the client. Traces still work. Reinstall the Gadget to match if the
noise bothers you on camera.

**`review` prints no `Candidate digest:` line** — the proposal has no spec
worth digesting, usually because the training run observed nothing. Fix the
trace first; the scenario stops here on purpose rather than approving
something empty.

**`approve` fails with `expected candidate digest mismatch`** — the
proposal changed between `review` and `approve`. Re-run `review` and use the
digest it prints. This is the mechanism working, not a bug.

**The stale apply unexpectedly *succeeds*** — the scenario stops and says
so. It means the retrace produced a byte-identical candidate, so the
approval is still valid. Ensure the drift action actually ran and wrote a
genuinely new path (`demo/.state/actions.log`).

**The apply is refused for a different reason** — read the message. Common
alternative: `proposal not Approved` if a `reject` ran, or `authorization
changed before apply` if the proposal changed during an interactive
confirmation. Both are the same fail-closed gate at a different point.

**`diff` prints `command reported a non-zero exit condition`** — that is the
documented exit-code contract: 0 identical, 1 differences found, 3 error.
The scenario routes it to `demo/.state/diff-a-b.err` so the rule lines read
in order.

**`PodLock` / `SPO SeccompProfile` show as skipped** — intended. Those
operators are not installed, so the scenario passes
`--skip=podlock,spo-seccompprofile`. Never work around this by claiming
enforcement that did not happen.

**Status not visible immediately after `approve`** — status propagation is
asynchronous. Re-read the resource.

**Stale resources from a previous take** — `./demo/reset.sh`, or
`./demo/reset.sh --recreate-pod` for a fully clean workload filesystem.

**Pod `apply` fails with `spec: Forbidden: pod updates may not change
fields…`** — a Pod spec is immutable and an older fixture revision is
running. `setup.sh` detects this and recreates the pod.

**Digest lines wrap and become unreadable** — use a terminal of at least
100 columns. `record.sh` refuses to record below that.

**ARM64 vs AMD64** — both work; the observed syscall set can differ. Never
read a specific event count aloud during a presentation.

**Image pulls stall** — pre-pull `hashicorp/http-echo:0.2.3` and
`curlimages/curl:8.3.0` into the cluster before presenting.

## Reset

```bash
./demo/reset.sh                  # proposal, approval, history, NetworkPolicy, local state
./demo/reset.sh --recreate-pod   # also recreate the workload pod
```

Reset touches only demo-owned resources in the demo namespace. It never
deletes the cluster.

## Reproduce from Scratch

```bash
git clone https://github.com/idriss-eliguene/landlock-genprof
cd landlock-genprof
make install-plugin
./demo/setup.sh --with-cluster
./demo/reset.sh
./demo/scenario.sh
```

## Evidence

The properties this demo shows are backed by tests and CI, not by the demo
itself:

| Property | Evidence |
|---|---|
| Digest is deterministic over selected spec fields | `internal/proposal/digest.go`, proposal tests |
| Approve rejects a non-matching digest | `internal/proposal/store.go`, `cmd/landlock-genprof/approve_test.go`, authoritative E2E |
| Apply fails closed on a digest mismatch | `internal/proposal/validate.go`, `TestRunApplyProposal_RejectsAfterSpecMutation` |
| Re-trace preserves approval status | `TestSave_DoesNotClobberApprovalStatus` |
| Pre-apply revalidation after planning | `TestRunApplyProposal_RejectsMutationAfterPlanningBeforeRevalidation` and siblings |
| Approval cannot be forged through a spec write | `internal/proposal/store_envtest_test.go` (`TestUpdateCannotModifyStatus`), run in CI via `make envtest` |
| Confidence tiers from cross-run frequency | `internal/policy/synthesize.go`, `internal/landlock/kernel.go` |
| Governed apply path and artifact plan | `cmd/landlock-genprof/apply_proposal.go`, apply tests, authoritative Core E2E |
| Capability-by-capability claim boundary | [`../docs/PROGRESS.md`](../docs/PROGRESS.md) |
