# Information architecture

The persistent shell contains Overview, Workloads, Observations, Proposals,
History, Attention, and Governance. The shell owns one global Operational
Context. A Kubernetes context is opened by the server, its durable cluster
identity is resolved authoritatively, and its namespace is then bound through
the EnvironmentSession. Pages consume that bound tuple; they do not recreate
independent authority selectors.

The compact shell control is labelled `Context`, `Cluster`, and `Namespace`.
`Context` means the kubeconfig context locator and is distinct from the
authenticated actor. Context details retain the session, context version,
cluster identity, actor, and discovery mode for operational transparency.

Namespace choices are always scoped to the bound context. In `DISCOVERED`
mode the server-authorized list is searchable; in `EXPLICIT_ONLY` mode the UI
does not enumerate namespaces and submits a known namespace for server-side
validation.

| Page | Primary question | Primary information/action | Detail |
|---|---|---|---|
| Overview | What is happening? | health, counts, attention | projection diagnostics |
| Workloads | What can I inspect? | workload identity and Observe | UIDs and runtime identity |
| Observations | What was observed? | lifecycle, evidence, facts | IDs, references, raw state |
| Proposals | Why does this exist? | evidence-bound proposal | digest and provenance |
| History | What happened? | timeline and actors | immutable IDs and versions |
| Attention | What needs review? | actionable items | diagnostic reason |
| Governance | What can I change? | deliberate authorized action | CAS/resourceVersion |
