package proposal

import (
	"strings"
	"testing"
)

func TestValidateApprovedCandidate_Matrix(t *testing.T) {
	// Base spec and its computed digest
	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: "podlock"}
	computed, err := CandidateDigest(spec)
	if err != nil {
		t.Fatalf("CandidateDigest error: %v", err)
	}

	cases := []struct {
		name    string
		status  Status
		wantErr bool
	}{
		{"Approved valid", Status{ApprovalState: ApprovalApproved, ApprovedCandidateDigest: computed, ApprovalMechanismVersion: "candidate-v1"}, false},
		{"Draft with binding", Status{ApprovalState: ApprovalDraft, ApprovedCandidateDigest: computed, ApprovalMechanismVersion: "candidate-v1"}, true},
		{"Reviewed with binding", Status{ApprovalState: ApprovalReviewed, ApprovedCandidateDigest: computed, ApprovalMechanismVersion: "candidate-v1"}, true},
		{"Rejected with binding", Status{ApprovalState: ApprovalRejected, ApprovedCandidateDigest: computed, ApprovalMechanismVersion: "candidate-v1"}, true},
		{"Approved missing digest", Status{ApprovalState: ApprovalApproved, ApprovedCandidateDigest: "", ApprovalMechanismVersion: "candidate-v1"}, true},
		{"Approved malformed digest", Status{ApprovalState: ApprovalApproved, ApprovedCandidateDigest: "sha256:bad", ApprovalMechanismVersion: "candidate-v1"}, true},
		{"Approved uppercase hex digest", Status{ApprovalState: ApprovalApproved, ApprovedCandidateDigest: strings.ToUpper(computed), ApprovalMechanismVersion: "candidate-v1"}, true},
		{"Approved unsupported mechanism", Status{ApprovalState: ApprovalApproved, ApprovedCandidateDigest: computed, ApprovalMechanismVersion: "candidate-v2"}, true},
		{"Approved mismatched digest", Status{ApprovalState: ApprovalApproved, ApprovedCandidateDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", ApprovalMechanismVersion: "candidate-v1"}, true},
		{"Approved missing mechanism", Status{ApprovalState: ApprovalApproved, ApprovedCandidateDigest: computed, ApprovalMechanismVersion: ""}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateApprovedCandidate(&spec, &tc.status)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateApprovedCandidate() error = %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
