// Package reconciliation contains the pure v0.8 governance projection domain.
// It deliberately operates only on already-loaded history and proposal values.
package reconciliation

import (
	"fmt"
	"sort"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

// EnvironmentSubject is the v0.8 read-model view of the existing population
// identity. It is an alias, not a second persisted identity.
type EnvironmentSubject = history.PopulationIdentity

// EnvironmentSubjectFromPopulationFingerprint applies the existing legacy
// scope normalization before exposing a subject to reconciliation.
func EnvironmentSubjectFromPopulationFingerprint(fingerprint history.PopulationFingerprint) (EnvironmentSubject, error) {
	return fingerprint.Identity()
}

// SubjectsEqual compares validated, explicit population identities. Workload
// UID is not a member of PopulationIdentity and therefore cannot participate.
func SubjectsEqual(left, right EnvironmentSubject) bool {
	if left.Validate() != nil || right.Validate() != nil {
		return false
	}
	return left == right
}

// ProposalRef identifies a Proposal object without transferring authority
// between objects that happen to contain equal candidate digests.
type ProposalRef struct {
	Namespace string
	Name      string
	UID       string
}

// CandidateProposal is the pure, already-loaded input to selection.
type CandidateProposal struct {
	Ref    ProposalRef
	Spec   proposal.Spec
	Status proposal.Status
}

// SelectionState is the complete result of ApprovedPolicy selection.
type SelectionState string

const (
	SelectionNone      SelectionState = "NONE"
	SelectionSelected  SelectionState = "SELECTED"
	SelectionAmbiguous SelectionState = "AMBIGUOUS"
)

// SelectionRejection records why an otherwise supplied Proposal was not a
// current candidate-v2 authority. It is diagnostic domain data only.
type SelectionRejection struct {
	Ref    ProposalRef
	Reason string
}

// ApprovedPolicySelection preserves ambiguity and stale-approval evidence
// for later projections. Proposal is set only in the SELECTED state.
type ApprovedPolicySelection struct {
	State              SelectionState
	Proposal           *CandidateProposal
	AmbiguousRefs      []ProposalRef
	RejectedCandidates []SelectionRejection
}

// SubjectMatchesCandidateV2 applies the frozen candidate-v2 subject basis.
// Candidate-v2 is CONTAINER-only, so BINARY subjects and BinaryPath-bearing
// identities cannot match it.
func SubjectMatchesCandidateV2(subject EnvironmentSubject, candidate proposal.CandidateV2) bool {
	if subject.Validate() != nil || candidate.Validate() != nil {
		return false
	}
	if subject.Scope != history.ScopeContainer || subject.BinaryPath != "" {
		return false
	}
	return candidate.Subject.Scope == proposal.CandidateV2ScopeContainer &&
		candidate.Subject.Target == subject.Target &&
		candidate.Subject.Container == subject.Container &&
		candidate.Subject.ImageIdentity == subject.ImageIdentity
}

// SelectApprovedPolicy selects current candidate-v2 authority for a subject.
// It never selects by time, object name, UID, or equal digest. Approval is
// validated per Proposal object and multiple valid objects remain ambiguous.
func SelectApprovedPolicy(subject EnvironmentSubject, candidates []CandidateProposal) ApprovedPolicySelection {
	result := ApprovedPolicySelection{State: SelectionNone}
	if subject.Validate() != nil {
		return result
	}

	valid := make([]CandidateProposal, 0, len(candidates))
	for index := range candidates {
		candidate := &candidates[index]
		version, err := candidate.Spec.NormalizedCandidateVersion()
		if err != nil || version != proposal.CandidateVersionV2 {
			result.RejectedCandidates = append(result.RejectedCandidates, rejection(candidate.Ref, "not candidate-v2"))
			continue
		}
		candidateValue, err := candidate.Spec.CandidateV2()
		if err != nil {
			result.RejectedCandidates = append(result.RejectedCandidates, rejection(candidate.Ref, err.Error()))
			continue
		}
		if !SubjectMatchesCandidateV2(subject, candidateValue) {
			result.RejectedCandidates = append(result.RejectedCandidates, rejection(candidate.Ref, "subject mismatch"))
			continue
		}
		if candidate.Status.ApprovalState != proposal.ApprovalApproved {
			result.RejectedCandidates = append(result.RejectedCandidates, rejection(candidate.Ref, "approval state is not Approved"))
			continue
		}
		if err := proposal.ValidateApprovedCandidate(&candidate.Spec, &candidate.Status); err != nil {
			result.RejectedCandidates = append(result.RejectedCandidates, rejection(candidate.Ref, fmt.Sprintf("approval validation failed: %v", err)))
			continue
		}
		valid = append(valid, *candidate)
	}

	sort.Slice(result.RejectedCandidates, func(left, right int) bool {
		return compareRefs(result.RejectedCandidates[left].Ref, result.RejectedCandidates[right].Ref) < 0
	})

	switch len(valid) {
	case 0:
		return result
	case 1:
		result.State = SelectionSelected
		result.Proposal = &valid[0]
		return result
	default:
		result.State = SelectionAmbiguous
		result.AmbiguousRefs = make([]ProposalRef, len(valid))
		for index := range valid {
			result.AmbiguousRefs[index] = valid[index].Ref
		}
		sort.Slice(result.AmbiguousRefs, func(left, right int) bool {
			return compareRefs(result.AmbiguousRefs[left], result.AmbiguousRefs[right]) < 0
		})
		return result
	}
}

func rejection(ref ProposalRef, reason string) SelectionRejection {
	return SelectionRejection{Ref: ref, Reason: reason}
}

func compareRefs(left, right ProposalRef) int {
	for _, pair := range [][2]string{{left.Namespace, right.Namespace}, {left.Name, right.Name}, {left.UID, right.UID}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
