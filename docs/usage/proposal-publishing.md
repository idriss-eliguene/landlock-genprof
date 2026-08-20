# Proposal publishing (mandatory)

Every `trace` run publishes its generated multi-domain profile as a
`SecurityProfileProposal` custom resource — stored as a cluster object
instead of only local files, reviewable via `kubectl`/GitOps. This isn't
an opt-in flag: it's the primary reviewable artifact this tool produces,
so a run fails outright if it can't publish (missing CRD or RBAC below)
rather than silently degrading to local files only. See
[`../../examples/nginx-generated-proposal.yaml`](../../examples/nginx-generated-proposal.yaml)
for a complete example.

```bash
kubectl get securityprofileproposal nginx-demo -o yaml

# Product-facing review summary (kubectl-plugin form — swap for
# `go run ./cmd/landlock-genprof review` from a source checkout, see
# ../../INSTALL.md)
kubectl landlock-genprof review nginx-demo
```

Each field is the **exact rendered content** of the corresponding local
file — `spec.podLock` is the full, real `profile.yaml`
(`apiVersion`/`kind`/`metadata`/`spec` included), `spec.networkPolicy`
the full `networkpolicy.yaml`, `spec.patchedManifest` the full
`<identity>-patched.yaml` — the live owner's (or
bare pod's) complete manifest with the generated `securityContext`
already merged in, not the bare fragment `--security-context-out`
produces, `spec.spoSeccompProfile` the full `<pod>-seccompprofile.yaml` — a security-profiles-operator SeccompProfile
custom resource, the sole seccomp-related field (its own `spec.syscalls`
already carries the same data a raw `spec.seccomp` field would, so
there's no separate copy to keep in sync).

These fields are proposal content, not independently authorized
artifacts. Do not extract them and run `kubectl apply -f -` for a governed
rollout. The authoritative workflow is:

```bash
kubectl landlock-genprof review nginx-demo --namespace default
# Use the exact Candidate digest printed by review:
kubectl landlock-genprof approve nginx-demo --namespace default \
  --expected-digest sha256:<candidate-digest-from-review>
kubectl landlock-genprof apply-proposal nginx-demo --namespace default
```

`make export-proposal PROPOSAL=<name>` remains available for
**NON-AUTHORITATIVE INSPECTION/DEBUG ONLY**. Its output is a mutable
snapshot of `proposal.spec`; it does not retain approval authority and
does not substitute for `approve` plus `apply-proposal`.

`spec.patchedManifest`'s `securityContext.seccompProfile.localhostProfile`
always references the **governed** profile's own
`operator/<governed-name>.json` path whenever `spec.spoSeccompProfile` is
non-empty — never the name of any externally-generated profile, so the
approved artifact and the reference to it are bound by one digest
(docs/adr/0008) — see the [SeccompProfile resource page](seccompprofile-resource.md)
for why a plain filename isn't enough and what applying
`spec.spoSeccompProfile` actually does.

`TrainingHistory` is the direct-evidence stage and
`SecurityProfileProposal` is the proposal and approval-status stage — both
are plain CRUD, with no controller. Continuous reconciliation remains future
roadmap work. The object's name is the target pod
(overwritten on every re-run, not accumulated — a proposal is the
*latest* recommendation, same as the local files). Requires the CRD and
additional RBAC, applied once:
[`../../deploy/crd-securityprofileproposal.yaml`](../../deploy/crd-securityprofileproposal.yaml),
[`../../deploy/rbac-proposal.yaml`](../../deploy/rbac-proposal.yaml).

## Approval status (`status.approvalState`)

Every proposal carries a lifecycle separate from the generated content
above — `Draft` (set once, when `trace` first publishes it) →
`Reviewed` (set automatically the first time `kubectl landlock-genprof
review` runs against it) → `Approved`/`Rejected` (only ever set by an
explicit human decision, never inferred):

```bash
kubectl landlock-genprof approve nginx-demo \
  --expected-digest sha256:<candidate-digest-from-review> \
  --reason "reviewed with the platform team"
kubectl landlock-genprof reject nginx-demo --reason "syscalls list looks too broad"
```

Approval is authoritative for governed application: `approve` persists
`approvalState=Approved`, the reviewed `approvedCandidateDigest`, and
`approvalMechanismVersion=candidate-v1`. `apply-proposal` validates that
binding and fails closed when approval is missing, malformed, stale, or
replaced. Its confirmation prompt is an additional operator confirmation,
not a substitute for digest-bound approval.

Stored via the CRD's `status` subresource specifically so a re-run of
`trace` against the same pod (which overwrites `.spec` in full, see
above) can never silently wipe an approval decision — `.status` is a
different write path entirely. This means `approve`/`reject`/`review`'s
`MarkReviewed` all need `securityprofileproposals/status` write access
in addition to the base resource, on top of whatever RBAC the *invoking
user's own* `kubectl` identity already has — same "runs under your own
RBAC, not the tracer's ServiceAccount" pattern `apply-proposal` uses
(see [`patched-manifest.md`](patched-manifest.md)). `deploy/rbac-proposal.yaml`
only grants this to the tracer's own ServiceAccount (needed to stamp the
initial `Draft` state on `Create`) — grant your own identity the matching
permission separately if `review`/`approve`/`reject` report a permissions
error setting status.
