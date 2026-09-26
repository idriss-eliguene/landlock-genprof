package authority

import (
	"strings"
	"testing"
	"time"
)

func TestSeccompRuntimeEvidenceCarriesAttemptAndUnknownRevocation(t *testing.T) {
	now := time.Now().UTC()
	fact, err := NewSeccompRuntimeEvidenceFact(SeccompRuntimeEvidence{
		AttemptID: "attempt-123", Version: "1", Digest: "sha256:" + strings.Repeat("a", 64),
		Subject: "proposal/candidate", ImageIdentity: "sha256:" + strings.Repeat("b", 64),
		WorkloadIdentity: "deployment/pod-uid", Runtime: "containerd", ObservedAt: now,
		ValidUntil: now.Add(time.Minute), Result: VerificationFactVerified,
	})
	if err != nil || !fact.Valid() {
		t.Fatalf("fact=%+v err=%v", fact, err)
	}
	if fact.attempt != ResolutionAttemptIdentity("attempt-123") || fact.revocation.State() != RevocationUnknown {
		t.Fatalf("attempt/revocation were not preserved: %#v", fact)
	}
	if _, err := NewSeccompRuntimeEvidenceFact(SeccompRuntimeEvidence{AttemptID: "", Version: "1", Digest: "sha256:" + strings.Repeat("a", 64)}); err == nil {
		t.Fatal("empty resolution attempt accepted")
	}
}
