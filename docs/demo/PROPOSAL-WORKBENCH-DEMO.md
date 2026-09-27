# Guided Proposal Workbench demo

This is a reproducible walkthrough for an Operations Center demo environment.
It is a procedure, not a claim that a live browser-to-Gadget run has been
performed. Run only in a disposable environment intended for the demo.

1. Start the demo with `make operations-center-demo` and open its printed URL.
2. Bind the `developer` identity to `payments`, open the `api` workload, and
   start a bounded Observation for its selected Pod/container.
3. Wait for the authoritative lifecycle to complete. In Observations, inspect
   the Proposal Workbench's workload UID, immutable image digest, source states,
   attributed/excluded counts, and any incomplete coverage. Use **Inspect
   evidence** to review normalized facts and exclusion explanations.
4. Return to the Observation. Generate a draft only when the UI confirms the
   current workload/Pod selection, workload UID, and image digest still match.
   The server independently re-resolves those identities before deriving the
   candidate.
5. Open the generated Proposal. Inspect candidate and review-context digests,
   capability attribution, and the source Observation. Switch to
   `security-reviewer`, bind `payments`, and use its demo-granted review and
   approval actions. Application remains a separate governed action and is not
   part of this walkthrough. Production deployments should keep reviewer and
   approver authority separated according to their own RBAC policy.

If no capability candidate can be derived, the Workbench keeps the Observation
visible and reports the backend's failure; it does not create an empty or
fabricated Proposal. UNKNOWN source qualification and historical attribution
remain visible. A generated candidate is never approved automatically.

The UI uses the existing Observation generation and governance APIs. The
backend binds generation to the currently resolved Pod, workload UID, namespace,
container and immutable image digest, and rejects stale or mismatched evidence.
This workflow does not prove causality, complete workload behavior, or policy
enforcement. Fixture and envtest results must not be presented as live runtime
evidence.
