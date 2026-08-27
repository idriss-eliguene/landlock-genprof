package authority

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func testResolvedRule(t *testing.T) TypedResolvedAuthorityRule {
	t.Helper()
	r, _ := decodeCompleteRule(t)
	d, err := AuthorityRuleDigestOf(r)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := NewSemanticReference(ObjectKindAuthorityRule, r.id, r.version, Digest(d))
	if err != nil {
		t.Fatal(err)
	}
	return TypedResolvedAuthorityRule{reference: ref, rule: r, digest: d}
}

func testRequirementMembers(t *testing.T) ([]MandatoryRequirement, VerifierSemanticIdentity) {
	t.Helper()
	_, scope, context, _, _, _ := testInputs(t)
	digest, _ := NewDigest("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	verifier, err := NewVerifierSemanticIdentity(VerifierSemanticIdentity{id: "verifier", version: "1", digest: digest, class: "class", inputSchema: "in", outputSchema: "out", property: "property", procedure: "procedure", constraints: []string{"constraint-a", "constraint-b"}})
	if err != nil {
		t.Fatal(err)
	}
	return []MandatoryRequirement{
		NewTrustRequirement("subject", "policy", "root", scope, context),
		NewVerificationRequirement("subject", "verifier", "property", scope, context),
		NewRevocationStatusRequirement("subject", "source"),
		NewMandatoryCompatibilityRequirement(string(CompatibilityRuleV2), "SET_CONTAINS", "field", "candidate", "baseline", "compat-ref", "SECCOMP", "subject", scope, context),
		NewCoverageRequirement("subject", "SECCOMP", "source", scope, context),
		NewCompletenessRequirement("subject", EmpiricalCompleteness, scope),
		NewAdequacyRequirement("subject", StructuralBaseline, scope, context, verifier),
		NewCertificationRequirement("subject", "certificate", CertificationScopeCoverage, scope, context, verifier),
	}, verifier
}

func TestMandatoryRequirementIdentityAndStaticBoundary(t *testing.T) {
	a := NewRevocationStatusRequirement("subject", "source-a")
	b := NewRevocationStatusRequirement("subject", "source-b")
	ai, err := a.MemberIdentity()
	if err != nil {
		t.Fatal(err)
	}
	bi, err := b.MemberIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if string(ai) == string(bi) {
		t.Fatal("source substitution preserved identity")
	}
	if !a.Valid() || !b.Valid() {
		t.Fatal("valid static requirements rejected")
	}
	if a.RevocationStatus.SourceRef == "NOT_REVOKED" {
		t.Fatal("dynamic state entered static requirement")
	}
}

func TestResolvedMandatoryRequirementSetRejectsEmpty(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	d, _ := AuthorityRuleDigestOf(r)
	ref, _ := NewSemanticReference(ObjectKindAuthorityRule, r.id, r.version, Digest(d))
	bound := TypedResolvedAuthorityRule{reference: ref, rule: r, digest: d}
	attempt, _ := NewResolutionAttemptIdentity("attempt-1")
	if _, err := NewResolvedMandatoryRequirementSet(bound, attempt, nil); err == nil {
		t.Fatal("empty set accepted")
	}
}

func TestResolvedMandatoryRequirementSetCanonicalizesDuplicates(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	d, _ := AuthorityRuleDigestOf(r)
	ref, _ := NewSemanticReference(ObjectKindAuthorityRule, r.id, r.version, Digest(d))
	bound := TypedResolvedAuthorityRule{reference: ref, rule: r, digest: d}
	attempt, _ := NewResolutionAttemptIdentity("attempt-1")
	a := NewRevocationStatusRequirement("subject", "source-a")
	b := NewRevocationStatusRequirement("subject", "source-b")
	one, err := NewResolvedMandatoryRequirementSet(bound, attempt, []MandatoryRequirement{a, b})
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewResolvedMandatoryRequirementSet(bound, attempt, []MandatoryRequirement{b, a, a})
	if err != nil {
		t.Fatal(err)
	}
	if one.ID() != two.ID() {
		t.Fatal("permutation/duplicate changed set identity")
	}
	if len(two.Requirements()) != 2 {
		t.Fatalf("got %d requirements", len(two.Requirements()))
	}
}

func mutateRequirementSetWire(t *testing.T, wire []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(wire))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		t.Fatal(err)
	}
	mutate(object)
	out, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRequirementWireRejectsUnknownFields(t *testing.T) {
	bound := testResolvedRule(t)
	attempt, _ := NewResolutionAttemptIdentity("attempt")
	members, _ := testRequirementMembers(t)
	for _, member := range members {
		t.Run(string(member.Family), func(t *testing.T) {
			set, _ := NewResolvedMandatoryRequirementSet(bound, attempt, []MandatoryRequirement{member})
			wire, _ := EncodeResolvedMandatoryRequirementSet(set)
			attack := mutateRequirementSetWire(t, wire, func(object map[string]any) {
				object["requirements"].([]any)[0].(map[string]any)["satisfied"] = true
			})
			if _, err := DecodeResolvedMandatoryRequirementSet(attack, bound); err == nil {
				t.Fatal("unknown member field accepted")
			}
		})
	}
	set, _ := NewResolvedMandatoryRequirementSet(bound, attempt, []MandatoryRequirement{members[6]})
	wire, _ := EncodeResolvedMandatoryRequirementSet(set)
	for _, tc := range []struct {
		name string
		path func(map[string]any) map[string]any
	}{
		{"ruleRef", func(o map[string]any) map[string]any { return o["ruleRef"].(map[string]any) }},
		{"scope", func(o map[string]any) map[string]any {
			return o["requirements"].([]any)[0].(map[string]any)["scope"].(map[string]any)
		}},
		{"scope-dimension", func(o map[string]any) map[string]any {
			return o["requirements"].([]any)[0].(map[string]any)["scope"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)
		}},
		{"context", func(o map[string]any) map[string]any {
			return o["requirements"].([]any)[0].(map[string]any)["context"].(map[string]any)
		}},
		{"verifier", func(o map[string]any) map[string]any {
			return o["requirements"].([]any)[0].(map[string]any)["verifier"].(map[string]any)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attack := mutateRequirementSetWire(t, wire, func(object map[string]any) { tc.path(object)["unknown"] = true })
			if _, err := DecodeResolvedMandatoryRequirementSet(attack, bound); err == nil {
				t.Fatal("nested unknown field accepted")
			}
		})
	}
}

func TestRequirementWireRejectsNonCanonicalNumbers(t *testing.T) {
	bound := testResolvedRule(t)
	attempt, _ := NewResolutionAttemptIdentity("attempt")
	members, _ := testRequirementMembers(t)
	cases := []struct {
		name  string
		index int
		field string
	}{
		{"completeness", 5, "requiredClass"},
		{"adequacy", 6, "requiredClass"},
		{"certification", 7, "property"},
	}
	for _, tc := range cases {
		for _, value := range []json.Number{"1.5", "-1", "999999999999999999999999", "0"} {
			t.Run(fmt.Sprintf("%s/%s", tc.name, value), func(t *testing.T) {
				set, _ := NewResolvedMandatoryRequirementSet(bound, attempt, []MandatoryRequirement{members[tc.index]})
				wire, _ := EncodeResolvedMandatoryRequirementSet(set)
				attack := mutateRequirementSetWire(t, wire, func(object map[string]any) { object["requirements"].([]any)[0].(map[string]any)[tc.field] = value })
				if _, err := DecodeResolvedMandatoryRequirementSet(attack, bound); err == nil {
					t.Fatal("invalid number accepted")
				}
			})
		}
	}
	set, _ := NewResolvedMandatoryRequirementSet(bound, attempt, []MandatoryRequirement{members[0]})
	wire, _ := EncodeResolvedMandatoryRequirementSet(set)
	for _, value := range []json.Number{"1.5", "-1", "999999999999999999999999", "0"} {
		attack := mutateRequirementSetWire(t, wire, func(object map[string]any) {
			object["requirements"].([]any)[0].(map[string]any)["scope"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["state"] = value
		})
		if _, err := DecodeResolvedMandatoryRequirementSet(attack, bound); err == nil {
			t.Fatalf("invalid scope state %s accepted", value)
		}
	}
}

func TestVerifierConstraintsRoundTrip(t *testing.T) {
	bound := testResolvedRule(t)
	attempt, _ := NewResolutionAttemptIdentity("attempt")
	members, verifier := testRequirementMembers(t)
	for _, member := range []MandatoryRequirement{members[6], members[7]} {
		t.Run(string(member.Family), func(t *testing.T) {
			set, err := NewResolvedMandatoryRequirementSet(bound, attempt, []MandatoryRequirement{member})
			if err != nil {
				t.Fatal(err)
			}
			wire, err := EncodeResolvedMandatoryRequirementSet(set)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeResolvedMandatoryRequirementSet(wire, bound)
			if err != nil {
				t.Fatal(err)
			}
			got := decoded.Requirements()[0]
			var gotVerifier VerifierSemanticIdentity
			if got.Adequacy != nil {
				gotVerifier = got.Adequacy.Verifier
			} else {
				gotVerifier = got.Certification.Verifier
			}
			if !reflect.DeepEqual(gotVerifier, verifier) {
				t.Fatal("verifier identity changed")
			}
			before, _ := member.MemberIdentity()
			after, _ := got.MemberIdentity()
			if !bytes.Equal(before, after) || set.ID() != decoded.ID() {
				t.Fatal("member or set identity changed")
			}
		})
	}
}

func mutateRequirementIdentity(r *MandatoryRequirement, suffix string) {
	switch r.Family {
	case RequirementTrust:
		r.Trust.Subject += suffix
	case RequirementVerification:
		r.Verification.Subject += suffix
	case RequirementRevocation:
		r.RevocationStatus.Subject += suffix
	case RequirementCompatibility:
		r.Compatibility.Subject += suffix
	case RequirementCoverage:
		r.Coverage.Subject += suffix
	case RequirementCompleteness:
		r.Completeness.Subject += suffix
	case RequirementAdequacy:
		r.Adequacy.Subject += suffix
		r.Adequacy.Verifier.constraints[0] += suffix
	case RequirementCertification:
		r.Certification.Subject += suffix
		r.Certification.Verifier.constraints[0] += suffix
	}
}

func memberIdentities(t *testing.T, members []MandatoryRequirement) [][]byte {
	t.Helper()
	out := make([][]byte, len(members))
	for i, member := range members {
		identity, err := member.MemberIdentity()
		if err != nil {
			t.Fatal(err)
		}
		out[i] = identity
	}
	return out
}

func assertMemberIdentities(t *testing.T, want [][]byte, got []MandatoryRequirement) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("membership length changed: %d != %d", len(got), len(want))
	}
	for i, member := range got {
		identity, err := member.MemberIdentity()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want[i], identity) {
			t.Fatalf("member %d identity changed", i)
		}
	}
}

func TestResolvedRequirementSetDefensivelyCopiesConstructorInputs(t *testing.T) {
	bound := testResolvedRule(t)
	attempt, _ := NewResolutionAttemptIdentity("attempt")
	inputs, _ := testRequirementMembers(t)
	set, err := NewResolvedMandatoryRequirementSet(bound, attempt, inputs)
	if err != nil {
		t.Fatal(err)
	}
	originalID := set.ID()
	original := set.Requirements()
	originalIdentities := memberIdentities(t, original)
	for i := range inputs {
		clone := cloneMandatoryRequirement(inputs[i])
		before, _ := inputs[i].MemberIdentity()
		after, _ := clone.MemberIdentity()
		if !bytes.Equal(before, after) {
			t.Fatalf("family %s clone changed identity", inputs[i].Family)
		}
		mutateRequirementIdentity(&inputs[i], "-constructor-mutation")
	}
	if set.ID() != originalID {
		t.Fatal("constructor alias changed set identity")
	}
	assertMemberIdentities(t, originalIdentities, set.Requirements())
}

func TestResolvedRequirementSetDefensivelyCopiesAccessorResults(t *testing.T) {
	bound := testResolvedRule(t)
	attempt, _, _, validity, _, _ := testInputs(t)
	inputs, _ := testRequirementMembers(t)
	set, err := NewResolvedMandatoryRequirementSet(bound, attempt, inputs)
	if err != nil {
		t.Fatal(err)
	}
	originalID := set.ID()
	original := set.Requirements()
	originalIdentities := memberIdentities(t, original)
	mutated := set.Requirements()
	for i := range mutated {
		mutateRequirementIdentity(&mutated[i], "-getter-mutation")
		if _, err := set.MatchRequest(mutated[i], validity.ObservedAt()); err == nil {
			t.Fatalf("mutated %s copy retained authoritative membership", mutated[i].Family)
		}
	}
	if set.ID() != originalID {
		t.Fatal("getter alias changed set identity")
	}
	assertMemberIdentities(t, originalIdentities, set.Requirements())
	for _, member := range original {
		if _, err := set.MatchRequest(member, validity.ObservedAt()); err != nil {
			t.Fatalf("original %s member lost authority: %v", member.Family, err)
		}
	}
}
