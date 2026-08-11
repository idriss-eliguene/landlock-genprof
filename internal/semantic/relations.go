package semantic

// Relation constructors and relation-index helpers for Slice 4.

// NewDefeatProposition constructs a Proposition asserting that attacker AssertionEvent
// defeated target AssertionEvent. Both arguments are AssertionEventIdentity tokens.
func NewDefeatProposition(attacker, target AssertionEventIdentity, vt ValidTime) Proposition {
	args := []SnapshotValue{NewIdentityRef("Assertion", string(attacker)), NewIdentityRef("Assertion", string(target))}
	phase := NewIdentityRef("Relation", "Defeat")
	term := NewIdentityRef("Relation", "Defeat")
	return NewProposition(phase, Actual, term, args, vt, QuantThisInstance)
}

// NewRebuttalProposition constructs a Proposition asserting Rebuttal between
// two Propositions. The arguments are Propositions themselves (by value).
func NewRebuttalProposition(a, b Proposition, vt ValidTime) Proposition {
	args := []SnapshotValue{CloneProposition(a), CloneProposition(b)}
	phase := NewIdentityRef("Relation", "Rebuttal")
	term := NewIdentityRef("Relation", "Rebuttal")
	return NewProposition(phase, Actual, term, args, vt, QuantThisInstance)
}

// NewSubsumptionProposition asserts that termA subsumes termB (tA ⊑ tB).
// Terms are referenced by IdentityRef tokens.
func NewSubsumptionProposition(termA, termB IdentityRef, vt ValidTime) Proposition {
	args := []SnapshotValue{termA, termB}
	phase := NewIdentityRef("Relation", "Subsumption")
	term := NewIdentityRef("Relation", "Subsumption")
	return NewProposition(phase, Actual, term, args, vt, QuantThisInstance)
}

// NewIncompatibilityProposition asserts that termA is incompatible with termB.
func NewIncompatibilityProposition(termA, termB IdentityRef, vt ValidTime) Proposition {
	args := []SnapshotValue{termA, termB}
	phase := NewIdentityRef("Relation", "Incompatibility")
	term := NewIdentityRef("Relation", "Incompatibility")
	return NewProposition(phase, Actual, term, args, vt, QuantThisInstance)
}

// relation indexing helpers (package-private)
func isDefeatProp(p Proposition) bool {
	return p.phase.TypeName() == "Relation" && p.phase.Token() == "Defeat"
}
func isRebuttalProp(p Proposition) bool {
	return p.phase.TypeName() == "Relation" && p.phase.Token() == "Rebuttal"
}
func isSubsumptionProp(p Proposition) bool {
	return p.phase.TypeName() == "Relation" && p.phase.Token() == "Subsumption"
}
func isIncompatibilityProp(p Proposition) bool {
	return p.phase.TypeName() == "Relation" && p.phase.Token() == "Incompatibility"
}

// updateRelationIndexes updates derived relation indexes for a newly stored
// AssertionEvent with public id "id". It inspects the Proposition and updates
// appropriate maps. This remains derived/cache-only and does not alter
// semantic truth which is the stored AssertionEvent.
func (g *Graph) updateRelationIndexes(id AssertionEventIdentity, e AssertionEvent) {
	p := e.Proposition()
	// Defeat: args [attacker AssertionRef, target AssertionRef]
	if isDefeatProp(p) {
		if len(p.args) >= 2 {
			if aRef, ok := p.args[0].(IdentityRef); ok {
				if tRef, ok2 := p.args[1].(IdentityRef); ok2 {
					att := AssertionEventIdentity(aRef.Token())
					tgt := AssertionEventIdentity(tRef.Token())
					g.defeatByAttacker[att] = append(g.defeatByAttacker[att], id)
					g.defeatByTarget[tgt] = append(g.defeatByTarget[tgt], id)
				}
			}
		}
		return
	}
	// Rebuttal: args [proposition A, proposition B]
	if isRebuttalProp(p) {
		if len(p.args) >= 2 {
			if pa, ok := p.args[0].(Proposition); ok {
				if pb, ok2 := p.args[1].(Proposition); ok2 {
					ka := CanonicalString(pa)
					kb := CanonicalString(pb)
					g.rebuttalByProp[ka] = append(g.rebuttalByProp[ka], id)
					g.rebuttalByProp[kb] = append(g.rebuttalByProp[kb], id)
				}
			}
		}
		return
	}
	// Subsumption: args [termA IdentityRef, termB IdentityRef]
	if isSubsumptionProp(p) {
		if len(p.args) >= 2 {
			if aRef, ok := p.args[0].(IdentityRef); ok {
				if bRef, ok2 := p.args[1].(IdentityRef); ok2 {
					g.subsumptionByTerm[aRef.Token()] = append(g.subsumptionByTerm[aRef.Token()], id)
					g.subsumptionByTerm[bRef.Token()] = append(g.subsumptionByTerm[bRef.Token()], id)
				}
			}
		}
		return
	}
	// Incompatibility: args [termA, termB]
	if isIncompatibilityProp(p) {
		if len(p.args) >= 2 {
			if aRef, ok := p.args[0].(IdentityRef); ok {
				if bRef, ok2 := p.args[1].(IdentityRef); ok2 {
					g.incompatibilityByTerm[aRef.Token()] = append(g.incompatibilityByTerm[aRef.Token()], id)
					g.incompatibilityByTerm[bRef.Token()] = append(g.incompatibilityByTerm[bRef.Token()], id)
				}
			}
		}
		return
	}
}

// RebuildRelationIndexes recomputes all derived relation indexes from the
// stored AssertionEvents. It is useful for tests verifying indexes are
// caches only.
func (g *Graph) RebuildRelationIndexes() {
	g.mu.Lock()
	defer g.mu.Unlock()
	// reset
	g.defeatByTarget = make(map[AssertionEventIdentity][]AssertionEventIdentity)
	g.defeatByAttacker = make(map[AssertionEventIdentity][]AssertionEventIdentity)
	g.rebuttalByProp = make(map[string][]AssertionEventIdentity)
	g.subsumptionByTerm = make(map[string][]AssertionEventIdentity)
	g.incompatibilityByTerm = make(map[string][]AssertionEventIdentity)
	// iterate stored assertions
	for id, ev := range g.assertions {
		g.updateRelationIndexes(id, ev)
	}
}

// Query APIs: return defensive copies
func (g *Graph) DefeatAssertionsByTarget(target AssertionEventIdentity) []AssertionEventIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := g.defeatByTarget[target]
	out := make([]AssertionEventIdentity, len(list))
	copy(out, list)
	return out
}

func (g *Graph) DefeatAssertionsByAttacker(attacker AssertionEventIdentity) []AssertionEventIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := g.defeatByAttacker[attacker]
	out := make([]AssertionEventIdentity, len(list))
	copy(out, list)
	return out
}

func (g *Graph) RebuttalAssertionsByProposition(p Proposition) []AssertionEventIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	k := CanonicalString(p)
	list := g.rebuttalByProp[k]
	out := make([]AssertionEventIdentity, len(list))
	copy(out, list)
	return out
}

func (g *Graph) SubsumptionAssertionsByTerm(t IdentityRef) []AssertionEventIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := g.subsumptionByTerm[t.Token()]
	out := make([]AssertionEventIdentity, len(list))
	copy(out, list)
	return out
}

func (g *Graph) IncompatibilityAssertionsByTerm(t IdentityRef) []AssertionEventIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := g.incompatibilityByTerm[t.Token()]
	out := make([]AssertionEventIdentity, len(list))
	copy(out, list)
	return out
}
