# Operations Center frontend migration v1

Status: foundation slice on the post-v0.9.0 release baseline.

This document defines the migration boundary for replacing the server-rendered
vanilla frontend with a React/TypeScript application. The existing UI remains
available at `/` while the migration UI is served at `/next/` until parity is
proven and a deliberate cutover is approved.

## Authority boundary

The Go Operations Center remains authoritative. The new frontend is a
presentation and interaction client only:

* EnvironmentSession, cluster, namespace, identity, and context version come
  from the authenticated server APIs.
* Kubernetes objects are read through the bounded backend projections. The
  browser never receives kubeconfig, bearer credentials, or a Kubernetes
  client.
* Observation lifecycle, evidence qualification, candidate-v2, digest,
  provenance, governance CAS, SPHM, History, and Attention remain server-owned.
* TanStack Query keys include the authoritative context. Selection state is
  local to a page/tab and is never a server-side global current resource.
* Context changes invalidate context-bound queries and clear only selections
  that no longer belong to that context. Stale mutations are surfaced and are
  never replayed automatically.

## Foundation architecture

The source lives under `web/operations-center` and is built with Vite. A small
Go asset package embeds the built `dist` directory and exposes `/next/` from
the existing Workbench server. This keeps the release binary self-contained
and leaves the current `/` implementation operational as a reference oracle.

The completed foundation and M3/M4 vertical slices cover:

1. application shell and navigation;
2. authenticated environment/context binding;
3. namespace switching through the existing EnvironmentSession API;
4. workload discovery and exact workload selection;
5. authoritative workload YAML projection and copy feedback;
6. Observation collection/detail, server-returned Start identity, lifecycle
   polling, durable Stop intent, and terminal convergence;
7. Evidence result/proof/facts inspection, including terminal UNKNOWN reasons.

The M5/M6 slice adds Proposal inspection and Governance interaction without
changing the backend contracts:

* workload-bound Proposal collection and exact Proposal detail selection;
* Generate Proposal busy/success/domain-rejection feedback using the exact
  server-returned identity;
* structured policy inspection, server-derived YAML, and the canonical
  candidate-v2 JSON projection;
* candidate digest and Observation provenance display;
* Review, Approve, Reject, and Apply actions bound to server resourceVersion
  and candidate digest, with explicit stale-conflict reconciliation and no
  automatic replay.

The M7 slice adds History and Attention while leaving Overview and Health/SPHM
for later milestones. History reads the server's exact-object custody
projection from `GET /api/v08/history`; Attention reads the server-derived
operational projection from `GET /api/v08/environment`. Neither surface
reconstructs domain truth in React.

## M7 History and Attention contract

History is a best-effort, identity-bound read of authoritative observations,
training-history contributions, proposals, apply attempts and rollback
attempts. Timestamped events and untimestamped facts remain separate. The
React timeline only connects resources through the `SourceRef` and
`RelatedRef` values supplied by the backend; it never links records merely
because their timestamps are close. Filters are presentation-only, and an
exact selected event/detail remains independent from the refreshed collection.

Attention is the existing reconciliation projection, not a health score. Its
stable categories are `CAPABILITY_OUTSIDE_APPROVED_POLICY`,
`APPROVED_NOT_APPLIED`, `APPLICATION_STATE_UNKNOWN`,
`NEW_CONTRIBUTION_SINCE_CANDIDATE`, `OBSERVATION_FAILED`, and
`MULTIPLE_VALID_APPROVED_PROPOSALS`. The React view renders the exact subject
and durable observation/proposal/application references emitted by the server.
It does not add severity thresholds, acknowledgement state, or SPHM
semantics. An empty Attention projection means only that no current
authoritative Attention item was returned for the bound context; it does not
establish security health.

Both M7 query keys include cluster, namespace, context version and session.
Context changes remount the local selection surfaces, while ordinary refresh
does not erase a selected exact detail. Namespace and authorization remain
server-owned through the existing EnvironmentSession request headers.

## M8 Overview/SRE projection contract

M8 consumes, but does not define, the existing SPHM v1 report from
`GET /api/health`. Its operational Attention and recent activity inputs are
server projections over the same session-bound namespace read model exposed by
M7. `GET /api/v08/overview?limit=N` is a read-only composition of the existing
environment and History projectors; it introduces no new domain state or
thresholds.

The Overview deliberately does not calculate a score. The displayed posture
and dimension states are the authoritative SPHM state vocabulary. In
particular, `UNKNOWN` evidence remains distinct from `HEALTHY`, while
`NOT_ESTABLISHED` is used where the product has no authoritative denominator,
threshold, drift proof, or enforcement read model. Loading and section errors
are also distinct from a zero-valued authoritative population.

| Metric | Definition / source | Scope, population, window | Zero semantics | Unknown / unavailable semantics | Drilldown |
| --- | --- | --- | --- | --- | --- |
| Operational posture | SPHM `overall` from `GET /api/health` | bound namespace; retained observation/proposal read model; no synthetic window | not applicable | uses the server state/reason | Attention, observations, proposals |
| Attention items | M7 environment Attention projection | bound namespace; current authoritative projection; no synthetic window | no current Attention items | read error is an error, not empty/healthy | exact Observation/Proposal refs or Attention |
| Evidence posture | SPHM `evidence` dimension | bound namespace observations used by SPHM | server-provided value only | terminal UNKNOWN remains UNKNOWN | Observations / exact Attention refs |
| Pipeline posture | SPHM `pipeline` dimension | bound namespace observations used by SPHM | server-provided value only | read error is not healthy | Observations / exact failure refs |
| Governance posture | SPHM `governance` dimension | bound namespace proposals used by SPHM | zero pending proposals is meaningful only for this population | state/reason remain server-owned | Proposals |
| Recent activity | bounded timestamped events from the existing History projector | bound namespace retained authoritative inputs; explicit bounded projection, not a time window | no timestamped retained events | untimestamped facts remain a History limitation | exact History, Observation, or Proposal |
| SPHM preview | all existing SPHM dimensions | same `/api/health` context | dimension-specific | `NOT_ESTABLISHED` and `NOT_APPLICABLE` are rendered explicitly | M9 reserved |

Coverage ratios, freshness thresholds, drift, and enforcement health are not
implemented in M8 because their authoritative product definitions are not
established. The Overview never treats an empty Attention list, zero facts,
or an unavailable section as proof of security health.

## API contract used by the foundation slice

| Surface | Existing endpoint | Authority notes |
| --- | --- | --- |
| Context discovery | `GET /api/v09/environments` | server-side environment connector |
| Open context | `POST /api/v09/environments` | returns session identity |
| Namespace discovery | `GET /api/v09/environments/{session}/namespaces` | session-bound |
| Namespace binding | `GET /api/v09/environments/{session}/capabilities?namespace=` | validates context and returns context version |
| Operational context | `GET /api/v08/operations-context` | authenticated server projection |
| Workloads | `GET /api/workloads` | bounded namespace-scoped read capability |
| Workload object | `GET /api/workloads/detail?...` | exact UID/context-bound safe manifest projection |

No new authority is introduced in this slice.

## Functional parity matrix

Legend: **Existing** is present in the current UI; **Foundation** is covered
by the migration slice; **Next** is retained for the next vertical slice.

| Surface / capability | Existing | Foundation | Next / qualification requirement |
| --- | --- | --- | --- |
| Environment, cluster, identity, namespace | Existing | Foundation | preserve context-version and stale rejection |
| Namespace isolation and fail-closed mismatch | Existing | Foundation | API + multi-tab regression |
| Workload collection and exact selection | Existing | Foundation | refresh-preserving selection |
| Workload kind/name/namespace/UID/container/image | Existing | Foundation | cross-check authoritative projection |
| Workload authoritative YAML | Existing | Foundation | copy, long values, RBAC/context binding |
| Start Observation and busy feedback | Existing | Foundation | semantic lifecycle waits |
| Observation lifecycle and Stop | Existing | Foundation | durable Stop, CAS, executor fencing |
| Failed Observation / executor loss | Existing | Foundation | forensic state and terminal immutability |
| Evidence facts and qualification | Existing | Foundation | AVAILABLE vs terminal UNKNOWN |
| Proposal generation | Existing | M5 | exact identity, busy/failure/success |
| Structured candidate-v2 | Existing | M5 | scannable policy hierarchy |
| Derived YAML / canonical Raw JSON | Existing | M5 | identity and representation persistence |
| Candidate digest and Observation provenance | Existing | M5 | server-owned identity/provenance display |
| Review / Approve / Reject / Apply | Existing | M6 | resourceVersion/CAS and no replay |
| History collection, exact detail, timeline and limits | Existing | M7 | `/api/v08/history`, exact `SourceRef` identity, filter/refresh preservation |
| Attention categories, exact references and drill-down | Existing | M7 | `/api/v08/environment`, no frontend severity or SPHM calculation |
| Overview operational posture, Attention, Evidence, Pipeline, Governance | Existing | M8 | consume authoritative SPHM/M7 projections without a score |
| Overview recent activity and exact drill-down | Existing | M8 | bounded server-owned History composition |
| Overview SPHM preview | Existing | M8 | render existing dimensions; full explanation remains M9 |
| Overview | Existing | M8 | responsive SRE cockpit; legacy route remains intact |
| Health / SPHM | Existing | Next | preserve SPHM v1 states and sources |
| Multi-tab isolation | Existing | Next | independent query/selection state |
| Keyboard and responsive qualification | Existing | Foundation | 1440/1280/1024/680 browser proof |

## Query and selection rules

Authoritative query keys use the tuple `(cluster, namespace, contextVersion,
resource identity)`. Collection data, selected resource identity, and selected
detail are separate state concepts. An empty or lagging collection response
cannot erase a selected exact detail that remains authoritatively readable.

The foundation implementation uses a context object local to the application
instance. It is not a module-level mutable singleton and is not shared between
browser tabs.

## Readiness and testing

The new UI's browser readiness boundary is semantic: the shell is mounted,
then the authoritative context is rendered, then the workload collection and
selected workload identity are visible. It does not use `networkidle`, fixed
sleeps, or timeout inflation. Existing periodic-refresh behavior is expected.

The old UI remains the parity oracle during migration. The new route will gain
journey coverage slice by slice before any cutover is considered.

## Security non-regression

The migration must preserve `CREDENTIALS_BROWSER_EXPOSED=NO`,
`KUBECONFIG_BROWSER_EXPOSED=NO`, namespace/cluster/session binding, RBAC and
trusted-proxy semantics, candidate digest/provenance, evidence truth, and
governance CAS. Any change to those contracts requires a separate product or
security decision and is outside this migration slice.
