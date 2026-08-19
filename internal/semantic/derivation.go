package semantic

import (
	"sort"
)

// package-private GroundSet value: immutable set of AssertionEventIdentity
type groundSet struct {
	members map[AssertionEventIdentity]struct{}
}

// newGroundSetFromSlice constructs a groundSet from provided ids.
// It normalizes duplicates and panics on empty input (test enforces non-empty).
func newGroundSetFromSlice(ids []AssertionEventIdentity) groundSet {
	gs := groundSet{members: make(map[AssertionEventIdentity]struct{}, len(ids))}
	for _, id := range ids {
		gs.members[id] = struct{}{}
	}
	// enforce non-empty by design
	if len(gs.members) == 0 {
		panic("groundSet must be non-empty")
	}
	return gs
}

// Members returns a deterministically ordered slice of members for testing/inspection.
func (g groundSet) Members() []AssertionEventIdentity {
	out := make([]AssertionEventIdentity, 0, len(g.members))
	for id := range g.members {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// equals compares groundSet structural equality (set-equality of members).
func (g groundSet) equals(o groundSet) bool {
	if len(g.members) != len(o.members) {
		return false
	}
	for id := range g.members {
		if _, ok := o.members[id]; !ok {
			return false
		}
	}
	return true
}

// candidateDerivation is a package-private immutable representation of a
// historical derivation that yields an incompatibility between two Terms.
// It records the exact AssertionEventIdentity members that support the derivation.
type candidateDerivation struct {
	left    IdentityRef
	right   IdentityRef
	grounds groundSet
}

// attack is a derived value for Defeat projection (package-private)
// attacker and target are AssertionEventIdentity tokens; grounds is the historic carrier(s).
type attack struct {
	attacker AssertionEventIdentity
	target   AssertionEventIdentity
	grounds  groundSet
}

// EnumerateDefeatAttacks returns a slice of derived Defeat attacks from the
// historical Graph. Each Attack corresponds to one asserted defeats AssertionEvent.
// The returned attacks are derived values and not persisted in the Graph.
func (g *Graph) EnumerateDefeatAttacks() []attack {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]attack, 0)
	// iterate stored assertions and pick defeat propositions (unique per carrier)
	for id, ev := range g.assertions {
		p := ev.Proposition()
		if !isDefeatProp(p) {
			continue
		}
		if len(p.args) < 2 {
			continue
		}
		// args: attacker AssertionRef, target AssertionRef
		aRef, ok1 := p.args[0].(IdentityRef)
		tRef, ok2 := p.args[1].(IdentityRef)
		if !ok1 || !ok2 {
			continue
		}
		att := AssertionEventIdentity(aRef.Token())
		tgt := AssertionEventIdentity(tRef.Token())
		gs := newGroundSetFromSlice([]AssertionEventIdentity{id})
		out = append(out, attack{attacker: att, target: tgt, grounds: gs})
	}
	// deterministic ordering for tests: sort by attacker then target then ground list
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
		ia := out[i].grounds.Members()
		ja := out[j].grounds.Members()
		for k := 0; k < len(ia) && k < len(ja); k++ {
			if string(ia[k]) != string(ja[k]) {
				return string(ia[k]) < string(ja[k])
			}
		}
		return len(ia) < len(ja)
	})
	return out
}

// EnumerateCandidateIncompatibilityDerivations enumerates all finite candidate
// derivations that could establish an incompatibility, working only from asserted
// Subsumption and Incompatibility AssertionEvents (historical Graph). This function
// is pure over the stored Graph and does not consider any Labelling/activation.
// It returns a slice of candidateDerivation where each entry contains the exact
// AssertionEvent identities forming the ground of that derivation.
func (g *Graph) EnumerateCandidateIncompatibilityDerivations() []candidateDerivation {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// collect all asserted incompatibility events and their term pairs
	type pair struct{ left, right IdentityRef }

	incompatEvents := make([]struct {
		id    AssertionEventIdentity
		left  IdentityRef
		right IdentityRef
	}, 0)
	for id, ev := range g.assertions {
		p := ev.Proposition()
		if !isIncompatibilityProp(p) {
			continue
		}
		if len(p.args) < 2 {
			continue
		}
		l, ok1 := p.args[0].(IdentityRef)
		r, ok2 := p.args[1].(IdentityRef)
		if !ok1 || !ok2 {
			continue
		}
		incompatEvents = append(incompatEvents, struct {
			id    AssertionEventIdentity
			left  IdentityRef
			right IdentityRef
		}{id: id, left: l, right: r})
	}

	// collect subsumption assertions with their (sub, super) tokens
	subsumptions := make([]struct {
		id    AssertionEventIdentity
		sub   IdentityRef
		super IdentityRef
	}, 0)
	for id, ev := range g.assertions {
		p := ev.Proposition()
		if !isSubsumptionProp(p) {
			continue
		}
		if len(p.args) < 2 {
			continue
		}
		s, ok1 := p.args[0].(IdentityRef)
		u, ok2 := p.args[1].(IdentityRef)
		if !ok1 || !ok2 {
			continue
		}
		subsumptions = append(subsumptions, struct {
			id    AssertionEventIdentity
			sub   IdentityRef
			super IdentityRef
		}{id: id, sub: s, super: u})
	}

	// index subsumptions by super token to allow downward substitution: if (u ⊑ X) then from (X,Y) derive (u,Y)
	subsBySuper := make(map[string][]int)
	for i, s := range subsumptions {
		sups := s.super.Token()
		subsBySuper[sups] = append(subsBySuper[sups], i)
	}

	// result set with dedup by (leftToken,rightToken,groundsKey)
	type resKey struct{ l, r, g string }
	seen := make(map[resKey]struct{})
	results := make([]candidateDerivation, 0)

	// helper to make groundKey
	makeGroundKey := func(gs groundSet) string {
		m := gs.Members()
		parts := make([]string, len(m))
		for i := range m {
			parts[i] = string(m[i])
		}
		return join(parts, ",")
	}

	// BFS per base incompatibility event
	for _, inv := range incompatEvents {
		startLeft := inv.left
		startRight := inv.right
		startGS := newGroundSetFromSlice([]AssertionEventIdentity{inv.id})
		// queue of states
		type state struct {
			l  IdentityRef
			r  IdentityRef
			gs groundSet
		}
		queue := []state{{l: startLeft, r: startRight, gs: startGS}}
		for len(queue) > 0 {
			st := queue[0]
			queue = queue[1:]
			key := resKey{l: st.l.Token(), r: st.r.Token(), g: makeGroundKey(st.gs)}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			// record derivation
			results = append(results, candidateDerivation{left: st.l, right: st.r, grounds: st.gs})
			// expand on left: find subsumptions u ⊑ currentLeft (i.e., sub.super == currentLeft)
			if idxs, ok := subsBySuper[st.l.Token()]; ok {
				for _, si := range idxs {
					s := subsumptions[si]
					// new pair (s.sub, st.r)
					// new ground set = union(st.gs, s.id)
					newMembers := make([]AssertionEventIdentity, 0, len(st.gs.members)+1)
					for m := range st.gs.members {
						newMembers = append(newMembers, m)
					}
					newMembers = append(newMembers, s.id)
					newGS := newGroundSetFromSlice(newMembers)
					newState := state{l: s.sub, r: st.r, gs: newGS}
					qk := resKey{l: newState.l.Token(), r: newState.r.Token(), g: makeGroundKey(newState.gs)}
					if _, ex := seen[qk]; !ex {
						queue = append(queue, newState)
					}
				}
			}
			// expand on right: find subsumptions v ⊑ currentRight
			if idxs, ok := subsBySuper[st.r.Token()]; ok {
				for _, si := range idxs {
					s := subsumptions[si]
					newMembers := make([]AssertionEventIdentity, 0, len(st.gs.members)+1)
					for m := range st.gs.members {
						newMembers = append(newMembers, m)
					}
					newMembers = append(newMembers, s.id)
					newGS := newGroundSetFromSlice(newMembers)
					newState := state{l: st.l, r: s.sub, gs: newGS}
					qk := resKey{l: newState.l.Token(), r: newState.r.Token(), g: makeGroundKey(newState.gs)}
					if _, ex := seen[qk]; !ex {
						queue = append(queue, newState)
					}
				}
			}
		}
	}

	// deterministic ordering of results for tests
	sort.Slice(results, func(i, j int) bool {
		li := results[i].left.Token()
		lj := results[j].left.Token()
		if li != lj {
			return li < lj
		}
		ri := results[i].right.Token()
		rj := results[j].right.Token()
		if ri != rj {
			return ri < rj
		}
		gi := makeGroundKey(results[i].grounds)
		gj := makeGroundKey(results[j].grounds)
		return gi < gj
	})

	return results
}
