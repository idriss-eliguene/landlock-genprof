# v0.8 Governance Operations boundary

This document is the release-candidate boundary for v0.8. It describes the
implemented read projection; it does not authorize publication or change the
underlying Kubernetes objects.

## Product position

landlock-genprof is **Evidence-driven Least-Privilege Governance for
Kubernetes**. v0.8 presents Governance Operations: a read and explanation
surface over already-durable observations, populations, proposals, and
application custody.

The operational question is:

> Across the workloads and populations already observed and governed, which
> have a reconciliation gap between durable evidence and durable governance
> state, and which named gap is it?

The projection is not a generic SOC, SIEM, CNAPP, KSPM, threat-detection,
vulnerability, compliance, or incident-response product.

## Identity and authority

An Environment subject reuses `PopulationIdentity`:

```text
Scope, Target, Container, ImageIdentity, BinaryPath
```

BINARY and CONTAINER populations remain distinct. Workload UID is not part of
PopulationIdentity or the candidate-v2 Proposal subject. Candidate-v2 policy
authority is Proposal-object-scoped:

* zero valid approved candidates: `NONE`;
* one valid candidate: `SELECTED`;
* more than one: `AMBIGUOUS`.

Validity requires candidate-v2 compatibility, exact subject matching,
`Approved` status, and `ValidateApprovedCandidate`. Equal candidate digests do
not transfer authority between Proposal objects.

## Five independent axes

The read model keeps these axes independent:

1. `DERIVATION`
2. `GOVERNANCE`
3. `APPLICATION`
4. `STRUCTURAL_ENFORCEMENT_KNOWLEDGE`
5. `BEHAVIORAL_VERIFICATION`

Behavioral verification is always `UNKNOWN` in v0.8. Positive structural
knowledge is limited to: “Kubernetes object confirmed to match the intended
spec immediately after application.” It does not establish current, kernel,
or behavioral enforcement.

## Attention taxonomy

Attention is a derived read-model fact with no persistence, severity, or
acknowledgment:

* `CAPABILITY_OUTSIDE_APPROVED_POLICY`
* `APPROVED_NOT_APPLIED`
* `APPLICATION_STATE_UNKNOWN`
* `NEW_CONTRIBUTION_SINCE_CANDIDATE`
* `OBSERVATION_FAILED_BOUND`
* `OBSERVATION_FAILED_UNBOUND`
* `MULTIPLE_VALID_APPROVED_PROPOSALS`

`NEW_CONTRIBUTION_SINCE_CANDIDATE` means a durable Observation contribution is
absent from the latest compatible candidate-v2 provenance snapshot. It does
not mean new behavior, new capability, or drift. Capability attention is
computed independently from accumulated positive capability facts.

## Environment, History, and UID ambiguity

Environment is a bounded, deterministic projection over bulk-loaded durable
objects. A subject may be prospective when an exact bound Observation exists
without a TrainingHistory Population. Population absence is not evidence
absence.

History separates timestamped events from untimestamped or unordered custody
facts. Approval transition history and contribution chronology are not fully
reconstructable from the durable schema. Dangling references remain visible.

`ProposalSubjectMatchedMultipleWorkloadUIDs` is a boolean positive-only
signal. True means at least two distinct matching workload UIDs are proven by
the supplied durable Observations. False means multiplicity is not proven; it
does not prove uniqueness or absence of recreation.

## Read and browser boundary

The v0.8 HTTP surface is read-only and namespace-pinned:

* `/api/v08/environment`
* `/api/v08/environment/detail`
* `/api/v08/history`
* `/api/v08/history/proposal`

Reads are bounded bulk reads assembled from multiple Kubernetes objects. They
are best-effort and not transactional snapshots. The browser may start/stop
Observations, inspect status, and generate proposals through the existing
supported paths, but it cannot approve, reject, revoke, apply, rollback,
acknowledge, or dismiss governance state.

## Claim matrix

| Claim | Classification |
|---|---|
| Environment projection | EMPIRICALLY_QUALIFIED read projection |
| Attention predicates | STRUCTURALLY_PROVEN and unit-qualified |
| Approved-policy ambiguity | STRUCTURALLY_PROVEN and envtest-qualified |
| Application custody | EMPIRICALLY_QUALIFIED durable attempt projection |
| Structural application-time confirmation | STRUCTURALLY_PROVEN, bounded read-back claim |
| Behavioral verification | NOT_PROVEN; always UNKNOWN |
| Current enforcement | NOT_PROVEN / OUT_OF_SCOPE |
| Complete observation coverage | NOT_PROVEN |
| Complete approval history | NOT_RECONSTRUCTABLE_BY_SCHEMA |
| Contribution chronology | NOT_RECONSTRUCTABLE_BY_SCHEMA |
| UID multiplicity disclosure | EMPIRICALLY_QUALIFIED positive-only signal |
| Transactional multi-object consistency | NOT_PROVEN; explicitly disclaimed |
| Multicluster governance | OUT_OF_SCOPE |
| Continuous monitoring or drift detection | OUT_OF_SCOPE |

## Release-candidate state

```yaml
candidateVersion: 0.8.0
state: RELEASE_CANDIDATE
g9QualifiedSHA: d9c3be6dc73cebf2330ab28472173de4be00c76f
candidateV2Digest: sha256:46062013486c3c47ba3d092d002fa12eb86eeb2019eafcd8b1e51805a9e32609
reviewContextV2Digest: sha256:559d6234e02a96a88bb3911cd2483b124bd9fd434ce6847ab76ca056cf3d662
realAPIServerBasis: envtest Kubernetes 1.36.2
kindQualification: NOT_QUALIFIED_IN_G9
localGosec: NOT_RUN_IN_G9
released: false
published: false
tagVerified: false
```

The final G10 commit SHA and tree are recorded in the final custody report;
this candidate record deliberately does not claim release, publication, or
tag verification.
