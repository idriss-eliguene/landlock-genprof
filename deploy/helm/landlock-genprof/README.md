# landlock-genprof Helm chart

Packages the project manifests under [`deploy/*.yaml`](../../) (RBAC + CRDs
for the tracer's ServiceAccount) as a Helm chart. See those files' own comments, and
[`docs/threat-model.md`](../../../docs/threat-model.md) §1, for the
per-rule rationale this chart's templates preserve.

For the v0.8 release candidate, this chart packages the Observation,
ObservationContributionReceipt, TrainingHistory, and Proposal CRDs. The
Observation writer and history-writer permissions remain explicit opt-ins;
enable them only for the service-account workflow that uses those adapters.
The local Observation Workbench instead uses the invoking kubeconfig identity
and requires that identity to have the relevant namespace-scoped reads.

**This chart does not deploy landlock-genprof itself.** There's no
Deployment/Pod here — `landlock-genprof` is a CLI tool (also usable as a
`kubectl` plugin, see the main [`README.md`](../../../README.md)), invoked
on demand from outside the cluster, not a long-running in-cluster service.

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
