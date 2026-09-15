package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	obsdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestWorkbenchUIUsesNamedGovernanceRoutes(t *testing.T) {
	w := httptest.NewRecorder()
	handleWorkbenchScript(w, httptest.NewRequest(http.MethodGet, "/workbench.js", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "/api/observations") || !strings.Contains(w.Body.String(), "/api/proposals") {
		t.Fatalf("Workbench script does not use durable read routes: status=%d body=%s", w.Code, w.Body.String())
	}
	for _, required := range []string{"/api/governance/proposals/", "proposal.review", "proposal.approve", "proposal.apply"} {
		if !strings.Contains(w.Body.String(), required) {
			t.Errorf("Workbench script missing named governance boundary %q", required)
		}
	}
	for _, required := range []string{"capabilitiesLoaded", "reviewEligible", "approveEligible", "rejectEligible", "applyEligible", "add(\"Review\",\"proposal.review\"", "add(\"Approve\",\"proposal.approve\"", "add(\"Apply\",\"proposal.apply\""} {
		if !strings.Contains(w.Body.String(), required) {
			t.Errorf("Workbench script missing capability/semantic gating expression %q", required)
		}
	}
	for _, forbidden := range []string{"/revoke", "PATCH", "/status", "LastApprovalSnapshot"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Errorf("script contains forbidden authority/action %q", forbidden)
		}
	}
}

func TestWorkbenchV08NavigationAndSemanticBoundaries(t *testing.T) {
	w := httptest.NewRecorder()
	handleWorkbenchScript(w, httptest.NewRequest(http.MethodGet, "/workbench.js", nil))
	script := w.Body.String()
	for _, required := range []string{"/api/v08/environment", "/api/v08/history", "Environment", "Attention", "Behavioral verification", "No accumulated population record", "Evidence state unknown", "APPROVED_NOT_APPLIED", "NEW_CONTRIBUTION_SINCE_CANDIDATE", "Projection DEGRADED", "malformed Observations remain visible"} {
		if !strings.Contains(script, required) {
			t.Errorf("G8 script missing %q", required)
		}
	}
	for _, forbidden := range []string{"innerHTML", "Secure workloads", "Protected workloads", "Risk score", "SOC", "Acknowledge", "Dismiss"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("G8 script contains forbidden UI construct/claim %q", forbidden)
		}
	}
}

func TestReadModelSelectorRequiresImmutableWorkloadUID(t *testing.T) {
	if _, reason := parseReadModelSelector(map[string][]string{"kind": {"Deployment"}, "name": {"api"}, "container": {"app"}}); reason == "" {
		t.Fatal("selector without workload UID was accepted")
	}
	s, reason := parseReadModelSelector(map[string][]string{"group": {"apps"}, "kind": {"Deployment"}, "name": {"api"}, "container": {"app"}, "workloadUID": {"uid-1"}})
	if reason != "" || s.workloadUID != "uid-1" {
		t.Fatalf("selector parse = %+v, %q", s, reason)
	}
}

func TestObservationIdentityUsesResolvedTargetImageRevision(t *testing.T) {
	cluster, err := obsdomain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	workload := obsdomain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: obsdomain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "workload-uid"}
	slot := obsdomain.ContainerSlot{Workload: workload, Container: "app"}
	spec, err := obsdomain.NewObservationSpec(obsdomain.RequestedTarget{Slot: slot}, []string{"filesystem"}, time.Minute, "test")
	if err != nil {
		t.Fatal(err)
	}
	o, err := obsdomain.NewObservation(obsdomain.ObservationID("identity-test"), spec)
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	revision, err := obsdomain.NewContainerImageRevision(slot, digest)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := obsdomain.NewResolvedTargetSet([]obsdomain.RuntimeContainerInstance{{Slot: slot, PodUID: "pod-uid", ImageRevision: &revision}})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Bind(resolved, obsdomain.BackendIdentity{Kind: "trace_open", Version: "v0.55.1"}, nil); err != nil {
		t.Fatal(err)
	}
	identity := observationIdentityOf(o)
	if identity.ImageIdentity != digest {
		t.Fatalf("image identity = %q, want %q", identity.ImageIdentity, digest)
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
