# Usage — the governed lifecycle

The normal product path is not a collection of exported files. It is a governed transition from knowledge to exact human authority:

```text
diagnose → select sources → acquire → propose → review → approve → apply → verify
```

This page assumes the CLI, RBAC, and CRDs are installed. See [installation](../INSTALL.md) or [test environment](test-environment.md) first.

## 1. Diagnose

```bash
kubectl landlock-genprof doctor
```

`doctor` checks the host kernel's Landlock support and local eBPF `bpffs` prerequisite. It does not validate cluster RBAC, CRDs, CNI behavior, or external operators; those remain installation and [enforcement prerequisite](enforcement-prerequisites.md) checks.

## 2. Select acquisition and policy sources

Seccomp source selection is explicit and is never inferred from cluster state.

| Mode | Syscall observer | Seccomp artifact | History/confidence |
|---|---|---|---|
| `--seccomp-source=spo` | SPO | Governed snapshot of SPO's derived `SeccompProfile` | No syscall `TrainingHistory`; confidence not applicable |
| `--seccomp-source=internal` | landlock-genprof tracer | Internally synthesized advisory profile | Syscalls may participate in landlock-genprof history/confidence |

SPO mode is the primary integration when real SPO-derived policy is available. landlock-genprof still observes filesystem and network behavior in this mode. The SPO artifact enters as derived policy with provenance, participates in the candidate digest, and receives no authority until the exact digest is approved. See [SPO-derived policy import](usage/spo-seccomp-import.md).

## 3. Acquire and publish a candidate

### SPO-derived seccomp path

After SPO has completed the named `ProfileRecording` and produced the named cluster-scoped profile:

```bash
kubectl landlock-genprof trace \
  --pod nginx-demo --namespace default \
  --binary /usr/sbin/nginx --duration 60s \
  --seccomp-source=spo \
  --spo-recording nginx-demo \
  --spo-profile nginx-demo-nginx
```

The source profile is validated for API shape, completion, lineage, inertness, and supported semantics. Import creates a governed snapshot; it does not mutate or retain a live reference to the source.

### Internal/advisory seccomp path

```bash
kubectl landlock-genprof trace \
  --pod nginx-demo --namespace default \
  --binary /usr/sbin/nginx --duration 60s \
  --seccomp-source=internal --history
```

`internal` is the current default and remains supported. Its syscall capture has a wider node-level observation caveat documented in the [threat model](threat-model.md). Use representative workload traffic; a short or incomplete run can omit legitimate behavior.

Both paths publish a `SecurityProfileProposal`. Proposal publication is mandatory, not an opt-in export. The proposal is the review boundary; its `CandidateDigest` gives exact content identity but does not itself grant authority.

Use `--restart` when startup-only behavior must be captured. It is disruptive and requires separate RBAC; see [target restart](usage/target-restart.md).

## 4. Review

```bash
kubectl landlock-genprof review nginx-demo --namespace default
```

Review the candidate contents, artifact availability, source provenance, confidence where applicable, and the printed candidate digest. For SPO-derived syscalls, the source is SPO, the epistemic class is derived policy, coverage is `unknown` unless SPO supplied it, and confidence is not applicable.

## 5. Explain or compare when necessary

`explain` and `diff` currently operate on raw Landlock candidate JSON files, not on the complete `SecurityProfileProposal`:

```bash
kubectl landlock-genprof explain \
  --candidate-file nginx-demo-candidate.json

kubectl landlock-genprof diff \
  nginx-demo-candidate-old.json nginx-demo-candidate-new.json
```

Use them to inspect filesystem rights, ABI requirements, evidence counts, or rule changes. They do not replace review of mixed-origin proposal artifacts or approval of the proposal digest.

## 6. Approve the exact digest

```bash
kubectl landlock-genprof approve nginx-demo \
  --namespace default \
  --expected-digest sha256:<digest-from-review> \
  --reason "reviewed with the platform security team"
```

Approval binds human authority to that exact candidate. If any digested content or provenance changes, the candidate digest changes and the previous approval becomes stale. Re-review and explicitly approve the new digest; authority never transfers by proposal name.

## 7. Governed apply

```bash
kubectl landlock-genprof apply-proposal nginx-demo --namespace default
```

The command fails closed before application when approval is absent, malformed, revoked, stale, or mismatched. It applies available enforcement artifacts in governed order. The patched workload manifest is deliberately excluded unless `--restart` is passed:

```bash
kubectl landlock-genprof apply-proposal nginx-demo \
  --namespace default --restart
```

With workload binding enabled, supported backend readiness and identity are checked before the workload is bound, and approval is revalidated immediately before binding. Application is sequential rather than transactional: a failure stops the remaining sequence, but previously applied resources are not rolled back.

## 8. ApplyAttempt custody and explicit rollback

Every governed apply creates a durable namespaced `ApplyAttempt` before the
first target mutation. Each mutation records its canonical target, live UID
and resourceVersion, controlled Before state, intended state, observed state,
and typed result where the outcome is known. The attempt remains
`IN_PROGRESS` while active and can finish as `APPLIED`, `PARTIALLY_APPLIED`,
`FAILED`, or `OUTCOME_UNKNOWN`. A current custody epoch qualifies newly
created attempts for rollback; missing or stale qualification is refused.

ApplyAttempt is custody, not approval. Approval remains bound to the exact
proposal candidate digest, and application remains distinct from enforcement
and behavioral verification. Apply is sequential and nontransactional; prior
successful mutations are not automatically undone.

An operator may explicitly request rollback of an eligible ApplyAttempt:

```bash
kubectl landlock-genprof rollback <apply-attempt> --namespace default
```

Rollback creates a separate durable `RollbackAttempt`. Before each inverse it
requires the source-attributable UID and resourceVersion and recorded
controlled state to match the live object. It restores only recorded
controlled Before state. Processing is dependency-aware: references are not
restored to unready policies and a policy is not deleted while a relevant
workload still references it.

Rollback uses Kubernetes RBAC plus explicit CLI confirmation, not a new
proposal approval. It is sequential and nontransactional. Partial results
remain durable; a definite `FAILED_NO_EFFECT` inverse may be reconsidered by a
later explicit continuation after fresh guards, while an `OUTCOME_UNKNOWN`
descendant is not automatically redispatched. Bare-Pod delete-then-create
records and rollback-of-rollback are unsupported. Rollback does not claim
atomicity, automatic recovery, compensation, exactly-once execution, generic
Kubernetes, kernel, CNI, or external-backend restoration.

## 9. Verify

The current `verify` command checks a Landlock candidate against a target kernel's ABI. It does not prove backend reconciliation or behavioral denial:

```bash
kubectl landlock-genprof verify \
  --candidate-file nginx-demo-candidate.json \
  --kernel 6.8
```

Backend-specific verification remains separate: confirm CNI realization and fresh network behavior, SPO reconciliation and workload binding, or PodLock/Landlock behavior as appropriate. Current demonstrated limits are recorded in [PROGRESS.md](PROGRESS.md): NetworkPolicy denial is Cilium-specific; SPO syscall denial and PodLock/Landlock kernel denial remain unproven.

## Advanced artifact outputs

Standalone outputs are useful for inspection, compatibility, and offline workflows. They are not equivalent to proposal review, digest-bound approval, or governed apply.

| Flag | Output | Detail |
|---|---|---|
| `--candidate-out` | Raw Landlock candidate JSON for `explain`, `diff`, and `verify` | Optional; omit the value to use the default filename |
| `--events-out` | Raw captured events for offline `synthesize` | Optional; omit the value to use the default filename |
| `--network-out` | Kubernetes `NetworkPolicy` | [NetworkPolicy output](usage/network-policy.md) |
| `--history` | Persist cross-run evidence in `TrainingHistory` | [Multi-run history](usage/multi-run-history.md) |
| `--seccomp-out` | Plain internally synthesized seccomp JSON | [Internal seccomp output](usage/seccomp-profile.md) |
| `--capabilities-out` | Capabilities add/drop fragment | [Capabilities output](usage/capabilities-fragment.md) |
| `--security-context-out` | Composed `securityContext` fragment | [securityContext output](usage/composed-security-context.md) |
| `--report-out` | Combined Markdown review report | [Report output](usage/unified-report.md) |
| `--patched-manifest-out` | Workload manifest with generated security context | [Patched manifest](usage/patched-manifest.md) |
| `--seccomp-profile-out` | SPO `SeccompProfile` resource | [SPO resource output](usage/seccompprofile-resource.md) |

Applying one of these files directly bypasses the proposal authority check. It must not be described as equivalent to `approve` plus `apply-proposal`.

## Operational prerequisites

- `NetworkPolicy` requires a CNI that implements it.
- `LandlockProfile` requires PodLock and a compatible Landlock environment.
- `SeccompProfile` requires SPO; reconciliation and runtime materialization are external to landlock-genprof.
- Workload `securityContext` fields are immutable on a running bare Pod, which is why governed binding is an explicit, disruptive operation.

See [enforcement prerequisites](enforcement-prerequisites.md) for installation and demonstrated-environment detail, and the [published CLI reference](https://idriss-eliguene.github.io/landlock-genprof/cli/landlock-genprof.html) for exact flags and exit behavior.
