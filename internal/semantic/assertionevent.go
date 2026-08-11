package semantic

import "time"

// Modality per RFC-0001 §6.2 — finite domain
type Modality int

const (
	Actual Modality = iota
	Dispositional
	Deontic
)

// ActKind per RFC-0001 §6.1
type ActKind int

const (
	ActContact ActKind = iota
	ActInference
	ActTestimony
	ActAuthority
)

// RecordTime is the instant at which an Assertion Event entered the
// Graph. It is an immutable wrapper around time.Time.
type RecordTime struct {
	t time.Time
}

func NewRecordTime(t time.Time) (RecordTime, error) {
	if t.IsZero() {
		return RecordTime{}, ErrInvalidRecordTime
	}
	return RecordTime{t: t}, nil
}

func (r RecordTime) Time() time.Time { return r.t }

var ErrInvalidRecordTime = &SemanticError{"invalid record time"}

// SemanticError is a small error type for construction validation.
type SemanticError struct{ msg string }

func (e *SemanticError) Error() string { return e.msg }

// ProducerRef represents the Act or Commitment that produced an
// Assertion Event. It is a typed, variant-like holder: for Slice 3 we
// support Act producers via ActIdentity. Future Commitment identity kinds
// may be added without changing the structural comparison semantics.
// Prefer NewProducerRefFromAct when constructing events produced by an Act.
type ProducerRef struct {
	act *ActIdentity
	// fallback for opaque identity tokens (legacy); kept for compatibility
	opaque *IdentityRef
}

// NewProducerRefFromAct constructs a ProducerRef that names an Act by
// its ActIdentity (typed, structural).
func NewProducerRefFromAct(id ActIdentity) ProducerRef { a := id; return ProducerRef{act: &a} }

// NewProducerRefFromIdentityRef constructs a ProducerRef from an opaque
// IdentityRef. Use only when ActIdentity is not available.
func NewProducerRefFromIdentityRef(id IdentityRef) ProducerRef { return ProducerRef{opaque: &id} }

func (p ProducerRef) HasAct() bool { return p.act != nil }
func (p ProducerRef) ActIdentity() (ActIdentity, bool) {
	if p.act == nil {
		return ActIdentity{}, false
	}
	return *p.act, true
}
func (p ProducerRef) OpaqueIdentityRef() (IdentityRef, bool) {
	if p.opaque == nil {
		return IdentityRef{}, false
	}
	return *p.opaque, true
}

// AssertionEvent is the immutable core structure recording one committed
// Proposition produced by an Act or Commitment at a RecordTime.
type AssertionEvent struct {
	producer    ProducerRef
	proposition Proposition
	recordTime  RecordTime
}

// NewAssertionEvent constructs an AssertionEvent validating minimal
// structural invariants. It does not invent assertion identities; the
// producer identity is supplied by the caller (see ProducerRef).
func NewAssertionEvent(producer ProducerRef, prop Proposition, rt RecordTime) (AssertionEvent, error) {
	// validate proposition shape: must have a term and a phase (IdentityRefs)
	if prop.term.token == "" || prop.phase.token == "" {
		return AssertionEvent{}, &SemanticError{"proposition missing term or phase identity"}
	}
	// validate modality is within finite domain
	if prop.modality < Actual || prop.modality > Deontic {
		return AssertionEvent{}, &SemanticError{"invalid modality value"}
	}
	return AssertionEvent{producer: producer, proposition: CloneProposition(prop), recordTime: rt}, nil
}

// Producer returns the ProducerRef (value copy).
func (a AssertionEvent) Producer() ProducerRef { return a.producer }

// Proposition returns a defensive copy of the Proposition.
func (a AssertionEvent) Proposition() Proposition { return CloneProposition(a.proposition) }

// RecordTime returns the event's RecordTime.
func (a AssertionEvent) RecordTime() RecordTime { return a.recordTime }

// Equals reports identity equality per RFC-0001 §8.2.1/8.2.2: two
// Assertion Events are identical iff they were produced by the same
// Act-or-Commitment (producer identity equality) and state the same
// Proposition (structural equality).
func (a AssertionEvent) Equals(b AssertionEvent) bool {
	// compare producer identity: prefer typed ActIdentity when present
	if a.producer.HasAct() && b.producer.HasAct() {
		ai, _ := a.producer.ActIdentity()
		bi, _ := b.producer.ActIdentity()
		if !ai.Equals(bi) {
			return false
		}
	} else if ai, ok := a.producer.OpaqueIdentityRef(); ok {
		if bi, ok2 := b.producer.OpaqueIdentityRef(); ok2 {
			if !StructuralEqual(ai, bi) {
				return false
			}
		} else {
			// mismatched producer kinds => not equal
			return false
		}
	} else {
		// no producer identity present on one or both -> not equal
		return false
	}
	// Proposition identity is structural equality in this package.
	return StructuralEqual(a.proposition, b.proposition)
}
