package reconciliation

import (
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

func TestProposalSubjectMatchedMultipleWorkloadUIDs(t *testing.T) {
	subject := subject(history.ScopeContainer, "")
	proposal := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"})
	uidA := uidObservation(t, "observation-a", "workload-a", "cluster-uid", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	uidB := uidObservation(t, "observation-b", "workload-b", "cluster-uid", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	tests := []struct {
		name         string
		proposals    []CandidateProposal
		observations []observationdomain.Observation
		want         bool
	}{
		{"no candidate", nil, []observationdomain.Observation{uidA, uidB}, false},
		{"no observations", []CandidateProposal{proposal}, nil, false},
		{"one uid", []CandidateProposal{proposal}, []observationdomain.Observation{uidA}, false},
		{"same uid repeated", []CandidateProposal{proposal}, []observationdomain.Observation{uidA, uidA}, false},
		{"two uids", []CandidateProposal{proposal}, []observationdomain.Observation{uidA, uidB}, true},
		{"order independent", []CandidateProposal{proposal}, []observationdomain.Observation{uidB, uidA}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ProposalSubjectMatchedMultipleWorkloadUIDs(subject, test.proposals, test.observations); got != test.want {
				t.Fatalf("ambiguity = %v, want %v", got, test.want)
			}
		})
	}
}

func TestUIDAmbiguityRequiresCandidateV2AndExactSubject(t *testing.T) {
	subject := subject(history.ScopeContainer, "")
	uidA := uidObservation(t, "observation-a", "workload-a", "cluster-uid", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	uidB := uidObservation(t, "observation-b", "workload-b", "cluster-uid", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	v1 := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal-v1", UID: "proposal-v1"})
	v1.Spec.CandidateVersion = "candidate-v1"
	if ProposalSubjectMatchedMultipleWorkloadUIDs(subject, []CandidateProposal{v1}, []observationdomain.Observation{uidA, uidB}) {
		t.Fatal("candidate-v1 produced candidate-v2 ambiguity")
	}
	otherImage := uidObservation(t, "observation-c", "workload-c", "cluster-uid", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	proposal := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal-v2", UID: "proposal-v2"})
	if ProposalSubjectMatchedMultipleWorkloadUIDs(subject, []CandidateProposal{proposal}, []observationdomain.Observation{uidA, otherImage}) {
		t.Fatal("different image identities were combined")
	}
}

func TestUIDAmbiguityDoesNotCombineClusters(t *testing.T) {
	subject := subject(history.ScopeContainer, "")
	proposal := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"})
	a := uidObservation(t, "observation-a", "workload-a", "cluster-a", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	b := uidObservation(t, "observation-b", "workload-b", "cluster-b", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if ProposalSubjectMatchedMultipleWorkloadUIDs(subject, []CandidateProposal{proposal}, []observationdomain.Observation{a, b}) {
		t.Fatal("UIDs from different clusters were combined")
	}
}

func TestEnvironmentEntryCarriesUIDAmbiguityBoolean(t *testing.T) {
	subject := subject(history.ScopeContainer, "")
	population := environmentPopulation(subject, history.ScopeContainer, "")
	proposal := approvedCandidate(t, ProposalRef{Namespace: "default", Name: "proposal", UID: "proposal-uid"})
	a := uidObservation(t, "observation-a", "workload-a", "cluster-uid", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	b := uidObservation(t, "observation-b", "workload-b", "cluster-uid", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	projection, err := ProjectEnvironment(EnvironmentInputs{Populations: []history.Population{population}, Observations: []observationdomain.Observation{a, b}, Proposals: []ProposalInput{{CandidateProposal: proposal}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Entries) != 1 || !projection.Entries[0].ProposalSubjectMatchedMultipleWorkloadUIDs {
		t.Fatalf("environment ambiguity = %#v", projection)
	}
}

func uidObservation(t *testing.T, id, workloadUID, clusterUID, image string) observationdomain.Observation {
	t.Helper()
	cluster, err := observationdomain.NewClusterIdentity(clusterUID)
	if err != nil {
		t.Fatal(err)
	}
	slot := observationdomain.ContainerSlot{Workload: observationdomain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: observationdomain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: workloadUID}, Container: "app"}
	spec, err := observationdomain.NewObservationSpec(observationdomain.RequestedTarget{Slot: slot}, []string{"capabilities"}, time.Minute, "session")
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observationdomain.NewObservation(observationdomain.ObservationID(id), spec)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := observationdomain.NewContainerImageRevision(slot, image)
	if err != nil {
		t.Fatal(err)
	}
	runtime := observationdomain.RuntimeContainerInstance{Slot: slot, PodUID: "pod-" + workloadUID, ImageRevision: &revision}
	resolved, err := observationdomain.NewResolvedTargetSet([]observationdomain.RuntimeContainerInstance{runtime})
	if err != nil {
		t.Fatal(err)
	}
	if err := obs.Bind(resolved, observationdomain.BackendIdentity{Kind: "core", Version: "v1"}, []observationdomain.ContainerImageRevision{revision}); err != nil {
		t.Fatal(err)
	}
	if err := obs.Transition(observationdomain.ExecutionStarting, observationdomain.CompletionReason("")); err != nil {
		t.Fatal(err)
	}
	if err := obs.Transition(observationdomain.ExecutionFailed, observationdomain.BackendFailure); err != nil {
		t.Fatal(err)
	}
	return obs
}
