package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	obsdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
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
	for _, required := range []string{"/api/v08/environment", "/api/v08/history", "Environment", "Attention", "Behavioral verification", "No accumulated population record", "Evidence qualification inconclusive", "APPROVED_NOT_APPLIED", "NEW_CONTRIBUTION_SINCE_CANDIDATE", "Projection DEGRADED", "malformed Observations remain visible", "/api/health"} {
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

func TestCollectionSelectorPreservesOpaqueContinuation(t *testing.T) {
	selector, continuation, reason := parseCollectionSelector(map[string][]string{
		"group": {"apps"}, "kind": {"Deployment"}, "name": {"api"}, "container": {"app"}, "workloadUID": {"uid-1"}, "continue": {"opaque-token"},
	})
	if reason != "" || selector.workloadUID != "uid-1" || continuation != "opaque-token" {
		t.Fatalf("selector=%+v continuation=%q reason=%q", selector, continuation, reason)
	}
}

func TestCollectionSelectorRejectsDuplicateContinuation(t *testing.T) {
	if _, _, reason := parseCollectionSelector(map[string][]string{"kind": {"Deployment"}, "name": {"api"}, "container": {"app"}, "workloadUID": {"uid-1"}, "continue": {"a", "b"}}); reason == "" {
		t.Fatal("duplicate continuation was accepted")
	}
}

func TestCollectionContinuationIsBoundToRequestScope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/observations?kind=Deployment&name=api&container=app&workloadUID=uid-1", nil)
	req.Header.Set("X-Environment-Session", "session-a")
	req.Header.Set("X-Environment-Context-Version", "7")
	req.Header.Set("X-Environment-Namespace", "payments")
	selector, _, reason := parseCollectionSelector(req.URL.Query())
	if reason != "" {
		t.Fatal(reason)
	}
	scope := collectionContinuationScope(req, "cluster-a", selector)
	token := sealCollectionContinuation("kube-continue", scope)
	if got, ok := openCollectionContinuation(token, scope); !ok || got != "kube-continue" {
		t.Fatalf("continuation did not open in its original scope: %q %v", got, ok)
	}
	req.Header.Set("X-Environment-Session", "session-b")
	otherScope := collectionContinuationScope(req, "cluster-a", selector)
	if _, ok := openCollectionContinuation(token, otherScope); ok {
		t.Fatal("continuation crossed an EnvironmentSession boundary")
	}
}

func TestProposalUIDMatchesPreventsSameNameRebind(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "proposal", "uid": "uid-b"}}}
	if proposalUIDMatches(obj, "uid-a") {
		t.Fatal("replacement Proposal UID was accepted as the old object")
	}
	if !proposalUIDMatches(obj, "uid-b") {
		t.Fatal("current Proposal UID was rejected")
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

func TestObservationProjectionUsesStableExecutionJSONContract(t *testing.T) {
	cluster, err := obsdomain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	workload := obsdomain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: obsdomain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "workload-uid"}
	slot := obsdomain.ContainerSlot{Workload: workload, Container: "app"}
	spec, err := obsdomain.NewObservationSpec(obsdomain.RequestedTarget{Slot: slot}, []string{"capabilities"}, time.Minute, "test")
	if err != nil {
		t.Fatal(err)
	}
	o, err := obsdomain.NewObservation(obsdomain.ObservationID("execution-contract"), spec)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := obskube.ToUnstructured(o, "default")
	if err != nil {
		t.Fatal(err)
	}
	got, err := observationProjection(obj)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	execution, ok := decoded["execution"].(map[string]interface{})
	if !ok {
		t.Fatalf("execution projection = %#v", decoded["execution"])
	}
	if execution["state"] != string(obsdomain.ExecutionRequested) {
		t.Fatalf("execution state = %#v", execution["state"])
	}
	if _, legacy := execution["State"]; legacy {
		t.Fatal("execution leaked Go field names")
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
	if got.Subject == nil || got.Subject.Target != "Deployment/api" || got.Artifact == nil || got.Artifact.Type != proposal.CandidateV2ArtifactContainerCaps || len(got.Artifact.ContainerCapabilities.Add) != 1 || got.Artifact.ContainerCapabilities.Add[0] != "CAP_CHOWN" {
		t.Fatalf("candidate decision object not projected: subject=%#v artifact=%#v", got.Subject, got.Artifact)
	}
	if got.Provenance == nil || len(got.Provenance.ObservationIDs) != 1 {
		t.Fatalf("provenance not projected: %+v", got.Provenance)
	}
	if got.CandidateYAML == "" {
		t.Fatal("candidate-v2 projection omitted derived YAML")
	}
	canonical, err := json.Marshal(mustCandidate(spec))
	if err != nil {
		t.Fatal(err)
	}
	var canonicalMap, yamlMap map[string]interface{}
	if err := json.Unmarshal(canonical, &canonicalMap); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(got.CandidateYAML), &yamlMap); err != nil {
		t.Fatalf("derived candidate YAML is invalid: %v", err)
	}
	if !reflect.DeepEqual(canonicalMap, yamlMap) {
		t.Fatalf("derived YAML changed candidate data: canonical=%#v yaml=%#v", canonicalMap, yamlMap)
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
