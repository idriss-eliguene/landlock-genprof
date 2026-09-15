package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestObservationExecutorRBACIsTechnicalAndNarrow(t *testing.T) {
	b, err := os.ReadFile("rbac-observation-executor.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "landlock-genprof-observation-executor") || strings.Contains(s, "landlock-genprof-tracer") {
		t.Fatal("executor must use a distinct ServiceAccount")
	}
	for _, required := range []string{"pods/portforward", "verbs: [\"list\"]"} {
		if !strings.Contains(s, required) {
			t.Fatalf("executor RBAC missing %s", required)
		}
	}
	for _, forbidden := range []string{"securityprofileproposals/status", "applyattempts", "rollbackattempts", "networkpolicies", "landlockprofiles", "seccompprofiles", "deployments", "statefulsets", "daemonsets", "resources: [\"secrets\"]"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("executor RBAC unexpectedly contains %s", forbidden)
		}
	}
	if !strings.Contains(s, "name: landlock-genprof-observation-executor-cluster-identity") || !strings.Contains(s, "resourceNames: [\"kube-system\"]") {
		t.Fatal("executor must retain only the bounded cluster identity read")
	}
}
