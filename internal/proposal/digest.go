package proposal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
)

// CandidateDigest computes a deterministic digest over the approval-
// relevant parts of a Spec. The canonicalization is an outer wrapper
// that deterministically orders the selected fields; artifact strings
// themselves are preserved verbatim (no inner normalization).
func CandidateDigest(spec Spec) (string, error) {
	// Build a canonical candidate structure with fields in a stable
	// order. encoding/json marshals struct fields in declaration order,
	// so this guarantees deterministic outer representation.
	payload := struct {
		Container         string `json:"container"`
		Binary            string `json:"binary"`
		PodLock           string `json:"podLock"`
		NetworkPolicy     string `json:"networkPolicy"`
		PatchedManifest   string `json:"patchedManifest"`
		SPOSeccompProfile string `json:"spoSeccompProfile"`
	}{
		Container:         spec.Container,
		Binary:            spec.Binary,
		PodLock:           spec.PodLock,
		NetworkPolicy:     spec.NetworkPolicy,
		PatchedManifest:   spec.PatchedManifest,
		SPOSeccompProfile: spec.SPOSeccompProfile,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling candidate for digest: %w", err)
	}

	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}

var digestRegexp = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ValidateCandidateDigest ensures the digest string matches the expected
// canonical form.
func ValidateCandidateDigest(value string) error {
	if !digestRegexp.MatchString(value) {
		return fmt.Errorf("invalid candidate digest: %q", value)
	}
	return nil
}
