# ADR-0024: Durable ApplyAttempt mutation custody

*Status: Accepted*

## Decision

`apply-proposal` creates a namespaced `ApplyAttempt` before the first
Kubernetes mutation. The governing invariant is: **no governed-target
mutation without durable pre-mutation custody**. The ApplyAttempt Create and
`/status` writes establish that custody and are intentionally outside this
invariant. For each sequential mutation it records the proposal UID,
approved candidate digest, canonical target, live resource identity and
resourceVersion, the relevant pre-state, and the intended state before
dispatching the mutation. It then records the observed state and typed result
when that outcome can be established.

The ApplyAttempt is custody and evidence, not authorization. Authorization
continues to come from the existing proposal approval, digest, and canonical
target binding. Each execution receives a distinct attempt name and attempts
are retained indefinitely/manual; there is no automatic garbage collection.

## Lifecycle and uncertainty

The lifecycle states are `IN_PROGRESS`, `APPLIED`, `PARTIALLY_APPLIED`,
`FAILED`, and `OUTCOME_UNKNOWN`. `OUTCOME_UNKNOWN` is used when a mutation or
its durable observed result cannot safely be determined. A known failed
mutation is not treated as a successful or partial mutation; partial
application requires a previously known successful mutation.

There remains an unavoidable boundary between durable intent and a remote API
request. A process can terminate after intent persistence and before, during,
or after dispatch. ApplyAttempt records the strongest state established by
the surviving process and does not claim perfect crash atomicity.

## Scope of snapshots

LandlockProfile, NetworkPolicy, and SPO SeccompProfile custody retains the
resource identity and whole policy `spec`. Deployment, StatefulSet, and
DaemonSet custody is field-scoped to the pod-template label and named
container `securityContext.capabilities` / `securityContext.seccompProfile`
fields that the existing patch path controls. This avoids treating a stale
whole workload object as a future restoration payload. Bare-Pod apply remains
excluded from the v0.5.0 controlled pilot; its existing delete-then-create
behavior is not made transactional here.

UID identifies the Kubernetes object, resourceVersion guards optimistic
concurrency, and the field-scoped `BeforeDigest` supports post-hoc outcome
disambiguation. The candidate digest remains authorization/proposal identity;
it is not a digest of live controlled-resource semantic state. These values
are not interchangeable.

## What this does not decide

v0.5.1 does not make apply transactional, atomic across resources, or
rollback-safe. It provides no automatic rollback, compensation, retry, or
ApplyAttempt deletion. A future v0.5.2 rollback operation must explicitly
interpret `Before`, `IntendedAfter`, and `ObservedAfter`; it must not treat an
ApplyAttempt as proof that rollback is safe or as a new authorization source.

The existing composition guard remains before mutation. Same-Pod PodLock plus
application-derived Seccomp compatibility remains unproven and fail-closed;
SPO output remains derived policy; NetworkPolicy behavioral claims remain
Cilium-bounded; and Landlock kernel denial remains unproven.

## Authority and RBAC

The application still uses the invoking Kubernetes credential. The new
ApplyAttempt writer permission is bounded to create/get on ApplyAttempt and
get/update/patch on its status, with no delete permission. This does not
create a separate Workbench reader credential or browser mutation path.
