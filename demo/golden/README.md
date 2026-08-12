Golden E2E demo for landlock-genprof

Purpose
-------
This demo proves the product pipeline from a single workload through three
training runs into a coordinated SecurityProfileProposal that can be
reviewed, approved (candidate-bound), and applied via the hardened apply.

Scope
-----
- Implements three controlled trace runs that exercise filesystem,
  network, syscall and capability observations where possible.
- Persists TrainingHistory via --history and verifies RunsRecorded increments.
- Publishes a single SecurityProfileProposal and verifies the four artifacts
  are present in the persisted CR.
- Demonstrates review -> candidate-digest-bound approve -> hardened apply.

Enforcement boundary
--------------------
This demo does NOT install enforcement operators (PodLock, SPO) and therefore
cannot guarantee enforcement. The script detects and reports presence of
SPO/PodLock and performs enforcement checks only if prerequisites are present.

Layout
------
- workload.yaml        : pod running nginx-demo
- echo-service.yaml    : simple echo TCP service for deterministic egress
- run-actions.sh       : actions executed inside the workload during traces
- hack/demo-golden.sh  : orchestration script (see usage)

Usage
-----
Read and satisfy prerequisites in the script header. Then:
  bash hack/demo-golden.sh

Cleanup
-------
The script prints cleanup hints. It attempts to be rerunnable but does not
automatically delete namespace/workloads unless explicitly asked.

Notes
-----
This demo computes TrainingHistory name using the project's RecordNameV2
algorithm (basename(binary) + short sha256 of full binary path). See
internal/history/store.go for exact semantics. All assertions use the
project's canonical JSON paths (spec.runsRecorded, filesystemAccesses[].seenInRuns,
spec.podLock, spec.networkPolicy, spec.spoSeccompProfile, spec.patchedManifest,
status.approvedCandidateDigest, status.approvalState).

Author
------
landlock-genprof demo automation
