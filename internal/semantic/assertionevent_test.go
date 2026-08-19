package semantic

import (
	"testing"
	"time"
)

func TestValidAssertionEventConstruction(t *testing.T) {
	phase := NewIdentityRef("Phase", "phase-1")
	term := NewIdentityRef("Term", "term-1")
	arg := NewLiteral("string", "value")
	prop := NewProposition(phase, Actual, term, []SnapshotValue{arg}, NewValidTime(nil, nil), QuantThisInstance)

	prodId := NewIdentityRef("Act", "act-1")
	producer := NewProducerRefFromIdentityRef(prodId)

	rt, _ := NewRecordTime(time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC))
	e, err := NewAssertionEvent(producer, prop, rt)
	if err != nil {
		t.Fatalf("unexpected error creating AssertionEvent: %v", err)
	}

	// producer identity preserved
	if oi, ok := e.Producer().OpaqueIdentityRef(); !ok || !StructuralEqual(oi, prodId) {
		t.Fatalf("producer identity not preserved")
	}

	// proposition preserved structurally
	p := e.Proposition()
	if !StructuralEqual(p, prop) {
		t.Fatalf("proposition not preserved structurally")
	}

	// record time preserved
	if !e.RecordTime().Time().Equal(rt.Time()) {
		t.Fatalf("record time mismatch")
	}
}

func TestMissingPropositionComponentsRejected(t *testing.T) {
	phase := NewIdentityRef("Phase", "")
	term := NewIdentityRef("Term", "")
	prop := NewProposition(phase, Actual, term, nil, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-x"))
	rt, _ := NewRecordTime(time.Date(2021, 2, 3, 4, 5, 6, 0, time.UTC))
	if _, err := NewAssertionEvent(producer, prop, rt); err == nil {
		t.Fatal("expected error for missing proposition identities")
	}
}

func TestModalityValidation(t *testing.T) {
	phase := NewIdentityRef("Phase", "p")
	term := NewIdentityRef("Term", "t")
	// invalid modality value
	prop := NewProposition(phase, Modality(999), term, nil, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-m"))
	rt, _ := NewRecordTime(time.Date(2022, 3, 4, 5, 6, 7, 0, time.UTC))
	if _, err := NewAssertionEvent(producer, prop, rt); err == nil {
		t.Fatal("expected error for invalid modality")
	}
}

func TestImmutabilityConstructorInput(t *testing.T) {
	phase := NewIdentityRef("Phase", "ph")
	term := NewIdentityRef("Term", "tr")
	arg := NewLiteral("string", "v1")
	args := []SnapshotValue{arg}
	prop := NewProposition(phase, Actual, term, args, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-immut"))
	rt, _ := NewRecordTime(time.Date(2020, 6, 7, 8, 9, 10, 0, time.UTC))
	e, err := NewAssertionEvent(producer, prop, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// mutate original args slice
	args[0] = NewLiteral("string", "mutated")

	p := e.Proposition()
	if !StructuralEqual(p.args[0], NewLiteral("string", "v1")) {
		t.Fatalf("constructor did not make defensive copy of args")
	}
}

func TestImmutabilityAccessorReturnsCopy(t *testing.T) {
	phase := NewIdentityRef("Phase", "ph2")
	term := NewIdentityRef("Term", "tr2")
	arg := NewLiteral("string", "vv")
	prop := NewProposition(phase, Actual, term, []SnapshotValue{arg}, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-acc"))
	rt, _ := NewRecordTime(time.Date(2019, 1, 1, 1, 1, 1, 0, time.UTC))
	e, err := NewAssertionEvent(producer, prop, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p1 := e.Proposition()
	// mutate returned proposition's args
	p1.args[0] = NewLiteral("string", "changed")
	p2 := e.Proposition()
	if !StructuralEqual(p2.args[0], arg) {
		t.Fatalf("accessor did not return defensive copy")
	}
}

func TestDifferentProducersWithSamePropositionAreDifferentEvents(t *testing.T) {
	phase := NewIdentityRef("Phase", "pp")
	term := NewIdentityRef("Term", "tt")
	arg := NewLiteral("string", "x")
	prop := NewProposition(phase, Actual, term, []SnapshotValue{arg}, NewValidTime(nil, nil), QuantThisInstance)

	p1 := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a1"))
	p2 := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "a2"))
	rt, _ := NewRecordTime(time.Date(2018, 7, 8, 9, 10, 11, 0, time.UTC))
	e1, _ := NewAssertionEvent(p1, prop, rt)
	e2, _ := NewAssertionEvent(p2, prop, rt)
	if e1.Equals(e2) {
		t.Fatalf("events with different producers should not be equal")
	}
	// propositions remain structurally equal
	if !StructuralEqual(e1.Proposition(), e2.Proposition()) {
		t.Fatalf("propositions should be structurally equal")
	}
}

func TestAssertionEventCanReferenceAnotherEvent(t *testing.T) {
	// create event E1 with explicit assertion identity token
	phase1 := NewIdentityRef("Phase", "ph-e1")
	term1 := NewIdentityRef("Term", "t-e1")
	prop1 := NewProposition(phase1, Actual, term1, nil, NewValidTime(nil, nil), QuantThisInstance)
	producer1 := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-e1"))
	rt1, _ := NewRecordTime(time.Date(2017, 1, 2, 3, 4, 5, 0, time.UTC))
	e1, _ := NewAssertionEvent(producer1, prop1, rt1)

	// represent a proposition in E2 that refers to E1 by an IdentityRef token
	refToE1 := NewIdentityRef("Assertion", "evt-e1")
	// no graph identity generation yet; this tests representability
	phase2 := NewIdentityRef("Phase", "ph-e2")
	term2 := NewIdentityRef("Term", "t-e2")
	prop2 := NewProposition(phase2, Actual, term2, []SnapshotValue{refToE1}, NewValidTime(nil, nil), QuantThisInstance)
	producer2 := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-e2"))
	rt2, _ := NewRecordTime(time.Date(2017, 6, 7, 8, 9, 10, 0, time.UTC))
	if _, err := NewAssertionEvent(producer2, prop2, rt2); err != nil {
		t.Fatalf("expected able to construct event referring to another event: %v", err)
	}

	// sanity: StructuralEqual compares IdentityRef atomically
	if !StructuralEqual(refToE1, refToE1) {
		t.Fatalf("identity ref equality failed")
	}
	_ = e1 // silence unused
}

func TestSelfReferenceRepresentability(t *testing.T) {
	phase := NewIdentityRef("Phase", "ph-self")
	term := NewIdentityRef("Term", "t-self")
	selfRef := NewIdentityRef("Assertion", "evt-self")
	prop := NewProposition(phase, Actual, term, []SnapshotValue{selfRef}, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-self"))
	rt, _ := NewRecordTime(time.Date(2016, 2, 3, 4, 5, 6, 0, time.UTC))
	if _, err := NewAssertionEvent(producer, prop, rt); err != nil {
		t.Fatalf("expected self-referential proposition representable: %v", err)
	}
}

func TestConstructorInputMutationDoesNotAffectEvent(t *testing.T) {
	phase := NewIdentityRef("Phase", "pm")
	term := NewIdentityRef("Term", "tm")
	arg := NewLiteral("string", "orig")
	args := []SnapshotValue{arg}
	prop := NewProposition(phase, Actual, term, args, NewValidTime(nil, nil), QuantThisInstance)
	producer := NewProducerRefFromIdentityRef(NewIdentityRef("Act", "act-m2"))
	rt, _ := NewRecordTime(time.Date(2015, 3, 4, 5, 6, 7, 0, time.UTC))
	e, err := NewAssertionEvent(producer, prop, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// mutate inputs
	args[0] = NewLiteral("string", "changed")
	prop.args[0] = NewLiteral("string", "changed2")

	p := e.Proposition()
	if !StructuralEqual(p.args[0], arg) {
		t.Fatalf("event mutated after constructor input changed")
	}
}
