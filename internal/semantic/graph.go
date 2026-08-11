package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	ErrIdentityConflict   = errors.New("identity conflict: same identity, different content")
	ErrProducedByMismatch = errors.New("produced-by mismatch: event producer does not match Act identity")
	ErrTemporalViolation  = errors.New("temporal violation: uses must precede outputs by RecordTime")
	ErrRecordTimeConflict = errors.New("record time conflict for same AssertionEvent identity")
)

// Graph is an in-memory append-only historical Graph storing Acts and
// Assertion Events. It enforces append-only semantics and referential
// invariants locally. It does not perform belief evaluation.
type Graph struct {
	mu sync.RWMutex
	// assertions: map from public AssertionEventIdentity -> stored event
	assertions map[AssertionEventIdentity]AssertionEvent
	// acts keyed by their canonical act key
	acts map[string]Act
	// mapping from canonical assertion key -> bucket of public AssertionEventIdentity
	_assertionIndex map[string][]AssertionEventIdentity
	// mapping producer token -> list of assertion ids created with that producer (legacy index)
	producerIndex map[string][]AssertionEventIdentity
	// per-act outputs discovered/declared: actKey -> set of assertion ids
	actOutputs map[string]map[AssertionEventIdentity]struct{}
	// Act identity index: ActIdentity.IdentityString() -> actCanonicalKey
	actIdentityIndex map[string]string
}

// NewGraph constructs an empty Graph.
func NewGraph() *Graph {
	return &Graph{
		assertions:       make(map[AssertionEventIdentity]AssertionEvent),
		acts:             make(map[string]Act),
		_assertionIndex:  make(map[string][]AssertionEventIdentity),
		producerIndex:    make(map[string][]AssertionEventIdentity),
		actOutputs:       make(map[string]map[AssertionEventIdentity]struct{}),
		actIdentityIndex: make(map[string]string),
	}
}

// assertionCanonicalKey produces a deterministic key for an AssertionEvent
// based on its producer identity token and the canonical form of its
// Proposition. This key is internal only and used to detect semantic
// identity collisions and for indexing.
func assertionCanonicalKey(e AssertionEvent) string {
	// producer part
	prodPart := ""
	if p, ok := e.Producer().ActIdentity(); ok {
		prodPart = "Act:" + p.IdentityString()
	} else if o, ok := e.Producer().OpaqueIdentityRef(); ok {
		prodPart = o.TypeName() + ":" + o.Token()
	} else {
		prodPart = "unknown"
	}
	prop := e.Proposition() // defensive copy
	return prodPart + "|" + CanonicalString(prop)
}

// actCanonicalKey produces a deterministic key for an Act identity used
// to lookup Acts in the Graph.
func actCanonicalKey(a Act) string {
	// source + kind + interval + uses tokens sorted
	parts := []string{string(a.Source()), string(rune(a.Kind() + '0'))}
	if a.Interval().Start != nil {
		parts = append(parts, a.Interval().Start.UTC().Format("2006-01-02T15:04:05.999999999Z"))
	} else {
		parts = append(parts, "<nil>")
	}
	parts = append(parts, ":")
	if a.Interval().End != nil {
		parts = append(parts, a.Interval().End.UTC().Format("2006-01-02T15:04:05.999999999Z"))
	} else {
		parts = append(parts, "<nil>")
	}
	// uses tokens: deterministic order from Act constructor
	if a.UsesPresent() {
		uses := make([]string, len(a.Uses()))
		for i, u := range a.Uses() {
			uses[i] = string(u)
		}
		sort.Strings(uses)
		parts = append(parts, "|uses:")
		parts = append(parts, uses...)
	} else {
		parts = append(parts, "|uses:<unreproducible>")
	}
	return join(parts, "|")
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// AppendAssertionEvent appends an AssertionEvent to the Graph and returns
// a stable public AssertionEventIdentity. If an AssertionEvent with identical
// identity already exists, the operation is idempotent (returns existing
// identity). If the same identity already exists with conflicting content, an
// identity-conflict error is returned. If the same semantic identity is
// presented with a different immutable RecordTime, ErrRecordTimeConflict is
// returned.
func (g *Graph) AppendAssertionEvent(e AssertionEvent) (AssertionEventIdentity, error) {
	key := assertionCanonicalKey(e)
	g.mu.Lock()
	defer g.mu.Unlock()
	// inspect bucket for structural matches
	bucket := g._assertionIndex[key]
	for _, existingID := range bucket {
		existing := g.assertions[existingID]
		if existing.Equals(e) {
			// same semantic identity; ensure RecordTime matches
			if !existing.RecordTime().Time().Equal(e.RecordTime().Time()) {
				return "", ErrRecordTimeConflict
			}
			return existingID, nil
		}
	}
	// no structural match found: create new public id (hash of canonical key + counter)
	base := "evt:" + sha256Hex(key)
	idToken := base
	counter := 0
	for {
		id := AssertionEventIdentity(idToken)
		if _, exists := g.assertions[id]; !exists {
			break
		}
		counter++
		idToken = fmt.Sprintf("%s-%d", base, counter)
	}
	id := NewAssertionEventIdentity(idToken)
	// Validate structural invariants that are locally decidable:
	// Attempt to resolve producer to an Act via ActIdentity index
	if aid, ok := e.Producer().ActIdentity(); ok {
		actKey, present := g.actIdentityIndex[aid.IdentityString()]
		if present {
			act := g.acts[actKey]
			// temporal checks: every use in act must have RecordTime < e.RecordTime
			for _, u := range act.Uses() {
				if ue, ok := g.assertions[u]; ok {
					if !ue.RecordTime().Time().Before(e.RecordTime().Time()) {
						return "", ErrTemporalViolation
					}
				}
			}
		}
	} else if oref, ok := e.Producer().OpaqueIdentityRef(); ok {
		// legacy fallback: check if opaque token matches an act canonical key
		if act, present := g.acts[oref.Token()]; present {
			for _, u := range act.Uses() {
				if ue, ok := g.assertions[u]; ok {
					if !ue.RecordTime().Time().Before(e.RecordTime().Time()) {
						return "", ErrTemporalViolation
					}
				}
			}
		}
	}
	// store defensive copy and indexes
	g.assertions[id] = e
	// append to bucket
	g._assertionIndex[key] = append(g._assertionIndex[key], id)
	// populate producerIndex for legacy uses
	if oref, ok := e.Producer().OpaqueIdentityRef(); ok {
		g.producerIndex[oref.Token()] = append(g.producerIndex[oref.Token()], id)
	}
	// if the producer corresponds to a known Act, register output
	if aid, ok := e.Producer().ActIdentity(); ok {
		if actKey, present := g.actIdentityIndex[aid.IdentityString()]; present {
			if _, ok := g.actOutputs[actKey]; !ok {
				g.actOutputs[actKey] = make(map[AssertionEventIdentity]struct{})
			}
			g.actOutputs[actKey][id] = struct{}{}
		}
	} else if oref, ok := e.Producer().OpaqueIdentityRef(); ok {
		if _, ok := g.actOutputs[oref.Token()]; !ok {
			g.actOutputs[oref.Token()] = make(map[AssertionEventIdentity]struct{})
		}
		g.actOutputs[oref.Token()][id] = struct{}{}
	}
	return id, nil
}

// GetAssertionEvent returns the stored AssertionEvent and whether it was
// found.
func (g *Graph) GetAssertionEvent(id AssertionEventIdentity) (AssertionEvent, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	e, ok := g.assertions[id]
	return e, ok
}

// AppendAct appends an Act to the Graph. If an Act with identical identity
// already exists, idempotent success occurs. If a conflict in content is
// detected for same identity, ErrIdentityConflict is returned.
// AppendAct validates declared outputs against locally present events and
// checks temporal constraints when both uses and outputs are locally materialized.
func (g *Graph) AppendAct(a Act) error {
	key := actCanonicalKey(a)
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing, ok := g.acts[key]; ok {
		if existing.Equals(a) {
			// ensure declared outputs are compatible if present
			// we allow declared outputs to be a subset of discovered outputs; no-op
			return nil
		}
		return ErrIdentityConflict
	}
	// Validate declared outputs (if any)
	if a.DeclaredOutputsPresent() {
		for _, outID := range a.DeclaredOutputs() {
			// if the event exists locally, ensure its producer references this act key
			if ev, ok := g.assertions[outID]; ok {
				if aid, ok2 := ev.Producer().ActIdentity(); ok2 {
					if mappedKey, present := g.actIdentityIndex[aid.IdentityString()]; !present || mappedKey != key {
						return ErrProducedByMismatch
					}
				} else if oref, ok2 := ev.Producer().OpaqueIdentityRef(); ok2 {
					if oref.Token() != key {
						return ErrProducedByMismatch
					}
				} else {
					return ErrProducedByMismatch
				}
			}
		}
	}
	// Validate temporal constraints against any locally present outputs
	// discovered via producerIndex as well as declared outputs
	// collect outputs for this actKey
	outputsSet := make(map[AssertionEventIdentity]struct{})
	// declared
	if a.DeclaredOutputsPresent() {
		for _, o := range a.DeclaredOutputs() {
			outputsSet[o] = struct{}{}
		}
	}
	// discovered (events whose producer token equals key)
	if prodList, ok := g.producerIndex[key]; ok {
		for _, oid := range prodList {
			outputsSet[oid] = struct{}{}
		}
	}
	// Now check temporal constraints: for every u in uses and every o in outputsSet
	for _, u := range a.Uses() {
		if ue, ok := g.assertions[u]; ok {
			for o := range outputsSet {
				if oe, ok2 := g.assertions[o]; ok2 {
					if !ue.RecordTime().Time().Before(oe.RecordTime().Time()) {
						return ErrTemporalViolation
					}
				}
			}
		}
	}
	// store act and initialize actOutputs set including declared outputs and discovered ones
	g.acts[key] = a
	if _, ok := g.actOutputs[key]; !ok {
		g.actOutputs[key] = make(map[AssertionEventIdentity]struct{})
	}
	// add declared outputs
	if a.DeclaredOutputsPresent() {
		for _, o := range a.DeclaredOutputs() {
			g.actOutputs[key][o] = struct{}{}
		}
	}
	// add discovered outputs
	if prodList, ok := g.producerIndex[key]; ok {
		for _, oid := range prodList {
			g.actOutputs[key][oid] = struct{}{}
		}
	}
	return nil
}

// GetActs returns a defensive copy of stored Acts (as a slice).
func (g *Graph) GetActs() []Act {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]Act, 0, len(g.acts))
	for _, a := range g.acts {
		out = append(out, a)
	}
	return out
}

// HasAssertionByKey returns whether a canonical assertion key is present
func (g *Graph) HasAssertionByKey(key string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g._assertionIndex[key]
	return ok
}

// GetActOutputs returns the set of outputs (defensive slice) for an act key
func (g *Graph) GetActOutputs(actKey string) []AssertionEventIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	set := g.actOutputs[actKey]
	out := make([]AssertionEventIdentity, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	// deterministic order
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}
