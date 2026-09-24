# Namespace Authorization Security Review

Date: 2026-09-24
Repository: `idriss-eliguene/landlock-genprof`
Scope: unpublished authorization work from `1888175197342e5f0683186b68a387a666180c60`

## Executive summary

The original namespace-access commit is preserved on
`wip/namespace-access-foundation`. It is not an ancestor of the released
master, but its implementation was subsequently integrated and extended by
later Operations Center commits. A second cherry-pick was therefore not
appropriate; attempting it produced only expected documentation/test
conflicts and was aborted without changing the branch.

The implementation now provides namespace pinning, request identity
propagation, Kubernetes API-server authorization, independently configured
review and approval roles, digest-bound approval, resource-version
compare-and-swap, explicit observation endpoint gates, and a separate
technical identity for cluster-scoped SPO profile realization.

The two original P1 findings are resolved in the application and deployment
boundary. Kubernetes still exposes review and approval through the same status
subresource permission, so the product role decision is enforced by signed
trusted-proxy group membership on the server. Human/team RoleBindings no
longer contain cluster-scoped SeccompProfile permissions; only the separately
bound profile-realizer identity receives them.

## Repository custody

| Item | Result |
|---|---|
| Current branch | `feat/namespace-authorization` |
| Target baseline | `origin/master` |
| Target master SHA | `b8f2049` (`origin/master` at qualification) |
| Published v0.10.0 tag | `c2b2d8c332456e30cd13f88be4c3d32ca23babe7` |
| Original authorization commit | `1888175197342e5f0683186b68a387a666180c60` |
| Recovery branch | `wip/namespace-access-foundation` |
| Original commit on released master | No; functionality is present through later commits |
| Unrelated untracked files | Preserved: `DEMO_B1_REPORT.md`, existing `docs/engineering/` work |
| Product code changed by this review | Yes; scoped authorization, endpoint, realization, RBAC, tests, and documentation changes |

The recovery branch contains the complete original commit. The original
commit is reachable from `feat/operations-center-v09-m2` and the recovery
branch; it was not deleted or rewritten. The analysis branch was created from
the latest `origin/master` and does not contain the unpublished authorization
commit as a separate commit.

## Implementation inventory

### Authentication and identity

Authenticated Operations Center mode uses a trusted external proxy with a
timestamped HMAC-signed identity. User and group names are normalized and
system principals are rejected. The backend may impersonate only configured
principals. Production configuration requires the HMAC secret, identity
allowlist, namespace/session configuration, executor kubeconfig, and allowed
host settings.

The local kubeconfig/development mode remains a trusted single-user adapter.
It is not a multitenant authentication boundary and must not be exposed as
such.

### Authorization

`internal/authz` evaluates bounded capabilities using namespace-scoped
SelfSubjectAccessReviews. The actual request then uses a request-scoped
impersonated Kubernetes client, so capability discovery is advisory rather
than a substitute for API-server authorization.

The implemented capability inventory is:

| Capability | Effective Kubernetes check |
|---|---|
| `workload.view` | Pod list |
| `observation.view` | Observation list |
| `observation.operate` | Observation create/status update |
| `proposal.view` | SecurityProfileProposal list |
| `proposal.generate` | SecurityProfileProposal create |
| `proposal.review` | SecurityProfileProposal status update |
| `proposal.approve` | SecurityProfileProposal status update |
| `proposal.apply` | ApplyAttempt create |
| `rollback.execute` | RollbackAttempt create |
| `workload.restart` | Pod delete |
| `history.view` | History, receipt, apply-attempt, and rollback-attempt list |

Namespace selection creates an immutable session context containing namespace,
cluster identity, session ID, and context version. Later requests validate the
server-owned context rather than trusting browser state. Explicit namespace
selection can work without Namespace LIST permission, but only when a bounded
namespace capability is allowed.

Review and approval are independently enforced by server-side role groups:
`LANDLOCK_GENPROF_REVIEW_GROUPS` and
`LANDLOCK_GENPROF_APPROVER_GROUPS`. Production startup requires both lists.
Kubernetes status authorization remains a necessary lower-level check, but it
is no longer treated as sufficient proof that review and approval are the same
application authority.

### Sensitive endpoints

Governance review, approve, reject, apply, and rollback require authenticated
mode, an identity, the relevant capability, a namespace-pinned dynamic client,
an expected resource version, and (for approval) the expected candidate
digest. Apply and rollback create durable attempt records. Direct API calls do
not rely on frontend restrictions.

Observation stop has an explicit operation capability and validates the
observation target namespace and cluster identity. Observation start, status,
and generate-proposal now require `observation.operate`, `observation.view`,
and `proposal.generate` respectively. These checks occur before dispatch,
while the request-scoped impersonated Kubernetes client remains the
authoritative lower-level enforcement point.

### RBAC and cluster scope

The backend service account receives only startup namespace identity access,
SelfSubjectAccessReview creation, and explicitly configured impersonation
rights. Reusable team roles are intentionally unbound and are expected to be
bound by cluster administration/GitOps.

SPO `SeccompProfile` is cluster-scoped in the targeted API. Human/team
governance and rollback roles no longer include `seccompprofiles` permissions.
Authenticated Operations Center apply and rollback use a separately configured
profile-realizer kubeconfig. The service is allowed only the SPO profile API;
the application validates the approved digest, target namespace, deterministic
profile name, ownership annotations, and strict UID/resource-version custody
before mutation. A human/team identity cannot call that client.

## Threat model

| Scenario | Assessment | Evidence / limitation |
|---|---|---|
| Unauthenticated authenticated-mode request | PASS | Trusted-proxy identity and HMAC validation; governance rejects missing identity |
| Namespace substitution | PASS | Session and namespace pinning plus explicit observation capability gates reject mismatches |
| Cross-namespace object reference | PASS for governed namespaced objects | Namespace comes from the server-owned session and dynamic client; negative envtests exist but could not run here |
| Direct API bypass of frontend | PASS in authenticated mode | Sensitive handlers enforce server-side capabilities before dispatch |
| Review without approval | PASS | Signed identity must match configured review group; approval additionally requires approver group |
| Apply privilege escalation | PASS subject to RBAC deployment | Apply requires `proposal.apply`, expected resource version, approval state, digest, and realizer boundary |
| Stale approval after candidate change | PASS by implementation/tests | Approval checks expected digest and resource version; existing governance regression tests cover this contract |
| Cluster-scoped profile access | PASS subject to technical binding | Human/team roles have no profile permission; separate realizer is target/digest/ownership checked |
| Missing identity | PASS in authenticated mode | Authentication is required; local mode is intentionally trusted and single-user |
| Information leakage through namespace discovery | PASS | Forbidden Namespace LIST becomes explicit-only; no existence oracle is returned |
| Confused deputy | PASS subject to deployment | Backend impersonates request identity; observation executor and profile realizer are separate technical identities |
| Authorization/execution TOCTOU | PARTIAL | API server reauthorizes each call; proposal resource-version/digest CAS exists; capability discovery is advisory and may become stale |
| Cluster-scoped resources generally | PASS subject to deployment | Only the dedicated profile-realizer identity receives the required cluster-scoped profile role |

## Governance integration

The implementation preserves the existing governance contracts:

1. observations and recordings are namespace-pinned;
2. proposal review and approval operate on the selected namespace;
3. approval records the actor, expected digest, and resource version;
4. apply requires the approved candidate and creates an `ApplyAttempt`;
5. rollback creates a `RollbackAttempt` and uses the selected namespace;
6. generated, applied, and behaviorally verified remain separate states.

The code does not prove kernel enforcement merely from generated or applied
objects. Existing provenance and readiness checks remain in force.

The important unresolved governance issue is permission separation: the
application names review and approval separately, but Kubernetes exposes both
through the same status subresource permission. This is not fixed by the
frontend or by the capability cache.

## Existing test evidence

The repository contains targeted unit and envtest coverage for impersonation,
namespace-scoped SSAR checks, cross-namespace proposal status updates,
ApplyAttempt and RollbackAttempt creation, executor/backend separation,
explicit namespace access without Namespace LIST, session invalidation, and
frontend namespace-negative paths.

Current execution from this review:

| Command | Result |
|---|---|
| `go test ./internal/authz ./internal/applyproposal ./cmd/landlock-genprof` | PASS |
| `go test -race ./internal/authz ./internal/applyproposal ./cmd/landlock-genprof` | PASS |
| `go test ./...` | FAIL in generated `book/dist`/environment package: missing generated relative README path and sandbox localhost bind; all scoped product packages passed |
| `go test ./internal/environment` | BLOCKED: sandbox denied `httptest` localhost bind |
| Combined focused `go test` | FAIL only because of the environment package bind restriction; authz and command packages passed |
| Focused `go test -race` | Same environment bind restriction; authz and command packages passed |
| `go vet ./internal/authz ./internal/applyproposal ./cmd/landlock-genprof` | PASS |
| `make envtest` | PASS; proposal/history/attempt/observation/authz envtests and Workbench E2E envtest passed with setup-envtest assets |
| `helm lint deploy/helm/landlock-genprof` | PASS |
| `helm template` with authenticated Operations Center and profile-realizer configuration | PASS |
| `git diff --check` | PASS |
| Disposable real Kubernetes integration | NOT_TESTED; no safe disposable cluster was available |

New targeted tests cover review-only versus approval-only identities,
cross-namespace/forged profile names, and unauthorized observation start,
status, and proposal generation. `make envtest` provided real API-server
authorization evidence through envtest, but it is not a substitute for a
disposable Kubernetes distribution with the actual SPO and deployment RBAC.

## Findings and corrections

### P1 — approval/review capability conflation — RESOLVED

Kubernetes still exposes both transitions through the status subresource, so
that permission remains a necessary API-server check. It is no longer treated
as sufficient application authorization. Authenticated Operations Center
startup requires separate `LANDLOCK_GENPROF_REVIEW_GROUPS` and
`LANDLOCK_GENPROF_APPROVER_GROUPS`; each request is checked against the signed
identity groups before the handler dispatches. Tests prove reviewer-only and
approver-only identities are separated.

### P1 — cluster-scoped SPO profile boundary — RESOLVED SUBJECT TO DEPLOYMENT

Human/team governance and rollback ClusterRoles no longer include
`seccompprofiles`. Operations Center receives a separate profile-realizer
kubeconfig and the Helm chart renders an unbound
`landlock-genprof-profile-realizer` ClusterRole containing only the SPO
SeccompProfile API permissions. Cluster administrators must bind that role
only to the technical realizer identity.

The apply and rollback paths select that client only for the cluster-scoped
profile. Before mutation, the application verifies the approved candidate
through the existing digest/resource-version gates, then verifies target
namespace, deterministic profile name, ownership annotations, UID, and
resource-version custody. Forged namespace/profile references are rejected.
The application never converts a namespace-local human binding into a
cluster-wide binding.

### P2 — endpoint capability gaps — RESOLVED

Observation start, status, and proposal generation now enforce
`observation.operate`, `observation.view`, and `proposal.generate`
server-side before dispatch. Direct unauthorized endpoint tests return 403.

### P2 — deployment-mode documentation boundary

Local kubeconfig mode is a trusted development adapter. Production deployment
must fail closed without trusted-proxy and impersonation configuration. The
configuration checks exist, but the distinction should remain prominent in
installation and Operations Center documentation.

## Compatibility and release scope

This is a security feature, not patch-level maintenance. It should be treated
as a minor feature/security-capability release. No release metadata, tag, or
product release was changed by this review.

Required compatibility work before release:

1. bind the dedicated profile-realizer ClusterRole only to a technical service
   identity;
2. run envtest and a disposable real Kubernetes RBAC test;
3. verify Helm bindings and upgrade/uninstall behavior;
4. exercise stale approval, digest mismatch, and cross-namespace rollback in
   the disposable environment;
5. review the deployment and Operations Center documentation with operators.

## Recommended implementation phases

1. **Authorization design:** retain separate signed review/approval groups and
   never use a broad human ClusterRoleBinding.
2. **Controlled realization:** deploy the profile-realizer technical identity
   with only the rendered profile role and keep its kubeconfig separate.
3. **Verification:** run envtest plus a real disposable cluster with separate
   namespace identities and test all cross-namespace and stale-digest paths.
4. **Operational rollout:** document exact bindings, trusted-proxy
   requirements, executor/realizer identities, failure modes, and rollback.
5. **Release review:** reassess evidence and version scope after deployment
   validation.

## Final disposition

Status: **P1/P2 implementation complete; deployment-level validation remains
blocked in this environment.**

No P0 or unresolved P1 was found in the corrected code. A PR may be prepared
after the dedicated profile-realizer ClusterRole is reviewed by the cluster
administrator and envtest/real-cluster validation is executed. No merge, tag,
or release was created.

## PR #265 disposable kind and real-RBAC gate (2026-09-24)

### Environment recovery

The retained failed kind node logged the failure during systemd manager
initialization:

```text
Failed to create control group inotify object: Too many open files
Failed to allocate manager object: Too many open files
```

Measurements in the dedicated `landlock-genprof-core` Lima VM showed that the
global file table was not exhausted (`file-nr=6329`, `file-max` effectively
unbounded), `nr_open` was `2147483584`, and only three inotify descriptors
were present. Docker/containerd service limits were high (`524288` for Docker;
`2147483648` for the VM systemd service), and the kind node PID 1 had a
`2147483584` open-file limit after successful startup. The failure therefore
occurred in the kind node's early per-process systemd/inotify bootstrap, not
from global file-table, cgroup, or steady-state Docker exhaustion. No global
limit was changed.

A fresh cluster `landlock-genprof-pr265-secgate2` became Ready with kind
`v0.33.0`, Kubernetes `v1.36.4`, Docker server `29.8.0`, the pinned ARM64
node image, kernel `7.0.0-31-generic`, and healthy CoreDNS. SPO `v1.0.0` and
cert-manager `v1.17.2` were installed with the repository installer.

The published GHCR Operations Center `v0.8.1` image was unavailable, so the
PR's `Dockerfile.operations-center` was built locally and loaded only into
this disposable cluster. This was an environment/publication limitation, not
a substitute product implementation.

### Real-cluster authorization and governed apply

The chart was deployed with separate review, approval, governance, and
profile-realizer identities. The realizer ClusterRole grants only:

* `get` on the specifically named `kube-system` Namespace, required for the
  cluster-identity check; and
* `get/create/update/patch/delete` on cluster-scoped SPO `SeccompProfile`.

The bounded Namespace read was a confirmed deployment defect in PR #265 and
was corrected in the follow-up commit. Human/team bindings did not receive
SPO write permissions.

| Gate | Result | Evidence |
|---|---|---|
| reviewer signed review | PASS | HTTP 200; state became `Reviewed` |
| reviewer attempting approval | PASS denial | HTTP 403 `AUTHORIZATION_DENIED` |
| approver signed approval | PASS | HTTP 200; digest-bound `Approved` state |
| human direct SeccompProfile create | PASS denial | Kubernetes API returned `Forbidden` |
| realizer direct SeccompProfile create | PASS | Kubernetes API created the object |
| cross-namespace proposal list | PASS denial | service account from `pr265-b` denied in `pr265-a` |
| forged namespace header | PASS | request remained bound to `pr265-system` |
| resourceVersion conflict | PASS denial | HTTP 409 |
| PodLock/Seccomp composition guard | PASS denial | HTTP 412 before mutation |
| governed apply | PASS | signed HTTP 200, `SUCCEEDED` |
| ApplyAttempt custody | PASS | proposal UID, target, digest, mutation and `APPLIED` state recorded |
| SPO realization | PASS | cluster-scoped profile created, `Ready=True`, `status=Installed` |

The successful ApplyAttempt used proposal UID
`afa2cb4a-6188-499a-b1a3-0c8d83849dbe`, candidate digest
`sha256:8bc226a43b0140b33f05116e9f28ded15663e64f260d051f1f3e90444e3bb05f`,
and realizer-created profile
`lg-v1-nginx-demo-208d49920be2927e`. The profile's ownership annotations and
SPO readiness were recorded. This proves API-level materialization and SPO
reconciliation; it does not prove kernel-level seccomp enforcement.

The malformed ownership-negative fixture was corrected in
`cmd/landlock-genprof/namespace_authorization_envtest_test.go`. The new
`TestOwnershipMismatchThroughRealAPI` qualification uses the real envtest
kube-apiserver and etcd, real namespace-local RoleBindings, two separately
certified users, an API-created target Deployment, and an API-created
SecurityProfileProposal with server-assigned UID and resourceVersion. It
exercises the production governance HTTP handler and real SSAR capability
discovery:

| Corrected scenario | Result | Evidence |
|---|---|---|
| Authorized team-A reviewer reviews the valid proposal | PASS | HTTP 200; API state became `Reviewed` |
| Review-only identity attempts approval | PASS denial | HTTP 403; `proposal.approve` application capability absent |
| Approver bound only in team-B attempts team-A approval | PASS denial | HTTP 403 from real API-server SSAR path |
| Proposal state after denied approval | PASS | API state unchanged; no `ApprovedBy` or approved digest |
| Unauthorized ApplyAttempt creation | PASS | team-A ApplyAttempt list remained empty |

This is real Kubernetes API-server/RBAC evidence, but envtest is not a
node-bearing Kubernetes cluster and does not install SPO. It therefore does
not replace the separate disposable-cluster governed-apply/SPO qualification;
the last such qualification remains recorded above and must be rerun on the
rebased PR head before merge if the release gate requires same-SHA evidence.

### Regression and remaining limitations

The real-node SPO D-MIN CI check remains the evidence for host-level eBPF
recorder behavior; this kind cluster does not replace it. Kubernetes audit
sink output was not configured in the disposable API server, so durable
ApplyAttempt records and application responses were used instead. The local
full `go test ./...`/race commands still discover generated `book/dist`
packages with a relative-path test assumption; product packages, targeted
race tests, vet, envtest, Helm lint, and documentation checks pass.

The chart correction is ready for review. No master branch, tag, release, or
unrelated worktree file was modified.
