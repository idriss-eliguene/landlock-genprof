# v0.5.0 G5 technical certification record

This record preserves the distinction between the product commit on which
the evidence was produced and this documentation commit.

## Contract

- Gate: G5 / #187
- Product evidence SHA: `5881f9807395d1eee8e89426f32110867f41e8fb`
- Product evidence tree: `457dbfc7be3f2096ef2fa1f5b5164222d44f3051`
- Certification record SHA: the commit containing this file
- Certification record is not retroactive product evidence.

The normalized negative-state contract is:

- reachable negative states require an exercising negative test;
- architecturally unreachable states require structural production-path
  unreachability evidence and must not be represented as runtime-tested.

`association.Stale` and `association.Orphaned` exist in the vocabulary, but
the certified Workbench path supplies discovered targets and explicit
producer-time bindings. It does not invoke a production path that produces
either state. They are therefore recorded as structurally unreachable for
this release, not empirically exercised.

## G5/G6 split

G5 owns the immutable product-evidence identity, release-relevant test and
claim matrix, certification environment record, and technical limitations.
G6 owns completion of the operational pilot support matrix, pilot runbook,
abort/rollback instructions, operational recovery and handoff, and the final
pilot-readiness decision. This record does not predeclare any G6 result.

## Evidence

| Evidence | Run | Job | Result | Scope |
|---|---:|---:|---|---|
| Core E2E | 33651464273 | 100319256359 | PASS | Core governance and Cilium-bounded behavior |
| SPO Interop E2E | 33651464011 | 100319256080 | PASS | SPO interoperability and derived-policy provenance |
| SPO D-MIN E2E | 33651463917 | 100319254744 | PASS | Fail-closed composition behavior |
| Canonical demo | 33651461562 | 100319250397 | PASS | Controlled real-node demonstration |
| build-and-test | 33651462943 | 100319251985 | PASS | Build and test suite |
| security | 33651462943 | 100319251965 | PASS | Security checks |
| goreleaser-check | 33651462943 | 100319252367 | PASS | Release packaging check |

There is no `deploy-master` workflow in the evidence set; it is not
applicable and no run is claimed for it.

The evidence was produced by GitHub Actions on the product evidence SHA.
Per-run node, kernel, Kubernetes, runtime, CNI, and backend version details
remain attributable to the cited jobs; fields not reproduced in this record
are `UNKNOWN` / `NOT_RECORDED`, not inferred.

## Claim matrix

| Property | Status | Proof class | Evidence | Scope / limitation |
|---|---|---|---|---|
| Canonical `GovernedTarget` | PROVEN | static, unit, envtest | `internal/k8s`, `internal/workload`, Workbench tests | Supported discovered workloads |
| RuntimeSubject/provenance | PROVEN | unit, envtest, G4 evidence | projection and Workbench tests | Runtime evidence is not invented when absent |
| Namespace pinning | PROVEN | static, security, envtest | ADR-0021/0023 and Workbench tests | One configured namespace |
| Association and partial knowledge | PROVEN | unit, envtest | `internal/association`, `internal/projection` tests | Exclusions remain observable |
| HTTP read authority | PROVEN | static, security | ADR-0021/0023 and source checks | `WorkbenchReadCapability` only |
| HTTP mutation absence | PROVEN | static, security, envtest | Workbench boundary tests | Browser has no mutation route |
| Exact CLI next steps | PROVEN | unit, HTML tests | Workbench tests | Advisory POSIX-shell-safe text |
| PodLock + application-derived Seccomp refusal | PROVEN | real-node E2E | SPO D-MIN / Golden evidence | Refusal is not compatibility proof |
| SPO provenance | PROVEN | real-node E2E | SPO Interop and D-MIN | SPO output is derived policy |
| Coverage/confidence separation | PROVEN | unit, E2E | importer and review evidence | Coverage is not confidence or authority |
| NetworkPolicy behavior | PROVEN | real-node E2E | Core E2E | Cilium-bounded only |
| Apply transactionality | PROVEN boundary | static, unit, E2E | apply tests and docs | Sequential; no automatic rollback |
| Landlock kernel denial | NOT_PROVEN | none claimed | — | No universal denial claim |

## Explicit limitations

- Same-Pod PodLock plus application-derived Seccomp compatibility is
  `NOT_PROVEN`; the safe behavior for unproven compatibility is fail-closed
  refusal.
- Landlock kernel denial is `NOT_PROVEN`.
- NetworkPolicy behavioral evidence is `CILIUM_BOUNDED`.
- SPO is `DERIVED_POLICY`, not landlock-genprof observation.
- Syscall coverage is not confidence.
- Approval is not application; application is not enforcement; enforcement is
  not behavioral verification.
- Remote Workbench, authentication, multitenancy, SaaS, automatic
  remediation, universal workload support, and SPO annotation stability are
  outside scope.

## Release state

- G5: technical evidence proven; final certification pending independent
  challenge and this record.
- G6: in progress / not yet certified.
- v0.5.0: not yet release certified.
- Pilot support matrix, pilot runbook, abort/rollback procedure, and final
  pilot-readiness decision remain `PENDING_G6`.
