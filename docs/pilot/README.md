# Founder-assisted pilot package

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

The future Hardening Review template defines polished presentation. This
package defines the operational contents only.

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
