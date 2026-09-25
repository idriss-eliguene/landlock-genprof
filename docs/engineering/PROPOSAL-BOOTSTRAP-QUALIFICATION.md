# Proposal Bootstrap Golden E2E Qualification

## Qualification scope

This report records the live Proposal Bootstrap campaign executed from
source commit `f25d5480a64f9c284db0bc26d0c7551e3cab3dc6`. The campaign used
only the dedicated disposable environment:

- Docker context: `lima-landlock-genprof-core`
- Kind cluster: `landlock-genprof-proposal-bootstrap`
- Kubernetes context: `kind-landlock-genprof-proposal-bootstrap`
- Kubernetes: `v1.36.4`
- Node image: `kindest/node:v1.36.4`
- Container runtime: `containerd 2.3.4`
- Cilium: `1.20.1`
- Inspektor Gadget: `v0.55.1`

Existing clusters were not modified. SPO and PodLock were intentionally not
installed.

## Golden workflow result

The existing three-trace Golden workflow completed successfully:

| Stage | Result |
| --- | --- |
| Trace 1 | `TrainingHistory.runsRecorded=1` |
| Trace 2 | `TrainingHistory.runsRecorded=2` |
| Trace 3 | `TrainingHistory.runsRecorded=3` |
| Confidence aggregation | PASS; high, medium and low observations retained |
| Proposal generation | PASS |
| Review | PASS |
| Wrong-digest approval rejection | PASS |
| Governance qualification | PASS |

The resulting TrainingHistory records the `nginx-demo` workload, `tools`
container and `/usr/bin/curl`, including filesystem, network, capability and
syscall evidence.

## Proposal and approval custody

- Proposal UID: `3227989d-50d4-42dd-bba0-401cf7d85d14`
- Proposal resourceVersion: `7125`
- Approved candidate digest:
  `sha256:72899462790a8e7f97dee317344037de1b594b9766a646bc84ce1f66f6b643d7`
- Approval state: `Approved`
- Approval mechanism: `candidate-v1`

An approval using an all-zero digest was rejected with an explicit candidate
digest mismatch. Approval of the recorded candidate digest succeeded.

## ApplyAttempt evidence

Two ApplyAttempts were recorded and both reached `APPLIED`:

1. `apply-20260925t095604-fdc59ceea117fcbb`, UID
   `dccfd95c-eab4-4819-869f-05b390fb0a86`: NetworkPolicy creation succeeded.
2. `apply-20260925t095605-8286f1d6eec22eba`, UID
   `794a2a9a-2400-4ea8-a30c-6779206989fa`: NetworkPolicy update and pod
   replacement succeeded.

Both attempts reference the same proposal UID, namespace, target workload,
container and approved digest. All recorded mutation results were
`SUCCEEDED`.

The direct CLI path recorded an empty `operatorIdentity`. Dedicated
profile-realizer attribution was therefore not established by this campaign;
the result must not be interpreted as proof that a privileged realizer ran.

## Qualification boundaries

- NetworkPolicy realization: **PASS**
- Pod mutation/restart realization: **PASS**
- SPO SeccompProfile realization: **NOT_VERIFIED**; SPO was absent
- PodLock realization: **NOT_VERIFIED**; PodLock was absent
- Dedicated profile-realizer execution: **NOT_VERIFIED**
- Kernel-level seccomp or Landlock enforcement: **NOT_VERIFIED**

Generated artifacts and proposal readiness are not evidence of runtime kernel
enforcement.

## Evidence custody

The complete final evidence is preserved outside the repository at:

`/Users/idrisseliguene/Documents/landlock-genprof-proposal-bootstrap-evidence-2026-09-25/`

The original campaign evidence remains at:

`/tmp/landlock-genprof-proposal-bootstrap.BwVDcR/live-evidence-linux-final/final-evidence/`

The durable copy contains the trace logs, proposal, TrainingHistory,
ApplyAttempt snapshots, approval output and machine-readable bootstrap result.
No kubeconfig, credential or private-key file was copied.

The original campaign manifest SHA-256 is:

`88f61ebb5939e28f186583a2d0fa18a343bf31fa62cc2a6eec0ef87dbb7cb666`

The copied evidence was independently verified against its relative-path
manifest. The copied manifest SHA-256 is:

`877ed068b925026f328ebdaa67118f7079a8115115a808d3b8f995a53bd11537`
