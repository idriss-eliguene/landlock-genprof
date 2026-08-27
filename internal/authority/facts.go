package authority

import (
	"fmt"
	"sort"
	"strings"
)

type ResolutionAttemptIdentity string

func NewResolutionAttemptIdentity(v string) (ResolutionAttemptIdentity, error) {
	if v == "" {
		return "", fmt.Errorf("empty resolution attempt identity")
	}
	return ResolutionAttemptIdentity(v), nil
}
func (i ResolutionAttemptIdentity) Valid() bool { return i != "" }

type TrustFact struct {
	attempt               ResolutionAttemptIdentity
	subject, policy, root string
	scope                 Scope
	context               SecurityContextIdentity
	validity              Validity
	revocation            CurrentRevocationFact
	provenance            ProvenanceRecord
	state                 TrustState
}

func (f TrustFact) Valid() bool {
	return f.attempt.Valid() && f.subject != "" && f.policy != "" && f.root != "" && f.scope.Valid() && validContext(f.context) && validPredicateValidity(f.validity) && f.revocation.Valid() && f.provenance.Valid() && f.state.Valid()
}
func (f TrustFact) State() TrustState { return f.state }

type CurrentRevocationFact struct {
	attempt         ResolutionAttemptIdentity
	subject, source string
	state           RevocationStatus
	provenance      ProvenanceRecord
	validUntil      Validity
}

func (f CurrentRevocationFact) Valid() bool {
	return f.attempt.Valid() && f.subject != "" && f.source != "" && validRevocation(f.state) && f.provenance.Valid() && validPredicateValidity(f.validUntil)
}
func (f CurrentRevocationFact) State() RevocationStatus { return f.state }

type VerificationFactState uint8

const (
	VerificationFactInvalid VerificationFactState = iota
	VerificationFactVerified
	VerificationFactFailed
	VerificationFactUnknown
)

func (s VerificationFactState) Valid() bool {
	return s >= VerificationFactVerified && s <= VerificationFactUnknown
}

type ResolvedVerificationFact struct {
	attempt                     ResolutionAttemptIdentity
	subject, verifier, property string
	scope                       Scope
	context                     SecurityContextIdentity
	validity                    Validity
	revocation                  CurrentRevocationFact
	provenance                  ProvenanceRecord
	state                       VerificationFactState
}

func (f ResolvedVerificationFact) Valid() bool {
	return f.attempt.Valid() && f.subject != "" && f.verifier != "" && f.property != "" && f.scope.Valid() && validContext(f.context) && validPredicateValidity(f.validity) && f.revocation.Valid() && f.provenance.Valid() && f.state.Valid()
}

type CompatibilityFactState uint8

const (
	CompatibilityFactInvalid CompatibilityFactState = iota
	CompatibilityCompatible
	CompatibilityIncompatible
	CompatibilityUnknown
)

func (s CompatibilityFactState) Valid() bool {
	return s >= CompatibilityCompatible && s <= CompatibilityUnknown
}

type CompatibilityFact struct {
	attempt                                                                                          ResolutionAttemptIdentity
	schema, predicate, field, candidate, baseline, requirement, authority, subject, backend, context string
	scope                                                                                            Scope
	validity                                                                                         Validity
	revocation                                                                                       CurrentRevocationFact
	provenance                                                                                       ProvenanceRecord
	state                                                                                            CompatibilityFactState
}

func (f CompatibilityFact) Valid() bool {
	return f.attempt.Valid() && f.schema != "" && f.predicate != "" && f.field != "" && f.candidate != "" && f.baseline != "" && f.requirement != "" && f.authority != "" && f.subject != "" && f.backend != "" && f.context != "" && f.scope.Valid() && validPredicateValidity(f.validity) && f.revocation.Valid() && f.provenance.Valid() && f.state.Valid()
}

type CoverageFact struct {
	attempt                  ResolutionAttemptIdentity
	subject, backend, source string
	scope                    Scope
	context                  SecurityContextIdentity
	validity                 Validity
	revocation               CurrentRevocationFact
	state                    ScopeCoverageState
	provenance               ProvenanceRecord
}

func (f CoverageFact) Valid() bool {
	return f.attempt.Valid() && f.subject != "" && f.backend != "" && f.source != "" && f.scope.Valid() && validContext(f.context) && validPredicateValidity(f.validity) && f.revocation.Valid() && f.state.Valid() && f.provenance.Valid()
}

type CompletenessFact struct {
	attempt    ResolutionAttemptIdentity
	subject    string
	class      CompletenessClass
	scope      Scope
	provenance ProvenanceRecord
	validity   Validity
	revocation CurrentRevocationFact
}

func (f CompletenessFact) Valid() bool {
	return f.attempt.Valid() && f.subject != "" && f.class.Valid() && f.scope.Valid() && f.provenance.Valid() && validPredicateValidity(f.validity) && f.revocation.Valid()
}

type AdequacyFact struct {
	attempt    ResolutionAttemptIdentity
	subject    string
	class      AdequacyClass
	scope      Scope
	context    SecurityContextIdentity
	verifier   VerifierSemanticIdentity
	provenance ProvenanceRecord
	validity   Validity
	revocation CurrentRevocationFact
}

func (f AdequacyFact) Valid() bool {
	return f.attempt.Valid() && f.subject != "" && f.class.Valid() && f.scope.Valid() && validContext(f.context) && validVerifier(f.verifier) && f.provenance.Valid() && validPredicateValidity(f.validity) && f.revocation.Valid()
}

type CertificationFact struct {
	attempt                     ResolutionAttemptIdentity
	subject, identity, property string
	scope                       Scope
	context                     SecurityContextIdentity
	verifier                    VerifierSemanticIdentity
	provenance                  ProvenanceRecord
	validity                    Validity
	revocation                  CurrentRevocationFact
}

func (f CertificationFact) Valid() bool {
	return f.attempt.Valid() && f.subject != "" && f.identity != "" && f.property != "" && f.scope.Valid() && validContext(f.context) && validVerifier(f.verifier) && f.provenance.Valid() && validPredicateValidity(f.validity)
}

type RequirementMatchOutcome uint8

const (
	MatchInvalid RequirementMatchOutcome = iota
	MatchSatisfied
	MatchRefuted
	MatchUnknown
	MatchNonMatching
)

func (o RequirementMatchOutcome) Valid() bool { return o >= MatchSatisfied && o <= MatchNonMatching }

type RequirementMatch struct {
	attempt                                           ResolutionAttemptIdentity
	authority, requirement, subject, backend, context string
	scope                                             Scope
	outcome                                           RequirementMatchOutcome
}

func (m RequirementMatch) Valid() bool {
	return m.attempt.Valid() && m.authority != "" && m.requirement != "" && m.subject != "" && m.backend != "" && m.context != "" && m.scope.Valid() && m.outcome.Valid()
}

type EvaluationFactSnapshot struct {
	attempt         ResolutionAttemptIdentity
	trusts          []TrustFact
	revocations     []CurrentRevocationFact
	verifications   []ResolvedVerificationFact
	compatibilities []CompatibilityFact
	coverages       []CoverageFact
	completeness    []CompletenessFact
	adequacies      []AdequacyFact
	certifications  []CertificationFact
}

func (s EvaluationFactSnapshot) Valid() bool { return s.attempt.Valid() }
func NewEvaluationFactSnapshot(a ResolutionAttemptIdentity, t []TrustFact, r []CurrentRevocationFact, v []ResolvedVerificationFact, c []CompatibilityFact) (EvaluationFactSnapshot, error) {
	return NewEvaluationFactSnapshotAll(a, t, r, v, c, nil, nil, nil, nil)
}
func NewEvaluationFactSnapshotAll(a ResolutionAttemptIdentity, t []TrustFact, r []CurrentRevocationFact, v []ResolvedVerificationFact, c []CompatibilityFact, k []CoverageFact, cm []CompletenessFact, ad []AdequacyFact, ce []CertificationFact) (EvaluationFactSnapshot, error) {
	if !a.Valid() {
		return EvaluationFactSnapshot{}, fmt.Errorf("invalid snapshot identity")
	}
	for _, x := range t {
		if !x.Valid() || x.attempt != a {
			return EvaluationFactSnapshot{}, fmt.Errorf("invalid trust snapshot")
		}
	}
	for _, x := range r {
		if !x.Valid() || x.attempt != a {
			return EvaluationFactSnapshot{}, fmt.Errorf("invalid revocation snapshot")
		}
	}
	for _, x := range v {
		if !x.Valid() || x.attempt != a {
			return EvaluationFactSnapshot{}, fmt.Errorf("invalid verification snapshot")
		}
	}
	for _, x := range c {
		if !x.Valid() || x.attempt != a {
			return EvaluationFactSnapshot{}, fmt.Errorf("invalid compatibility snapshot")
		}
	}
	for _, x := range k {
		if !x.Valid() || x.attempt != a {
			return EvaluationFactSnapshot{}, fmt.Errorf("invalid coverage snapshot")
		}
	}
	for _, x := range cm {
		if !x.Valid() || x.attempt != a {
			return EvaluationFactSnapshot{}, fmt.Errorf("invalid completeness snapshot")
		}
	}
	for _, x := range ad {
		if !x.Valid() || x.attempt != a {
			return EvaluationFactSnapshot{}, fmt.Errorf("invalid adequacy snapshot")
		}
	}
	for _, x := range ce {
		if !x.Valid() || x.attempt != a {
			return EvaluationFactSnapshot{}, fmt.Errorf("invalid certification snapshot")
		}
	}
	c, err := dedupCompatibilityFacts(c)
	if err != nil {
		return EvaluationFactSnapshot{}, err
	}
	if t, err = dedupTrustFacts(t); err != nil {
		return EvaluationFactSnapshot{}, err
	}
	if r, err = dedupRevocationFacts(r); err != nil {
		return EvaluationFactSnapshot{}, err
	}
	if v, err = dedupVerificationFacts(v); err != nil {
		return EvaluationFactSnapshot{}, err
	}
	if k, err = dedupCoverageFacts(k); err != nil {
		return EvaluationFactSnapshot{}, err
	}
	if cm, err = dedupCompletenessFacts(cm); err != nil {
		return EvaluationFactSnapshot{}, err
	}
	if ad, err = dedupAdequacyFacts(ad); err != nil {
		return EvaluationFactSnapshot{}, err
	}
	if ce, err = dedupCertificationFacts(ce); err != nil {
		return EvaluationFactSnapshot{}, err
	}
	return EvaluationFactSnapshot{a, append([]TrustFact(nil), t...), append([]CurrentRevocationFact(nil), r...), append([]ResolvedVerificationFact(nil), v...), c, append([]CoverageFact(nil), k...), append([]CompletenessFact(nil), cm...), append([]AdequacyFact(nil), ad...), append([]CertificationFact(nil), ce...)}, nil
}

func dedupTrustFacts(in []TrustFact) ([]TrustFact, error) {
	seen := map[string]TrustFact{}
	out := []TrustFact{}
	for _, f := range in {
		k := strings.Join([]string{string(f.attempt), f.subject, f.policy, f.root, scopeIdentity(f.scope), fmt.Sprintf("%#v", f.context), fmt.Sprintf("%#v", f.validity), fmt.Sprintf("%#v", f.revocation), fmt.Sprintf("%#v", f.provenance)}, "\x00")
		if o, ok := seen[k]; ok {
			if o.state != f.state {
				return nil, fmt.Errorf("conflicting trust facts")
			}
			continue
		}
		seen[k] = f
		out = append(out, f)
	}
	return out, nil
}
func dedupRevocationFacts(in []CurrentRevocationFact) ([]CurrentRevocationFact, error) {
	seen := map[string]CurrentRevocationFact{}
	out := []CurrentRevocationFact{}
	for _, f := range in {
		k := strings.Join([]string{string(f.attempt), f.subject, f.source, fmt.Sprintf("%#v", f.validUntil), fmt.Sprintf("%#v", f.provenance)}, "\x00")
		if o, ok := seen[k]; ok {
			if o.state != f.state {
				return nil, fmt.Errorf("conflicting revocation facts")
			}
			continue
		}
		seen[k] = f
		out = append(out, f)
	}
	return out, nil
}
func dedupVerificationFacts(in []ResolvedVerificationFact) ([]ResolvedVerificationFact, error) {
	seen := map[string]ResolvedVerificationFact{}
	out := []ResolvedVerificationFact{}
	for _, f := range in {
		k := strings.Join([]string{string(f.attempt), f.subject, f.verifier, f.property, scopeIdentity(f.scope), fmt.Sprintf("%#v", f.context), fmt.Sprintf("%#v", f.validity), fmt.Sprintf("%#v", f.revocation), fmt.Sprintf("%#v", f.provenance)}, "\x00")
		if o, ok := seen[k]; ok {
			if o.state != f.state {
				return nil, fmt.Errorf("conflicting verification facts")
			}
			continue
		}
		seen[k] = f
		out = append(out, f)
	}
	return out, nil
}
func dedupCoverageFacts(in []CoverageFact) ([]CoverageFact, error) {
	seen := map[string]CoverageFact{}
	out := []CoverageFact{}
	for _, f := range in {
		k := strings.Join([]string{string(f.attempt), f.subject, f.backend, f.source, scopeIdentity(f.scope), fmt.Sprintf("%#v", f.context), fmt.Sprintf("%#v", f.validity), fmt.Sprintf("%#v", f.revocation)}, "\x00")
		if o, ok := seen[k]; ok {
			if o.state != f.state {
				return nil, fmt.Errorf("conflicting coverage facts")
			}
			continue
		}
		seen[k] = f
		out = append(out, f)
	}
	return out, nil
}
func dedupCompletenessFacts(in []CompletenessFact) ([]CompletenessFact, error) {
	seen := map[string]CompletenessFact{}
	out := []CompletenessFact{}
	for _, f := range in {
		k := strings.Join([]string{string(f.attempt), f.subject, fmt.Sprintf("%#v", f.class), scopeIdentity(f.scope), fmt.Sprintf("%#v", f.validity), fmt.Sprintf("%#v", f.revocation), fmt.Sprintf("%#v", f.provenance)}, "\x00")
		if o, ok := seen[k]; ok {
			if o.class != f.class {
				return nil, fmt.Errorf("conflicting completeness facts")
			}
			continue
		}
		seen[k] = f
		out = append(out, f)
	}
	return out, nil
}
func dedupAdequacyFacts(in []AdequacyFact) ([]AdequacyFact, error) {
	seen := map[string]AdequacyFact{}
	out := []AdequacyFact{}
	for _, f := range in {
		k := strings.Join([]string{string(f.attempt), f.subject, fmt.Sprintf("%#v", f.class), scopeIdentity(f.scope), fmt.Sprintf("%#v", f.context), fmt.Sprintf("%#v", f.verifier), fmt.Sprintf("%#v", f.validity), fmt.Sprintf("%#v", f.revocation), fmt.Sprintf("%#v", f.provenance)}, "\x00")
		if o, ok := seen[k]; ok {
			if o.class != f.class {
				return nil, fmt.Errorf("conflicting adequacy facts")
			}
			continue
		}
		seen[k] = f
		out = append(out, f)
	}
	return out, nil
}
func dedupCertificationFacts(in []CertificationFact) ([]CertificationFact, error) {
	seen := map[string]CertificationFact{}
	out := []CertificationFact{}
	for _, f := range in {
		k := strings.Join([]string{string(f.attempt), f.subject, f.identity, f.property, scopeIdentity(f.scope), fmt.Sprintf("%#v", f.context), fmt.Sprintf("%#v", f.verifier), fmt.Sprintf("%#v", f.validity), fmt.Sprintf("%#v", f.revocation), fmt.Sprintf("%#v", f.provenance)}, "\x00")
		if o, ok := seen[k]; ok {
			if fmt.Sprintf("%#v", o) != fmt.Sprintf("%#v", f) {
				return nil, fmt.Errorf("conflicting certification facts")
			}
			continue
		}
		seen[k] = f
		out = append(out, f)
	}
	return out, nil
}

func scopeIdentity(s Scope) string {
	d := s.Dimensions()
	xs := make([]string, 0, len(d))
	for _, x := range d {
		xs = append(xs, string(x.Dimension)+"="+fmt.Sprint(x.State))
	}
	sort.Strings(xs)
	return strings.Join(append(xs, s.Target(), s.Context()), "|")
}
func compatibilityIdentity(f CompatibilityFact) string {
	return strings.Join([]string{string(f.attempt), f.schema, f.predicate, f.field, f.candidate, f.baseline, f.requirement, f.authority, f.subject, f.backend, scopeIdentity(f.scope), f.context, fmt.Sprintf("%#v", f.validity), fmt.Sprintf("%#v", f.provenance), fmt.Sprintf("%#v", f.revocation)}, "\x00")
}
func dedupCompatibilityFacts(in []CompatibilityFact) ([]CompatibilityFact, error) {
	out := make([]CompatibilityFact, 0, len(in))
	seen := map[string]CompatibilityFact{}
	for _, f := range in {
		k := compatibilityIdentity(f)
		if old, ok := seen[k]; ok {
			if old.state != f.state || fmt.Sprintf("%#v", old) != fmt.Sprintf("%#v", f) {
				return nil, fmt.Errorf("conflicting compatibility facts")
			}
			continue
		}
		seen[k] = f
		out = append(out, f)
	}
	return out, nil
}
