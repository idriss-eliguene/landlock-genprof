package authority

import (
	"reflect"
	"testing"
)

func TestCompletenessMatchingRequiresExactClass(t *testing.T) {
	a, s, _, v, p, rev := testInputs(t)
	f := CompletenessFact{attempt: a, subject: "subject", class: EmpiricalCompleteness, scope: s, provenance: p, validity: v, revocation: rev}
	snap, err := NewEvaluationFactSnapshotAll(a, nil, nil, nil, nil, nil, []CompletenessFact{f}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := MatchRequest{Family: FamilyCompleteness, Attempt: a, Authority: "auth", Requirement: "req", Subject: "subject", Scope: s, At: v.ObservedAt(), RequiredCompletenessClass: EmpiricalCompleteness}
	m, err := MatchSnapshot(req, snap)
	if err != nil || m.outcome != MatchSatisfied {
		t.Fatalf("positive class match: %v %v", m.outcome, err)
	}
	req.RequiredCompletenessClass = StructuralCompleteness
	m, err = MatchSnapshot(req, snap)
	if err != nil || m.outcome == MatchSatisfied {
		t.Fatalf("wrong class satisfied: %v %v", m.outcome, err)
	}
}

func TestAdequacyMatchingRequiresExactClass(t *testing.T) {
	a, s, c, v, p, rev := testInputs(t)
	dig := Digest("sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	ver, _ := NewVerifierSemanticIdentity(VerifierSemanticIdentity{id: "verifier", version: "1", digest: dig, class: "class", inputSchema: "in", outputSchema: "out", property: "property", procedure: "procedure"})
	f := AdequacyFact{attempt: a, subject: "subject", class: StructuralBaseline, scope: s, context: c, verifier: ver, provenance: p, validity: v, revocation: rev}
	snap, err := NewEvaluationFactSnapshotAll(a, nil, nil, nil, nil, nil, nil, []AdequacyFact{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := MatchRequest{Family: FamilyAdequacy, Attempt: a, Authority: "auth", Requirement: "req", Subject: "subject", Scope: s, TypedContext: c, Verifier: ver, At: v.ObservedAt(), RequiredAdequacyClass: StructuralBaseline}
	m, err := MatchSnapshot(req, snap)
	if err != nil || m.outcome != MatchSatisfied {
		t.Fatalf("positive class match: %v %v", m.outcome, err)
	}
	req.RequiredAdequacyClass = ExternalCertification
	m, err = MatchSnapshot(req, snap)
	if err != nil || m.outcome == MatchSatisfied {
		t.Fatalf("wrong class satisfied: %v %v", m.outcome, err)
	}
}

func TestRequirementClassAdaptersPreserveTypedOperands(t *testing.T) {
	a, s, c, v, p, rev := testInputs(t)
	dig := Digest("sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	ver, _ := NewVerifierSemanticIdentity(VerifierSemanticIdentity{id: "verifier", version: "1", digest: dig, class: "class", inputSchema: "in", outputSchema: "out", property: "property", procedure: "procedure"})
	bound := testResolvedRule(t)
	cr := NewCompletenessRequirement("subject", EmpiricalCompleteness, s)
	ar := NewAdequacyRequirement("subject", StructuralBaseline, s, c, ver)
	set, err := NewResolvedMandatoryRequirementSet(bound, a, []MandatoryRequirement{cr, ar})
	if err != nil {
		t.Fatal(err)
	}
	cm, err := set.MatchRequest(cr, v.ObservedAt())
	if err != nil || cm.RequiredCompletenessClass != EmpiricalCompleteness || !reflect.DeepEqual(cm.Scope, s) {
		t.Fatalf("completeness adapter lost operands: %#v %v", cm, err)
	}
	am, err := set.MatchRequest(ar, v.ObservedAt())
	if err != nil || am.RequiredAdequacyClass != StructuralBaseline || !reflect.DeepEqual(am.Scope, s) || am.TypedContext != c || am.Producer != ver.ID() {
		t.Fatalf("adequacy adapter lost operands: %#v %v", am, err)
	}
	_ = p
	_ = rev
}
