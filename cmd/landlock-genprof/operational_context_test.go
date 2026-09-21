package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func operationalContextRequest(t *testing.T, server *workbenchServer) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18080"+operationalContextPath, nil)
	request.Host = "127.0.0.1:18080"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func TestOperationalContextRequiresAuthentication(t *testing.T) {
	_, reads := workbenchReadFixture(t, "team-a")
	server, err := newWorkbenchServer(reads, 18080)
	if err != nil {
		t.Fatal(err)
	}
	response := operationalContextRequest(t, server)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s, want 401", response.Code, response.Body.String())
	}
}

func TestOperationalContextSeparatesAuthorityPlatformAndProjection(t *testing.T) {
	_, reads := workbenchReadFixture(t, "team-a")
	server, err := newWorkbenchServer(reads, 18080)
	if err != nil {
		t.Fatal(err)
	}
	server.authenticated = true
	server.requestIdentity = authn.Identity{Username: "alice@company"}
	server.clusterIdentity = "kube-system-uid-a"
	server.observations = &observationAPI{}
	server.discoverCaps = func(context.Context, string) (map[authz.Capability]bool, error) {
		return map[authz.Capability]bool{authz.ProposalReview: true, authz.ProposalApprove: false}, nil
	}
	response := operationalContextRequest(t, server)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body operationalContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Context.Cluster.Identity != "kube-system-uid-a" || body.Context.Namespace != "team-a" || body.Context.Actor.Username != "alice@company" {
		t.Fatalf("context=%+v", body.Context)
	}
	if body.Authority.Status != operationalHealthy || !body.Authority.Capabilities[authz.ProposalReview] || body.Authority.Capabilities[authz.ProposalApprove] {
		t.Fatalf("authority=%+v", body.Authority)
	}
	if body.Platform.Status != operationalHealthy || body.Platform.KubernetesAPI != operationalHealthy {
		t.Fatalf("platform=%+v", body.Platform)
	}
	if body.Projection.Status != projectionHealthy || body.Projection.ValidObjectCount != 0 || body.ReadTime == "" {
		t.Fatalf("projection=%+v readTime=%q", body.Projection, body.ReadTime)
	}
	if len(body.Platform.ProductResources) != 6 {
		t.Fatalf("product resources=%d, want 6", len(body.Platform.ProductResources))
	}
}

type operationalContextCapabilityFailureReads struct {
	k8s.WorkbenchReadCapability
}

func TestOperationalContextCapabilityFailureDoesNotFabricateAuthority(t *testing.T) {
	_, reads := workbenchReadFixture(t, "team-a")
	server, err := newWorkbenchServer(reads, 18080)
	if err != nil {
		t.Fatal(err)
	}
	server.authenticated = true
	server.requestIdentity = authn.Identity{Username: "alice@company"}
	server.clusterIdentity = "cluster-a"
	server.discoverCaps = func(context.Context, string) (map[authz.Capability]bool, error) {
		return nil, errors.New("ssar unavailable")
	}
	response := operationalContextRequest(t, server)
	var body operationalContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Authority.Status != operationalUnknown || len(body.Authority.Capabilities) != 0 {
		t.Fatalf("authority=%+v, want UNKNOWN with no fabricated capabilities", body.Authority)
	}
}

type operationalContextMissingResourceReads struct {
	k8s.WorkbenchReadCapability
}

func (v operationalContextMissingResourceReads) ListContributionReceipts(context.Context) (*unstructured.UnstructuredList, error) {
	return nil, &k8s.ReadError{State: k8s.ReadBackendNotInstalled, Resource: "observationcontributionreceipts", Err: errors.New("not served")}
}

func TestOperationalContextMissingResourceIsPlatformDegraded(t *testing.T) {
	_, reads := workbenchReadFixture(t, "team-a")
	server, err := newWorkbenchServer(operationalContextMissingResourceReads{WorkbenchReadCapability: reads}, 18080)
	if err != nil {
		t.Fatal(err)
	}
	server.authenticated = true
	server.requestIdentity = authn.Identity{Username: "alice@company"}
	server.clusterIdentity = "cluster-a"
	server.discoverCaps = func(context.Context, string) (map[authz.Capability]bool, error) {
		return map[authz.Capability]bool{}, nil
	}
	response := operationalContextRequest(t, server)
	var body operationalContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Platform.Status != operationalDegraded || body.Platform.ProductResources[5].Reason != "REQUIRED_RESOURCE_MISSING" {
		t.Fatalf("platform=%+v", body.Platform)
	}
	if body.Projection.Status != projectionHealthy {
		t.Fatalf("projection=%q, missing CRD must not fabricate malformed data", body.Projection.Status)
	}
}
