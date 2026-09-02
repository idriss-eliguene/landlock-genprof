# v0.5.0 founder-assisted pilot package

This is the operational entry point for a founder-assisted Kubernetes
least-privilege hardening pilot. It is not self-service onboarding, a generic
security audit, compliance certification, penetration testing, or proof that a
workload has the smallest possible authority.

The founder/operator remains involved. The released workflow is:

```text
preflight → environment capture → evidence acquisition/import
          → proposal → review → exact-digest approval
          → governed application → bounded verification
          → handoff → cleanup or recovery
```

The product lifecycle remains authoritative in [`docs/usage.md`](../usage.md);
this package defines the engagement and operating contract around it.

## v0.5.0 readiness boundary

This package is for a controlled, founder-assisted pilot of the already
certified G5 product evidence. It is not a universal workload or backend
compatibility statement. G5 evidence and claim custody are recorded in
[`docs/G5-CERTIFICATION.md`](../G5-CERTIFICATION.md).

Support status uses these meanings: `CERTIFIED` is backed by the cited
release evidence; `SUPPORTED_FOR_PILOT` is permitted only within the stated
scope and prerequisites; `EXPERIMENTAL` requires explicit operator approval;
`NOT_CERTIFIED`, `UNSUPPORTED`, `OUT_OF_SCOPE`, and `UNKNOWN` are stop states,
not implied support.

## Entry criteria

Record all of the following before starting:

- target cluster, environment, namespace, workload, owner, and customer point
  of contact;
- customer authorization for the named read and change scope;
- an authorized kubeconfig and the Kubernetes permissions needed for the
  selected operation;
- the supported operating boundary in [`INSTALL.md`](../../INSTALL.md),
  [`docs/enforcement-prerequisites.md`](../enforcement-prerequisites.md), and
  [`docs/PROGRESS.md`](../PROGRESS.md);
- required external components: Inspektor Gadget for tracing, a
  NetworkPolicy-capable CNI for network enforcement, SPO for SPO-derived
  SeccompProfile materialization, or PodLock for a PodLock activity;
- evidence collection and export authorization, storage location, retention
  period, and deletion owner;
- a recovery owner, change window, and pre-change state-capture plan;
- a staging or non-production preference when restart or workload binding is
  required.

Do not proceed when a prerequisite, authorization, target, or recovery owner
is ambiguous. Unsupported and unknown conditions remain explicitly recorded.

## Workload selection

Prefer an operationally understood, bounded, representative workload with an
available owner and a safe restart/redeploy path when verification requires
one. Controller-managed workloads are preferable when a replacement pod is
needed.

Do not use an initial pilot to claim compatibility with arbitrary workloads,
every CNI, same-Pod PodLock plus SPO non-interference, or behavioral
capabilities/securityContext enforcement. Current repository evidence records
these as bounded or unproven; review [`docs/PROGRESS.md`](../PROGRESS.md) and
the enforcement prerequisites before selecting a backend.

## v0.5.0 support matrix

| Workload or backend | Discovery / integration status | Pilot status | Boundary |
|---|---|---|---|
| Pod | Canonical target supported for regular containers | SUPPORTED_FOR_PILOT for discovery/read; NOT_SUPPORTED for v0.5.0 controlled-pilot apply | Container must be explicitly selected when needed |
| Deployment | Supported owner resolution and canonical target | SUPPORTED_FOR_PILOT | Apply changes `spec.template`; Kubernetes performs the native rollout |
| StatefulSet | Supported owner resolution and canonical target | SUPPORTED_FOR_PILOT | Apply/rollout behavior depends on `updateStrategy` |
| DaemonSet | Supported owner resolution and canonical target | SUPPORTED_FOR_PILOT | Apply/rollout behavior depends on `updateStrategy` |
| ReplicaSet | Resolved only as an intermediate owner; supported Deployment is the canonical target | NOT_CERTIFIED as a direct workload target | Do not treat ReplicaSet vocabulary as independent pilot coverage |
| Other or unresolved owner | Explicit unsupported/unresolved result | UNSUPPORTED | Stop; do not synthesize a target |
| Kubernetes core reads | Bounded `ReadSession` and Workbench read capability | CERTIFIED | One configured namespace; no browser mutation |
| NetworkPolicy | Object/selector reads and Cilium evidence | SUPPORTED_FOR_PILOT with Cilium | Behavioral claim is CILIUM-BOUNDED |
| SPO `SeccompProfile` | Provenance-bearing derived-policy import | SUPPORTED_FOR_PILOT | Tested SPO scope/version only; not direct observation |
| PodLock/Landlock | External backend integration and guarded application path | EXPERIMENTAL | Landlock kernel denial is NOT_PROVEN |
| PodLock + application-derived Seccomp in one Pod | Product guard refuses unproven composition | UNSUPPORTED | Compatibility is NOT_PROVEN; refusal is fail-closed |
| TrainingHistory | Optional direct-evidence history path | EXPERIMENTAL | Not a substitute for current runtime evidence |

The G5 release evidence used Ubuntu 24.04 GitHub runners. Core E2E used the
pinned kind node image `kindest/node:v1.34.0` with Cilium; SPO/PodLock evidence
used k3s `v1.33.6+k3s1` where stated by the workflow, with SPO v1.0.0 and
PodLock 0.1.1 in the applicable jobs. Exact node kernel, runtime, and CNI
observations are environment-capture fields for each pilot; values not
recorded by the evidence are `NOT_RECORDED`, not inferred or certified.

Known certification infrastructure findings are not product support claims:
the repository records bpftrace readiness-tail failures, k3s node-registration
races, and an SPO recorder preflight with zero associations as
`HARNESS_FLAKE`, `INFRASTRUCTURE_RACE`, or `UNKNOWN` according to the cited
run. Re-run or classify affected evidence before treating it as a pilot
result; do not silently convert an infrastructure failure into a product
failure.

## v0.5.0 claim matrix

| Claim | Pilot status | Boundary |
|---|---|---|
| Canonical workload identity and namespace pinning | CERTIFIED | Explicitly discovered workload in one configured namespace |
| Workbench read-only operation | CERTIFIED | Local HTTP surface; no browser Kubernetes mutation |
| Supported workload discovery | SUPPORTED_FOR_PILOT | Pod, Deployment, StatefulSet, DaemonSet only |
| Association and partial knowledge | CERTIFIED | Exclusions and `UNKNOWN` remain visible |
| SPO interoperability | SUPPORTED_FOR_PILOT | Derived policy with provenance, tested scope only |
| NetworkPolicy behavior | SUPPORTED_FOR_PILOT | CILIUM-BOUNDED |
| PodLock/Landlock kernel denial | NOT_CERTIFIED | Kernel denial is NOT_PROVEN |
| Same-Pod PodLock plus application-derived Seccomp | UNSUPPORTED | Compatibility NOT_PROVEN; refusal is fail-closed |
| Approval and application | CERTIFIED boundary | Approval is not application; apply is sequential and nontransactional |
| Behavioral verification | SUPPORTED_FOR_PILOT where applicable | Backend-specific evidence only; not universal enforcement |

## Pilot prerequisites and safety pre-flight

Mandatory prerequisites are an authorized kubeconfig, a named namespace and
workload owner, customer authorization and change window, the permissions
needed by the selected command, a recovery owner, pre-change state capture,
and the installed CLI. Run `kubectl landlock-genprof doctor` for host
prerequisites. Backend-specific prerequisites are mandatory only when that
backend is selected: Cilium for NetworkPolicy behavior, SPO and its CRDs for
SPO-derived SeccompProfile materialization, and a compatible PodLock
environment for PodLock activity. Do not assume the project's kind setup is a
PodLock-compatible environment.

Before proceeding, stop on an unsupported or unresolved workload, ambiguous
canonical identity, unavailable backend, permission/read error, unknown
evidence required for the decision, unrecognized SPO provenance/schema, the
same-Pod PodLock plus application-derived Seccomp composition, an unsupported
environment, or a missing recovery owner. `UNKNOWN` is never approval.

Bare Pods remain supported for discovery, read, and Workbench review, but are
excluded from v0.5.0 controlled-pilot apply because apply uses a delete-then-
create replacement path: if deletion succeeds and recreation fails, the
workload can remain absent. v0.5.0 has no durable ApplyAttempt journal or
automatic rollback for that failure mode. For Deployments, StatefulSets, and
DaemonSets, Kubernetes owns any rollout caused by a changed Pod template.
landlock-genprof does not persist Deployment revisions, ReplicaSet identity,
or `pod-template-hash` as Apply identity. `kubectl rollout undo` is therefore
not a landlock-genprof-aware rollback mechanism.

The browser Workbench is read-only and one-namespace-centric. Review and
selection happen there; `approve` and `apply-proposal` remain explicit CLI
operator mutations. No browser action, polling, remote Workbench service, or
automatic remediation is part of this pilot.

The Workbench application capability is read-only, but the current Workbench
is a local CLI process and uses the invoking operator's Kubernetes credential.
The repository has no deployed Workbench Pod/Deployment to which a dedicated
reader ServiceAccount could be bound. Consequently v0.5.0 does not claim
credential-level separation for the Workbench; the existing tracer
ServiceAccount is not a Workbench reader identity. Creating such a deployed
reader would be a separate process-architecture change.

## Pilot success and abort criteria

A pilot assessment succeeds only when the target resolves canonically, the
bounded Workbench loads, relevant projection states are interpretable,
association is unambiguous, evidence and proposal provenance are captured,
the operator records the exact digest and decisions, and a recovery owner and
evidence handoff are identified. Application and behavioral verification are
separate outcomes and must be recorded only within their backend scope.

Abort immediately for canonical or association ambiguity, required read
errors, unsupported backend/environment, unexpected authority or mutation,
partial application, failed readiness, unexpected behavior, a triggered
composition guard, or lost evidence custody. Preserve logs and live state;
stop further mutation; classify the outcome as product failure, unsupported
environment, backend unavailable, insufficient evidence, operator abort,
harness failure, or infrastructure failure. Do not relabel a harness failure
as product failure or use `UNKNOWN` as success.

## Environment capture

Store this capture with the pilot evidence:

| Fact | Actual capture or source |
|---|---|
| CLI revision | `landlock-genprof version`; record tag or build revision |
| Kubernetes version | `kubectl version` |
| Nodes, architecture, and kernel | `kubectl get nodes -o wide` plus the authorized node procedure |
| Workload identity | `kubectl get pod <pod> -n <namespace> -o yaml` |
| Container and binary | Record the selected `trace` container and binary path |
| Existing controls | Record workload `securityContext`, seccomp reference, capabilities, labels, and relevant NetworkPolicy |
| CNI/operators | Record CNI, Inspektor Gadget, SPO, and PodLock versions/configuration when used |
| Host prerequisites | `kubectl landlock-genprof doctor` and [`INSTALL.md`](../../INSTALL.md) |
| Source provenance | Internal acquisition, or named SPO recording/profile and lineage |

Do not infer certification from a successful command or one environment.

## Permissions and responsibilities

Install only the manifests needed from [`INSTALL.md`](../../INSTALL.md):

- `deploy/rbac.yaml` provides the tracer identity, target-pod read, and
  Inspektor Gadget access;
- `deploy/rbac-proposal.yaml` is required for proposal publication;
- `deploy/rbac-history.yaml` is opt-in for `trace --history`;
- `deploy/rbac-restart.yaml` is opt-in and grants disruptive restart/patch
  permissions;
- `deploy/rbac-patched-manifest.yaml` provides read access needed to compose
  a patched manifest.

Review, approval, and apply use the invoking user's Kubernetes permissions.
They are not silently authorized by the tracer identity.

Customer responsibilities:

- authorize access, workload scope, and changes;
- provide workload context and decide whether behavior is legitimate;
- own the recovery decision and provide an available workload owner;
- approve evidence transfer, retention, and deletion.

Founder/operator responsibilities:

- validate environment and permissions;
- preserve evidence provenance and claim boundaries;
- generate and explain proposals;
- obtain exact-candidate approval before application;
- record application and verification separately;
- stop on fail-closed conditions and execute the agreed recovery procedure;
- hand off evidence and unresolved limitations.

## Execution sequence

Use only the actual CLI documented in [`docs/usage.md`](../usage.md):

1. Run `kubectl landlock-genprof doctor` and complete the entry criteria.
2. Capture the environment and retain original controller/workload manifests.
3. Acquire evidence with `trace --pod <pod> --namespace <ns> --binary <path>`.
   Select `--seccomp-source=spo` only with a named SPO recording and profile;
   otherwise use the documented internal/advisory source.
4. Retain the published `SecurityProfileProposal`, candidate digest, source
   evidence, provenance, and generated artifacts.
5. Review with `kubectl landlock-genprof review <proposal> --namespace <ns>`.
6. Approve the reviewed digest with `approve <proposal> --namespace <ns>
   --expected-digest sha256:<digest> --reason <reason>`.
7. Apply only with `apply-proposal <proposal> --namespace <ns>`. Use
   `--restart` only after accepting its disruptive semantics and extra RBAC.
8. Record backend-specific verification. `verify --candidate-file <path>` is
   an ABI check; it is not by itself behavioral enforcement verification.

`evidence show`, `evidence list`, `explain`, `diff`, `export`, `policy list`,
and `policy status` are inspection aids where their documented inputs apply.
Directly applying an exported artifact bypasses proposal authority and is not
equivalent to exact-digest approval plus governed apply.

## Authority and application boundaries

Generation and proposal publication do not authorize application. Approval is
bound to the exact candidate digest. A changed or stale proposal must be
reviewed and approved again; stale authority fails closed.

Application is sequential, not transactional. It stops on failure; resources
already applied remain, and there is no automatic rollback. Read
[`ADR-0007`](../adr/0007-governed-apply-ordering-and-enforcement-readiness.md)
for ordering and readiness details.

SPO `SeccompProfile` input is derived policy with provenance, not direct
landlock-genprof syscall evidence. Coverage is not confidence or
authorization. Observed behavior is not automatically legitimate, and not
observed is not unnecessary.

## Stop conditions

Stop before continuing when:

- the environment or backend is outside the reviewed boundary;
- target, authorization, evidence, provenance, recovery prerequisites, or
  permissions are missing;
- proposal identity, digest, or approval is absent, stale, or mismatched;
- backend readiness or an application step fails;
- an intermediate artifact may have applied and state is unknown;
- verification is outside the tested/certified scope or returns unknown;
- the customer revokes authorization or the change window ends.

`STOP` means no further mutation or interpretation. `FAILURE` records the
failed operation and observed state. `NOT VERIFIED` records an unknown or
unsupported verification result; it is not converted into success or failure.

## Cleanup and evidence handoff

Cleanup is manual and evidence-aware. For an assessment-only pilot, retain the
agreed evidence and remove only temporary working copies on the agreed
schedule. For an applied pilot, first determine whether a resource predated
the pilot or is still referenced. Remove only customer-authorized,
pilot-created resources with exact identity; never perform broad namespace
cleanup or delete an unowned resource.

The minimum handoff contains, where applicable:

- environment capture and workload identity;
- source/evidence inventory and provenance;
- proposal identity and exact candidate digest;
- generated artifacts and intended backend;
- approval state and reason;
- application ordering, outcome, and partial-application state;
- backend verification result and its tested/certified boundary;
- limitations, unknowns, unresolved items, and any recovery actions.

The [Hardening Review template](hardening-review-template.md) defines polished
customer presentation. This package defines the operational contents only.

## Pilot acceptance

Assessment success: agreed workloads were assessed; evidence, provenance, and
candidate changes were captured and reviewed; and the evidence package was
handed off with limitations explicit.

Application success is separate: the exact candidate was approved and governed
application completed, or stopped with state recorded.

Behavioral verification success is separate again: applicable backend
verification ran within its tested boundary and its result was recorded.
None of these states proves universal compatibility or minimal possible
authority.

See [`recovery.md`](recovery.md) and [`data-handling.md`](data-handling.md).

## G6 release decision

No fresh customer pilot execution is required to establish this readiness
package: the exact-SHA G5 real-node evidence supplies the technical evidence,
while this document supplies the bounded operational contract. A real pilot
remains a controlled future operation subject to the prerequisites and stop
conditions above; it is not retroactively claimed here.

G6 pilot readiness is limited to the support matrix, prerequisites, workflow,
abort/recovery, evidence-handling, and claim boundaries in this package.
v0.5.0 remains release-certified only when the repository's final G6 decision
records this package as complete without widening any G5 claim.
