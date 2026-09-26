package proposal

import (
	"sort"
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
			{ObservationID: "obs-a", Sources: []history.ObservationSourceContribution{{Source: "capabilities", EvidenceState: state, AttributionState: "COMPLETED", BackendHealthy: true, AttachedForWindow: true, FlushConfirmed: true, AttributedCount: int64(len(capabilities)), NormalizedFactCount: int64(len(capabilities))}}, CapabilityFacts: sortedCapabilityNames(capabilities), CapabilityFactsComplete: state != "UNKNOWN"},
			{ObservationID: "obs-b", Sources: []history.ObservationSourceContribution{{Source: "capabilities", EvidenceState: state, AttributionState: "COMPLETED", BackendHealthy: true, AttachedForWindow: true, FlushConfirmed: true, AttributedCount: int64(len(capabilities)), NormalizedFactCount: int64(len(capabilities))}}, CapabilityFacts: sortedCapabilityNames(capabilities), CapabilityFactsComplete: state != "UNKNOWN"},
		},
	}
}

func sortedCapabilityNames(capabilities []string) []string {
	set := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		set[capability] = true
	}
	result := make([]string, 0, len(set))
	for capability := range set {
		result = append(result, capability)
	}
	sort.Strings(result)
	return result
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

func TestDeriveCapabilityAttributionIsPerObservationAndDeterministic(t *testing.T) {
	population := containerHistoryPopulation("AVAILABLE", "CAP_NET_ADMIN", "CAP_CHOWN", "CAP_NET_ADMIN")
	population.ObservationContributions[0].CapabilityFacts = []string{"CAP_CHOWN"}
	population.ObservationContributions[1].CapabilityFacts = []string{"CAP_NET_ADMIN"}
	spec, err := DeriveContainerCapabilityProposal(population)
	if err != nil {
		t.Fatal(err)
	}
	got := spec.Provenance.CapabilityAttribution
	if len(got) != 2 || got[0].Capability != "CAP_CHOWN" || got[1].Capability != "CAP_NET_ADMIN" {
		t.Fatalf("capability attribution order = %#v", got)
	}
	if got[0].State != CapabilityAttributionKnown || len(got[0].ObservationIDs) != 1 || got[0].ObservationIDs[0] != "obs-a" {
		t.Fatalf("CAP_CHOWN attribution = %#v", got[0])
	}
	if got[1].State != CapabilityAttributionKnown || len(got[1].ObservationIDs) != 1 || got[1].ObservationIDs[0] != "obs-b" {
		t.Fatalf("CAP_NET_ADMIN attribution = %#v", got[1])
	}
}

func TestDeriveCapabilityAttributionSupportsMultipleObservationsAndUnknownHistory(t *testing.T) {
	population := containerHistoryPopulation("AVAILABLE", "CAP_CHOWN")
	spec, err := DeriveContainerCapabilityProposal(population)
	if err != nil {
		t.Fatal(err)
	}
	evidence := spec.Provenance.CapabilityAttribution[0]
	if evidence.State != CapabilityAttributionKnown || len(evidence.ObservationIDs) != 2 || evidence.ObservationIDs[0] != "obs-a" || evidence.ObservationIDs[1] != "obs-b" {
		t.Fatalf("multi-observation attribution = %#v", evidence)
	}

	population.ObservationContributions[1].CapabilityFactsComplete = false
	population.ObservationContributions[1].CapabilityFacts = nil
	spec, err = DeriveContainerCapabilityProposal(population)
	if err != nil {
		t.Fatal(err)
	}
	evidence = spec.Provenance.CapabilityAttribution[0]
	if evidence.State != CapabilityAttributionUnknown || len(evidence.ObservationIDs) != 1 || evidence.ObservationIDs[0] != "obs-a" {
		t.Fatalf("incomplete attribution must retain only known matches and be UNKNOWN: %#v", evidence)
	}
}
