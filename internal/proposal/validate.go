package proposal

import "fmt"

// ValidateApprovedCandidate verifies status/Spec consistency for apply-time
// authorization. Returns nil if the status indicates Approved and the
// ApprovedCandidateDigest matches CandidateDigest(spec) according to a
// supported ApprovalMechanismVersion.
func ValidateApprovedCandidate(spec *Spec, status *Status) error {
	if spec == nil {
		return fmt.Errorf("proposal spec is nil")
	}
	version, err := normalizedCandidateVersion(spec.CandidateVersion)
	if err != nil {
		return err
	}
	if version == CandidateVersionV2 {
		return validateApprovedCandidateV2(spec, status)
	}
	return validateApprovedCandidateV1(spec, status)
}

func validateApprovedCandidateV1(spec *Spec, status *Status) error {
	if status == nil {
		return fmt.Errorf("no approval status: legacy or missing status; re-approval required")
	}
	if status.ApprovalState != ApprovalApproved {
		return fmt.Errorf("proposal not Approved: state=%q", status.ApprovalState)
	}
	if status.ApprovedCandidateDigest == "" {
		return fmt.Errorf("legacy approval requires explicit re-approval: approvedCandidateDigest missing")
	}
	// Validate digest syntax
	if err := ValidateCandidateDigest(status.ApprovedCandidateDigest); err != nil {
		return fmt.Errorf("invalid approved candidate digest: %w", err)
	}
	// Check mechanism version
	if status.ApprovalMechanismVersion != CandidateVersionV1 {
		return fmt.Errorf("unsupported approval mechanism version: %q", status.ApprovalMechanismVersion)
	}
	// Recompute digest over the provided spec
	computed, err := CandidateDigest(*spec)
	if err != nil {
		return fmt.Errorf("computing candidate digest: %w", err)
	}
	if computed != status.ApprovedCandidateDigest {
		return fmt.Errorf("approved candidate digest mismatch: approved=%s computed=%s", status.ApprovedCandidateDigest, computed)
	}
	return nil
}

func validateApprovedCandidateV2(spec *Spec, status *Status) error {
	if status == nil || status.ApprovalState != ApprovalApproved {
		return fmt.Errorf("proposal not Approved")
	}
	if status.ApprovalMechanismVersion != CandidateVersionV2 {
		return fmt.Errorf("candidate-v2 approval mechanism mismatch: %q", status.ApprovalMechanismVersion)
	}
	if status.ApprovedCandidateDigest == "" || status.ApprovedReviewContextDigest == "" {
		return fmt.Errorf("candidate-v2 approval requires candidate and review-context digests")
	}
	candidate, err := spec.candidateV2()
	if err != nil {
		return err
	}
	computedCandidate, err := CandidateDigestV2(candidate)
	if err != nil {
		return err
	}
	if computedCandidate != status.ApprovedCandidateDigest {
		return fmt.Errorf("approved candidate-v2 digest mismatch: approved=%s computed=%s", status.ApprovedCandidateDigest, computedCandidate)
	}
	contextV2, err := spec.reviewContextV2()
	if err != nil {
		return err
	}
	computedContext, err := ReviewContextDigestV2(contextV2)
	if err != nil {
		return err
	}
	if computedContext != status.ApprovedReviewContextDigest {
		return fmt.Errorf("approved review context digest mismatch: approved=%s computed=%s", status.ApprovedReviewContextDigest, computedContext)
	}
	return nil
}
