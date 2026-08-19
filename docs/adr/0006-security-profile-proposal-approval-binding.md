# ADR-0006: Bind SecurityProfileProposal approval to exact candidate content

Status: Accepted
Date: 2026-08-12

Context
-------
SecurityProfileProposal currently stores mutable .spec and approval in .status. Approving an object name can outlive the exact candidate content: a later trace/save can overwrite .spec while .status remains Approved. This allows approval transfer from reviewed candidate A to an unreviewed candidate B.

Decision to make
----------------
Ensure approval authorizes an exact candidate revision, not merely an object name.

Invariant (security)
---------------------
INV-PROPOSAL-APPROVAL-01: A proposal is considered approved only when the candidate content being consumed equals the candidate content that was approved.

Options considered
------------------
A) Immutable Spec: make .spec immutable after Create (or after Reviewed/Approved). Security correctness: high. Kubernetes-native: limited (requires admission/webhook to enforce immutability or server-side GC). UX: simple. Migration: requires backfill or deletion strategy. Implementation cost: moderate.

B) Candidate digest binding (preferred start): compute deterministic digest over approval-relevant candidate content; store approvedCandidateDigest in .status. Application requires equality check between current candidate digest and approvedCandidateDigest. Security correctness: high if digest covers correct fields and canonicalization is deterministic. Kubernetes-native: good; status subresource used for binding. Auditability: digest is explicit. Concurrency: RetryOnConflict + digest check prevents transfer. Migration: existing proposals without digest considered untrusted; treat as Draft/Reviewed only. Implementation cost: low–moderate.

C) One object per revision: create new proposal object per candidate revision (e.g., suffix by digest or timestamp). Security correctness: high. Kubernetes-native: natural. UX: proliferation of objects; harder cleanup. Migration: manageable. Implementation cost: moderate.

D) metadata.generation binding: store observedGeneration as reviewed generation. Security correctness: weak — generation can change without content change or be insufficiently precise for content equality. Not recommended alone.

E) Digest + generation (defense-in-depth): store both digest and observedGeneration. Improves observability and concurrency. Cost: slightly higher, recommended variant of B.

Candidate digest: fields classification
--------------------------------------
Security-relevant (must be covered by digest): container, binary, podLock, networkPolicy, patchedManifest, spoSeccompProfile — these are the rendered artifacts that are applied.
Provenance/audit metadata (should NOT affect digest): generatedAt, HistoryUsed.
Rationale: generatedAt is nondeterministic timestamp; HistoryUsed is reviewer/audit metadata. Including these causes false mismatches and brittle digests.

Canonicalization choice
-----------------------
Canonicalization approach: compute digest over canonical serialization of selected Spec fields in a deterministic field order (e.g., JSON with sorted keys) using UTF-8 normalized bytes then SHA-256, truncated hex (8–16 chars optional) for human display; store full hex in status.approvedCandidateDigest. Do not depend on YAML formatting or comment placement. If rendered artifacts include confidence comments, those comments are part of the stored spec strings; digest will therefore reflect them. Make that explicit: confidence comments embedded in artifact strings are digest-relevant unless artifacts are reserialized canonically instead.

Apply-proposal invariant
------------------------
INV-PROPOSAL-APPLY-01: apply-proposal MUST refuse to apply unless status.approvalState == Approved AND CandidateDigest(current Spec) == status.approvedCandidateDigest.

Concurrency countermodels (summary)
----------------------------------
CASE 1 (stale read approve): Approve operation computes digest at time of approval; Save that digest into status with RetryOnConflict. If a concurrent Save overwrote spec earlier, RetryOnConflict forces re-read — approval binds to actual current content at authoritative write time. If reviewer used a stale UI view and the object changed before their approval was persisted, RetryOnConflict prevents accidental binding to the newest content without reviewer seeing it. Conclusion: digest binding prevents A->B transfer.

CASE 2 (approve then immediate write): If approval persisted (digest X) then a later Save writes B (digest Y), status remains approved but digest mismatch prevents apply. Conclusion: fail-closed.

CASE 3 (concurrent reviewers): RetryOnConflict and UpdateStatus semantics preserve last persisted intended state; approval digest ensures correctness. Race handled by RetryOnConflict and standard k8s primitives.

CASE 4 (Save during SetApprovalState retry): SetApprovalState uses RetryOnConflict; on retry it re-reads latest spec and re-writes status with approvedCandidateDigest computed against the spec it observed; this may cause review to re-evaluate and prevents binding B when A changed mid-flight.

CASE 5 (manual Spec edit): Manual edit that changes spec invalidates approved digest; apply requires digest equality; therefore manual mutation cannot inherit approval.

CASE 6 (status-only tamper): If status is set to Approved without approvedCandidateDigest, apply must refuse. Implementation must treat empty/missing digest as untrusted.

CASE 7 (malformed digest): refuse apply.

History/evidence linkage
------------------------
This ADR leaves evidence/history linkage orthogonal. Proposal currently contains HistoryUsed only; adding TrainingHistoryRef, RunsRecorded, or SeenInRuns is a separate product decision and NOT required for approval-binding correctness.

Confidence
----------
Confidence is informational. If confidence comments are embedded in rendered artifact strings, they become part of the digest input (unless canonicalized out). Recommend keeping confidence outside approval-relevant canonicalization unless product explicitly decides otherwise. For now: treat embedded comments as part of spec strings and document impact; prefer that canonicalization step for digest strips human-only comments if product wants them ignored.

Migration
---------
Existing proposals without approvedCandidateDigest: treat as Draft/Reviewed only (require re-approval) or allow a one-time migration that computes digest deterministically and records it with human audit/logging. Migration approach is a separate product/operational decision; default: do not migrate automatically — require re-approval.

CRD changes (proposed fields)
-----------------------------
Add to status:
  approvedCandidateDigest: string (nullable)
  approvalMechanismVersion: string (optional, for future evolution)
No change to spec fields required.

Status transition impact
------------------------
SetApprovalState(Approved) must compute digest over current spec and persist it in status.approvedCandidateDigest within the same UpdateStatus transaction (RetryOnConflict). Reject attempts to set Approved without a digest. MarkReviewed unchanged.

Implementation plan (high-level, non-code)
-----------------------------------------
1) Add approvedCandidateDigest to Status schema (CRD change) and to internal/proposal/types.go Status struct.
2) Implement CandidateDigest(spec) deterministic canonicalization helper and unit tests.
3) Update SetApprovalState to compute digest, include it in setStatus, and use RetryOnConflict.
4) Update apply-proposal to verify digest equality before applying.
5) Add tests for stale reviewer, post-approval mutation, manual mutation, malformed digest, missing digest.
6) Add migration guidance in docs.

Hostile review summary
----------------------
- Approval transfer A->B: prevented by digest check (fail-closed).
- Approved without digest: must be refused; treat as untrusted.
- Stale reviewer: RetryOnConflict forces re-read at persist-time; digest binding prevents accidental binding to unseen newer content.
- Concurrent trace: later Save cannot inherit previous digest; apply will refuse.
- Manual Spec mutation: invalidates digest; apply refused.
- ResourceVersion/generation: insufficient alone; generation may help observability but not cryptographic binding.
- Confidence comments: if present in artifact strings, they affect digest — make that explicit and provide a canonicalization option if needed.

Files touched in later implementation (anticipated)
--------------------------------------------------
- docs/adr/0006-security-profile-proposal-approval-binding.md (this file)
- deploy/crd-securityprofileproposal.yaml (status schema addition)
- internal/proposal/types.go (Status struct)
- internal/proposal/store.go (SetApprovalState / validation)
- cmd/landlock-genprof/apply-proposal.go (apply check)
- internal/proposal/store_test.go, cmd tests (concurrency/stale cases)
- pkg/util/canonicalize or internal/proposal/digest.go (digest canonicalizer)

Pre-implementation checklist (current)
--------------------------------------
1. baseline HEAD: ba24e4b9a7c3aa17cd25f68c7d11c0d4e4658e16 (whitespace fix committed)
2. whitespace fix result: committed as test(history): fix trailing whitespace
3. ADR number/path selected: docs/adr/0006-security-profile-proposal-approval-binding.md
4. ADR status: Proposed
5. exact security defect: approval bound to object/name only; Spec replacement can transfer approval
6. approval invariant: INV-PROPOSAL-APPROVAL-01 (see above)
7. apply invariant: INV-PROPOSAL-APPLY-01 (see above)
8. current candidate mutation behavior: Save overwrites Spec; status preserved; no digest binding
9. Option A (immutable Spec): strong security; operationally heavy; requires admission/webhook or server enforcement; moderate cost
10. Option B (digest binding): preferred start; good k8s fit; low–moderate cost; strong security
11. Option C (object-per-revision): strong security; UX and housekeeping cost; moderate cost
12. Option D (generation): insufficient alone; weak security
13. Option E (digest+generation): defense-in-depth; recommended variant of B
14. recommended design: Option B with Option E variant (digest authority + observedGeneration recorded)
15. digest authority: SHA-256 over canonical JSON serialization of selected Spec fields (container,binary,podLock,networkPolicy,patchedManifest,spoSeccompProfile) with deterministic ordering
16. exact digest input proposal: canonicalized JSON of selected fields, UTF-8 bytes, SHA-256 hex stored in status.approvedCandidateDigest
17. generatedAt decision: exclude from digest (presentation metadata)
18. historyUsed decision: exclude from digest (presentation metadata)
19. confidence interaction: informational; if embedded in artifact strings, it affects digest; recommend canonical serializer that can strip reviewer-only comments if product desires
20. stale-reviewer result: approval will bind only to the spec present at persist-time; RetryOnConflict avoids accidental bind to unrelated concurrent writes
21. post-approval mutation result: mutations invalidate digest; apply fails
22. concurrent trace result: later trace cannot inherit prior approval; apply fails until re-approved
23. concurrent reviewer result: RetryOnConflict semantics and digest ensure correct persisted binding; last persisted approval stands
24. manual Spec mutation result: invalidates digest; apply fails
25. Approved-without-digest result: must be treated as untrusted and not usable for apply
26. malformed-digest result: refuse apply
27. apply-proposal required checks: verify status.approvalState==Approved && CandidateDigest(spec)==status.approvedCandidateDigest
28. CRD fields proposed: status.approvedCandidateDigest:string, optional status.approvalMechanismVersion:string
29. status transition impact: SetApprovalState must compute and persist digest atomically with status update; MarkReviewed unchanged
30. migration behavior: do not auto-migrate existing proposals to have approvedCandidateDigest; require re-approval or explicit migration tool
31. evidence/history linkage: out of scope for this ADR; record as future product debt
32. RFC impact: none; ADR adheres to RFC constraints
33. semantic identity impact: none
34. hostile review result: digest binding resists A->B transfer and related attacks; confidence comment impact must be documented
35. exact files that implementation would later change: (see Files touched above)
36. whether production code changed: NO (only ADR drafted and whitespace fix committed)
37. whether tests changed: NO
38. anything committed: whitespace fix committed (ba24e4b)
39. anything pushed: NO
40. implementation readiness: ready once product agrees on digest canonicalization and migration policy; CRD change + small code changes required.

Decision
--------
Propose Option B (Candidate digest binding) with Option E (digest + observedGeneration) as a defense-in-depth variant. This is an ADR-level decision per governance.md.

Closure decisions
-----------------
This ADR is now Accepted and records the following mandatory, normative choices for the repository's proposal approval model. Implementation may begin after this ADR is landed and stakeholders have reviewed the chosen migration policy.

INV-PROPOSAL-REVIEW-01 (stale-review protection)
-------------------------------------------------
An approval operation MUST identify the exact candidate revision the reviewer intends to authorize. The approval request must include the reviewer's asserted CandidateDigest (the digest displayed during review) as an approval precondition. The server-side SetApprovalState implementation MUST:

1. re-fetch the current Spec for the named proposal (RetryOnConflict semantics apply);
2. compute CandidateDigest(current Spec) according to the mechanism/version declared in the ADR;
3. compare the computed CandidateDigest against the reviewer's supplied expectedCandidateDigest;
4. persist status.ApprovalState=Approved and status.approvedCandidateDigest=computed digest only if they match.

If the supplied expectedCandidateDigest does not match the computed digest, the SetApprovalState operation MUST fail with a clear error instructing the reviewer to refresh and re-run review against the current candidate.

Rationale: this prevents a stale-reviewer from approving an object that changed between their GET and their approval action; approval binds to the exact bytes the reviewer confirmed.

Digest format and versioning
---------------------------
Candidate digests are recorded as self-describing strings: "sha256:<hex>". All digests are produced using the CandidateDigest mechanism version recorded in status.approvalMechanismVersion (recommended value: "candidate-v1"). The verifier MUST reject unsupported mechanism versions.

Status transition rules (normative)
-----------------------------------
- Pending / Draft / Reviewed: status.approvedCandidateDigest MUST be absent.
- Approved: status.approvedCandidateDigest MUST be present and contain a valid digest. Attempts to set Approved without an approvedCandidateDigest MUST be rejected.
- Approved -> Rejected: the implementation SHOULD clear status.approvedCandidateDigest to avoid retaining active authorization material. Audit history of previous approvals may be recorded in an out-of-band audit log if desired.

ResourceVersion / generation
----------------------------
ResourceVersion and metadata.generation are observability / concurrency aids only. They do not serve as the authorization authority. Implementations MAY record observedGeneration in status for debugging/observability, but digest equality is the sole content authorization check.

Apply-proposal enforcement (normative)
--------------------------------------
apply-proposal MUST enforce INV-PROPOSAL-APPLY-01 strictly: it must refuse to apply unless

1. status.ApprovalState == Approved
2. status.approvedCandidateDigest is present and well-formed
3. CandidateDigest(current Spec) recomputes successfully
4. recomputed digest equals status.approvedCandidateDigest

No permissive or warning-only mode is acceptable by default. Any unsafe/developer override must be an explicit, separate product decision and recorded as an ADR.

Confidence and artifact bytes
-----------------------------
Confidence remains informational. However, any bytes present in persisted approval-relevant artifact strings (e.g., a "# confidence: ..." comment in PodLock YAML) are part of the CandidateDigest input under the canonicalization rules chosen in this ADR. A byte-level change (comments, whitespace, formatting) therefore changes the digest and constitutes a distinct candidate.

Migration policy (normative)
----------------------------
Existing proposals with status.ApprovalState == Approved but without status.approvedCandidateDigest MUST NOT be treated as valid for apply. Such proposals require explicit re-approval by a human who supplies the expectedCandidateDigest during approval. No automatic backfill is performed by default.

RBAC / trust boundary
---------------------
CandidateDigest binding ensures content approval integrity (prevents approval transfer) but does NOT authenticate reviewer intent. The authority to mutate status (set Approved) MUST remain controlled by Kubernetes RBAC. This ADR does not introduce reviewer signatures or PKI.

Hostile review summary (final)
------------------------------
All countermodels from the review were considered. Under the chosen design (digest binding + approval precondition by expectedCandidateDigest + RetryOnConflict), the system is fail-closed: post-approval candidate mutation, manual spec edits, stale-reviewer misbinding, malformed-digest, and status-only tampering cannot enable an unreviewed candidate to be applied.

Decision record
---------------
Status: Accepted
Date accepted: 2026-08-12

-- End of ADR-0006 (Accepted)
