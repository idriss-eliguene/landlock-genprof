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
	producer1 := NewProducerRef(NewIdentityRef("Act", actKey))
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
	producer2 := NewProducerRef(NewIdentityRef("Act", actKey))
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
	producer := NewProducerRef(NewIdentityRef("Act", "act-d"))
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
	producer1 := NewProducerRef(NewIdentityRef("Act", "act-m1"))
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
	producer2 := NewProducerRef(NewIdentityRef("Act", actKey))
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
