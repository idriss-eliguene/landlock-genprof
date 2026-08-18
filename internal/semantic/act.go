package semantic

import (
	"sort"
	"strings"
)

// Act represents an immutable RFC-0001 Act identity and minimal shape.
// Identity components per RFC §8.3.1: ⟨ Source, ActKind, interval, uses set ⟩
// usesPresent indicates whether the Act's uses set is determinate; where
// false the Act is unreproducible and its identity must not be presented
// as determinate (per 8.3.2). Construction performs defensive copies and
// normalizes set-valued inputs.
// Note: declaredOutputs is NOT part of Act identity; it records outputs
// asserted at construction time. The Graph maintains the authoritative
// union of declared and discovered outputs per Act key.
type Act struct {
	source                 SubjectIdentity
	kind                   ActKind
	interval               ValidTime
	uses                   []AssertionEventIdentity // normalized set semantics (unique, order not significant)
	usesPresent            bool
	declaredOutputs        []AssertionEventIdentity
	declaredOutputsPresent bool
}

// ActIdentity is an explicit, typed immutable representation of an Act's
// identity per RFC §8.3.1: ⟨Source, ActKind, interval, uses set⟩. It is
// separate from any Graph-specific key or representation.
type ActIdentity struct {
	Source   SubjectIdentity
	Kind     ActKind
	Interval ValidTime
	Uses     []AssertionEventIdentity // stored normalized deterministically
}

// NewAct constructs an Act. If uses == nil, the Act is recorded as
// unreproducible (usesPresent=false) per RFC-0001 §8.3.2. If uses is
// provided, duplicates are removed and ordering normalized deterministically.
// The outputs parameter is optional; pass nil when not available.
func NewAct(source SubjectIdentity, kind ActKind, interval ValidTime, uses []AssertionEventIdentity, outputs []AssertionEventIdentity) Act {
	act := Act{source: source, kind: kind, interval: interval}
	if uses == nil {
		act.usesPresent = false
		act.uses = nil
	} else {
		// normalize duplicates and produce deterministic ordering
		seen := make(map[string]struct{})
		uniq := make([]AssertionEventIdentity, 0, len(uses))
		for _, u := range uses {
			s := string(u)
			if _, ok := seen[s]; !ok {
				seen[s] = struct{}{}
				uniq = append(uniq, u)
			}
		}
		// deterministic order: lexical on token string
		sort.Slice(uniq, func(i, j int) bool { return string(uniq[i]) < string(uniq[j]) })
		act.uses = uniq
		act.usesPresent = true
	}
	if outputs == nil {
		act.declaredOutputsPresent = false
		act.declaredOutputs = nil
	} else {
		seen := make(map[string]struct{})
		uniq := make([]AssertionEventIdentity, 0, len(outputs))
		for _, u := range outputs {
			s := string(u)
			if _, ok := seen[s]; !ok {
				seen[s] = struct{}{}
				uniq = append(uniq, u)
			}
		}
		sort.Slice(uniq, func(i, j int) bool { return string(uniq[i]) < string(uniq[j]) })
		act.declaredOutputs = uniq
		act.declaredOutputsPresent = true
	}
	return act
}

// Identity returns the ActIdentity for this Act (defensive copy).
func (a Act) Identity() ActIdentity {
	ai := ActIdentity{Source: a.source, Kind: a.kind, Interval: a.interval}
	if a.usesPresent {
		ai.Uses = make([]AssertionEventIdentity, len(a.uses))
		copy(ai.Uses, a.uses)
	} else {
		ai.Uses = nil
	}
	return ai
}

// ActIdentity equality: structural, uses set equality (order-insensitive)
func (x ActIdentity) Equals(y ActIdentity) bool {
	if x.Source != y.Source {
		return false
	}
	if x.Kind != y.Kind {
		return false
	}
	if !validTimeEqual(x.Interval, y.Interval) {
		return false
	}
	// uses set equality: normalize by lexical ordering and compare
	xUses := make([]string, 0, len(x.Uses))
	yUses := make([]string, 0, len(y.Uses))
	for _, u := range x.Uses {
		xUses = append(xUses, string(u))
	}
	for _, u := range y.Uses {
		yUses = append(yUses, string(u))
	}
	sort.Strings(xUses)
	sort.Strings(yUses)
	if len(xUses) != len(yUses) {
		return false
	}
	for i := range xUses {
		if xUses[i] != yUses[i] {
			return false
		}
	}
	return true
}

// IdentityString produces a deterministic string for ActIdentity used for
// internal indexing only. It is NOT the semantic identity.
func (x ActIdentity) IdentityString() string {
	// ActKind is a closed enum, ActContact..ActAuthority (0..3), never
	// decoded from untrusted input — Acts are built in-process with a
	// compile-time constant kind — so the int -> rune conversion cannot
	// overflow and the '0'-offset encoding stays injective.
	// #nosec G115 -- bounded ActKind domain, see comment above
	parts := []string{string(x.Source), string(rune(x.Kind + '0'))}
	if x.Interval.Start != nil {
		parts = append(parts, x.Interval.Start.UTC().Format("2006-01-02T15:04:05.999999999Z"))
	} else {
		parts = append(parts, "<nil>")
	}
	parts = append(parts, ":")
	if x.Interval.End != nil {
		parts = append(parts, x.Interval.End.UTC().Format("2006-01-02T15:04:05.999999999Z"))
	} else {
		parts = append(parts, "<nil>")
	}
	if len(x.Uses) == 0 {
		parts = append(parts, "|uses:<unreproducible>")
	} else {
		us := make([]string, len(x.Uses))
		for i, u := range x.Uses {
			us[i] = string(u)
		}
		sort.Strings(us)
		parts = append(parts, "|uses:")
		parts = append(parts, us...)
	}
	return strings.Join(parts, "|")
}

// Accessors (defensive/value semantics)
func (a Act) Source() SubjectIdentity { return a.source }
func (a Act) Kind() ActKind           { return a.kind }
func (a Act) Interval() ValidTime     { return a.interval }

// Uses returns a defensive copy of the uses set as a slice. The order is
// deterministic but semantically unordered (set semantics).
func (a Act) Uses() []AssertionEventIdentity {
	if !a.usesPresent {
		return nil
	}
	out := make([]AssertionEventIdentity, len(a.uses))
	copy(out, a.uses)
	return out
}

// UsesPresent reports whether the Act's uses set is determinate.
func (a Act) UsesPresent() bool { return a.usesPresent }

// DeclaredOutputs returns the outputs declared at Act construction time.
func (a Act) DeclaredOutputs() []AssertionEventIdentity {
	if !a.declaredOutputsPresent {
		return nil
	}
	out := make([]AssertionEventIdentity, len(a.declaredOutputs))
	copy(out, a.declaredOutputs)
	return out
}

func (a Act) DeclaredOutputsPresent() bool { return a.declaredOutputsPresent }

// Equals reports identity equality between Acts using RFC identity: all
// four components compared. Where usesPresent differs, acts are distinct.
// Note: declaredOutputs is NOT part of identity.
func (a Act) Equals(b Act) bool {
	if a.source != b.source {
		return false
	}
	if a.kind != b.kind {
		return false
	}
	// interval equality: compare start/end pointers/time values
	if !validTimeEqual(a.interval, b.interval) {
		return false
	}
	if a.usesPresent != b.usesPresent {
		return false
	}
	if !a.usesPresent {
		return true
	} // both unreproducible
	// uses set equality: both slices are normalized deterministically in NewAct
	if len(a.uses) != len(b.uses) {
		return false
	}
	for i := range a.uses {
		if a.uses[i] != b.uses[i] {
			return false
		}
	}
	return true
}

func validTimeEqual(x, y ValidTime) bool {
	if x.Start == nil && y.Start != nil {
		return false
	}
	if x.Start != nil && y.Start == nil {
		return false
	}
	if x.End == nil && y.End != nil {
		return false
	}
	if x.End != nil && y.End == nil {
		return false
	}
	if x.Start != nil && y.Start != nil && !x.Start.Equal(*y.Start) {
		return false
	}
	if x.End != nil && y.End != nil && !x.End.Equal(*y.End) {
		return false
	}
	return true
}
