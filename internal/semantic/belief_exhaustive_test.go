package semantic

import (
	"testing"
)

// helpers to build labellings enumeration
func enumerateLabellings(ids []AssertionEventIdentity) []Labelling {
	N := len(ids)
	if N == 0 {
		return []Labelling{BottomLabelling()}
	}
	res := []Labelling{}
	total := 1
	for i := 0; i < N; i++ {
		total *= 3
	}
	for v := 0; v < total; v++ {
		m := make(map[AssertionEventIdentity]beliefStatus)
		t := v
		for i := 0; i < N; i++ {
			r := t % 3
			t = t / 3
			id := ids[i]
			if r == 1 {
				m[id] = _beliefIn
			} else if r == 2 {
				m[id] = _beliefOut
			}
		}
		res = append(res, NewLabellingFromMap(m))
	}
	return res
}

// check monotonicity for every L1 <= L2 pair
func assertPhiMonotone(t *testing.T, g *Graph, ids []AssertionEventIdentity) {
	labs := enumerateLabellings(ids)
	countPairs := 0
	for i := 0; i < len(labs); i++ {
		for j := 0; j < len(labs); j++ {
			if LabellingLessOrEqual(labs[i], labs[j], ids) {
				countPairs++
				r1, err1 := g.phi(labs[i])
				if err1 != nil {
					t.Fatalf("phi error: %v", err1)
				}
				r2, err2 := g.phi(labs[j])
				if err2 != nil {
					t.Fatalf("phi error: %v", err2)
				}
				if !LabellingLessOrEqual(r1, r2, ids) {
					t.Fatalf("phi not monotone for pair on ids %v", ids)
				}
			}
		}
	}
	if countPairs == 0 {
		t.Fatalf("no comparable labelling pairs for ids %v", ids)
	}
}

// Fixture builders
func fixtureIsolatedEvent(t *testing.T) (*Graph, []AssertionEventIdentity) {
	g := NewGraph()
	act := NewAct(NewSubjectIdentity("sa"), ActContact, NewValidTime(nil, nil), nil, nil)
	if err := g.AppendAct(act); err != nil {
		t.Fatal(err)
	}
	p := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "E"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	ev, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), p, mustRTtForAppend(t))
	id, _ := g.AppendAssertionEvent(ev)
	return g, []AssertionEventIdentity{id}
}

func fixtureSingleDefeat(t *testing.T) (*Graph, []AssertionEventIdentity) {
	g := NewGraph()
	act1 := NewAct(NewSubjectIdentity("s1"), ActContact, NewValidTime(nil, nil), nil, nil)
	act2 := NewAct(NewSubjectIdentity("s2"), ActContact, NewValidTime(nil, nil), nil, nil)
	actD := NewAct(NewSubjectIdentity("sd"), ActContact, NewValidTime(nil, nil), nil, nil)
	for _, a := range []Act{act1, act2, actD} {
		if err := g.AppendAct(a); err != nil {
			t.Fatal(err)
		}
	}
	p1 := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "A"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	p2 := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "B"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	e1, _ := NewAssertionEvent(NewProducerRefFromAct(act1.Identity()), p1, mustRTtForAppend(t))
	e2, _ := NewAssertionEvent(NewProducerRefFromAct(act2.Identity()), p2, mustRTtForAppend(t))
	id1, _ := g.AppendAssertionEvent(e1)
	id2, _ := g.AppendAssertionEvent(e2)
	// d: e1 defeats e2
	dp := NewDefeatProposition(id1, id2, NewValidTime(nil, nil))
	d, _ := NewAssertionEvent(NewProducerRefFromAct(actD.Identity()), dp, mustRTtForAppend(t))
	did, _ := g.AppendAssertionEvent(d)
	return g, []AssertionEventIdentity{id1, id2, did}
}

func fixtureMutualDefeat(t *testing.T) (*Graph, []AssertionEventIdentity) {
	g := NewGraph()
	act1 := NewAct(NewSubjectIdentity("s1"), ActContact, NewValidTime(nil, nil), nil, nil)
	act2 := NewAct(NewSubjectIdentity("s2"), ActContact, NewValidTime(nil, nil), nil, nil)
	for _, a := range []Act{act1, act2} {
		if err := g.AppendAct(a); err != nil {
			t.Fatal(err)
		}
	}
	p1 := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "X"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	p2 := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "Y"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	e1, _ := NewAssertionEvent(NewProducerRefFromAct(act1.Identity()), p1, mustRTtForAppend(t))
	e2, _ := NewAssertionEvent(NewProducerRefFromAct(act2.Identity()), p2, mustRTtForAppend(t))
	id1, _ := g.AppendAssertionEvent(e1)
	id2, _ := g.AppendAssertionEvent(e2)
	// defeats both ways
	d1 := NewDefeatProposition(id1, id2, NewValidTime(nil, nil))
	d2 := NewDefeatProposition(id2, id1, NewValidTime(nil, nil))
	eD1, _ := NewAssertionEvent(NewProducerRefFromAct(act1.Identity()), d1, mustRTtForAppend(t))
	eD2, _ := NewAssertionEvent(NewProducerRefFromAct(act2.Identity()), d2, mustRTtForAppend(t))
	idD1, _ := g.AppendAssertionEvent(eD1)
	idD2, _ := g.AppendAssertionEvent(eD2)
	return g, []AssertionEventIdentity{id1, id2, idD1, idD2}
}

// Additional fixtures omitted for brevity: Act premise chain, support+defeat, multi-member groundset,
// multiple attacks on one target, rebuttal-derived attack. Implement a subset covering N<=3.

func fixtureActPremiseChain(t *testing.T) (*Graph, []AssertionEventIdentity) {
	g := NewGraph()
	// create Act A1 producing E1
	act1 := NewAct(NewSubjectIdentity("p-a1"), ActContact, NewValidTime(nil, nil), nil, nil)
	if err := g.AppendAct(act1); err != nil {
		t.Fatal(err)
	}
	p1 := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "E1"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	e1, _ := NewAssertionEvent(NewProducerRefFromAct(act1.Identity()), p1, mustRTtForAppend(t))
	id1, _ := g.AppendAssertionEvent(e1)

	// create Act A2 that uses E1 and produces E2
	act2 := NewAct(NewSubjectIdentity("p-a2"), ActContact, NewValidTime(nil, nil), []AssertionEventIdentity{id1}, nil)
	if err := g.AppendAct(act2); err != nil {
		t.Fatal(err)
	}
	p2 := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "E2"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	e2, _ := NewAssertionEvent(NewProducerRefFromAct(act2.Identity()), p2, mustRTtForAppend(t))
	id2, _ := g.AppendAssertionEvent(e2)
	return g, []AssertionEventIdentity{id1, id2}
}

func fixtureSupportPlusDefeat(t *testing.T) (*Graph, []AssertionEventIdentity) {
	g := NewGraph()
	// create acts
	actU := NewAct(NewSubjectIdentity("s-u"), ActContact, NewValidTime(nil, nil), nil, nil)
	actO := NewAct(NewSubjectIdentity("s-o"), ActContact, NewValidTime(nil, nil), nil, nil)
	actMakeO := NewAct(NewSubjectIdentity("s-make-o"), ActInference, NewValidTime(nil, nil), nil, nil)
	actD := NewAct(NewSubjectIdentity("s-d"), ActContact, NewValidTime(nil, nil), nil, nil)
	for _, a := range []Act{actU, actO, actMakeO, actD} {
		if err := g.AppendAct(a); err != nil {
			t.Fatal(err)
		}
	}
	// create U and O assertions: U produced by actU
	pu := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "U"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	eU, _ := NewAssertionEvent(NewProducerRefFromAct(actU.Identity()), pu, mustRTtForAppend(t))
	idU, _ := g.AppendAssertionEvent(eU)
	// create actMakeO that uses U and produces O: to model support
	actMakeO = NewAct(NewSubjectIdentity("s-make-o"), ActInference, NewValidTime(nil, nil), []AssertionEventIdentity{idU}, nil)
	if err := g.AppendAct(actMakeO); err != nil {
		t.Fatal(err)
	}
	po := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "O"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	eO, _ := NewAssertionEvent(NewProducerRefFromAct(actMakeO.Identity()), po, mustRTtForAppend(t))
	idO, _ := g.AppendAssertionEvent(eO)
	// create a Defeat where O defeats U (attack-based out condition)
	dp := NewDefeatProposition(idO, idU, NewValidTime(nil, nil))
	eD, _ := NewAssertionEvent(NewProducerRefFromAct(actD.Identity()), dp, mustRTtForAppend(t))
	idD, _ := g.AppendAssertionEvent(eD)
	return g, []AssertionEventIdentity{idU, idO, idD}
}

func fixtureMultiMemberGroundSet(t *testing.T) (*Graph, []AssertionEventIdentity) {
	g := NewGraph()
	act := NewAct(NewSubjectIdentity("g-act"), ActContact, NewValidTime(nil, nil), nil, nil)
	if err := g.AppendAct(act); err != nil {
		t.Fatal(err)
	}
	// base incompatibility inv between T1 and T2
	T1 := NewIdentityRef("Term", "T1")
	T2 := NewIdentityRef("Term", "T2")
	inv := NewIncompatibilityProposition(T1, T2, NewValidTime(nil, nil))
	invEv, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), inv, mustRTtForAppend(t))
	invId, _ := g.AppendAssertionEvent(invEv)
	// add a subsumption S: U ⊑ T1 to expand derivation and produce ground set {invId, sId}
	sub := NewSubsumptionProposition(NewIdentityRef("Term", "U"), T1, NewValidTime(nil, nil))
	sEv, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), sub, mustRTtForAppend(t))
	sId, _ := g.AppendAssertionEvent(sEv)
	// create attacker asserting proposition about U, and target about T2
	pa := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "U"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	ea, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pa, mustRTtForAppend(t))
	aid, _ := g.AppendAssertionEvent(ea)
	pb := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "T2"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	eb, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pb, mustRTtForAppend(t))
	bid, _ := g.AppendAssertionEvent(eb)
	// create Rebuttal prop pa rebuts pb
	rp := NewRebuttalProposition(pa, pb, NewValidTime(nil, nil))
	rev, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), rp, mustRTtForAppend(t))
	_, _ = g.AppendAssertionEvent(rev)
	// Now candidate derivation should include grounds {invId, sId}
	cds := g.EnumerateCandidateIncompatibilityDerivations()
	found := false
	for _, cd := range cds {
		if cd.left.Token() == "U" && cd.right.Token() == "T2" {
			if cd.grounds.equals(newGroundSetFromSlice([]AssertionEventIdentity{invId, sId})) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected multi-member candidate derivation")
	}
	return g, []AssertionEventIdentity{aid, bid, invId, sId}
}

func fixtureMultipleAttacksOneTarget(t *testing.T) (*Graph, []AssertionEventIdentity) {
	g := NewGraph()
	act := NewAct(NewSubjectIdentity("m-act"), ActContact, NewValidTime(nil, nil), nil, nil)
	if err := g.AppendAct(act); err != nil {
		t.Fatal(err)
	}
	// two attackers A1,A2 and target T
	pA1 := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "A1"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	eA1, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pA1, mustRTtForAppend(t))
	idA1, _ := g.AppendAssertionEvent(eA1)
	pA2 := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "A2"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	eA2, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pA2, mustRTtForAppend(t))
	idA2, _ := g.AppendAssertionEvent(eA2)
	pT := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "T"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	eT, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pT, mustRTtForAppend(t))
	idT, _ := g.AppendAssertionEvent(eT)
	// create defeats: A1 defeats T (d1), A2 defeats T (d2)
	d1 := NewDefeatProposition(idA1, idT, NewValidTime(nil, nil))
	eD1, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), d1, mustRTtForAppend(t))
	idD1, _ := g.AppendAssertionEvent(eD1)
	d2 := NewDefeatProposition(idA2, idT, NewValidTime(nil, nil))
	eD2, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), d2, mustRTtForAppend(t))
	idD2, _ := g.AppendAssertionEvent(eD2)
	return g, []AssertionEventIdentity{idA1, idA2, idT, idD1, idD2}
}

func fixtureRebuttalDerivedAttack(t *testing.T) (*Graph, []AssertionEventIdentity) {
	g := NewGraph()
	act := NewAct(NewSubjectIdentity("r-act"), ActContact, NewValidTime(nil, nil), nil, nil)
	if err := g.AppendAct(act); err != nil {
		t.Fatal(err)
	}
	// create terms and base incompatibility and subsumption to get a candidate derivation
	T1 := NewIdentityRef("Term", "TR1")
	T2 := NewIdentityRef("Term", "TR2")
	inv := NewIncompatibilityProposition(T1, T2, NewValidTime(nil, nil))
	invEv, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), inv, mustRTtForAppend(t))
	invId, _ := g.AppendAssertionEvent(invEv)
	// subsumption S: U ⊑ T1
	sub := NewSubsumptionProposition(NewIdentityRef("Term", "TU"), T1, NewValidTime(nil, nil))
	sEv, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), sub, mustRTtForAppend(t))
	sId, _ := g.AppendAssertionEvent(sEv)
	// attacker AE asserting proposition about TU (U), target AE about T2
	pa := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "TU"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	ea, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pa, mustRTtForAppend(t))
	aid, _ := g.AppendAssertionEvent(ea)
	pb := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "TR2"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	eb, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), pb, mustRTtForAppend(t))
	bid, _ := g.AppendAssertionEvent(eb)
	// Rebuttal assertion referencing pa and pb
	rp := NewRebuttalProposition(pa, pb, NewValidTime(nil, nil))
	rev, _ := NewAssertionEvent(NewProducerRefFromAct(act.Identity()), rp, mustRTtForAppend(t))
	_, _ = g.AppendAssertionEvent(rev)
	// ensure CandidateAttacks contains the attack (cd ground should include invId and sId)
	attacks := g.CandidateAttacks()
	found := false
	for _, A := range attacks {
		if A.attacker == aid && A.target == bid {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected rebuttal-derived attack in CandidateAttacks")
	}
	return g, []AssertionEventIdentity{aid, bid, invId, sId}
}

func TestExhaustiveN1N2N3_Monotonicity_Smoke(t *testing.T) {
	// use all 8 fixtures
	fixtures := []func(*testing.T) (*Graph, []AssertionEventIdentity){
		fixtureIsolatedEvent,
		fixtureSingleDefeat,
		fixtureMutualDefeat,
		fixtureActPremiseChain,
		fixtureSupportPlusDefeat,
		fixtureMultiMemberGroundSet,
		fixtureMultipleAttacksOneTarget,
		fixtureRebuttalDerivedAttack,
	}
	totalPairs := 0
	for _, f := range fixtures {
		g, ids := f(t)
		assertPhiMonotone(t, g, ids)
		// count comparable pairs
		labs := enumerateLabellings(ids)
		for i := 0; i < len(labs); i++ {
			for j := 0; j < len(labs); j++ {
				if LabellingLessOrEqual(labs[i], labs[j], ids) {
					totalPairs++
				}
			}
		}
	}
	if totalPairs == 0 {
		t.Fatalf("no comparable pairs checked")
	}
}
