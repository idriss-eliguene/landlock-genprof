package authority

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const authorityRuleSchemaID = "authority-rule.v1"
const authorityRuleKind = "AUTHORITY_RULE"
const authorityRuleDomain = "landlock-genprof/rfc0004/authority-rule/v1"

var seccompMandatoryDimensions = map[string]struct{}{"STARTUP_BOOTSTRAP": {}, "CONTAINER_LIFETIME": {}, "PROCESS_TREE": {}, "EXECUTABLE_SET": {}, "WORKLOAD_STATE": {}, "ARCHITECTURE_ABI": {}, "IMAGE_IDENTITY": {}, "KERNEL_RUNTIME_COMPATIBILITY": {}}

type AuthorityRule struct {
	schemaID, schemaVersion, id, issuer                                        string
	version                                                                    SemanticVersion
	backend                                                                    BackendID
	envelopeRef, baselineRef, compatibilityRef, trustPolicyRef, compositionRef SemanticReference
	registryRefs                                                               []SemanticReference
	targetScope                                                                RuleTargetScope
	mandatoryDimensions                                                        []ScopeDimension
	evidenceClasses                                                            []EvidenceClass
	completenessClasses                                                        []CompletenessClass
	adequacyRequirements                                                       []AdequacyClass
	provenanceRequirements                                                     []ProvenanceRequirement
	certificationProperties                                                    []CertificationPropertyToken
	validity                                                                   SemanticValidity
	revocationRef                                                              RevocationReference
}

func ps(m map[string]json.RawMessage, k string) (string, error) {
	p, ok := m[k]
	if !ok {
		return "", fmt.Errorf("missing %s", k)
	}
	var s string
	if json.Unmarshal(p, &s) != nil || s == "" {
		return "", fmt.Errorf("invalid %s", k)
	}
	return s, nil
}
func pref(p json.RawMessage) (SemanticReference, error) {
	f, e := StrictObject(p)
	if e != nil {
		return SemanticReference{}, e
	}
	for k := range f {
		if k != "kind" && k != "id" && k != "version" && k != "digest" {
			return SemanticReference{}, fmt.Errorf("unknown reference field %s", k)
		}
	}
	k, e := ps(f, "kind")
	if e != nil {
		return SemanticReference{}, e
	}
	id, e := ps(f, "id")
	if e != nil {
		return SemanticReference{}, e
	}
	vs, e := ps(f, "version")
	if e != nil {
		return SemanticReference{}, e
	}
	v, e := ParseSemanticVersion(vs)
	if e != nil {
		return SemanticReference{}, e
	}
	ds, e := ps(f, "digest")
	if e != nil {
		return SemanticReference{}, e
	}
	d, e := NewDigest(ds)
	if e != nil {
		return SemanticReference{}, e
	}
	ok, e := ParseObjectKind(k)
	if e != nil {
		return SemanticReference{}, e
	}
	return NewSemanticReference(ok, id, v, d)
}
func preq(m map[string]json.RawMessage, k string, kind ObjectKind) (SemanticReference, error) {
	p, ok := m[k]
	if !ok {
		return SemanticReference{}, fmt.Errorf("missing %s", k)
	}
	r, e := pref(p)
	if e != nil || r.ValidateKind(kind) != nil {
		return SemanticReference{}, fmt.Errorf("invalid %s", k)
	}
	return r, nil
}
func NewAuthorityRule(f map[string]json.RawMessage) (AuthorityRule, error) {
	allow := map[string]bool{"schemaId": true, "schemaVersion": true, "kind": true, "id": true, "version": true, "issuer": true, "backend": true, "envelopeRef": true, "targetScope": true, "mandatoryCoverageDimensions": true, "acceptedEvidenceClasses": true, "acceptedCompletenessClasses": true, "adequacyRequirements": true, "baselineRef": true, "compatibilityRuleRef": true, "trustPolicyRef": true, "provenanceRequirements": true, "certificationProperties": true, "validity": true, "revocationRef": true, "registryRefs": true, "compositionRef": true}
	for k := range f {
		if !allow[k] {
			return AuthorityRule{}, fmt.Errorf("unknown field %s", k)
		}
	}
	if e := ValidateV1Envelope(f); e != nil {
		return AuthorityRule{}, e
	}
	schema, _ := ps(f, "schemaId")
	kind, _ := ps(f, "kind")
	if schema != authorityRuleSchemaID || kind != authorityRuleKind {
		return AuthorityRule{}, fmt.Errorf("invalid identity")
	}
	sv, _ := ps(f, "schemaVersion")
	id, e := ps(f, "id")
	if e != nil {
		return AuthorityRule{}, e
	}
	issuer, e := ps(f, "issuer")
	if e != nil {
		return AuthorityRule{}, e
	}
	ver, e := ps(f, "version")
	if e != nil {
		return AuthorityRule{}, e
	}
	v, e := ParseSemanticVersion(ver)
	if e != nil {
		return AuthorityRule{}, e
	}
	be, e := ps(f, "backend")
	if e != nil || be != "SECCOMP" {
		return AuthorityRule{}, fmt.Errorf("invalid backend")
	}
	env, e := preq(f, "envelopeRef", ObjectKindBackendEnvelopeRegistry)
	if e != nil {
		return AuthorityRule{}, e
	}
	base, e := preq(f, "baselineRef", ObjectKindBaseline)
	if e != nil {
		return AuthorityRule{}, e
	}
	comp, e := preq(f, "compatibilityRuleRef", ObjectKindCompatibilityRule)
	if e != nil {
		return AuthorityRule{}, e
	}
	regs, e := rset(f["registryRefs"], ObjectKindRegistry)
	if e != nil {
		return AuthorityRule{}, e
	}
	var tr, op SemanticReference
	if p, ok := f["trustPolicyRef"]; ok {
		tr, e = pref(p)
		if e != nil || tr.ValidateKind(ObjectKindTrustPolicy) != nil {
			return AuthorityRule{}, fmt.Errorf("invalid trust reference")
		}
	}
	if p, ok := f["compositionRef"]; ok {
		op, e = pref(p)
		if e != nil || op.ValidateKind(ObjectKindCompositionOperator) != nil {
			return AuthorityRule{}, fmt.Errorf("invalid composition reference")
		}
	}
	scope, e := tscope(f["targetScope"])
	if e != nil {
		return AuthorityRule{}, e
	}
	dims, e := dims(f["mandatoryCoverageDimensions"])
	if e != nil {
		return AuthorityRule{}, e
	}
	ev, e := evset(f["acceptedEvidenceClasses"])
	if e != nil {
		return AuthorityRule{}, e
	}
	co, e := coset(f["acceptedCompletenessClasses"])
	if e != nil {
		return AuthorityRule{}, e
	}
	ad, e := adset(f["adequacyRequirements"])
	if e != nil {
		return AuthorityRule{}, e
	}
	pr, e := prset(f["provenanceRequirements"])
	if e != nil {
		return AuthorityRule{}, e
	}
	cp, e := cpset(f["certificationProperties"])
	if e != nil {
		return AuthorityRule{}, e
	}
	va, e := validity(f["validity"])
	if e != nil {
		return AuthorityRule{}, e
	}
	rr, e := revref(f["revocationRef"])
	if e != nil {
		return AuthorityRule{}, e
	}
	return AuthorityRule{schemaID: schema, schemaVersion: sv, id: id, issuer: issuer, version: v, backend: BackendID(be), envelopeRef: env, baselineRef: base, compatibilityRef: comp, trustPolicyRef: tr, compositionRef: op, registryRefs: regs, targetScope: scope, mandatoryDimensions: dims, evidenceClasses: ev, completenessClasses: co, adequacyRequirements: ad, provenanceRequirements: pr, certificationProperties: cp, validity: va, revocationRef: rr}, nil
}
func rset(p json.RawMessage, k ObjectKind) ([]SemanticReference, error) {
	var a []json.RawMessage
	if json.Unmarshal(p, &a) != nil || len(a) == 0 {
		return nil, fmt.Errorf("invalid refs")
	}
	o := make([]SemanticReference, 0, len(a))
	seen := map[string]bool{}
	for _, x := range a {
		r, e := pref(x)
		if e != nil || r.ValidateKind(k) != nil {
			return nil, fmt.Errorf("invalid ref")
		}
		q := r.ID() + r.Version().String()
		if seen[q] {
			return nil, fmt.Errorf("duplicate ref")
		}
		seen[q] = true
		o = append(o, r)
	}
	sort.Slice(o, func(i, j int) bool { return o[i].ID() < o[j].ID() })
	return o, nil
}
func dims(p json.RawMessage) ([]ScopeDimension, error) {
	var a []string
	if json.Unmarshal(p, &a) != nil || len(a) == 0 {
		return nil, fmt.Errorf("invalid dimensions")
	}
	s := map[string]bool{}
	o := make([]ScopeDimension, 0, len(a))
	for _, x := range a {
		if _, ok := seccompMandatoryDimensions[x]; !ok || s[x] {
			return nil, fmt.Errorf("invalid dimension")
		}
		s[x] = true
		o = append(o, ScopeDimension(x))
	}
	for x := range seccompMandatoryDimensions {
		if !s[x] {
			return nil, fmt.Errorf("missing dimension")
		}
	}
	sort.Slice(o, func(i, j int) bool { return o[i] < o[j] })
	return o, nil
}
func tscope(p json.RawMessage) (RuleTargetScope, error) {
	f, e := StrictObject(p)
	if e != nil {
		return RuleTargetScope{}, e
	}
	for k := range f {
		if k != "class" && k != "dimensions" {
			return RuleTargetScope{}, fmt.Errorf("unknown scope field %s", k)
		}
	}
	c, e := ps(f, "class")
	if e != nil {
		return RuleTargetScope{}, e
	}
	var a []string
	if json.Unmarshal(f["dimensions"], &a) != nil {
		return RuleTargetScope{}, fmt.Errorf("invalid scope")
	}
	d := make([]ScopeDimension, len(a))
	for i, x := range a {
		d[i], e = ParseScopeDimension(x)
		if e != nil {
			return RuleTargetScope{}, e
		}
	}
	return NewRuleTargetScope(TargetScopeClass(c), d)
}
func evset(p json.RawMessage) ([]EvidenceClass, error) {
	var a []string
	if json.Unmarshal(p, &a) != nil {
		return nil, fmt.Errorf("invalid evidence")
	}
	o := make([]EvidenceClass, len(a))
	s := map[EvidenceClass]bool{}
	for i, x := range a {
		var e error
		o[i], e = ParseEvidenceClass(x)
		if e != nil || s[o[i]] {
			return nil, fmt.Errorf("invalid evidence")
		}
		s[o[i]] = true
	}
	return o, nil
}
func coset(p json.RawMessage) ([]CompletenessClass, error) {
	var a []string
	if json.Unmarshal(p, &a) != nil {
		return nil, fmt.Errorf("invalid completeness")
	}
	o := make([]CompletenessClass, len(a))
	s := map[CompletenessClass]bool{}
	for i, x := range a {
		var e error
		o[i], e = ParseCompletenessClass(x)
		if e != nil || s[o[i]] {
			return nil, fmt.Errorf("invalid completeness")
		}
		s[o[i]] = true
	}
	return o, nil
}
func adset(p json.RawMessage) ([]AdequacyClass, error) {
	var a []string
	if json.Unmarshal(p, &a) != nil {
		return nil, fmt.Errorf("invalid adequacy")
	}
	o := make([]AdequacyClass, len(a))
	s := map[AdequacyClass]bool{}
	for i, x := range a {
		var e error
		o[i], e = ParseAdequacyClass(x)
		if e != nil || s[o[i]] {
			return nil, fmt.Errorf("invalid adequacy")
		}
		s[o[i]] = true
	}
	return o, nil
}
func prset(p json.RawMessage) ([]ProvenanceRequirement, error) {
	var a []string
	if json.Unmarshal(p, &a) != nil {
		return nil, fmt.Errorf("invalid provenance")
	}
	o := make([]ProvenanceRequirement, len(a))
	s := map[ProvenanceRequirement]bool{}
	for i, x := range a {
		var e error
		o[i], e = ParseProvenanceRequirement(x)
		if e != nil || s[o[i]] {
			return nil, fmt.Errorf("invalid provenance")
		}
		s[o[i]] = true
	}
	return o, nil
}
func cpset(p json.RawMessage) ([]CertificationPropertyToken, error) {
	var a []string
	if json.Unmarshal(p, &a) != nil {
		return nil, fmt.Errorf("invalid certification")
	}
	o := make([]CertificationPropertyToken, len(a))
	s := map[CertificationPropertyToken]bool{}
	for i, x := range a {
		var e error
		o[i], e = ParseCertificationPropertyToken(x)
		if e != nil || s[o[i]] {
			return nil, fmt.Errorf("invalid certification")
		}
		s[o[i]] = true
	}
	return o, nil
}
func validity(p json.RawMessage) (SemanticValidity, error) {
	f, e := StrictObject(p)
	if e != nil {
		return SemanticValidity{}, e
	}
	for k := range f {
		if k != "notBefore" && k != "notAfter" {
			return SemanticValidity{}, fmt.Errorf("unknown validity field %s", k)
		}
	}
	a, e := ps(f, "notBefore")
	if e != nil {
		return SemanticValidity{}, e
	}
	b, e := ps(f, "notAfter")
	if e != nil {
		return SemanticValidity{}, e
	}
	x, e := time.Parse(time.RFC3339Nano, a)
	if e != nil {
		return SemanticValidity{}, e
	}
	y, e := time.Parse(time.RFC3339Nano, b)
	if e != nil {
		return SemanticValidity{}, e
	}
	return NewSemanticValidity(x, y)
}
func revref(p json.RawMessage) (RevocationReference, error) {
	r, e := pref(p)
	if e != nil {
		return RevocationReference{}, e
	}
	return NewRevocationReference(r)
}
func (r AuthorityRule) Valid() bool {
	return r.schemaID == authorityRuleSchemaID && r.id != "" && r.backend == BackendSeccomp && r.targetScope.Valid() && r.validity.Valid() && r.revocationRef.Valid()
}
func (r AuthorityRule) TargetScope() RuleTargetScope { return r.targetScope }
func (r AuthorityRule) MandatoryDimensions() []ScopeDimension {
	return append([]ScopeDimension(nil), r.mandatoryDimensions...)
}
func (r AuthorityRule) EvidenceClasses() []EvidenceClass {
	return append([]EvidenceClass(nil), r.evidenceClasses...)
}
func (r AuthorityRule) CompletenessClasses() []CompletenessClass {
	return append([]CompletenessClass(nil), r.completenessClasses...)
}
func (r AuthorityRule) AdequacyRequirements() []AdequacyClass {
	return append([]AdequacyClass(nil), r.adequacyRequirements...)
}
func (r AuthorityRule) ProvenanceRequirements() []ProvenanceRequirement {
	return append([]ProvenanceRequirement(nil), r.provenanceRequirements...)
}
func (r AuthorityRule) CertificationProperties() []CertificationPropertyToken {
	return append([]CertificationPropertyToken(nil), r.certificationProperties...)
}
func (r AuthorityRule) Validity() SemanticValidity               { return r.validity }
func (r AuthorityRule) RevocationReference() RevocationReference { return r.revocationRef }
func (r AuthorityRule) CanonicalBytes() ([]byte, error) {
	return json.Marshal(map[string]any{"schemaId": r.schemaID, "schemaVersion": r.schemaVersion, "kind": authorityRuleKind, "id": r.id, "version": r.version.String()})
}
func (r AuthorityRule) Digest() (Digest, error) {
	b, e := r.CanonicalBytes()
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(append(append([]byte(authorityRuleDomain), 0), b...))
	return NewDigest("sha256:" + hex.EncodeToString(h[:]))
}
