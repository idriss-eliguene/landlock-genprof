package semantic

import "testing"

func TestRebuttalBucketCollisionFiltering(t *testing.T) {
	g := NewGraph()
	// create Act for producers
	act := NewAct(NewSubjectIdentity("s1"), ActContact, NewValidTime(nil, nil), nil, nil)
	if err := g.AppendAct(act); err != nil {
		t.Fatal(err)
	}
	// two different propositions
	pA := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "T1"), []SnapshotValue{NewLiteral("s", "a")}, NewValidTime(nil, nil), QuantThisInstance)
	pB := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "T2"), []SnapshotValue{NewLiteral("s", "b")}, NewValidTime(nil, nil), QuantThisInstance)
	// create two assertion events but force them into same index bucket using test seam
	eA, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pA, mustRTtForAppend(t))
	eB, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pB, mustRTtForAppend(t))
	key := "FORCED-COLLISION"
	idA, err := g.appendAssertionEventWithKey(eA, key)
	if err != nil {
		t.Fatal(err)
	}
	idB, err := g.appendAssertionEventWithKey(eB, key)
	if err != nil {
		t.Fatal(err)
	}
	// create incompatibility base for TA -> TB
	T1 := NewIdentityRef("Term", "T1")
	T2 := NewIdentityRef("Term", "T2")
	inv := NewIncompatibilityProposition(T1, T2, NewValidTime(nil, nil))
	invEv, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), inv, mustRTtForAppend(t))
	invId, _ := g.AppendAssertionEvent(invEv)
	// ensure candidate derivation exists for T1->T2
	_ = g.EnumerateCandidateIncompatibilityDerivations()
	// create a Rebuttal that refers to pA (only pA should match structurally)
	rprop := NewRebuttalProposition(pA, pB, NewValidTime(nil, nil))
	re, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), rprop, mustRTtForAppend(t))
	_, _ = g.AppendAssertionEvent(re)
	// CandidateAttacks should not pair idB as attacker for pA even though it sits in same bucket
	attacks := g.CandidateAttacks()
	foundA := false
	foundB := false
	for _, A := range attacks {
		if A.attacker == idA {
			foundA = true
		}
		if A.attacker == idB {
			foundB = true
		}
	}
	if !foundA {
		t.Fatalf("expected attack from structurally-matching attacker idA")
	}
	if foundB {
		t.Fatalf("did not expect attack from non-matching bucket member idB")
	}
	_ = invId
}
