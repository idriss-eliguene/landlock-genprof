package semantic

import (
	"testing"
	"time"
)

func TestBeliefStatusOrder(t *testing.T) {
	pairs := []struct {
		a, b beliefStatus
		want bool
	}{
		{_beliefUndecided, _beliefUndecided, true},
		{_beliefUndecided, _beliefIn, true},
		{_beliefUndecided, _beliefOut, true},
		{_beliefIn, _beliefIn, true},
		{_beliefOut, _beliefOut, true},
		{_beliefIn, _beliefOut, false},
		{_beliefOut, _beliefIn, false},
		{_beliefIn, _beliefUndecided, false},
		{_beliefOut, _beliefUndecided, false},
	}
	for _, p := range pairs {
		if statusLessOrEqual(p.a, p.b) != p.want {
			t.Fatalf("statusLessOrEqual(%v,%v) = %v want %v", p.a, p.b, !p.want, p.want)
		}
	}
}

func TestLabellingLookupEquals(t *testing.T) {
	L := NewLabellingFromMap(map[AssertionEventIdentity]beliefStatus{
		NewAssertionEventIdentity("e1"): _beliefIn,
	})
	if L.Lookup(NewAssertionEventIdentity("e1")) != _beliefIn {
		t.Fatalf("lookup")
	}
	if L.Lookup(NewAssertionEventIdentity("e2")) != _beliefUndecided {
		t.Fatalf("missing should be undecided")
	}
}

func TestAttackStatusCombinations(t *testing.T) {
	// create sample ids
	ae := NewAssertionEventIdentity("att")
	tid := NewAssertionEventIdentity("tgt")
	g1 := newGroundSetFromSlice([]AssertionEventIdentity{NewAssertionEventIdentity("g1")})
	A := attack{attacker: ae, target: tid, grounds: g1}

	Lmap := map[AssertionEventIdentity]beliefStatus{}
	// attacker in, ground in => active
	Lmap[ae] = _beliefIn
	Lmap[NewAssertionEventIdentity("g1")] = _beliefIn
	L := NewLabellingFromMap(Lmap)
	if attackStatus(A, L) != "active" {
		t.Fatalf("expected active")
	}

	// attacker out => void
	Lmap[ae] = _beliefOut
	L = NewLabellingFromMap(Lmap)
	if attackStatus(A, L) != "void" {
		t.Fatalf("expected void when attacker out")
	}

	// attacker undecided, ground in => potential
	Lmap[ae] = _beliefUndecided
	Lmap[NewAssertionEventIdentity("g1")] = _beliefIn
	L = NewLabellingFromMap(Lmap)
	if attackStatus(A, L) != "potential" {
		t.Fatalf("expected potential")
	}

	// attacker in, ground out => void
	Lmap[ae] = _beliefIn
	Lmap[NewAssertionEventIdentity("g1")] = _beliefOut
	L = NewLabellingFromMap(Lmap)
	if attackStatus(A, L) != "void" {
		t.Fatalf("expected void when ground out")
	}
}

func mustRTt(t *testing.T) RecordTime {
	rt, err := NewRecordTime(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestCandidateAttacksAndPhi(t *testing.T) {
	g := NewGraph()
	// Terms T1, T2
	T1 := NewIdentityRef("Term", "T1")
	T2 := NewIdentityRef("Term", "T2")
	// AE_a: proposition pa about T1
	pa := NewProposition(NewIdentityRef("Phase", "p"), Actual, T1, []SnapshotValue{NewLiteral("s", "a")}, NewValidTime(nil, nil), QuantThisInstance)
	ae, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "aa")), pa, mustRTt(t))
	aid, _ := g.AppendAssertionEvent(ae)
	// AE_b: proposition pb about T2
	pb := NewProposition(NewIdentityRef("Phase", "p"), Actual, T2, []SnapshotValue{NewLiteral("s", "b")}, NewValidTime(nil, nil), QuantThisInstance)
	be, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "bb")), pb, mustRTt(t))
	bid, _ := g.AppendAssertionEvent(be)

	// create incompatibility base: T1 incompatible with T2
	invProp := NewIncompatibilityProposition(T1, T2, NewValidTime(nil, nil))
	invEv, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "inv")), invProp, mustRTt(t))
	invId, _ := g.AppendAssertionEvent(invEv)

	// create candidate derivation should include (T1,T2) with grounds {invId}
	cds := g.EnumerateCandidateIncompatibilityDerivations()
	found := false
	for _, cd := range cds {
		if cd.left.Token() == "T1" && cd.right.Token() == "T2" {
			if cd.grounds.equals(newGroundSetFromSlice([]AssertionEventIdentity{invId})) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected candidate derivation for (T1,T2)")
	}

	// add Rebuttal assertion R: pa rebuts pb
	rprop := NewRebuttalProposition(pa, pb, NewValidTime(nil, nil))
	re, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "r")), rprop, mustRTt(t))
	_, _ = g.AppendAssertionEvent(re)

	// CandidateAttacks must include attack from aid -> bid grounded by invId
	attacks := g.CandidateAttacks()
	match := false
	for _, A := range attacks {
		if A.attacker == aid && A.target == bid {
			if A.grounds.equals(newGroundSetFromSlice([]AssertionEventIdentity{invId})) {
				match = true
				break
			}
		}
	}
	if !match {
		t.Fatalf("expected rebuttal attack between AE pairs")
	}

	// test phi: with attacker in and ground in, target should go out
	Lmap := map[AssertionEventIdentity]beliefStatus{}
	Lmap[aid] = _beliefIn
	Lmap[invId] = _beliefIn
	L := NewLabellingFromMap(Lmap)
	res := g.phi(L)
	if res.Lookup(bid) != _beliefOut {
		t.Fatalf("expected target out; got %v", res.Lookup(bid))
	}

	// if ground is out => attack void => target may be in if no other attacks and premises in
	Lmap[invId] = _beliefOut
	L = NewLabellingFromMap(Lmap)
	res2 := g.phi(L)
	// since there are zero premises, InCondition holds when all attacks void
	if res2.Lookup(bid) != _beliefIn {
		t.Fatalf("expected target in when ground out; got %v", res2.Lookup(bid))
	}
}

func TestPhiSimultaneity(t *testing.T) {
	g := NewGraph()
	// simple chain: u supports o; o defeats u (cycle)
	// create propositions
	pu := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "U"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	po := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "O"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	pa, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a1")), pu, mustRTt(t))
	ida, _ := g.AppendAssertionEvent(pa)
	pb, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a2")), po, mustRTt(t))
	idb, _ := g.AppendAssertionEvent(pb)
	// support: U subsumes O (treat as support via subsumption)
	sub := NewSubsumptionProposition(NewIdentityRef("Term", "U"), NewIdentityRef("Term", "O"), NewValidTime(nil, nil))
	sube, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "s1")), sub, mustRTt(t))
	_, _ = g.AppendAssertionEvent(sube)
	// defeat: O defeats U
	dp := NewDefeatProposition(idb, ida, NewValidTime(nil, nil))
	d, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "d1")), dp, mustRTt(t))
	_, _ = g.AppendAssertionEvent(d)

	// labelling: all undecided
	L := BottomLabelling()
	res := g.phi(L)
	// phi evaluates all events against input L (undecided): idb has no attacks and no premises => in; ida is attacked and remains undecided.
	if res.Lookup(ida) != _beliefUndecided || res.Lookup(idb) != _beliefIn {
		t.Fatalf("expected ida undecided and idb in; got %v and %v", res.Lookup(ida), res.Lookup(idb))
	}
}
