package proposal

import "fmt"

// ValidateApprovedCandidate verifies status/Spec consistency for apply-time
// authorization. Returns nil if the status indicates Approved and the
// ApprovedCandidateDigest matches CandidateDigest(spec) according to a
// supported ApprovalMechanismVersion.
func ValidateApprovedCandidate(spec *Spec, status *Status) error {
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
	if status.ApprovalMechanismVersion != "candidate-v1" {
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
