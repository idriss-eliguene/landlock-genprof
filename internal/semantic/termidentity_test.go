package semantic

import "testing"

func TestTermIdentityEqualityVariants(t *testing.T) {
	a := NewSubjectIdentity("A")
	b := NewSubjectIdentity("B")
	// base components
	sorts := []string{"X"}
	g := NewRecord(map[string]SnapshotValue{"p": NewLiteral("string", "v")})
	c := NewRecord(map[string]SnapshotValue{"q": NewLiteral("string", "w")})
	base, err := NewTermIdentity(a, 1, sorts, g, c)
	if err != nil {
		t.Fatal(err)
	}
	// change authority
	t2, err := NewTermIdentity(b, 1, []string{"X"}, g, c)
	if err != nil {
		t.Fatal(err)
	}
	if base.Equals(t2) {
		t.Fatal("different authority should differ")
	}
	// change arity
	t3, err := NewTermIdentity(a, 2, []string{"X", "Y"}, g, c)
	if err != nil {
		t.Fatal(err)
	}
	if base.Equals(t3) {
		t.Fatal("different arity should differ")
	}
	// change sorts
	t4, err := NewTermIdentity(a, 1, []string{"Y"}, g, c)
	if err != nil {
		t.Fatal(err)
	}
	if base.Equals(t4) {
		t.Fatal("different argument sorts should differ")
	}
	// change grounding
	g2 := NewRecord(map[string]SnapshotValue{"p": NewLiteral("string", "different")})
	t5, err := NewTermIdentity(a, 1, []string{"X"}, g2, c)
	if err != nil {
		t.Fatal(err)
	}
	if base.Equals(t5) {
		t.Fatal("different grounding should differ")
	}
	// change consequence
	c2 := NewRecord(map[string]SnapshotValue{"q": NewLiteral("string", "different")})
	t6, err := NewTermIdentity(a, 1, []string{"X"}, g, c2)
	if err != nil {
		t.Fatal(err)
	}
	if base.Equals(t6) {
		t.Fatal("different consequence should differ")
	}
}
