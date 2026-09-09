package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestGovernanceRoutesRequireAuthenticatedRequest(t *testing.T) {
	server := &workbenchServer{}
	for _, path := range []string{
		"/api/governance/proposals/example/review",
		"/api/governance/proposals/example/approve",
		"/api/governance/proposals/example/reject",
		"/api/governance/proposals/example/apply",
		"/api/governance/apply-attempts/example/rollback",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		res := httptest.NewRecorder()
		server.mux().ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401", path, res.Code)
		}
	}
}

func governanceTestServer(t *testing.T, object *unstructured.Unstructured) *workbenchServer {
	t.Helper()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)
	return governanceTestServerWithClient(t, dynamicClient)
}

func governanceTestServerWithClient(t *testing.T, dynamicClient dynamic.Interface) *workbenchServer {
	t.Helper()
	coreClient := k8sfake.NewSimpleClientset()
	reads, err := k8s.NewReadSessionForClients(coreClient, dynamicClient, coreClient.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	return &workbenchServer{
		reads: reads, dynamic: dynamicClient, authenticated: true,
		requestIdentity: authn.Identity{Username: "alice@company"},
		discoverCaps: func(context.Context, string) (map[authz.Capability]bool, error) {
			return map[authz.Capability]bool{authz.ProposalReview: true, authz.ProposalApprove: true, authz.ProposalApply: true, authz.RollbackExecute: true}, nil
		},
	}
}

func TestGovernanceHTTPStaleReviewFailsConflict(t *testing.T) {
	object, _ := validGovernanceProposal(t)
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)
	server := governanceTestServerWithClient(t, client)
	request := httptest.NewRequest(http.MethodPost, "/api/governance/proposals/valid/review", strings.NewReader(`{"expectedResourceVersion":"0"}`))
	response := httptest.NewRecorder()
	server.mux().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale review status=%d body=%s, want 409", response.Code, response.Body.String())
	}
}

func malformedGovernanceProposal() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "SecurityProfileProposal",
		"metadata": map[string]interface{}{"name": "malformed", "namespace": "team-a", "uid": "proposal-uid", "resourceVersion": "1"},
		"spec":     map[string]interface{}{"candidateVersion": "v2", "subject": map[string]interface{}{"scope": "CONTAINER", "target": "not-a-valid-subject"}},
		"status":   map[string]interface{}{"approvalState": "Draft"},
	}}
}

func validGovernanceProposal(t *testing.T) (*unstructured.Unstructured, string) {
	t.Helper()
	spec := proposal.Spec{
		CandidateVersion: proposal.CandidateVersionV2,
		GeneratedAt:      "2026-09-08T00:00:00Z",
		Subject:          &proposal.SubjectV2{Scope: proposal.CandidateV2ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:" + strings.Repeat("a", 64)},
		CapabilityArtifact: &proposal.ArtifactV2{Type: proposal.CandidateV2ArtifactContainerCaps, ContainerCapabilities: proposal.ContainerCapabilitiesV2{
			Drop: []string{"ALL"}, Add: []string{"CAP_CHOWN"},
		}},
		Provenance:       &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: []string{"observation-1"}},
		Qualification:    &proposal.ProposalQualification{Filesystem: "EMPTY", Exec: "UNKNOWN", NetworkConnect: "EMPTY", NetworkBind: "EMPTY", Capabilities: "AVAILABLE"},
		DerivationStatus: &proposal.ProposalDerivationStatus{Capabilities: "SUPPORTED", PodLock: "UNSUPPORTED", NetworkPolicy: "NOT_AVAILABLE", Seccomp: "UNSUPPORTED"},
	}
	if err := proposal.ValidateProposalSpec(spec); err != nil {
		t.Fatal(err)
	}
	specMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := proposal.CandidateDigestV2(mustCandidate(spec))
	if err != nil {
		t.Fatal(err)
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "SecurityProfileProposal",
		"metadata": map[string]interface{}{"name": "valid", "namespace": "team-a", "uid": "proposal-uid", "resourceVersion": "1"},
		"spec":     specMap, "status": map[string]interface{}{"approvalState": "Draft"},
	}}, digest
}

func TestGovernanceHTTPNamedTransitionsDelegateAndSucceedForValidProposal(t *testing.T) {
	object, digest := validGovernanceProposal(t)
	server := governanceTestServer(t, object)
	for _, operation := range []struct {
		name      string
		wantField string
	}{
		{name: "review", wantField: "reviewedBy"},
		{name: "approve", wantField: "approvedBy"},
		{name: "reject", wantField: "rejectedBy"},
	} {
		_, resourceVersion, err := proposal.GetStatusWithResourceVersion(context.Background(), server.dynamic, "team-a", "valid")
		if err != nil {
			t.Fatal(err)
		}
		body := `{"expectedResourceVersion":"` + resourceVersion + `"}`
		if operation.name == "approve" {
			body = `{"expectedDigest":"` + digest + `","expectedResourceVersion":"` + resourceVersion + `"}`
		}
		request := httptest.NewRequest(http.MethodPost, "/api/governance/proposals/valid/"+operation.name, strings.NewReader(body))
		response := httptest.NewRecorder()
		server.mux().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", operation.name, response.Code, response.Body.String())
		}
		status, err := proposal.GetStatus(context.Background(), server.dynamic, "team-a", "valid")
		if err != nil {
			t.Fatalf("%s status read: %v", operation.name, err)
		}
		for field, got := range map[string]string{"reviewedBy": status.ReviewedBy, "approvedBy": status.ApprovedBy, "rejectedBy": status.RejectedBy} {
			if field == operation.wantField && got != "alice@company" {
				t.Fatalf("%s %s = %q, want authenticated actor", operation.name, field, got)
			}
			if field != operation.wantField && got == "bob@company" {
				t.Fatalf("%s accepted spoofed actor in %s", operation.name, field)
			}
		}
	}
	status, err := proposal.GetStatus(context.Background(), server.dynamic, "team-a", "valid")
	if err != nil {
		t.Fatal(err)
	}
	if status.ReviewedBy != "alice@company" || status.ApprovedBy != "alice@company" || status.RejectedBy != "alice@company" {
		t.Fatalf("transition attribution = reviewed=%q approved=%q rejected=%q", status.ReviewedBy, status.ApprovedBy, status.RejectedBy)
	}
}

func TestGovernanceHTTPRejectsActorSpoofingFields(t *testing.T) {
	object, _ := validGovernanceProposal(t)
	server := governanceTestServer(t, object)
	for _, operation := range []string{"review", "approve", "reject"} {
		request := httptest.NewRequest(http.MethodPost, "/api/governance/proposals/valid/"+operation, strings.NewReader(`{"actor":"bob@company","reviewedBy":"bob@company","approvedBy":"bob@company","rejectedBy":"bob@company"}`))
		response := httptest.NewRecorder()
		server.mux().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s actor spoof status=%d body=%s, want strict request rejection", operation, response.Code, response.Body.String())
		}
	}
}

func TestGovernanceHTTPRejectsMalformedProposalForEveryNamedProposalOperation(t *testing.T) {
	for _, operation := range []string{"review", "approve", "reject", "apply"} {
		t.Run(operation, func(t *testing.T) {
			server := governanceTestServer(t, malformedGovernanceProposal())
			request := httptest.NewRequest(http.MethodPost, "/api/governance/proposals/malformed/"+operation, strings.NewReader(`{"expectedDigest":"sha256:`+strings.Repeat("a", 64)+`","expectedResourceVersion":"1"}`))
			response := httptest.NewRecorder()
			server.mux().ServeHTTP(response, request)
			if response.Code >= http.StatusOK && response.Code < http.StatusMultipleChoices {
				t.Fatalf("malformed %s returned success: status=%d body=%s", operation, response.Code, response.Body.String())
			}
		})
	}
}

func TestGovernanceHTTPStaleApprovalFailsClosed(t *testing.T) {
	server := governanceTestServer(t, malformedGovernanceProposal())
	request := httptest.NewRequest(http.MethodPost, "/api/governance/proposals/malformed/approve", strings.NewReader(fmt.Sprintf(`{"expectedDigest":"sha256:%s","expectedResourceVersion":"1"}`, strings.Repeat("a", 64))))
	response := httptest.NewRecorder()
	server.mux().ServeHTTP(response, request)
	if response.Code < http.StatusBadRequest {
		t.Fatalf("stale/malformed approval status=%d body=%s", response.Code, response.Body.String())
	}
}
