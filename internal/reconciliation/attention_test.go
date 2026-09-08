package reconciliation

import (
	"reflect"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func proposalInput(t *testing.T, name string, created time.Time) ProposalInput {
	t.Helper()
	return ProposalInput{CandidateProposal: approvedCandidate(t, ProposalRef{Namespace: "default", Name: name, UID: name + "-uid"}), CreatedAt: created}
}

func TestAttentionCapabilityPredicate(t *testing.T) {
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	proposal := proposalInput(t, "approved", created)
	base := history.Population{Scope: history.ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CapabilityAccesses: []history.CapabilityAccessRecord{{Name: "CAP_CHOWN"}}}
	got, err := EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: base, Proposals: []ProposalInput{proposal}})
	if err != nil || hasCategory(got, AttentionCapabilityOutsideApprovedPolicy) {
		t.Fatalf("covered capability attention = %#v, err=%v", got, err)
	}
	base.CapabilityAccesses = append(base.CapabilityAccesses, history.CapabilityAccessRecord{Name: "CAP_SYS_ADMIN"})
	got, err = EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: base, Proposals: []ProposalInput{proposal}})
	if err != nil || !hasCategory(got, AttentionCapabilityOutsideApprovedPolicy) {
		t.Fatalf("outside capability attention = %#v, err=%v", got, err)
	}
	reason := reasonFor(got, AttentionCapabilityOutsideApprovedPolicy)
	if !reflect.DeepEqual(reason.EvidenceRefs, []string{"CAP_SYS_ADMIN"}) {
		t.Fatalf("capability refs = %#v", reason.EvidenceRefs)
	}

	base.CapabilityAccesses = nil
	base.FilesystemAccesses = []history.FileAccessRecord{{Path: "/etc/config"}}
	base.NetworkAccesses = []history.NetworkAccessRecord{{Port: 443}}
	base.SyscallAccesses = []history.SyscallAccessRecord{{Name: "execve"}}
	got, err = EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: base, Proposals: []ProposalInput{proposal}})
	if err != nil || hasCategory(got, AttentionCapabilityOutsideApprovedPolicy) {
		t.Fatalf("non-capability facts produced A: %#v, err=%v", got, err)
	}

	draft := proposal
	draft.Status.ApprovalState = "Draft"
	base.CapabilityAccesses = []history.CapabilityAccessRecord{{Name: "CAP_SYS_ADMIN"}}
	got, err = EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: base, Proposals: []ProposalInput{draft}})
	if err != nil || hasCategory(got, AttentionCapabilityOutsideApprovedPolicy) {
		t.Fatalf("non-approved proposal produced A: %#v, err=%v", got, err)
	}
}

func TestAttentionApplicationPredicates(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	states := []struct {
		name  string
		state string
		wantB bool
		wantC bool
	}{
		{"not attempted", "", true, false},
		{"failed", attempt.StateFailed, true, false},
		{"partial", attempt.StatePartiallyApplied, true, false},
		{"rolled back", "rollback", true, false},
		{"rollback failed", "rollback-failed", true, false},
		{"unknown", attempt.StateOutcomeUnknown, false, true},
		{"applied", attempt.StateApplied, false, false},
	}
	for _, test := range states {
		t.Run(test.name, func(t *testing.T) {
			var applies []ApplyAttemptInput
			var rollbacks []RollbackAttemptInput
			if test.state == "rollback" || test.state == "rollback-failed" {
				rollbackState := attempt.StateApplied
				if test.state == "rollback-failed" {
					rollbackState = attempt.StateFailed
				}
				rollbacks = []RollbackAttemptInput{rollbackInput("approved-uid", "apply-uid", "rollback", "2026-09-01T00:00:02Z", rollbackState, base.Add(time.Second))}
			} else if test.state != "" {
				applies = []ApplyAttemptInput{applyInput(t, "approved-uid", "apply", "2026-09-01T00:00:01Z", test.state, base)}
			}
			proposal := proposalInput(t, "approved", base)
			got, err := EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: history.Population{Scope: history.ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: subject(history.ScopeContainer, "").ImageIdentity}, Proposals: []ProposalInput{proposal}, ApplyAttempts: applies, RollbackAttempts: rollbacks})
			if err != nil {
				t.Fatal(err)
			}
			if hasCategory(got, AttentionApprovedNotApplied) != test.wantB || hasCategory(got, AttentionApplicationStateUnknown) != test.wantC {
				t.Fatalf("attention = %#v, want B=%v C=%v", got, test.wantB, test.wantC)
			}
		})
	}
	// IN_PROGRESS is neither ordinary not-applied nor uncertain outcome.
	proposal := proposalInput(t, "approved", base)
	got, err := EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), Proposals: []ProposalInput{proposal}, ApplyAttempts: []ApplyAttemptInput{applyInput(t, "approved-uid", "apply", "2026-09-01T00:00:01Z", attempt.StateInProgress, base)}})
	if err != nil || hasCategory(got, AttentionApprovedNotApplied) || hasCategory(got, AttentionApplicationStateUnknown) {
		t.Fatalf("in-progress attention = %#v, err=%v", got, err)
	}
}

func TestAttentionDerivationBaselineAndContributionDelta(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	p1 := proposalInput(t, "p1", base)
	p1.Spec.Provenance = &proposal.ProposalProvenance{PopulationScope: "CONTAINER", ObservationIDs: []string{"O1"}}
	p2 := proposalInput(t, "p2", base.Add(time.Minute))
	p2.Spec.Provenance = &proposal.ProposalProvenance{PopulationScope: "CONTAINER", ObservationIDs: []string{"O1", "O2"}}
	population := history.Population{Scope: history.ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: subject(history.ScopeContainer, "").ImageIdentity, ObservationContributions: []history.ObservationContribution{{ObservationID: "O1"}, {ObservationID: "O2"}}}
	input := AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: population, Proposals: []ProposalInput{p1}}
	got, err := EvaluateAttention(input)
	if err != nil || !hasCategory(got, AttentionNewContributionSinceCandidate) {
		t.Fatalf("P1 delta = %#v, err=%v", got, err)
	}
	input.Proposals = []ProposalInput{p1, p2}
	got, err = EvaluateAttention(input)
	if err != nil || hasCategory(got, AttentionNewContributionSinceCandidate) {
		t.Fatalf("P2 baseline delta = %#v, err=%v", got, err)
	}
	population.ObservationContributions = append(population.ObservationContributions, history.ObservationContribution{ObservationID: "O3"})
	input.Population = population
	got, err = EvaluateAttention(input)
	if err != nil || !hasCategory(got, AttentionNewContributionSinceCandidate) {
		t.Fatalf("zero-novel O3 delta = %#v, err=%v", got, err)
	}
	if got.Reasons[len(got.Reasons)-1].Category != AttentionNewContributionSinceCandidate || !reflect.DeepEqual(reasonFor(got, AttentionNewContributionSinceCandidate).ObservationRefs, []string{"O3"}) {
		t.Fatalf("O3 refs = %#v", reasonFor(got, AttentionNewContributionSinceCandidate))
	}

	legacy := p2
	legacy.CreatedAt = base.Add(2 * time.Minute)
	legacy.Spec.CandidateVersion = ""
	got, err = EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: population, Proposals: []ProposalInput{p2, legacy}})
	if err != nil || !hasCategory(got, AttentionNewContributionSinceCandidate) {
		t.Fatalf("candidate-v1 incorrectly became baseline: %#v, err=%v", got, err)
	}
}

func TestAttentionContributionDeltaAndCapabilityOutsidePolicyAreIndependent(t *testing.T) {
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	candidate := proposalInput(t, "approved", created)
	population := history.Population{Scope: history.ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: subject(history.ScopeContainer, "").ImageIdentity, CapabilityAccesses: []history.CapabilityAccessRecord{{Name: "CAP_SYS_ADMIN"}}, ObservationContributions: []history.ObservationContribution{{ObservationID: "O1"}, {ObservationID: "O2"}}}
	got, err := EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: population, Proposals: []ProposalInput{candidate}})
	if err != nil || !hasCategory(got, AttentionNewContributionSinceCandidate) || !hasCategory(got, AttentionCapabilityOutsideApprovedPolicy) {
		t.Fatalf("independent D+A attention = %#v, err=%v", got, err)
	}
}

func TestAttentionObservationFailedBoundAndUnbound(t *testing.T) {
	unbound := failedObservation(t, false, "failed-unbound")
	bound := failedObservation(t, true, "failed-bound")
	got, err := EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), FailedObservations: []observationdomain.Observation{unbound, bound}})
	if err != nil {
		t.Fatal(err)
	}
	reasons := reasonsFor(got, AttentionObservationFailed)
	if len(reasons) != 2 || reasons[0].ExplanationCode != "OBSERVATION_FAILED_BOUND" || reasons[1].ExplanationCode != "OBSERVATION_FAILED_UNBOUND" {
		t.Fatalf("failed observation reasons = %#v", reasons)
	}
	if reasons[0].Subject == nil || reasons[1].Subject != nil {
		t.Fatalf("bound/unbound subjects = %#v", reasons)
	}
	completed := completedObservation(t, "completed")
	got, err = EvaluateAttention(AttentionInput{Subject: subject(history.ScopeContainer, ""), FailedObservations: []observationdomain.Observation{completed}})
	if err != nil || hasCategory(got, AttentionObservationFailed) {
		t.Fatalf("completed observation produced E: %#v, err=%v", got, err)
	}
}

func TestAttentionAmbiguityAndDeterminism(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	first := proposalInput(t, "one", base)
	second := proposalInput(t, "two", base.Add(time.Minute))
	population := history.Population{Scope: history.ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: subject(history.ScopeContainer, "").ImageIdentity, ObservationContributions: []history.ObservationContribution{{ObservationID: "O2"}}}
	input := AttentionInput{Subject: subject(history.ScopeContainer, ""), Population: population, Proposals: []ProposalInput{first, second}}
	got, err := EvaluateAttention(input)
	if err != nil || !hasCategory(got, AttentionMultipleValidApprovedProposals) {
		t.Fatalf("ambiguity = %#v, err=%v", got, err)
	}
	if hasCategory(got, AttentionCapabilityOutsideApprovedPolicy) || hasCategory(got, AttentionApprovedNotApplied) || hasCategory(got, AttentionApplicationStateUnknown) {
		t.Fatalf("ambiguous governance leaked policy-specific attention: %#v", got)
	}
	input.Proposals = []ProposalInput{second, first}
	permuted, err := EvaluateAttention(input)
	if err != nil || !reflect.DeepEqual(got, permuted) {
		t.Fatalf("input permutation changed attention: %#v vs %#v", got, permuted)
	}
}

func failedObservation(t *testing.T, bound bool, id string) observationdomain.Observation {
	return terminalObservation(t, bound, id, observationdomain.ExecutionFailed)
}

func terminalObservation(t *testing.T, bound bool, id string, terminal observationdomain.ExecutionState) observationdomain.Observation {
	t.Helper()
	cluster, err := observationdomain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	slot := observationdomain.ContainerSlot{Workload: observationdomain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: observationdomain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "workload-uid"}, Container: "app"}
	spec, err := observationdomain.NewObservationSpec(observationdomain.RequestedTarget{Slot: slot}, []string{"capabilities"}, time.Minute, "session")
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observationdomain.NewObservation(observationdomain.ObservationID(id), spec)
	if err != nil {
		t.Fatal(err)
	}
	if bound {
		revision, err := observationdomain.NewContainerImageRevision(slot, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		if err != nil {
			t.Fatal(err)
		}
		runtime := observationdomain.RuntimeContainerInstance{Slot: slot, PodUID: "pod-uid", ImageRevision: &revision}
		resolved, err := observationdomain.NewResolvedTargetSet([]observationdomain.RuntimeContainerInstance{runtime})
		if err != nil {
			t.Fatal(err)
		}
		if err := obs.Bind(resolved, observationdomain.BackendIdentity{Kind: "core", Version: "v1"}, []observationdomain.ContainerImageRevision{revision}); err != nil {
			t.Fatal(err)
		}
	}
	if err := obs.Transition(observationdomain.ExecutionStarting, observationdomain.CompletionReason("")); err != nil {
		t.Fatal(err)
	}
	if terminal == observationdomain.ExecutionFailed {
		if err := obs.Transition(observationdomain.ExecutionFailed, observationdomain.BackendFailure); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := obs.Transition(observationdomain.ExecutionRunning, observationdomain.CompletionReason("")); err != nil {
			t.Fatal(err)
		}
		if err := obs.Transition(observationdomain.ExecutionCompleting, observationdomain.CompletionReason("")); err != nil {
			t.Fatal(err)
		}
		if err := obs.Transition(terminal, observationdomain.CompletedNormally); err != nil {
			t.Fatal(err)
		}
	}
	return obs
}

func completedObservation(t *testing.T, id string) observationdomain.Observation {
	return terminalObservation(t, false, id, observationdomain.ExecutionCompleted)
}

func hasCategory(result AttentionResult, category AttentionCategory) bool {
	return len(reasonsFor(result, category)) > 0
}

func reasonsFor(result AttentionResult, category AttentionCategory) []AttentionReason {
	var reasons []AttentionReason
	for _, reason := range result.Reasons {
		if reason.Category == category {
			reasons = append(reasons, reason)
		}
	}
	return reasons
}

func reasonFor(result AttentionResult, category AttentionCategory) AttentionReason {
	for _, reason := range result.Reasons {
		if reason.Category == category {
			return reason
		}
	}
	return AttentionReason{}
}
