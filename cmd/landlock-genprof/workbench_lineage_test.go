package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func testLineageIdentity(uid, cluster, container string) observationIdentity {
	return observationIdentity{ClusterIdentity: cluster, Namespace: "payments", Group: "apps", Kind: "Deployment", WorkloadName: "api", WorkloadUID: uid, Container: container}
}

func TestClassifyProposalLineageRequiresAllObservationIDs(t *testing.T) {
	selected := testLineageIdentity("uid-a", "cluster-a", "app")
	observations := map[string]observationIdentity{
		"oa": selected,
		"ob": testLineageIdentity("uid-b", "cluster-a", "app"),
	}
	base := func(ids ...string) proposalRead {
		return proposalRead{Provenance: &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: ids}}
	}
	for _, tc := range []struct {
		name     string
		proposal proposalRead
		want     lineageAssociation
	}{
		{"exact", base("oa"), lineageExact},
		{"mixed", base("oa", "ob"), lineageMixed},
		{"unknown observation", base("missing"), lineageInsufficient},
		{"empty", base(), lineageInsufficient},
		{"missing provenance", proposalRead{}, lineageInsufficient},
		{"other workload", base("ob"), lineageNotAssociated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyProposalLineage(tc.proposal, observations, selected); got != tc.want {
				t.Fatalf("classification = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClassifyProposalLineageDeleteRecreateDoesNotAttach(t *testing.T) {
	old := testLineageIdentity("uid-a", "cluster-a", "app")
	current := testLineageIdentity("uid-b", "cluster-a", "app")
	proposal := proposalRead{Provenance: &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: []string{"old-observation"}}}
	observations := map[string]observationIdentity{"old-observation": old}
	if got := classifyProposalLineage(proposal, observations, current); got != lineageNotAssociated {
		t.Fatalf("recreated workload classification = %s, want %s", got, lineageNotAssociated)
	}
}

func TestLineageSelectorRequiresNamespaceAndImmutableUID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/workloads/policy?group=apps&kind=Deployment&name=api&container=app&workloadUID=uid-a", nil)
	if _, reason := parseLineageSelector(request); reason == "" {
		t.Fatal("lineage selector without namespace was accepted")
	}
	request = httptest.NewRequest(http.MethodGet, "/api/workloads/policy?namespace=payments&group=apps&kind=Deployment&name=api&container=app&workloadUID=uid-a", nil)
	sel, reason := parseLineageSelector(request)
	if reason != "" || sel.workloadUID != "uid-a" {
		t.Fatalf("lineage selector = %+v, reason=%q", sel, reason)
	}
}
