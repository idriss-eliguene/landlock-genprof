package main

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// TestCertificationApply is an explicitly opt-in real-cluster seam used by
// the pairwise Golden. It is skipped for normal test runs and cannot be
// reached through the production CLI. The same approved proposal and plan
// validation as apply-proposal are retained; only the already-reviewed
// composition guard is bypassed for this disposable certification run.
func TestCertificationApply(t *testing.T) {
	name := os.Getenv("LANDLOCK_CERTIFICATION_PROPOSAL")
	if name == "" {
		t.Skip("certification proposal not requested")
	}
	ns := os.Getenv("LANDLOCK_CERTIFICATION_NAMESPACE")
	if ns == "" {
		t.Fatal("LANDLOCK_CERTIFICATION_NAMESPACE is required")
	}
	var out bytes.Buffer
	err := runApplyProposalInternal(t.Context(), &out, bytes.NewReader(nil), applyProposalOptions{
		namespace:        ns,
		yes:              true,
		restart:          true,
		readinessTimeout: 3 * time.Minute,
	}, name, true)
	if err != nil {
		t.Fatalf("certification apply failed: %v\n%s", err, out.String())
	}
	output := os.Getenv("LANDLOCK_CERTIFICATION_OUTPUT")
	if output == "" {
		t.Fatal("LANDLOCK_CERTIFICATION_OUTPUT is required")
	}
	if err := os.WriteFile(output, out.Bytes(), 0o600); err != nil {
		t.Fatalf("write certification output: %v", err)
	}
}
