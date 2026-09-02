# Pilot recovery procedure

This is a founder-assisted procedure for the v0.5.0 pilot package. It is not
automatic rollback, transactional apply, or a break-glass authority path.
landlock-genprof has no durable ApplyAttempt mutation journal in v0.5.0.

## Before apply

Retain the proposal name, candidate digest, approval state, target identity,
exact generated artifacts, original controller/workload manifests, existing
security resources, current pod/controller state, and recovery owner. Record
which resources predate the pilot. Confirm the kubeconfig, permissions, and
customer authorization needed to restore that state. Do not apply without a
recoverable pre-change state.

## Partial application

`apply-proposal` is sequential and fail-stop. On failure, artifacts already
applied remain; later artifacts are not applied and automatic rollback does
not occur. Record the exit status, failing artifact, last successful step,
proposal digest, and live object state in operator-owned change-management
evidence. No product-persisted apply-time recovery evidence is implied. Do
not assume workload binding either occurred or did not occur without
inspection.

If live state cannot be reconciled with the recorded sequence, stop and
escalate to the customer recovery owner before further mutation.

## Recovery steps

1. Stop the apply sequence and preserve logs, proposal state, live descriptions,
   and evidence.
2. If the workload/controller changed, and the customer authorizes restoration,
   restore the retained original manifest with the normal Kubernetes procedure,
   for example `kubectl apply -f <retained-original-manifest>`.
3. Identify whether each applied enforcement resource was created by the pilot
   or replaced a customer resource. Restore a retained customer resource only
   with authorization.
4. Remove a pilot-created temporary resource only after confirming exact
   identity and that no retained workload references it. Do not delete an
   unowned or pre-existing resource.
5. Re-read workload and backend state. Record configured, applied, and
   behaviorally verified states separately.
6. Run only applicable bounded verification. Record `NOT VERIFIED` when the
   backend or environment is outside the evidence boundary.

There is no automated rollback command or `rollback <attempt-id>` command.
Direct `kubectl apply` of an exported artifact is not governed authorization
and must not be used as a bypass. `kubectl rollout undo` may be used only as a
customer-authorized Kubernetes controller operation; it is not a
landlock-genprof rollback mechanism.

## Escalation and completion

Escalate when original state is missing, ownership is ambiguous, a resource
was overwritten, binding may have occurred after failure, recovery permissions
are unavailable, or authorization is revoked. Preserve state; do not invent
an emergency approval.

Recovery is complete only when the customer recovery owner accepts the
restored or intentionally retained state, remaining applied resources are
known, applicable verification is recorded, and the handoff includes the
incident, actions, unresolved items, and limitations.
