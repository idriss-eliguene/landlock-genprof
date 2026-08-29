# Kubernetes Least-Privilege Hardening Review

**Customer:** `<customer>`  
**Engagement:** `<engagement>`  
**Prepared by:** `<operator>`  
**Review date:** `<date>`  
**Status:** `DRAFT | REVIEW REQUIRED | APPROVED | APPLIED | VERIFIED_IN_SCOPE`

This document is a reviewable summary of a selected workload and the
landlock-genprof artifacts available for it. It is not a penetration test,
compliance certificate, vulnerability scanner report, or proof of globally
minimal privilege. Complete only fields supported by the evidence. Use
`UNKNOWN`, `NOT_AVAILABLE`, or `NOT_PROVABLE` instead of filling a gap with an
assumption.

The operating procedure, prerequisites, recovery, and data handling belong to
the founder-assisted [pilot package in issue #157](https://github.com/idriss-eliguene/landlock-genprof/issues/157).

## 1. Executive summary

### Reviewed target

- **Workload:** `<name>`
- **Namespace:** `<namespace>`
- **Container/binary:** `<container>` / `<binary>`
- **Scope agreed with customer:** `<scope>`

### Result by state

| State | Result | Evidence or explanation |
|---|---|---|
| Assessment | `COMPLETE | PARTIAL | STOPPED` | `<what was assessed>` |
| Candidate | `PRODUCED | NOT_PRODUCED` | `<proposal/artifact reference>` |
| Approval | `APPROVED | REJECTED | REVIEWED | DRAFT | UNKNOWN` | `<proposal status>` |
| Application | `APPLIED | PARTIALLY_APPLIED | NOT_APPLIED | UNKNOWN` | `<command result and scope>` |
| Verification | `VERIFIED_IN_SCOPE | NOT_VERIFIED | NOT_CERTIFIED | UNKNOWN` | `<backend and boundary>` |

### Decision requested

`REVIEW CANDIDATE | RETAIN | APPLY IN STAGING | COLLECT MORE EVIDENCE |
VERIFY BACKEND | DEFER | REJECT | ESCALATE FOR WORKLOAD OWNER REVIEW`

Human decision and business legitimacy remain with the customer and workload
owner. An observation does not establish that behavior is legitimate, and a
missing observation does not establish that a permission is unnecessary.

## 2. Engagement scope

- **Objective:** `<bounded review objective>`
- **Included namespaces/workloads:** `<scope>`
- **Excluded workloads/domains:** `<scope>`
- **Observation window:** `<start/end or duration>`
- **Customer authorization:** `<owner, date, and change scope>`
- **Change window:** `<if application is in scope>`
- **Recovery owner:** `<name/team>`
- **Evidence handling agreement:** `<reference to agreed handling>`

Do not describe this review as universal Kubernetes compatibility, a generic
security assessment, or a guarantee of safe removal.

## 3. Environment and workload

| Field | Value | Source / status |
|---|---|---|
| CLI version/revision | `<value>` | `landlock-genprof version` |
| Kubernetes version | `<value>` | `kubectl version` |
| Nodes/kernel/architecture | `<value>` | `kubectl get nodes -o wide` and authorized node capture |
| Container runtime | `<value>` | `<customer/environment capture>` |
| CNI | `<value>` | `<environment capture>` |
| Inspektor Gadget | `<version/status>` | `<environment capture>` |
| SPO | `<version/status or not used>` | `<environment capture>` |
| PodLock | `<version/status or not used>` | `<environment capture>` |
| Landlock ABI | `<value or unknown>` | `doctor` / `verify` result |
| Target workload | `<name/namespace/kind>` | `<kubectl manifest reference>` |
| Target container/binary | `<value>` | trace invocation |

Support and certification status must be taken from the repository's
[current evidence](../PROGRESS.md) and [enforcement prerequisites](../enforcement-prerequisites.md),
not inferred from this table alone.

## 4. Current authority and security configuration

This section describes what is configured now. Do not call a configuration
excessive merely because the observation window did not contain its behavior.

| Domain | Current configuration | Source | Status |
|---|---|---|---|
| Filesystem / Landlock | `<current profile or not configured>` | `<manifest/resource>` | `CONFIGURED | UNKNOWN | NOT_AVAILABLE` |
| Network | `<NetworkPolicy and relevant labels>` | `<manifest/resource>` | `CONFIGURED | UNKNOWN | NOT_AVAILABLE` |
| Seccomp | `<RuntimeDefault/Localhost/none>` | `<workload manifest>` | `CONFIGURED | UNKNOWN | NOT_AVAILABLE` |
| Capabilities | `<add/drop/current securityContext>` | `<workload manifest>` | `CONFIGURED | UNKNOWN | NOT_AVAILABLE` |
| Other controls | `<relevant controls>` | `<source>` | `<status>` |

The generated patched manifest is a proposed artifact, not proof of the
current live configuration. Attach the pre-change manifest or state capture.

## 5. Available evidence and provenance

### Evidence inventory

| Evidence item | Source identity | Collection/import context | Time | Direct or derived | Scope / limitations |
|---|---|---|---|---|---|
| `<events file or observation>` | `<backend/source>` | `<trace or import>` | `<timestamp or unknown>` | `DIRECT EVIDENCE` | `<scope>` |
| `<SPO profile, if used>` | `<recording/profile>` | `SPO import` | `<timestamp or unknown>` | `DERIVED POLICY` | `<coverage/lineage>` |
| `<configured manifest>` | `Kubernetes API` | `<capture command>` | `<timestamp>` | `CONFIGURED STATE` | `<scope>` |

SPO-generated `SeccompProfile` is derived policy from landlock-genprof's
perspective. It must not be labeled a direct syscall trace. Coverage metadata
is informational; it is not confidence, authorization, or proof of
completeness.

### Evidence interpretation

- **Observed:** behavior captured by the identified source within its stated
  scope.
- **Not observed:** behavior absent from that capture; it is not a claim of
  necessity or non-necessity.
- **Derived:** policy produced by transforming evidence or importing another
  system's policy.
- **Unknown:** the evidence or field is unavailable or not established.

## 6. Proposed hardening changes

The proposal is a candidate for review, not an authorization.

| Domain | Available evidence | Derived policy / candidate | Proposed change | Provenance | Reviewer action |
|---|---|---|---|---|---|
| Filesystem / Landlock | `<observed paths and scope>` | `<candidate reference>` | `<candidate delta>` | `<source>` | `REVIEW / RETAIN / DEFER` |
| Network | `<observed/configured scope>` | `<NetworkPolicy reference>` | `<candidate delta>` | `<source>` | `REVIEW / RETAIN / DEFER` |
| Seccomp | `<direct or SPO-derived source>` | `<profile reference>` | `<candidate delta>` | `<source and lineage>` | `REVIEW / COLLECT MORE EVIDENCE` |
| Capabilities/securityContext | `<observed/configured scope>` | `<fragment or patched manifest>` | `<candidate delta>` | `<source>` | `REVIEW / VERIFY BACKEND` |

Use `NOT_AVAILABLE` when a domain was not generated. Use `NOT_PROVABLE` when
the artifact does not establish the claimed interpretation. Do not force a
domain into the table when its semantics are not available.

## 7. Authority and policy delta

### Before

`<current configuration, with exact references>`

### Candidate

`<proposed artifact or proposal field, with exact references>`

### Delta requiring human review

`<meaningful additions/removals or changed references, bounded to the artifact>`

The phrase “candidate reduction” means that the candidate changes the
represented authority for review. It does not establish that the previous
configuration was unnecessary or that the candidate is the smallest possible
policy.

## 8. Proposal identity and approval

| Field | Value |
|---|---|
| Proposal name/namespace | `<value>` |
| Generated at | `<spec.generatedAt>` |
| Candidate digest | `<sha256:...>` |
| Approval mechanism version | `<candidate-v1 or unknown>` |
| Review state | `DRAFT | REVIEWED | UNKNOWN` |
| Approval state | `APPROVED | REJECTED | REVIEWED | DRAFT | UNKNOWN` |
| Approved candidate digest | `<status.approvedCandidateDigest or unknown>` |
| Approval reason | `<status.reason or manual record>` |
| Staleness/revalidation | `MATCHED | STALE | NOT_CHECKED | UNKNOWN` |

Approval authorizes the exact candidate digest only. A proposal name is not an
approval, and approval is not application.

## 9. Application outcome

| Field | Value |
|---|---|
| Application command | `<exact command or NOT_RUN>` |
| Confirmation/change authorization | `<record>` |
| Artifact order | `<record actual order>` |
| Per-artifact result | `<applied/failed/not attempted, or UNKNOWN>` |
| Workload binding | `APPLIED | NOT_APPLIED | UNKNOWN` |
| Partial application | `NO | YES | UNKNOWN` |
| Recovery action | `<reference to pilot recovery record>` |

Application is separate from approval and enforcement. Multi-artifact apply is
sequential, not transactional: a failure stops later work and does not
automatically roll back resources already applied.

## 10. Enforcement and behavioral verification

| Domain/backend | Configured | Proposed | Approved | Applied | Enforcement evidence | Behavioral verification | Tested/certified scope |
|---|---|---|---|---|---|---|---|
| NetworkPolicy/CNI | `<state>` | `<state>` | `<state>` | `<state>` | `<evidence or NONE>` | `<result>` | `<CNI/workload scope>` |
| SPO/Seccomp | `<state>` | `<state>` | `<state>` | `<state>` | `<reconciliation/materialization>` | `<result>` | `<recorded scope>` |
| PodLock/Landlock | `<state>` | `<state>` | `<state>` | `<state>` | `<evidence or NONE>` | `<result>` | `<scope or NOT_CERTIFIED>` |
| Capabilities/securityContext | `<state>` | `<state>` | `<state>` | `<state>` | `<evidence or NONE>` | `<result>` | `<scope or NOT_CERTIFIED>` |

Kubernetes API application is not enforcement evidence. Enforcement evidence
is not automatically behavioral verification. Verification is bounded to the
named backend, workload, environment, and test procedure; it is not universal
compatibility.

## 11. Limitations and unknowns

List explicitly:

- observation window and startup/restart coverage;
- direct evidence and derived-policy boundaries;
- optional or unknown coverage metadata;
- unsupported or uncertified backends and environments;
- CNI-specific NetworkPolicy behavior;
- PodLock/Landlock and capabilities/securityContext verification limits;
- sequential application and any partial state;
- missing current-state, application, or verification records;
- customer decisions not made by the tooling.

## 12. Recommended human action

Choose one action per candidate and record the decision owner and date:

`REVIEW CANDIDATE | RETAIN | APPLY IN STAGING | COLLECT MORE EVIDENCE |
VERIFY BACKEND | DEFER | REJECT | ESCALATE`

Recommendation: `<bounded recommendation>`  
Decision owner: `<customer/workload owner>`  
Decision date: `<date>`  
Reason: `<human rationale>`

## 13. Artifact inventory and recovery reference

| Artifact | Identity/path | Source | Retained until | Handoff status |
|---|---|---|---|---|
| Proposal | `<name/namespace>` | Kubernetes API | `<date>` | `<status>` |
| Raw evidence | `<path/resource>` | `<source>` | `<date>` | `<status>` |
| Candidate | `<path/digest>` | `<proposal>` | `<date>` | `<status>` |
| Generated policy artifacts | `<paths>` | `<proposal>` | `<date>` | `<status>` |
| Application/recovery record | `<path/reference>` | `<operator>` | `<date>` | `<status>` |

Recovery reference: [`pilot recovery procedure`](https://github.com/idriss-eliguene/landlock-genprof/issues/157).

## 14. Methodology and claim boundary

This review distinguishes:

```text
configuration → evidence → derived policy → proposal → approval
             → application → enforcement evidence → behavioral verification
```

Coverage is not confidence. Confidence is not authorization. Observed is not
legitimate. Not observed is not unnecessary. A reviewable candidate is not a
guarantee of safety, compatibility, or minimality.
