package semantic

import (
	"sort"
	"time"
)

// SnapshotValue is the abstract domain type used by TermIdentity's
// grounding/consequence fields. Concrete implementations are provided in
// this package: Literal, IdentityRef, Tuple, Set, Record, Proposition.
// Implementations must be immutable by convention: constructor functions
// should make defensive copies and callers must not assume mutability.
type SnapshotValue interface {
	isSnapshot()
}

// Literal represents a primitive literal (string, number, boolean).
// For simplicity this implementation treats literals as strings with an
// explicit kind marker; extensions can add typed numbers/booleans.
type Literal struct {
	kind  string
	value string
}

// NewLiteral constructs a Literal of given kind (e.g. "string", "int",
// "bool") and value. Caller must provide canonical string form for
// non-string kinds.
func NewLiteral(kind, value string) Literal {
	return Literal{kind: kind, value: value}
}

func (Literal) isSnapshot() {}

// IdentityRef is an atomic reference to a recorded identity (Subject,
// Term, or Assertion Event). The semantics: identity comparisons treat
// the referenced token atomically by recorded identity equality.
// Use NewIdentityRef to construct.
type IdentityRef struct {
	token    string
	typeName string // optional human sort like "Subject"/"Term"/"Assertion"
}

func NewIdentityRef(typeName, token string) IdentityRef {
	return IdentityRef{token: token, typeName: typeName}
}

func (i IdentityRef) Token() string    { return i.token }
func (i IdentityRef) TypeName() string { return i.typeName }

func (IdentityRef) isSnapshot() {}

// Tuple is an ordered sequence of SnapshotValues (positional).
// Construction makes defensive copies.
type Tuple struct {
	elems []SnapshotValue
}

func NewTuple(elems []SnapshotValue) Tuple {
	c := make([]SnapshotValue, len(elems))
	copy(c, elems)
	return Tuple{elems: c}
}

func (Tuple) isSnapshot() {}

// Set is a mathematical unordered set of SnapshotValues. Construction
// normalizes duplicates using StructuralEqual semantics.
type Set struct {
	members []SnapshotValue // normalized, uniqueness guaranteed
}

// NewSet constructs a Set from input members, normalizing duplicates.
func NewSet(members []SnapshotValue) Set {
	normalized := make([]SnapshotValue, 0, len(members))
	for _, m := range members {
		found := false
		for _, n := range normalized {
			if StructuralEqual(m, n) {
				found = true
				break
			}
		}
		if !found {
			normalized = append(normalized, m)
		}
	}
	return Set{members: normalized}
}

func (Set) isSnapshot() {}

// Record represents a map from string field names to SnapshotValues.
// Field order is not significant; construction must make a defensive copy
// and preserve an internal deterministic ordering for equality checks.
type Record struct {
	fields map[string]SnapshotValue
	// cached ordered keys for deterministic equality comparisons
	keys []string
}

// NewRecord constructs a Record. It makes defensive copies of the
// provided map and computes a stable ordering of keys.
func NewRecord(m map[string]SnapshotValue) Record {
	flds := make(map[string]SnapshotValue, len(m))
	keys := make([]string, 0, len(m))
	for k, v := range m {
		flds[k] = v
		keys = append(keys, k)
	}
	// stable ordering of keys: simple lexical sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return Record{fields: flds, keys: keys}
}

// CloneSnapshot produces a deep copy of a SnapshotValue so callers can
// take defensive copies before storing values in immutable structs.
func CloneSnapshot(s SnapshotValue) SnapshotValue {
	if s == nil {
		return nil
	}
	switch v := s.(type) {
	case Literal:
		return NewLiteral(v.kind, v.value)
	case IdentityRef:
		return NewIdentityRef(v.typeName, v.token)
	case Tuple:
		copyElems := make([]SnapshotValue, len(v.elems))
		for i := range v.elems {
			copyElems[i] = CloneSnapshot(v.elems[i])
		}
		return NewTuple(copyElems)
	case Set:
		copyMembers := make([]SnapshotValue, len(v.members))
		for i := range v.members {
			copyMembers[i] = CloneSnapshot(v.members[i])
		}
		return NewSet(copyMembers)
	case Record:
		m := make(map[string]SnapshotValue, len(v.fields))
		for k, val := range v.fields {
			m[k] = CloneSnapshot(val)
		}
		return NewRecord(m)
	case Proposition:
		args := make([]SnapshotValue, len(v.args))
		for i := range v.args {
			args[i] = CloneSnapshot(v.args[i])
		}
		return NewProposition(v.phase, v.modality, v.term, args, NewValidTime(v.validTime.Start, v.validTime.End), v.quantification)
	default:
		return nil
	}
}

func (Record) isSnapshot() {}

// CanonicalString produces a deterministic, canonical string representation
// of a SnapshotValue for internal identity token construction. It is not a
// serialization for external use; it is used solely to derive stable
// tokens within the in-memory Graph. It respects set semantics by sorting
// set members' canonical forms.
func CanonicalString(s SnapshotValue) string {
	if s == nil {
		return "<nil>"
	}
	switch v := s.(type) {
	case Literal:
		return "L{" + v.kind + "}" + v.value
	case IdentityRef:
		return "I{" + v.typeName + "}" + v.token
	case Tuple:
		parts := make([]string, len(v.elems))
		for i := range v.elems {
			parts[i] = CanonicalString(v.elems[i])
		}
		return "T(" + join(parts, ",") + ")"
	case Set:
		parts := make([]string, len(v.members))
		for i := range v.members {
			parts[i] = CanonicalString(v.members[i])
		}
		// sort to ensure order-independence
		sort.Strings(parts)
		return "S{" + join(parts, ",") + "}"
	case Record:
		parts := make([]string, len(v.keys))
		for i := range v.keys {
			k := v.keys[i]
			parts[i] = k + ":" + CanonicalString(v.fields[k])
		}
		return "R{" + join(parts, ",") + "}"
	case Proposition:
		// Proposition canonical form uses phase, modality numeric, term, args, validTime, quantification
		args := make([]string, len(v.args))
		for i := range v.args {
			args[i] = CanonicalString(v.args[i])
		}
		vt := "vt["
		if v.validTime.Start != nil {
			vt += v.validTime.Start.UTC().Format(time.RFC3339Nano)
		}
		vt += ":"
		if v.validTime.End != nil {
			vt += v.validTime.End.UTC().Format(time.RFC3339Nano)
		}
		vt += "]"
		return "P{" + CanonicalString(v.phase) + ";m=" + string(rune(v.modality+'0')) + ";t=" + CanonicalString(v.term) + ";a=" + join(args, ",") + ";" + vt + ";q=" + string(rune(v.quantification+'0')) + "}"
	default:
		return "<unknown>"
	}
}

// join is a tiny helper to avoid importing strings for trivial use.
func join(parts []string, sep string) string {
	res := ""
	for i := range parts {
		if i != 0 {
			res += sep
		}
		res += parts[i]
	}
	return res
}

// Quantification indicates whether a Proposition quantifies over this
// instance or over all instances of a Phase.
type Quantification int

const (
	QuantThisInstance Quantification = iota
	QuantAllInstances
)

// ValidTime is a half-open interval [Start, End). Nil Start/End represent
// unbounded ends.
type ValidTime struct {
	Start *time.Time
	End   *time.Time
}

func NewValidTime(start, end *time.Time) ValidTime {
	return ValidTime{Start: start, End: end}
}

// Proposition represents a structured Proposition per RFC-0001 §8.1.1.
// Identity components: Phase, Modality, Term, Arguments, ValidTime,
// Quantification. Term and Phase are represented as IdentityRef so they
// remain atomic for StructuralEqual.
type Proposition struct {
	phase          IdentityRef
	modality       Modality
	term           IdentityRef
	args           []SnapshotValue // positional
	validTime      ValidTime
	quantification Quantification
}

// NewProposition constructs a Proposition. It makes defensive copies of
// positional args.
func NewProposition(phase IdentityRef, modality Modality, term IdentityRef, args []SnapshotValue, validTime ValidTime, quant Quantification) Proposition {
	c := make([]SnapshotValue, len(args))
	copy(c, args)
	return Proposition{phase: phase, modality: modality, term: term, args: c, validTime: validTime, quantification: quant}
}

func (Proposition) isSnapshot() {}

// CloneProposition convenience for tests and defensive copies
func CloneProposition(p Proposition) Proposition {
	args := make([]SnapshotValue, len(p.args))
	for i := range p.args {
		args[i] = CloneSnapshot(p.args[i])
	}
	return NewProposition(p.phase, p.modality, p.term, args, NewValidTime(p.validTime.Start, p.validTime.End), p.quantification)
}
