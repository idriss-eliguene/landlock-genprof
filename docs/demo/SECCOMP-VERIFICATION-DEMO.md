# Bounded Seccomp twin-Pod verification demo

This demonstration is opt-in and requires an already configured Operations
Center deployment, an approved SPO-backed proposal, a ready target Pod, and a
dedicated verifier kubeconfig. It does not operate on or modify the
application's Pod.

## Enablement

1. Provision the resources in `deploy/rbac-seccomp-verifier.yaml` in the
   selected cluster. The verifier API identity is the
   `landlock-genprof-seccomp-verifier` ServiceAccount; the two probe Pods use
   the tokenless `seccomp-verifier` ServiceAccount. Store only the former's
   kubeconfig in the configured Kubernetes Secret.
2. Grant the intended operator `get` and custom `verify` access to
   `securityprofileproposals` using `deploy/rbac-proposal-verification.yaml`
   (or an equivalent namespace-scoped policy). Existing proposal status
   update authorization is still required to append evidence. Reviewer or
   approver permission alone does not grant verification.
3. Enable `operationsCenter.seccompVerification` in the Helm values, setting
   its namespace, kubeconfig Secret/key, and a digest-pinned probe image. The
   feature is disabled by default. Keep the verifier namespace restricted by
   Pod Security Admission.

## Run

1. Open the Operations Center and select the authorized cluster and proposal
   namespace.
2. Open an approved proposal that contains an SPO `SeccompProfile`. Select
   one currently ready replica and enter its exact Pod name and UID in the
   proposal detail's “Seccomp twin-Pod experiment” panel.
3. Choose “Run bounded twin-Pod experiment” once. The API checks the separate
   `proposal.verify` capability, approval digest and target/profile identity.
   It creates two fixed, short-lived Pods on the target node: one with the
   approved Localhost profile and one with `RuntimeDefault`; both run the
   same digest-pinned `getpriority(PRIO_PROCESS, 0)` probe.
4. Refresh proposal detail and inspect the appended per-Pod fact: target
   identity, approved profile identity/content digest, twin/control exits,
   verifier identity/image, observation and validity timestamps, and
   revocation state. The runner deletes only the exact Pod UIDs it created.

`VERIFIED` means only that the fixed probe returned `EPERM` in the twin and
succeeded under the `RuntimeDefault` control, with the recorded identity
checks satisfied. `NOT_VERIFIED` means both probes completed and the twin
probe succeeded. Missing control, runtime failures, cleanup uncertainty,
timeouts, or identity drift are `UNKNOWN`. A fact's display freshness is
time- and approval-bound; revocation remains `UNKNOWN` because this verifier
has no revocation source.

The result does not prove that the original application process has the
profile active, that the node-local profile bytes match the API object's
content, that other replicas were tested, or that any syscall other than
`getpriority` is denied. Materialization, target Pod configuration, and
twin-Pod behavior are distinct evidence dimensions.

## Disposable CI integration

The real probe/Pod runner integration is exercised in the disposable SPO Kind
workflow by:

```sh
bash test/e2e/seccomp-verification-runner.sh
```

That runner test provisions and removes only its own twin/control Pods in the
CI verifier namespace. It is not a local-cluster procedure and does not
exercise the Operations Center HTTP authorization or proposal persistence
path. Unit/API authorization and envtest persistence checks cover those
separately. No successful runtime qualification is claimed until that CI
workflow passes on the implementation commit.
