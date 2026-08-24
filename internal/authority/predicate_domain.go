package authority

import (
	"fmt"
	"time"
)

// Scope is an explicit, dimensional execution scope; it is never a boolean.
type ScopeDimension string

const (
	ScopeTemporal        ScopeDimension = "temporal"
	ScopeStartup         ScopeDimension = "startup-lifecycle"
	ScopeProcessTree     ScopeDimension = "process-tree"
	ScopeExecutableSet   ScopeDimension = "executable-set"
	ScopeWorkload        ScopeDimension = "workload-container"
	ScopeWorkloadState   ScopeDimension = "workload-state"
	ScopeRunHistory      ScopeDimension = "run-history"
	ScopeArchitectureABI ScopeDimension = "architecture-abi"
	ScopeImageContext    ScopeDimension = "image-context"
)

var validScopeDimensions = map[ScopeDimension]struct{}{
	ScopeTemporal: {}, ScopeStartup: {}, ScopeProcessTree: {}, ScopeExecutableSet: {},
	ScopeWorkload: {}, ScopeWorkloadState: {}, ScopeRunHistory: {}, ScopeArchitectureABI: {}, ScopeImageContext: {},
}

func ParsePredicateScopeDimension(s string) (ScopeDimension, error) {
	d := ScopeDimension(s)
	if _, ok := validScopeDimensions[d]; !ok {
		return "", fmt.Errorf("invalid scope dimension %q", s)
	}
	return d, nil
}

type ScopeCoverageState uint8

const (
	ScopeCoverageInvalid ScopeCoverageState = iota
	ScopeCovers
	ScopeDoesNotCover
	ScopeCoverageUnknown
)

func (s ScopeCoverageState) Valid() bool {
	return s == ScopeCovers || s == ScopeDoesNotCover || s == ScopeCoverageUnknown
}

type ScopeDimensionResult struct {
	Dimension ScopeDimension
	State     ScopeCoverageState
}
type Scope struct {
	dimensions      []ScopeDimensionResult
	target, context string
}

func NewScope(dimensions []ScopeDimensionResult, target, context string) (Scope, error) {
	if len(dimensions) == 0 || target == "" || context == "" {
		return Scope{}, fmt.Errorf("invalid scope")
	}
	seen := map[ScopeDimension]bool{}
	cp := append([]ScopeDimensionResult(nil), dimensions...)
	for _, d := range cp {
		if _, err := ParsePredicateScopeDimension(string(d.Dimension)); err != nil || !d.State.Valid() || seen[d.Dimension] {
			return Scope{}, fmt.Errorf("invalid scope dimension")
		}
		seen[d.Dimension] = true
	}
	return Scope{dimensions: cp, target: target, context: context}, nil
}
func (s Scope) Dimensions() []ScopeDimensionResult {
	return append([]ScopeDimensionResult(nil), s.dimensions...)
}
func (s Scope) Target() string  { return s.target }
func (s Scope) Context() string { return s.context }

type CoverageClass uint8

const (
	CoverageInvalid CoverageClass = iota
	CoverageMeasured
	CoverageAssumed
	CoverageExternallyCertified
	CoverageUnknown
)

func (c CoverageClass) Valid() bool {
	return c == CoverageMeasured || c == CoverageAssumed || c == CoverageExternallyCertified || c == CoverageUnknown
}

type CoverageRecord struct {
	evidence                   []EvidenceRef
	evidenceScope, targetScope Scope
	class                      CoverageClass
	method, assumptions        string
}

func NewCoverageRecord(e []EvidenceRef, es, ts Scope, class CoverageClass, method, assumptions string) (CoverageRecord, error) {
	if len(e) == 0 || !es.Valid() || !ts.Valid() || !class.Valid() || method == "" {
		return CoverageRecord{}, fmt.Errorf("invalid coverage record")
	}
	return CoverageRecord{evidence: append([]EvidenceRef(nil), e...), evidenceScope: es, targetScope: ts, class: class, method: method, assumptions: assumptions}, nil
}

func (s Scope) Valid() bool { return len(s.dimensions) > 0 && s.target != "" && s.context != "" }
func validPredicateValidity(v Validity) bool {
	if v.ObservedAt().IsZero() {
		return false
	}
	if until := v.ValidUntil(); until != nil && until.Before(v.ObservedAt()) {
		return false
	}
	return true
}
func validRevocation(s RevocationStatus) bool {
	return s == RevocationUnknown || s == RevocationNotRevoked || s == RevocationRevoked
}
func validContext(c SecurityContextIdentity) bool {
	_, err := NewSecurityContextIdentity(c)
	return err == nil
}
func validVerifier(v VerifierSemanticIdentity) bool {
	_, err := NewVerifierSemanticIdentity(v)
	return err == nil
}
func (c CoverageRecord) Evidence() []EvidenceRef { return append([]EvidenceRef(nil), c.evidence...) }
func (c CoverageRecord) Class() CoverageClass    { return c.class }
func (c CoverageRecord) EvidenceScope() Scope    { return c.evidenceScope }
func (c CoverageRecord) TargetScope() Scope      { return c.targetScope }
func (c CoverageRecord) Method() string          { return c.method }

type CompletenessClass uint8

const (
	CompletenessInvalid CompletenessClass = iota
	EmpiricalCompleteness
	StructuralCompleteness
	DeclaredCompleteness
	ExternallyCertifiedCompleteness
)

func (c CompletenessClass) Valid() bool {
	return c == EmpiricalCompleteness || c == StructuralCompleteness || c == DeclaredCompleteness || c == ExternallyCertifiedCompleteness
}

type CompletenessRecord struct {
	class                         CompletenessClass
	scope                         Scope
	issuer, identity, assumptions string
	validity                      Validity
	revocation                    RevocationStatus
}

func NewCompletenessRecord(class CompletenessClass, scope Scope, issuer, identity, assumptions string, validity Validity, revocation RevocationStatus) (CompletenessRecord, error) {
	if !class.Valid() || issuer == "" || identity == "" || !scope.Valid() || !validPredicateValidity(validity) || !validRevocation(revocation) {
		return CompletenessRecord{}, fmt.Errorf("invalid completeness")
	}
	return CompletenessRecord{class: class, scope: scope, issuer: issuer, identity: identity, assumptions: assumptions, validity: validity, revocation: revocation}, nil
}
func (c CompletenessRecord) Class() CompletenessClass     { return c.class }
func (c CompletenessRecord) Scope() Scope                 { return c.scope }
func (c CompletenessRecord) Revocation() RevocationStatus { return c.revocation }

type AdequacyClass uint8

const (
	AdequacyInvalid AdequacyClass = iota
	StructuralBaseline
	ExternalCertification
	BackendFormalInvariant
	BoundedBehavioral
	TrustedBaselineObservedDelta
)

func (a AdequacyClass) Valid() bool {
	return a == StructuralBaseline || a == ExternalCertification || a == BackendFormalInvariant || a == BoundedBehavioral || a == TrustedBaselineObservedDelta
}

type AdequacyEvidence struct {
	class                     AdequacyClass
	identity, issuer, backend string
	scope                     Scope
	context                   SecurityContextIdentity
	verifier                  VerifierSemanticIdentity
	validity                  Validity
	revocation                RevocationStatus
	provenance                ProvenanceRecord
}

func NewAdequacyEvidence(class AdequacyClass, identity, issuer, backend string, scope Scope, context SecurityContextIdentity, verifier VerifierSemanticIdentity, validity Validity, revocation RevocationStatus, provenance ProvenanceRecord) (AdequacyEvidence, error) {
	if !class.Valid() || identity == "" || issuer == "" || backend == "" || !scope.Valid() || !validContext(context) || !validVerifier(verifier) || !validPredicateValidity(validity) || !validRevocation(revocation) || !provenance.Valid() {
		return AdequacyEvidence{}, fmt.Errorf("invalid adequacy evidence")
	}
	return AdequacyEvidence{class: class, identity: identity, issuer: issuer, backend: backend, scope: scope, context: context, verifier: verifier, validity: validity, revocation: revocation, provenance: provenance}, nil
}
func (a AdequacyEvidence) Class() AdequacyClass         { return a.class }
func (a AdequacyEvidence) Revocation() RevocationStatus { return a.revocation }

type CertificationProperty uint8

const (
	CertificationPropertyInvalid CertificationProperty = iota
	CertificationScopeCoverage
	CertificationBaselineCompatibility
	CertificationPolicyAdequacyBounded
	CertificationProvenanceValidity
)

func (p CertificationProperty) Valid() bool {
	return p == CertificationScopeCoverage || p == CertificationBaselineCompatibility || p == CertificationPolicyAdequacyBounded || p == CertificationProvenanceValidity
}

type Certification struct {
	identity, version string
	digest            Digest
	issuer            string
	property          CertificationProperty
	backend           string
	scope             Scope
	context           SecurityContextIdentity
	verifier          VerifierSemanticIdentity
	validity          Validity
	revocation        RevocationStatus
}

func NewCertification(identity, version string, digest Digest, issuer string, p CertificationProperty, backend string, scope Scope, context SecurityContextIdentity, verifier VerifierSemanticIdentity, validity Validity, revocation RevocationStatus) (Certification, error) {
	if identity == "" || version == "" || !digest.Valid() || issuer == "" || !p.Valid() || backend == "" || !scope.Valid() || !validContext(context) || !validVerifier(verifier) || !validPredicateValidity(validity) || !validRevocation(revocation) {
		return Certification{}, fmt.Errorf("invalid certification")
	}
	return Certification{identity: identity, version: version, digest: digest, issuer: issuer, property: p, backend: backend, scope: scope, context: context, verifier: verifier, validity: validity, revocation: revocation}, nil
}
func (c Certification) Property() CertificationProperty { return c.property }
func (c Certification) Revocation() RevocationStatus    { return c.revocation }

type VerificationResult uint8

const (
	VerificationInvalid VerificationResult = iota
	VerificationVerified
	VerificationFailed
	VerificationUnknown
)

func (r VerificationResult) Valid() bool {
	return r == VerificationVerified || r == VerificationFailed || r == VerificationUnknown
}

type VerificationFact struct {
	verifier                                              VerifierSemanticIdentity
	property, subject, backend, inputIdentity, provenance string
	scope                                                 Scope
	context                                               SecurityContextIdentity
	result                                                VerificationResult
	validity                                              Validity
	revocation                                            RevocationStatus
}

func NewVerificationFact(v VerifierSemanticIdentity, property, subject, backend, input, provenance string, scope Scope, context SecurityContextIdentity, result VerificationResult, validity Validity, revocation RevocationStatus) (VerificationFact, error) {
	if property == "" || subject == "" || backend == "" || input == "" || provenance == "" || !result.Valid() || !scope.Valid() || !validContext(context) || !validVerifier(v) || !validPredicateValidity(validity) || !validRevocation(revocation) {
		return VerificationFact{}, fmt.Errorf("invalid verification fact")
	}
	return VerificationFact{verifier: v, property: property, subject: subject, backend: backend, inputIdentity: input, provenance: provenance, scope: scope, context: context, result: result, validity: validity, revocation: revocation}, nil
}
func (v VerificationFact) Result() VerificationResult   { return v.result }
func (v VerificationFact) Subject() string              { return v.subject }
func (v VerificationFact) Revocation() RevocationStatus { return v.revocation }

type Baseline struct {
	reference                                                                     BaselineRef
	owner, backend, architecture, abi, kernelRuntime, image, workload, provenance string
	scope                                                                         Scope
	compatibility                                                                 CompatibilityRuleRef
	validity                                                                      Validity
	revocation                                                                    RevocationStatus
}

func NewBaseline(ref BaselineRef, owner, backend string, scope Scope, compat CompatibilityRuleRef, validity Validity, revocation RevocationStatus) (Baseline, error) {
	if owner == "" || backend == "" || !ref.Digest().Valid() || !scope.Valid() || !compat.Digest().Valid() || !validPredicateValidity(validity) || !validRevocation(revocation) {
		return Baseline{}, fmt.Errorf("invalid baseline")
	}
	return Baseline{reference: ref, owner: owner, backend: backend, scope: scope, compatibility: compat, validity: validity, revocation: revocation}, nil
}

type CompositionOperation uint8

const (
	CompositionInvalid CompositionOperation = iota
	RequireEqual
	SetUnionIfIdenticalAction
	SetIntersection
	RejectOnConflict
	PreserveBaseline
)

func (o CompositionOperation) Valid() bool {
	return o == RequireEqual || o == SetUnionIfIdenticalAction || o == SetIntersection || o == RejectOnConflict || o == PreserveBaseline
}

type Composition struct {
	operator  CompositionOperatorRef
	operation CompositionOperation
	inputs    []Digest
}

func NewComposition(op CompositionOperatorRef, operation CompositionOperation, inputs []Digest) (Composition, error) {
	if !op.Digest().Valid() || !operation.Valid() || len(inputs) == 0 {
		return Composition{}, fmt.Errorf("invalid composition")
	}
	for _, d := range inputs {
		if !d.Valid() {
			return Composition{}, fmt.Errorf("invalid composition input")
		}
	}
	return Composition{operator: op, operation: operation, inputs: append([]Digest(nil), inputs...)}, nil
}

type ProvenanceRecord struct {
	producer, mechanism, version, run string
	scope                             Scope
	validity                          Validity
	revocation                        RevocationStatus
	verification                      VerifierSemanticIdentity
}

func NewProvenanceRecord(producer, mechanism, version, run string, scope Scope, validity Validity, revocation RevocationStatus, verification VerifierSemanticIdentity) (ProvenanceRecord, error) {
	if producer == "" || mechanism == "" || version == "" || run == "" || !scope.Valid() || !validPredicateValidity(validity) || !validRevocation(revocation) || !validVerifier(verification) {
		return ProvenanceRecord{}, fmt.Errorf("invalid provenance")
	}
	return ProvenanceRecord{producer: producer, mechanism: mechanism, version: version, run: run, scope: scope, validity: validity, revocation: revocation, verification: verification}, nil
}

func (p ProvenanceRecord) Valid() bool {
	return p.producer != "" && p.mechanism != "" && p.version != "" && p.run != "" && p.scope.Valid() && validPredicateValidity(p.validity) && validRevocation(p.revocation) && validVerifier(p.verification)
}

type TrustState uint8

const (
	TrustStateInvalid TrustState = iota
	Trusted
	Untrusted
	TrustUnknown
)

func (s TrustState) Valid() bool { return s == Trusted || s == Untrusted || s == TrustUnknown }

type TrustAuthorization struct {
	issuer, artifactClass, backend string
	properties                     []string
	scope                          Scope
	context                        SecurityContextIdentity
	validity                       Validity
	revocation                     RevocationStatus
	state                          TrustState
}

func NewTrustAuthorization(issuer, artifactClass, backend string, properties []string, scope Scope, context SecurityContextIdentity, validity Validity, revocation RevocationStatus, state TrustState) (TrustAuthorization, error) {
	if state == Trusted {
		return TrustAuthorization{}, fmt.Errorf("trusted state requires controlled authority source")
	}
	return newTrustAuthorization(issuer, artifactClass, backend, properties, scope, context, validity, revocation, state)
}
func newTrustAuthorization(issuer, artifactClass, backend string, properties []string, scope Scope, context SecurityContextIdentity, validity Validity, revocation RevocationStatus, state TrustState) (TrustAuthorization, error) {
	if issuer == "" || artifactClass == "" || backend == "" || len(properties) == 0 || !scope.Valid() || !validContext(context) || !validPredicateValidity(validity) || !validRevocation(revocation) || !state.Valid() {
		return TrustAuthorization{}, fmt.Errorf("invalid trust authorization")
	}
	return TrustAuthorization{issuer: issuer, artifactClass: artifactClass, backend: backend, properties: append([]string(nil), properties...), scope: scope, context: context, validity: validity, revocation: revocation, state: state}, nil
}
func (t TrustAuthorization) State() TrustState    { return t.state }
func (t TrustAuthorization) Properties() []string { return append([]string(nil), t.properties...) }

type CompatibilityPredicate uint8

const (
	CompatibilityInvalid CompatibilityPredicate = iota
	CompatibilityExactEquality
	CompatibilitySetMembership
	CompatibilityExactVersion
	CompatibilityVersionRange
	CompatibilityArchitectureABI
	CompatibilityDigestEquality
)

func (p CompatibilityPredicate) Valid() bool {
	return p == CompatibilityExactEquality || p == CompatibilitySetMembership || p == CompatibilityExactVersion || p == CompatibilityVersionRange || p == CompatibilityArchitectureABI || p == CompatibilityDigestEquality
}

type CompatibilityRule struct {
	reference       CompatibilityRuleRef
	predicate       CompatibilityPredicate
	field, expected string
}

func NewCompatibilityRule(ref CompatibilityRuleRef, p CompatibilityPredicate, field, expected string) (CompatibilityRule, error) {
	if !ref.Digest().Valid() || !p.Valid() || field == "" || expected == "" {
		return CompatibilityRule{}, fmt.Errorf("invalid compatibility rule")
	}
	return CompatibilityRule{reference: ref, predicate: p, field: field, expected: expected}, nil
}

type TemporalValidity struct {
	validity Validity
	state    Presence
}

func NewTemporalValidity(v Validity, state Presence) (TemporalValidity, error) {
	if state == PresenceInvalid || (state != PresenceAbsent && !validPredicateValidity(v)) {
		return TemporalValidity{}, fmt.Errorf("invalid temporal validity")
	}
	return TemporalValidity{validity: v, state: state}, nil
}
func (t TemporalValidity) Validity() Validity { return t.validity }
func (t TemporalValidity) State() Presence    { return t.state }

type RevocationFact struct {
	subject, source, epoch string
	status                 RevocationStatus
	observedAt             time.Time
}

func NewRevocationFact(subject, source, epoch string, status RevocationStatus, observedAt time.Time) (RevocationFact, error) {
	if subject == "" || source == "" || epoch == "" || !status.Valid() || observedAt.IsZero() {
		return RevocationFact{}, fmt.Errorf("invalid revocation fact")
	}
	return RevocationFact{subject: subject, source: source, epoch: epoch, status: status, observedAt: observedAt}, nil
}
func (r RevocationFact) Status() RevocationStatus { return r.status }
