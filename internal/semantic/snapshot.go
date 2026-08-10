package semantic

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
		return NewProposition(v.predicate, args)
	default:
		return nil
	}
}

func (Record) isSnapshot() {}

// Proposition represents a structured Proposition: a predicate name and
// positional arguments. Some arguments may be set-valued (use Set) to
// indicate unordered collections per RFC-0002.
type Proposition struct {
	predicate string
	args      []SnapshotValue // positional
}

func NewProposition(predicate string, args []SnapshotValue) Proposition {
	c := make([]SnapshotValue, len(args))
	copy(c, args)
	return Proposition{predicate: predicate, args: c}
}

func (Proposition) isSnapshot() {}
