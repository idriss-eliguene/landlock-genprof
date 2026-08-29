# Pilot data and evidence handling

This procedure covers founder-assisted handling when the workflow produces
local artifacts or the operator exports evidence for customer review.

## Data that may be processed

Depending on selected commands and integrations, the package may include
Kubernetes metadata and identifiers; filesystem paths; network endpoints;
process and syscall observations; raw events; generated Landlock,
NetworkPolicy, seccomp, capabilities, securityContext, patched-manifest, and
report artifacts; proposal identity and candidate digest; approval,
provenance, coverage, application, verification, logs, and diagnostics.

SPO syscall material imported by landlock-genprof is derived policy with
provenance, not direct landlock-genprof runtime evidence. Coverage is not
confidence, authorization, or proof that an unobserved item is unnecessary.

## Product behavior versus operator transfer

The CLI reads and writes the Kubernetes API and local files required by the
selected command. Proposal and optional history resources persist in the
cluster when those options are used. Local outputs are written at documented
paths. No telemetry or external upload path was found in the inspected product
workflow; this does not prove that data never leaves the cluster because the
founder may manually export files, reports, or logs through a customer-approved
channel.

## Handling procedure

Before the pilot, agree on namespaces, workload scope, evidence types,
permitted retention, storage location, access list, deletion date, transfer
channel, and deletion owner. During the pilot:

- keep evidence in the agreed access-controlled workspace;
- preserve raw evidence and provenance; do not edit raw files in place;
- identify derived artifacts by source and proposal digest;
- avoid copying secrets into reports or transcripts;
- retain approval reasons and customer decisions with the proposal record;
- do not transfer evidence externally without explicit customer approval.

After handoff, delete working copies on the agreed schedule unless a recovery
or incident hold applies. Retain enough proposal, digest, approval,
application, and recovery evidence to explain the outcome until handoff is
accepted.

Do not claim that the product collects no sensitive data, that no data leaves
the cluster, or that retention is automatic. State the actual selected
commands, integrations, files, resources, and operator transfers.
