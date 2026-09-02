# ADR-0023: Local Workbench HTTP trust boundary

## Status

Accepted

## Context

v0.4 shipped `ui <proposal>`: a loopback-only HTTP server that read one
SecurityProfileProposal once at startup and served a fixed page for the
process's lifetime. v0.5 introduces browser-triggered, bounded, live
Kubernetes reads (issue #185). That is a new trust boundary: a browser page
the operator merely has open can now cause this process to read the cluster
on demand, and the read path that server used — a full write-capable
`dynamic.Interface` from `newDynamicClient` — was never scoped for that.

## Decision

The Workbench HTTP server's only Kubernetes dependency is
`k8s.WorkbenchReadCapability` (ADR-0021): twelve bounded GET/LIST methods,
no client-go interface, no dynamic client, no write verb. `workbenchServer`
holds exactly one Kubernetes-typed field, of that interface type; nothing in
the HTTP package imports a write-capable client package. This is a
structural property enforced by the type checker, not a behavioral audit —
see `TestWorkbenchReadCapability_ExposesOnlyBoundedReadMethods` and
`TestWorkbenchServer_HoldsNoWriteCapableKubernetesField`, which fail at
compile-adjacent test time if either type is ever widened.

`ui <proposal>` is preserved: same command name, same required argument, same
flags. Internally it now performs one bounded live read per request instead
of one read before `ListenAndServe`, superseding the v0.4 fixed-snapshot
contract — see "v0.4 snapshot → v0.5 live-read transition" below. Two new
routes, `/api/workloads` and `/api/projection`, expose G1 discovery and G2
projection over the same bounded capability, for the future G4 Workbench
experience (#186).

## Trust boundary

| Input | Classification |
|---|---|
| HTTP client (any local process) | Untrusted |
| Request path, query parameters | Untrusted → validated before use |
| Request body | Untrusted; rejected outright (read-only API) |
| `Host` header | Untrusted → validated (primary DNS-rebinding defense) |
| `Origin`, `Sec-Fetch-Site` | Untrusted → validated when present |
| Namespace | Trusted internal state — pinned into `ReadSession` at process startup from the `-n` flag; never accepted from a request |
| Kubernetes API response | External authority |
| Projection/discovery result | Derived information — not enforcement |

## Authority model

Every route is `GET`-only. The method is checked once, in `ServeHTTP`,
before the body check or routing — every route this server has is GET-only,
so a non-`GET` request is a 405 method-contract violation regardless of
whether it also carries a body. No handler calls, or can call, Create,
Update, Patch, Delete, UpdateStatus, apply, approve, publish, or save.
Approval and application remain exclusively CLI operations.

## Local-only binding

The listener always binds `127.0.0.1:<port>`; the host portion is not
operator-configurable — there is no host flag, and address construction is
centralized in one function (`workbenchListenAddress`), unit-tested to
never produce `0.0.0.0`, `[::]`, `localhost`, or an empty host.

## Host validation

The server precomputes its own `127.0.0.1:<port>` authority at construction
and rejects any request whose `Host` header does not match it exactly —
including a bare `127.0.0.1` with no port, `localhost:<port>`, and any
attacker domain that DNS-rebinds to loopback. This is the primary defense
against DNS rebinding: a page served from an attacker domain that resolves
to `127.0.0.1` still sends that domain (or nothing) as `Host`.

## Browser-origin policy

- **Fetch Metadata** (`Sec-Fetch-Site`), when present, is authoritative:
  only `same-origin` and `none` (direct navigation) are accepted;
  `cross-site` and `same-site` are rejected. It is browser-set and
  unforgeable by page script.
- **Origin**, when present and Fetch Metadata is absent, must exactly match
  this server's own `http://127.0.0.1:<port>` — never reflected.
- **Neither header present** is treated as a non-browser local client and
  admitted, subject to Host validation, which already ran. A curl-style
  local tool sends neither header; this server must remain usable for it.
- **CORS**: no `Access-Control-Allow-Origin` is ever emitted, permissive or
  reflected. Its absence is what stops a cross-origin browser page from
  reading a response at all.
- **CSRF**: no token is required — there is no mutation endpoint to protect.
  The residual risk this section addresses is cross-origin *triggering* of
  bounded cluster reads, not state-changing requests.

These are browser-boundary controls. **They are not authentication.** An
arbitrary local process can forge any header this server reads, including
`Host`, `Origin`, and `Sec-Fetch-Site`. G3 does not defend against that, and
does not claim to.

## Request validation

`/api/projection` accepts exactly `group`, `kind`, `name`, `container`;
`/api/workloads` accepts none. Any other parameter, a duplicate value, more
than 8 parameters total, or a value failing the appropriate
Kubernetes-identifier pattern (RFC 1123 subdomain for `group`/`name`, a
DNS-1123 label for `container`, an upper-camel identifier for `kind`, all
bounded to 253 characters) is rejected with 400 before any cluster read.

A request selector is never assembled into a `GovernedTarget` directly: it
only narrows which container, already discovered by `workload.Discover` and
already carrying a real `Container.Target`, is selected. See
`resolveGovernedTarget`.

## Bounded read capability

`/api/projection` deliberately supplies `projection.Inputs{Evidence: nil}`.
G3 introduces no TrainingHistory loader over the bounded capability — the
existing evidence-loading paths (`association.EvidenceFromPopulation`,
`k8s.WorkbenchReadCapability.GetTrainingHistory`) have no production
consumer, and `internal/history.Get` requires a write-capable
`dynamic.Interface`, which this server must not hold. `Runtime` therefore
reports `EMPTY`: an honest statement that no evidence was supplied, not that
none exists. A bounded evidence loader is separate, later work.
`RuntimeSubjects` reuses exactly what the same bounded discovery pass
already returned for the resolved target — no additional cluster access.

## Deadlines, concurrency, and response bounds

Every Kubernetes-backed request gets an explicit `context.WithTimeout`
(`workbenchClusterReadDeadline`, 8s), strictly shorter than the server's
`WriteTimeout` (15s) so a request that hits its own deadline can still
render a typed error before the transport would forcibly close the
connection. Simultaneous cluster-backed requests are bounded by a counting
semaphore (`workbenchMaxConcurrentReads`, 8); a saturated server fails fast
with 503 and `Retry-After: 1` rather than queuing unboundedly, which keeps
the bound trivially testable and avoids stacking requests past their own
deadlines under load.

Response boundedness rests on two layers. The primary proof is structural:
`ReadSession` is pinned to one namespace at construction (ADR-0021), so
every list this server can trigger is already scoped to that namespace's
object count — not redesigned here. A defense-in-depth byte cap
(`workbenchMaxResponseBytes`, 4 MiB) additionally buffers and checks every
JSON response before writing it, refusing with 500 rather than sending a
partial body if it is ever exceeded; this is not expected to trigger and is
not a substitute for the namespace-scoping bound.

## Error semantics

Typed `k8s.ReadState` is preserved across the HTTP boundary as a JSON
`state` field alongside the transport status, never collapsed to an empty
200: `NOT_FOUND`→404, `PERMISSION_DENIED`→403, `BACKEND_NOT_INSTALLED`→503,
`TIMEOUT`→504, `UNSUPPORTED`→501 (declared for completeness; unreachable in
production today), unclassified→`UNKNOWN`/500. The `reason` text at this
boundary is a fixed, generic string per state — never the wrapped
Kubernetes/client-go error text, which can name RBAC subjects or other
cluster internals a browser must not see. This is more conservative than a
successful projection body's `Section.Reason`, which is hand-authored G2
text and is passed through verbatim — see the wire representation below.

## Wire representation

`internal/projection`'s structs carry no JSON tags and are not marshaled
directly: doing so would flatten `Enforcement`/`BehavioralVerification`
(embedded `Section`, no own fields) into the top-level object by accident,
and would tie the wire format to certified G2 internals. Instead,
`cmd/landlock-genprof/workbench_server.go` defines explicit `dto*` types and
one hand-written conversion function per domain type
(`dtoFromDeclared`, `dtoFromRuntime`, `dtoFromGovernance`, ...). Every field
is a direct copy — `State`, `Reason`, association verdicts, exclusion
reasons, approval-binding validity — nothing is computed, summarized, or
reinterpreted. `TestProjectionDTO_*` proves specific semantic properties
(EMPTY/AVAILABLE/UNKNOWN survive; a mixed attributed+excluded population
keeps both sides; `Enforcement`/`BehavioralVerification` stay nested), not
JSON snapshot equality.

## Server lifecycle

`net.Listen` first, so a bind failure is reported before anything else
starts; `Serve` runs in its own goroutine; the main goroutine selects
between a `Serve` error and `ctx.Done()`. On cancellation, `Shutdown` is
given a bounded 5s window to drain in-flight requests before the process
exits regardless. `ReadHeaderTimeout` (5s), `ReadTimeout` (10s),
`WriteTimeout` (15s), `IdleTimeout` (60s), and `MaxHeaderBytes` (64 KiB) are
all explicit and non-zero — bounding Slowloris-style slow-header and
slow-body attacks and idle-connection accumulation.

## v0.4 snapshot → v0.5 live-read transition

The v0.4 contract — "fixed at startup, refreshed only on restart" — is
intentionally retired by this decision, exactly as #185 requires. The
route's own disclosure text changed accordingly ("live read performed at
...; reload to read the cluster again"), and the certification test that
asserted the old fixed-snapshot behavior was replaced with
`TestWorkbenchE2E_LiveReadObservesChangeWithoutRestart`, which proves a
request-triggered fresh read observes a legitimate approval recorded
against the *same running process*, with no restart. This proves
request-triggered live reads. It does not prove, and does not use, watch
semantics — no informer or watch was introduced.

## Residual risks / non-goals

- **G3 introduces no authentication.** The guarantee is not "only the
  legitimate user can access the API." Any local process — legitimate tool,
  malware, or another user's process on a shared machine — can already
  connect to `127.0.0.1:<port>` and forge every header this server
  validates, including `Host`. `READ_ONLY != PUBLIC`, and this decision
  never claims otherwise.
- The two G2-documented defensive NetworkPolicy information-loss paths
  (malformed `podSelector`; malformed `ingress`/`egress`/`policyTypes`
  shapes) are unchanged and remain unreachable through a conformant
  Kubernetes API server. G3 does not touch NetworkPolicy parsing.
- No TrainingHistory/evidence loader exists yet; `Runtime` is always
  `EMPTY` in this build. This is disclosed, not silently absent.

## Claim boundary

G3 proves an HTTP trust boundary and a bounded live-read path. It proves
nothing else. Unchanged: `LANDLOCK_KERNEL_DENIAL` remains `NOT_PROVEN`;
`APPLY_TRANSACTIONALITY` remains `NONTRANSACTIONAL`; NetworkPolicy
behavioral-denial evidence remains Cilium-bounded; PodLock +
application-derived Seccomp same-Pod compatibility remains `NOT_PROVEN` at
every level, and the production composition guard that refuses that
composition (`validateCompositionCompatibility`) is untouched — its
fail-closed refusal remains `PROVEN`. G3 does not convert projection into
enforcement proof, application into behavioral verification, evidence into
policy, coverage into confidence, or approval into application.
