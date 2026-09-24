# landlock-genprof Helm chart

Packages the project manifests under [`deploy/*.yaml`](../../) (RBAC + CRDs
for the tracer's ServiceAccount) as a Helm chart. See those files' own comments, and
[`docs/threat-model.md`](../../../docs/threat-model.md) §1, for the
per-rule rationale this chart's templates preserve.

For the v0.8.1 release, this chart packages the Observation,
ObservationContributionReceipt, TrainingHistory, and Proposal CRDs. The
Observation writer and history-writer permissions remain explicit opt-ins;
enable them only for the service-account workflow that uses those adapters.
The local Observation Workbench instead uses the invoking kubeconfig identity
and requires that identity to have the relevant namespace-scoped reads.

The chart can optionally deploy the v0.8 Operations Center. The default
`operationsCenter.enabled=false` preserves the CLI-only installation. When
enabled, it creates exactly one Operations Center Deployment and a ClusterIP
Service. The Service is an internal target for an externally managed trusted
proxy; this chart deliberately creates no direct Operations Center Ingress.
NetworkPolicy hardening is enabled by default for the Operations Center and
requires explicit trusted-proxy namespace/pod selectors and Kubernetes API
CIDRs and ports. Incomplete selector or egress configuration fails Helm
rendering.

The Operations Center requires two pre-created Secrets when enabled:

* `operationsCenter.trustedProxySecret.name`, containing the configured
  `hmac-secret` key by default;
* `operationsCenter.executorKubeconfigSecret.name`, containing the `config`
  key by default.

Secret values are never chart defaults or ConfigMap data. Production mode is
fixed in the Deployment. v0.8 requires one replica and uses a `Recreate`
strategy because process-local Observation cancellation state is not
multi-replica safe. The backend binds to the Pod network interface and is
protected by the ClusterIP/proxy topology and component-scoped NetworkPolicy.
ClusterIP alone is not the security boundary. The policy allows only the
configured trusted-proxy pods to the application port and allows egress only
to configured Kubernetes API CIDRs and ports and kube-system DNS. Lifecycle
probes use the binary's internal loopback `healthz` command, so probe traffic
does not require a broad NetworkPolicy ingress exception. The application
allows up to five seconds for graceful HTTP draining and the Pod has a
ten-second termination grace period. Projection degradation is not a process
liveness or readiness failure. The one-replica `Recreate` contract
intentionally permits a bounded restart/rotation availability gap.
On Cilium clusters, the chart also renders a narrowly scoped
`CiliumNetworkPolicy` allowing only the `kube-apiserver` entity on those API
ports; this is required because host-networked API endpoints are not
reliably selectable by portable `ipBlock` rules after Service translation.

At least one explicit human username must be supplied in
`operationsCenter.impersonation.allowedUsers` because the backend performs
request-scoped Kubernetes impersonation with both the authenticated username
and groups. Group-only authentication allowlists remain supported by the
application contract, but cannot produce a functioning Kubernetes RBAC
binding for this Deployment without a corresponding username permission.

## What gets installed

Always (no toggle — `trace` doesn't work without them):

- `Namespace`/`ServiceAccount` for the tracer identity
- Pod read access + Inspektor Gadget access (`rbac.yaml`)
- `SecurityProfileProposal` publishing RBAC (`rbac-proposal.yaml`) —
  mandatory since every `trace` run publishes one
- Patched-manifest RBAC (`rbac-patched-manifest.yaml`) — used by
  `publishProposal` itself, not just `--patched-manifest-out`
- Project CRDs, including `SecurityProfileProposal`, `TrainingHistory`,
  `ApplyAttempt`, and `RollbackAttempt`

The optional `rbac-workbench.yaml` role grants only unbound `get`/`list`
visibility for the five v0.8 Workbench read families: Observation,
SecurityProfileProposal, TrainingHistory, ApplyAttempt, and RollbackAttempt.
It also retains the exact ApplyAttempt CRD read needed by the legacy custody
view. It does not grant mutation authority and is disabled by default.

Opt-in, via `values.yaml` (each is a real, documented increase in blast
radius — enable only if you intend to use the matching flag):

| Value | Flag it enables | What it grants |
|---|---|---|
| `restart.enabled` | `trace --restart` | delete/create pods, patch Deployments/StatefulSets/DaemonSets |
| `history.enabled` | `trace --history` | create/update `TrainingHistory` objects |
| `observation.enabled` | Observation persistence adapter | create/read Observations and update Observation status |

## Prerequisites

[Inspektor Gadget](https://www.inspektor-gadget.io/) must already be
deployed in the cluster (`kubectl gadget deploy`, namespace `gadget` by
default — see `values.yaml`'s `gadget.namespace`) — this chart only grants
the tracer permission to reach it, it doesn't install it.

## Install

```bash
helm install landlock-genprof deploy/helm/landlock-genprof
```

### Cluster-scoped RBAC ownership

The legacy CLI `ClusterRole` and `ClusterRoleBinding` resources are Helm-owned
by default through `rbac.legacyClusterRoles.create=true`. A clean installation
may therefore create them, and a Helm upgrade may update them.

If a platform already manages a compatible legacy RBAC set, set
`rbac.legacyClusterRoles.create=false` before installation. In that mode the
chart renders none of those legacy cluster-scoped objects and never adopts,
labels, annotates, or mutates same-named foreign resources. The platform
administrator must verify that the external roles and bindings match the
required manifests before enabling CLI features. A default installation into
a cluster containing same-named foreign objects fails closed with Helm's
ownership error; do not resolve that error by adopting or deleting the
foreign object.

Operations Center and Observation executor RBAC remain separately rendered
resources with their own names and ownership. They are not satisfied by the
legacy switch.

The executor Gadget Role and RoleBinding are Helm-owned by default. Set
`observationExecutor.gadgetAccess.create=false` only when a compatible,
platform-managed binding already exists in the configured Gadget namespace.
The chart never adopts, relabels, annotates, or mutates a foreign same-named
Gadget resource.

To install the optional, still-unbound Workbench attempt reader role:

```bash
helm install landlock-genprof deploy/helm/landlock-genprof \
  --set workbench.readerRole.create=true
```

This role grants only read access to the five v0.8 Workbench object families
and the exact ApplyAttempt CRD for the current custody epoch. It grants no
target or browser mutation authority; an operator must create any desired
binding explicitly.

With `--restart`/`--history` support:

```bash
helm install landlock-genprof deploy/helm/landlock-genprof \
  --set restart.enabled=true \
  --set history.enabled=true
```

For the service-account-backed Observation adapter, add
`--set observation.enabled=true`. This does not grant browser approval,
application, rollback, or other governance authority. To expose the optional
legacy ApplyAttempt/RollbackAttempt read view, separately set
`workbench.readerRole.create=true` and bind the resulting unbound role as
appropriate for the local operator.

Operations Center example, using existing Secret names and no secret values:

```bash
helm upgrade --install landlock-genprof deploy/helm/landlock-genprof \
  --set namespace.name=g5-filesystem \
  --set namespace.create=false \
  --set operationsCenter.enabled=true \
  --set operationsCenter.trustedProxySecret.name=operations-center-hmac \
  --set operationsCenter.executorKubeconfigSecret.name=operations-center-executor-kubeconfig \
  --set operationsCenter.profileRealizerKubeconfigSecret.name=operations-center-profile-realizer-kubeconfig \
  --set operationsCenter.networkPolicy.trustedProxy.namespaceSelector.matchLabels.kubernetes\\.io/metadata\\.name=trusted-proxy \
  --set operationsCenter.networkPolicy.trustedProxy.podSelector.matchLabels.app=trusted-proxy \
  --set operationsCenter.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 \
  --set observationExecutor.networkPolicy.kubernetesApiCIDRs[0]=10.96.0.1/32 \
  --set operationsCenter.impersonation.allowedGroups[0]=operations-team \
  --set operationsCenter.reviewGroups[0]=security-reviewers \
  --set operationsCenter.approverGroups[0]=security-approvers \
  --set operationsCenter.teamRoles.create=true \
  --set observationExecutor.enabled=true \
  --set 'observationExecutor.targetNamespaces[0]=g5-filesystem'
```

The trusted proxy remains external to this chart and must strip client
identity headers before signing. Do not expose the Operations Center Service
through NodePort, LoadBalancer, or a direct Ingress. The production HMAC
rotation model is restart-based; dual-key rotation is not implemented.

The profile-realizer kubeconfig is a separate technical identity. It is used
only for the controlled creation, readiness read, update, and rollback of
cluster-scoped SPO `SeccompProfile` objects. Human/team namespace-local
RoleBindings do not receive those permissions. When `teamRoles.create=true`,
the chart renders the unbound `landlock-genprof-profile-realizer` ClusterRole;
cluster administrators must bind it only to the dedicated profile-realizer
identity. The application still verifies the approved candidate digest,
deterministic target name, ownership annotations, and namespace before any
profile mutation.

### Observability

Set `operationsCenter.observability.metrics.enabled=true` and/or
`observationExecutor.observability.metrics.enabled=true` only with explicit
monitoring namespace/pod selectors. The chart then exposes the selected
component's internal metrics port through narrowly selected NetworkPolicy
ingress; metrics are not available to arbitrary Pods and do not replace the
trusted proxy. The default log level is `INFO`. Metrics and logs contain only
bounded operational labels/fields and never HMAC, signature, token,
kubeconfig, Secret, UID, resource-name, or request-ID metric values.

Monitoring is optional: a missing scraper must not affect process health,
readiness, governance, or Observation execution. Product and governance state
remain authoritative in Kubernetes objects; logs and metrics are diagnostics.

## Upgrading — the CRD caveat

Helm installs everything under `crds/` on the **first** `helm install`
only, and **never** touches it again on `helm upgrade` — this is Helm's
own documented behavior, not a bug in this chart (see
[Helm's own docs on CRDs](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/)).
This project's CRD schemas have already changed more than once (field
renames on `SecurityProfileProposal` — see `docs/roadmap.md`), so after
`helm upgrade` to a version whose CRDs changed, re-apply them yourself:

```bash
kubectl apply -f deploy/helm/landlock-genprof/crds/
```

## Uninstall

```bash
helm uninstall landlock-genprof
```

Doesn't remove the CRDs (same Helm behavior as above, deliberately: it
would delete every `SecurityProfileProposal`/`TrainingHistory` object
cluster-wide along with them). Remove them yourself if that's actually what
you want:

```bash
kubectl delete -f deploy/helm/landlock-genprof/crds/
```
