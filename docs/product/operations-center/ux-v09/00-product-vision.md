# 00 — Product vision

## What the Operations Center is

The Operations Center is a local, environment-aware console for
`landlock-genprof`'s Kubernetes runtime-security workflow: discover
workloads, capture real runtime behavior as evidence, turn qualified
evidence into a reviewable seccomp/capability proposal, and govern its
approval and application under Kubernetes RBAC and optimistic-concurrency
(CAS) control.

It is not a general Kubernetes dashboard and does not try to replace one. It
exposes exactly the surfaces needed to run this one workflow safely:
Overview, Workloads, Observations, Proposals & Governance, History,
Attention.

## Who it is for

Platform/DevOps engineers running observations against their own workloads,
and security reviewers governing the proposals those observations produce.
See [01-personas.md](01-personas.md).

## The one sentence a first-time operator should understand

> "I pick where and as whom I'm operating, I watch a workload run, I get an
> evidence-backed proposal, and nothing gets applied to the cluster without
> an explicit, auditable governance decision."

## Product principles this redesign held to

1. **The backend's uncertainty is the UI's uncertainty.** If the backend
   cannot prove evidence is available, the UI must not imply that it can.
   `UNKNOWN` is never styled or worded as success. See
   [06-evidence-model.md](06-evidence-model.md).
2. **State drives controls, not the other way around.** An action is only
   ever shown as available when the current lifecycle state actually
   permits it; a disabled control always carries a stated reason. See
   [05-observation-state-model.md](05-observation-state-model.md) and
   [04-interaction-model.md](04-interaction-model.md).
3. **Workload identity is the operator's identity for the data.**
   Observation UUIDs, digests, and resourceVersions are real and stay
   inspectable, but they are secondary to "which Deployment, which
   container."
4. **Authorization is never faked.** Every capability the UI gates on
   (`proposal.review`, `proposal.approve`, `proposal.apply`, ...) comes from
   the server's own SSAR-derived capability read (`/api/v08/capabilities`).
   The UI has no local notion of roles or personas.
5. **Do not invent certainty the backend does not have.** No fabricated
   metrics, no synthesized "risk scores," no fake empty/healthy state when a
   read actually failed (`UNAVAILABLE` and `EMPTY` are rendered
   differently, everywhere).

## What changed in this pass vs. what stayed

Preserved as-is (see [12-security-ux-constraints.md](12-security-ux-constraints.md)
for the full invariant list): RBAC/SSAR authority, CAS/resourceVersion
semantics, candidate digest and provenance semantics, observation
attribution semantics, the environment-session binding model.

Changed: navigation grouping (Governance folded into Proposals), the
Observations action bar (from three always-on buttons to a state-driven
lifecycle control), several disclosure-quality microcopy strings, one
concrete client-side bug (Identity selector default binding), and CSS for
several control-spacing defects found in the visual review loop. Full diff
is in [13-implementation-map.md](13-implementation-map.md).
