package main

import (
	"os"
	"strings"
	"testing"
)

func TestMakefile_ApplyProposalUsesAuthoritativeCLIPath(t *testing.T) {
	b, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("reading Makefile: %v", err)
	}
	content := string(b)

	start := strings.Index(content, "\napply-proposal:")
	if start == -1 {
		t.Fatal("apply-proposal target not found in Makefile")
	}
	rest := content[start+1:]
	end := strings.Index(rest, "\n\n")
	block := rest
	if end >= 0 {
		block = rest[:end]
	}

	if !strings.Contains(block, "kubectl landlock-genprof apply-proposal") {
		t.Fatalf("apply-proposal target must route through authoritative CLI path; block:\n%s", block)
	}
	if strings.Contains(block, "kubectl apply -f") {
		t.Fatalf("apply-proposal target must not directly kubectl apply exported spec artifacts; block:\n%s", block)
	}
}

func TestMakefile_ExportProposalMarkedNonAuthoritative(t *testing.T) {
	b, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("reading Makefile: %v", err)
	}
	content := string(b)

	start := strings.Index(content, "\nexport-proposal:")
	if start == -1 {
		t.Fatal("export-proposal target not found in Makefile")
	}
	rest := content[start+1:]
	end := strings.Index(rest, "\n\n")
	block := rest
	if end >= 0 {
		block = rest[:end]
	}

	for _, marker := range []string{
		"inspection only",
		"non-authoritative",
		"Do NOT apply them",
		"kubectl landlock-genprof apply-proposal",
	} {
		if !strings.Contains(block, marker) {
			t.Fatalf("export-proposal target missing marker %q; block:\n%s", marker, block)
		}
	}
}
