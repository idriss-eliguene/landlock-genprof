package authority

import (
	"testing"
	"time"
)

func predicateTestScope(t *testing.T) Scope {
	t.Helper()
	s, err := NewScope([]ScopeDimensionResult{{ScopeTemporal, ScopeCovers}, {ScopeProcessTree, ScopeCoverageUnknown}}, "target", "context")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func predicateTestContext(t *testing.T) SecurityContextIdentity {
	t.Helper()
	c, err := NewSecurityContextIdentity(SecurityContextIdentity{ImageIdentity: "image", Architecture: "arch", ABI: "abi", KernelRuntimeClass: "kernel", WorkloadIdentity: "workload", ExecutableIdentity: "exe"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func predicateTestVerifier(t *testing.T) VerifierSemanticIdentity {
	t.Helper()
	v, err := NewVerifierSemanticIdentity(VerifierSemanticIdentity{id: "v", version: "1", digest: Digest("sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), class: "class", inputSchema: "in", outputSchema: "out", property: "property", procedure: "procedure"})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func predicateTestValidity() Validity { return Validity{observedAt: time.Unix(100, 0).UTC()} }

func predicateTestProvenance(t *testing.T) ProvenanceRecord {
	t.Helper()
	p, err := NewProvenanceRecord("producer", "mechanism", "1", "run", predicateTestScope(t), predicateTestValidity(), RevocationUnknown, predicateTestVerifier(t))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTypedPredicateDomainValidation(t *testing.T) {
	if _, err := NewScope(nil, "target", "context"); err == nil {
		t.Fatal("empty scope accepted")
	}
	if _, err := NewCoverageRecord(nil, Scope{}, Scope{}, CoverageMeasured, "trace", ""); err == nil {
		t.Fatal("coverage without evidence accepted")
	}
	if _, err := NewCertification("c", "1", Digest(""), "issuer", CertificationScopeCoverage, "seccomp", Scope{}, SecurityContextIdentity{}, VerifierSemanticIdentity{}, Validity{}, RevocationUnknown); err == nil {
		t.Fatal("invalid certification accepted")
	}
	if _, err := NewCompatibilityRule(CompatibilityRuleRef{}, CompatibilityExactEquality, "image", "x"); err == nil {
		t.Fatal("invalid compatibility rule accepted")
	}
	if _, err := NewBackendSecurityEnvelope([]EnvelopeDimensionResult{{Dimension: StartupBootstrap, State: EnvelopeSatisfied}}); err == nil {
		t.Fatal("partial envelope accepted")
	}
}

func TestBackendEnvelopeIsConstructedNotAsserted(t *testing.T) {
	d := make([]EnvelopeDimensionResult, 0, 8)
	for dimension := StartupBootstrap; dimension <= KernelRuntimeCompatibility; dimension++ {
		d = append(d, EnvelopeDimensionResult{Dimension: dimension, State: EnvelopeSatisfied})
	}
	envelope, err := NewBackendSecurityEnvelope(d)
	if err != nil || envelope.Evaluate() != EnvelopeSatisfied {
		t.Fatalf("envelope construction failed: %v", err)
	}
	d[0].State = EnvelopeUnsatisfied
	if envelope.Evaluate() != EnvelopeSatisfied {
		t.Fatal("envelope aliased caller input")
	}
}

func TestPredicateScopeClosedAndImmutable(t *testing.T) {
	valid := []ScopeDimensionResult{{ScopeTemporal, ScopeCovers}, {ScopeProcessTree, ScopeCoverageUnknown}}
	s, err := NewScope(valid, "target", "context")
	if err != nil {
		t.Fatal(err)
	}
	valid[0].State = ScopeDoesNotCover
	if s.Dimensions()[0].State != ScopeCovers {
		t.Fatal("scope aliases input")
	}
	got := s.Dimensions()
	got[0].State = ScopeDoesNotCover
	if s.Dimensions()[0].State != ScopeCovers {
		t.Fatal("scope getter aliases state")
	}
	for _, token := range []string{"arbitrary", "", " temporal", "TEMPORAL", "temporal "} {
		if _, err := NewScope([]ScopeDimensionResult{{ScopeDimension(token), ScopeCovers}}, "target", "context"); err == nil {
			t.Fatalf("invalid dimension %q accepted", token)
		}
	}
	if _, err := NewScope([]ScopeDimensionResult{{ScopeTemporal, ScopeCovers}, {ScopeTemporal, ScopeCoverageUnknown}}, "target", "context"); err == nil {
		t.Fatal("duplicate dimension accepted")
	}
	if (Scope{}).Valid() {
		t.Fatal("zero scope valid")
	}
}

func TestPredicateNestedValidationAndTrustBoundary(t *testing.T) {
	s := predicateTestScope(t)
	c := predicateTestContext(t)
	v := predicateTestVerifier(t)
	valid := predicateTestValidity()
	if _, err := NewAdequacyEvidence(StructuralBaseline, "id", "issuer", "backend", Scope{}, c, v, valid, RevocationUnknown, predicateTestProvenance(t)); err == nil {
		t.Fatal("zero adequacy scope accepted")
	}
	if _, err := NewCertification("id", "1", Digest("sha256:"+"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), "issuer", CertificationScopeCoverage, "backend", s, SecurityContextIdentity{}, v, valid, RevocationUnknown); err == nil {
		t.Fatal("zero certification context accepted")
	}
	if _, err := NewVerificationFact(VerifierSemanticIdentity{}, "property", "subject", "backend", "input", "provenance", s, c, VerificationVerified, valid, RevocationUnknown); err == nil {
		t.Fatal("zero verifier accepted")
	}
	if _, err := NewTemporalValidity(Validity{}, PresencePresent); err == nil {
		t.Fatal("zero temporal validity accepted")
	}
	if _, err := NewTrustAuthorization("issuer", "artifact", "backend", []string{"property"}, s, c, valid, RevocationUnknown, Trusted); err == nil {
		t.Fatal("caller minted trusted state")
	}
}

func TestPredicateEnumClosure(t *testing.T) {
	tests := []struct {
		name          string
		valid         func() bool
		zero, unknown func() bool
	}{
		{"coverage state", func() bool { return ScopeCovers.Valid() && ScopeDoesNotCover.Valid() && ScopeCoverageUnknown.Valid() }, func() bool { return ScopeCoverageState(0).Valid() }, func() bool { return ScopeCoverageState(99).Valid() }},
		{"coverage class", func() bool {
			return CoverageMeasured.Valid() && CoverageAssumed.Valid() && CoverageExternallyCertified.Valid() && CoverageUnknown.Valid()
		}, func() bool { return CoverageClass(0).Valid() }, func() bool { return CoverageClass(99).Valid() }},
		{"completeness", func() bool {
			return EmpiricalCompleteness.Valid() && StructuralCompleteness.Valid() && DeclaredCompleteness.Valid() && ExternallyCertifiedCompleteness.Valid()
		}, func() bool { return CompletenessClass(0).Valid() }, func() bool { return CompletenessClass(99).Valid() }},
		{"adequacy", func() bool {
			return StructuralBaseline.Valid() && ExternalCertification.Valid() && BackendFormalInvariant.Valid() && BoundedBehavioral.Valid() && TrustedBaselineObservedDelta.Valid()
		}, func() bool { return AdequacyClass(0).Valid() }, func() bool { return AdequacyClass(99).Valid() }},
		{"certification", func() bool {
			return CertificationScopeCoverage.Valid() && CertificationBaselineCompatibility.Valid() && CertificationPolicyAdequacyBounded.Valid() && CertificationProvenanceValidity.Valid()
		}, func() bool { return CertificationProperty(0).Valid() }, func() bool { return CertificationProperty(99).Valid() }},
		{"verification", func() bool {
			return VerificationVerified.Valid() && VerificationFailed.Valid() && VerificationUnknown.Valid()
		}, func() bool { return VerificationResult(0).Valid() }, func() bool { return VerificationResult(99).Valid() }},
		{"compatibility", func() bool {
			return CompatibilityExactEquality.Valid() && CompatibilitySetMembership.Valid() && CompatibilityExactVersion.Valid() && CompatibilityVersionRange.Valid() && CompatibilityArchitectureABI.Valid() && CompatibilityDigestEquality.Valid()
		}, func() bool { return CompatibilityPredicate(0).Valid() }, func() bool { return CompatibilityPredicate(99).Valid() }},
		{"composition", func() bool {
			return RequireEqual.Valid() && SetUnionIfIdenticalAction.Valid() && SetIntersection.Valid() && RejectOnConflict.Valid() && PreserveBaseline.Valid()
		}, func() bool { return CompositionOperation(0).Valid() }, func() bool { return CompositionOperation(99).Valid() }},
		{"trust", func() bool { return Trusted.Valid() && Untrusted.Valid() && TrustUnknown.Valid() }, func() bool { return TrustState(0).Valid() }, func() bool { return TrustState(99).Valid() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.valid() || tc.zero() || tc.unknown() {
				t.Fatal("enum closure failure")
			}
		})
	}
}

func TestPredicateConstructorsRejectUnknownEnums(t *testing.T) {
	s := predicateTestScope(t)
	c := predicateTestContext(t)
	v := predicateTestVerifier(t)
	valid := predicateTestValidity()
	p := predicateTestProvenance(t)
	digest := Digest("sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if _, err := NewScope([]ScopeDimensionResult{{ScopeTemporal, ScopeCoverageState(99)}}, "target", "context"); err == nil {
		t.Fatal("unknown scope state accepted")
	}
	if _, err := NewCompletenessRecord(CompletenessClass(99), s, "issuer", "id", "", valid, RevocationUnknown); err == nil {
		t.Fatal("unknown completeness accepted")
	}
	if _, err := NewAdequacyEvidence(AdequacyClass(99), "id", "issuer", "backend", s, c, v, valid, RevocationUnknown, p); err == nil {
		t.Fatal("unknown adequacy accepted")
	}
	if _, err := NewCertification("id", "1", digest, "issuer", CertificationProperty(99), "backend", s, c, v, valid, RevocationUnknown); err == nil {
		t.Fatal("unknown certification accepted")
	}
	if _, err := NewVerificationFact(v, "property", "subject", "backend", "input", "provenance", s, c, VerificationResult(99), valid, RevocationUnknown); err == nil {
		t.Fatal("unknown verification accepted")
	}
	ref, _ := NewCompatibilityRuleRef("id", "1", digest)
	if _, err := NewCompatibilityRule(ref, CompatibilityPredicate(99), "field", "expected"); err == nil {
		t.Fatal("unknown compatibility accepted")
	}
	op, _ := NewCompositionOperatorRef("id", "1", digest)
	if _, err := NewComposition(op, CompositionOperation(99), []Digest{digest}); err == nil {
		t.Fatal("unknown composition accepted")
	}
	if _, err := NewTrustAuthorization("issuer", "artifact", "backend", []string{"property"}, s, c, valid, RevocationUnknown, TrustState(99)); err == nil {
		t.Fatal("unknown trust state accepted")
	}
}
