package semantic

import (
	"errors"
	"sort"
	"sync"
)

var (
	ErrIdentityConflict = errors.New("identity conflict: same identity, different content")
)

// Graph is an in-memory append-only historical Graph storing Acts and
// Assertion Events. It enforces append-only semantics and referential
// invariants locally. It does not perform belief evaluation.
type Graph struct {
	mu         sync.RWMutex
	assertions map[AssertionEventIdentity]AssertionEvent
	acts       map[string]Act // key is canonical act key
	// reverse index: map from assertion token string -> assertion identity (redundant but helpful)
	_assertionIndex map[string]AssertionEventIdentity
}

// NewGraph constructs an empty Graph.
func NewGraph() *Graph {
	return &Graph{
		assertions:      make(map[AssertionEventIdentity]AssertionEvent),
		acts:            make(map[string]Act),
		_assertionIndex: make(map[string]AssertionEventIdentity),
	}
}

// assertionCanonicalKey produces a deterministic key for an AssertionEvent
// based on its producer identity token and the canonical form of its
// Proposition. This key is internal only and used to mint a stable
// AssertionEventIdentity for storage.
func assertionCanonicalKey(e AssertionEvent) string {
	prod := e.producer.Identity()
	prop := e.Proposition() // defensive copy
	return prod.TypeName() + ":" + prod.Token() + "|" + CanonicalString(prop)
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

// AppendAssertionEvent appends an AssertionEvent to the Graph and returns
// a stable AssertionEventIdentity. If an AssertionEvent with identical
// identity already exists, the operation is idempotent (returns existing
// identity). If the same identity exists with conflicting content, an
// identity-conflict error is returned.
func (g *Graph) AppendAssertionEvent(e AssertionEvent) (AssertionEventIdentity, error) {
	key := assertionCanonicalKey(e)
	id := NewAssertionEventIdentity(key)
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing, ok := g.assertions[id]; ok {
		// identical identity present: ensure content equality
		if existing.Equals(e) {
			return id, nil
		}
		return "", ErrIdentityConflict
	}
	// store defensive copy
	g.assertions[id] = e
	g._assertionIndex[key] = id
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
func (g *Graph) AppendAct(a Act) error {
	key := actCanonicalKey(a)
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing, ok := g.acts[key]; ok {
		if existing.Equals(a) {
			return nil
		}
		return ErrIdentityConflict
	}
	// store defensive copy
	g.acts[key] = a
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

// Assert whether assertion identity exists by checking canonical key
func (g *Graph) HasAssertionByKey(key string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g._assertionIndex[key]
	return ok
}
