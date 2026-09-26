# ADR-0037: Bounded SPO Seccomp verification with twin Pods

## Status

Proposed

## Decision

An explicitly authorized verification operation may create a short-lived twin
Pod and RuntimeDefault control Pod in a dedicated verifier namespace. Both use
the same digest-pinned, repository-built `seccomp-verifier-probe` image, node,
and runtime class. The twin declares the approved SPO Localhost profile; the
control declares `RuntimeDefault`. The probe performs only
`getpriority(PRIO_PROCESS, 0)` and reports success (exit 0), observed `EPERM`
(exit 10), or inconclusive execution (exit 20). The Pod termination status is
the result channel; no command input, shell, exec, ephemeral-container, or log
access is needed.

The verifier identity is separate from human reviewer identities. It receives
only namespace-scoped Pod create/get/delete permissions in the verifier
namespace. Probe Pods use a dedicated ServiceAccount with token mounting
disabled. Human invocation requires a separate `proposal.verify` authorization;
review or approval permission alone is insufficient.

## Claims and evidence boundaries

The durable experiment fact binds its result to the approved candidate digest,
SPO profile object UID and canonical content digest, target namespace/workload,
target Pod UID/container ID/node/runtime, twin and control Pod identities,
probe image digest/version, and observation time. Relevant target and profile
identities are re-read before and after the experiment. The CRD content digest
is not represented as a digest of the node-local profile file.

The experiment establishes only that the fixed probe observed the stated
behavior in a twin Pod configured with the selected Localhost profile relative
to a RuntimeDefault control. It does not establish that the original
application process has that filter active. Materialization, target workload
activation, and twin-Pod behavioral verification remain separate facts and
separate projection states. Missing or stale evidence is `UNKNOWN`; a failed
valid probe is `NOT_VERIFIED`. No aggregate state may collapse those facts into
a universal `VERIFIED` claim.

## Consequences and limitations

Verification requires the workload node to have the referenced SPO profile
available and the node to permit scheduling the short-lived verifier Pods.
Probe startup failures and incomplete cleanup do not produce affirmative
evidence. RuntimeDefault is a comparative control, not an unconfined control
and not a universal baseline. The result is time-bounded and does not prove
other syscalls, processes, replicas, or later workload incarnations.
