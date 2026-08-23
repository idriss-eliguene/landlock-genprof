package authority

import (
	"strings"
	"testing"
)

func bindingMetadata(t *testing.T) AuthorityMetadata {
	d := testDigest(t)
	rule, _ := NewAuthorityRuleRef("rule", "v1", d)
	trust, _ := NewTrustPolicyRef("trust", "v1", d)
	base, _ := NewBaselineRef("base", "v1", d)
	reg, _ := NewRegistryRef("registry", "v1", d)
	comp, _ := NewCompatibilityRuleRef("compat", "v1", d)
	op, _ := NewCompositionOperatorRef("op", "v1", d)
	rb, _ := NewRegistryBinding(reg)
	ctx, _ := NewSecurityContextIdentity(SecurityContextIdentity{ImageIdentity: "img", Architecture: "amd64", ABI: "linux", KernelRuntimeClass: "k", WorkloadIdentity: "workload", ExecutableIdentity: "bin"})
	m, err := NewAuthorityMetadata(AuthorityMetadata{authorityRule: rule, trustPolicy: trust, baseline: base, registries: []RegistryBinding{rb}, compatibility: comp, composition: op, context: ctx})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCanonicalArtifactOrderAndDomain(t *testing.T) {
	a := ArtifactProjection{Container: "c", Binary: "b", PodLock: "p"}
	b1, err := CanonicalArtifactBytes(a)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := CanonicalArtifactBytes(ArtifactProjection{Binary: "b", Container: "c", PodLock: "p"})
	if string(b1) != string(b2) {
		t.Fatal("field order changed canonical bytes")
	}
	d, _, _ := ArtifactDigestOf(a)
	if !d.Valid() {
		t.Fatal("invalid artifact digest")
	}
	if strings.Contains(string(b1), "\n") {
		t.Fatal("canonical bytes contain newline")
	}
}

func TestAuthorityCanonicalIdentityChanges(t *testing.T) {
	m := bindingMetadata(t)
	d1, b1, err := AuthorityMetadataDigestOf(m)
	if err != nil {
		t.Fatal(err)
	}
	d2, b2, _ := AuthorityMetadataDigestOf(m)
	if d1 != d2 || string(b1) != string(b2) {
		t.Fatal("canonicalization is not deterministic")
	}
	ctx := m.Context()
	ctx.ImageIdentity = "other"
	m2, err := NewAuthorityMetadata(AuthorityMetadata{authorityRule: m.AuthorityRule(), trustPolicy: m.TrustPolicy(), baseline: m.Baseline(), registries: m.Registries(), compatibility: m.Compatibility(), composition: m.Composition(), context: ctx})
	if err != nil {
		t.Fatal(err)
	}
	d3, _, _ := AuthorityMetadataDigestOf(m2)
	if d1 == d3 {
		t.Fatal("context mutation did not change identity")
	}
}

func TestAuthorityReferenceSetOrderDoesNotChangeIdentity(t *testing.T) {
	m := bindingMetadata(t)
	d := testDigest(t)
	r2, _ := NewRegistryRef("registry-2", "v1", d)
	b2, _ := NewRegistryBinding(r2)
	m2, err := NewAuthorityMetadata(AuthorityMetadata{authorityRule: m.AuthorityRule(), trustPolicy: m.TrustPolicy(), baseline: m.Baseline(), registries: []RegistryBinding{b2, m.Registries()[0]}, compatibility: m.Compatibility(), composition: m.Composition(), context: m.Context()})
	if err != nil {
		t.Fatal(err)
	}
	m3, _ := NewAuthorityMetadata(AuthorityMetadata{authorityRule: m.AuthorityRule(), trustPolicy: m.TrustPolicy(), baseline: m.Baseline(), registries: []RegistryBinding{m.Registries()[0], b2}, compatibility: m.Compatibility(), composition: m.Composition(), context: m.Context()})
	d1, _, _ := AuthorityMetadataDigestOf(m2)
	d2, _, _ := AuthorityMetadataDigestOf(m3)
	if d1 != d2 {
		t.Fatal("registry set order changed authority identity")
	}
}

func TestAuthorityRejectsDuplicateRegistryIdentity(t *testing.T) {
	m := bindingMetadata(t)
	regs := m.Registries()
	_, err := NewAuthorityMetadata(AuthorityMetadata{authorityRule: m.AuthorityRule(), trustPolicy: m.TrustPolicy(), baseline: m.Baseline(), registries: []RegistryBinding{regs[0], regs[0]}, compatibility: m.Compatibility(), composition: m.Composition(), context: m.Context()})
	if err == nil {
		t.Fatal("duplicate registry identity accepted")
	}
}

func TestTypedReferenceKindAndBoundIdentity(t *testing.T) {
	d := testDigest(t)
	r, _ := NewAuthorityRuleRef("same", "v1", d)
	b, _ := NewBaselineRef("same", "v1", d)
	if string(r.Digest()) != string(b.Digest()) {
		t.Fatal("setup")
	}
	a, _, _ := ArtifactDigestOf(ArtifactProjection{Container: "c"})
	m := bindingMetadata(t)
	md, _, _ := AuthorityMetadataDigestOf(m)
	bd1, _, _ := BoundCandidateDigestOf(a, md)
	other, _, _ := ArtifactDigestOf(ArtifactProjection{Container: "d"})
	bd2, _, _ := BoundCandidateDigestOf(other, md)
	if bd1 == bd2 {
		t.Fatal("artifact substitution did not change bound identity")
	}
}

func TestModernDigestsRejectLegacyAndMalformed(t *testing.T) {
	if _, _, err := BoundCandidateDigestOf(ArtifactDigest("sha256:bad"), AuthorityMetadataDigest("sha256:bad")); err == nil {
		t.Fatal("malformed modern digest accepted")
	}
	if ArtifactDigest("sha256:bad").Valid() {
		t.Fatal("bad digest valid")
	}
	if _, err := CanonicalAuthorityBytes(AuthorityMetadata{}); err == nil {
		t.Fatal("zero authority metadata canonicalized")
	}
	if _, err := CanonicalArtifactBytes(ArtifactProjection{Container: string([]byte{0xff})}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestGoldenArtifactVector(t *testing.T) {
	d, b, err := ArtifactDigestOf(ArtifactProjection{Container: "c"})
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := `{"Binary":"","Container":"c","NetworkPolicy":"","PatchedManifest":"","PodLock":"","SPOSeccompProfile":"","artifactSchema":"1"}`
	if string(b) != wantBytes || d != ArtifactDigest("sha256:aa41ebfd721ca8e85a652a2f53809f107908702448a227e7ad6923b86cfe0d28") {
		t.Fatalf("golden mismatch: %s %s", b, d)
	}
}

func TestGoldenAuthorityVector(t *testing.T) {
	d, b, err := AuthorityMetadataDigestOf(bindingMetadata(t))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"authorityRule":{"digest":{"algorithm":"sha256","hex":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"id":"rule","kind":"AuthorityRuleRef","version":"v1"},"authoritySchema":"1","baseline":{"digest":{"algorithm":"sha256","hex":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"id":"base","kind":"BaselineRef","version":"v1"},"certifications":[],"compatibility":{"digest":{"algorithm":"sha256","hex":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"id":"compat","kind":"CompatibilityRuleRef","version":"v1"},"composition":{"digest":{"algorithm":"sha256","hex":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"id":"op","kind":"CompositionOperatorRef","version":"v1"},"context":{"abi":"linux","architecture":"amd64","configuration":"","environment":"","executable":"bin","featureSet":"","image":"img","kernelRuntime":"k","libc":"","namespaceSecurity":"","persistentState":"","privilege":"","workload":"workload"},"evidence":[],"registries":[{"digest":{"algorithm":"sha256","hex":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"id":"registry","kind":"RegistryRef","version":"v1"}],"trustPolicy":{"digest":{"algorithm":"sha256","hex":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"id":"trust","kind":"TrustPolicyRef","version":"v1"},"verifiers":[]}`
	if string(b) != want || d != AuthorityMetadataDigest("sha256:dd8f32575a3514b361a7ff8c3f4a4108b0bff4e0570fc54b4d4a8f43faa423c6") {
		t.Fatal("authority golden vector mismatch")
	}
}

func TestCanonicalStringRules(t *testing.T) {
	a, _ := CanonicalArtifactBytes(ArtifactProjection{Container: "\u00e9"})
	b, _ := CanonicalArtifactBytes(ArtifactProjection{Container: "e\u0301"})
	if string(a) == string(b) {
		t.Fatal("unicode normalization occurred")
	}
	c, _ := CanonicalArtifactBytes(ArtifactProjection{Container: "quote\\\"\n"})
	if !strings.Contains(string(c), "\\\"") || !strings.Contains(string(c), "\\n") {
		t.Fatalf("control/string escaping missing: %s", c)
	}
}
