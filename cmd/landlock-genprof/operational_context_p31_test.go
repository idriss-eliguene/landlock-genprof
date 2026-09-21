package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// This exercises the real Operations Context HTTP handler and the real
// namespace-scoped SSAR projection. The fake Kubernetes client only makes the
// authorization API deterministic; it does not replace the handler path.
func TestOperationalContextUsesOneProjectionAndThirteenSSAR(t *testing.T) {
	_, reads := workbenchReadFixture(t, "team-a")
	server, err := newWorkbenchServer(reads, "", 18080)
	if err != nil {
		t.Fatal(err)
	}
	server.authenticated = true
	server.requestIdentity = authn.Identity{Username: "alice@company"}

	client := fake.NewSimpleClientset()
	var ssarCalls atomic.Int32
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		ssarCalls.Add(1)
		request := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		if request.Spec.ResourceAttributes.Namespace != "team-a" {
			t.Fatalf("SSAR namespace = %q, want team-a", request.Spec.ResourceAttributes.Namespace)
		}
		return true, &authorizationv1.SelfSubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})

	var projections atomic.Int32
	server.discoverCaps = func(ctx context.Context, namespace string) (map[authz.Capability]bool, error) {
		projections.Add(1)
		return authz.DiscoverCapabilities(ctx, client, namespace)
	}

	response := operationalContextRequest(t, server)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body operationalContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Context.Namespace != "team-a" {
		t.Fatalf("context namespace=%q, want team-a", body.Context.Namespace)
	}
	if got := projections.Load(); got != 1 {
		t.Fatalf("Operations Context projections=%d, want 1", got)
	}
	if got := ssarCalls.Load(); got != 13 {
		t.Fatalf("Operations Context SSAR calls=%d, want 13", got)
	}
}
