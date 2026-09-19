# ADR-0035: SPHM telemetry, temporal semantics and qualified signal architecture

## Status

Accepted for M9 architecture and future implementation planning. The custom
metrics engine and providers described here are not implemented by this ADR.

## Context

The Operations Center evaluates a bounded SPHM v1 projection from authoritative
Observation, Evidence, Proposal, EnvironmentSession, History, and Attention
read models. It distinguishes missing proof from missing observability, but
does not yet provide a general temporal evidence contract or provider-neutral
telemetry pipeline.

The system must not claim evidence is healthy because a browser refreshed, an
HTTP request succeeded, or a metric has a numeric value. It must remain safe
under namespace/session isolation, preserve provenance, and avoid turning an
unavailable denominator or threshold into a fabricated metric.

## Problem

Future SPHM dimensions such as Coverage, Freshness, Drift, and Enforcement may
need signals from Kubernetes, runtime security, native landlock-genprof events,
Prometheus, OpenTelemetry, and custom adapters. Without an explicit contract,
providers could bypass qualification, lose workload/profile identity, create
unbounded cardinality, or make the browser an accidental source of security
truth.

## Decision

Future telemetry will enter SPHM through a server-owned Qualified Signal Model.
Providers may collect candidate observations, but only qualified signals with
explicit provenance, completeness, temporal metadata, identity binding, and
qualification state may contribute to an established SPHM result. SPHM owns
state semantics and deterministic aggregation. The Operations Center consumes a
current, context-bound server projection; browser polling is never the source
of truth.

The delivery architecture is hybrid: event-driven ingestion where a source
supports it, plus bounded periodic reconciliation as the correctness and
recovery safety net. No provider, stream, exporter, or custom metrics engine
is introduced by M9.

## Temporal model

Signals use this temporal chain when the source can establish each point:

```text
ObservedAt -> CollectedAt -> QualifiedAt -> ProjectedAt
```

`ObservedAt` is when the phenomenon occurred. `CollectedAt` is when a collector
received it. `QualifiedAt` is when attribution and qualification established
what it means. `ProjectedAt` is when the server-owned SPHM read model
incorporated the qualified state. A missing timestamp remains missing; browser
fetch time is never substituted.

Freshness is a domain relationship between authoritative evidence age and an
explicit configured policy. A successful `GET /api/health` updates neither
`ObservedAt` nor evidence freshness. Until an evidence-age contract and policy
exist, Freshness remains `NOT_ESTABLISHED`.

## Qualified Signal Model

The future conceptual contract is:

```text
MetricObservation
  MetricID, Source, Value, Unit
  ObservedAt, CollectedAt, QualifiedAt
  WorkloadIdentity, Namespace, ProfileIdentity
  Provenance, CollectionState, QualificationState
  Completeness, Freshness
```

This is a design contract, not a production type added by M9. A value is not
proof: `Value != Qualification`, and neither a numeric value nor successful
collection establishes `HEALTHY` without qualification and completeness
evidence.

## Providers and SPHM integration

Future providers may include Kubernetes watches/resources, landlock-genprof
native and Observation lifecycle events, runtime security telemetry,
Prometheus, OpenTelemetry, and explicitly reviewed custom adapters. Providers
normalize into the Qualified Signal Model, then SPHM evaluates the signals.
Providers do not assign SPHM state and the frontend does not aggregate health.
Future exports are projections of established server state, not alternative
authority paths.

## Failure, stale, and unknown semantics

Collection failure, qualification failure, incomplete data, stale signals,
and unavailable model support remain distinguishable. Missing data never
becomes `HEALTHY`; stale evidence never becomes fresh because it was projected;
an unknown signal is not a zero-valued signal. Provider failure may produce an
operational Attention item, but it cannot fabricate successful security proof.

## Security and context boundaries

Every signal and projection is bound to authenticated authority, cluster,
namespace, EnvironmentSession, context version, and exact workload/profile
identity where applicable. A browser-selected namespace is not authority.
Provider credentials, kubeconfig, bearer tokens, and unrestricted Kubernetes
clients remain backend-only. Cross-namespace aggregation requires an explicit
server authorization contract; the default projection is namespace-scoped.
Candidate digest, provenance, Observation attribution, governance CAS, and
fail-closed behavior are unchanged.

## Cardinality, backpressure, and collection limits

Providers must declare identity dimensions and cardinality budgets. Unbounded
labels such as raw command lines, arbitrary paths, or user-provided strings
must not become metric labels. Collection must apply bounded queues,
backpressure, source-specific limits, and explicit drop/overflow accounting.
Dropping a signal is not silent success; qualification must preserve
uncertainty and provenance.

## Event-driven and reconciliation model

```text
source events -> provider boundary -> Qualified Signal Model -> SPHM -> /api/health
                         ^
                         |
                periodic reconciliation
```

Reconciliation re-reads authoritative source state, repairs missed events,
and supplies recovery after provider restart. The browser remains a consumer
and must not cause expensive source recomputation through arbitrary polling.

## Rejected alternatives

- Browser polling as source of truth: refresh time is not evidence time.
- One fixed refresh interval as freshness policy: resource classes differ.
- Metric value without provenance or qualification: quantity is not proof.
- Missing data implies healthy: it hides uncertainty and authority failure.
- Arbitrary weighted health score: epistemic states and hard invariants cannot
  be averaged safely.
- Frontend-derived health semantics: it duplicates and can contradict server
  authority.
- Direct arbitrary PromQL producing `SPHM=HEALTHY`: external metrics require
  qualification, identity, provenance, and completeness.

## Consequences

SPHM can later incorporate telemetry without moving security truth into React
or weakening evidence semantics. The cost is explicit schema/versioning work,
provider qualification, cardinality control, and reconciliation design. Until
those contracts exist, Coverage, Freshness, Drift, and Enforcement remain
`NOT_ESTABLISHED` where appropriate.

## Implementation phases

1. Temporal foundation: persist/project `ObservedAt`, `CollectedAt`,
   `QualifiedAt`, `ProjectedAt`, with explicit freshness policy.
2. Qualified Signal Model: provenance, completeness, qualification, and exact
   identity binding.
3. Native collectors: Kubernetes, landlock-genprof runtime, and authoritative
   enforcement telemetry.
4. External providers: Prometheus, OpenTelemetry, and reviewed custom
   adapters.
5. Advanced SPHM dimensions: Coverage, Freshness, Drift, and Enforcement.
6. SPHM self-observability: collection/qualification/projection lag, collector
   health, and stale-signal inventory.
7. Ecosystem integration: Prometheus/OTel export and downstream integrations.

## References

- [SPHM v1 and frontend migration contract](../product/operations-center/frontend-migration-v1.md)
- [Operations Center namespace authorization](0034-operations-center-namespace-authorization.md)
- [Evidence provenance model](0005-evidence-provenance-model.md)
- [Bounded Workbench Kubernetes read authority](0021-bounded-workbench-kubernetes-read-authority.md)
