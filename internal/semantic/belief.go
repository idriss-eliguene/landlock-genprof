package semantic

import (
	"sort"
)

// BeliefStatus per RFC: undecided, in, out (package-private)
type beliefStatus int

const (
	_beliefUndecided beliefStatus = iota
	_beliefIn
	_beliefOut
)

func (b beliefStatus) String() string {
	switch b {
	case _beliefUndecided:
		return "undecided"
	case _beliefIn:
		return "in"
	case _beliefOut:
		return "out"
	default:
		return "?"
	}
}

// Labelling is an immutable value mapping AssertionEventIdentity -> BeliefStatus.
// Missing entries are treated as undecided semantically.
type Labelling struct {
	m map[AssertionEventIdentity]beliefStatus
}

// NewLabellingFromMap constructs a Labelling from a map, copying contents.
func NewLabellingFromMap(src map[AssertionEventIdentity]beliefStatus) Labelling {
	m := make(map[AssertionEventIdentity]beliefStatus, len(src))
	for k, v := range src {
		m[k] = v
	}
	return Labelling{m: m}
}

// BottomLabelling constructs the bottom labelling (everything undecided)
// represented as an empty internal map (sparse convention).
func BottomLabelling() Labelling { return Labelling{m: make(map[AssertionEventIdentity]beliefStatus)} }

// Lookup returns the belief status for id; missing => undecided.
func (L Labelling) Lookup(id AssertionEventIdentity) beliefStatus {
	if L.m == nil {
		return _beliefUndecided
	}
	if v, ok := L.m[id]; ok {
		return v
	}
	return _beliefUndecided
}

// Equals compares two labellings over a provided evaluation universe. If
// universe is nil, comparison is over union of keys present in both maps.
func (L Labelling) Equals(o Labelling, universe []AssertionEventIdentity) bool {
	if universe == nil {
		// build union keys
		keys := make(map[AssertionEventIdentity]struct{})
		for k := range L.m {
			keys[k] = struct{}{}
		}
		for k := range o.m {
			keys[k] = struct{}{}
		}
		for k := range keys {
			if L.Lookup(k) != o.Lookup(k) {
				return false
			}
		}
		return true
	}
	for _, id := range universe {
		if L.Lookup(id) != o.Lookup(id) {
			return false
		}
	}
	return true
}

// statusLessOrEqual implements information order on beliefStatus.
func statusLessOrEqual(a, b beliefStatus) bool {
	if a == b {
		return true
	}
	if a == _beliefUndecided && (b == _beliefIn || b == _beliefOut) {
		return true
	}
	return false
}

// LabellingLessOrEqual returns true iff L1 <= L2 pointwise over universe.
// If universe is nil, uses union of keys from both labellings.
func LabellingLessOrEqual(L1, L2 Labelling, universe []AssertionEventIdentity) bool {
	if universe == nil {
		keys := make(map[AssertionEventIdentity]struct{})
		for k := range L1.m {
			keys[k] = struct{}{}
		}
		for k := range L2.m {
			keys[k] = struct{}{}
		}
		for k := range keys {
			if !statusLessOrEqual(L1.Lookup(k), L2.Lookup(k)) {
				return false
			}
		}
		return true
	}
	for _, id := range universe {
		if !statusLessOrEqual(L1.Lookup(id), L2.Lookup(id)) {
			return false
		}
	}
	return true
}

// evaluationUniverse returns all AssertionEventIdentity stored in graph in sorted order.
func (g *Graph) evaluationUniverse() []AssertionEventIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]AssertionEventIdentity, 0, len(g.assertions))
	for id := range g.assertions {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// Attack composition: use historical defeat attacks and rebuttal mappings.
// CandidateAttacks returns a defensive copy of derived attacks from Graph only.
func (g *Graph) CandidateAttacks() []attack {
	g.mu.RLock()
	defer g.mu.RUnlock()
	// start with defeat attacks (EnumerateDefeatAttacks uses g.assertions but with RLock inside; call internal logic inline to avoid locking twice)
	defeats := g.EnumerateDefeatAttacks()
	out := make([]attack, 0, len(defeats))
	out = append(out, defeats...)

	// build proposition canonical -> assertion list map
	propMap := make(map[string][]AssertionEventIdentity)
	for aid, ev := range g.assertions {
		k := CanonicalString(ev.Proposition())
		propMap[k] = append(propMap[k], aid)
	}

	// build derivation map key = leftToken + "|" + rightToken -> []candidateDerivation
	derivs := g.EnumerateCandidateIncompatibilityDerivations()
	derivMap := make(map[string][]candidateDerivation)
	for _, d := range derivs {
		key := d.left.Token() + "|" + d.right.Token()
		derivMap[key] = append(derivMap[key], d)
	}

	// iterate stored assertions to find rebuttal propositions (we must examine each Rebuttal assertion event)
	for _, ev := range g.assertions {
		p := ev.Proposition()
		if !isRebuttalProp(p) {
			continue
		}
		// args are propositions
		if len(p.args) < 2 {
			continue
		}
		pa, ok1 := p.args[0].(Proposition)
		pb, ok2 := p.args[1].(Proposition)
		if !ok1 || !ok2 {
			continue
		}
		// find assertion events that state pa and pb
		kpa := CanonicalString(pa)
		kpb := CanonicalString(pb)
		alist := propMap[kpa]
		tlist := propMap[kpb]
		if len(alist) == 0 || len(tlist) == 0 {
			// no assertion events stating these propositions -> cannot form attacks
			continue
		}
		// find derivations for terms pa.term -> pb.term
		key := pa.term.Token() + "|" + pb.term.Token()
		cds := derivMapLookup(derivMap, key)
		if len(cds) == 0 {
			// no candidate derivations -> no rebuttal attack derivable historically
			continue
		}
		// for each pair of assertion events and each derivation, create attack
		for _, a := range alist {
			for _, t := range tlist {
				for _, cd := range cds {
					out = append(out, attack{attacker: a, target: t, grounds: cd.grounds})
				}
			}
		}
	}

	// deterministic ordering
	sort.Slice(out, func(i, j int) bool {
		iatk := string(out[i].attacker)
		jatk := string(out[j].attacker)
		if iatk != jatk {
			return iatk < jatk
		}
		itgt := string(out[i].target)
		jtgt := string(out[j].target)
		if itgt != jtgt {
			return itgt < jtgt
		}
		gi := out[i].grounds.Members()
		gj := out[j].grounds.Members()
		for k := 0; k < len(gi) && k < len(gj); k++ {
			if string(gi[k]) != string(gj[k]) {
				return string(gi[k]) < string(gj[k])
			}
		}
		return len(gi) < len(gj)
	})
	return out
}

func derivMapLookup(m map[string][]candidateDerivation, key string) []candidateDerivation {
	if v, ok := m[key]; ok {
		return v
	}
	return nil
}

// attackStatus returns active/void/potential per RFC given attack and Labelling L.
func attackStatus(a attack, L Labelling) string {
	att := L.Lookup(a.attacker)
	if att == _beliefOut {
		return "void"
	}
	// if any ground member out -> void
	for _, g := range a.grounds.Members() {
		if L.Lookup(g) == _beliefOut {
			return "void"
		}
	}
	// active if attacker in and all grounds in
	if att == _beliefIn {
		allIn := true
		for _, g := range a.grounds.Members() {
			if L.Lookup(g) != _beliefIn {
				allIn = false
				break
			}
		}
		if allIn {
			return "active"
		}
	}
	return "potential"
}

// OutCondition and InCondition
func (g *Graph) OutCondition(e AssertionEventIdentity, L Labelling) bool {
	// attack active targeting e
	attacks := g.CandidateAttacks()
	for _, A := range attacks {
		if A.target == e {
			if attackStatus(A, L) == "active" {
				return true
			}
		}
	}
	// premise out
	for _, p := range g.premises(e) {
		if L.Lookup(p) == _beliefOut {
			return true
		}
	}
	return false
}

func (g *Graph) InCondition(e AssertionEventIdentity, L Labelling) bool {
	// every candidate attack targeting e is void
	attacks := g.CandidateAttacks()
	for _, A := range attacks {
		if A.target == e {
			if attackStatus(A, L) != "void" {
				return false
			}
		}
	}
	// every premise in
	for _, p := range g.premises(e) {
		if L.Lookup(p) != _beliefIn {
			return false
		}
	}
	return true
}

// premises returns the premises (uses set) of the Act that produced AssertionEvent e
// If producer cannot be resolved to a known Act, returns empty slice.
func (g *Graph) premises(e AssertionEventIdentity) []AssertionEventIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ev, ok := g.assertions[e]
	if !ok {
		return nil
	}
	if aid, ok2 := ev.Producer().ActIdentity(); ok2 {
		if actKey, present := g.actIdentityIndex[aid.IdentityString()]; present {
			if act, ok3 := g.acts[actKey]; ok3 {
				return act.Uses()
			}
		}
	}
	return nil
}

// phi applies one step of RFC operator Φ over Graph G and input Labelling L.
// Returns a new Labelling value (copied map), does not mutate G or L.
func (g *Graph) phi(L Labelling) Labelling {
	univ := g.evaluationUniverse()
	resMap := make(map[AssertionEventIdentity]beliefStatus)
	for _, e := range univ {
		if g.OutCondition(e, L) {
			resMap[e] = _beliefOut
			continue
		}
		if g.InCondition(e, L) {
			resMap[e] = _beliefIn
			continue
		}
		// undecided omitted (sparse)
	}
	return NewLabellingFromMap(resMap)
}
