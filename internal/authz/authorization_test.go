package authz

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

func TestImpersonationIsCopiedAndRequestScoped(t *testing.T) {
	base := &rest.Config{Host: "https://cluster.example"}
	a, err := NewImpersonatedConfig(base, authn.Identity{Username: "alice", Groups: []string{"team-a"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewImpersonatedConfig(base, authn.Identity{Username: "bob", Groups: []string{"team-b"}})
	if err != nil {
		t.Fatal(err)
	}
	if a == b || a.Impersonate.UserName != "alice" || b.Impersonate.UserName != "bob" {
		t.Fatalf("request identities were not isolated: %#v %#v", a.Impersonate, b.Impersonate)
	}
	if base.Impersonate.UserName != "" {
		t.Fatal("base config was mutated")
	}
}

func TestImpersonationRejectsExistingUIDAndExtra(t *testing.T) {
	for name, mutate := range map[string]func(*rest.Config){
		"uid":   func(c *rest.Config) { c.Impersonate.UID = "uid" },
		"extra": func(c *rest.Config) { c.Impersonate.Extra = map[string][]string{"x": {"y"}} },
	} {
		t.Run(name, func(t *testing.T) {
			base := &rest.Config{Host: "https://cluster.example"}
			mutate(base)
			if _, err := NewImpersonatedConfig(base, authn.Identity{Username: "alice"}); err == nil {
				t.Fatal("expected existing impersonation rejection")
			}
		})
	}
}

func TestImpersonationRejectsSystemPrincipals(t *testing.T) {
	for _, identity := range []authn.Identity{
		{Username: "system:masters"},
		{Username: "system:serviceaccount:team-a:runner"},
		{Username: "alice", Groups: []string{"system:masters"}},
	} {
		if _, err := NewImpersonatedConfig(&rest.Config{Host: "https://cluster.example"}, identity); err == nil {
			t.Fatalf("system principal %#v was accepted", identity)
		}
	}
}

func TestDiscoverCapabilitiesUsesNamespaceScopedSSAR(t *testing.T) {
	client := fake.NewSimpleClientset()
	var calls atomic.Int32
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		calls.Add(1)
		create := action.(k8stesting.CreateAction)
		req := create.GetObject().(*authorizationv1.SelfSubjectAccessReview)
		if req.Spec.ResourceAttributes.Namespace != "team-a" {
			t.Fatalf("SSAR namespace = %q, want team-a", req.Spec.ResourceAttributes.Namespace)
		}
		return true, &authorizationv1.SelfSubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: req.Spec.ResourceAttributes.Resource == "pods"}}, nil
	})
	result, err := DiscoverCapabilities(context.Background(), client, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if !result[WorkloadView] || result[ProposalView] {
		t.Fatalf("unexpected capability result: %#v", result)
	}
	if got := calls.Load(); got != 13 {
		t.Fatalf("SSAR calls=%d, want exact-rule deduplication to reduce 14 to 13", got)
	}
}

func TestDiscoverCapabilitiesRejectsInvalidNamespace(t *testing.T) {
	if _, err := DiscoverCapabilities(context.Background(), fake.NewSimpleClientset(), "team/a"); err == nil {
		t.Fatal("expected invalid namespace rejection")
	}
}
