# ADR-0017: Revalidation, Revocation, Drift, and Runtime Remediation

Status: Proposed

Date: 2026-08-23

## Decision question

How does landlock-genprof preserve immutable historical eligibility and
authorization while determining whether a physically realized policy remains
currently authoritative, and how may it respond to expiry, revocation,
unknown facts, drift, and runtime replacement?

RFC-0003 and ADR-0010 through ADR-0016 remain authoritative. This ADR owns
currentness, revalidation, invalidation, and remediation authority. ADR-0015
remains the sole per-mutation enforcement gate; ADR-0016 owns backend
observations and mutation mechanics.

## Selected architecture

Use immutable historical records plus an explicit current-authority
projection. Revalidation consumes explicit snapshots and current facts and
returns a closed result: `CURRENT_VALID`, `CURRENT_INVALID`, or
`CURRENT_UNKNOWN`. It never mutates historical EligibilityRecord,
AuthorizationRecord, or ApplyPlan objects.

The runtime state is orthogonal to semantic authority:

`ACTIVE` means the target is physically active and current authority is valid.
`ACTIVE_UNAUTHORIZED` means the target remains physically active but current
authorization is known invalid. `SUSPENDED_UNKNOWN` means physical state may
remain present but mandatory authority/currentness is unknown. `NOT_ACTIVE`
means active realization is not established. Partial realization is never
complete Active.

## Revalidation boundary

The pure/current operation receives:

- current BoundCandidateDigest;
- exact current EligibilityRecord and AuthorizationRecord identities;
- CurrentAuthorityFacts;
- CurrentSecurityContext;
- current RootTrustConfiguration identity;
- backend realization and runtime Active facts;
- explicit EvaluationTime.

No ambient lookup, clock, cache, event ordering, or controller state is used
inside semantic revalidation. ADR-0016 observation adapters produce the
inputs. Known negative facts dominate unknown facts. `CURRENT_VALID` retains
authorization applicability; `CURRENT_INVALID` removes it; `CURRENT_UNKNOWN`
never permits a new security mutation.

## Triggers and time

Triggers are semantic classes, not controller mechanisms: validity or
freshness expiry; candidate, image, context, kernel, runtime, or feature
changes; TrustPolicy/root or AuthorityRule changes; provenance,
certification, verifier, baseline, or compatibility invalidation; backend
drift, realization loss, target replacement, and availability loss.
Triggers cause revalidation but do not themselves decide authority.

The inclusive ADR-0013 boundaries apply:

`EvaluationTime <= validUntil`

and

`EvaluationTime <= observedAt + maxAge`.

Past-deadline records are mechanically stale even if no controller has run.
Observation failure is `UNKNOWN`, never unchanged or positive.

## Supersession and revocation

Eligibility, authorization, current-fact, and realization records are
selected only by exact semantic lineage, epoch, and supersession identity.
List order, timestamps, Kubernetes resourceVersion, cache order, and latest
object discovery are not currentness rules.

Revocation is `REVOKED`, `NOT_REVOKED`, or `UNKNOWN`, bound to its source and
epoch. Revocation is monotonic within an epoch unless the registered authority
semantics explicitly permit reinstatement. `REVOKED` is known invalid;
`UNKNOWN` fails closed for new mutation. Historical authorization remains
historically true.

## Current projection

The current projection binds BoundCandidateDigest, EligibilityRecord and
AuthorizationRecord identities, context fingerprint, root/trust identity,
current-fact epoch, evaluation time, next deadline, semantic currentness,
runtime realization state, and deterministic invalidation reasons. It is a
projection, not an authority source; a bare `Active=True` or `Eligible=True`
is insufficient.

Reasons include candidate/context/image change, rule or trust revocation,
baseline/provenance invalidation, stale facts, backend drift, realization
loss, runtime replacement, capability disablement, expiry, and unknown state.
Persisted reason sets use canonical predicate/reason ordering where semantic.

## Runtime and backend composition

Each mandatory backend contributes independent current predicates. All
mandatory predicates positive yields `ACTIVE`. A known mandatory failure
produces the registered degraded or unauthorized state. A mandatory unknown
produces `SUSPENDED_UNKNOWN`. For example, Seccomp Active, PodLock Active,
NetworkPolicy Unknown, and capabilities Active cannot aggregate to complete
Active.

Backend disappearance distinguishes `MATERIALIZATION_LOST`, `ACTIVE_LOST`,
and `ACTIVE_UNKNOWN` according to ADR-0016 realization criteria. A new Pod,
container, node, or runtime instance requires fresh target-bound authority;
old Active evidence never applies automatically to the replacement.

## Drift and event loss

Desired drift, runtime drift, and authority drift are distinct current facts.
ADR-0016 detects and canonicalizes them; this ADR interprets them. Correctness
does not depend on every watch or webhook event: resync, reread, or equivalent
mechanisms must discover missed changes before a security-significant
mutation. Runtime status may be stale between checks but cannot be presented
as current after its deadline.

The required reconciliation sequence is:

`observe drift → current fact → revalidate → current authority decision →
remediation decision → ADR-0015-gated mutation`.

Desired-state mismatch alone never authorizes repair.

## Remediation authority

Remediation is not an automatic inverse of revocation. Use a distinct,
immutable `RemediationPolicy`/`RemediationAuthorization` where remediation is
needed. It binds policy identity/version/digest, issuer/trust, trigger class,
backend and target scope, permitted operations, maximum privilege effect,
validity, revocation, and provenance. Controllers may not invent a default
cleanup policy.

`CurrentAuthorityState + RemediationPolicy + CurrentBackendState` produces a
deterministic RemediationPlan. Each security-significant remediation step,
including deletion, detachment, replacement, quarantine, restart, or
compensation, crosses ADR-0015. Removing a restrictive NetworkPolicy,
Seccomp policy, or PodLock profile is not automatically safe and never follows
from revocation alone.

Emergency actions such as blocking restart, quarantine, termination, or
leaving a restrictive policy installed are permitted only when explicitly
covered by remediation policy. Security fail-closed means no privilege
widening without authority; it does not mandate one universal availability
outcome.

Remediation states are separate from authority: `REMEDIATION_PLANNED`,
`REMEDIATION_IN_PROGRESS`, `REMEDIATION_SUCCEEDED`,
`REMEDIATION_FAILED`, and `REMEDIATION_UNKNOWN`. Partial remediation follows
ADR-0015 partial-application rules; there is no implicit rollback.

## Required transitions

When an active policy is revoked, the state becomes `ACTIVE_UNAUTHORIZED`,
not `NOT_ACTIVE`, until physical realization changes. When mandatory facts
become unavailable it becomes `SUSPENDED_UNKNOWN`. Existing physical
restrictions are not removed merely because current authority is unknown or
invalid. A new or replacement workload attachment while unknown requires a
new ADR-0015 `PERMITTED` result and therefore cannot proceed on uncertainty.

Legacy proposals do not receive a modern CurrentAuthorityProjection.
`AUTHORITY_ADVISORY_MODE` may report diagnostics but cannot authorize
enforcement or remediation; only `RFC0003_ENFORCEMENT_MODE` permits modern
ADR-F/H behavior. Disabling that capability blocks new mutations immediately.

## Security invariants

1. Historical eligibility and authorization records are immutable.
2. Current authority is a projection, not record existence.
3. Expired or stale facts cannot remain positive.
4. Known revocation invalidates current authority.
5. Unknown mandatory facts produce unknown currentness.
6. Physical Active does not imply current authorization.
7. `ACTIVE_UNAUTHORIZED` is representable.
8. `SUSPENDED_UNKNOWN` is representable.
9. Revocation does not authorize policy removal.
10. Privilege-widening remediation requires explicit authority.
11. Every remediation mutation crosses ADR-0015.
12. Drift creates facts, not automatic authority.
13. Replacement runtime instances require fresh authority.
14. Old Active evidence cannot apply to a new instance.
15. Partial realization cannot aggregate to complete Active.
16. Observation failure is unknown, not unchanged.
17. Event delivery is not required for correctness.
18. Current status binds exact candidate, records, and context.
19. Stale current projections are mechanically detectable.
20. Historical replay cannot authorize current mutation.
21. Disabling enforcement capability blocks new mutations.
22. Loss of authority cannot automatically widen privilege.
23. Remediation policy is explicit, trusted, bound, and revocable.
24. Reconciliation cannot bypass current authority.

## Ownership and handoffs

The revalidation scheduler triggers checks; fact collectors produce immutable
current facts; the runtime authority projector writes the current projection;
the remediation planner creates plans; and the remediation executor invokes
ADR-0015. Each semantic dimension has one conceptual writer. ADR-G supplies
backend observations, target/pre-state, realization, and mutation results.
ADR-H does not reinterpret backend semantics or replace ADR-F.

## Alternatives

Immediate detach on revocation, controller-specific cleanup heuristics,
boolean current-state flags, and an external policy engine owning lifecycle
semantics were rejected. The selected immutable-history plus current
projection plus explicit remediation-policy model preserves confinement,
auditability, and deterministic currentness.

## Tests

Future tests cover unit current projection, exact time boundaries, revocation,
supersession, multi-backend aggregation, stale/unknown facts, replacement
runtime, stale Active evidence, revoked remediation policies, apply/revocation
and remediation concurrency, status projection, lost events, controller
restart, backend drift, unknown backend state, and E2E remediation. Historical
replay tests must reproduce past knowledge without permitting current mutation.

## Claim boundary

This ADR defines revalidation, currentness, revocation consequences, drift
interpretation, and remediation authority architecture only. It does not
implement controllers, polling, remediation actions, backend probes, storage,
or runtime enforcement.
