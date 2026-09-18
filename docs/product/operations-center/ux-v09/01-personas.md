# 01 — Personas

Three personas, matching the mission brief and the real authorization model
(Kubernetes RBAC/SSAR — there is no application-level role concept; see
[12-security-ux-constraints.md](12-security-ux-constraints.md)).

## Platform / DevOps engineer

**Goals:** pick cluster/identity/namespace, find their workload, start an
observation, understand what was captured, generate a candidate proposal,
understand what applying it would change.

**Mental model:** Kubernetes resources — Deployments, Pods, containers,
rollout state. Not familiar with `landlock-genprof` internals (attribution,
qualification, candidate digests) and should not need to be for the primary
flow.

**Primary surfaces:** Workloads → Observations → Proposals & Governance
(Review/Generate only, if their capabilities allow).

**What the UI owes them:** the workload-first observation cards
(`observationTitle`/`observationSubtitle` in `workbench_ui.go`), the
lifecycle status line ("Latest observation: completed.", "Observing runtime
activity…"), and evidence explained in prose before its state token.

## Security reviewer

**Goals:** inspect observations and their evidence quality, inspect
capability facts and provenance, review proposals, understand policy
impact, approve/reject/apply where their capabilities allow, inspect
history.

**Mental model:** evidence, trust, least privilege, provenance, governance
state machines. Comfortable with the raw identifiers (candidate digest,
resourceVersion, observation ID) as forensic material.

**Primary surfaces:** Observations (evidence detail, "Technical observation
details"), Proposals & Governance (full action set), History.

**What the UI owes them:** the explicit `Evidence state unknown` /
`Evidence captured` / `No attributable evidence` distinction (never
softened into a generic "OK"), the forensic disclosures (`<details>` blocks
with candidate digest, resourceVersion, attribution state, `Behavioral
verification: UNKNOWN`), and a governance surface that names the exact
reason an action is unavailable (`NOT_SEMANTICALLY_ELIGIBLE`,
`NOT_AUTHORIZED`, `STALE`).

## Restricted user

**Goals:** operate only within what they're authorized for; get a clear
explanation when something is unavailable, without the UI either hiding
that a resource class exists or leaking what it contains.

**Mental model:** "I know my namespace; the cluster is bigger than what I
can see."

**How the UI honors this today:**

- Namespace binding supports both list-based discovery and explicit entry
  (`discovery.mode === "EXPLICIT_ONLY"` → the namespace `<select>` is
  replaced by a labeled text input; see
  `cmd/landlock-genprof/workbench_ui.go`, `loadEnvironmentContexts`). A
  restricted identity that cannot `list` namespaces can still open one it
  knows by name.
- Governance buttons never disappear when a capability is missing; they
  render disabled with `NOT_AUTHORIZED — capability is unavailable.`
  (`proposalActions` → `add(...)` in `workbench_ui.go`). The operator always
  sees that the action *exists* and *why* they can't take it, rather than a
  UI that pretends the action doesn't exist.
- Capabilities come only from `/api/v08/capabilities`
  (SSAR-derived, server-side). There is no client-side role table to keep in
  sync or to accidentally get wrong.

This persona is where "don't leak, don't lie" is hardest, and it is also
where this redesign changed the least — the existing capability-gating
pattern already gets this right; see
[12-security-ux-constraints.md](12-security-ux-constraints.md) for what was
verified rather than changed.
