package authority

import (
	"testing"
	"time"
)

func TestCompatibilityMatchingEnforcesApplicability(t *testing.T) {
	a, scope, _, validity, prov, rev := testInputs(t)
	end := validity.ObservedAt().Add(time.Hour)
	validity.validUntil = &end
	future := validity.ObservedAt().Add(-time.Second)
	base := CompatibilityFact{attempt: a, schema: string(CompatibilityRuleV2), predicate: "SET_CONTAINS", field: "capabilities", candidate: "candidate", baseline: "baseline", requirement: "compat-req", authority: "authority", subject: "subject", backend: "backend", context: "context", scope: scope, validity: validity, revocation: rev, provenance: prov, state: CompatibilityCompatible}
	match := func(t *testing.T, f CompatibilityFact, at time.Time) RequirementMatchOutcome {
		t.Helper()
		r, err := MatchSnapshot(MatchRequest{Family: FamilyCompatibility, Attempt: a, Authority: "authority", Requirement: "compat-req", Subject: "subject", Backend: "backend", Context: "context", Scope: scope, At: at, Schema: string(CompatibilityRuleV2), Predicate: "SET_CONTAINS", Field: "capabilities", Candidate: "candidate", Baseline: "baseline"}, EvaluationFactSnapshot{attempt: a, compatibilities: []CompatibilityFact{f}})
		if err != nil {
			return MatchInvalid
		}
		return r.outcome
	}
	if got := match(t, base, validity.ObservedAt()); got != MatchSatisfied {
		t.Fatalf("valid compatible fact: %v", got)
	}
	if got := match(t, base, end); got != MatchSatisfied {
		t.Fatalf("upper validity boundary: %v", got)
	}
	if got := match(t, base, time.Time{}); got == MatchSatisfied {
		t.Fatal("missing evaluation time satisfied")
	}
	if got := match(t, base, future); got == MatchSatisfied {
		t.Fatal("future evaluation satisfied")
	}
	if got := match(t, base, end.Add(time.Nanosecond)); got == MatchSatisfied {
		t.Fatal("expired evaluation satisfied")
	}
	malformed := base
	malformed.validity = Validity{}
	if got := match(t, malformed, validity.ObservedAt()); got == MatchSatisfied {
		t.Fatal("malformed validity satisfied")
	}
	for _, state := range []RevocationStatus{RevocationRevoked, RevocationUnknown} {
		f := base
		f.revocation.state = state
		if got := match(t, f, validity.ObservedAt()); got == MatchSatisfied {
			t.Fatalf("revocation %v satisfied", state)
		}
	}
}

func TestDirectCompatibilityMatchingUsesCanonicalContract(t *testing.T) {
	a, scope, _, validity, prov, rev := testInputs(t)
	end := validity.ObservedAt().Add(time.Hour)
	validity.validUntil = &end
	fact := CompatibilityFact{attempt: a, schema: string(CompatibilityRuleV2), predicate: "SET_CONTAINS", field: "capabilities", candidate: "candidate", baseline: "baseline", requirement: "compat-req", authority: "authority", subject: "subject", backend: "backend", context: "context", scope: scope, validity: validity, revocation: rev, provenance: prov, state: CompatibilityCompatible}
	request := MatchRequest{Family: FamilyCompatibility, Attempt: a, Authority: "authority", Requirement: "compat-req", Subject: "subject", Backend: "backend", Context: "context", Scope: scope, At: validity.ObservedAt(), Schema: string(CompatibilityRuleV2), Predicate: "SET_CONTAINS", Field: "capabilities", Candidate: "candidate", Baseline: "baseline"}
	direct := func(req MatchRequest, f CompatibilityFact) RequirementMatchOutcome {
		r, err := MatchCompatibility(req, []CompatibilityFact{f})
		if err != nil {
			return MatchInvalid
		}
		return r.outcome
	}
	if got := direct(request, fact); got != MatchSatisfied {
		t.Fatalf("direct positive: %v", got)
	}
	for _, tc := range []struct {
		name  string
		at    time.Time
		state RevocationStatus
	}{
		{"future", validity.ObservedAt().Add(-time.Second), RevocationNotRevoked},
		{"expired", end.Add(time.Nanosecond), RevocationNotRevoked},
		{"revoked", validity.ObservedAt(), RevocationRevoked},
		{"unknown-revocation", validity.ObservedAt(), RevocationUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request
			r.At = tc.at
			f := fact
			f.revocation.state = tc.state
			if got := direct(r, f); got == MatchSatisfied {
				t.Fatalf("unsafe direct satisfaction: %v", got)
			}
		})
	}
	if got := direct(request, fact); got != MatchSatisfied {
		t.Fatalf("control no longer satisfies: %v", got)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*MatchRequest)
	}{
		{"schema", func(r *MatchRequest) { r.Schema = "compatibility-rule.v1" }},
		{"predicate", func(r *MatchRequest) { r.Predicate = "EQUAL" }},
		{"field", func(r *MatchRequest) { r.Field = "other" }},
		{"candidate", func(r *MatchRequest) { r.Candidate = "other" }},
		{"baseline", func(r *MatchRequest) { r.Baseline = "other" }},
		{"requirement", func(r *MatchRequest) { r.Requirement = "other" }},
		{"authority", func(r *MatchRequest) { r.Authority = "other" }},
		{"subject", func(r *MatchRequest) { r.Subject = "other" }},
		{"backend", func(r *MatchRequest) { r.Backend = "other" }},
		{"context", func(r *MatchRequest) { r.Context = "other" }},
		{"scope", func(r *MatchRequest) { r.Scope = Scope{} }},
		{"attempt", func(r *MatchRequest) { r.Attempt = ResolutionAttemptIdentity("other") }},
	} {
		t.Run("binding-"+tc.name, func(t *testing.T) {
			r := request
			tc.mutate(&r)
			if got := direct(r, fact); got == MatchSatisfied {
				t.Fatalf("under-specified/substituted request satisfied: %v", got)
			}
		})
	}
	missing := request
	missing.At = time.Time{}
	if got := direct(missing, fact); got == MatchSatisfied {
		t.Fatal("missing evaluation time satisfied")
	}
}

func TestFamilyIdentityIncludesProvenance(t *testing.T) {
	a, scope, ctx, validity, p1, rev := testInputs(t)
	p2, err := NewProvenanceRecord("other-producer", "mechanism", "1", "run", scope, validity, RevocationUnknown, p1.verification)
	if err != nil {
		t.Fatal(err)
	}
	trust := func(p ProvenanceRecord) TrustFact {
		r, _ := newTrustResolutionResult(a, "subject", "policy", "root", scope, ctx, validity, rev, p, Trusted)
		f, _ := DeriveTrustFact(r)
		return f
	}
	verification := func(p ProvenanceRecord) ResolvedVerificationFact {
		r, _ := newVerificationExecutionResult(a, "subject", "verifier", "property", scope, ctx, validity, rev, p, VerificationFactVerified)
		f, _ := DeriveVerificationFact(r)
		return f
	}
	revocation := func(p ProvenanceRecord) CurrentRevocationFact {
		r, _ := newCurrentRevocationResult(a, "subject", "source", RevocationNotRevoked, p, validity)
		f, _ := DeriveCurrentRevocationFact(r)
		return f
	}
	compatibility := func(p ProvenanceRecord) CompatibilityFact {
		return CompatibilityFact{attempt: a, schema: string(CompatibilityRuleV2), predicate: "SET_CONTAINS", field: "capabilities", candidate: "candidate", baseline: "baseline", requirement: "compat-req", authority: "authority", subject: "subject", backend: "backend", context: "context", scope: scope, validity: validity, revocation: rev, provenance: p, state: CompatibilityCompatible}
	}
	coverage := func(p ProvenanceRecord) CoverageFact {
		r, _ := newCoverageObservationResult(a, "subject", "backend", "source", scope, ctx, validity, rev, ScopeCovers, p)
		f, _ := DeriveCoverageFact(r)
		return f
	}
	completeness := func(p ProvenanceRecord) CompletenessFact {
		r, _ := newCompletenessEvidenceResult(a, "subject", EmpiricalCompleteness, scope, p, validity, rev)
		f, _ := DeriveCompletenessFact(r)
		return f
	}
	adequacy := func(p ProvenanceRecord) AdequacyFact {
		r, _ := newAdequacyEvidenceResult(a, "subject", StructuralBaseline, scope, ctx, verSrcIdentityForTest(p), p, validity, rev)
		f, _ := DeriveAdequacyFact(r)
		return f
	}
	certification := func(p ProvenanceRecord) CertificationFact {
		r, _ := newCertificationResolutionResult(a, "subject", "certificate", "property", scope, ctx, verSrcIdentityForTest(p), p, validity, rev)
		f, _ := DeriveCertificationFact(r)
		return f
	}
	if _, err := NewEvaluationFactSnapshotAll(a, []TrustFact{trust(p1), trust(p2)}, []CurrentRevocationFact{revocation(p1), revocation(p2)}, []ResolvedVerificationFact{verification(p1), verification(p2)}, []CompatibilityFact{compatibility(p1), compatibility(p2)}, []CoverageFact{coverage(p1), coverage(p2)}, []CompletenessFact{completeness(p1), completeness(p2)}, []AdequacyFact{adequacy(p1), adequacy(p2)}, []CertificationFact{certification(p1), certification(p2)}); err != nil {
		t.Fatalf("provenance-distinct facts collapsed or conflicted: %v", err)
	}
}

func verSrcIdentityForTest(p ProvenanceRecord) VerifierSemanticIdentity { return p.verification }
