package authority

import (
	"fmt"
	"time"
)

// AuthorityMetadata is the immutable interpretation snapshot referenced by a
// candidate. It contains references, never resolved implementations.
type AuthorityMetadata struct {
	authorityRule  AuthorityRuleRef
	trustPolicy    TrustPolicyRef
	baseline       BaselineRef
	registries     []RegistryBinding
	evidence       []EvidenceRef
	verifiers      []VerifierRef
	certifications []CertificationRef
	compatibility  CompatibilityRuleRef
	composition    CompositionOperatorRef
	context        SecurityContextIdentity
}

func NewAuthorityMetadata(v AuthorityMetadata) (AuthorityMetadata, error) {
	if !v.authorityRule.Digest().Valid() || !v.trustPolicy.Digest().Valid() ||
		!v.baseline.Digest().Valid() || !v.compatibility.Digest().Valid() ||
		!v.composition.Digest().Valid() {
		return AuthorityMetadata{}, fmt.Errorf("authority metadata has invalid mandatory reference")
	}
	ctx, err := NewSecurityContextIdentity(v.context)
	if err != nil {
		return AuthorityMetadata{}, err
	}
	if len(v.registries) == 0 {
		return AuthorityMetadata{}, fmt.Errorf("authority metadata requires a registry")
	}
	for _, r := range v.registries {
		if !r.Registry().Digest().Valid() {
			return AuthorityMetadata{}, fmt.Errorf("invalid registry binding")
		}
	}
	v.context = ctx
	v.registries = append([]RegistryBinding(nil), v.registries...)
	v.evidence = append([]EvidenceRef(nil), v.evidence...)
	v.verifiers = append([]VerifierRef(nil), v.verifiers...)
	v.certifications = append([]CertificationRef(nil), v.certifications...)
	return v, nil
}

func (m AuthorityMetadata) AuthorityRule() AuthorityRuleRef     { return m.authorityRule }
func (m AuthorityMetadata) TrustPolicy() TrustPolicyRef         { return m.trustPolicy }
func (m AuthorityMetadata) Baseline() BaselineRef               { return m.baseline }
func (m AuthorityMetadata) Compatibility() CompatibilityRuleRef { return m.compatibility }
func (m AuthorityMetadata) Composition() CompositionOperatorRef { return m.composition }
func (m AuthorityMetadata) Context() SecurityContextIdentity    { return m.context }
func (m AuthorityMetadata) Registries() []RegistryBinding {
	return append([]RegistryBinding(nil), m.registries...)
}
func (m AuthorityMetadata) Evidence() []EvidenceRef { return append([]EvidenceRef(nil), m.evidence...) }
func (m AuthorityMetadata) Verifiers() []VerifierRef {
	return append([]VerifierRef(nil), m.verifiers...)
}
func (m AuthorityMetadata) Certifications() []CertificationRef {
	return append([]CertificationRef(nil), m.certifications...)
}

type Validity struct {
	observedAt time.Time
	validUntil *time.Time
	maxAge     time.Duration
}

func NewValidity(observedAt time.Time, validUntil *time.Time, maxAge time.Duration) (Validity, error) {
	if observedAt.IsZero() || maxAge < 0 || (validUntil != nil && validUntil.IsZero()) {
		return Validity{}, fmt.Errorf("invalid validity values")
	}
	var until *time.Time
	if validUntil != nil {
		t := *validUntil
		until = &t
	}
	return Validity{observedAt: observedAt, validUntil: until, maxAge: maxAge}, nil
}

func (v Validity) ObservedAt() time.Time { return v.observedAt }
func (v Validity) ValidUntil() *time.Time {
	if v.validUntil == nil {
		return nil
	}
	t := *v.validUntil
	return &t
}
func (v Validity) MaxAge() time.Duration { return v.maxAge }

type EligibilityRecord struct {
	candidate       Digest
	metadata        AuthorityMetadata
	result          EligibilityResult
	validity        Validity
	revocation      RevocationStatus
	evaluationEpoch uint64
}

func NewEligibilityRecord(v EligibilityRecord) (EligibilityRecord, error) {
	if !v.candidate.Valid() || !v.result.Valid() || !v.revocation.Valid() || v.evaluationEpoch == 0 {
		return EligibilityRecord{}, fmt.Errorf("invalid eligibility record foundation")
	}
	return v, nil
}

func (r EligibilityRecord) Candidate() Digest            { return r.candidate }
func (r EligibilityRecord) Metadata() AuthorityMetadata  { return r.metadata }
func (r EligibilityRecord) Result() EligibilityResult    { return r.result }
func (r EligibilityRecord) Validity() Validity           { return r.validity }
func (r EligibilityRecord) Revocation() RevocationStatus { return r.revocation }
func (r EligibilityRecord) EvaluationEpoch() uint64      { return r.evaluationEpoch }
