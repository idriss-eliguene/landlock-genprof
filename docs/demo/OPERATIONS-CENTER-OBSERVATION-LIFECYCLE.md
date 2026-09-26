# Operations Center: authorized Observation lifecycle

This short demo uses the repository's disposable Operations Center environment
and real Kubernetes workloads, namespace authorization, and Inspektor Gadget
recording. It does not seed Observation events or claim that a capture covers
Pods that were not actually resolved and recorded.

## Start the demo environment

On a machine where the supported Core environment is already available, run:

```sh
make operations-center-demo
```

Open the canonical React URL printed by the command. The demo provisions its
own labeled namespaces and personas; do not run the reset target if you need
to preserve those demo resources.

## Walk through one capture

1. In Operational Context, choose the demo identity `developer` and bind the
   `payments` namespace. Open Workloads and select the `api` Deployment's
   concrete Pod/container dossier.
2. Open Observations. Check that `observation.view` and
   `observation.operate` are allowed, the workload/Pod UIDs and runtime image
   identity are present, and Observation history is complete. If a prerequisite
   is unavailable, use the displayed reason and refresh or correct the context;
   the UI will not submit a capture with an uncertain target snapshot.
3. Choose evidence sources and a bounded duration, then start the capture.
   Follow the authoritative REQUESTED → STARTING → RUNNING lifecycle. Exercise
   the selected workload during the window; the demo API workload periodically
   writes its heartbeat file. Stop the Observation when you have enough sample
   activity, or let its requested window finish.
4. Select the completed Observation and inspect its qualification state,
   captured facts, source attachment/flush/attribution evidence, selected Pod
   anchor, resolved Pod UID/container ID/image digest, and any recorded target
   changes. A FAILED or UNKNOWN result is not complete evidence; inspect its
   stage, code, and reason before deciding what to do next.
5. Use Generate proposal only when the existing authorization allows it and
   the Observation is a completed, frozen result. The generated proposal is
   the normal unapproved candidate workflow; review and approval remain
   separate actions with their existing authority boundaries.

## What this demonstrates—and what it does not

The server authorizes each request in the bound namespace. Start binds the
client's selected workload and Pod snapshot to the live Kubernetes identity;
the executor independently records concrete targets and fails visibly when
the selected anchor was replaced before capture. The resulting Observation
may still have partial coverage: inspect the persisted source proof and each
resolved target rather than treating a successful start or stop as proof of
complete evidence.

This is behavioral observation evidence for the recorded window and resolved
targets. It is not proof that a generated policy is safe, that a later workload
revision behaves identically, or that every replica was observed. Proposal
review, approval, application, and bounded Seccomp verification are separate
Operations Center workflows.
