package proposal

import "testing"

func TestProposalSpecEquivalenceUsesCompleteContent(t *testing.T) {
	legacy := Spec{Container: "app", Binary: "/bin/app", GeneratedAt: "2026-09-07T10:00:00Z", HistoryUsed: true}
	legacyV1 := legacy
	legacyV1.CandidateVersion = CandidateVersionV1
	if equivalent, err := proposalSpecsEquivalent(legacy, legacyV1); err != nil || !equivalent {
		t.Fatalf("legacy version normalization equivalent=%v err=%v", equivalent, err)
	}
	legacyChanged := legacy
	legacyChanged.Binary = "/bin/other"
	assertNotEquivalent(t, legacy, legacyChanged)

	base := v2SpecFixture()
	base.GeneratedAt = "2026-09-07T10:00:00Z"
	base.HistoryUsed = true
	identical := base
	if equivalent, err := proposalSpecsEquivalent(base, identical); err != nil || !equivalent {
		t.Fatalf("identical v2 equivalent=%v err=%v", equivalent, err)
	}
	generatedLater := base
	generatedLater.GeneratedAt = "2026-09-07T10:00:01Z"
	if equivalent, err := proposalSpecsEquivalent(base, generatedLater); err != nil || !equivalent {
		t.Fatalf("generated-at-only difference equivalent=%v err=%v", equivalent, err)
	}

	cases := []struct {
		name   string
		mutate func(*Spec)
	}{
		{"provenance", func(s *Spec) {
			s.Provenance = &ProposalProvenance{PopulationScope: CandidateV2ScopeContainer, ObservationIDs: []string{"different"}}
		}},
		{"qualification", func(s *Spec) { s.Qualification.Capabilities = "UNKNOWN" }},
		{"derivation status", func(s *Spec) { s.DerivationStatus.PodLock = "SUPPORTED" }},
		{"subject", func(s *Spec) { s.Subject.Target = "StatefulSet/api" }},
		{"artifact", func(s *Spec) { s.CapabilityArtifact.ContainerCapabilities.Add = []string{"CAP_SYS_ADMIN"} }},
		{"legacy field", func(s *Spec) { s.HistoryUsed = false }},
		{"candidate version", func(s *Spec) { s.CandidateVersion = CandidateVersionV1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := v2SpecFixture()
			changed.GeneratedAt = base.GeneratedAt
			tc.mutate(&changed)
			assertNotEquivalent(t, base, changed)
		})
	}
}

func assertNotEquivalent(t *testing.T, left, right Spec) {
	t.Helper()
	equivalent, err := proposalSpecsEquivalent(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if equivalent {
		t.Fatal("different Proposal specs were considered equivalent")
	}
}
