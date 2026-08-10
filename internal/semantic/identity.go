package semantic

import "errors"

var (
	ErrInvalidArity  = errors.New("invalid arity")
	ErrArityMismatch = errors.New("arity does not match argument sorts length")
)

// SubjectIdentity is an opaque semantic identity atom representing a recorded
// Subject/Authority. It is intentionally a distinct named type (backed by a
// string) to preserve type-safety; construction is via NewSubjectIdentity.
// It deliberately does not expose any mutable fields or persistence IDs.
type SubjectIdentity string

// AssertionEventIdentity is an opaque semantic identity atom for an
// Assertion Event. It is a typed alias of string with a constructor to
// avoid accidental mixing with other identity atoms.
type AssertionEventIdentity string

// NewSubjectIdentity constructs a SubjectIdentity from a stable recorded
// identity token. Callers should treat the input as an opaque recorded
// identity supplied by the minting system; this package does not parse it.
func NewSubjectIdentity(token string) SubjectIdentity {
	return SubjectIdentity(token)
}

// NewAssertionEventIdentity constructs an AssertionEventIdentity from an
// opaque recorded identity token.
func NewAssertionEventIdentity(token string) AssertionEventIdentity {
	return AssertionEventIdentity(token)
}

// TermIdentity is the immutable RFC-0001 canonical tuple:
// ⟨Minting Authority, Arity, Argument Sorts, Grounding Conditions, Consequence Conditions⟩
// Its fields are unexported slices/values; construction happens via NewTermIdentity
// and accessors return defensive copies.
type TermIdentity struct {
	authority SubjectIdentity
	arity     int
	// argumentSorts is a positional, ordered slice of sort names.
	argumentSorts []string
	// grounding and consequence snapshots are represented in the
	// semantic SnapshotValue domain (see snapshot.go).
	grounding   SnapshotValue
	consequence SnapshotValue
}

// NewTermIdentity constructs an immutable TermIdentity. It validates the
// obvious invariants (arity matches argumentSorts length) and makes
// defensive copies of input slices. It returns an error if construction
// invariants are violated.
func NewTermIdentity(authority SubjectIdentity, arity int, argumentSorts []string, grounding SnapshotValue, consequence SnapshotValue) (TermIdentity, error) {
	if arity < 0 {
		return TermIdentity{}, ErrInvalidArity
	}
	if len(argumentSorts) != arity {
		return TermIdentity{}, ErrArityMismatch
	}
	// Defensive copy of argumentSorts
	sorts := make([]string, len(argumentSorts))
	copy(sorts, argumentSorts)
	// defensive clone of snapshots to avoid caller mutation affecting stored
	// identity.
	var gClone, cClone SnapshotValue
	if grounding != nil {
		gClone = CloneSnapshot(grounding)
	}
	if consequence != nil {
		cClone = CloneSnapshot(consequence)
	}
	return TermIdentity{
		authority:     authority,
		arity:         arity,
		argumentSorts: sorts,
		grounding:     gClone,
		consequence:   cClone,
	}, nil
}

// Authority returns the SubjectIdentity of the Term. It is a value copy.
func (t TermIdentity) Authority() SubjectIdentity { return t.authority }

// Arity returns the Term's arity.
func (t TermIdentity) Arity() int { return t.arity }

// ArgumentSorts returns a defensive copy of the ordered argument sorts.
func (t TermIdentity) ArgumentSorts() []string {
	s := make([]string, len(t.argumentSorts))
	copy(s, t.argumentSorts)
	return s
}

// Grounding returns the grounding snapshot value (may be nil).
func (t TermIdentity) Grounding() SnapshotValue { return t.grounding }

// Consequence returns the consequence snapshot value (may be nil).
func (t TermIdentity) Consequence() SnapshotValue { return t.consequence }

// Equals reports whether two TermIdentity values are structurally equal
// according to RFC-0002: all tuple components must StructuralEqual (or
// simple equality for atoms/arity/argument sorts).
func (t TermIdentity) Equals(o TermIdentity) bool {
	if t.authority != o.authority {
		return false
	}
	if t.arity != o.arity {
		return false
	}
	// argument sorts: ordered comparison
	if len(t.argumentSorts) != len(o.argumentSorts) {
		return false
	}
	for i := range t.argumentSorts {
		if t.argumentSorts[i] != o.argumentSorts[i] {
			return false
		}
	}
	// grounding and consequence: use StructuralEqual
	if !StructuralEqual(t.grounding, o.grounding) {
		return false
	}
	if !StructuralEqual(t.consequence, o.consequence) {
		return false
	}
	return true
}
