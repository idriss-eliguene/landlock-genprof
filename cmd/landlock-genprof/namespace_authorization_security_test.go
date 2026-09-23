package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestApplicationApprovalRolesAreIndependent(t *testing.T) {
	server := &workbenchServer{
		requestIdentity: authn.Identity{Username: "reviewer", Groups: []string{"security-reviewers"}},
		reviewGroups:    []string{"security-reviewers"},
		approverGroups:  []string{"security-approvers"},
	}
	if !server.applicationCapabilityAllowed(authz.ProposalReview) {
		t.Fatal("reviewer should be allowed to review")
	}
	if server.applicationCapabilityAllowed(authz.ProposalApprove) {
		t.Fatal("reviewer must not be allowed to approve")
	}
	server.requestIdentity.Groups = []string{"security-approvers"}
	if server.applicationCapabilityAllowed(authz.ProposalReview) {
		t.Fatal("approver-only identity must not be granted review by the review role")
	}
	if !server.applicationCapabilityAllowed(authz.ProposalApprove) {
		t.Fatal("approver should be allowed to approve")
	}
}

func TestClusterScopedProfileAuthorizationBindsApprovedTarget(t *testing.T) {
	target := k8s.GovernedTarget{Namespace: "team-a", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "api"}, Container: "server"}
	name := spobackend.GovernedProfileName(target.Namespace, target.Workload.Name, target.Container)
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": spobackend.APIVersion, "kind": spobackend.SeccompProfileKind,
		"metadata": map[string]interface{}{"name": name},
	}}
	obj.SetAnnotations(spobackend.OwnershipAnnotations(target.Namespace, target.Workload.Name, target.Container))
	if err := authorizeClusterScopedProfile(context.Background(), "team-a", "proposal", "sha256:x", target, obj); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	obj.SetName(spobackend.GovernedProfileName("team-b", target.Workload.Name, target.Container))
	if err := authorizeClusterScopedProfile(context.Background(), "team-a", "proposal", "sha256:x", target, obj); err == nil {
		t.Fatal("forged cross-namespace profile name was accepted")
	}
}

func TestObservationSensitiveEndpointsRequireCapabilities(t *testing.T) {
	core := k8sfake.NewSimpleClientset()
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	observations, err := newObservationAPI(core, dyn, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	server := &workbenchServer{
		reads: reads, dynamic: dyn, observations: observations, authenticated: true,
		requestIdentity: authn.Identity{Username: "unauthorized"},
		discoverCaps: func(context.Context, string) (map[authz.Capability]bool, error) {
			return map[authz.Capability]bool{}, nil
		},
	}
	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"start", http.MethodPost, "/api/observations/start", `{"namespace":"team-a","pod":"api","container":"server","duration":1}`},
		{"status", http.MethodGet, "/api/observations/status?namespace=team-a&observationID=x", ""},
		{"generate", http.MethodPost, "/api/observations/generate-proposal", `{"namespace":"team-a","observationID":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			response := httptest.NewRecorder()
			switch tc.name {
			case "start":
				server.handleObservationStart(response, request)
			case "status":
				server.handleObservationStatus(response, request)
			default:
				server.handleObservationGenerateProposal(response, request)
			}
			if response.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s, want 403", response.Code, response.Body.String())
			}
		})
	}
}
