# Step 12 — Proposal publishing (mandatory)

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
`<identity>-patched.yaml` (Step 13 below) — the live owner's (or
bare pod's) complete manifest with the generated `securityContext`
already merged in, not the bare fragment `--security-context-out`
produces, `spec.spoSeccompProfile` the full `<pod>-seccompprofile.yaml`
(Step 14 below) — a security-profiles-operator SeccompProfile
custom resource, the sole seccomp-related field (its own `spec.syscalls`
already carries the same data a raw `spec.seccomp` field would, so
there's no separate copy to keep in sync). Copy any of them directly out
of `kubectl get -o yaml` and use as-is (`kubectl apply -f -` for all
four).

`spec.patchedManifest`'s `securityContext.seccompProfile.localhostProfile`
always references SPO's own `operator/<namespace>/<pod>.json` naming
convention whenever `spec.spoSeccompProfile` is non-empty — see Step
14 for why a plain filename isn't enough and what applying
`spec.spoSeccompProfile` actually does.

This is the **first slice of a larger evidence/proposal/approved-policy
model**: `TrainingHistory` (`--history`, Step 7) is the evidence
stage, `SecurityProfileProposal` is the proposal stage — both are plain
CRUD, no controller. An eventual approved-policy stage
(`WorkloadSecurityProfile`) and an enforcement operator to keep it from
drifting are **not** part of this — that's real controller-runtime work,
deliberately out of scope for now. The object's name is the target pod
(overwritten on every re-run, not accumulated — a proposal is the
*latest* recommendation, same as the local files). Requires the CRD and
additional RBAC, applied once:
[`../../deploy/crd-securityprofileproposal.yaml`](../../deploy/crd-securityprofileproposal.yaml),
[`../../deploy/rbac-proposal.yaml`](../../deploy/rbac-proposal.yaml).
