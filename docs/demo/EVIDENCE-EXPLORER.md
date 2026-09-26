# Operations Center Evidence Explorer demo

This walkthrough uses the supported Operations Center demo environment and
real namespace-scoped Kubernetes authorization and Inspektor Gadget recording.
It creates demo resources; use only an environment dedicated to this demo.

## Run

Start the project's disposable demo environment and open the canonical React
URL it prints:

```sh
make operations-center-demo
```

1. Bind the `developer` identity to `payments`, open Workloads, and select the
   `api` Deployment's concrete Pod/container.
2. Start an Observation with the desired sources and bounded duration. Let the
   authoritative lifecycle reach a terminal state; generate a proposal only
   from a completed, frozen Observation when the server enables that action.
3. Open Evidence in the Operations Center. Select the Observation, filter its
   normalized facts by source or text, inspect source qualification, runtime
   target identities, and any Pod/workload changes recorded during capture.
4. Review excluded-event reason counts when present. Historical Observations
   that predate reason-summary storage show the excluded count and explicitly
   mark the reason as unavailable rather than guessing.
5. In the proposal section, follow the namespace-authorized lineage to an
   associated Proposal. Inspect each proposed capability's attribution state
   and Observation IDs, then follow an Observation link back to its evidence.
   `UNKNOWN` or partial attribution remains visible and is not upgraded by the
   UI.

The persisted Observation contains bounded, deduplicated normalized facts and
aggregate exclusion-reason counts. It does not retain the raw Gadget event
stream or per-event timestamps/payloads. The Explorer therefore inspects the
evidence the system actually persisted; it does not reconstruct raw events,
prove causality, claim complete workload behavior, or claim a capability is
safe. Candidate contents, candidate digests, and approval authority are not
changed by viewing evidence.
