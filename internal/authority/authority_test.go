package authority

import (
	"testing"
	"time"
)

func testDigest(t *testing.T) Digest {
	d, err := NewDigest("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDigestAndTypedReferences(t *testing.T) {
	d := testDigest(t)
	if _, err := NewAuthorityRuleRef("", "v1", d); err == nil {
		t.Fatal("empty ID accepted")
	}
	if _, err := NewAuthorityRuleRef("r", "", d); err == nil {
		t.Fatal("empty version accepted")
	}
	if _, err := NewAuthorityRuleRef("r", "v1", Digest("sha256:bad")); err == nil {
		t.Fatal("bad digest accepted")
	}
	r, err := NewAuthorityRuleRef("r", "v1", d)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBaselineRef("r", "v1", d)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID() != b.ID() {
		t.Fatal("typed references lost their kind boundary")
	}
}

func TestAuthorityMetadataCopiesCollections(t *testing.T) {
	d := testDigest(t)
	rule, _ := NewAuthorityRuleRef("rule", "v1", d)
	trust, _ := NewTrustPolicyRef("trust", "v1", d)
	base, _ := NewBaselineRef("base", "v1", d)
	reg, _ := NewRegistryRef("reg", "v1", d)
	comp, _ := NewCompatibilityRuleRef("compat", "v1", d)
	op, _ := NewCompositionOperatorRef("op", "v1", d)
	ev, _ := NewEvidenceRef("e", "v1", d)
	ctx, err := NewSecurityContextIdentity(SecurityContextIdentity{ImageIdentity: "img", Architecture: "amd64", ABI: "linux", KernelRuntimeClass: "k", WorkloadIdentity: "w", ExecutableIdentity: "x"})
	if err != nil {
		t.Fatal(err)
	}
	rb, _ := NewRegistryBinding(reg)
	input := AuthorityMetadata{authorityRule: rule, trustPolicy: trust, baseline: base, registries: []RegistryBinding{rb}, evidence: []EvidenceRef{ev}, compatibility: comp, composition: op, context: ctx}
	got, err := NewAuthorityMetadata(input)
	if err != nil {
		t.Fatal(err)
	}
	input.registries[0] = RegistryBinding{}
	input.evidence[0] = EvidenceRef{}
	regs := got.Registries()
	evs := got.Evidence()
	regs[0] = RegistryBinding{}
	evs[0] = EvidenceRef{}
	if !got.Registries()[0].Registry().Digest().Valid() || !got.Evidence()[0].Digest().Valid() {
		t.Fatal("constructor retained caller alias")
	}
}

func TestStatesRemainOrthogonal(t *testing.T) {
	if ReviewApproved == ReviewInvalid || EligibilityUnknown == EligibilityEligible || AuthorizationNotAuthorized == AuthorizationAuthorized {
		t.Fatal("invalid state collapse")
	}
	if MaterializationNotMaterialized == MaterializationMaterialized || RuntimeNotActive == RuntimeActive {
		t.Fatal("cross-dimension state values collapsed")
	}
}

func TestEligibilityRecordRequiresExplicitValues(t *testing.T) {
	if _, err := NewEligibilityRecord(EligibilityRecord{}); err == nil {
		t.Fatal("zero record accepted")
	}
	d := testDigest(t)
	rule, _ := NewAuthorityRuleRef("rule", "v1", d)
	trust, _ := NewTrustPolicyRef("trust", "v1", d)
	base, _ := NewBaselineRef("base", "v1", d)
	reg, _ := NewRegistryRef("reg", "v1", d)
	comp, _ := NewCompatibilityRuleRef("compat", "v1", d)
	op, _ := NewCompositionOperatorRef("op", "v1", d)
	ctx, _ := NewSecurityContextIdentity(SecurityContextIdentity{ImageIdentity: "img", Architecture: "amd64", ABI: "linux", KernelRuntimeClass: "k", WorkloadIdentity: "w", ExecutableIdentity: "x"})
	rb, _ := NewRegistryBinding(reg)
	meta, _ := NewAuthorityMetadata(AuthorityMetadata{authorityRule: rule, trustPolicy: trust, baseline: base, registries: []RegistryBinding{rb}, compatibility: comp, composition: op, context: ctx})
	validity, _ := NewValidity(time.Unix(1, 0), nil, time.Minute)
	if _, err := NewEligibilityRecord(EligibilityRecord{candidate: d, metadata: meta, result: EligibilityUnknown, validity: validity, revocation: RevocationUnknown, evaluationEpoch: 1}); err != nil {
		t.Fatal(err)
	}
}
