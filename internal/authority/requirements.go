package authority

import (
	"bytes"
	"fmt"
	"time"
)

// MandatoryRequirementFamily is the closed P2.6-C family discriminator.
type MandatoryRequirementFamily string

const (
	RequirementTrust         MandatoryRequirementFamily = "trust"
	RequirementVerification  MandatoryRequirementFamily = "verification"
	RequirementRevocation    MandatoryRequirementFamily = "revocation"
	RequirementCompatibility MandatoryRequirementFamily = "compatibility"
	RequirementCoverage      MandatoryRequirementFamily = "coverage"
	RequirementCompleteness  MandatoryRequirementFamily = "completeness"
	RequirementAdequacy      MandatoryRequirementFamily = "adequacy"
	RequirementCertification MandatoryRequirementFamily = "certification"
)

const mandatoryRequirementVersion = "1"

type TrustRequirement struct {
	Subject, PolicyRef, RootRef string
	Scope                       Scope
	Context                     SecurityContextIdentity
}
type VerificationRequirement struct {
	Subject, Verifier, Property string
	Scope                       Scope
	Context                     SecurityContextIdentity
}
type RevocationStatusRequirement struct{ Subject, SourceRef string }
type MandatoryCompatibilityRequirement struct {
	Schema, Predicate, Field, Candidate, Baseline, RequirementRef, Backend, Subject string
	Scope                                                                           Scope
	Context                                                                         SecurityContextIdentity
}
type CoverageRequirement struct {
	Subject, Backend, SourceRef string
	Scope                       Scope
	Context                     SecurityContextIdentity
}
type CompletenessRequirement struct {
	Subject       string
	RequiredClass CompletenessClass
	Scope         Scope
}
type AdequacyRequirement struct {
	Subject       string
	RequiredClass AdequacyClass
	Scope         Scope
	Context       SecurityContextIdentity
	Verifier      VerifierSemanticIdentity
}
type CertificationRequirement struct {
	Subject, CertificateRef string
	Property                CertificationProperty
	Scope                   Scope
	Context                 SecurityContextIdentity
	Verifier                VerifierSemanticIdentity
}

func NewTrustRequirement(subject, policyRef, rootRef string, scope Scope, context SecurityContextIdentity) MandatoryRequirement {
	return MandatoryRequirement{Family: RequirementTrust, Version: mandatoryRequirementVersion, Trust: &TrustRequirement{subject, policyRef, rootRef, scope, context}}
}
func NewVerificationRequirement(subject, verifier, property string, scope Scope, context SecurityContextIdentity) MandatoryRequirement {
	return MandatoryRequirement{Family: RequirementVerification, Version: mandatoryRequirementVersion, Verification: &VerificationRequirement{subject, verifier, property, scope, context}}
}
func NewRevocationStatusRequirement(subject, sourceRef string) MandatoryRequirement {
	return MandatoryRequirement{Family: RequirementRevocation, Version: mandatoryRequirementVersion, RevocationStatus: &RevocationStatusRequirement{subject, sourceRef}}
}
func NewMandatoryCompatibilityRequirement(schema, predicate, field, candidate, baseline, requirementRef, backend, subject string, scope Scope, context SecurityContextIdentity) MandatoryRequirement {
	return MandatoryRequirement{Family: RequirementCompatibility, Version: mandatoryRequirementVersion, Compatibility: &MandatoryCompatibilityRequirement{schema, predicate, field, candidate, baseline, requirementRef, backend, subject, scope, context}}
}
func NewCoverageRequirement(subject, backend, sourceRef string, scope Scope, context SecurityContextIdentity) MandatoryRequirement {
	return MandatoryRequirement{Family: RequirementCoverage, Version: mandatoryRequirementVersion, Coverage: &CoverageRequirement{subject, backend, sourceRef, scope, context}}
}
func NewCompletenessRequirement(subject string, requiredClass CompletenessClass, scope Scope) MandatoryRequirement {
	return MandatoryRequirement{Family: RequirementCompleteness, Version: mandatoryRequirementVersion, Completeness: &CompletenessRequirement{subject, requiredClass, scope}}
}
func NewAdequacyRequirement(subject string, requiredClass AdequacyClass, scope Scope, context SecurityContextIdentity, verifier VerifierSemanticIdentity) MandatoryRequirement {
	return MandatoryRequirement{Family: RequirementAdequacy, Version: mandatoryRequirementVersion, Adequacy: &AdequacyRequirement{subject, requiredClass, scope, context, verifier}}
}
func NewCertificationRequirement(subject, certificateRef string, property CertificationProperty, scope Scope, context SecurityContextIdentity, verifier VerifierSemanticIdentity) MandatoryRequirement {
	return MandatoryRequirement{Family: RequirementCertification, Version: mandatoryRequirementVersion, Certification: &CertificationRequirement{subject, certificateRef, property, scope, context, verifier}}
}

// MandatoryRequirement is a closed tagged union. Exactly one pointer is set.
type MandatoryRequirement struct {
	Family           MandatoryRequirementFamily
	Version          string
	Trust            *TrustRequirement
	Verification     *VerificationRequirement
	RevocationStatus *RevocationStatusRequirement
	Compatibility    *MandatoryCompatibilityRequirement
	Coverage         *CoverageRequirement
	Completeness     *CompletenessRequirement
	Adequacy         *AdequacyRequirement
	Certification    *CertificationRequirement
}

func cloneMandatoryRequirement(r MandatoryRequirement) MandatoryRequirement {
	cloneScope := func(s Scope) Scope {
		return Scope{dimensions: append([]ScopeDimensionResult(nil), s.dimensions...), target: s.target, context: s.context}
	}
	cloneVerifier := func(v VerifierSemanticIdentity) VerifierSemanticIdentity {
		v.constraints = append([]string(nil), v.constraints...)
		return v
	}
	out := MandatoryRequirement{Family: r.Family, Version: r.Version}
	if r.Trust != nil {
		v := *r.Trust
		v.Scope = cloneScope(v.Scope)
		out.Trust = &v
	}
	if r.Verification != nil {
		v := *r.Verification
		v.Scope = cloneScope(v.Scope)
		out.Verification = &v
	}
	if r.RevocationStatus != nil {
		v := *r.RevocationStatus
		out.RevocationStatus = &v
	}
	if r.Compatibility != nil {
		v := *r.Compatibility
		v.Scope = cloneScope(v.Scope)
		out.Compatibility = &v
	}
	if r.Coverage != nil {
		v := *r.Coverage
		v.Scope = cloneScope(v.Scope)
		out.Coverage = &v
	}
	if r.Completeness != nil {
		v := *r.Completeness
		v.Scope = cloneScope(v.Scope)
		out.Completeness = &v
	}
	if r.Adequacy != nil {
		v := *r.Adequacy
		v.Scope = cloneScope(v.Scope)
		v.Verifier = cloneVerifier(v.Verifier)
		out.Adequacy = &v
	}
	if r.Certification != nil {
		v := *r.Certification
		v.Scope = cloneScope(v.Scope)
		v.Verifier = cloneVerifier(v.Verifier)
		out.Certification = &v
	}
	return out
}

func (r MandatoryRequirement) validShape() bool {
	count := 0
	if r.Trust != nil {
		count++
	}
	if r.Verification != nil {
		count++
	}
	if r.RevocationStatus != nil {
		count++
	}
	if r.Compatibility != nil {
		count++
	}
	if r.Coverage != nil {
		count++
	}
	if r.Completeness != nil {
		count++
	}
	if r.Adequacy != nil {
		count++
	}
	if r.Certification != nil {
		count++
	}
	return r.Version == mandatoryRequirementVersion && count == 1
}

func (r MandatoryRequirement) Valid() bool {
	if !r.validShape() {
		return false
	}
	switch r.Family {
	case RequirementTrust:
		return r.Trust.Subject != "" && r.Trust.PolicyRef != "" && r.Trust.RootRef != "" && r.Trust.Scope.Valid() && validContext(r.Trust.Context)
	case RequirementVerification:
		return r.Verification.Subject != "" && r.Verification.Verifier != "" && r.Verification.Property != "" && r.Verification.Scope.Valid() && validContext(r.Verification.Context)
	case RequirementRevocation:
		return r.RevocationStatus.Subject != "" && r.RevocationStatus.SourceRef != ""
	case RequirementCompatibility:
		q := r.Compatibility
		return q.Schema != "" && q.Predicate != "" && q.Field != "" && q.Candidate != "" && q.Baseline != "" && q.RequirementRef != "" && q.Backend != "" && q.Subject != "" && q.Scope.Valid() && validContext(q.Context)
	case RequirementCoverage:
		return r.Coverage.Subject != "" && r.Coverage.Backend != "" && r.Coverage.SourceRef != "" && r.Coverage.Scope.Valid() && validContext(r.Coverage.Context)
	case RequirementCompleteness:
		return r.Completeness.Subject != "" && r.Completeness.RequiredClass.Valid() && r.Completeness.Scope.Valid()
	case RequirementAdequacy:
		q := r.Adequacy
		return q.Subject != "" && q.RequiredClass.Valid() && q.Scope.Valid() && validContext(q.Context) && validVerifier(q.Verifier)
	case RequirementCertification:
		q := r.Certification
		return q.Subject != "" && q.CertificateRef != "" && q.Property.Valid() && q.Scope.Valid() && validContext(q.Context) && validVerifier(q.Verifier)
	default:
		return false
	}
}

func mandatoryScopeIdentity(s Scope) map[string]any {
	return map[string]any{"dimensions": scopeTokensResult(s.dimensions), "target": s.target, "context": s.context}
}
func scopeTokensResult(v []ScopeDimensionResult) []any {
	out := make([]any, len(v))
	for i, d := range v {
		out[i] = map[string]any{"dimension": string(d.Dimension), "state": int(d.State)}
	}
	return out
}
func contextIdentity(c SecurityContextIdentity) map[string]any {
	return map[string]any{"image": c.ImageIdentity, "architecture": c.Architecture, "abi": c.ABI, "kernel": c.KernelRuntimeClass, "workload": c.WorkloadIdentity, "executable": c.ExecutableIdentity, "libc": c.LibcIdentity, "privilege": c.PrivilegeContext, "namespace": c.NamespaceSecurity, "configuration": c.ConfigurationID, "environment": c.EnvironmentID, "features": c.FeatureSetID, "persistent": c.PersistentStateID}
}
func verifierIdentity(v VerifierSemanticIdentity) map[string]any {
	return map[string]any{"id": v.ID(), "version": v.Version(), "digest": v.Digest().String(), "class": v.Class(), "inputSchema": v.InputSchema(), "outputSchema": v.OutputSchema(), "property": v.Property(), "procedure": v.Procedure(), "constraints": stringsAny(v.Constraints())}
}

func (r MandatoryRequirement) identityObject() map[string]any {
	m := map[string]any{"family": string(r.Family), "schemaVersion": r.Version}
	switch r.Family {
	case RequirementTrust:
		q := r.Trust
		m["subject"] = q.Subject
		m["policyRef"] = q.PolicyRef
		m["rootRef"] = q.RootRef
		m["scope"] = mandatoryScopeIdentity(q.Scope)
		m["context"] = contextIdentity(q.Context)
	case RequirementVerification:
		q := r.Verification
		m["subject"] = q.Subject
		m["verifier"] = q.Verifier
		m["property"] = q.Property
		m["scope"] = mandatoryScopeIdentity(q.Scope)
		m["context"] = contextIdentity(q.Context)
	case RequirementRevocation:
		m["subject"] = r.RevocationStatus.Subject
		m["sourceRef"] = r.RevocationStatus.SourceRef
	case RequirementCompatibility:
		q := r.Compatibility
		m["schema"] = q.Schema
		m["predicate"] = q.Predicate
		m["field"] = q.Field
		m["candidate"] = q.Candidate
		m["baseline"] = q.Baseline
		m["requirementRef"] = q.RequirementRef
		m["backend"] = q.Backend
		m["subject"] = q.Subject
		m["scope"] = mandatoryScopeIdentity(q.Scope)
		m["context"] = contextIdentity(q.Context)
	case RequirementCoverage:
		q := r.Coverage
		m["subject"] = q.Subject
		m["backend"] = q.Backend
		m["sourceRef"] = q.SourceRef
		m["scope"] = mandatoryScopeIdentity(q.Scope)
		m["context"] = contextIdentity(q.Context)
	case RequirementCompleteness:
		q := r.Completeness
		m["subject"] = q.Subject
		m["requiredClass"] = int(q.RequiredClass)
		m["scope"] = mandatoryScopeIdentity(q.Scope)
	case RequirementAdequacy:
		q := r.Adequacy
		m["subject"] = q.Subject
		m["requiredClass"] = int(q.RequiredClass)
		m["scope"] = mandatoryScopeIdentity(q.Scope)
		m["context"] = contextIdentity(q.Context)
		m["verifier"] = verifierIdentity(q.Verifier)
	case RequirementCertification:
		q := r.Certification
		m["subject"] = q.Subject
		m["certificateRef"] = q.CertificateRef
		m["property"] = int(q.Property)
		m["scope"] = mandatoryScopeIdentity(q.Scope)
		m["context"] = contextIdentity(q.Context)
		m["verifier"] = verifierIdentity(q.Verifier)
	}
	return m
}

func (r MandatoryRequirement) MemberIdentity() ([]byte, error) {
	if !r.Valid() {
		return nil, fmt.Errorf("invalid mandatory requirement")
	}
	return canonical(r.identityObject())
}

func canonicalIdentityString(v any) (string, error) {
	b, err := canonical(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// MatchRequest constructs the P2.6-B request from authoritative set membership.
// A detached MandatoryRequirement cannot create this binding.
func (s ResolvedMandatoryRequirementSet) MatchRequest(r MandatoryRequirement, at time.Time) (MatchRequest, error) {
	if !s.Valid() || !r.Valid() || at.IsZero() {
		return MatchRequest{}, fmt.Errorf("invalid requirement match inputs")
	}
	memberID, err := r.MemberIdentity()
	if err != nil {
		return MatchRequest{}, err
	}
	found := false
	for _, member := range s.requirements {
		id, _ := member.MemberIdentity()
		if bytes.Equal(id, memberID) {
			found = true
			break
		}
	}
	if !found {
		return MatchRequest{}, fmt.Errorf("requirement is not a member of resolved set")
	}
	authorityID, err := canonicalIdentityString(referenceJSON(s.ruleRef))
	if err != nil {
		return MatchRequest{}, err
	}
	q := MatchRequest{Attempt: s.attempt, At: at, Authority: authorityID, Requirement: string(memberID), Backend: string(s.rule.Rule().backend)}
	setContext := func(scope Scope, context *SecurityContextIdentity) error {
		if context == nil {
			q.Context = scope.Context()
			return nil
		}
		encoded, err := canonicalIdentityString(contextIdentity(*context))
		if err != nil {
			return err
		}
		q.Context = encoded
		q.TypedContext = *context
		return nil
	}
	switch r.Family {
	case RequirementTrust:
		q.Family = FamilyTrust
		q.Subject = r.Trust.Subject
		q.Policy = r.Trust.PolicyRef
		q.Root = r.Trust.RootRef
		q.Scope = r.Trust.Scope
		if err := setContext(r.Trust.Scope, &r.Trust.Context); err != nil {
			return MatchRequest{}, err
		}
	case RequirementVerification:
		q.Family = FamilyVerification
		q.Subject = r.Verification.Subject
		q.Producer = r.Verification.Verifier
		q.Property = r.Verification.Property
		q.Scope = r.Verification.Scope
		if err := setContext(r.Verification.Scope, &r.Verification.Context); err != nil {
			return MatchRequest{}, err
		}
	case RequirementRevocation:
		q.Family = FamilyRevocation
		q.Subject = r.RevocationStatus.Subject
		q.Source = r.RevocationStatus.SourceRef
	case RequirementCompatibility:
		v := r.Compatibility
		q.Family = FamilyCompatibility
		q.Subject = v.Subject
		q.Backend = v.Backend
		q.Schema = v.Schema
		q.Predicate = v.Predicate
		q.Field = v.Field
		q.Candidate = v.Candidate
		q.Baseline = v.Baseline
		q.CompatibilityRequirementRef = v.RequirementRef
		q.Scope = v.Scope
		if err := setContext(v.Scope, &v.Context); err != nil {
			return MatchRequest{}, err
		}
	case RequirementCoverage:
		v := r.Coverage
		q.Family = FamilyCoverage
		q.Subject = v.Subject
		q.Backend = v.Backend
		q.Source = v.SourceRef
		q.Scope = v.Scope
		if err := setContext(v.Scope, &v.Context); err != nil {
			return MatchRequest{}, err
		}
	case RequirementCertification:
		v := r.Certification
		q.Family = FamilyCertification
		q.Subject = v.Subject
		q.Source = v.CertificateRef
		q.Property = fmt.Sprint(v.Property)
		q.Scope = v.Scope
		if err := setContext(v.Scope, &v.Context); err != nil {
			return MatchRequest{}, err
		}
		q.Producer = v.Verifier.ID()
		q.Verifier = v.Verifier
	case RequirementCompleteness:
		q.Family = FamilyCompleteness
		q.Subject = r.Completeness.Subject
		q.Scope = r.Completeness.Scope
		if err := setContext(r.Completeness.Scope, nil); err != nil {
			return MatchRequest{}, err
		}
		q.RequiredCompletenessClass = r.Completeness.RequiredClass
	case RequirementAdequacy:
		q.Family = FamilyAdequacy
		q.Subject = r.Adequacy.Subject
		q.Scope = r.Adequacy.Scope
		if err := setContext(r.Adequacy.Scope, &r.Adequacy.Context); err != nil {
			return MatchRequest{}, err
		}
		q.Producer = r.Adequacy.Verifier.ID()
		q.Verifier = r.Adequacy.Verifier
		q.RequiredAdequacyClass = r.Adequacy.RequiredClass
	default:
		return MatchRequest{}, fmt.Errorf("match adapter not implemented for family %s", r.Family)
	}
	return q, nil
}

type ResolvedMandatoryRequirementSet struct {
	schemaVersion, id string
	rule              TypedResolvedAuthorityRule
	ruleRef           SemanticReference
	attempt           ResolutionAttemptIdentity
	requirements      []MandatoryRequirement
}

func NewResolvedMandatoryRequirementSet(rule TypedResolvedAuthorityRule, attempt ResolutionAttemptIdentity, requirements []MandatoryRequirement) (ResolvedMandatoryRequirementSet, error) {
	if !rule.Valid() || !attempt.Valid() || len(requirements) == 0 {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("invalid resolved mandatory requirement set")
	}
	seen := map[string]struct{}{}
	normalized := make([]MandatoryRequirement, 0, len(requirements))
	for _, input := range requirements {
		r := cloneMandatoryRequirement(input)
		b, err := r.MemberIdentity()
		if err != nil {
			return ResolvedMandatoryRequirementSet{}, err
		}
		k := string(b)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		normalized = append(normalized, r)
	}
	for i := 1; i < len(normalized); i++ {
		for j := i; j > 0; j-- {
			a, _ := normalized[j-1].MemberIdentity()
			b, _ := normalized[j].MemberIdentity()
			if bytes.Compare(a, b) <= 0 {
				break
			}
			normalized[j-1], normalized[j] = normalized[j], normalized[j-1]
		}
	}
	rr := rule.Reference()
	payload := map[string]any{"schemaVersion": "1", "ruleRef": referenceJSON(rr), "resolutionAttempt": string(attempt), "requirements": make([]any, len(normalized))}
	for i, r := range normalized {
		payload["requirements"].([]any)[i] = r.identityObject()
	}
	raw, err := canonical(payload)
	if err != nil {
		return ResolvedMandatoryRequirementSet{}, err
	}
	id := hashDomain("landlock-genprof/rfc0004/resolved-mandatory-requirement-set/v1", raw)
	return ResolvedMandatoryRequirementSet{schemaVersion: "1", id: id, rule: rule, ruleRef: rr, attempt: attempt, requirements: normalized}, nil
}
func (s ResolvedMandatoryRequirementSet) Valid() bool {
	return s.schemaVersion == "1" && s.id != "" && s.rule.Valid() && s.rule.Reference() == s.ruleRef && s.ruleRef.Valid() && s.ruleRef.Kind() == ObjectKindAuthorityRule && s.attempt.Valid() && len(s.requirements) > 0 && func() bool {
		for _, r := range s.requirements {
			if !r.Valid() {
				return false
			}
		}
		return true
	}()
}
func (s ResolvedMandatoryRequirementSet) ID() string                         { return s.id }
func (s ResolvedMandatoryRequirementSet) RuleRef() SemanticReference         { return s.ruleRef }
func (s ResolvedMandatoryRequirementSet) Attempt() ResolutionAttemptIdentity { return s.attempt }
func (s ResolvedMandatoryRequirementSet) Requirements() []MandatoryRequirement {
	out := make([]MandatoryRequirement, len(s.requirements))
	for i, requirement := range s.requirements {
		out[i] = cloneMandatoryRequirement(requirement)
	}
	return out
}

func (s ResolvedMandatoryRequirementSet) CanonicalBytes() ([]byte, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("invalid resolved mandatory requirement set")
	}
	reqs := make([]any, len(s.requirements))
	for i, r := range s.requirements {
		reqs[i] = r.identityObject()
	}
	return canonical(map[string]any{"schemaVersion": s.schemaVersion, "ruleRef": referenceJSON(s.ruleRef), "resolutionAttempt": string(s.attempt), "requirements": reqs})
}
