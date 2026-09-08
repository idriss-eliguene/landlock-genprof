package reconciliation

import (
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func applyInput(t *testing.T, proposalUID, name, started string, state string, created time.Time, mutations ...attempt.MutationRecord) ApplyAttemptInput {
	t.Helper()
	return ApplyAttemptInput{
		Namespace: "default", Name: name, UID: name + "-uid", CreatedAt: created,
		Spec:   attempt.Spec{ProposalNamespace: "default", ProposalName: "proposal", ProposalUID: proposalUID, ApprovedCandidateDigest: "sha256:approved", StartedAt: started},
		Status: attempt.Status{State: state, Mutations: mutations},
	}
}

func rollbackInput(proposalUID, sourceUID, name, started string, state string, created time.Time, mutations ...attempt.MutationRecord) RollbackAttemptInput {
	return RollbackAttemptInput{
		Namespace: "default", Name: name, UID: name + "-uid", CreatedAt: created,
		Spec:   attempt.RollbackSpec{SourceNamespace: "default", SourceName: "apply", SourceUID: sourceUID, ProposalNamespace: "default", ProposalName: "proposal", ProposalUID: proposalUID, ApprovedCandidateDigest: "sha256:approved", StartedAt: started},
		Status: attempt.Status{State: state, Mutations: mutations},
	}
}

func confirmedMutation(result string) attempt.MutationRecord {
	return attempt.MutationRecord{ID: "m1", IntendedAfterDigest: "sha256:intended", ObservedAfter: "{}", ObservedAfterDigest: "sha256:intended", Result: result}
}

func projectApproved(t *testing.T, applies []ApplyAttemptInput, rollbacks []RollbackAttemptInput) VerificationProjection {
	t.Helper()
	candidate := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"})
	got, err := ProjectVerification(subject(history.ScopeContainer, ""), []CandidateProposal{candidate}, applies, rollbacks)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestProjectVerificationGovernanceAndDerivation(t *testing.T) {
	tests := []struct {
		name       string
		candidate  CandidateProposal
		wantDerive DerivationState
		wantGovern GovernanceState
		wantApply  ApplicationState
	}{
		{"draft", func() CandidateProposal {
			base := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"})
			base.Status = proposal.Status{ApprovalState: proposal.ApprovalDraft}
			return base
		}(), DerivationStructurallyValid, GovernanceDraft, ApplicationNoCurrentPolicy},
		{"approved no attempt", approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"}), DerivationStructurallyValid, GovernanceApproved, ApplicationNotAttempted},
		{"rejected", func() CandidateProposal {
			base := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"})
			base.Status = proposal.Status{ApprovalState: proposal.ApprovalRejected}
			return base
		}(), DerivationStructurallyValid, GovernanceRejected, ApplicationNoCurrentPolicy},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ProjectVerification(subject(history.ScopeContainer, ""), []CandidateProposal{test.candidate}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.Derivation != test.wantDerive || got.Governance != test.wantGovern || got.Application != test.wantApply {
				t.Fatalf("projection = %#v, want derivation=%q governance=%q application=%q", got, test.wantDerive, test.wantGovern, test.wantApply)
			}
			if got.Behavioral != BehavioralVerificationUnknown {
				t.Fatalf("behavioral = %q, want UNKNOWN", got.Behavioral)
			}
		})
	}

	stale := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"})
	stale.Status.ApprovedCandidateDigest = "sha256:stale"
	got, err := ProjectVerification(subject(history.ScopeContainer, ""), []CandidateProposal{stale}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Governance != GovernanceStaleApproval || got.Application != ApplicationNoCurrentPolicy {
		t.Fatalf("stale projection = %#v", got)
	}
	stale = approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"})
	stale.Status.ApprovedReviewContextDigest = "sha256:stale-review-context"
	got, err = ProjectVerification(subject(history.ScopeContainer, ""), []CandidateProposal{stale}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Governance != GovernanceStaleApproval {
		t.Fatalf("stale review-context projection = %#v", got)
	}
}

func TestProjectVerificationApplicationHistory(t *testing.T) {
	times := func(seconds int) string {
		return time.Date(2026, 9, 1, 0, 0, seconds, 0, time.UTC).Format(time.RFC3339Nano)
	}
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		applies   []ApplyAttemptInput
		rollbacks []RollbackAttemptInput
		want      ApplicationState
	}{
		{"no attempt", nil, nil, ApplicationNotAttempted},
		{"applied", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateApplied, base, confirmedMutation(attempt.ResultSucceeded))}, nil, ApplicationApplied},
		{"partial", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StatePartiallyApplied, base, confirmedMutation(attempt.ResultSucceeded), attempt.MutationRecord{ID: "m2", Result: attempt.ResultFailed})}, nil, ApplicationPartiallyApplied},
		{"failed", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateFailed, base)}, nil, ApplicationFailed},
		{"unknown", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateOutcomeUnknown, base)}, nil, ApplicationOutcomeUnknown},
		{"apply then rollback", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateApplied, base)}, []RollbackAttemptInput{rollbackInput("proposal-uid", "a-uid", "r", times(2), attempt.StateApplied, base.Add(time.Second))}, ApplicationRolledBack},
		{"rollback failed", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateApplied, base)}, []RollbackAttemptInput{rollbackInput("proposal-uid", "a-uid", "r", times(2), attempt.StateFailed, base.Add(time.Second))}, ApplicationRollbackFailed},
		{"rollback unknown", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateApplied, base)}, []RollbackAttemptInput{rollbackInput("proposal-uid", "a-uid", "r", times(2), attempt.StateOutcomeUnknown, base.Add(time.Second))}, ApplicationRollbackOutcomeUnknown},
		{"apply then failed reapply", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateApplied, base), applyInput(t, "proposal-uid", "b", times(2), attempt.StateFailed, base.Add(time.Second))}, nil, ApplicationFailed},
		{"unknown then applied", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateOutcomeUnknown, base), applyInput(t, "proposal-uid", "b", times(2), attempt.StateApplied, base.Add(time.Second))}, nil, ApplicationApplied},
		{"applied then unknown", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(1), attempt.StateApplied, base), applyInput(t, "proposal-uid", "b", times(2), attempt.StateOutcomeUnknown, base.Add(time.Second))}, nil, ApplicationOutcomeUnknown},
		{"rollback then applied", []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", times(2), attempt.StateApplied, base.Add(2*time.Second))}, []RollbackAttemptInput{rollbackInput("proposal-uid", "a-uid", "r", times(1), attempt.StateApplied, base)}, ApplicationApplied},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := projectApproved(t, test.applies, test.rollbacks)
			if got.Application != test.want {
				t.Fatalf("application = %q, want %q", got.Application, test.want)
			}
		})
	}
}

func TestProjectVerificationAttemptJoinAndOrdering(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	first := applyInput(t, "proposal-uid", "z", "2026-09-01T00:00:01Z", attempt.StateFailed, base, confirmedMutation(attempt.ResultFailed))
	second := applyInput(t, "proposal-uid", "a", "2026-09-01T00:00:01Z", attempt.StateApplied, base, confirmedMutation(attempt.ResultSucceeded))
	got := projectApproved(t, []ApplyAttemptInput{first, second}, nil)
	permuted := projectApproved(t, []ApplyAttemptInput{second, first}, nil)
	if got.Application != ApplicationApplied || permuted.Application != ApplicationApplied {
		t.Fatalf("equal-time ordering was input-dependent: %q, %q", got.Application, permuted.Application)
	}

	other := applyInput(t, "other-proposal", "other", "2026-09-01T00:00:03Z", attempt.StateFailed, base.Add(3*time.Second))
	got = projectApproved(t, []ApplyAttemptInput{other}, nil)
	if got.Application != ApplicationNotAttempted || got.CurrentAttemptsIgnored != 1 {
		t.Fatalf("different ProposalUID was not ignored: %#v", got)
	}
	// Equal candidate digests do not make a different ProposalUID relevant.
	other.Spec.ApprovedCandidateDigest = "sha256:approved"
	got = projectApproved(t, []ApplyAttemptInput{other}, nil)
	if got.Application != ApplicationNotAttempted {
		t.Fatalf("digest equality transferred authority: %q", got.Application)
	}
}

func TestStructuralKnowledgeRequiresExactReadbackDigest(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		status attempt.Status
		want   StructuralKnowledge
	}{
		{"none", attempt.Status{State: attempt.StateApplied}, StructuralNone},
		{"confirmed", attempt.Status{State: attempt.StateApplied, Mutations: []attempt.MutationRecord{confirmedMutation(attempt.ResultSucceeded)}}, StructuralConfirmedAtApplication},
		{"missing observed", attempt.Status{State: attempt.StateApplied, Mutations: []attempt.MutationRecord{{Result: attempt.ResultSucceeded, IntendedAfterDigest: "sha256:x"}}}, StructuralNotConfirmed},
		{"wrong digest", attempt.Status{State: attempt.StateApplied, Mutations: []attempt.MutationRecord{{Result: attempt.ResultSucceeded, ObservedAfter: "{}", IntendedAfterDigest: "sha256:x", ObservedAfterDigest: "sha256:y"}}}, StructuralNotConfirmed},
		{"partial", attempt.Status{State: attempt.StatePartiallyApplied, Mutations: []attempt.MutationRecord{confirmedMutation(attempt.ResultSucceeded), attempt.MutationRecord{Result: attempt.ResultFailed}}}, StructuralPartial},
		{"unknown", attempt.Status{State: attempt.StateOutcomeUnknown, Mutations: []attempt.MutationRecord{{Result: attempt.ResultUnknown}}}, StructuralUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := projectApproved(t, []ApplyAttemptInput{applyInput(t, "proposal-uid", "a", "2026-09-01T00:00:01Z", test.status.State, base, test.status.Mutations...)}, nil)
			if got.Structural != test.want {
				t.Fatalf("structural = %q, want %q", got.Structural, test.want)
			}
		})
	}
}

func TestProjectVerificationAmbiguousGovernanceDoesNotProjectApplication(t *testing.T) {
	first := approvedCandidate(t, ProposalRef{Name: "one", UID: "uid-one"})
	second := approvedCandidate(t, ProposalRef{Name: "two", UID: "uid-two"})
	got, err := ProjectVerification(subject(history.ScopeContainer, ""), []CandidateProposal{first, second}, []ApplyAttemptInput{applyInput(t, "uid-one", "a", "2026-09-01T00:00:01Z", attempt.StateApplied, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), confirmedMutation(attempt.ResultSucceeded))}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Governance != GovernanceAmbiguous || got.Application != ApplicationNotProjectableAmbiguousPolicy || got.Proposal != nil {
		t.Fatalf("ambiguous projection = %#v", got)
	}
	if got.Behavioral != BehavioralVerificationUnknown {
		t.Fatalf("behavioral = %q, want UNKNOWN", got.Behavioral)
	}
}
