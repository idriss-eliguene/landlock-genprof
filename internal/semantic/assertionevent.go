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
// Assertion Event. It is intentionally opaque here: Slice 2 does not
// implement Acts or Commitments. ProducerRef should be constructed by
// higher layers that implement Act/Commitment identity semantics.
// Use NewProducerRef to create one from an IdentityRef whose typeName
// distinguishes Act vs Commitment.
type ProducerRef struct {
	id IdentityRef
}

func NewProducerRef(id IdentityRef) ProducerRef { return ProducerRef{id: id} }

func (p ProducerRef) Identity() IdentityRef { return p.id }

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
	if !StructuralEqual(a.producer.id, b.producer.id) {
		return false
	}
	// Proposition identity is structural equality in this package.
	return StructuralEqual(a.proposition, b.proposition)
}
