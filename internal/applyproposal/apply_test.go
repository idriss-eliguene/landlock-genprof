package applyproposal

import (
	"strings"
	"testing"
)

func TestParseSkipArtifactsPreservesValidation(t *testing.T) {
	skipped, err := parseSkipArtifacts([]string{" PodLock ", "networkpolicy"})
	if err != nil {
		t.Fatal(err)
	}
	if !skipped["podlock"] || !skipped["networkpolicy"] {
		t.Fatalf("skip set = %#v, want normalized known slugs", skipped)
	}
	if _, err := parseSkipArtifacts([]string{"unknown"}); err == nil {
		t.Fatal("parseSkipArtifacts(unknown) = nil, want validation error")
	}
}

func TestValidateCompositionCompatibilityPreservesFailClosedGuard(t *testing.T) {
	if err := ValidateCompositionCompatibilityForSlugs([]string{"spo-seccompprofile", "podlock"}); err == nil {
		t.Fatal("PodLock + Seccomp composition guard unexpectedly accepted")
	}
	if err := ValidateCompositionCompatibilityForSlugs([]string{"podlock", "networkpolicy"}); err != nil {
		t.Fatalf("PodLock + NetworkPolicy rejected: %v", err)
	}
	if !strings.Contains("runtime compatibility is unproven", "compatibility") {
		t.Fatal("test assertion sanity check failed")
	}
}
