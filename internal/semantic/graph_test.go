package semantic

import (
	"testing"
	"time"
)

func TestGraphAppendAndLookup(t *testing.T) {
	g := NewGraph()
	// create Act A2 that uses E1 and may produce E2
	uses := []AssertionEventIdentity{}
	act := NewAct(NewSubjectIdentity("src-1"), ActInference, NewValidTime(nil, nil), uses, nil)
	actKey := actCanonicalKey(act)
	// create event E1 whose producer token references the actKey
	phase1 := NewIdentityRef("Phase", "ph-e1")
	term1 := NewIdentityRef("Term", "t-e1")
	prop1 := NewProposition(phase1, Actual, term1, nil, NewValidTime(nil, nil), QuantThisInstance)
	producer1 := NewProducerRefFromIdentityRef(NewIdentityRef("Act", actKey))
	rt1, _ := NewRecordTime(time.Date(2017, 1, 2, 3, 4, 5, 0, time.UTC))
	e1, _ := NewAssertionEvent(producer1, prop1, rt1)
	id1, err := g.AppendAssertionEvent(e1)
	if err != nil {
		t.Fatalf("append e1 failed: %v", err)
	}
	// append act; existing event should be discovered as output
	if err := g.AppendAct(act); err != nil {
		t.Fatalf("append act failed: %v", err)
	}
	// create E2 produced by act (same producer token)
	producer2 := NewProducerRefFromIdentityRef(NewIdentityRef("Act", actKey))
	phase2 := NewIdentityRef("Phase", "ph-e2")
	term2 := NewIdentityRef("Term", "t-e2")
	prop2 := NewProposition(phase2, Actual, term2, nil, NewValidTime(nil, nil), QuantThisInstance)
	rt2, _ := NewRecordTime(time.Date(2017, 1, 2, 4, 4, 5, 0, time.UTC))
	e2, _ := NewAssertionEvent(producer2, prop2, rt2)
	id2, err := g.AppendAssertionEvent(e2)
	if err != nil {
		t.Fatalf("append e2 failed: %v", err)
	}
	// verify lookup
	if _, ok := g.GetAssertionEvent(id1); !ok {
		t.Fatalf("e1 not found")
	}
	if _, ok := g.GetAssertionEvent(id2); !ok {
		t.Fatalf("e2 not found")
	}
}

func TestAppendDuplicateBehavior(t *testing.T) {
	g := NewGraph()
	phase := NewIdentityRef("Phase", "phd")
	term := NewIdentityRef("Term", "td")
	prop := NewProposition(phase, Actual, term, nil, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-d"))
	rt, _ := NewRecordTime(time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC))
	e, _ := NewAssertionEvent(producer, prop, rt)
	id, err := g.AppendAssertionEvent(e)
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}
	// append identical event again: idempotent success
	id2, err := g.AppendAssertionEvent(e)
	if err != nil {
		t.Fatalf("second append failed: %v", err)
	}
	if id != id2 {
		t.Fatalf("ids differ on idempotent append")
	}
	// attempt to append conflicting event with same canonical identity key but different RecordTime
	rt2, _ := NewRecordTime(time.Date(2019, 2, 2, 0, 0, 0, 0, time.UTC))
	eMut, _ := NewAssertionEvent(producer, prop, rt2)
	if _, err := g.AppendAssertionEvent(eMut); err == nil {
		t.Fatalf("expected record time conflict error when same identity presented with different RecordTime")
	}
}

func TestActUsesSetSemantics(t *testing.T) {
	// create uses with duplicates and ensure normalization
	u := AssertionEventIdentity("e1")
	u2 := AssertionEventIdentity("e1")
	act := NewAct(NewSubjectIdentity("s"), ActContact, NewValidTime(nil, nil), []AssertionEventIdentity{u, u2, AssertionEventIdentity("e2")}, nil)
	uses := act.Uses()
	if len(uses) != 2 {
		t.Fatalf("expected deduplicated uses length 2, got %d", len(uses))
	}
}

func TestAppendActDuplicateConflict(t *testing.T) {
	g := NewGraph()
	act1 := NewAct(NewSubjectIdentity("s1"), ActContact, NewValidTime(nil, nil), nil, nil)
	if err := g.AppendAct(act1); err != nil {
		t.Fatalf("append act1 failed: %v", err)
	}
	// create conflicting act with same identity key but different content: usesPresent true vs false will differ identity
	act2 := NewAct(NewSubjectIdentity("s1"), ActContact, NewValidTime(nil, nil), []AssertionEventIdentity{AssertionEventIdentity("x")}, nil)
	if err := g.AppendAct(act2); err != nil {
		t.Fatalf("expected append to succeed for distinct act identity: %v", err)
	}
}

func TestMixedHistoricalExample(t *testing.T) {
	g := NewGraph()
	// E1
	phase1 := NewIdentityRef("Phase", "ph-m1")
	term1 := NewIdentityRef("Term", "t-m1")
	prop1 := NewProposition(phase1, Actual, term1, nil, NewValidTime(nil, nil), QuantThisInstance)
	producer1 := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-m1"))
	rt1, _ := NewRecordTime(time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC))
	e1, _ := NewAssertionEvent(producer1, prop1, rt1)
	id1, _ := g.AppendAssertionEvent(e1)
	// A2 uses E1 and outputs E2
	act := NewAct(NewSubjectIdentity("s-m"), ActInference, NewValidTime(nil, nil), []AssertionEventIdentity{id1}, nil)
	if err := g.AppendAct(act); err != nil {
		t.Fatalf("append act failed: %v", err)
	}
	// E2 (producer token equal to actKey)
	actKey := actCanonicalKey(act)
	producer2 := NewProducerRefFromIdentityRef(NewIdentityRef("Act", actKey))
	prop2 := NewProposition(NewIdentityRef("Phase", "ph-m2"), Actual, NewIdentityRef("Term", "t-m2"), nil, NewValidTime(nil, nil), QuantThisInstance)
	rt2, _ := NewRecordTime(time.Date(2010, 1, 1, 0, 1, 0, 0, time.UTC))
	e2, _ := NewAssertionEvent(producer2, prop2, rt2)
	id2, _ := g.AppendAssertionEvent(e2)
	// verify relationships
	gotE1, ok := g.GetAssertionEvent(id1)
	if !ok {
		t.Fatalf("e1 missing")
	}
	_ = gotE1
	gotE2, ok := g.GetAssertionEvent(id2)
	if !ok {
		t.Fatalf("e2 missing")
	}
	_ = gotE2
}

func TestDefeatAssertionConstructionAndQuery(t *testing.T) {
	g := NewGraph()
	// create base events E1 (attacker) and E2 (target)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a-def"))
	p1 := NewProposition(NewIdentityRef("Phase", "ph"), Actual, NewIdentityRef("Term", "t1"), nil, NewValidTime(nil, nil), QuantThisInstance)
	rt, _ := NewRecordTime(time.Now().UTC())
	e1, _ := NewAssertionEvent(producer, p1, rt)
	id1, _ := g.AppendAssertionEvent(e1)
	p2 := NewProposition(NewIdentityRef("Phase", "ph"), Actual, NewIdentityRef("Term", "t2"), nil, NewValidTime(nil, nil), QuantThisInstance)
	e2, _ := NewAssertionEvent(producer, p2, rt)
	id2, _ := g.AppendAssertionEvent(e2)
	// create defeat proposition and assertion
	defProp := NewDefeatProposition(id1, id2, NewValidTime(nil, nil))
	defEvt, _ := NewAssertionEvent(producer, defProp, rt)
	did, err := g.AppendAssertionEvent(defEvt)
	if err != nil {
		t.Fatalf("append defeat failed: %v", err)
	}
	// query by target and attacker
	byT := g.DefeatAssertionsByTarget(id2)
	if len(byT) != 1 || byT[0] != did {
		t.Fatalf("defeat by target query failed")
	}
	byA := g.DefeatAssertionsByAttacker(id1)
	if len(byA) != 1 || byA[0] != did {
		t.Fatalf("defeat by attacker query failed")
	}
}

func TestRebuttalSubsumptionIncompatibilityConstructionAndQuery(t *testing.T) {
	g := NewGraph()
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a-rel"))
	rt, _ := NewRecordTime(time.Now().UTC())
	// propositions for rebuttal
	pA := NewProposition(NewIdentityRef("Phase", "ph"), Actual, NewIdentityRef("Term", "ta"), nil, NewValidTime(nil, nil), QuantThisInstance)
	pB := NewProposition(NewIdentityRef("Phase", "ph"), Actual, NewIdentityRef("Term", "tb"), nil, NewValidTime(nil, nil), QuantThisInstance)
	reb := NewRebuttalProposition(pA, pB, NewValidTime(nil, nil))
	rebEv, _ := NewAssertionEvent(producer, reb, rt)
	rebId, err := g.AppendAssertionEvent(rebEv)
	if err != nil {
		t.Fatalf("append rebuttal failed: %v", err)
	}
	// query by proposition
	res := g.RebuttalAssertionsByProposition(pA)
	if len(res) != 1 || res[0] != rebId {
		t.Fatalf("rebuttal query failed")
	}
	// subsumption and incompatibility by terms
	tA := NewIdentityRef("Term", "ta")
	tB := NewIdentityRef("Term", "tb")
	sub := NewSubsumptionProposition(tA, tB, NewValidTime(nil, nil))
	subEv, _ := NewAssertionEvent(producer, sub, rt)
	subId, err := g.AppendAssertionEvent(subEv)
	if err != nil {
		t.Fatalf("append subsumption failed: %v", err)
	}
	resSub := g.SubsumptionAssertionsByTerm(tA)
	if len(resSub) == 0 {
		t.Fatalf("subsumption query failed")
	}
	// incompatibility
	inc := NewIncompatibilityProposition(tA, tB, NewValidTime(nil, nil))
	incEv, _ := NewAssertionEvent(producer, inc, rt)
	incId, err := g.AppendAssertionEvent(incEv)
	if err != nil {
		t.Fatalf("append incompatibility failed: %v", err)
	}
	resInc := g.IncompatibilityAssertionsByTerm(tB)
	if len(resInc) == 0 {
		t.Fatalf("incompatibility query failed")
	}
	_ = subId
	_ = incId
}

func TestRelationIndexesAreRebuildable(t *testing.T) {
	g := NewGraph()
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a-reb"))
	rt, _ := NewRecordTime(time.Now().UTC())
	// create a rebuttal asserted
	pA := NewProposition(NewIdentityRef("Phase", "ph"), Actual, NewIdentityRef("Term", "tR1"), nil, NewValidTime(nil, nil), QuantThisInstance)
	pB := NewProposition(NewIdentityRef("Phase", "ph"), Actual, NewIdentityRef("Term", "tR2"), nil, NewValidTime(nil, nil), QuantThisInstance)
	reb := NewRebuttalProposition(pA, pB, NewValidTime(nil, nil))
	rebEv, _ := NewAssertionEvent(producer, reb, rt)
	rebId, _ := g.AppendAssertionEvent(rebEv)
	// take copies of indexes
	orig := g.RebuttalAssertionsByProposition(pA)
	// rebuild
	g.RebuildRelationIndexes()
	after := g.RebuttalAssertionsByProposition(pA)
	if len(orig) != len(after) || (len(orig) > 0 && orig[0] != after[0]) {
		t.Fatalf("rebuild mismatch")
	}
	_ = rebId
}

func TestRawSubsumptionNotTransitiveOrReflexive(t *testing.T) {
	g := NewGraph()
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a-sub"))
	rt, _ := NewRecordTime(time.Now().UTC())
	// A ⊑ B and B ⊑ C but no A ⊑ C asserted
	a := NewIdentityRef("Term", "A")
	b := NewIdentityRef("Term", "B")
	c := NewIdentityRef("Term", "C")
	s1 := NewSubsumptionProposition(a, b, NewValidTime(nil, nil))
	s2 := NewSubsumptionProposition(b, c, NewValidTime(nil, nil))
	s1Ev, _ := NewAssertionEvent(producer, s1, rt)
	s2Ev, _ := NewAssertionEvent(producer, s2, rt)
	g.AppendAssertionEvent(s1Ev)
	g.AppendAssertionEvent(s2Ev)
	res := g.SubsumptionAssertionsByTerm(a)
	// res should only include directly asserted subsumptions; no transitive A⊑C
	if len(res) == 0 {
		t.Fatalf("expected direct subsumption present")
	}
	// ensure reflexive A⊑A not present unless asserted
	resSelf := g.SubsumptionAssertionsByTerm(a)
	foundSelf := false
	for _, id := range resSelf {
		p := g.assertions[id].Proposition()
		if pr, ok := p.args[0].(IdentityRef); ok {
			if pr.Token() == a.Token() {
				// check second arg
				if pr2, ok2 := p.args[1].(IdentityRef); ok2 && pr2.Token() == a.Token() {
					foundSelf = true
				}
			}
		}
	}
	if foundSelf {
		t.Fatalf("unexpected reflexive subsumption present")
	}
}

func TestRelationAssertionsPreserveProvenanceAndRecordTime(t *testing.T) {
	g := NewGraph()
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a-prov"))
	rt, _ := NewRecordTime(time.Now().UTC())
	// create defeat assertion and verify producer and recordtime preserved
	p1 := NewProposition(NewIdentityRef("Phase", "ph"), Actual, NewIdentityRef("Term", "tx"), nil, NewValidTime(nil, nil), QuantThisInstance)
	e1, _ := NewAssertionEvent(producer, p1, rt)
	id1, _ := g.AppendAssertionEvent(e1)
	def := NewDefeatProposition(id1, id1, NewValidTime(nil, nil))
	defEv, _ := NewAssertionEvent(producer, def, rt)
	did, _ := g.AppendAssertionEvent(defEv)
	got := g.assertions[did]
	// provenance
	if oi, ok := got.Producer().OpaqueIdentityRef(); !ok || oi.Token() != "a-prov" {
		t.Fatalf("producer not preserved")
	}
	if !got.RecordTime().Time().Equal(rt.Time()) {
		t.Fatalf("record time not preserved")
	}
}

// Hostile collision tests: index-key and handle collision seams.
func TestAssertionIndexKeyCollisionDoesNotMergeDistinctEvents(t *testing.T) {
	g := NewGraph()
	// two structurally distinct events but force same internal index key
	phase := NewIdentityRef("Phase", "p-c1")
	term := NewIdentityRef("Term", "t-c1")
	prop1 := NewProposition(phase, Actual, term, []SnapshotValue{NewLiteral("string", "v1")}, NewValidTime(nil, nil), QuantThisInstance)
	prop2 := NewProposition(phase, Actual, term, []SnapshotValue{NewLiteral("string", "v2")}, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-c"))
	rt, _ := NewRecordTime(time.Now().UTC())
	e1, _ := NewAssertionEvent(producer, prop1, rt)
	e2, _ := NewAssertionEvent(producer, prop2, rt)
	// force same key
	forcedKey := "hostile:collision"
	id1, err := g.appendAssertionEventWithKey(e1, forcedKey)
	if err != nil {
		t.Fatalf("append e1 failed: %v", err)
	}
	id2, err := g.appendAssertionEventWithKey(e2, forcedKey)
	if err != nil {
		t.Fatalf("append e2 failed: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected distinct handles for distinct events, got same: %s", id1)
	}
	// both retrievable
	if _, ok := g.GetAssertionEvent(id1); !ok {
		t.Fatalf("e1 not retrievable")
	}
	if _, ok := g.GetAssertionEvent(id2); !ok {
		t.Fatalf("e2 not retrievable")
	}
	// structural equality reports false
	if g.assertions[id1].Equals(g.assertions[id2]) {
		t.Fatalf("distinct events reported equal")
	}
	// bucket length == 2
	bucket := g._assertionIndex[forcedKey]
	if len(bucket) != 2 {
		t.Fatalf("expected bucket len 2, got %d", len(bucket))
	}
}

func TestPublicHandleAllocatorHandlesCollision(t *testing.T) {
	g := NewGraph()
	phase := NewIdentityRef("Phase", "p-h1")
	term := NewIdentityRef("Term", "t-h1")
	prop1 := NewProposition(phase, Actual, term, []SnapshotValue{NewLiteral("string", "a")}, NewValidTime(nil, nil), QuantThisInstance)
	prop2 := NewProposition(phase, Actual, term, []SnapshotValue{NewLiteral("string", "b")}, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-h"))
	rt, _ := NewRecordTime(time.Now().UTC())
	e1, _ := NewAssertionEvent(producer, prop1, rt)
	e2, _ := NewAssertionEvent(producer, prop2, rt)
	forcedKey := "handle:collision"
	id1, err := g.appendAssertionEventWithKey(e1, forcedKey)
	if err != nil {
		t.Fatalf("append e1 failed: %v", err)
	}
	// Append second event with same forced key: allocator should detect id1 exists and allocate different id
	id2, err := g.appendAssertionEventWithKey(e2, forcedKey)
	if err != nil {
		t.Fatalf("append e2 failed: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected allocator to choose distinct id when initial candidate collides")
	}
	// both present
	if _, ok := g.GetAssertionEvent(id1); !ok {
		t.Fatalf("id1 missing")
	}
	if _, ok := g.GetAssertionEvent(id2); !ok {
		t.Fatalf("id2 missing")
	}
}

func TestIdempotentAppendUnderForcedBucket(t *testing.T) {
	g := NewGraph()
	phase := NewIdentityRef("Phase", "p-id1")
	term := NewIdentityRef("Term", "t-id1")
	prop := NewProposition(phase, Actual, term, []SnapshotValue{NewLiteral("string", "id")}, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-id"))
	rt, _ := NewRecordTime(time.Now().UTC())
	e, _ := NewAssertionEvent(producer, prop, rt)
	forcedKey := "idempotent:bucket"
	id1, err := g.appendAssertionEventWithKey(e, forcedKey)
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}
	id2, err := g.appendAssertionEventWithKey(e, forcedKey)
	if err != nil {
		t.Fatalf("second append failed: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id for idempotent append")
	}
	// now attempt with differing RecordTime
	rt2, _ := NewRecordTime(time.Now().Add(time.Hour))
	eMut, _ := NewAssertionEvent(producer, prop, rt2)
	if _, err := g.appendAssertionEventWithKey(eMut, forcedKey); err == nil {
		t.Fatalf("expected ErrRecordTimeConflict when same identity with different RecordTime")
	}
}
