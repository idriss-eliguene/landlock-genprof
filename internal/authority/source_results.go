package authority

import (
	"fmt"
	"time"
)

type TemporalApplicability uint8

const (
	TemporalInvalid TemporalApplicability = iota
	TemporalApplicable
	TemporalNotYetValid
	TemporalExpired
	TemporalUnknown
)

func CheckValidityAt(v Validity, at time.Time) TemporalApplicability {
	if at.IsZero() || !validPredicateValidity(v) {
		return TemporalInvalid
	}
	if at.Before(v.ObservedAt()) {
		return TemporalNotYetValid
	}
	if u := v.ValidUntil(); u != nil && at.After(*u) {
		return TemporalExpired
	}
	if v.MaxAge() > 0 && at.After(v.ObservedAt().Add(v.MaxAge())) {
		return TemporalExpired
	}
	return TemporalApplicable
}

// Family-specific source results are intentionally distinct. Their
// constructors are package-private: ordinary callers can validate facts but
// cannot select a positive authority result.
type TrustResolutionResult struct {
	attempt               ResolutionAttemptIdentity
	subject, policy, root string
	scope                 Scope
	context               SecurityContextIdentity
	validity              Validity
	revocation            CurrentRevocationFact
	provenance            ProvenanceRecord
	state                 TrustState
}

func newTrustResolutionResult(a ResolutionAttemptIdentity, subject, policy, root string, scope Scope, ctx SecurityContextIdentity, v Validity, r CurrentRevocationFact, p ProvenanceRecord, state TrustState) (TrustResolutionResult, error) {
	if !a.Valid() || subject == "" || policy == "" || root == "" || !scope.Valid() || !validContext(ctx) || !validPredicateValidity(v) || !r.Valid() || r.attempt != a || !p.Valid() || !state.Valid() {
		return TrustResolutionResult{}, fmt.Errorf("invalid trust source result")
	}
	return TrustResolutionResult{a, subject, policy, root, scope, ctx, v, r, p, state}, nil
}
func DeriveTrustFact(r TrustResolutionResult) (TrustFact, error) {
	if !r.Valid() {
		return TrustFact{}, fmt.Errorf("invalid trust source result")
	}
	return TrustFact{r.attempt, r.subject, r.policy, r.root, r.scope, r.context, r.validity, r.revocation, r.provenance, r.state}, nil
}
func DeriveTrustFactAt(r TrustResolutionResult, at time.Time) (TrustFact, error) {
	if CheckValidityAt(r.validity, at) != TemporalApplicable {
		return TrustFact{}, fmt.Errorf("trust result not applicable")
	}
	if r.revocation.state != RevocationNotRevoked {
		return TrustFact{}, fmt.Errorf("revocation not admissible")
	}
	return DeriveTrustFact(r)
}
func (r TrustResolutionResult) Valid() bool {
	return r.attempt.Valid() && r.subject != "" && r.policy != "" && r.root != "" && r.scope.Valid() && validContext(r.context) && validPredicateValidity(r.validity) && r.revocation.Valid() && r.revocation.attempt == r.attempt && r.provenance.Valid() && r.state.Valid()
}

type VerificationExecutionResult struct {
	attempt                     ResolutionAttemptIdentity
	subject, verifier, property string
	scope                       Scope
	context                     SecurityContextIdentity
	validity                    Validity
	revocation                  CurrentRevocationFact
	provenance                  ProvenanceRecord
	state                       VerificationFactState
}

func newVerificationExecutionResult(a ResolutionAttemptIdentity, s, v, p string, sc Scope, c SecurityContextIdentity, iv Validity, r CurrentRevocationFact, pr ProvenanceRecord, state VerificationFactState) (VerificationExecutionResult, error) {
	if !a.Valid() || s == "" || v == "" || p == "" || !sc.Valid() || !validContext(c) || !validPredicateValidity(iv) || !r.Valid() || r.attempt != a || !pr.Valid() || !state.Valid() {
		return VerificationExecutionResult{}, fmt.Errorf("invalid verification source result")
	}
	return VerificationExecutionResult{a, s, v, p, sc, c, iv, r, pr, state}, nil
}
func DeriveVerificationFact(r VerificationExecutionResult) (ResolvedVerificationFact, error) {
	if !r.Valid() {
		return ResolvedVerificationFact{}, fmt.Errorf("invalid verification source result")
	}
	return ResolvedVerificationFact{r.attempt, r.subject, r.verifier, r.property, r.scope, r.context, r.validity, r.revocation, r.provenance, r.state}, nil
}
func DeriveVerificationFactAt(r VerificationExecutionResult, at time.Time) (ResolvedVerificationFact, error) {
	if CheckValidityAt(r.validity, at) != TemporalApplicable || r.revocation.state != RevocationNotRevoked {
		return ResolvedVerificationFact{}, fmt.Errorf("verification result not applicable")
	}
	return DeriveVerificationFact(r)
}
func (r VerificationExecutionResult) Valid() bool {
	return r.attempt.Valid() && r.subject != "" && r.verifier != "" && r.property != "" && r.scope.Valid() && validContext(r.context) && validPredicateValidity(r.validity) && r.revocation.Valid() && r.revocation.attempt == r.attempt && r.provenance.Valid() && r.state.Valid()
}

type CurrentRevocationResult struct {
	attempt         ResolutionAttemptIdentity
	subject, source string
	state           RevocationStatus
	provenance      ProvenanceRecord
	validity        Validity
}

func newCurrentRevocationResult(a ResolutionAttemptIdentity, s, src string, state RevocationStatus, p ProvenanceRecord, v Validity) (CurrentRevocationResult, error) {
	if !a.Valid() || s == "" || src == "" || !validRevocation(state) || !p.Valid() || !validPredicateValidity(v) {
		return CurrentRevocationResult{}, fmt.Errorf("invalid revocation source result")
	}
	return CurrentRevocationResult{a, s, src, state, p, v}, nil
}
func DeriveCurrentRevocationFact(r CurrentRevocationResult) (CurrentRevocationFact, error) {
	if !r.Valid() {
		return CurrentRevocationFact{}, fmt.Errorf("invalid revocation source result")
	}
	return CurrentRevocationFact{r.attempt, r.subject, r.source, r.state, r.provenance, r.validity}, nil
}
func DeriveCurrentRevocationFactAt(r CurrentRevocationResult, at time.Time) (CurrentRevocationFact, error) {
	if CheckValidityAt(r.validity, at) != TemporalApplicable {
		return CurrentRevocationFact{}, fmt.Errorf("revocation result not current")
	}
	return DeriveCurrentRevocationFact(r)
}
func (r CurrentRevocationResult) Valid() bool {
	return r.attempt.Valid() && r.subject != "" && r.source != "" && validRevocation(r.state) && r.provenance.Valid() && validPredicateValidity(r.validity)
}

type CoverageObservationResult struct {
	attempt                  ResolutionAttemptIdentity
	subject, backend, source string
	scope                    Scope
	context                  SecurityContextIdentity
	validity                 Validity
	revocation               CurrentRevocationFact
	state                    ScopeCoverageState
	provenance               ProvenanceRecord
}

func newCoverageObservationResult(a ResolutionAttemptIdentity, s, backend, source string, sc Scope, ctx SecurityContextIdentity, v Validity, r CurrentRevocationFact, state ScopeCoverageState, p ProvenanceRecord) (CoverageObservationResult, error) {
	if !a.Valid() || s == "" || backend == "" || source == "" || !sc.Valid() || !validContext(ctx) || !validPredicateValidity(v) || !r.Valid() || r.attempt != a || !state.Valid() || !p.Valid() {
		return CoverageObservationResult{}, fmt.Errorf("invalid coverage source result")
	}
	return CoverageObservationResult{a, s, backend, source, sc, ctx, v, r, state, p}, nil
}
func DeriveCoverageFact(r CoverageObservationResult) (CoverageFact, error) {
	if !r.Valid() {
		return CoverageFact{}, fmt.Errorf("invalid coverage source result")
	}
	return CoverageFact{r.attempt, r.subject, r.backend, r.source, r.scope, r.context, r.validity, r.revocation, r.state, r.provenance}, nil
}
func (r CoverageObservationResult) Valid() bool {
	return r.attempt.Valid() && r.subject != "" && r.backend != "" && r.source != "" && r.scope.Valid() && validContext(r.context) && validPredicateValidity(r.validity) && r.revocation.Valid() && r.revocation.attempt == r.attempt && r.state.Valid() && r.provenance.Valid()
}

type CompletenessEvidenceResult struct {
	attempt    ResolutionAttemptIdentity
	subject    string
	class      CompletenessClass
	scope      Scope
	provenance ProvenanceRecord
	validity   Validity
	revocation CurrentRevocationFact
}

func newCompletenessEvidenceResult(a ResolutionAttemptIdentity, s string, c CompletenessClass, sc Scope, p ProvenanceRecord, v Validity, r CurrentRevocationFact) (CompletenessEvidenceResult, error) {
	if !a.Valid() || s == "" || !c.Valid() || !sc.Valid() || !p.Valid() || !validPredicateValidity(v) || !r.Valid() || r.attempt != a {
		return CompletenessEvidenceResult{}, fmt.Errorf("invalid completeness source result")
	}
	return CompletenessEvidenceResult{a, s, c, sc, p, v, r}, nil
}
func DeriveCompletenessFact(r CompletenessEvidenceResult) (CompletenessFact, error) {
	if !r.Valid() {
		return CompletenessFact{}, fmt.Errorf("invalid completeness source result")
	}
	return CompletenessFact{r.attempt, r.subject, r.class, r.scope, r.provenance, r.validity, r.revocation}, nil
}
func (r CompletenessEvidenceResult) Valid() bool {
	return r.attempt.Valid() && r.subject != "" && r.class.Valid() && r.scope.Valid() && r.provenance.Valid() && validPredicateValidity(r.validity) && r.revocation.Valid() && r.revocation.attempt == r.attempt
}

type AdequacyEvidenceResult struct {
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

func newAdequacyEvidenceResult(a ResolutionAttemptIdentity, s string, c AdequacyClass, sc Scope, ctx SecurityContextIdentity, vf VerifierSemanticIdentity, p ProvenanceRecord, v Validity, r CurrentRevocationFact) (AdequacyEvidenceResult, error) {
	if !a.Valid() || s == "" || !c.Valid() || !sc.Valid() || !validContext(ctx) || !validVerifier(vf) || !p.Valid() || !validPredicateValidity(v) || !r.Valid() || r.attempt != a {
		return AdequacyEvidenceResult{}, fmt.Errorf("invalid adequacy source result")
	}
	return AdequacyEvidenceResult{a, s, c, sc, ctx, vf, p, v, r}, nil
}
func DeriveAdequacyFact(r AdequacyEvidenceResult) (AdequacyFact, error) {
	if !r.Valid() {
		return AdequacyFact{}, fmt.Errorf("invalid adequacy source result")
	}
	return AdequacyFact{r.attempt, r.subject, r.class, r.scope, r.context, r.verifier, r.provenance, r.validity, r.revocation}, nil
}
func (r AdequacyEvidenceResult) Valid() bool {
	return r.attempt.Valid() && r.subject != "" && r.class.Valid() && r.scope.Valid() && validContext(r.context) && validVerifier(r.verifier) && r.provenance.Valid() && validPredicateValidity(r.validity) && r.revocation.Valid() && r.revocation.attempt == r.attempt
}

type CertificationResolutionResult struct {
	attempt                     ResolutionAttemptIdentity
	subject, identity, property string
	scope                       Scope
	context                     SecurityContextIdentity
	verifier                    VerifierSemanticIdentity
	provenance                  ProvenanceRecord
	validity                    Validity
	revocation                  CurrentRevocationFact
}

func newCertificationResolutionResult(a ResolutionAttemptIdentity, s, id, prop string, sc Scope, ctx SecurityContextIdentity, vf VerifierSemanticIdentity, p ProvenanceRecord, v Validity, r CurrentRevocationFact) (CertificationResolutionResult, error) {
	if !a.Valid() || s == "" || id == "" || prop == "" || !sc.Valid() || !validContext(ctx) || !validVerifier(vf) || !p.Valid() || !validPredicateValidity(v) || !r.Valid() || r.attempt != a {
		return CertificationResolutionResult{}, fmt.Errorf("invalid certification source result")
	}
	return CertificationResolutionResult{a, s, id, prop, sc, ctx, vf, p, v, r}, nil
}
func DeriveCertificationFact(r CertificationResolutionResult) (CertificationFact, error) {
	if !r.Valid() {
		return CertificationFact{}, fmt.Errorf("invalid certification source result")
	}
	return CertificationFact{r.attempt, r.subject, r.identity, r.property, r.scope, r.context, r.verifier, r.provenance, r.validity, r.revocation}, nil
}
func (r CertificationResolutionResult) Valid() bool {
	return r.attempt.Valid() && r.subject != "" && r.identity != "" && r.property != "" && r.scope.Valid() && validContext(r.context) && validVerifier(r.verifier) && r.provenance.Valid() && validPredicateValidity(r.validity) && r.revocation.Valid() && r.revocation.attempt == r.attempt
}
