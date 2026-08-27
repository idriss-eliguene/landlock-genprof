package authority

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"
)

const BindingMechanism = "landlock-genprof/bound-candidate/v1"

type ArtifactDigest string
type AuthorityMetadataDigest string
type BoundCandidateDigest string

func (d ArtifactDigest) Valid() bool          { return validDigest(string(d)) }
func (d AuthorityMetadataDigest) Valid() bool { return validDigest(string(d)) }
func (d BoundCandidateDigest) Valid() bool    { return validDigest(string(d)) }
func validDigest(s string) bool {
	if len(s) != 71 || s[:7] != "sha256:" {
		return false
	}
	_, err := hex.DecodeString(s[7:])
	return err == nil
}

type ArtifactProjection struct {
	Container         string
	Binary            string
	PodLock           string
	NetworkPolicy     string
	PatchedManifest   string
	SPOSeccompProfile string
}

func CanonicalArtifactBytes(a ArtifactProjection) ([]byte, error) {
	return canonical(map[string]any{"artifactSchema": "1", "Binary": a.Binary, "Container": a.Container,
		"NetworkPolicy": a.NetworkPolicy, "PatchedManifest": a.PatchedManifest,
		"PodLock": a.PodLock, "SPOSeccompProfile": a.SPOSeccompProfile})
}

func ArtifactDigestOf(a ArtifactProjection) (ArtifactDigest, []byte, error) {
	b, err := CanonicalArtifactBytes(a)
	if err != nil {
		return "", nil, err
	}
	return ArtifactDigest(hashDomain("landlock-genprof/artifact/v1", b)), b, nil
}

func canonical(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := writeCanonical(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeCanonical(b *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			if !utf8.ValidString(k) || containsNUL(k) {
				return fmt.Errorf("invalid object key")
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeString(b, k); err != nil {
				return err
			}
			b.WriteByte(':')
			if err := writeCanonical(b, x[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case string:
		if !utf8.ValidString(x) || containsNUL(x) {
			return fmt.Errorf("invalid string")
		}
		return writeString(b, x)
	case bool:
		enc, _ := json.Marshal(x)
		b.Write(enc)
	case int:
		b.WriteString(fmt.Sprintf("%d", x))
	case int64:
		b.WriteString(fmt.Sprintf("%d", x))
	case nil:
		b.WriteString("null")
	default:
		return fmt.Errorf("unsupported canonical scalar %T", v)
	}
	return nil
}
func writeString(b *bytes.Buffer, s string) error {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	raw := tmp.Bytes()
	b.Write(raw[:len(raw)-1])
	return nil
}
func containsNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}
func hashDomain(domain string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write([]byte{0})
	h.Write(payload)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func refValue(kind string, id, version string, digest Digest) map[string]any {
	return map[string]any{"digest": map[string]any{"algorithm": "sha256", "hex": digest.String()[7:]}, "id": id, "kind": kind, "version": version}
}

func sortedRefs[T any](refs []T, encode func(T) map[string]any) []any {
	vals := make([]map[string]any, len(refs))
	for i, r := range refs {
		vals[i] = encode(r)
	}
	sort.Slice(vals, func(i, j int) bool {
		a, _ := canonical(vals[i])
		b, _ := canonical(vals[j])
		return bytes.Compare(a, b) < 0
	})
	out := make([]any, len(vals))
	for i := range vals {
		out[i] = vals[i]
	}
	return out
}

func CanonicalAuthorityBytes(m AuthorityMetadata) ([]byte, error) {
	validated, err := NewAuthorityMetadata(m)
	if err != nil {
		return nil, err
	}
	m = validated
	regs := m.Registries()
	regv := sortedRefs(regs, func(r RegistryBinding) map[string]any {
		rr := r.Registry()
		return refValue("RegistryRef", rr.ID(), rr.Version(), rr.Digest())
	})
	ctx := m.Context()
	evs := m.Evidence()
	evv := sortedRefs(evs, func(r EvidenceRef) map[string]any { return refValue("EvidenceRef", r.ID(), r.Version(), r.Digest()) })
	vers := m.Verifiers()
	verv := sortedRefs(vers, func(r VerifierRef) map[string]any { return refValue("VerifierRef", r.ID(), r.Version(), r.Digest()) })
	certs := m.Certifications()
	certv := sortedRefs(certs, func(r CertificationRef) map[string]any {
		return refValue("CertificationRef", r.ID(), r.Version(), r.Digest())
	})
	root := map[string]any{"authoritySchema": "1", "authorityRule": refValue("AuthorityRuleRef", m.AuthorityRule().ID(), m.AuthorityRule().Version(), m.AuthorityRule().Digest()), "baseline": refValue("BaselineRef", m.Baseline().ID(), m.Baseline().Version(), m.Baseline().Digest()), "certifications": certv, "compatibility": refValue("CompatibilityRuleRef", m.Compatibility().ID(), m.Compatibility().Version(), m.Compatibility().Digest()), "composition": refValue("CompositionOperatorRef", m.Composition().ID(), m.Composition().Version(), m.Composition().Digest()), "context": map[string]any{"abi": ctx.ABI, "architecture": ctx.Architecture, "configuration": ctx.ConfigurationID, "environment": ctx.EnvironmentID, "executable": ctx.ExecutableIdentity, "featureSet": ctx.FeatureSetID, "image": ctx.ImageIdentity, "kernelRuntime": ctx.KernelRuntimeClass, "libc": ctx.LibcIdentity, "namespaceSecurity": ctx.NamespaceSecurity, "persistentState": ctx.PersistentStateID, "privilege": ctx.PrivilegeContext, "workload": ctx.WorkloadIdentity}, "evidence": evv, "registries": regv, "trustPolicy": refValue("TrustPolicyRef", m.TrustPolicy().ID(), m.TrustPolicy().Version(), m.TrustPolicy().Digest()), "verifiers": verv}
	return canonical(root)
}

func AuthorityMetadataDigestOf(m AuthorityMetadata) (AuthorityMetadataDigest, []byte, error) {
	b, err := CanonicalAuthorityBytes(m)
	if err != nil {
		return "", nil, err
	}
	return AuthorityMetadataDigest(hashDomain("landlock-genprof/authority-metadata/v1", b)), b, nil
}

func CanonicalBoundCandidateBytes(a ArtifactDigest, m AuthorityMetadataDigest) ([]byte, error) {
	if !a.Valid() || !m.Valid() {
		return nil, fmt.Errorf("invalid modern digest")
	}
	return canonical(map[string]any{"artifactDigest": map[string]any{"algorithm": "sha256", "hex": string(a)[7:]}, "authorityMetadataDigest": map[string]any{"algorithm": "sha256", "hex": string(m)[7:]}, "bindingMechanism": map[string]any{"id": "landlock-genprof/bound-candidate", "version": "1"}, "hashAlgorithm": "sha256"})
}
func BoundCandidateDigestOf(a ArtifactDigest, m AuthorityMetadataDigest) (BoundCandidateDigest, []byte, error) {
	b, err := CanonicalBoundCandidateBytes(a, m)
	if err != nil {
		return "", nil, err
	}
	return BoundCandidateDigest(hashDomain("landlock-genprof/bound-candidate/v1", b)), b, nil
}
