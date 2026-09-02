# Demonstrated capabilities

This page is the canonical record of what the project has demonstrated. It is deliberately separate from the [product roadmap](PRODUCT_ROADMAP.md), which records intended future capabilities.

## How to read status claims

The lifecycle terms are cumulative and must not be collapsed:

| Term | Meaning |
|---|---|
| **Observed** | Runtime behavior was captured by an identified source |
| **Derived** | Evidence was transformed into a policy artifact |
| **Proposed** | Artifacts were assembled into a candidate |
| **Reviewed** | A human inspected that candidate |
| **Approved** | Human authority was bound to the exact candidate digest |
| **Applied** | An approved artifact was submitted to its API |
| **Enforced** | The external backend realized the policy |
| **Verified** | A behavioral check proved the expected restriction |

Code existence, artifact generation, and API persistence are not evidence of behavioral enforcement.

## Current capability ledger

| Capability | Status | Demonstrated scope | Remaining limit |
|---|---|---|---|
| Evidence model and internal trace acquisition | Done | Workload filesystem, network, syscall, and capability evidence can be captured and validated | The internal tracer is an acquisition adapter, not a claim over external observations |
| Multi-run learning | Done | `TrainingHistory`, `seenInRuns`, and confidence behavior demonstrated across three runs | SPO-derived syscalls are structurally excluded from this history |
| Candidate generation | Done | Filesystem, NetworkPolicy, seccomp, and capability artifacts can form one `SecurityProfileProposal` | Generated does not mean authorized |
| Candidate identity | Done | Selected proposal fields produce a deterministic `sha256:` candidate digest | Digest vectors must evolve with the proposal schema |
| Governed approval | Done | Review, digest-bound approval, rejection, and persisted mechanism version demonstrated | Reviewer UX and structured rationale remain incomplete |
| Stale-authority rejection | Done | Changed, revoked, missing, or mismatched approval fails closed before application | New approval states require equivalent negative coverage |
| Governed apply | Done | `apply-proposal` reloads and revalidates the candidate, applies in governed order, and binds the workload last | Successful multi-artifact application is sequential, not transactional |
| NetworkPolicy | Verified on Cilium | Generated, approved, applied, realized, and fresh connections denied | This does not generalize to every CNI or workload |
| PodLock/Landlock artifact | Applied path only | Generation and approval-bound API application are implemented and tested | PodLock consumption and kernel denial are not demonstrated |
| SPO reconciliation | E2E-proven | A governed cluster-scoped SPO `SeccompProfile` was applied, reconciled, identity-checked, bound to a running workload, and exercised at the syscall boundary in real-node run `32561123023` | Evidence covers the tested candidate and syscalls, not every profile or runtime combination |
| SPO-derived policy import | E2E-proven | Real `ProfileRecording` with `mergeStrategy: Containers` produced two partial profiles and an exact merged union; explicit merged-provenance import, v1 coverage normalization, widening review, approval, apply, and stale-authority rejection were demonstrated | Coverage remains optional informational metadata; contributor lineage, confidence, and authority are not inferred |
| Governed Seccomp runtime boundary | E2E-proven | In run `32561123023`, approved `getpid` succeeded and naturally absent `getpriority` succeeded in control but returned `EPERM` under the applied governed profile | This is a tested behavioral boundary, not universal least privilege or complete Seccomp verification |
| Candidate explanation and diff | Done | `explain` and `diff` render evidence and candidate changes | Broader assurance/rationale UX remains future work |
| Operator/controller reconciliation | Not started | No controller capability is claimed | Requires a separate product contract and E2E evidence |

## SPO source boundary

SPO owns syscall observation and produces the real derived `SeccompProfile` in SPO mode. landlock-genprof does not claim those syscalls as its own observations, does not insert them into `TrainingHistory`, and does not invent confidence for them.

landlock-genprof observes filesystem and network behavior in that mode. It imports the SPO result as **derived policy**, creates a governed snapshot with provenance, includes it in the candidate digest, and requires human approval of that exact candidate. The SPO source object is not mutated and supplies no deployment authority.

The normative contract is [ADR-0008](adr/0008-spo-derived-policy-import-boundary.md). Apply ordering and fail-closed readiness are defined by [ADR-0007](adr/0007-governed-apply-ordering-and-enforcement-readiness.md).

## Authoritative evidence

- Buyer-facing demo run [`33332753584`](https://github.com/idriss-eliguene/landlock-genprof/actions/runs/33332753584), SHA `bfc3132ba4d51db5ca3b6dff84467a4a99f8436f`: the documented real-node
  scenario completed successfully. It produced candidate A
  `sha256:3432b896c62d6d386abcb5a59c23afa7341881edde1c9cb807abeb52cb4f4e8d`
  and candidate B
  `sha256:dba2222af3d77530effca0d2f01b954bead53501a876e4085f6dde9ce89348f8`;
  the stale approval for A was refused before application with the canonical
  digest-mismatch error, the real SPO source was shown as derived policy, and
  candidate B reached the governed apply path. The instrumented buyer cut
  measured `BUYER_DEMO_ELAPSED_SECONDS=94.468`, excluding preparation and
  cleanup. This demonstrates the scenario and timing in that tested
  environment; it does not prove universal compatibility, global minimality,
  or behavioral verification for every backend.

- Core E2E `32037183484`, SHA `902f99228203a27aeb52da11f301760d8bc5ff60`: multi-run confidence, proposal generation, digest-bound approval/apply, and Cilium NetworkPolicy behavioral verification.
- SPO Interop E2E `32230551571`, SHA `0e6ce7062f0cfd1b80bc42f654e371ae2a275f65`: SPO v1 profile application, reconciliation, governed identity, and workload binding. This is not syscall-denial evidence.
- SPO D-MIN E2E `32264419754`: real SPO recording, derived-policy import, governance, approval, apply, and a running bound workload. This is not a claim that SPO output is raw landlock-genprof evidence.
- SPO merged-provenance E2E [`32561123023`](https://github.com/idriss-eliguene/landlock-genprof/actions/runs/32561123023), SHA `5fb93a45aa724e9b1a9021b96ad1da1b54911bde`: two real `Containers` contributors, exact union and #3355 coverage (`v1`, `total=2`), widening visibility, normalized digest/approval, stale-authority rejection, and governed runtime Seccomp behavior. `getpid` was present and succeeded; naturally absent `getpriority` succeeded in control and returned `EPERM` after apply. The target referenced `operator/lg-v1-merged-target-2ed57712c490f4d5.json` and the reviewed digest was `sha256:f0d4f5116d6aca3dc0233ff15fbaea914411ffafc93602338a29be2bc432b5e3`.

## Current verification gates

1. Install a compatible PodLock environment and demonstrate Landlock behavioral denial.
2. Define capability/security-context verification evidence separately from artifact application.
3. Complete the reviewer rationale and assurance experience without weakening digest-bound authority.

## v0.5.0 Cluster Workbench release status

G0, G0.5, G1, G1.5, and G1.6 are closed. G2, G3, G4, and G5 are
`CERTIFIED_AND_MERGED`. G5 merge commit:
`cde96c3178129146970b9a03071d2b7c4ef6bbc9`; master tree:
`62ccc3a88e3e49aeaa06745148f220c0573ab337`. G6 is `IN_PROGRESS` for pilot
readiness and operational closure. v0.5.0 remains `NOT_YET_RELEASE_CERTIFIED`.

The full G5 evidence and claim boundary are recorded in
[G5-CERTIFICATION.md](G5-CERTIFICATION.md) and the
[pilot package](pilot/README.md). This status does not claim
Landlock kernel denial, same-Pod PodLock plus application-derived Seccomp
compatibility, generic NetworkPolicy behavior, or transactional apply.

G6 package status: `PILOT_SUPPORT_MATRIX = COMPLETE`, `PILOT_RUNBOOK =
COMPLETE`, `ABORT_ROLLBACK_PROCEDURE = COMPLETE`, `EVIDENCE_HANDLING =
COMPLETE`, and `PILOT_READINESS = NOT_YET_CERTIFIED` pending final gate
review. A fresh customer pilot execution is not claimed by this package.

Deferred custody work remains bounded: v0.5.1 for durable `ApplyAttempt` and
pre-state mutation custody; v0.5.2 for explicit `rollback <attempt-id>`;
v0.6.0 for the Full Visual Workbench; and v0.7.0+ for browser mutation via an
explicit mutation executor. No durable apply journal or rollback command is
part of v0.5.0.

Release certification procedures belong in [CONTRIBUTING.md](../CONTRIBUTING.md). Historical evidence and planning documents remain in the repository but are not current status authority.
