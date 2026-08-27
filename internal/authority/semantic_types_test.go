package authority

import (
	"testing"
	"time"
)

func TestSemanticReferenceAndKinds(t *testing.T) {
	d, _ := NewDigest("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	v, _ := ParseSemanticVersion("1.2.3.4")
	r, err := NewSemanticReference(ObjectKindAuthorityRule, "rule", v, d)
	if err != nil || !r.Valid() {
		t.Fatal(err)
	}
	if err := r.ValidateKind(ObjectKindAuthorityRule); err != nil {
		t.Fatal(err)
	}
	if r.ValidateKind(ObjectKindTrustPolicy) == nil {
		t.Fatal("expected kind mismatch")
	}
	if _, err := ParseObjectKind("authority_rule"); err == nil {
		t.Fatal("case/alias accepted")
	}
}

func TestPrimitiveEnumParsing(t *testing.T) {
	for _, s := range []string{"OBSERVATION", "COVERAGE_RECORD", "COMPLETENESS_RECORD", "CERTIFICATION_RECORD", "VERIFICATION_RECORD", "PROVENANCE_RECORD", "BACKEND_REALIZATION"} {
		if _, err := ParseEvidenceClass(s); err != nil {
			t.Fatal(s, err)
		}
	}
	for _, s := range []string{"EMPIRICAL_COMPLETENESS", "STRUCTURAL_COMPLETENESS", "DECLARED_COMPLETENESS", "EXTERNALLY_CERTIFIED_COMPLETENESS"} {
		if _, err := ParseCompletenessClass(s); err != nil {
			t.Fatal(s, err)
		}
	}
	for _, s := range []string{"STRUCTURAL_BASELINE", "EXTERNAL_CERTIFICATION", "BACKEND_FORMAL_INVARIANT", "BOUNDED_BEHAVIORAL", "TRUSTED_BASELINE_OBSERVED_DELTA"} {
		if _, err := ParseAdequacyClass(s); err != nil {
			t.Fatal(s, err)
		}
	}
	for _, s := range []string{"", "unknown", " observation", "OBSERVATION "} {
		if _, err := ParseEvidenceClass(s); err == nil {
			t.Fatal("accepted", s)
		}
	}
}

func TestTypedScopeAndDimensions(t *testing.T) {
	dims := []ScopeDimension{ScopeDimension("CONTAINER_LIFETIME"), ScopeDimension("PROCESS_TREE")}
	s, err := NewRuleTargetScope(TargetScopeContainerLifetime, dims)
	if err != nil || !s.Valid() {
		t.Fatal(err)
	}
	if len(s.Dimensions()) != 2 || s.Class() != TargetScopeContainerLifetime {
		t.Fatal("scope mismatch")
	}
	dims[0] = ScopeDimension("bad")
	if !s.Valid() {
		t.Fatal("scope aliased caller input")
	}
	if _, err := NewRuleTargetScope(TargetScopeClass("container"), []ScopeDimension{ScopeDimension("PROCESS_TREE")}); err == nil {
		t.Fatal("unknown scope accepted")
	}
}

func TestSemanticValidityAndRevocationReference(t *testing.T) {
	n := time.Unix(10, 0).UTC()
	a, err := NewSemanticValidity(n, n)
	if err != nil || !a.Valid() || !a.NotBefore().Equal(a.NotAfter()) {
		t.Fatal(err)
	}
	if _, err := NewSemanticValidity(time.Time{}, n); err == nil {
		t.Fatal("zero validity accepted")
	}
	d, _ := NewDigest("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	v, _ := ParseSemanticVersion("1.0.0.0")
	r, _ := NewSemanticReference(ObjectKindRevocation, "rev", v, d)
	rr, err := NewRevocationReference(r)
	if err != nil || !rr.Valid() {
		t.Fatal(err)
	}
	for _, kind := range []ObjectKind{ObjectKindRegistry, ObjectKindTrustPolicy, ObjectKindBaseline} {
		wrong, _ := NewSemanticReference(kind, "rev", v, d)
		if _, err := NewRevocationReference(wrong); err == nil {
			t.Fatalf("accepted wrong revocation kind %s", kind)
		}
	}
}

func TestRevocationObjectKindClosed(t *testing.T) {
	if _, err := ParseObjectKind("REVOCATION"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Revocation", "revocation", " REVOCATION", "REVOCATION "} {
		if _, err := ParseObjectKind(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}
