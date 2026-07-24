# landlock-genprof Helm chart

Packages the plain manifests under [`deploy/*.yaml`](../../) (RBAC + CRDs
for the tracer's ServiceAccount) as a Helm chart, instead of applying seven
separate files by hand. See those files' own comments, and
[`docs/threat-model.md`](../../../docs/threat-model.md) §1, for the
per-rule rationale this chart's templates preserve.

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
- Both CRDs (`SecurityProfileProposal`, `TrainingHistory`)

Opt-in, via `values.yaml` (each is a real, documented increase in blast
radius — enable only if you intend to use the matching flag):

| Value | Flag it enables | What it grants |
|---|---|---|
| `restart.enabled` | `trace --restart` | delete/create pods, patch Deployments/StatefulSets/DaemonSets |
| `history.enabled` | `trace --history` | create/update `TrainingHistory` objects |

## Prerequisites

[Inspektor Gadget](https://www.inspektor-gadget.io/) must already be
deployed in the cluster (`kubectl gadget deploy`, namespace `gadget` by
default — see `values.yaml`'s `gadget.namespace`) — this chart only grants
the tracer permission to reach it, it doesn't install it.

## Install

```bash
helm install landlock-genprof deploy/helm/landlock-genprof
```

With `--restart`/`--history` support:

```bash
helm install landlock-genprof deploy/helm/landlock-genprof \
  --set restart.enabled=true \
  --set history.enabled=true
```

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
