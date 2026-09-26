package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestSeccompVerifierRBACIsDedicatedAndExecutionRestricted(t *testing.T) {
	b, err := os.ReadFile("rbac-seccomp-verifier.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"landlock-genprof-seccomp-verifier", `resources: ["pods"]`, `verbs: ["create", "get", "delete"]`, `resourceNames: ["kube-system"]`} {
		if !strings.Contains(s, want) {
			t.Fatalf("verifier RBAC missing %q", want)
		}
	}
	for _, forbidden := range []string{"pods/exec", "pods/ephemeralcontainers", "pods/log", `verbs: ["list"]`} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("verifier RBAC grants forbidden access %q", forbidden)
		}
	}
	proposalRole, err := os.ReadFile("rbac-proposal-verification.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(proposalRole), `verbs: ["get", "verify"]`) || strings.Contains(string(proposalRole), "pods/exec") {
		t.Fatal("proposal.verify permission is not isolated from Pod execution")
	}
}
