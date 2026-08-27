package authority

import (
	"testing"
	"time"
)

func testInputs(t *testing.T) (ResolutionAttemptIdentity, Scope, SecurityContextIdentity, Validity, ProvenanceRecord, CurrentRevocationFact) {
	t.Helper()
	a, _ := NewResolutionAttemptIdentity("attempt")
	s, _ := NewScope([]ScopeDimensionResult{{ScopeTemporal, ScopeCovers}}, "target", "context")
	c, _ := NewSecurityContextIdentity(SecurityContextIdentity{ImageIdentity: "image", Architecture: "arch", ABI: "abi", KernelRuntimeClass: "kernel", WorkloadIdentity: "workload", ExecutableIdentity: "exe"})
	v := Validity{observedAt: time.Unix(100, 0)}
	dig := Digest("sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	ver, _ := NewVerifierSemanticIdentity(VerifierSemanticIdentity{id: "verifier", version: "1", digest: dig, class: "class", inputSchema: "in", outputSchema: "out", property: "property", procedure: "procedure"})
	p, _ := NewProvenanceRecord("producer", "mechanism", "1", "run", s, v, RevocationUnknown, ver)
	rr, _ := newCurrentRevocationResult(a, "subject", "source", RevocationNotRevoked, p, v)
	r, _ := DeriveCurrentRevocationFact(rr)
	return a, s, c, v, p, r
}
func TestSourceResultDerivationAndAttemptCoherence(t *testing.T) {
	a, s, c, v, p, r := testInputs(t)
	tr, err := newTrustResolutionResult(a, "subject", "policy", "root", s, c, v, r, p, Trusted)
	if err != nil {
		t.Fatal(err)
	}
	f, err := DeriveTrustFact(tr)
	if err != nil || !f.Valid() {
		t.Fatal(err)
	}
	b, _ := NewResolutionAttemptIdentity("other")
	r2 := r
	r2.attempt = b
	if _, err := newTrustResolutionResult(a, "subject", "policy", "root", s, c, v, r2, p, Trusted); err == nil {
		t.Fatal("mixed attempt accepted")
	}
}
func TestCompatibilityV2Relations(t *testing.T) {
	a, _ := NewTypedSet([]string{"a"}, false)
	b, _ := NewTypedSet([]string{"a", "b"}, false)
	o, err := EvaluateCompatibility(CompatibilityRuleV2, CompatibilitySetContains, CompatibilityOperands{Set: a}, CompatibilityOperands{Set: b}, CompatibilityOperands{})
	if err != nil || o != CompatibilityResultCompatible {
		t.Fatalf("set containment: %v %v", o, err)
	}
	m1, _ := NewTypedMap(map[string]TypedMapValue{"A": {Value: "1"}}, false)
	m2, _ := NewTypedMap(map[string]TypedMapValue{"A": {Value: "1"}, "B": {Value: "2"}}, false)
	o, err = EvaluateCompatibility(CompatibilityRuleV2, CompatibilityMapContains, CompatibilityOperands{Map: m1}, CompatibilityOperands{Map: m2}, CompatibilityOperands{})
	if err != nil || o != CompatibilityResultCompatible {
		t.Fatalf("map containment: %v %v", o, err)
	}
	if o, err = EvaluateCompatibility(CompatibilityRuleV1, CompatibilitySetContains, CompatibilityOperands{Set: a}, CompatibilityOperands{Set: b}, CompatibilityOperands{}); err == nil || o != CompatibilityResultInvalid {
		t.Fatal("V1 containment accepted")
	}
}
func TestSnapshotRejectsMixedAttempts(t *testing.T) {
	a, s, c, v, p, r := testInputs(t)
	tr, _ := newTrustResolutionResult(a, "subject", "policy", "root", s, c, v, r, p, Untrusted)
	f, _ := DeriveTrustFact(tr)
	b, _ := NewResolutionAttemptIdentity("other")
	r2 := r
	r2.attempt = b
	if _, err := NewEvaluationFactSnapshot(a, []TrustFact{f}, []CurrentRevocationFact{r2}, nil, nil); err == nil {
		t.Fatal("mixed snapshot accepted")
	}
}

func TestCompatibilityAdversarialAndAggregation(t *testing.T) {
	a, _ := NewTypedSet([]string{"a"}, false)
	b, _ := NewTypedSet([]string{"a", "b"}, false)
	if o, e := EvaluateCompatibility(CompatibilityRuleV2, CompatibilitySetContains, CompatibilityOperands{Set: a}, CompatibilityOperands{Set: b}, CompatibilityOperands{ScalarKnown: true}); e == nil || o != CompatibilityResultInvalid {
		t.Fatal("expected containment expected-operand rejection")
	}
	if o, e := EvaluateCompatibility(CompatibilityRuleV1, CompatibilityMapContains, CompatibilityOperands{Map: TypedMap{}}, CompatibilityOperands{Map: TypedMap{}}, CompatibilityOperands{}); e == nil || o != CompatibilityResultInvalid {
		t.Fatal("expected V1 rejection")
	}
	if _, e := NewTypedMapEntries([]RawMapEntry{{Key: "A", Value: TypedMapValue{Value: "1"}}, {Key: "A", Value: TypedMapValue{Value: "1"}}}, false); e == nil {
		t.Fatal("duplicate raw key accepted")
	}
	if o, e := AggregateCompatibility(nil, nil); e != nil || o != CompatibilityResultNotApplicable {
		t.Fatal("nil requirements not applicable")
	}
	if o, e := AggregateCompatibility([]CompatibilityRequirement{{Authority: "a", Requirement: "r", Field: "x", Schema: CompatibilityRuleV2, Predicate: CompatibilitySetContains}, {Authority: "a", Requirement: "r2", Field: "y", Schema: CompatibilityRuleV2, Predicate: CompatibilitySetContains}}, []CompatibilityOutcome{CompatibilityResultCompatible, CompatibilityResultUnknown}); e != nil || o != CompatibilityResultUnknown {
		t.Fatal("unknown precedence")
	}
	if o, e := AggregateCompatibility([]CompatibilityRequirement{{Authority: "a", Requirement: "r", Field: "x", Schema: CompatibilityRuleV2, Predicate: CompatibilitySetContains}, {Authority: "a", Requirement: "r2", Field: "y", Schema: CompatibilityRuleV2, Predicate: CompatibilitySetContains}}, []CompatibilityOutcome{CompatibilityResultCompatible, CompatibilityResultIncompatible}); e != nil || o != CompatibilityResultIncompatible {
		t.Fatal("incompatible precedence")
	}
}

func TestCoverageTemporalAndRevocationApplicability(t *testing.T) {
	a, s, c, v, p, r := testInputs(t)
	cr, err := newCoverageObservationResult(a, "subject", "backend", "source", s, c, v, r, ScopeCovers, p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := DeriveCoverageFact(cr)
	if err != nil || !f.Valid() {
		t.Fatal(err)
	}
	if _, err := newCoverageObservationResult(a, "subject", "backend", "source", s, c, v, CurrentRevocationFact{}, ScopeCovers, p); err == nil {
		t.Fatal("invalid revocation accepted")
	}
	before := v.ObservedAt().Add(-time.Second)
	if CheckValidityAt(v, before) != TemporalNotYetValid {
		t.Fatal("future validity classification")
	}
	if _, err := MatchSnapshot(MatchRequest{Family: FamilyCoverage, Attempt: a, Authority: "auth", Requirement: "cov", Subject: "subject", Backend: "backend", Source: "source", Context: "context", Scope: s, At: before}, EvaluationFactSnapshot{attempt: a, coverages: []CoverageFact{f}}); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageMatchRejectsContextSubstitution(t *testing.T) {
	a, s, c, v, p, r := testInputs(t)
	cr, _ := newCoverageObservationResult(a, "subject", "backend", "source", s, c, v, r, ScopeCovers, p)
	f, _ := DeriveCoverageFact(cr)
	snap := EvaluationFactSnapshot{attempt: a, coverages: []CoverageFact{f}}
	base := MatchRequest{Family: FamilyCoverage, Attempt: a, Authority: "auth", Requirement: "cov", Subject: "subject", Backend: "backend", Source: "source", Scope: s, TypedContext: c, At: v.ObservedAt()}
	m, err := MatchSnapshot(base, snap)
	if err != nil || m.outcome != MatchSatisfied {
		t.Fatalf("exact context did not satisfy: %v %#v", err, m)
	}
	other, _ := NewSecurityContextIdentity(SecurityContextIdentity{ImageIdentity: "different", Architecture: "arch", ABI: "abi", KernelRuntimeClass: "kernel", WorkloadIdentity: "workload", ExecutableIdentity: "exe"})
	base.TypedContext = other
	m, err = MatchSnapshot(base, snap)
	if err != nil || m.outcome == MatchSatisfied {
		t.Fatalf("context substitution satisfied: %v %#v", err, m)
	}
}
