package semantic

import "testing"

func TestStructuralEqual_AtomicIdentity(t *testing.T) {
	a := NewIdentityRef("Subject", "A")
	b := NewIdentityRef("Subject", "A")
	c := NewIdentityRef("Subject", "B")
	if !StructuralEqual(a, b) {
		t.Fatal("expected A == A")
	}
	if StructuralEqual(a, c) {
		t.Fatal("expected A != B")
	}
}

func TestStructuralEqual_TupleOrder(t *testing.T) {
	x := NewTuple([]SnapshotValue{NewIdentityRef("Term", "A"), NewLiteral("string", "x")})
	y := NewTuple([]SnapshotValue{NewLiteral("string", "x"), NewIdentityRef("Term", "A")})
	if StructuralEqual(x, y) {
		t.Fatal("expected ordered tuples to differ")
	}
}

func TestStructuralEqual_SetMath(t *testing.T) {
	a := NewIdentityRef("Subject", "A")
	b := NewIdentityRef("Subject", "B")
	s1 := NewSet([]SnapshotValue{a, b})
	s2 := NewSet([]SnapshotValue{b, a})
	if !StructuralEqual(s1, s2) {
		t.Fatal("expected sets equal regardless of order")
	}
}

func TestStructuralEqual_DuplicateSetInput(t *testing.T) {
	a := NewIdentityRef("Subject", "A")
	s := NewSet([]SnapshotValue{a, a, a})
	if len(s.members) != 1 {
		t.Fatalf("expected duplicate inputs normalized, got %d", len(s.members))
	}
}

func TestStructuralEqual_NestedStructures(t *testing.T) {
	x := NewRecord(map[string]SnapshotValue{
		"p": NewProposition("pred", []SnapshotValue{NewLiteral("string", "v")}),
		"s": NewSet([]SnapshotValue{NewIdentityRef("Subject", "A")}),
	})
	y := NewRecord(map[string]SnapshotValue{
		"p": NewProposition("pred", []SnapshotValue{NewLiteral("string", "v")}),
		"s": NewSet([]SnapshotValue{NewIdentityRef("Subject", "A")}),
	})
	if !StructuralEqual(x, y) {
		t.Fatal("expected nested structures to be equal")
	}
}

func TestStructuralEqual_PropositionArgsOrder(t *testing.T) {
	p1 := NewProposition("Q", []SnapshotValue{NewLiteral("string", "a"), NewLiteral("string", "b")})
	p2 := NewProposition("Q", []SnapshotValue{NewLiteral("string", "b"), NewLiteral("string", "a")})
	if StructuralEqual(p1, p2) {
		t.Fatal("expected proposition positional args order to matter")
	}
}

func TestStructuralEqual_DifferentPredicate(t *testing.T) {
	p1 := NewProposition("P", []SnapshotValue{NewLiteral("string", "v")})
	p2 := NewProposition("R", []SnapshotValue{NewLiteral("string", "v")})
	if StructuralEqual(p1, p2) {
		t.Fatal("expected different predicate names to be unequal")
	}
}

func TestStructuralEqual_ReferenceAtomicity(t *testing.T) {
	a := NewIdentityRef("Assertion", "E1")
	b := NewIdentityRef("Assertion", "E2")
	// Even if hypothetically the referenced content were identical, the
	// identity comparison must remain atomic and not traverse referenced
	// content.
	if StructuralEqual(a, b) {
		t.Fatal("expected distinct assertion identities to be unequal")
	}
}
