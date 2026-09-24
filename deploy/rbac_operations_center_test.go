package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestOperationsCenterRBACSeparatesBackendAndNamespaceGrants(t *testing.T) {
	b, err := os.ReadFile("rbac-operations-center.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "landlock-genprof-operations-center") || strings.Contains(s, "landlock-genprof-tracer") {
		t.Fatal("Operations Center backend must use a distinct ServiceAccount")
	}
	if !strings.Contains(s, "selfsubjectaccessreviews") {
		t.Fatal("backend plumbing must include SSAR")
	}
	if strings.Contains(s, "resources: [\"users\", \"groups\"]") && !strings.Contains(s, "resourceNames:") {
		t.Fatal("backend users/groups impersonation must be constrained")
	}
	if strings.Contains(s, "kind: RoleBinding") || strings.Contains(s, "kind: ClusterRoleBinding\nmetadata:\n  name: landlock-genprof-team") {
		t.Fatal("team capability roles must not ship with broad bindings")
	}
	for _, forbidden := range []string{"networkpolicies", "podlocks", "seccompapprofiles", "landlockprofiles", "pods/exec"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("backend RBAC unexpectedly grants %s", forbidden)
		}
	}
}

func TestOperationsCenterHelmImpersonationUsesCoreRBACResources(t *testing.T) {
	b, err := os.ReadFile("helm/landlock-genprof/templates/rbac-operations-center.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `apiGroups: [""]`) || !strings.Contains(s, `resources: ["users"]`) || !strings.Contains(s, `resources: ["groups"]`) {
		t.Fatal("human impersonation must use core users/groups RBAC resources")
	}
	if strings.Contains(s, `apiGroups: ["authentication.k8s.io"]`) {
		t.Fatal("human impersonation must not use authentication.k8s.io RBAC resources")
	}
}

func TestProfileRealizerHelmRoleHasOnlyBoundedIdentityReadOutsideSPO(t *testing.T) {
	b, err := os.ReadFile("helm/landlock-genprof/templates/rbac-operations-center-team.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	marker := "name: landlock-genprof-profile-realizer"
	start := strings.Index(s, marker)
	if start < 0 {
		t.Fatal("profile-realizer ClusterRole is missing")
	}
	end := strings.Index(s[start:], "---")
	if end < 0 {
		t.Fatal("profile-realizer ClusterRole is not delimited")
	}
	rule := s[start : start+end]
	if !strings.Contains(rule, `resources: ["namespaces"]`) || !strings.Contains(rule, `resourceNames: ["kube-system"]`) {
		t.Fatal("profile-realizer must have only the bounded kube-system identity read outside SPO")
	}
	if strings.Contains(rule, `resources: ["pods"]`) || strings.Contains(rule, `verbs: ["list"]`) {
		t.Fatal("profile-realizer unexpectedly has broad cluster read access")
	}
}
