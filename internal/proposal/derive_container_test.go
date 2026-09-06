package proposal

import (
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
)

func containerHistoryPopulation(state string, capabilities ...string) history.Population {
	accesses := make([]history.CapabilityAccessRecord, 0, len(capabilities))
	for _, capability := range capabilities {
		accesses = append(accesses, history.CapabilityAccessRecord{Name: capability})
	}
	return history.Population{
		Scope:              history.ScopeContainer,
		Target:             "Deployment/api",
		Container:          "app",
		ImageIdentity:      "sha256:" + repeated("a", 64),
		CapabilityAccesses: accesses,
		ObservationContributions: []history.ObservationContribution{
			{ObservationID: "obs-a", Sources: []history.ObservationSourceContribution{{Source: "capabilities", EvidenceState: state, AttributionState: "COMPLETED", AttributedCount: int64(len(capabilities)), NormalizedFactCount: int64(len(capabilities))}}},
			{ObservationID: "obs-b", Sources: []history.ObservationSourceContribution{{Source: "capabilities", EvidenceState: state, AttributionState: "COMPLETED", AttributedCount: int64(len(capabilities)), NormalizedFactCount: int64(len(capabilities))}}},
		},
	}
}

func repeated(value string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += value
	}
	return result
}

func TestDeriveContainerCapabilityProposal(t *testing.T) {
	spec, err := DeriveContainerCapabilityProposal(containerHistoryPopulation("UNKNOWN", "CAP_NET_ADMIN", "CAP_CHOWN", "CAP_NET_ADMIN"))
	if err != nil {
		t.Fatal(err)
	}
	if spec.CandidateVersion != CandidateVersionV2 || spec.Subject.Scope != CandidateV2ScopeContainer || spec.Subject.Target != "Deployment/api" || spec.Subject.Container != "app" || spec.Subject.ImageIdentity == "" {
		t.Fatalf("subject = %#v", spec.Subject)
	}
	if len(spec.CapabilityArtifact.ContainerCapabilities.Drop) != 1 || spec.CapabilityArtifact.ContainerCapabilities.Drop[0] != "ALL" {
		t.Fatalf("drop = %#v", spec.CapabilityArtifact.ContainerCapabilities.Drop)
	}
	got := spec.CapabilityArtifact.ContainerCapabilities.Add
	want := []string{"CAP_CHOWN", "CAP_NET_ADMIN"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("add = %#v, want %#v", got, want)
	}
	if spec.Qualification.Capabilities != "UNKNOWN" || len(spec.Provenance.ObservationIDs) != 2 || spec.Provenance.ObservationIDs[0] != "obs-a" {
		t.Fatalf("review context = %#v %#v", spec.Qualification, spec.Provenance)
	}
	if _, err := spec.CandidateV2(); err != nil {
		t.Fatalf("CandidateV2: %v", err)
	}
	if _, err := spec.ReviewContextV2(); err != nil {
		t.Fatalf("ReviewContextV2: %v", err)
	}
}

func TestDeriveContainerCapabilityProposalRejectsInvalidOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		pop  history.Population
	}{
		{name: "zero", pop: containerHistoryPopulation("EMPTY")},
		{name: "invalid capability", pop: containerHistoryPopulation("AVAILABLE", "CAP_NOT_A_REAL_CAPABILITY")},
		{name: "inconsistent empty", pop: containerHistoryPopulation("EMPTY", "CAP_CHOWN")},
		{name: "binary", pop: func() history.Population {
			p := containerHistoryPopulation("AVAILABLE", "CAP_CHOWN")
			p.Scope = history.ScopeBinary
			p.BinaryPath = "/usr/bin/app"
			return p
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DeriveContainerCapabilityProposal(test.pop); err == nil {
				t.Fatal("expected derivation rejection")
			}
		})
	}
}

func TestDeriveContainerCapabilityProposalPreservesQualification(t *testing.T) {
	for _, state := range []string{"AVAILABLE", "UNKNOWN"} {
		spec, err := DeriveContainerCapabilityProposal(containerHistoryPopulation(state, "CAP_CHOWN"))
		if err != nil {
			t.Fatal(err)
		}
		if spec.Qualification.Capabilities != state {
			t.Fatalf("state = %q, want %q", spec.Qualification.Capabilities, state)
		}
	}
}
