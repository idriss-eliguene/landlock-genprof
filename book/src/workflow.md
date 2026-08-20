# The governed workflow

landlock-genprof separates evidence collection from deployment authority. A training run can create a candidate, but only an explicit approval of that candidate's digest authorizes it for application.

## Lifecycle at a glance

| Stage | Command | Result | Trust boundary |
|---|---|---|---|
| Check readiness | `doctor` | Environment and dependency diagnostics | A broken prerequisite must not look like policy failure |
| Observe and synthesize | `trace` | Runtime evidence, generated artifacts, and a published proposal | Observed behavior is evidence, not permission |
| Inspect | `review` | Candidate contents, confidence, readiness, and digest | The reviewer evaluates the exact candidate |
| Authorize | `approve --expected-digest …` | Approval bound to that digest | Changed content cannot inherit an older approval |
| Compare | `diff` | Material changes between candidates | A changed digest becomes reviewable, not merely different |
| Apply | `apply-proposal` | Available approved artifacts sent to their APIs | Missing, stale, or mismatched approval fails closed |
| Prove | `verify` and backend-specific checks | Evidence about realized policy and workload behavior | Applied does not automatically mean enforced or verified |

## 1. Check the environment

```bash
kubectl landlock-genprof doctor
```

Resolve host kernel or eBPF prerequisite failures before collecting evidence. `doctor` does not validate cluster CRDs, RBAC, or external backends; check those separately in [installation](INSTALL.md) and [enforcement prerequisites](docs/enforcement-prerequisites.md).

## 2. Observe and synthesize

Run the workload through representative behavior during the training window. In internal mode, landlock-genprof observes filesystem, network, syscall, and capability behavior. In SPO mode, landlock-genprof continues to observe filesystem and network behavior, while SPO supplies the real derived `SeccompProfile`; that policy enters as a derived artifact, never as landlock-genprof observation or authorization. The resulting artifacts are published together as a `SecurityProfileProposal`.

```bash
kubectl landlock-genprof trace \
  --pod nginx-demo -n default \
  --binary /usr/sbin/nginx \
  --duration 60s
```

Short or unrepresentative training can omit legitimate behavior. Multi-run history and confidence make that uncertainty visible; they do not eliminate it. See [the usage guide](docs/usage.md) for output options and [multi-run learning](docs/usage/multi-run-history.md) for history semantics.

## 3. Review the exact candidate

```bash
kubectl landlock-genprof review nginx-demo
```

Review rule coverage, confidence, artifact readiness, and the candidate digest. Treat low-confidence rules and absent expected behavior as reasons to retrain or investigate.

## 4. Approve by digest

```bash
kubectl landlock-genprof approve nginx-demo \
  --expected-digest sha256:<digest-from-review>
```

Approval binds authority to content, not merely to a proposal name. If a later training run changes the candidate, the previous approval no longer authorizes it.

If the candidate changes after approval, compare it before deciding again:

```bash
kubectl landlock-genprof diff <old-candidate-file> <new-candidate-file>
```

Review and approve the new digest explicitly. Importing an SPO-derived profile never inherits enforcement authority from SPO or from an earlier candidate.

## 5. Apply and verify

```bash
kubectl landlock-genprof apply-proposal nginx-demo
```

Application revalidates the digest before changing cluster resources. Each artifact also needs its own backend and readiness conditions. Confirm those in [enforcement prerequisites](docs/enforcement-prerequisites.md), then use the relevant behavioral checks. The [progress page](project/progress.md) records which backends have been demonstrated end to end.

## Where to go next

- Follow the [full usage guide](docs/usage.md) for task-oriented details.
- Use the [CLI reference](cli/landlock-genprof.md) for exact flags.
- Study [architecture](docs/architecture.md) for data flow and package boundaries.
- Read the [threat model](docs/threat-model.md) before production evaluation.
