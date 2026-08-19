package semantic

import (
	"go/build"
	"strings"
	"testing"
)

func TestNoForbiddenDependency(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("build.ImportDir error: %v", err)
	}

	forbidden := []string{
		"internal/profile",
		"internal/landlock",
		"internal/exporter",
		"internal/policy",
		"internal/history",
		"internal/proposal",
		"k8s.io/",
		"sigs.k8s.io/",
		"github.com/inspektor-gadget",
	}

	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("internal/semantic imports %q (forbidden match %q)", imp, bad)
			}
		}
	}
}
