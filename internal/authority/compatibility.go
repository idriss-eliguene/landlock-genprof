package authority

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type CompatibilitySchema string

const (
	CompatibilityRuleV1 CompatibilitySchema = "compatibility-rule.v1"
	CompatibilityRuleV2 CompatibilitySchema = "compatibility-rule.v2"
)

type CompatibilityOutcome uint8

const (
	CompatibilityResultInvalid CompatibilityOutcome = iota
	CompatibilityResultCompatible
	CompatibilityResultIncompatible
	CompatibilityResultUnknown
	CompatibilityResultNotApplicable
)

type TypedSet struct {
	values         map[string]struct{}
	unknown, valid bool
}

func NewTypedSet(values []string, unknown bool) (TypedSet, error) {
	m := map[string]struct{}{}
	for _, v := range values {
		if v == "" {
			return TypedSet{}, fmt.Errorf("invalid set member")
		}
		if _, ok := m[v]; ok {
			return TypedSet{}, fmt.Errorf("duplicate set member")
		}
		m[v] = struct{}{}
	}
	return TypedSet{m, unknown, true}, nil
}

type TypedMapValue struct {
	Value   string
	Unknown bool
}
type TypedMap struct {
	values         map[string]TypedMapValue
	unknown, valid bool
}
type RawMapEntry struct {
	Key   string
	Value TypedMapValue
}

func NewTypedMapEntries(entries []RawMapEntry, unknown bool) (TypedMap, error) {
	m := make(map[string]TypedMapValue, len(entries))
	for _, e := range entries {
		if _, ok := m[e.Key]; ok {
			return TypedMap{}, fmt.Errorf("duplicate raw map key")
		}
		m[e.Key] = e.Value
	}
	return NewTypedMap(m, unknown)
}

func validMapKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		if i == 0 {
			if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_') {
				return false
			}
		} else if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}
func NewTypedMap(values map[string]TypedMapValue, unknown bool) (TypedMap, error) {
	m := map[string]TypedMapValue{}
	for k, v := range values {
		if !validMapKey(k) || !utf8.ValidString(v.Value) || stringsIndexNUL(v.Value) {
			return TypedMap{}, fmt.Errorf("invalid map entry")
		}
		m[k] = v
	}
	return TypedMap{m, unknown, true}, nil
}
func stringsIndexNUL(s string) bool {
	for _, r := range s {
		if r == 0 {
			return true
		}
	}
	return false
}

type CompatibilityOperands struct {
	Scalar      string
	ScalarKnown bool
	Set         TypedSet
	Map         TypedMap
}

type CompatibilityEvaluationResult struct {
	attempt                                                                                          ResolutionAttemptIdentity
	schema, predicate, field, candidate, baseline, requirement, authority, subject, backend, context string
	scope                                                                                            Scope
	validity                                                                                         Validity
	revocation                                                                                       CurrentRevocationFact
	provenance                                                                                       ProvenanceRecord
	outcome                                                                                          CompatibilityOutcome
}

func (r CompatibilityEvaluationResult) Valid() bool {
	return r.attempt.Valid() && r.schema != "" && r.predicate != "" && r.field != "" && r.candidate != "" && r.baseline != "" && r.requirement != "" && r.authority != "" && r.subject != "" && r.backend != "" && r.context != "" && r.scope.Valid() && validPredicateValidity(r.validity) && r.revocation.Valid() && r.revocation.attempt == r.attempt && r.provenance.Valid() && r.outcome <= CompatibilityResultNotApplicable
}

func DeriveCompatibilityFact(r CompatibilityEvaluationResult) (CompatibilityFact, error) {
	if !r.Valid() || r.outcome == CompatibilityResultInvalid {
		return CompatibilityFact{}, fmt.Errorf("invalid compatibility evaluation result")
	}
	state := CompatibilityUnknown
	switch r.outcome {
	case CompatibilityResultCompatible:
		state = CompatibilityCompatible
	case CompatibilityResultIncompatible:
		state = CompatibilityIncompatible
	case CompatibilityResultUnknown:
		state = CompatibilityUnknown
	default:
		return CompatibilityFact{}, fmt.Errorf("non-applicable compatibility result")
	}
	return CompatibilityFact{r.attempt, r.schema, r.predicate, r.field, r.candidate, r.baseline, r.requirement, r.authority, r.subject, r.backend, r.context, r.scope, r.validity, r.revocation, r.provenance, state}, nil
}

func EvaluateCompatibility(schema CompatibilitySchema, predicate CompatibilityPredicate, candidate, baseline, expected CompatibilityOperands) (CompatibilityOutcome, error) {
	if schema != CompatibilityRuleV1 && schema != CompatibilityRuleV2 {
		return CompatibilityResultInvalid, fmt.Errorf("unsupported compatibility schema")
	}
	if !predicate.Valid() {
		return CompatibilityResultInvalid, fmt.Errorf("unknown compatibility predicate")
	}
	if schema == CompatibilityRuleV1 && (predicate == CompatibilitySetContains || predicate == CompatibilityMapContains) {
		return CompatibilityResultInvalid, fmt.Errorf("containment is V2-only")
	}
	if (predicate == CompatibilitySetContains || predicate == CompatibilityMapContains) && (expected.ScalarKnown || expected.Set.valid || expected.Map.valid) {
		return CompatibilityResultInvalid, fmt.Errorf("containment has no expected operand")
	}
	switch predicate {
	case CompatibilitySetContains:
		return evalSetContains(candidate.Set, baseline.Set)
	case CompatibilityMapContains:
		return evalMapContains(candidate.Map, baseline.Map)
	case CompatibilitySetMembership:
		if !candidate.ScalarKnown {
			return CompatibilityResultUnknown, nil
		}
		if !expected.Set.valid {
			return CompatibilityResultInvalid, fmt.Errorf("invalid membership set")
		}
		if _, ok := expected.Set.values[candidate.Scalar]; ok {
			return CompatibilityResultCompatible, nil
		}
		return CompatibilityResultIncompatible, nil
	case CompatibilityExactEquality:
		if !candidate.ScalarKnown || !baseline.ScalarKnown || !expected.ScalarKnown {
			return CompatibilityResultUnknown, nil
		}
		if candidate.Scalar == baseline.Scalar && candidate.Scalar == expected.Scalar {
			return CompatibilityResultCompatible, nil
		}
		return CompatibilityResultIncompatible, nil
	case CompatibilityExactVersion:
		if !candidate.ScalarKnown || !baseline.ScalarKnown || !expected.ScalarKnown {
			return CompatibilityResultUnknown, nil
		}
		if !validVersion(candidate.Scalar) || !validVersion(baseline.Scalar) || !validVersion(expected.Scalar) {
			return CompatibilityResultInvalid, fmt.Errorf("invalid version")
		}
		if normalizeVersion(candidate.Scalar) == normalizeVersion(baseline.Scalar) && normalizeVersion(candidate.Scalar) == normalizeVersion(expected.Scalar) {
			return CompatibilityResultCompatible, nil
		}
		return CompatibilityResultIncompatible, nil
	case CompatibilityArchitectureABI, CompatibilityDigestEquality:
		if !candidate.ScalarKnown || !baseline.ScalarKnown || !expected.ScalarKnown {
			return CompatibilityResultUnknown, nil
		}
		if candidate.Scalar == baseline.Scalar && candidate.Scalar == expected.Scalar {
			return CompatibilityResultCompatible, nil
		}
		return CompatibilityResultIncompatible, nil
	case CompatibilityVersionRange:
		if !candidate.ScalarKnown || !baseline.ScalarKnown || !expected.ScalarKnown {
			return CompatibilityResultUnknown, nil
		}
		return evalVersionRange(candidate.Scalar, baseline.Scalar, expected.Scalar)
	default:
		return CompatibilityResultInvalid, fmt.Errorf("predicate requires domain-specific evaluator")
	}
}

func validVersion(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '.' {
			if i == 0 || i == len(s)-1 {
				return false
			}
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	parts := strings.Split(s, ".")
	for _, p := range parts {
		if len(p) > 1 && p[0] == '0' {
			return false
		}
	}
	return true
}
func normalizeVersion(s string) string {
	p := strings.Split(s, ".")
	for len(p) > 1 && p[len(p)-1] == "0" {
		p = p[:len(p)-1]
	}
	return strings.Join(p, ".")
}
func evalVersionRange(a, b, r string) (CompatibilityOutcome, error) {
	if len(r) < 5 {
		return CompatibilityResultInvalid, fmt.Errorf("invalid version range")
	}
	open, close := r[0], r[len(r)-1]
	if (open != '[' && open != '(') || (close != ']' && close != ')') {
		return CompatibilityResultInvalid, fmt.Errorf("invalid version range")
	}
	p := strings.Split(r[1:len(r)-1], ",")
	if len(p) != 2 || !validVersion(p[0]) || !validVersion(p[1]) {
		return CompatibilityResultInvalid, fmt.Errorf("invalid version range")
	}
	if !inRange(a, p[0], p[1], open == '[', close == ']') || !inRange(b, p[0], p[1], open == '[', close == ']') {
		return CompatibilityResultIncompatible, nil
	}
	return CompatibilityResultCompatible, nil
}
func versionCmp(a, b string) int {
	pa, pb := strings.Split(normalizeVersion(a), "."), strings.Split(normalizeVersion(b), ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
func inRange(v, lo, hi string, incLo, incHi bool) bool {
	cl, ch := versionCmp(v, lo), versionCmp(v, hi)
	return (cl > 0 || incLo && cl == 0) && (ch < 0 || incHi && ch == 0)
}
func evalSetContains(a, b TypedSet) (CompatibilityOutcome, error) {
	if !a.valid || !b.valid {
		return CompatibilityResultInvalid, fmt.Errorf("invalid set operand")
	}
	if a.unknown || b.unknown {
		return CompatibilityResultUnknown, nil
	}
	for k := range a.values {
		if _, ok := b.values[k]; !ok {
			return CompatibilityResultIncompatible, nil
		}
	}
	return CompatibilityResultCompatible, nil
}
func evalMapContains(a, b TypedMap) (CompatibilityOutcome, error) {
	if !a.valid || !b.valid {
		return CompatibilityResultInvalid, fmt.Errorf("invalid map operand")
	}
	if a.unknown || b.unknown {
		return CompatibilityResultUnknown, nil
	}
	for k, v := range a.values {
		tv, ok := b.values[k]
		if !ok {
			return CompatibilityResultIncompatible, nil
		}
		if v.Unknown || tv.Unknown {
			return CompatibilityResultUnknown, nil
		}
		if v.Value != tv.Value {
			return CompatibilityResultIncompatible, nil
		}
	}
	return CompatibilityResultCompatible, nil
}
