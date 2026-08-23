// Package authority contains backend-neutral RFC-0003 authority-domain
// values. It has no persistence, resolver, clock, or backend dependencies.
package authority

import (
	"fmt"
	"regexp"
)

// Digest is a lexically validated digest value. P0 does not calculate or
// verify digests; that is the responsibility of the canonical-binding phase.
type Digest string

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func NewDigest(value string) (Digest, error) {
	if !digestPattern.MatchString(value) {
		return "", fmt.Errorf("invalid digest lexical form")
	}
	return Digest(value), nil
}

func (d Digest) Valid() bool    { return digestPattern.MatchString(string(d)) }
func (d Digest) String() string { return string(d) }

type referenceData struct {
	id, version string
	digest      Digest
}

func newReference(id, version string, digest Digest) (referenceData, error) {
	if id == "" || version == "" || !digest.Valid() {
		return referenceData{}, fmt.Errorf("reference requires non-empty id, version, and valid digest")
	}
	return referenceData{id: id, version: version, digest: digest}, nil
}

type AuthorityRuleRef struct{ referenceData }
type TrustPolicyRef struct{ referenceData }
type BaselineRef struct{ referenceData }
type RegistryRef struct{ referenceData }
type EvidenceRef struct{ referenceData }
type VerifierRef struct{ referenceData }
type CertificationRef struct{ referenceData }
type CompatibilityRuleRef struct{ referenceData }
type CompositionOperatorRef struct{ referenceData }

func newTyped[T any](id, version string, digest Digest, makeRef func(referenceData) T) (T, error) {
	d, err := newReference(id, version, digest)
	if err != nil {
		var zero T
		return zero, err
	}
	return makeRef(d), nil
}

func NewAuthorityRuleRef(id, version string, digest Digest) (AuthorityRuleRef, error) {
	return newTyped(id, version, digest, func(d referenceData) AuthorityRuleRef { return AuthorityRuleRef{d} })
}
func NewTrustPolicyRef(id, version string, digest Digest) (TrustPolicyRef, error) {
	return newTyped(id, version, digest, func(d referenceData) TrustPolicyRef { return TrustPolicyRef{d} })
}
func NewBaselineRef(id, version string, digest Digest) (BaselineRef, error) {
	return newTyped(id, version, digest, func(d referenceData) BaselineRef { return BaselineRef{d} })
}
func NewRegistryRef(id, version string, digest Digest) (RegistryRef, error) {
	return newTyped(id, version, digest, func(d referenceData) RegistryRef { return RegistryRef{d} })
}
func NewEvidenceRef(id, version string, digest Digest) (EvidenceRef, error) {
	return newTyped(id, version, digest, func(d referenceData) EvidenceRef { return EvidenceRef{d} })
}
func NewVerifierRef(id, version string, digest Digest) (VerifierRef, error) {
	return newTyped(id, version, digest, func(d referenceData) VerifierRef { return VerifierRef{d} })
}
func NewCertificationRef(id, version string, digest Digest) (CertificationRef, error) {
	return newTyped(id, version, digest, func(d referenceData) CertificationRef { return CertificationRef{d} })
}
func NewCompatibilityRuleRef(id, version string, digest Digest) (CompatibilityRuleRef, error) {
	return newTyped(id, version, digest, func(d referenceData) CompatibilityRuleRef { return CompatibilityRuleRef{d} })
}
func NewCompositionOperatorRef(id, version string, digest Digest) (CompositionOperatorRef, error) {
	return newTyped(id, version, digest, func(d referenceData) CompositionOperatorRef { return CompositionOperatorRef{d} })
}

func (r referenceData) ID() string      { return r.id }
func (r referenceData) Version() string { return r.version }
func (r referenceData) Digest() Digest  { return r.digest }

type RegistryBinding struct{ registry RegistryRef }

func NewRegistryBinding(registry RegistryRef) (RegistryBinding, error) {
	if !registry.Digest().Valid() {
		return RegistryBinding{}, fmt.Errorf("invalid registry binding")
	}
	return RegistryBinding{registry: registry}, nil
}

func (b RegistryBinding) Registry() RegistryRef { return b.registry }
