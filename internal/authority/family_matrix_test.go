package authority

import (
	"testing"
	"time"
)

func positiveFamilyInputs(t *testing.T) (ResolutionAttemptIdentity, Scope, SecurityContextIdentity, Validity, ProvenanceRecord, CurrentRevocationFact) {
	return testInputs(t)
}

func TestEightFamilyPositiveControls(t *testing.T) {
	a, scope, ctx, validity, prov, rev := positiveFamilyInputs(t)
	trustSrc, _ := newTrustResolutionResult(a, "subject", "policy", "root", scope, ctx, validity, rev, prov, Trusted)
	trust, _ := DeriveTrustFactAt(trustSrc, validity.ObservedAt())
	verSrc, _ := newVerificationExecutionResult(a, "subject", "verifier", "property", scope, ctx, validity, rev, prov, VerificationFactVerified)
	ver, _ := DeriveVerificationFactAt(verSrc, validity.ObservedAt())
	revSrc, _ := newCurrentRevocationResult(a, "subject", "source", RevocationNotRevoked, prov, validity)
	revFact, _ := DeriveCurrentRevocationFactAt(revSrc, validity.ObservedAt())
	setA, _ := NewTypedSet([]string{"a"}, false)
	setB, _ := NewTypedSet([]string{"a", "b"}, false)
	compatSrc := CompatibilityEvaluationResult{attempt: a, schema: string(CompatibilityRuleV2), predicate: "SET_CONTAINS", field: "capabilities", candidate: "candidate", baseline: "baseline", requirement: "compat-req", authority: "authority", subject: "subject", backend: "backend", context: "context", scope: scope, validity: validity, revocation: rev, provenance: prov, outcome: CompatibilityResultCompatible}
	compat, _ := DeriveCompatibilityFact(compatSrc)
	covSrc, _ := newCoverageObservationResult(a, "subject", "backend", "source", scope, ctx, validity, rev, ScopeCovers, prov)
	cov, _ := DeriveCoverageFact(covSrc)
	compSrc, _ := newCompletenessEvidenceResult(a, "subject", EmpiricalCompleteness, scope, prov, validity, rev)
	comp, _ := DeriveCompletenessFact(compSrc)
	adequacySrc, _ := newAdequacyEvidenceResult(a, "subject", StructuralBaseline, scope, ctx, verSrcIdentity(verSrc), prov, validity, rev)
	adequacy, _ := DeriveAdequacyFact(adequacySrc)
	certSrc, _ := newCertificationResolutionResult(a, "subject", "certificate", "property", scope, ctx, verSrcIdentity(verSrc), prov, validity, rev)
	cert, _ := DeriveCertificationFact(certSrc)
	if !trust.Valid() || !ver.Valid() || !revFact.Valid() || !compat.Valid() || !cov.Valid() || !comp.Valid() || !adequacy.Valid() || !cert.Valid() {
		t.Fatal("positive source-result derivation failed")
	}
	if out, err := MatchSnapshot(MatchRequest{Family: FamilyTrust, Attempt: a, Authority: "authority", Requirement: "trust", Subject: "subject", Policy: "policy", Root: "root", Scope: scope, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: a, trusts: []TrustFact{trust}}); err != nil || out.outcome != MatchSatisfied {
		t.Fatalf("trust positive: %#v %v", out, err)
	}
	if out, err := MatchSnapshot(MatchRequest{Family: FamilyVerification, Attempt: a, Authority: "authority", Requirement: "verification", Subject: "subject", Producer: "verifier", Property: "property", Scope: scope, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: a, verifications: []ResolvedVerificationFact{ver}}); err != nil || out.outcome != MatchSatisfied {
		t.Fatalf("verification positive: %#v %v", out, err)
	}
	if out, err := MatchSnapshot(MatchRequest{Family: FamilyRevocation, Attempt: a, Authority: "authority", Requirement: "revocation", Subject: "subject", Source: "source", Scope: scope, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: a, revocations: []CurrentRevocationFact{revFact}}); err != nil || out.outcome != MatchSatisfied {
		t.Fatalf("revocation positive: %#v %v", out, err)
	}
	if out, err := MatchSnapshot(MatchRequest{Family: FamilyCompatibility, Attempt: a, Authority: "authority", Requirement: "compat-req", Subject: "subject", Backend: "backend", Context: "context", Scope: scope, At: validity.ObservedAt(), Schema: string(CompatibilityRuleV2), Predicate: "SET_CONTAINS", Field: "capabilities", Candidate: "candidate", Baseline: "baseline"}, EvaluationFactSnapshot{attempt: a, compatibilities: []CompatibilityFact{compat}}); err != nil || out.outcome != MatchSatisfied {
		mm, ee := MatchCompatibility(MatchRequest{Family: FamilyCompatibility, Attempt: a, Authority: "authority", Requirement: "compat-req", Subject: "subject", Backend: "backend", Context: "context", Scope: scope, At: validity.ObservedAt(), Schema: string(CompatibilityRuleV2), Predicate: "SET_CONTAINS", Field: "capabilities", Candidate: "candidate", Baseline: "baseline"}, []CompatibilityFact{compat})
		t.Fatalf("compatibility positive: %#v %v direct=%#v/%v fact=%#v valid=%v", out, err, mm, ee, compat, compat.Valid())
	}
	if out, err := MatchSnapshot(MatchRequest{Family: FamilyCoverage, Attempt: a, Authority: "authority", Requirement: "coverage", Subject: "subject", Backend: "backend", Source: "source", TypedContext: ctx, Scope: scope, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: a, coverages: []CoverageFact{cov}}); err != nil || out.outcome != MatchSatisfied {
		t.Fatalf("coverage positive: %#v %v", out, err)
	}
	if out, err := MatchSnapshot(MatchRequest{Family: FamilyCompleteness, Attempt: a, Authority: "authority", Requirement: "completeness", Subject: "subject", Scope: scope, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: a, completeness: []CompletenessFact{comp}}); err != nil || out.outcome != MatchSatisfied {
		t.Fatalf("completeness positive: %#v %v", out, err)
	}
	if out, err := MatchSnapshot(MatchRequest{Family: FamilyAdequacy, Attempt: a, Authority: "authority", Requirement: "adequacy", Subject: "subject", Scope: scope, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: a, adequacies: []AdequacyFact{adequacy}}); err != nil || out.outcome != MatchSatisfied {
		t.Fatalf("adequacy positive: %#v %v", out, err)
	}
	if out, err := MatchSnapshot(MatchRequest{Family: FamilyCertification, Attempt: a, Authority: "authority", Requirement: "certification", Subject: "subject", Source: "certificate", Property: "property", Scope: scope, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: a, certifications: []CertificationFact{cert}}); err != nil || out.outcome != MatchSatisfied {
		t.Fatalf("certification positive: %#v %v", out, err)
	}
	_ = setA
	_ = setB
}

func verSrcIdentity(v VerificationExecutionResult) VerifierSemanticIdentity {
	return v.provenance.verification
}

func TestFamilyCrossSubstitutionFailsClosed(t *testing.T) {
	a, scope, _, validity, _, _ := positiveFamilyInputs(t)
	snap := EvaluationFactSnapshot{attempt: a}
	for _, tc := range []struct {
		name   string
		family FactFamily
	}{
		{"trust-as-verification", FamilyVerification}, {"verification-as-trust", FamilyTrust}, {"certification-as-trust", FamilyTrust}, {"coverage-as-completeness", FamilyCompleteness}, {"completeness-as-coverage", FamilyCoverage}, {"adequacy-as-certification", FamilyCertification}, {"compatibility-as-coverage", FamilyCoverage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := MatchSnapshot(MatchRequest{Family: tc.family, Attempt: a, Authority: "authority", Requirement: "req", Subject: "subject", Scope: scope, At: validity.ObservedAt()}, snap)
			if err == nil && m.outcome == MatchSatisfied {
				t.Fatal("cross-family substitution satisfied")
			}
		})
	}
}

func TestTemporalAndRevocationAdversarialMatrix(t *testing.T) {
	a, scope, ctx, validity, prov, rev := positiveFamilyInputs(t)
	src, _ := newTrustResolutionResult(a, "subject", "policy", "root", scope, ctx, validity, rev, prov, Trusted)
	cases := []struct {
		name string
		at   time.Time
		want TemporalApplicability
	}{
		{"inside", validity.ObservedAt(), TemporalApplicable}, {"before", validity.ObservedAt().Add(-time.Second), TemporalNotYetValid}, {"missing", time.Time{}, TemporalInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckValidityAt(validity, tc.at); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	if _, err := DeriveTrustFactAt(src, validity.ObservedAt()); err != nil {
		t.Fatal(err)
	}
	unknown, _ := newCurrentRevocationResult(a, "subject", "source", RevocationUnknown, prov, validity)
	src.revocation, _ = DeriveCurrentRevocationFact(unknown)
	if _, err := DeriveTrustFactAt(src, validity.ObservedAt()); err == nil {
		t.Fatal("unknown revocation admitted")
	}
}
