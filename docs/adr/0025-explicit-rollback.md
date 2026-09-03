# ADR-0025: Explicit rollback with strict custody

## Status

Accepted for v0.5.2 implementation.

## Decision

Rollback is an explicit CLI-only operation over a current-epoch-qualified
`ApplyAttempt`. It creates a separate namespaced `RollbackAttempt`; an
`ApplyAttempt` is never rewritten and a `RollbackAttempt` cannot be used as a
rollback source.

Before each inverse mutation, the rollback path persists rollback intent and
the current controlled Before state, then performs the inverse operation. The
operation is dependency-aware and normally reverses successful apply records
in reverse order. It is nontransactional: partial, failed, and unknown
outcomes remain durable and there is no automatic continuation.

## Qualification and integrity

The ApplyAttempt CRD enforces immutable Spec fields. Terminal Status is
immutable once terminal; transition into a terminal state remains allowed.
RollbackAttempt applies the same API-enforced rules.

After the hardened CRD has been installed and empirically probed, an
administrator publishes a fresh, opaque, random custody epoch in the CRD
annotation `landlockgenprof.io/custody-epoch`. New ApplyAttempts copy that
epoch. Missing, stale, or mismatched epochs are refused. The epoch is a
qualification marker, not a secret, signature, credential, or authorship
proof. It prevents retroactive qualification of historical attempts.

For each CREATE/UPDATE, the API response UID/resourceVersion must match an
immediate GET. Only that exact resourceVersion is stored as
`AttributableAfterRV`. A mismatch leaves the mutation non-rollbackable.

Rollback uses strict UID + resourceVersion + controlled-state guards. Updates
use the fresh resourceVersion and do not retry conflicts. Created-object
deletion uses UID and resourceVersion server-side preconditions and is marked
successful only after confirmed absence.

## Resource and dependency boundary

Workload rollback restores only landlock-genprof-controlled fields. Policy
rollback restores controlled whole specs where that is the existing custody
model. A policy is not deleted while a relevant live workload still refers to
it. Restoring an enforcement reference requires the existing readiness gate.
The existing same-Pod PodLock plus application-derived Seccomp guard is run
before every inverse workload mutation.

Bare-Pod DELETE_THEN_CREATE records are structurally refused. Rollback does
not claim kernel, CNI, external-backend, or generic Kubernetes restoration.

## Authority and non-goals

Rollback requires the invoking Kubernetes credential's exact target-resource
RBAC plus explicit CLI confirmation. This is not a new proposal approval and
does not make ApplyAttempt an authorization source. Custody permissions are
separate from target mutation permissions; no long-running tracer receives
new target delete authority.

There is no browser rollback or browser mutation, automatic recovery,
compensation, transaction, atomic rollback, exactly-once execution, garbage
collection, or rollback-of-rollback.
