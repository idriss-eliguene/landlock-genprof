package authority

import (
	"fmt"
	"sort"
	"time"
)

// ObjectKind is the closed RFC-0004 V1 reference-kind vocabulary.
type ObjectKind string
type BackendID string
type EvidenceClass uint8

const (
	BackendSeccomp                          BackendID  = "SECCOMP"
	ObjectKindAuthorityRule                 ObjectKind = "AUTHORITY_RULE"
	ObjectKindTrustPolicy                   ObjectKind = "TRUST_POLICY"
	ObjectKindRegistry                      ObjectKind = "SECURITY_FIELD_REGISTRY"
	ObjectKindCertificationPropertyRegistry ObjectKind = "CERTIFICATION_PROPERTY_REGISTRY"
	ObjectKindArchitectureABIRegistry       ObjectKind = "ARCHITECTURE_ABI_REGISTRY"
	ObjectKindBackendEnvelopeRegistry       ObjectKind = "BACKEND_ENVELOPE_REGISTRY"
	ObjectKindEvidence                      ObjectKind = "EVIDENCE"
	ObjectKindCertification                 ObjectKind = "CERTIFICATION"
	ObjectKindBaseline                      ObjectKind = "BASELINE"
	ObjectKindCompatibilityRule             ObjectKind = "COMPATIBILITY_RULE"
	ObjectKindCompositionOperator           ObjectKind = "COMPOSITION_OPERATOR"
	ObjectKindProvenance                    ObjectKind = "PROVENANCE"
	ObjectKindVerifier                      ObjectKind = "VERIFIER_SEMANTIC"
	ObjectKindRevocation                    ObjectKind = "REVOCATION"
)

var objectKinds = map[ObjectKind]struct{}{
	ObjectKindAuthorityRule: {}, ObjectKindTrustPolicy: {}, ObjectKindRegistry: {},
	ObjectKindCertificationPropertyRegistry: {}, ObjectKindArchitectureABIRegistry: {},
	ObjectKindBackendEnvelopeRegistry: {}, ObjectKindEvidence: {}, ObjectKindCertification: {},
	ObjectKindBaseline: {}, ObjectKindCompatibilityRule: {}, ObjectKindCompositionOperator: {},
	ObjectKindProvenance: {}, ObjectKindVerifier: {}, ObjectKindRevocation: {},
}

func ParseObjectKind(s string) (ObjectKind, error) {
	k := ObjectKind(s)
	if _, ok := objectKinds[k]; !ok {
		return "", fmt.Errorf("invalid object kind %q", s)
	}
	return k, nil
}
func (k ObjectKind) Valid() bool { _, ok := objectKinds[k]; return ok }

// SemanticReference is an immutable RFC-0004 exact reference.
type SemanticReference struct {
	kind    ObjectKind
	id      string
	version SemanticVersion
	digest  Digest
}

func NewSemanticReference(kind ObjectKind, id string, version SemanticVersion, digest Digest) (SemanticReference, error) {
	if !kind.Valid() || id == "" || !digest.Valid() {
		return SemanticReference{}, fmt.Errorf("invalid semantic reference")
	}
	return SemanticReference{kind: kind, id: id, version: version, digest: digest}, nil
}
func (r SemanticReference) Valid() bool              { return r.kind.Valid() && r.id != "" && r.digest.Valid() }
func (r SemanticReference) Kind() ObjectKind         { return r.kind }
func (r SemanticReference) ID() string               { return r.id }
func (r SemanticReference) Version() SemanticVersion { return r.version }
func (r SemanticReference) Digest() Digest           { return r.digest }
func (r SemanticReference) ValidateKind(expected ObjectKind) error {
	if !r.Valid() || r.kind != expected {
		return fmt.Errorf("reference kind mismatch")
	}
	return nil
}

// RuleTargetScope is a closed target-scope requirement. Dimensions are a set.
type TargetScopeClass string

const (
	TargetScopeBinary            TargetScopeClass = "BINARY"
	TargetScopeProcess           TargetScopeClass = "PROCESS"
	TargetScopeProcessTree       TargetScopeClass = "PROCESS_TREE"
	TargetScopeContainer         TargetScopeClass = "CONTAINER"
	TargetScopeWorkload          TargetScopeClass = "WORKLOAD"
	TargetScopeContainerLifetime TargetScopeClass = "CONTAINER_LIFETIME"
)

var targetScopeClasses = map[TargetScopeClass]struct{}{TargetScopeBinary: {}, TargetScopeProcess: {}, TargetScopeProcessTree: {}, TargetScopeContainer: {}, TargetScopeWorkload: {}, TargetScopeContainerLifetime: {}}

type RuleTargetScope struct {
	class      TargetScopeClass
	dimensions []ScopeDimension
}

func NewRuleTargetScope(class TargetScopeClass, dimensions []ScopeDimension) (RuleTargetScope, error) {
	if _, ok := targetScopeClasses[class]; !ok || len(dimensions) == 0 {
		return RuleTargetScope{}, fmt.Errorf("invalid target scope")
	}
	seen := make(map[ScopeDimension]struct{}, len(dimensions))
	cp := append([]ScopeDimension(nil), dimensions...)
	for _, d := range cp {
		if !d.ValidRFC() {
			return RuleTargetScope{}, fmt.Errorf("invalid scope dimension")
		}
		if _, ok := seen[d]; ok {
			return RuleTargetScope{}, fmt.Errorf("duplicate scope dimension")
		}
		seen[d] = struct{}{}
	}
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return RuleTargetScope{class: class, dimensions: cp}, nil
}
func (s RuleTargetScope) Valid() bool {
	_, ok := targetScopeClasses[s.class]
	return ok && len(s.dimensions) > 0
}
func (s RuleTargetScope) Class() TargetScopeClass { return s.class }
func (s RuleTargetScope) Dimensions() []ScopeDimension {
	return append([]ScopeDimension(nil), s.dimensions...)
}

var rfcScopeDimensions = map[ScopeDimension]struct{}{
	ScopeDimension("STARTUP_BOOTSTRAP"): {}, ScopeDimension("CONTAINER_LIFETIME"): {},
	ScopeDimension("PROCESS_TREE"): {}, ScopeDimension("EXECUTABLE_SET"): {},
	ScopeDimension("WORKLOAD_STATE"): {}, ScopeDimension("ARCHITECTURE_ABI"): {},
	ScopeDimension("IMAGE_IDENTITY"): {}, ScopeDimension("KERNEL_RUNTIME_COMPATIBILITY"): {},
	ScopeDimension("TEMPORAL"): {}, ScopeDimension("STARTUP_LIFECYCLE"): {},
	ScopeDimension("WORKLOAD_CONTAINER"): {}, ScopeDimension("RUN_HISTORY"): {},
	ScopeDimension("IMAGE_CONTEXT"): {},
}

func ParseScopeDimension(s string) (ScopeDimension, error) {
	d := ScopeDimension(s)
	if !d.ValidRFC() {
		return "", fmt.Errorf("invalid scope dimension %q", s)
	}
	return d, nil
}
func (d ScopeDimension) ValidRFC() bool    { _, ok := rfcScopeDimensions[d]; return ok }
func (d ScopeDimension) RFCString() string { return string(d) }

func ParseEvidenceClass(s string) (EvidenceClass, error) {
	v := EvidenceClassFromToken(s)
	if v == 0 {
		return 0, fmt.Errorf("invalid evidence class %q", s)
	}
	return v, nil
}
func EvidenceClassFromToken(s string) EvidenceClass {
	switch s {
	case "OBSERVATION":
		return EvidenceObservation
	case "COVERAGE_RECORD":
		return EvidenceCoverageRecord
	case "COMPLETENESS_RECORD":
		return EvidenceCompletenessRecord
	case "CERTIFICATION_RECORD":
		return EvidenceCertificationRecord
	case "VERIFICATION_RECORD":
		return EvidenceVerificationRecord
	case "PROVENANCE_RECORD":
		return EvidenceProvenanceRecord
	case "BACKEND_REALIZATION":
		return EvidenceBackendRealization
	}
	return 0
}
func (e EvidenceClass) Token() string {
	switch e {
	case EvidenceObservation:
		return "OBSERVATION"
	case EvidenceCoverageRecord:
		return "COVERAGE_RECORD"
	case EvidenceCompletenessRecord:
		return "COMPLETENESS_RECORD"
	case EvidenceCertificationRecord:
		return "CERTIFICATION_RECORD"
	case EvidenceVerificationRecord:
		return "VERIFICATION_RECORD"
	case EvidenceProvenanceRecord:
		return "PROVENANCE_RECORD"
	case EvidenceBackendRealization:
		return "BACKEND_REALIZATION"
	}
	return ""
}

const (
	EvidenceObservation EvidenceClass = iota + 1
	EvidenceCoverageRecord
	EvidenceCompletenessRecord
	EvidenceCertificationRecord
	EvidenceVerificationRecord
	EvidenceProvenanceRecord
	EvidenceBackendRealization
)

func ParseCompletenessClass(s string) (CompletenessClass, error) {
	switch s {
	case "EMPIRICAL_COMPLETENESS":
		return EmpiricalCompleteness, nil
	case "STRUCTURAL_COMPLETENESS":
		return StructuralCompleteness, nil
	case "DECLARED_COMPLETENESS":
		return DeclaredCompleteness, nil
	case "EXTERNALLY_CERTIFIED_COMPLETENESS":
		return ExternallyCertifiedCompleteness, nil
	}
	return CompletenessInvalid, fmt.Errorf("invalid completeness class")
}
func (c CompletenessClass) Token() string {
	switch c {
	case EmpiricalCompleteness:
		return "EMPIRICAL_COMPLETENESS"
	case StructuralCompleteness:
		return "STRUCTURAL_COMPLETENESS"
	case DeclaredCompleteness:
		return "DECLARED_COMPLETENESS"
	case ExternallyCertifiedCompleteness:
		return "EXTERNALLY_CERTIFIED_COMPLETENESS"
	}
	return ""
}
func ParseAdequacyClass(s string) (AdequacyClass, error) {
	switch s {
	case "STRUCTURAL_BASELINE":
		return StructuralBaseline, nil
	case "EXTERNAL_CERTIFICATION":
		return ExternalCertification, nil
	case "BACKEND_FORMAL_INVARIANT":
		return BackendFormalInvariant, nil
	case "BOUNDED_BEHAVIORAL":
		return BoundedBehavioral, nil
	case "TRUSTED_BASELINE_OBSERVED_DELTA":
		return TrustedBaselineObservedDelta, nil
	}
	return AdequacyInvalid, fmt.Errorf("invalid adequacy class")
}
func (a AdequacyClass) Token() string {
	switch a {
	case StructuralBaseline:
		return "STRUCTURAL_BASELINE"
	case ExternalCertification:
		return "EXTERNAL_CERTIFICATION"
	case BackendFormalInvariant:
		return "BACKEND_FORMAL_INVARIANT"
	case BoundedBehavioral:
		return "BOUNDED_BEHAVIORAL"
	case TrustedBaselineObservedDelta:
		return "TRUSTED_BASELINE_OBSERVED_DELTA"
	}
	return ""
}

type ProvenanceRequirement string

const (
	ProvenanceSourceIdentity     ProvenanceRequirement = "SOURCE_IDENTITY"
	ProvenanceMechanismIdentity  ProvenanceRequirement = "MECHANISM_IDENTITY"
	ProvenanceRunIdentity        ProvenanceRequirement = "RUN_IDENTITY"
	ProvenanceScopeBound         ProvenanceRequirement = "SCOPE_BOUND"
	ProvenanceContextBound       ProvenanceRequirement = "CONTEXT_BOUND"
	ProvenanceVerifierBound      ProvenanceRequirement = "VERIFIER_BOUND"
	ProvenanceCertificationBound ProvenanceRequirement = "CERTIFICATION_BOUND"
	ProvenanceCurrentValidity    ProvenanceRequirement = "CURRENT_VALIDITY"
)

func ParseProvenanceRequirement(s string) (ProvenanceRequirement, error) {
	p := ProvenanceRequirement(s)
	switch p {
	case ProvenanceSourceIdentity, ProvenanceMechanismIdentity, ProvenanceRunIdentity, ProvenanceScopeBound, ProvenanceContextBound, ProvenanceVerifierBound, ProvenanceCertificationBound, ProvenanceCurrentValidity:
		return p, nil
	}
	return "", fmt.Errorf("invalid provenance requirement")
}

type CertificationPropertyToken string

const (
	CertificationScopeCoverageToken         CertificationPropertyToken = "SCOPE_COVERAGE"
	CertificationBaselineCompatibilityToken CertificationPropertyToken = "BASELINE_COMPATIBILITY"
	// #nosec G101 -- public certification-property enum token, not credential material.
	CertificationPolicyAdequacyBoundedToken CertificationPropertyToken = "POLICY_ADEQUACY_BOUNDED"
	CertificationProvenanceValidityToken    CertificationPropertyToken = "PROVENANCE_VALIDITY"
)

func ParseCertificationPropertyToken(s string) (CertificationPropertyToken, error) {
	p := CertificationPropertyToken(s)
	switch p {
	case CertificationScopeCoverageToken, CertificationBaselineCompatibilityToken, CertificationPolicyAdequacyBoundedToken, CertificationProvenanceValidityToken:
		return p, nil
	}
	return "", fmt.Errorf("invalid certification property")
}

// SemanticValidity is the RFC-0004 inclusive validity interval.
type SemanticValidity struct{ notBefore, notAfter time.Time }

func NewSemanticValidity(notBefore, notAfter time.Time) (SemanticValidity, error) {
	if notBefore.IsZero() || notAfter.IsZero() || notBefore.After(notAfter) {
		return SemanticValidity{}, fmt.Errorf("invalid validity interval")
	}
	return SemanticValidity{notBefore: notBefore, notAfter: notAfter}, nil
}
func (v SemanticValidity) Valid() bool {
	return !v.notBefore.IsZero() && !v.notAfter.IsZero() && !v.notBefore.After(v.notAfter)
}
func (v SemanticValidity) NotBefore() time.Time { return v.notBefore }
func (v SemanticValidity) NotAfter() time.Time  { return v.notAfter }

type RevocationReference struct{ reference SemanticReference }

func NewRevocationReference(r SemanticReference) (RevocationReference, error) {
	if !r.Valid() || r.kind != ObjectKindRevocation {
		return RevocationReference{}, fmt.Errorf("invalid revocation reference")
	}
	return RevocationReference{reference: r}, nil
}
func (r RevocationReference) Valid() bool                  { return r.reference.Valid() }
func (r RevocationReference) Reference() SemanticReference { return r.reference }
