package semantic

import (
	"testing"
	"time"
)

func mustRT(t *testing.T) RecordTime {
	rt, err := NewRecordTime(time.Now().UTC())
	if err != nil {
		t.Fatalf("NewRecordTime: %v", err)
	}
	return rt
}

func TestGroundSetBasic(t *testing.T) {
	ids := []AssertionEventIdentity{NewAssertionEventIdentity("e1"), NewAssertionEventIdentity("e2"), NewAssertionEventIdentity("e1")}
	gs := newGroundSetFromSlice(ids)
	members := gs.Members()
	if len(members) != 2 {
		t.Fatalf("expected 2 unique members, got %d", len(members))
	}
	// order-insensitive equality check
	gs2 := newGroundSetFromSlice([]AssertionEventIdentity{NewAssertionEventIdentity("e2"), NewAssertionEventIdentity("e1")})
	if !gs.equals(gs2) {
		t.Fatalf("groundSet equality failed: %+v vs %+v", gs.Members(), gs2.Members())
	}
}

func TestEnumerateDefeatAttacks(t *testing.T) {
	g := NewGraph()
	// create two assertion events X and Y
	propX := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "tX"), []SnapshotValue{NewLiteral("string", "x")}, NewValidTime(nil, nil), QuantThisInstance)
	rx, err := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a1")), propX, mustRT(t))
	if err != nil {
		t.Fatalf("create X: %v", err)
	}
	xid, err := g.AppendAssertionEvent(rx)
	if err != nil {
		t.Fatalf("append X: %v", err)
	}

	propY := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "tY"), []SnapshotValue{NewLiteral("string", "y")}, NewValidTime(nil, nil), QuantThisInstance)
	ry, err := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a2")), propY, mustRT(t))
	if err != nil {
		t.Fatalf("create Y: %v", err)
	}
	yid, err := g.AppendAssertionEvent(ry)
	if err != nil {
		t.Fatalf("append Y: %v", err)
	}

	// create defeat assertion R stating X defeats Y
	rprop := NewDefeatProposition(xid, yid, NewValidTime(nil, nil))
	r, err := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a3")), rprop, mustRT(t))
	if err != nil {
		t.Fatalf("create R: %v", err)
	}
	rid, err := g.AppendAssertionEvent(r)
	if err != nil {
		t.Fatalf("append R: %v", err)
	}

	attacks := g.EnumerateDefeatAttacks()
	if len(attacks) != 1 {
		t.Fatalf("expected 1 attack, got %d", len(attacks))
	}
	a := attacks[0]
	if a.attacker != xid {
		t.Fatalf("attacker mismatch: got %s want %s", a.attacker, xid)
	}
	if a.target != yid {
		t.Fatalf("target mismatch: got %s want %s", a.target, yid)
	}
	members := a.grounds.Members()
	if len(members) != 1 || members[0] != rid {
		t.Fatalf("ground set incorrect: %+v", members)
	}
	// regression: carrier r should not equal attacker xid unless user constructed so
	if a.attacker == AssertionEventIdentity(rid) {
		t.Fatalf("defeat carrier erroneously used as attacker: %s", a.attacker)
	}
}

func findDerivation(results []candidateDerivation, left, right IdentityRef, wantMembers []AssertionEventIdentity) *candidateDerivation {
	for i := range results {
		cd := &results[i]
		if cd.left.Token() == left.Token() && cd.right.Token() == right.Token() {
			// compare ground membership sets
			want := newGroundSetFromSlice(wantMembers)
			if cd.grounds.equals(want) {
				return cd
			}
		}
	}
	return nil
}

func TestCandidateIncompatibilityDerivations(t *testing.T) {
	g := NewGraph()
	// terms A,B,C,D as IdentityRef
	A := NewIdentityRef("Term", "A")
	B := NewIdentityRef("Term", "B")
	C := NewIdentityRef("Term", "C")
	D := NewIdentityRef("Term", "D")

	// create base incompatibility B ⟂ D (event inv)
	invProp := NewIncompatibilityProposition(B, D, NewValidTime(nil, nil))
	invEv, err := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "invAct")), invProp, mustRT(t))
	if err != nil {
		t.Fatalf("create inv: %v", err)
	}
	invid, err := g.AppendAssertionEvent(invEv)
	if err != nil {
		t.Fatalf("append inv: %v", err)
	}

	// create subsumption A ⊑ B (e1)
	s1 := NewSubsumptionProposition(A, B, NewValidTime(nil, nil))
	s1ev, err := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "s1")), s1, mustRT(t))
	if err != nil {
		t.Fatalf("create s1: %v", err)
	}
	s1id, err := g.AppendAssertionEvent(s1ev)
	if err != nil {
		t.Fatalf("append s1: %v", err)
	}

	// create alternative chain A ⊑ C, C ⊑ B to produce two derivations (A<-C<-B path)
	// s2: C ⊑ B
	s2 := NewSubsumptionProposition(C, B, NewValidTime(nil, nil))
	s2ev, err := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "s2")), s2, mustRT(t))
	if err != nil {
		t.Fatalf("create s2: %v", err)
	}
	s2id, err := g.AppendAssertionEvent(s2ev)
	if err != nil {
		t.Fatalf("append s2: %v", err)
	}
	// s3: A ⊑ C
	s3 := NewSubsumptionProposition(A, C, NewValidTime(nil, nil))
	s3ev, err := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "s3")), s3, mustRT(t))
	if err != nil {
		t.Fatalf("create s3: %v", err)
	}
	s3id, err := g.AppendAssertionEvent(s3ev)
	if err != nil {
		t.Fatalf("append s3: %v", err)
	}

	// enumerate candidate derivations
	results := g.EnumerateCandidateIncompatibilityDerivations()
	if len(results) == 0 {
		t.Fatalf("expected some derivations")
	}
	// expect base derivation (B,D) with ground {invid}
	if findDerivation(results, B, D, []AssertionEventIdentity{invid}) == nil {
		t.Fatalf("expected base derivation for (B,D) with ground inv")
	}
	// expect derivation (A,D) via s1 + inv -> ground {s1id,invid}
	if findDerivation(results, A, D, []AssertionEventIdentity{s1id, invid}) == nil {
		t.Fatalf("expected derivation (A,D) via s1 + inv")
	}
	// expect alternative derivation (A,D) via s3 + s2 + inv -> ground {s3id,s2id,invid}
	if findDerivation(results, A, D, []AssertionEventIdentity{s3id, s2id, invid}) == nil {
		t.Fatalf("expected alternative derivation (A,D) via s3+s2+inv")
	}
}

func TestSubsumptionCycleTerminates(t *testing.T) {
	g := NewGraph()
	// terms X,Y
	X := NewIdentityRef("Term", "X")
	Y := NewIdentityRef("Term", "Y")
	// create cycle X ⊑ Y and Y ⊑ X and incompatibility Y ⟂ Z
	Z := NewIdentityRef("Term", "Z")
	s1 := NewSubsumptionProposition(X, Y, NewValidTime(nil, nil))
	s1ev, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "c1")), s1, mustRT(t))
	_, _ = g.AppendAssertionEvent(s1ev)
	s2 := NewSubsumptionProposition(Y, X, NewValidTime(nil, nil))
	s2ev, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "c2")), s2, mustRT(t))
	_, _ = g.AppendAssertionEvent(s2ev)
	inv := NewIncompatibilityProposition(Y, Z, NewValidTime(nil, nil))
	invev, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "c3")), inv, mustRT(t))
	_, _ = g.AppendAssertionEvent(invev)

	results := g.EnumerateCandidateIncompatibilityDerivations()
	// should terminate and include derivations (Y,Z) and (X,Z) but finite
	foundY := false
	foundX := false
	for _, d := range results {
		if d.left.Token() == "Y" && d.right.Token() == "Z" {
			foundY = true
		}
		if d.left.Token() == "X" && d.right.Token() == "Z" {
			foundX = true
		}
	}
	if !foundY || !foundX {
		t.Fatalf("expected finite derivations for cycle; foundY=%v foundX=%v len=%d", foundY, foundX, len(results))
	}
}
