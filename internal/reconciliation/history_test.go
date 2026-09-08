package reconciliation

import (
	"reflect"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

func TestHistoryObservationAndUnorderedContribution(t *testing.T) {
	base := failedObservation(t, true, "obs-1")
	execution := base.Execution()
	execution.State = observationdomain.ExecutionCompleted
	execution.Completion = observationdomain.CompletedNormally
	execution.StartedAt = time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	execution.CompletedAt = time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC)
	completed, err := observationdomain.RestoreObservation(base.ID(), base.Spec(), base.Binding(), execution, base.Result(), base.Provenance())
	if err != nil {
		t.Fatal(err)
	}

	subject := subject(history.ScopeContainer, "")
	population := history.Population{
		Scope: subject.Scope, Target: subject.Target, Container: subject.Container, ImageIdentity: subject.ImageIdentity,
		ObservationContributions: []history.ObservationContribution{{ObservationID: "missing-observation"}},
	}
	got, err := ProjectPopulationHistory(subject, HistoryInputs{
		Populations:  []HistoryPopulationInput{{Population: population, Namespace: "default", Name: "history", UID: "history-uid"}},
		Observations: []HistoryObservationInput{{Observation: completed, CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.TimestampedEvents) != 3 {
		t.Fatalf("timestamped events = %#v", got.TimestampedEvents)
	}
	if got.TimestampedEvents[0].Kind != HistoryObservationCompleted || got.TimestampedEvents[1].Kind != HistoryObservationStarted || got.TimestampedEvents[2].Kind != HistoryObservationCreated {
		t.Fatalf("timestamp ordering = %#v", got.TimestampedEvents)
	}
	if len(got.UntimestampedFacts) != 2 || got.UntimestampedFacts[0].Kind != HistoryContributionPresent {
		t.Fatalf("untimestamped facts = %#v", got.UntimestampedFacts)
	}
	if !containsString(got.Limitations, HistoryLimitContributionTime) || !containsString(got.Limitations, HistoryLimitDanglingReference) {
		t.Fatalf("limitations = %#v", got.Limitations)
	}
}

func TestHistoryProposalApprovalAndAttemptCustody(t *testing.T) {
	ref := ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"}
	p := approvedCandidate(t, ref)
	created := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	pinput := ProposalInput{CandidateProposal: p, CreatedAt: created}
	pinput.Status.LastApprovalSnapshot = nil
	apply := applyInput(t, ref.UID, "apply", "2026-09-01T04:00:00Z", attempt.StateApplied, created)
	apply.Status.CompletedAt = "2026-09-01T05:00:00Z"
	rollback := rollbackInput(ref.UID, "missing-apply", "rollback", "2026-09-01T06:00:00Z", attempt.StateFailed, created)
	rollback.Status.CompletedAt = "2026-09-01T07:00:00Z"
	got, err := ProjectProposalHistory(ref, HistoryInputs{Proposals: []ProposalInput{pinput}, ApplyAttempts: []ApplyAttemptInput{apply}, RollbackAttempts: []RollbackAttemptInput{rollback}})
	if err != nil {
		t.Fatal(err)
	}
	if !containsKind(got.TimestampedEvents, HistoryProposalGenerated) || !containsKind(got.TimestampedEvents, HistoryApplyCompleted) || !containsKind(got.TimestampedEvents, HistoryRollbackCompleted) {
		t.Fatalf("events = %#v", got.TimestampedEvents)
	}
	if !containsDetail(got.UntimestampedFacts, "SOURCE_ATTEMPT_NOT_FOUND") {
		t.Fatalf("dangling rollback = %#v", got.UntimestampedFacts)
	}
	if !containsString(got.Limitations, HistoryLimitApprovalIncomplete) || !containsString(got.Limitations, HistoryLimitNonTransactional) {
		t.Fatalf("limitations = %#v", got.Limitations)
	}
}

func TestHistoryDoesNotInventContributionTimeAndIsBounded(t *testing.T) {
	subject := subject(history.ScopeContainer, "")
	population := history.Population{Scope: subject.Scope, Target: subject.Target, Container: subject.Container, ImageIdentity: subject.ImageIdentity, ObservationContributions: []history.ObservationContribution{{ObservationID: "o1"}, {ObservationID: "o2"}}}
	input := HistoryInputs{Populations: []HistoryPopulationInput{{Population: population}}, Limit: 1}
	first, err := ProjectPopulationHistory(subject, input)
	if err != nil {
		t.Fatal(err)
	}
	permuted := input
	permuted.Populations = append([]HistoryPopulationInput(nil), input.Populations...)
	second, err := ProjectPopulationHistory(subject, permuted)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Truncated || first.TotalCount != 4 || len(first.UntimestampedFacts)+len(first.TimestampedEvents) != 1 {
		t.Fatalf("bounded projection = %#v", first)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("permutation changed projection: %#v vs %#v", first, second)
	}
	if containsKind(first.TimestampedEvents, HistoryContributionPresent) {
		t.Fatal("contribution was presented as timestamped")
	}
}

func containsKind(events []HistoryEvent, want HistoryEventKind) bool {
	for _, event := range events {
		if event.Kind == want {
			return true
		}
	}
	return false
}
func containsDetail(events []HistoryEvent, want string) bool {
	for _, event := range events {
		if event.DetailCode == want {
			return true
		}
	}
	return false
}
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
