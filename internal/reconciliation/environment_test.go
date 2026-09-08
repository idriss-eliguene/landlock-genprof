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

func environmentPopulation(subject EnvironmentSubject, scope history.PopulationScope, binaryPath string) history.Population {
	return history.Population{Scope: scope, Target: subject.Target, Container: subject.Container, ImageIdentity: subject.ImageIdentity, BinaryPath: binaryPath}
}

func TestProjectEnvironmentPopulationObservationDeduplication(t *testing.T) {
	subject := subject(history.ScopeContainer, "")
	population := environmentPopulation(subject, history.ScopeContainer, "")
	observation := failedObservation(t, true, "observation-1")
	projection, err := ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{population}, Observations: []observationdomain.Observation{observation}})
	if err != nil || projection.TotalCount != 1 || len(projection.Entries) != 1 {
		t.Fatalf("deduplication = %#v, err=%v", projection, err)
	}
	entry := projection.Entries[0]
	if entry.PopulationPresence != PopulationPresent || !reflect.DeepEqual(entry.ObservationRefs, []string{"observation-1"}) {
		t.Fatalf("entry = %#v", entry)
	}

	projection, err = ProjectEnvironment(EnvironmentInputs{Observations: []observationdomain.Observation{observation, observation}})
	if err != nil || projection.TotalCount != 1 || projection.Entries[0].PopulationPresence != PopulationNotFound {
		t.Fatalf("prospective deduplication = %#v, err=%v", projection, err)
	}
}

func TestProjectEnvironmentProspectiveAndUnboundFailures(t *testing.T) {
	bound := failedObservation(t, true, "bound-failed")
	unbound := failedObservation(t, false, "unbound-failed")
	projection, err := ProjectEnvironment(EnvironmentInputs{Observations: []observationdomain.Observation{bound, unbound}})
	if err != nil || projection.TotalCount != 1 || projection.UnattributedFailedObservationCount != 1 {
		t.Fatalf("failed observation projection = %#v, err=%v", projection, err)
	}
	if !hasCategory(AttentionResult{Reasons: projection.Entries[0].Attention}, AttentionObservationFailed) {
		t.Fatalf("bound failed attention missing: %#v", projection.Entries[0])
	}
}

func TestProjectEnvironmentScopeAndIdentityIsolation(t *testing.T) {
	base := subject(history.ScopeContainer, "")
	binary := environmentPopulation(base, history.ScopeBinary, "/app/server")
	container := environmentPopulation(base, history.ScopeContainer, "")
	changedImage := container
	changedImage.ImageIdentity = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	changedContainer := container
	changedContainer.Container = "sidecar"
	projection, err := ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{changedContainer, changedImage, binary, container}})
	if err != nil || projection.TotalCount != 4 {
		t.Fatalf("identity rows = %#v, err=%v", projection, err)
	}
	for _, entry := range projection.Entries {
		if entry.Subject.Scope == history.ScopeBinary && entry.Governance != GovernanceNone {
			t.Fatalf("binary row inherited governance: %#v", entry)
		}
	}
}

func TestProjectEnvironmentGovernanceAttentionAndApplication(t *testing.T) {
	subject := subject(history.ScopeContainer, "")
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	approved := proposalInput(t, "approved", created)
	population := environmentPopulation(subject, history.ScopeContainer, "")
	population.CapabilityAccesses = []history.CapabilityAccessRecord{{Name: "CAP_SYS_ADMIN"}}
	population.ObservationContributions = []history.ObservationContribution{{ObservationID: "O1"}, {ObservationID: "O2"}}
	projection, err := ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{population}, Proposals: []ProposalInput{approved}})
	if err != nil || projection.Entries[0].Governance != GovernanceApproved || !hasCategory(AttentionResult{Reasons: projection.Entries[0].Attention}, AttentionCapabilityOutsideApprovedPolicy) || !hasCategory(AttentionResult{Reasons: projection.Entries[0].Attention}, AttentionNewContributionSinceCandidate) {
		t.Fatalf("approved A+D projection = %#v, err=%v", projection, err)
	}

	second := proposalInput(t, "second", created.Add(time.Minute))
	projection, err = ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{population}, Proposals: []ProposalInput{approved, second}})
	if err != nil || projection.Entries[0].Governance != GovernanceAmbiguous || projection.Entries[0].Application != ApplicationNotProjectableAmbiguousPolicy || !hasCategory(AttentionResult{Reasons: projection.Entries[0].Attention}, AttentionMultipleValidApprovedProposals) {
		t.Fatalf("ambiguous projection = %#v, err=%v", projection, err)
	}

	unknown := applyInput(t, "approved-uid", "apply", "2026-09-01T00:00:01Z", attempt.StateOutcomeUnknown, created)
	projection, err = ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{population}, Proposals: []ProposalInput{approved}, ApplyAttempts: []ApplyAttemptInput{unknown}})
	if err != nil || projection.Entries[0].Application != ApplicationOutcomeUnknown || !hasCategory(AttentionResult{Reasons: projection.Entries[0].Attention}, AttentionApplicationStateUnknown) {
		t.Fatalf("unknown application projection = %#v, err=%v", projection, err)
	}

	rollback := rollbackInput("approved-uid", "apply-uid", "rollback", "2026-09-01T00:00:02Z", attempt.StateApplied, created.Add(time.Minute))
	projection, err = ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{population}, Proposals: []ProposalInput{approved}, ApplyAttempts: []ApplyAttemptInput{applyInput(t, "approved-uid", "apply", "2026-09-01T00:00:01Z", attempt.StateApplied, created)}, RollbackAttempts: []RollbackAttemptInput{rollback}})
	if err != nil || projection.Entries[0].Application != ApplicationRolledBack || !hasCategory(AttentionResult{Reasons: projection.Entries[0].Attention}, AttentionApprovedNotApplied) {
		t.Fatalf("rollback projection = %#v, err=%v", projection, err)
	}
}

func TestProjectEnvironmentDerivationBaselineAndVersionIsolation(t *testing.T) {
	subject := subject(history.ScopeContainer, "")
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	old := proposalInput(t, "old", created)
	old.Spec.Provenance = &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: []string{"O1"}}
	newer := proposalInput(t, "newer", created.Add(time.Minute))
	newer.Spec.Provenance = &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: []string{"O1", "O2"}}
	population := environmentPopulation(subject, history.ScopeContainer, "")
	population.ObservationContributions = []history.ObservationContribution{{ObservationID: "O1"}, {ObservationID: "O2"}}
	projection, err := ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{population}, Proposals: []ProposalInput{old, newer}})
	if err != nil || hasCategory(AttentionResult{Reasons: projection.Entries[0].Attention}, AttentionNewContributionSinceCandidate) {
		t.Fatalf("newer derivation baseline did not resolve D: %#v, err=%v", projection, err)
	}
	v1 := proposalInput(t, "v1", created.Add(2*time.Minute))
	v1.Spec.CandidateVersion = proposal.CandidateVersionV1
	projection, err = ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{population}, Proposals: []ProposalInput{old, v1}})
	if err != nil || !hasCategory(AttentionResult{Reasons: projection.Entries[0].Attention}, AttentionNewContributionSinceCandidate) {
		t.Fatalf("candidate-v1 became v2 baseline: %#v, err=%v", projection, err)
	}
}

func TestProjectEnvironmentOrderingAndTruncation(t *testing.T) {
	a := subject(history.ScopeContainer, "")
	b := a
	b.Target = "Deployment/other"
	c := a
	c.Container = "sidecar"
	inputs := EnvironmentInputs{Populations: []history.Population{environmentPopulation(c, history.ScopeContainer, ""), environmentPopulation(b, history.ScopeContainer, ""), environmentPopulation(a, history.ScopeContainer, "")}, Limit: 2}
	first, err := ProjectEnvironment(inputs)
	if err != nil || first.TotalCount != 3 || !first.Truncated || len(first.Entries) != 2 {
		t.Fatalf("truncation = %#v, err=%v", first, err)
	}
	permuted := inputs
	permuted.Populations = []history.Population{inputs.Populations[2], inputs.Populations[0], inputs.Populations[1]}
	second, err := ProjectEnvironment(permuted)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("input ordering changed projection: %#v vs %#v", first, second)
	}
}
