package semantic

import "testing"

func TestTermIdentityImmutability(t *testing.T) {
	authority := NewSubjectIdentity("auth1")
	sorts := []string{"A", "B"}
	g := NewRecord(map[string]SnapshotValue{"k": NewLiteral("string", "v")})
	c := NewRecord(map[string]SnapshotValue{"k": NewLiteral("string", "v2")})
	tid, err := NewTermIdentity(authority, 2, sorts, g, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mutate original slices
	sorts[0] = "X"
	// mutate grounding record used to construct (should not affect stored)
	g.fields["k"] = NewLiteral("string", "changed")

	// verify TermIdentity unchanged
	as := tid.ArgumentSorts()
	if as[0] != "A" {
		t.Fatalf("expected defensive copy of argument sorts, got %v", as)
	}
	// verify grounding unchanged
	gv := tid.Grounding().(Record)
	lit := gv.fields["k"].(Literal)
	if lit.value != "v" {
		t.Fatalf("expected grounding preserved, got %s", lit.value)
	}
}

func TestSnapshotImmutabilityAccessor(t *testing.T) {
	// get members via direct access (not exposed) but ensure constructor
	// normalized duplicates; then try to mutate input and ensure set
	// remains independent.
	in := []SnapshotValue{NewIdentityRef("Subject", "X")}
	s := NewSet(in)
	in[0] = NewIdentityRef("Subject", "Y")
	if !StructuralEqual(s, NewSet([]SnapshotValue{NewIdentityRef("Subject", "X")})) {
		t.Fatal("expected set to be immutable w.r.t constructor input mutation")
	}
}
