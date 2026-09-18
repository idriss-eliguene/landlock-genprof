# Security Profile Health Model v1

SPHM v1 is the Operations Center's explicit health model. It is a projection
of authoritative, namespace-scoped product data; it is not an industry
standard and it does not invent a security score.

## State vocabulary

* `HEALTHY`: the dimension's established checks passed.
* `ATTENTION`: an established operational condition needs investigation.
* `CRITICAL`: reserved for an established hard-invariant violation. The v1
  projection does not synthesize one from missing evidence.
* `UNKNOWN`: the dimension applies, but required proof is missing or
  inconclusive. Terminal Observation evidence with `FlushConfirmed=false` is
  the canonical example.
* `NOT_ESTABLISHED`: the current product does not own the denominator,
  threshold, or enforcement signal needed to make the claim.
* `NOT_APPLICABLE`: the dimension does not apply to the selected object.

## Dimensions and aggregation

v1 exposes Authority, Coverage, Evidence qualification, Freshness, Drift,
Governance, Observation pipeline, and Enforcement visibility. There is no
weighted overall score. Aggregation is deterministic: an established hard
invariant violation would be `CRITICAL`; otherwise evidence proof gaps are
`UNKNOWN`; otherwise established operational conditions are `ATTENTION`;
otherwise the report is `HEALTHY`. Unsupported dimensions remain
`NOT_ESTABLISHED` and are never treated as healthy.

The Health API is evaluated against the request's authenticated
EnvironmentSession/read capability and returns the cluster identity, namespace,
and context version used for the report. It does not change authorization or
create a global selected resource.

## Authoritative metric contract

| Metric | Source | Definition | v1 limitation |
| --- | --- | --- | --- |
| Authority | EnvironmentSession/read session | Bound context used for the read | A server read cannot prove enforcement |
| Coverage | none | Eligible workload denominator | `NOT_ESTABLISHED`; no canonical eligible population |
| Evidence qualification | Observation read model | Completed/frozen evidence states | `UNKNOWN` preserves missing proof; facts are not confidence |
| Freshness | Observation/Proposal timestamps | Age is available in source objects | No configured freshness threshold |
| Drift | none | Governed versus enforced comparison | `NOT_ESTABLISHED`; no enforcement read model |
| Governance | SecurityProfileProposal read model | Draft/Reviewed pending count | No invented backlog threshold |
| Pipeline | Observation read model | Failed and terminal observation counts | Rates/window policy are not configured |
| Enforcement visibility | none | Current enforced profile state | `NOT_ESTABLISHED` |

## Drill-down and temporal semantics

Every actionable Observation and proposal item includes its exact identity and
workload. The UI links back to the existing exact-object surfaces; it does not
match by latest item or collection order. v1 reports current state and source
timestamps only. Historical trend series are `NOT_ESTABLISHED` because the
retained model does not provide an authoritative time-series contract.

## Non-goals

SPHM v1 does not claim profile enforcement, invent eligible-workload coverage,
translate `UNKNOWN` into failure, define arbitrary age thresholds, or expose
credentials. Future versions may add these only when authoritative product
contracts exist.
