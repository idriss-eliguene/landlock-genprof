package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyClusterRBACHasExplicitOwnershipSwitch(t *testing.T) {
	values, err := os.ReadFile("helm/landlock-genprof/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(values), "legacyClusterRoles:") || !strings.Contains(string(values), "create: true") {
		t.Fatal("legacy cluster RBAC ownership default is not explicit")
	}
	for _, name := range []string{
		"rbac-applyattempt.yaml",
		"rbac-base.yaml",
		"rbac-patched-manifest.yaml",
		"rbac-proposal.yaml",
		"rbac-rollbackattempt.yaml",
	} {
		body, err := os.ReadFile("helm/landlock-genprof/templates/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), ".Values.rbac.legacyClusterRoles.create") {
			t.Fatalf("%s lacks explicit legacy cluster-RBAC ownership gate", name)
		}
	}
}

func TestExecutorGadgetRBACHasExplicitOwnershipSwitch(t *testing.T) {
	values, err := os.ReadFile("helm/landlock-genprof/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(values), "gadgetAccess:") {
		t.Fatal("executor Gadget RBAC ownership default is not explicit")
	}
	body, err := os.ReadFile("helm/landlock-genprof/templates/rbac-observation-executor.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), ".Values.observationExecutor.gadgetAccess.create") {
		t.Fatal("executor Gadget RBAC lacks explicit ownership gate")
	}
}

func TestOwnershipDocumentationRejectsSilentAdoption(t *testing.T) {
	for _, path := range []string{"helm/landlock-genprof/README.md", "../docs/v08-governance-operations.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, required := range []string{"never adopts", "foreign", "legacyClusterRoles.create=false"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing ownership guidance %q", path, required)
			}
		}
	}
}
