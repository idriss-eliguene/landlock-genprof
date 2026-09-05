# ADR-0030: Executor claim and lease mechanism

Status: Accepted

Date: 2026-09-05

## Context

Durable observations may be requested concurrently by more than one
executor. Kubernetes already provides conditional object updates through
`resourceVersion`.

## Problem

Concurrent starts must not create two owners, and an executor that disappears
must not leave a durable record claiming successful completion.

## Decision

The minimum v0.7 concurrency mechanism is `executorID`, `claimGeneration`,
`leaseExpiry`, and Kubernetes resourceVersion CAS. The transition
`REQUESTED → STARTING` is one atomic conditional mutation. A losing claimant
observes that the Observation is already claimed. No separate lock service or
Lease object is required.

## Detailed semantics

The claiming executor owns lease renewal. A stale lease never implies
successful completion. Non-terminal state with an expired lease is surfaced as
lost/stale; after bounded recovery/grace semantics, a valid writer may
terminalize execution as `FAILED` with reason `EXECUTOR_LOST`. Failure to
persist authoritative state fails closed rather than allowing execution to
continue indefinitely while the durable record lies.

Every executor-authored write—claim, heartbeat, execution-state update, result
update, completion update, or equivalent executor-owned mutation—MUST verify
that the Observation's current `executorID` and `claimGeneration` match the
writer's recorded claim. A mismatch is refused as a stale-executor write,
independently of whether the writer has a fresh `resourceVersion`.

`resourceVersion` protects optimistic concurrency and detects intervening
mutations. `executorID` plus `claimGeneration` fences stale execution
authority. Both checks are required for executor-authored non-terminal and
terminal writes, as applicable. Terminal state remains immutable.

The lifecycle remains `REQUESTED → STARTING → RUNNING → COMPLETING →
COMPLETED | FAILED`. `UNKNOWN` is evidence knowledge, not an execution state.
The exact heartbeat and grace durations remain implementation-defined.

## Invariants

Claim acquisition is CAS-protected; a stale lease is not success; terminal
state is not silently overwritten; partial evidence is retained independently
of execution outcome.

## Consequences

Kubernetes is the sole concurrency authority for this minimum model. Recovery
must be bounded and auditable, and CAS conflicts are normal observable
outcomes rather than reasons to bypass conditional updates.

## Rejected alternatives

- Separate distributed lock service.
- Mandatory separate Lease object.
- Treating lease expiry as automatic success or automatic retry.
- Continuing after authoritative persistence failure.

## Compatibility / migration

No shipped controller currently owns Observation execution, so this is a
forward-compatible contract and does not alter existing controller ownership,
ApplyAttempt, or RollbackAttempt semantics.

## Security considerations

CAS and explicit executor ownership prevent duplicate observation authority.
Recovery must not allow an unqualified writer to rewrite another executor's
terminal facts.

## Claim boundary

Exclusive execution ownership is not evidence quality, enforcement, or
exactly-once execution. Apply and rollback remain nontransactional.

## Open implementation details

Heartbeat interval, lease duration, recovery grace, and the authorization of a
recovery writer require implementation review.

## Non-goals

No lock service, consensus system, separate Lease object, or executor code is
implemented here.

## References

- [ADR-0026](0026-observation-identity-and-record-decomposition.md)
- [ADR-0029](0029-observation-persistence-adapter-boundary.md)
