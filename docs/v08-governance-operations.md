# v0.8 Governance Operations boundary

This document is the release-candidate boundary for v0.8. It describes the
implemented read projection; it does not authorize publication or change the
underlying Kubernetes objects.

## Product position

landlock-genprof is **Evidence-driven Least-Privilege Governance for
Kubernetes**. v0.8 presents Governance Operations: a read and explanation
surface over already-durable observations, populations, proposals, and
application custody.

The operational question is:

> Across the workloads and populations already observed and governed, which
> have a reconciliation gap between durable evidence and durable governance
> state, and which named gap is it?

The projection is not a generic SOC, SIEM, CNAPP, KSPM, threat-detection,
vulnerability, compliance, or incident-response product.

## Identity and authority

An Environment subject reuses `PopulationIdentity`:

```text
Scope, Target, Container, ImageIdentity, BinaryPath
```

BINARY and CONTAINER populations remain distinct. Workload UID is not part of
PopulationIdentity or the candidate-v2 Proposal subject. Candidate-v2 policy
authority is Proposal-object-scoped:

* zero valid approved candidates: `NONE`;
* one valid candidate: `SELECTED`;
* more than one: `AMBIGUOUS`.

Validity requires candidate-v2 compatibility, exact subject matching,
`Approved` status, and `ValidateApprovedCandidate`. Equal candidate digests do
not transfer authority between Proposal objects.

## Five independent axes

The read model keeps these axes independent:

1. `DERIVATION`
2. `GOVERNANCE`
3. `APPLICATION`
4. `STRUCTURAL_ENFORCEMENT_KNOWLEDGE`
5. `BEHAVIORAL_VERIFICATION`

Behavioral verification is always `UNKNOWN` in v0.8. Positive structural
knowledge is limited to: “Kubernetes object confirmed to match the intended
spec immediately after application.” It does not establish current, kernel,
or behavioral enforcement.

## Attention taxonomy

Attention is a derived read-model fact with no persistence, severity, or
acknowledgment:

* `CAPABILITY_OUTSIDE_APPROVED_POLICY`
* `APPROVED_NOT_APPLIED`
* `APPLICATION_STATE_UNKNOWN`
* `NEW_CONTRIBUTION_SINCE_CANDIDATE`
* `OBSERVATION_FAILED_BOUND`
* `OBSERVATION_FAILED_UNBOUND`
* `MULTIPLE_VALID_APPROVED_PROPOSALS`

`NEW_CONTRIBUTION_SINCE_CANDIDATE` means a durable Observation contribution is
absent from the latest compatible candidate-v2 provenance snapshot. It does
not mean new behavior, new capability, or drift. Capability attention is
computed independently from accumulated positive capability facts.

## Environment, History, and UID ambiguity

Environment is a bounded, deterministic projection over bulk-loaded durable
objects. A subject may be prospective when an exact bound Observation exists
without a TrainingHistory Population. Population absence is not evidence
absence.

History separates timestamped events from untimestamped or unordered custody
facts. Approval transition history and contribution chronology are not fully
reconstructable from the durable schema. Dangling references remain visible.

`ProposalSubjectMatchedMultipleWorkloadUIDs` is a boolean positive-only
signal. True means at least two distinct matching workload UIDs are proven by
the supplied durable Observations. False means multiplicity is not proven; it
does not prove uniqueness or absence of recreation.

## Read and browser boundary

The v0.8 HTTP surface is read-only and namespace-pinned:

* `/api/v08/environment`
* `/api/v08/environment/detail`
* `/api/v08/history`
* `/api/v08/history/proposal`

Reads are bounded bulk reads assembled from multiple Kubernetes objects. They
are best-effort and not transactional snapshots. The browser may start/stop
Observations, inspect status, and generate proposals through the existing
supported paths, but it cannot approve, reject, revoke, apply, rollback,
acknowledge, or dismiss governance state.

## Claim matrix

| Claim | Classification |
|---|---|
| Environment projection | EMPIRICALLY_QUALIFIED read projection |
| Attention predicates | STRUCTURALLY_PROVEN and unit-qualified |
| Approved-policy ambiguity | STRUCTURALLY_PROVEN and envtest-qualified |
| Application custody | EMPIRICALLY_QUALIFIED durable attempt projection |
| Structural application-time confirmation | STRUCTURALLY_PROVEN, bounded read-back claim |
| Behavioral verification | NOT_PROVEN; always UNKNOWN |
| Current enforcement | NOT_PROVEN / OUT_OF_SCOPE |
| Complete observation coverage | NOT_PROVEN |
| Complete approval history | NOT_RECONSTRUCTABLE_BY_SCHEMA |
| Contribution chronology | NOT_RECONSTRUCTABLE_BY_SCHEMA |
| UID multiplicity disclosure | EMPIRICALLY_QUALIFIED positive-only signal |
| Transactional multi-object consistency | NOT_PROVEN; explicitly disclaimed |
| Multicluster governance | OUT_OF_SCOPE |
| Continuous monitoring or drift detection | OUT_OF_SCOPE |

## Release-candidate state

```yaml
candidateVersion: 0.8.0
state: RELEASE_CANDIDATE
g9QualifiedSHA: d9c3be6dc73cebf2330ab28472173de4be00c76f
candidateV2Digest: sha256:46062013486c3c47ba3d092d002fa12eb86eeb2019eafcd8b1e51805a9e32609
reviewContextV2Digest: sha256:559d6234e02a96a88bb3911cd2483b124bd9fd434ce6847ab76ca056cf3d662
realAPIServerBasis: envtest Kubernetes 1.36.2
kindQualification: NOT_QUALIFIED_IN_G9
localGosec: NOT_RUN_IN_G9
released: false
published: false
tagVerified: false
```

The final G10 commit SHA and tree are recorded in the final custody report;
this candidate record deliberately does not claim release, publication, or
tag verification.

## G8 closure and residual UX debt

G8 is closed and certified. The qualification progression is retained as
historical evidence:

```text
G8-Q1 FAIL  -> G8-R1 PASS  (same-origin CSP connection policy)
G8-Q1-R2 FAIL -> G8-R2 PASS (navigation and authoritative status composition)
G8-Q1-R3 FAIL -> G8-R3 PASS (responsive Attention diagnostics)
G8-Q1-R4 PASS_WITH_MINOR_VISUAL_ISSUES -> G8 CLOSED
```

Final G8 acceptance is `PASS_WITH_MINOR_VISUAL_ISSUES`. Real Chrome
qualification established zero blocker defects, zero high defects, all six
canonical surfaces reachable, truthful Platform/Projection state, and a
readable malformed Observation diagnostic at 1440x900, 1280x800, and
1024x768. No G8-Q1-R5 is required.

### G8-Q1-R4-001

* **Classification:** `MEDIUM`, `NON_BLOCKING`, UX / presentation-state
  lifecycle.
* **Observation:** after History renders its truthful unavailable state, the
  global error banner remains visible when navigating to Attention.
* **Impact:** the stale banner can make the operator associate the previous
  History read failure with the current Attention surface.
* **Boundary:** the defect does not change backend state, authentication,
  authorization, RBAC, authority, Platform or Projection state, malformed
  classification, `BINDING_INVALID`, `NOT_ELIGIBLE`, governance eligibility,
  custody, or Kubernetes objects. The Attention diagnostic remains readable
  and authoritative.
* **Likely layer:** presentation-state lifecycle; the precise cause remains a
  future implementation investigation rather than a certified root-cause
  claim.
* **Desired behavior:** a view-scoped error/unavailable message should stop
  presenting as current after navigation away, unless it represents a truly
  global persistent condition.
* **Disposition:** `DEFERRED`, `NON_BLOCKING_FOR_G8`, tracked for post-G8 UX
  maintenance.
* **Remediation boundary:** presentation state only. Future work must not
  change backend APIs/read models, authentication, authorization, RBAC,
  impersonation, G6 operational semantics, projection, governance, custody,
  or malformed-object semantics.
* **Evidence:** G8-Q1-R4 real Chrome visual closure qualification.

## G9-A0 production-readiness architecture audit

G1-G8 certify the semantic, security, operational-context, workflow, and
visual boundaries. They do not certify a production deployment. This audit
records the remaining deployment and operability boundary without changing
those certified semantics.

### Current deployment model

The project retains its CLI/kubectl-plugin workflows, and the Operations
Center remains optional. When enabled, the Helm chart now creates the D1
production image-backed Deployment and ClusterIP Service, the D2
component-scoped NetworkPolicy, and the D3 lifecycle probes/shutdown contract.
At the time of this historical G9-A0 snapshot, production observability was
still a later gate; G9-O1 now records and qualifies the implemented contract
below.

Implemented deployment material is limited to:

* project CRDs under `deploy/` and Helm `crds/`;
* Operations Center backend ServiceAccount, SSAR permission, and exact
  impersonation allowlist plumbing in
  `deploy/rbac-operations-center.yaml` and
  `templates/rbac-operations-center.yaml`;
* reusable namespace-scoped team capability ClusterRoles and explicit
  RoleBinding inputs in `templates/rbac-operations-center-team.yaml` and
  `templates/rbac-operations-center-bindings.yaml`;
* a distinct executor ServiceAccount and namespace/Gadget Roles in
  `deploy/rbac-observation-executor.yaml` and its Helm template.

The executor now has a separate `executor` process in the production image
and an opt-in Helm Deployment. It consumes only durable `REQUESTED`
Observations, claims them with the existing resourceVersion CAS and executor
lease, and runs the source-specific Runner under the executor ServiceAccount.
The Deployment is fixed at one replica with `Recreate`; it has no Service and
no governance RBAC. The executor NetworkPolicy permits no ingress and only
configured Kubernetes API and cluster-DNS egress. Gadget access is through
the Kubernetes API port-forward subresource, not through Operations Center
authority.

The Runner renews the durable lease during long observations. On orderly
shutdown it cancels collection and persists a bounded terminal result when
the Kubernetes API remains available. On abrupt loss, the next executor
scan terminalizes an expired non-terminal claim as `FAILED/EXECUTOR_LOST`.
This is recoverable truthful state, not exactly-once execution or HA.

### Trusted proxy and authentication boundary

`internal/authn/identity.go` implements the signed external identity
assertion: one user, groups, proxy marker, timestamp, and HMAC-SHA256
signature. `cmd/landlock-genprof/workbench_authorization.go` verifies the
allowlist, constructs a fresh request-scoped impersonated client, and checks
base/executor cluster identity. `internal/authz/authorization.go` rejects
pre-existing impersonation and request UID/Extra authority.

The authenticating proxy/identity provider is external and has no repository
Deployment or Helm artifact. The HMAC secret is read from
`LANDLOCK_GENPROF_TRUSTED_PROXY_HMAC_SECRET`; no Kubernetes Secret manifest,
rotation procedure, dual-key rotation, or external proxy TLS/trust
configuration is defined. The five-minute timestamp window is enforced, but
there is no process-local or shared nonce/replay store, so an identical valid
assertion can be reused within that window. Clock-skew policy beyond the
window is not operationally documented.

The backend defaults to legacy local mode when the HMAC secret is absent;
production must fail closed by deployment policy and must not expose that mode
behind a network Service. Because no production network graph or proxy
deployment exists, direct backend bypass prevention is not established.

### Stateful and multi-replica behavior

The authenticated read/governance path is largely request-scoped and uses
Kubernetes CAS/authorization, but `cmd/landlock-genprof/observation_api.go`
stores active Observation cancellation handles in an in-process `stop` map.
Observation start/stop behavior therefore diverges across replicas and is
lost on process restart. There is no shared session, replay, cache, or rate
limit state. A single replica is the safe v0.8 deployment constraint until
Observation control ownership is externalized or explicitly routed.

### Kubernetes security posture

RBAC separation is materially defined: the Operations Center backend receives
SSAR and explicitly allowlisted human impersonation, while team access is
namespace-local and the executor has separate Gadget/target permissions.
The Operations Center D2 baseline is explicit: non-root execution, no
privilege escalation, all capabilities dropped, RuntimeDefault seccomp,
read-only root filesystem, no host namespaces/hostPath/hostPort, and an
explicit ServiceAccount token because in-cluster request-scoped Kubernetes
clients are required. The executor kubeconfig remains a separate read-only
Secret mount. The executor retains a separate authority and network role;
its complete network hardening remains a follow-up rather than being merged
into the backend policy.

### Executor configuration and lifecycle

Enable the executor with `observationExecutor.enabled=true` and an explicit
`observationExecutor.targetNamespaces` list. The image uses the same minimal
distroless binary as the Operations Center but starts the dedicated
`executor` command; the shared binary does not share the Operations Center
ServiceAccount or authority. The executor reads its in-cluster ServiceAccount
token and does not consume the Operations Center executor-kubeconfig Secret.

The target namespace Role grants only Observation list/get/status update,
target Pod/ReplicaSet reads, while the Gadget Role grants Gadget Pod listing
and `pods/portforward` creation. Proposal, ApplyAttempt, RollbackAttempt,
impersonation, and workload mutation permissions are absent. Initial CPU and
memory values are bounded operational defaults and require field tuning.

The process has no HTTP server; its lifecycle is supervised by Kubernetes
process termination and the durable claim lease. Shutdown has a five-second
worker budget inside the ten-second Pod termination grace. There is no claim
of zero downtime, multi-executor HA, or durable execution continuation
through a process crash. A crashed executor leaves a non-terminal claim only
until its 30-second lease expires; the next executor scan marks it
`EXECUTOR_LOST`.

### Lifecycle, health, and observability (G9-A0 historical snapshot)

The HTTP server has request/read/write/idle/header bounds and a five-second
graceful shutdown in `runWorkbench`, but no Kubernetes startup/readiness/
liveness probes, termination policy, readiness state, signal-supervised
Deployment, or in-flight governance shutdown policy is deployed. G6's
Platform/Projection model is an operator read model and must not be reused as
a pod liveness probe; Projection `DEGRADED` must remain service-live.

At the time of G9-A0, operational telemetry was minimal: panic and response-limit messages used the
standard logger in `workbench_server.go`. There is no structured request
correlation, metrics endpoint, latency/error metrics, actor/operation
correlation, redaction policy, or executor telemetry contract. Durable
ApplyAttempt/RollbackAttempt actor evidence remains the custody source; logs
must not replace it.

### Upgrade, supply chain, and lifecycle

Helm deliberately preserves CRDs on upgrade/uninstall and documents manual
CRD application, which protects evidence from casual uninstall. A formal
schema compatibility, conversion, upgrade rehearsal, version-skew, and
rollback procedure is not defined. The release workflow publishes
cross-platform CLI binaries and a Helm chart, but the repository contains no
production Operations Center image. `.goreleaser.yaml` provides builds and
checksums, while SBOM, provenance, signing, image scanning, and a deployable
server artifact are not defined.

### Production gap register

| ID | Area | Current state | Required state | Severity | Recommended gate |
|---|---|---|---|---|---|
| G9-001 | Operations Center deployment | D1 Deployment/ClusterIP artifact exists | Versioned, production-operated service boundary | P0 | G9-D3/G9-R1 |
| G9-002 | Trusted proxy | External contract only | Deployed TLS/proxy boundary with header stripping and backend isolation | P0 | G9-A1 |
| G9-003 | Auth secret | Env-only, no Secret/rotation | Secret-managed provisioning and rotation | P0 | G9-A1 |
| G9-004 | Direct backend exposure | ClusterIP plus D2 proxy-selector policy | Trusted proxy plus network-enforced backend boundary | P0 | G9-R1 |
| G9-005 | Process security | D2 Operations Center pod hardening defined | Full release policy and runtime qualification | P1 | G9-R1 |
| G9-006 | Executor supervision | Dedicated worker, durable lease/recovery, bounded resources and Recreate workload | Field recovery rehearsal, telemetry and version-skew qualification | P1 | G9-O1/G9-U1 |
| G9-007 | Single-replica state | Executor and Operations Center fixed at one replica; local cancellation remains non-durable | Shared control ownership or HA qualification | P1 | G9-U1 |
| G9-008 | Probes/lifecycle | D3 startup/readiness/liveness and bounded drain implemented and qualified | Production telemetry and broader recovery rehearsal | P2 | G9-O1/G9-U1 |
| G9-009 | Network isolation | Backend and executor policies bound proxy/API/DNS paths | Runtime qualification and broader egress rehearsal | P1 | G9-R1 |
| G9-010 | Observability | Standard logs only | Structured telemetry, correlation, redaction, metrics and alerts | P1 | G9-O1 |
| G9-011 | Upgrade/recovery | CRD preservation documented; rehearsal absent | Upgrade/rollback/version-skew qualification | P1 | G9-U1 |
| G9-012 | Release artifact | CLI release only; no server image | Reproducible signed deployable server artifact | P1 | G9-R1 |
| G9-013 | UX banner lifecycle | G8-Q1-R4-001 deferred | View-scoped presentation state | P2 | Post-G8 UX maintenance |
| G9-014 | Rate limiting | No request rate-limit policy | Bounded ingress/backend abuse control | P2 | G9-A1 |
| G9-015 | Secret replay | Timestamp freshness only | Explicit replay/clock-skew operational policy | P2 | G9-A1 |

### Minimum proposed G9 sequence

1. **G9-A1 — Trusted boundary and configuration:** external proxy/TLS,
   Secret provisioning/rotation, fail-closed production configuration, header
   stripping, clock/replay policy. Excludes new auth semantics.
2. **G9-D1 — Deployable Operations Center:** immutable server image,
   Deployment/Service, namespace/service-account wiring, direct-exposure
   prevention, resource and rollout policy. Excludes semantic core changes.
3. **G9-D2 — Pod and network hardening:** security contexts, token/filesystem
   policy, NetworkPolicy, minimal egress, and executor isolation.
4. **G9-D3 — Lifecycle and health:** probes, graceful drain, startup ordering,
   dependency degradation, restart behavior, and single-replica constraint
   enforcement.
5. **G9-E1 — Executor operability:** executor process artifact, claim/recovery
   supervision, restart/crash behavior, resource limits, health, and
   version-skew qualification.
6. **G9-O1 — Production observability:** structured logs, correlation,
   redaction, metrics, alertable failure classes, and custody/telemetry
   separation.
7. **G9-U1 — Upgrade and recovery qualification:** CRD compatibility,
   evidence preservation, Helm upgrade/uninstall behavior, rollback, and
   Operations Center/executor skew.
8. **G9-R1 — Release qualification:** signed/reproducible server artifact,
   SBOM/provenance/scanning, staged deployment smoke, restart, proxy failure,
   and network-isolation evidence.

### Release boundary

The current G1-G8 system plus G9-D1/D2/D3 artifacts is **INTEGRATION_READY**, not
STAGING_READY or PRODUCTION_READY. Semantic/security qualification and
real-browser evidence are strong, and the deployable backend now has a
non-root/read-only pod posture and component-scoped network policy. Lifecycle
probes and bounded shutdown are implemented and qualified. Executor
supervision, production telemetry, release-image controls, and
upgrade/recovery qualification remain open.

This audit found no evidence invalidating G1 core boundaries, G2 identity and
tenant isolation, G3 executor separation, G4 malformed containment, G5
governance semantics, G6 operational truth, G7 workflow architecture, or G8
visual certification.

## G9-A1 production trust boundary and configuration contract

G9-A1 establishes the configuration contract for a future production
Deployment. It does not create that Deployment or change the certified
authentication semantics.

### Authentication and replay

The accepted headers are `X-Operations-Center-User`,
`X-Operations-Center-Groups`, `X-Operations-Center-Proxy`,
`X-Operations-Center-Timestamp`, and `X-Operations-Center-Signature`.
The canonical HMAC-SHA256 payload is:

```text
timestamp.RFC3339Nano in UTC
username
sorted groups, one per line
```

The secret must contain at least 32 bytes. Assertions are accepted only when
the timestamp is within five minutes of the verifier clock, the proxy marker
is `true`, the identity is allowlisted, and the signature matches. Future
timestamps outside the same window are rejected. Duplicate assertion headers,
system identities, service-account identities, UID/Extra impersonation, and
unallowlisted identities fail closed.

There is no replay cache. The replay key is therefore not defined, replay
state is `NONE`, and the five-minute freshness window is the only replay
boundary. An identical valid assertion can be accepted by the same verifier,
an independent process, or a restarted process while it remains within the
window. This is unchanged certified behavior and is not a claim of nonce-based
replay prevention.

### Explicit deployment mode

`LANDLOCK_GENPROF_DEPLOYMENT_MODE` accepts `production`, `local`, or
`development`; an empty value remains local-compatible. In `production` mode
startup validation requires:

* `LANDLOCK_GENPROF_TRUSTED_PROXY_HMAC_SECRET`, at least 32 bytes;
* a non-empty valid `LANDLOCK_GENPROF_ALLOWED_USERS` or
  `LANDLOCK_GENPROF_ALLOWED_GROUPS` allowlist;
* a namespace;
* `LANDLOCK_GENPROF_OBSERVATION_EXECUTOR_KUBECONFIG`.

Invalid or unsupported mode values fail closed. Production mode cannot
silently enter the legacy local path. Local/development mode remains available
for `make ui-lima`, CLI, and envtest workflows.

### Production trust contract

The future trusted proxy must authenticate the human, strip all inbound
client-supplied Operations Center identity headers, derive the configured
username/groups, inject a fresh signed assertion, and forward only through the
intended TLS boundary. The backend must not be exposed through a NodePort,
LoadBalancer, direct Ingress, or any other browser-reachable path that bypasses
that proxy. G9-D1/D2 must enforce a proxy-only ClusterIP path and corresponding
NetworkPolicy.

The HMAC secret is a Kubernetes Secret input to the future Deployment. It must
not be stored in Git, ConfigMap data, command arguments, logs, or browser
responses. The current verifier supports one active key only, so v0.8 rotation
requires a controlled restart; dual-key or zero-downtime rotation is not
claimed. Secret rotation must be paired with proxy/backend coordination.

### Replica contract

The future v0.8 Deployment must use exactly one Operations Center replica.
`cmd/landlock-genprof/observation_api.go` retains active Observation
cancellation handles in process-local state; that state is lost on restart and
cannot be safely shared by concurrent replicas. A restart is not equivalent to
replica concurrency and requires explicit operational recovery handling.

## G9-D1 deployable Operations Center contract

The chart now contains an optional Operations Center Deployment and Service.
It is enabled with `operationsCenter.enabled=true` and remains disabled by
default. The Deployment:

* runs the production Operations Center image for Linux ARM64;
* fixes `LANDLOCK_GENPROF_DEPLOYMENT_MODE=production`;
* binds the server to the Pod network interface;
* uses the existing Operations Center ServiceAccount/RBAC;
* reads HMAC material from a pre-created Secret reference;
* reads the required executor kubeconfig from a pre-created Secret reference;
* fixes replicas to `1` and uses `Recreate` to prevent unsupported overlap;
* applies a component-scoped NetworkPolicy when enabled, allowing only the
  configured trusted-proxy selector to the application port and only the
  configured Kubernetes API CIDRs plus kube-system DNS for egress.

The Service is `ClusterIP` only. No NodePort, LoadBalancer, or direct backend
Ingress is rendered. The trusted proxy remains external and must target the
ClusterIP Service after stripping client identity headers and injecting the
signed assertion. ClusterIP alone is not the security boundary; the
component-scoped NetworkPolicy requires deliberate proxy selectors and API
CIDRs and fails rendering when those are missing. Network-level enforcement
does not replace application signature validation.

The executor kubeconfig is a temporary v0.8 compatibility model. It is
mounted read-only from Secret material because the current application
startup contract requires a kubeconfig path. The executor identity receives
only the existing Gadget/target permissions plus the bounded `get` of the
`kube-system` Namespace needed for cluster identity comparison. The backend
and executor cluster-identity reads are limited to that named Namespace.

The production image is built by `Dockerfile.operations-center` from a Go
builder into a distroless static runtime. It has no source tree or development
toolchain at runtime. Image publication/tag promotion remains a later G9
release concern.

## G9-D2 pod and network hardening contract

The Operations Center Deployment uses a component-scoped `NetworkPolicy` when
enabled. Its ingress rule requires both an explicit trusted-proxy namespace
selector and pod selector and permits only the Operations Center HTTP port.
Its egress rules require explicit Kubernetes API CIDRs and ports and permit
only TCP to those CIDRs plus UDP/TCP 53 to the configured kube-system DNS
selector. The default API ports cover the in-cluster Service port 443 and the
common control-plane endpoint port 6443 after Service translation.
On Cilium clusters the chart additionally renders a component-scoped
`CiliumNetworkPolicy` with only the `kube-apiserver` entity on those ports;
this is necessary for host-networked control-plane endpoints and does not
grant world or arbitrary external egress.
The chart rejects missing selectors/CIDRs and rejects all-address egress. It
does not create a namespace-wide default deny policy, so unrelated workloads
are not changed by this component contract.

The backend pod runs non-root with privilege escalation disabled, all Linux
capabilities dropped, RuntimeDefault seccomp, and a read-only root
filesystem. Host networking, host PID/IPC, hostPath, and hostPort are
explicitly disabled or absent. The ServiceAccount token remains mounted
because `k8s.RestConfig()` and request-scoped Kubernetes clients require
in-cluster API access. The executor kubeconfig remains a separate read-only
Secret mount and no executor Gadget egress is added to the backend policy.

The trusted proxy remains external to this chart. ClusterIP alone is not a
security boundary: NetworkPolicy enforcement and signed application
authentication together define the v0.8 backend access boundary. The
Operations Center policy is qualified separately from the executor policy;
complete executor network hardening is deferred to G9-E1.

## G9-D3 lifecycle and availability contract

The Operations Center exposes three exact, data-free process endpoints:
`/healthz/startup`, `/healthz/live`, and `/healthz/ready`. They do not require
human trusted-proxy authentication and never expose identity, Secrets,
Kubernetes objects, or governance evidence. The exact-path routing exception
does not apply to `/api`; an unsigned protected API request remains rejected.
The probes use the image's hidden `healthz` command, which makes a loopback
HTTP request from inside the container. This avoids weakening the D2 ingress
NetworkPolicy for kubelet/node traffic.

Startup becomes successful after mandatory production configuration and
Kubernetes client initialization have completed and the listener has bound.
Liveness reports only that the HTTP process has started; it does not depend on
the Kubernetes API, trusted proxy, executor, Gadget, Cilium, or G6 Platform or
Projection health. Readiness is true after startup and false once graceful
shutdown begins. Projection `DEGRADED` and malformed Observations therefore do
not make a live Pod unready.

On cancellation/SIGTERM the process marks readiness false before calling
`http.Server.Shutdown`. Accepted in-flight requests receive up to five
seconds to drain; new listener work is stopped by the server shutdown. The
Deployment grants ten seconds of Kubernetes termination time, providing a
bounded margin. No arbitrary `preStop` sleep is used. The 1-replica
`Recreate` strategy remains intentional: restarts and HMAC rotation create a
bounded availability gap, and no zero-downtime or HA claim is made.

Observation cancellation handles remain process-local and are not durable
across an Operations Center restart. The dedicated executor owns durable
Observation execution independently; its 30-second claim lease and next-scan
`EXECUTOR_LOST` terminalization bound crash ambiguity. This lifecycle contract
does not change G6 operational health or any governance semantics.

## G9-E1 executor operability contract

The chart's `observationExecutor` is a separate Deployment, ServiceAccount,
RBAC set, and NetworkPolicy. It is fixed at one replica with `Recreate` and
uses the same minimal image with the `executor` entrypoint. It has no Service,
no ingress, and no governance permissions. Its namespace-local permissions
are limited to listing/getting Observations and updating `observations/status`,
reading target Pods/ReplicaSets, reading the named `kube-system` Namespace for
cluster identity, and listing/port-forwarding Gadget Pods.

The worker scans only configured target namespaces. A requested Observation is
claimed by `executorID` and `claimGeneration` with Kubernetes
`resourceVersion` CAS. The Runner renews its 30-second lease while active;
CAS fencing prevents a second executor from writing the same claim. An
expired non-terminal claim is terminalized as `FAILED/EXECUTOR_LOST` by the
next scan. This is bounded recovery, not exactly-once execution: an executor
crash can stop a Gadget session, and the persisted record remains non-terminal
until lease expiry and recovery.

SIGTERM cancels collection and gives the Runner a five-second persistence
window; if that window cannot finish, lease recovery is the truthful fallback.
The Operations Center only creates the durable `REQUESTED` record in
authenticated mode and does not execute Gadget directly. The local CLI path
retains its development synchronous behavior. Executor and Operations Center
authority therefore remain distinct.

## G9-O1 production observability contract

The Operations Center and Observation executor emit bounded JSON events to
their configured standard output. The production default log level is
`INFO`; `DEBUG` is an explicit diagnostic choice. Events carry component,
event, request/operation or Observation correlation, bounded reason, and
durable executor identifiers where available. Request IDs are generated by
the server when absent, validated to a bounded character set, returned in
the response, and never used for authorization.

Credential-bearing fields are redacted before serialization. HMAC material,
proxy signatures, Authorization-equivalent headers, ServiceAccount tokens,
executor kubeconfig contents, and Secret objects must never be logged.
Request bodies, evidence payloads, and raw filesystem events are not access
logged by default. Routes in metrics are normalized and do not contain
resource names or IDs.

Metrics are opt-in and served on a separate internal port (`9090` for the
Operations Center and `9091` for the executor by default). Enabling metrics
requires the corresponding component NetworkPolicy and explicit monitoring
namespace and pod selectors. The chart adds only that selected monitoring
ingress; it does not make the trusted-proxy application port public. Metrics
include bounded request, authentication, authorization, governance,
projection-exclusion, executor claim/active/completion/failure/lost,
lease-renewal, and Gadget-failure signals. Metric labels are limited to
bounded taxonomies, HTTP methods, normalized routes, status classes, and
component-safe results; usernames, resource names, UIDs, request IDs, paths,
and raw errors are forbidden.

Logs and metrics are diagnostic planes only. Durable Observation, Proposal,
ApplyAttempt, RollbackAttempt, custody, and G6 Platform/Projection state
remain the source of truth. Metrics availability does not affect D3
liveness/readiness or execution, and no Kubernetes Events are emitted for
normal request or lease activity; low-frequency event integration remains a
future operational policy decision. Recommended initial signals are HTTP 5xx,
authentication failures, executor lost recovery, Gadget failures, lease
renewal failures, and excluded projection objects. No SLO thresholds are
claimed by this contract.

## G9-U1 upgrade, recovery, and installation-safety contract

The six product CRDs are installed from Helm `crds/` as platform-level
prerequisites. Helm does not remove them on uninstall and does not update
them on ordinary upgrade; schema changes require an explicitly reviewed CRD
application and compatibility check. Durable TrainingHistory,
SecurityProfileProposal, ApplyAttempt, RollbackAttempt, Observation, and
ObservationContributionReceipt objects therefore survive application
uninstall/reinstall. Logs, metrics, HTTP connections, process memory,
request IDs, and executor cancellation handles are transient and are not
recovery state.

Legacy CLI cluster-scoped RBAC is Helm-owned by default. The explicit
`rbac.legacyClusterRoles.create=false` mode is for a platform-managed,
pre-verified compatible RBAC set; it renders no legacy cluster-scoped RBAC
and never adopts same-named foreign resources. A default install encountering
foreign ownership metadata fails closed with an actionable Helm ownership
error. Operators must choose external ownership or a distinct clean target;
they must not relabel, annotate, delete, or silently adopt the foreign
resource.

The executor Gadget Role and RoleBinding follow the same ownership contract.
They are Helm-owned by default; `observationExecutor.gadgetAccess.create=false`
is permitted only for a pre-verified, platform-managed compatible binding in
the configured Gadget namespace. The chart never adopts, relabels, annotates,
or mutates a foreign same-named Gadget resource.

Operations Center and executor workloads remain single-replica/Recreate, so
upgrades and HMAC rotation have bounded downtime. A completed Observation is
not re-executed by application reinstall. An active executor claim remains
fenced by its durable resourceVersion/claim generation and lease; after
expiry, the next executor scan records `EXECUTOR_LOST` rather than claiming
success. Helm release rollback is distinct from product ApplyAttempt rollback;
the former changes application artifacts, while the latter is a named,
authorized product operation. Helm rollback is supported only where the two
application revisions remain CRD-compatible; CRD migration rollback is not
claimed.

The minimum recovery boundary is: restore compatible CRDs and externally
managed Secret/configuration, reinstall the chart with recorded values, wait
for production validation and Recreate workloads to become Ready, then
verify signed authentication, NetworkPolicy, and durable object reads.
Backups must cover all six CRD object populations, Secret material through
the platform Secret-management process, and Helm values/release metadata;
logs and metrics are not backups. Cluster loss and cross-cluster disaster
recovery remain outside this v0.8 contract.
