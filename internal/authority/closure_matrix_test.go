package authority

import (
	"testing"
	"time"
)

func TestFamilyIdentitySubstitutionMatrix(t *testing.T) {
	a, s, c, v, p, r := testInputs(t)
	trustSrc, _ := newTrustResolutionResult(a, "subject", "policy", "root", s, c, v, r, p, Trusted)
	trust, _ := DeriveTrustFactAt(trustSrc, v.ObservedAt())
	verSrc, _ := newVerificationExecutionResult(a, "subject", "verifier", "property", s, c, v, r, p, VerificationFactVerified)
	ver, _ := DeriveVerificationFactAt(verSrc, v.ObservedAt())
	revSrc, _ := newCurrentRevocationResult(a, "subject", "source", RevocationNotRevoked, p, v)
	rev, _ := DeriveCurrentRevocationFactAt(revSrc, v.ObservedAt())
	covSrc, _ := newCoverageObservationResult(a, "subject", "backend", "source", s, c, v, r, ScopeCovers, p)
	cov, _ := DeriveCoverageFact(covSrc)
	compSrc, _ := newCompletenessEvidenceResult(a, "subject", EmpiricalCompleteness, s, p, v, r)
	comp, _ := DeriveCompletenessFact(compSrc)
	adSrc, _ := newAdequacyEvidenceResult(a, "subject", StructuralBaseline, s, c, verSrcIdentity(verSrc), p, v, r)
	ad, _ := DeriveAdequacyFact(adSrc)
	certSrc, _ := newCertificationResolutionResult(a, "subject", "certificate", "property", s, c, verSrcIdentity(verSrc), p, v, r)
	cert, _ := DeriveCertificationFact(certSrc)
	cases := []struct {
		name   string
		base   MatchRequest
		snap   EvaluationFactSnapshot
		mutate func(*MatchRequest)
	}{
		{"trust-subject", MatchRequest{Family: FamilyTrust, Attempt: a, Authority: "a", Requirement: "r", Subject: "subject", Policy: "policy", Root: "root", Scope: s, At: v.ObservedAt()}, EvaluationFactSnapshot{attempt: a, trusts: []TrustFact{trust}}, func(q *MatchRequest) { q.Subject = "other" }},
		{"verification-property", MatchRequest{Family: FamilyVerification, Attempt: a, Authority: "a", Requirement: "r", Subject: "subject", Producer: "verifier", Property: "property", Scope: s, At: v.ObservedAt()}, EvaluationFactSnapshot{attempt: a, verifications: []ResolvedVerificationFact{ver}}, func(q *MatchRequest) { q.Property = "other" }},
		{"revocation-source", MatchRequest{Family: FamilyRevocation, Attempt: a, Authority: "a", Requirement: "r", Subject: "subject", Source: "source", Scope: s, At: v.ObservedAt()}, EvaluationFactSnapshot{attempt: a, revocations: []CurrentRevocationFact{rev}}, func(q *MatchRequest) { q.Source = "other" }},
		{"coverage-context", MatchRequest{Family: FamilyCoverage, Attempt: a, Authority: "a", Requirement: "r", Subject: "subject", Backend: "backend", Source: "source", TypedContext: c, Scope: s, At: v.ObservedAt()}, EvaluationFactSnapshot{attempt: a, coverages: []CoverageFact{cov}}, func(q *MatchRequest) {
			q.TypedContext, _ = NewSecurityContextIdentity(SecurityContextIdentity{ImageIdentity: "other", Architecture: "arch", ABI: "abi", KernelRuntimeClass: "kernel", WorkloadIdentity: "workload", ExecutableIdentity: "exe"})
		}},
		{"completeness-subject", MatchRequest{Family: FamilyCompleteness, Attempt: a, Authority: "a", Requirement: "r", Subject: "subject", Scope: s, At: v.ObservedAt()}, EvaluationFactSnapshot{attempt: a, completeness: []CompletenessFact{comp}}, func(q *MatchRequest) { q.Subject = "other" }},
		{"adequacy-subject", MatchRequest{Family: FamilyAdequacy, Attempt: a, Authority: "a", Requirement: "r", Subject: "subject", Scope: s, At: v.ObservedAt()}, EvaluationFactSnapshot{attempt: a, adequacies: []AdequacyFact{ad}}, func(q *MatchRequest) { q.Subject = "other" }},
		{"certification-property", MatchRequest{Family: FamilyCertification, Attempt: a, Authority: "a", Requirement: "r", Subject: "subject", Source: "certificate", Property: "property", Scope: s, At: v.ObservedAt()}, EvaluationFactSnapshot{attempt: a, certifications: []CertificationFact{cert}}, func(q *MatchRequest) { q.Property = "other" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, e := MatchSnapshot(tc.base, tc.snap)
			if e != nil || m.outcome != MatchSatisfied {
				t.Fatalf("control: %#v %v", m, e)
			}
			q := tc.base
			tc.mutate(&q)
			m, e = MatchSnapshot(q, tc.snap)
			if e != nil && m.outcome == MatchSatisfied {
				t.Fatalf("mutation unexpectedly satisfied: %#v %v", m, e)
			}
			if e == nil && m.outcome == MatchSatisfied {
				t.Fatal("identity substitution satisfied")
			}
		})
	}
}

func TestFamilyDuplicateConflictAndAttemptMatrix(t *testing.T) {
	a, s, c, v, p, r := testInputs(t)
	src, _ := newTrustResolutionResult(a, "subject", "policy", "root", s, c, v, r, p, Trusted)
	f, _ := DeriveTrustFact(src)
	if _, e := NewEvaluationFactSnapshot(a, []TrustFact{f, f}, nil, nil, nil); e != nil {
		t.Fatalf("exact duplicate rejected: %v", e)
	}
	neg, _ := newTrustResolutionResult(a, "subject", "policy", "root", s, c, v, r, p, Untrusted)
	nf, _ := DeriveTrustFact(neg)
	if _, e := NewEvaluationFactSnapshot(a, []TrustFact{f, nf}, nil, nil, nil); e == nil {
		t.Fatal("conflicting trust facts accepted")
	}
	zero := ResolutionAttemptIdentity("")
	if _, e := NewEvaluationFactSnapshot(zero, []TrustFact{f}, nil, nil, nil); e == nil {
		t.Fatal("zero attempt accepted")
	}
	b, _ := NewResolutionAttemptIdentity("other")
	r2 := r
	r2.attempt = b
	src2, e := newTrustResolutionResult(b, "subject", "policy", "root", s, c, v, r2, p, Trusted)
	if e != nil {
		t.Fatal(e)
	}
	f2, _ := DeriveTrustFact(src2)
	if _, e = NewEvaluationFactSnapshot(a, []TrustFact{f, f2}, nil, nil, nil); e == nil {
		t.Fatal("mixed attempts accepted")
	}
}

func TestCrossFamilyRuntimeDispatchFailsClosed(t *testing.T) {
	a, s, _, v, _, _ := testInputs(t)
	snap := EvaluationFactSnapshot{attempt: a}
	for _, family := range []FactFamily{FamilyTrust, FamilyVerification, FamilyRevocation, FamilyCompatibility, FamilyCoverage, FamilyCompleteness, FamilyAdequacy, FamilyCertification} {
		m, e := MatchSnapshot(MatchRequest{Family: family, Attempt: a, Authority: "a", Requirement: "r", Subject: "subject", Scope: s, At: v.ObservedAt()}, snap)
		if e == nil && m.outcome == MatchSatisfied {
			t.Fatalf("family %v satisfied without family fact", family)
		}
	}
}

func TestTemporalApplicabilityMatrix(t *testing.T) {
	start := time.Unix(100, 0)
	end := start.Add(time.Hour)
	v, e := NewValidity(start, &end, 0)
	if e != nil {
		t.Fatal(e)
	}
	cases := []struct {
		name string
		at   time.Time
		want TemporalApplicability
	}{{"lower", start, TemporalApplicable}, {"upper", end, TemporalApplicable}, {"before", start.Add(-time.Nanosecond), TemporalNotYetValid}, {"after", end.Add(time.Nanosecond), TemporalExpired}, {"missing", time.Time{}, TemporalInvalid}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckValidityAt(v, tc.at); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	a, s, c, _, p, r := testInputs(t)
	src, _ := newTrustResolutionResult(a, "subject", "policy", "root", s, c, v, r, p, Trusted)
	if _, e := DeriveTrustFactAt(src, start.Add(-time.Nanosecond)); e == nil {
		t.Fatal("future result admitted")
	}
	if _, e := DeriveTrustFactAt(src, end.Add(time.Nanosecond)); e == nil {
		t.Fatal("expired result admitted")
	}
}
