# ADR-0009: SPO merged SeccompProfile provenance and target separation

Status: Accepted

Date: 2026-08-21

## Context

ADR-0008 defines SPO import as an explicit, inert, derived-policy snapshot
whose supported enforcement content and provenance enter a
`SecurityProfileProposal`. Its strong-lineage contract is appropriate for
`ProfileRecording.spec.mergeStrategy: None`: the generated profile identifies
the recording namespace, recording, and source container, and import requires
that source-container correspondence.

`mergeStrategy: Containers` has different semantics. Within one
`ProfileRecording`, SPO groups partial profiles by container name and unions
each group. The merged `SeccompProfile` retains recording-level provenance but
does not retain the exact contributing partial-profile, pod, or
container-instance set. That set cannot be reconstructed from the final
object. ADR-0008 therefore refuses merged profiles because they cannot satisfy
its strong-lineage tuple.

That refusal must remain the behavior of the strong-lineage contract. It does
not follow, however, that an exact merged policy cannot be governed under a
different, explicit contract. Governance can authorize a derived-policy
snapshot for an independently specified target without claiming that the
target uniquely produced the policy.

The trust ceiling remains structural Kubernetes metadata protected by the
cluster's RBAC boundary. This decision does not introduce cryptographic
provenance and does not strengthen what SPO's output proves.

## Upstream semantic boundary

SPO owns syscall observation, the `ProfileRecording` lifecycle, and
`SeccompProfile` generation. landlock-genprof consumes the resulting
`SeccompProfile` as externally derived policy at the artifact layer.

The imported policy is not landlock-genprof observation. It MUST NOT enter
`TrainingHistory`, acquire landlock-genprof confidence, or grant deployment
authority merely because SPO produced it. SPO coverage is not observation
identity or authority.

## Decision

landlock-genprof defines two explicit SPO import contracts.

### Strong-lineage import

The ADR-0008 `mergeStrategy: None` contract remains unchanged and fail-closed.
Import requires the existing recording namespace, recording identity, and
source-container correspondence. Missing or mismatched lineage is fatal.

### Merged-provenance import

`mergeStrategy: Containers` MAY be imported only through an explicitly
selected merged-provenance mode. The import records honest recording-level
provenance, the merged derivation, unavailable contributor lineage, the exact
policy snapshot, and an independently specified application target.

There is no downgrade or fallback between these contracts. In particular:

    strong-lineage validation fails
        X
    reinterpret the source as merged provenance

A caller must select the intended semantic contract before validation. A
failure under one contract remains a failure under that contract.

## Identity separation

The following identities are distinct:

- **source provenance** describes where and how the derived policy originated;
- **application target** describes where the governed policy is intended to be
  applied;
- **policy identity** is the exact imported enforcement content and its
  authorization-relevant semantics;
- **authorization subject** is the approval-relevant
  `SecurityProfileProposal.Spec` content bound by `CandidateDigest`.

Policy origin is not application target. A recording may truthfully be the
origin of a merged policy without proving that the target workload or
container contributed to that recording.

## Merged provenance contract

The minimum merged provenance tuple is:

    Pmerged = (
        sourceSystem = SPO,
        sourceKind = ProfileRecording,
        recordingNamespace,
        recordingName,
        sourceProfileName,
        derivation = merged,
        mergeStrategy = containers,
        contributorLineageState = unavailable
    )

The application target is separate:

    T = (
        targetNamespace,
        targetWorkload,
        targetContainer
    )

Both tuples describe the governed candidate. Neither tuple supplies authority.
The exact target representation may reuse existing proposal and generated
artifact fields, but it MUST be explicit, review-visible, and digest-bound.

The source policy MUST remain a complete, non-partial, inert snapshot and MUST
contain only enforcement semantics landlock-genprof can preserve exactly, as
required by ADR-0008. Recording identity is read from SPO's recording labels;
profile names and coverage MUST NOT be used to infer missing core provenance.

## Information-loss contract

Merged import does not claim or reconstruct:

- pod name or pod UID;
- replica identity;
- source controller or workload identity;
- container-instance identity;
- the exact set of contributing partial profiles.

This absence is an expected semantic state. It MUST be represented as
unavailable, not guessed from object names, coverage counts, or other
implementation details. If a source asserts unique contributor lineage that
cannot be supported, import fails closed.

## Policy widening

Unioning partial profiles can yield a policy broader than the policy observed
for any single contributor. A broad recording selector can combine profiles
from distinct workloads that use the same container name. Applying that union
to only one target can therefore grant syscalls learned from other
contributors.

Merged derivation, unavailable contributor lineage, and this least-privilege
widening risk MUST be visible before approval. Review does not restore least
privilege; it exposes the trade-off so a human can decide whether the exact
union is appropriate for the explicit target.

## Optional coverage metadata

SPO syscall coverage is optional provenance enrichment. For the merged format
introduced upstream, `total` counts contributing partial profiles and each
per-syscall value counts partial profiles containing that syscall. It does not
identify those contributors.

Coverage MUST NOT become:

- contributor or container lineage;
- syscall invocation frequency;
- confidence or probability;
- landlock-genprof `TrainingHistory`;
- landlock-genprof-owned observation;
- authorization.

Coverage is not covered by SPO's stability guarantee. Absence, malformed
content, and an unsupported version are distinct review states, but none may
be used to infer missing information. Bad or absent optional coverage alone
MUST NOT invalidate an otherwise valid merged-policy snapshot. A future
adapter may normalize supported metadata for review; this ADR does not define
its parser or wire schema.

## CandidateDigest and authorization

The existing authorization architecture remains:

    SecurityProfileProposal.Spec
        -> candidate-v1 CandidateDigest
        -> review
        -> explicit human approval of that exact digest

No `AuthorizationManifest` is introduced. `CandidateDigest` identifies the
authorization subject; it is not authority.

For merged import, the authorization subject MUST bind at least:

- the exact imported enforcement policy;
- source recording namespace and name;
- source profile name;
- merged derivation and `containers` merge strategy;
- unavailable contributor-lineage state;
- the explicit application target.

Authorization-relevant provenance may remain embedded in the governed
`SeccompProfile` annotations, as in ADR-0008, provided it is represented in the
authoritative review and remains covered by `CandidateDigest`. Changing any
bound property produces a different candidate and makes prior approval
unusable for the changed candidate.

Optional coverage is authorization-bound only if its normalized semantic state
or value is presented as decision-relevant by the authoritative review. Raw,
unstable annotation encoding MUST NOT define identity merely because its bytes
differ.

## Snapshot semantics

Import creates a governed snapshot, not a live dependency or delegation of
authority to SPO. Mutation or deletion of the source after import does not
mutate an existing proposal. A later import may create a different candidate;
if authorization-bound content changes, `CandidateDigest` changes and the old
approval is stale.

Deletion of a source `ProfileRecording` is compatible with merged provenance
because deletion is part of SPO's merge lifecycle. Its absence does not permit
inventing missing provenance and does not relax validation of the final source
profile.

## Review contract

Before a merged candidate can be approved, authoritative review MUST show:

- source system: SPO;
- source recording namespace and name;
- source profile name;
- derivation: merged;
- merge strategy: containers;
- contributor lineage: unavailable;
- a warning that union semantics may widen policy;
- coverage state and supported normalized coverage, when available;
- the explicit target namespace, workload, and container;
- the exact policy or a complete, decision-equivalent representation;
- `CandidateDigest`.

Review MUST NOT display guessed contributors or imply that the target produced
the source policy.

## Failure semantics

| Condition | Required behavior |
|---|---|
| Strong-lineage validation failure | **FAIL-CLOSED**; no merged fallback |
| Merged mode not explicitly selected | **FAIL-CLOSED** |
| Recording namespace or recording identity missing/malformed | **FAIL-CLOSED** |
| Derivation mode absent or ambiguous | **FAIL-CLOSED** |
| Source marked partial | **FAIL-CLOSED** |
| Unsupported enforcement content | **FAIL-CLOSED** |
| Unsupported claim of unique contributor lineage | **FAIL-CLOSED** |
| Source profile belongs to another explicitly selected recording | **FAIL-CLOSED** |
| Target changes after approval | New candidate; prior approval is stale |
| Source policy changes after snapshot | Existing snapshot unchanged; re-import creates a new candidate |
| Source recording deleted after merge | Allowed under explicit merged provenance; no lineage is inferred |
| Coverage absent | Import proceeds; review reports unavailable coverage |
| Coverage malformed or unsupported | Import proceeds only if core policy/provenance is valid; review reports the state and interprets no counts |

A copied profile carrying intact structural labels remains indistinguishable
from its source to this model. That is a residual consequence of ADR-0008's
RBAC-based structural trust ceiling, not cryptographic provenance. The selected
source profile name, exact content, provenance state, and target remain
reviewed and digest-bound.

## Security considerations

- **Policy widening.** A merged union may be broader than any contributor;
  review must expose this and cannot claim least privilege.
- **Provenance overstatement.** Contributor identity is unavailable and must
  never be inferred from names or coverage.
- **Source/target confusion.** Recording origin and application target are
  independently represented and bound.
- **Structural metadata forgery.** A principal able to write cluster-scoped
  `SeccompProfile` metadata can copy or forge labels. Kubernetes RBAC remains
  the trust boundary; this decision adds no authentication claim.
- **Stale authorization.** Any authorization-bound policy, provenance, or
  target change produces a different candidate.
- **Fallback downgrade.** Strong-lineage failure cannot silently select the
  weaker merged contract.
- **Application is not enforcement.** Governed apply submits the approved
  artifacts to external backends; it does not prove enforcement.
- **Enforcement is not verification.** Backend readiness or reconciliation
  does not by itself demonstrate runtime denial behavior.

## Challenge record

The decision was tested against the following counterexamples before
acceptance:

| Counterexample | Fail-safe result |
|---|---|
| Profile copied with intact SPO labels | Residual RBAC trust limitation is disclosed; exact selected snapshot remains reviewed and bound |
| Profile from another recording | Recording identity mismatch is fatal |
| Missing recording metadata | Fatal core-provenance failure |
| Missing coverage | Valid policy may import; coverage shown unavailable |
| Malformed/unsupported coverage | No inference; explicit review state; core import remains independent |
| Changed target workload/container | Candidate changes; prior approval is stale |
| Broad selector widens the union | Widening and unavailable contributors are review-visible |
| Unique contributor lineage claimed | Rejected because the claim cannot be supported |
| Source recording deleted after merge | Expected lifecycle state; snapshot provenance remains recording-level |
| Source changes after snapshot | Existing proposal remains unchanged; re-import creates a new candidate |
| Merged mode used after strong failure | Rejected because modes are selected before validation and never fall back |

No counterexample requires weakening ADR-0008's strong-lineage path.

## Relationship to ADR-0008

ADR-0008 remains authoritative for:

- SPO ownership of syscall observation and profile generation;
- the derived-policy artifact boundary;
- prohibition on fabricated `TrainingHistory` and confidence;
- explicit source selection and no fallback to internal synthesis;
- exact, inert snapshot import and supported-field checks;
- `SecurityProfileProposal`, `CandidateDigest`, and human approval;
- the rule that import grants no deployment authority;
- external enforcement ownership;
- the `mergeStrategy: None` strong-lineage contract.

This ADR supersedes ADR-0008 only where ADR-0008 excludes merged-profile
import because unique container lineage is absent. It replaces that exclusion
with an explicit, weaker recording-level contract for
`mergeStrategy: Containers`. It does not amend the strong-lineage contract or
any other ADR-0008 decision.

## Consequences

**Benefits.** Merged SPO policy can enter governance without laundering it
into direct evidence or claiming lineage SPO discarded. Source and target are
made explicit, and the exact union remains subject to existing digest-bound
human authorization.

**Costs.** Merged policy has weaker provenance and can be less least-privilege.
The product must maintain two explicit import contracts and present their
different assurances accurately. Optional upstream metadata needs a small
compatibility boundary rather than direct semantic dependence.

## Non-goals

- reconstructing contributor lineage;
- changing or replacing SPO;
- creating a generic provenance framework;
- cryptographic provenance or authentication;
- introducing `AuthorizationManifest`;
- treating coverage as confidence, evidence, or authority;
- automatically proving least privilege;
- defining parser, CLI, CRD, or controller implementation details beyond the
  semantic constraints above;
- changing apply ordering, backend ownership, enforcement, or verification
  semantics.

## References

- [ADR-0008](0008-spo-derived-policy-import-boundary.md) — SPO derived-policy
  snapshot, strong lineage, TrainingHistory isolation, and authorization.
- [ADR-0007](0007-governed-apply-ordering-and-enforcement-readiness.md) —
  governed apply, readiness, and external enforcement boundaries.
- `internal/spoimport/spoimport.go` — current strong-lineage and supported
  enforcement-content validation.
- `internal/proposal/digest.go` — existing `candidate-v1` digest contract.
