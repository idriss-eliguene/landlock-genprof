## Summary

What does this PR change?

## Problem / Motivation

What problem or missing property does it address?

## Scope

What is included?

## Non-Scope

What is explicitly not addressed?

## Security / Trust Boundary

Does this change trust boundaries, source authority, provenance, evidence,
authorization, digest/identity, lifecycle, or enforcement behavior? Explain,
or write `N/A`.

## Governance Invariants

Address each, or write `N/A`: deterministic candidate identity; approval
binding; stale-authority rejection; fail-closed behavior; zero-mutation
rejection; provenance preservation; evidence versus derived-policy separation.

## Behavior / Semantics

Describe externally observable semantic changes, not only implementation details.

## Testing

List unit, integration, E2E/runtime, and negative/adversarial tests as applicable.

## Evidence

What does the supplied evidence prove, and what does it explicitly not prove?

## Claim Boundary

**Strongest claim established by this PR:**

**This PR does NOT establish:**

## Compatibility

Address applicable kernels, Kubernetes versions, runtimes, CNI/backend, SPO and
PodLock versions, and cross-backend composition. Use `N/A` where appropriate.

## Failure Semantics

For apply/reconcile/runtime changes, describe fail-open versus fail-closed,
partial failure, rollback, retry/idempotency, and mutation-before-failure.

## Documentation / ADR

- [ ] Documentation updated
- [ ] No documentation change required
- [ ] ADR added
- [ ] Existing ADR remains valid
- [ ] New ADR required

## Release Impact

Choose one and explain: patch / minor / major / no release impact / undecided
(maintainer decision required).

## Checklist

- [ ] I reviewed the full diff against the target branch.
- [ ] Tests cover the changed semantics.
- [ ] Negative/security cases are covered where applicable.
- [ ] No unrelated work is included.
- [ ] Documentation matches the strongest supported claim.
- [ ] Experimental behavior is not presented as certified.
- [ ] Observation/coverage was not converted into confidence or authorization without an explicit model change.
- [ ] Any trust-boundary change is documented.
- [ ] Existing normative ADRs remain satisfied or a new ADR is included.
