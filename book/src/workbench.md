# Governance Operations Workbench

The v0.8 Workbench is a trusted-local, loopback-only, server-rendered
Governance Operations read surface. Its navigation is six top-level surfaces:

```text
Overview · Workloads · Observations · Proposals · History · Attention
```

History is a canonical top-level surface, not a drill-down. The browser
remains read-only for governance operations.

## Install and launch

Use the [installation guide](INSTALL.md) for the current source/pre-release
baseline. The Workbench uses the invoking kubeconfig identity and keeps its
namespace pinned to the requested scope:

```bash
kubectl landlock-genprof ui --namespace <namespace>
```

The default URL is `http://127.0.0.1:8080`. Use `--port <port>` to select
another local port. The listener is local-only and has no remote-management,
session, or authentication contract.

## Workload and Observation flow

The Workbench begins with workload/container selection and displays the
selected identity: namespace, GroupKind, workload name and UID, container,
image identity, and ClusterIdentity where available. Display locators are not
authority-bearing identity.

The user can:

1. Start an Observation through the certified API.
2. Read authoritative Status; accepted does not fabricate `RUNNING`.
3. Stop an Observation through the certified API where meaningful.
4. Rediscover durable Observations after closing and reopening the browser.
5. Inspect Execution, Attribution, Evidence, and Proposal eligibility as
   separate axes.

Evidence sources are shown independently: filesystem, exec, network connect,
network bind, and capabilities. `AVAILABLE`, `EMPTY`, and `UNKNOWN` remain
distinct. UNKNOWN preserves positive facts; EMPTY does not mean that the
workload never performed a behavior; AVAILABLE does not mean complete workload
behavior.

## Proposal flow

Generate Proposal calls the certified API and reloads the persisted Proposal
through the read model. Candidate-v2 displays:

- Subject Scope `CONTAINER`, Target, Container, and ImageIdentity;
- artifact `CONTAINER_CAPABILITIES`;
- Drop `ALL`;
- canonical observed `CAP_*` facts in Add;
- provenance ObservationIDs, qualification, and derivation status;
- CandidateDigest and ReviewContextDigest as separate bindings.

Candidate-v2 does not contain workload UID. The Observation is workload-UID
bound; the Proposal subject is intentionally weaker and must not be presented
as UID-bound.

The browser can display approval state, approved digests, authority state, and
`LastApprovalSnapshot` when the read model provides them. The last snapshot is
last recorded approval custody, not a complete approval history.

## Authority boundary

The browser cannot Approve, Reject, Revoke, Apply, or Rollback. It cannot
activate custody or execute CLI commands. Where useful, the page may display
copyable CLI-only guidance. Approval is not application; application is not
enforcement; enforcement is not behavioral verification.

The Workbench does not claim complete workload behavior, complete least
privilege, current enforcement, behavioral verification, fleet governance, or
a full Security Operating Center. Environment and History are best-effort
multi-object projections, not transactional snapshots.

## Trust and bounded reads

Reads are namespace-scoped and bounded through the Workbench read capability.
The page uses durable Kubernetes state rather than browser-local authority.
Host/origin and Fetch Metadata protections remain active on mutation routes,
and the trusted-local listener must not be exposed through an ingress or used
as a shared multi-user service.
