# Buyer-facing five-minute demo

This is the buyer cut of the canonical demo. It is for a Head of Platform,
DevSecOps lead, Kubernetes security lead, or platform architect. It shows the
released product boundary between security knowledge and explicit workload
authorization; it is not a vulnerability report, compliance certificate, or
claim that the resulting policy is globally minimal.

The live timer starts after preparation. Preparation uses a real-node k3s
cluster because the selected scenario uses real SPO recordings and the
project's Linux observation path. It may take several minutes. The buyer cut
itself uses the existing [`scenario.sh`](../scenario.sh), which remains the
single demo orchestrator and runs the real CLI and Kubernetes API path.

## Prepare

From a Linux host with the documented demo prerequisites:

```bash
./demo/setup.sh --with-cluster   # first run; provisions the real-node environment
./demo/setup.sh                  # later runs against the prepared environment
./demo/reset.sh --recreate-pod
```

Preparation must finish with `Demo environment ready`. It checks the
Kubernetes context, Inspektor Gadget readiness, required CRDs, the golden
workload, and two real SPO source profiles. Do not proceed when a check fails.
See [`demo/README.md`](../README.md) and
[`docs/enforcement-prerequisites.md`](../../docs/enforcement-prerequisites.md)
for environment boundaries.

## Five-minute live sequence

Use a terminal at least 100 columns wide. The fastest reproducible rehearsal
is:

```bash
DEMO_FAST=1 ./demo/scenario.sh
```

For a narrated session, run `./demo/scenario.sh --paced` and keep a second
terminal ready. At the candidate review pause, optionally run:

```bash
kubectl landlock-genprof ui nginx-demo -n landlock-genprof-e2e
```

Open the printed loopback URL in a browser. The Workbench is **experimental,
local, and read-only**. It is a snapshot of the proposal loaded before the
server starts; restart it to refresh. It has no approval or application
controls. Stop it with `Ctrl-C`, then continue the scenario in the first
terminal. If a browser transition would interrupt the session, keep the CLI
review as the primary surface; the security claims do not depend on the
browser.

| Time | Action | Buyer message |
|---|---|---|
| 00:00–00:30 | `doctor` and `abi check` run by the scenario | We qualify the environment before discussing authority. |
| 00:30–01:10 | The scenario imports a real SPO recording while observing filesystem and network behavior | SPO material is derived policy; it is not direct landlock-genprof syscall evidence. |
| 01:10–01:50 | `review` and optional Workbench snapshot show proposal identity, domains, provenance, and the exact candidate digest | A candidate is reviewable, but it has not been authorized. |
| 01:50–02:20 | `approve --expected-digest <digest A>` | Human approval binds to the exact candidate, not merely a proposal name. |
| 02:20–03:10 | The workload changes and a second real trace produces candidate B | New information does not inherit the approval for candidate A. |
| 03:10–03:45 | `apply-proposal ... --yes` refuses the stale approval | The changed candidate fails closed before application. |
| 03:45–04:20 | The scenario shows the bounded filesystem diff and unchanged source/provenance state | The refusal is an inspectable authority change, not a hidden hash error. |
| 04:20–05:00 | Candidate B is explicitly approved and governed apply completes where the prepared backend supports it | Approval, application, enforcement, and behavioral verification remain separate states. |

The two observation windows are real. `DEMO_FAST=1` removes presentation pauses,
not product operations. A recorded presentation may visibly accelerate the
observation windows only with a caption stating that they were sped up.

## What is demonstrated

- one real `SecurityProfileProposal` combines filesystem/network observation
  with an SPO-derived SeccompProfile;
- provenance remains visible and SPO policy is not relabeled as direct
  observation;
- the canonical candidate digest is approved explicitly;
- a changed candidate is rejected by governed apply when the old approval is
  stale;
- the successful path is still bounded by the backend and environment
  evidence actually available.

The scenario's assertions are part of the demo contract. A failed assertion
stops the demo. An expected stale-approval rejection is a successful
fail-closed demonstration, not a demo failure.

## What is not claimed

The demo does not claim that an unobserved permission is unnecessary, that an
observed behavior is legitimate, that a generated candidate is globally
minimal, or that API application proves enforcement. It does not certify
universal workload compatibility or same-Pod PodLock plus SPO non-interference.
The exact backend limitations are recorded in
[`docs/PROGRESS.md`](../../docs/PROGRESS.md) and
[`docs/enforcement-prerequisites.md`](../../docs/enforcement-prerequisites.md).

## Close and handoff

After presenting:

```bash
./demo/reset.sh
```

This removes demo-owned proposal, history, network policy, governed profile,
and local state while preserving the prepared cluster and SPO source
recordings. For a customer engagement, hand off the evidence and decisions
using the [pilot package](../../docs/pilot/README.md) and the
[Hardening Review template](../../docs/pilot/hardening-review-template.md).
The template is the customer deliverable; this runbook is only the demo
operation guide.

## Presenter claim matrix

| Claim | Evidence | Classification |
|---|---|---|
| Exact candidate approval binding | `approve --expected-digest` and governed apply revalidation | PROVEN |
| Stale approval fails closed | Candidate A approval followed by candidate B apply attempt | DEMONSTRATED / PROVEN by production validation |
| SPO source remains derived policy | Real SPO source profile and provenance output | DEMONSTRATED |
| Candidate identity and provenance are reviewable | `review` and optional local Workbench snapshot | DEMONSTRATED |
| API application equals enforcement | No supporting evidence in this cut | NOT_PROVEN; do not claim |
| Universal compatibility or global minimality | No supporting evidence | NOT_PROVEN; do not claim |

