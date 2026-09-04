package main

import (
	"os"
	"strings"
	"testing"
)

func TestWorkbenchReaderRoleIsReadOnlyAndUnbound(t *testing.T) {
	b, err := os.ReadFile("../../deploy/rbac-workbench.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, forbidden := range []string{"create", "update", "patch", "delete", "watch", "RoleBinding", "ClusterRoleBinding", "*"} {
		if strings.Contains(s, "verbs: [\""+forbidden) || strings.Contains(s, "resources: [\""+forbidden) || strings.Contains(s, forbidden+"\"") {
			t.Errorf("workbench reader contains forbidden authority token %q", forbidden)
		}
	}
	if !strings.Contains(s, "verbs: [\"get\", \"list\"]") || !strings.Contains(s, "customresourcedefinitions") {
		t.Fatal("workbench reader role does not expose the required bounded reads")
	}
}
