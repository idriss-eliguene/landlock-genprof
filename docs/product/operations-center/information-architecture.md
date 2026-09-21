# Information architecture

The persistent shell contains Home, Workloads, Proposals & Governance,
History, Attention, and Health. The shell owns one global Operational
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
| Home | What needs attention now? | attention, failures, recent authoritative signals | projection diagnostics |
| Workloads | What can I inspect? | workload identity and Observe | UIDs and runtime identity |
| Proposals & Governance | Why does this exist and what decision is pending? | evidence-bound proposal and deliberate action | digest, provenance, CAS |
| History | What happened? | timeline and actors | immutable IDs and versions |
| Attention | What needs review? | actionable items | diagnostic reason |
| Health | Is the security pipeline observable? | SPHM dimensions and proof | unknown/not-established states |

Observation and Evidence remain reachable through transitional routes and
workload entry points. They are not Level-1 navigation owners until the
workload dossier milestone.

## M10.5 routing and restoration

The React surface uses history-backed routes at the canonical root: `/` (Home),
`/workloads`, `/proposals`, `/history`, `/attention`, and `/health`.
`/observations` and `/evidence` remain transitional routes for existing
journeys. `/next/` is a temporary compatibility prefix that redirects to the
corresponding canonical root route. A URL is a locator, not an authority grant;
server resolution is required before an object is rendered as authoritative.

The shell may write a per-tab `sessionStorage` restoration hint containing a
context name and requested namespace. This is only a restore candidate. The
server must open the context and bind the namespace again before the UI becomes
bound or namespace-scoped queries run. Session IDs, context versions, actors,
capabilities, cluster identities, and authorization are never restored from
browser storage. `localStorage` and cross-tab authority synchronization are
not used.

The M10.5 scope establishes shell and state foundations only. It does not
create a global Activity read model, redesign Observation/Evidence or
governance, decide Candidate YAML semantics, or add backend/domain APIs.
