package main

import (
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestReadModelSelectorRequiresImmutableWorkloadUID(t *testing.T) {
	if _, reason := parseReadModelSelector(map[string][]string{"kind": {"Deployment"}, "name": {"api"}, "container": {"app"}}); reason == "" {
		t.Fatal("selector without workload UID was accepted")
	}
	s, reason := parseReadModelSelector(map[string][]string{"group": {"apps"}, "kind": {"Deployment"}, "name": {"api"}, "container": {"app"}, "workloadUID": {"uid-1"}})
	if reason != "" || s.workloadUID != "uid-1" {
		t.Fatalf("selector parse = %+v, %q", s, reason)
	}
}

func TestProposalReadModelUsesCertifiedDigestsAndAuthority(t *testing.T) {
	spec := proposal.Spec{
		CandidateVersion:   proposal.CandidateVersionV2,
		GeneratedAt:        "2026-09-07T00:00:00Z",
		Subject:            &proposal.SubjectV2{Scope: proposal.CandidateV2ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		CapabilityArtifact: &proposal.ArtifactV2{Type: proposal.CandidateV2ArtifactContainerCaps, ContainerCapabilities: proposal.ContainerCapabilitiesV2{Drop: []string{"ALL"}, Add: []string{"CAP_CHOWN"}}},
		Provenance:         &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: []string{"observation-1"}},
		Qualification:      &proposal.ProposalQualification{Filesystem: "EMPTY", Exec: "UNKNOWN", NetworkConnect: "EMPTY", NetworkBind: "EMPTY", Capabilities: "AVAILABLE"},
		DerivationStatus:   &proposal.ProposalDerivationStatus{Capabilities: "SUPPORTED", PodLock: "UNSUPPORTED", NetworkPolicy: "NOT_AVAILABLE", Seccomp: "UNSUPPORTED"},
	}
	if err := proposal.ValidateProposalSpec(spec); err != nil {
		t.Fatal(err)
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		t.Fatal(err)
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "SecurityProfileProposal", "metadata": map[string]interface{}{"name": "proposal-1", "uid": "uid-1"}, "spec": m}}
	got, err := proposalProjection(obj)
	if err != nil {
		t.Fatal(err)
	}
	wantCandidate, _ := proposal.CandidateDigestV2(mustCandidate(spec))
	wantReview, _ := proposal.ReviewContextDigestV2(mustReviewContext(spec))
	if got.CandidateDigest != wantCandidate || got.ReviewContextDigest != wantReview {
		t.Fatalf("digests = %q/%q, want %q/%q", got.CandidateDigest, got.ReviewContextDigest, wantCandidate, wantReview)
	}
	if got.Status.ApprovalState != proposal.ApprovalDraft || got.CurrentAuthority != "NOT_APPROVED" {
		t.Fatalf("governance projection = %+v authority=%q", got.Status, got.CurrentAuthority)
	}
	if got.Provenance == nil || len(got.Provenance.ObservationIDs) != 1 {
		t.Fatalf("provenance not projected: %+v", got.Provenance)
	}
}

func mustCandidate(s proposal.Spec) proposal.CandidateV2 {
	c, err := s.CandidateV2()
	if err != nil {
		panic(err)
	}
	return c
}
func mustReviewContext(s proposal.Spec) proposal.ProposalReviewContextV2 {
	c, err := s.ReviewContextV2()
	if err != nil {
		panic(err)
	}
	return c
}
