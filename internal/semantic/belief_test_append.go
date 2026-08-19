package semantic

import (
	"testing"
	"time"
)

func mustRTtForAppend(t *testing.T) RecordTime {
	rt, err := NewRecordTime(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestPhiUnresolvedProducer(t *testing.T) {
	g := NewGraph()
	// create assertion event with opaque producer identity (not an Act)
	p := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "X"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	ev, _ := NewAssertionEvent(NewProducerRefFromIdentityRef(NewIdentityRef("Act", "opaque-act")), p, mustRTtForAppend(t))
	_, _ = g.AppendAssertionEvent(ev)
	_, err := g.phi(BottomLabelling())
	if err == nil {
		t.Fatalf("expected phi to error on unresolved producer")
	}
}

func TestPhiMissingActUse(t *testing.T) {
	g := NewGraph()
	// create an Act that declares a use pointing to a missing AssertionEvent
	act := NewAct(NewSubjectIdentity("s1"), ActInference, NewValidTime(nil, nil), []AssertionEventIdentity{AssertionEventIdentity("missing-e")}, nil)
	err := g.AppendAct(act)
	if err != nil {
		t.Fatalf("append act: %v", err)
	}
	// create assertion event produced by that Act identity
	ai := act.Identity()
	evProp := NewProposition(NewIdentityRef("Phase", "p"), Actual, NewIdentityRef("Term", "Y"), []SnapshotValue{}, NewValidTime(nil, nil), QuantThisInstance)
	ev, _ := NewAssertionEvent(NewProducerRefFromAct(ai), evProp, mustRTtForAppend(t))
	_, _ = g.AppendAssertionEvent(ev)
	_, err = g.phi(BottomLabelling())
	if err == nil {
		t.Fatalf("expected phi to error when Act uses reference missing")
	}
}
