package authority

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

type AuthorityRuleDigest string

func (d AuthorityRuleDigest) Valid() bool    { _, err := NewDigest(string(d)); return err == nil }
func (d AuthorityRuleDigest) String() string { return string(d) }

type TypedResolvedAuthorityRule struct {
	reference SemanticReference
	rule      AuthorityRule
	digest    AuthorityRuleDigest
}

func (r TypedResolvedAuthorityRule) Valid() bool {
	if !r.reference.Valid() || r.reference.Kind() != ObjectKindAuthorityRule || !r.digest.Valid() || !r.rule.Valid() {
		return false
	}
	d, err := AuthorityRuleDigestOf(r.rule)
	return err == nil && d == r.digest && r.reference.ID() == r.rule.id && r.reference.Version().Compare(r.rule.version) == 0 && r.reference.Digest().String() == string(r.digest)
}
func (r TypedResolvedAuthorityRule) Reference() SemanticReference { return r.reference }
func (r TypedResolvedAuthorityRule) Rule() AuthorityRule          { return r.rule }
func (r TypedResolvedAuthorityRule) Digest() AuthorityRuleDigest  { return r.digest }

func referenceJSON(r SemanticReference) map[string]any {
	return map[string]any{"kind": string(r.Kind()), "id": r.ID(), "version": r.Version().String(), "digest": r.Digest().String()}
}

func CanonicalAuthorityRuleBytes(r AuthorityRule) ([]byte, error) {
	if !r.Valid() {
		return nil, fmt.Errorf("invalid authority rule")
	}
	m := map[string]any{
		"schemaId": r.schemaID, "schemaVersion": r.schemaVersion, "kind": authorityRuleKind,
		"id": r.id, "version": r.version.String(), "issuer": r.issuer, "backend": string(r.backend),
		"envelopeRef": referenceJSON(r.envelopeRef), "targetScope": map[string]any{"class": string(r.targetScope.Class()), "dimensions": scopeTokens(r.targetScope.Dimensions())},
		"mandatoryCoverageDimensions": scopeTokens(r.mandatoryDimensions),
		"acceptedEvidenceClasses":     evidenceTokens(r.evidenceClasses),
		"acceptedCompletenessClasses": completenessTokens(r.completenessClasses),
		"adequacyRequirements":        adequacyTokens(r.adequacyRequirements),
		"baselineRef":                 referenceJSON(r.baselineRef), "compatibilityRuleRef": referenceJSON(r.compatibilityRef),
		"provenanceRequirements":  provenanceTokens(r.provenanceRequirements),
		"certificationProperties": certificationTokens(r.certificationProperties),
		"validity":                map[string]any{"notBefore": r.validity.NotBefore().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), "notAfter": r.validity.NotAfter().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")},
		"revocationRef":           referenceJSON(r.revocationRef.Reference()),
		"registryRefs":            referencesJSON(r.registryRefs),
	}
	if r.trustPolicyRef.Valid() {
		m["trustPolicyRef"] = referenceJSON(r.trustPolicyRef)
	}
	if r.compositionRef.Valid() {
		m["compositionRef"] = referenceJSON(r.compositionRef)
	}
	return canonical(m)
}

func scopeTokens(v []ScopeDimension) []any {
	o := make([]any, len(v))
	for i, x := range v {
		o[i] = x.RFCString()
	}
	return o
}
func evidenceTokens(v []EvidenceClass) []any {
	o := make([]string, len(v))
	for i, x := range v {
		o[i] = x.Token()
	}
	return stringsAny(sortCanonicalStrings(o))
}
func completenessTokens(v []CompletenessClass) []any {
	o := make([]string, len(v))
	for i, x := range v {
		o[i] = x.Token()
	}
	return stringsAny(sortCanonicalStrings(o))
}
func adequacyTokens(v []AdequacyClass) []any {
	o := make([]string, len(v))
	for i, x := range v {
		o[i] = x.Token()
	}
	return stringsAny(sortCanonicalStrings(o))
}
func provenanceTokens(v []ProvenanceRequirement) []any {
	o := make([]string, len(v))
	for i, x := range v {
		o[i] = string(x)
	}
	return stringsAny(sortCanonicalStrings(o))
}
func certificationTokens(v []CertificationPropertyToken) []any {
	o := make([]string, len(v))
	for i, x := range v {
		o[i] = string(x)
	}
	return stringsAny(sortCanonicalStrings(o))
}
func referencesJSON(v []SemanticReference) []any {
	o := make([]any, len(v))
	for i, x := range v {
		o[i] = referenceJSON(x)
	}
	sort.Slice(o, func(i, j int) bool { a, _ := canonical(o[i]); b, _ := canonical(o[j]); return bytes.Compare(a, b) < 0 })
	return o
}
func sortCanonicalStrings(v []string) []string {
	sort.Slice(v, func(i, j int) bool { a, _ := canonical(v[i]); b, _ := canonical(v[j]); return bytes.Compare(a, b) < 0 })
	return v
}
func stringsAny(v []string) []any {
	o := make([]any, len(v))
	for i, x := range v {
		o[i] = x
	}
	return o
}

func AuthorityRuleDigestOf(r AuthorityRule) (AuthorityRuleDigest, error) {
	b, err := CanonicalAuthorityRuleBytes(r)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(append(append([]byte(authorityRuleDomain), 0), b...))
	d := AuthorityRuleDigest("sha256:" + hex.EncodeToString(h[:]))
	if !d.Valid() {
		return "", fmt.Errorf("invalid authority rule digest")
	}
	return d, nil
}

func DecodeAndBindAuthorityRule(ref SemanticReference, raw []byte) (TypedResolvedAuthorityRule, error) {
	if err := ref.ValidateKind(ObjectKindAuthorityRule); err != nil {
		return TypedResolvedAuthorityRule{}, err
	}
	f, err := StrictObject(raw)
	if err != nil {
		return TypedResolvedAuthorityRule{}, err
	}
	r, err := NewAuthorityRule(f)
	if err != nil {
		return TypedResolvedAuthorityRule{}, err
	}
	d, err := AuthorityRuleDigestOf(r)
	if err != nil {
		return TypedResolvedAuthorityRule{}, err
	}
	if ref.ID() != r.id || ref.Version().Compare(r.version) != 0 || ref.Digest().String() != d.String() {
		return TypedResolvedAuthorityRule{}, fmt.Errorf("authority rule binding mismatch")
	}
	return TypedResolvedAuthorityRule{reference: ref, rule: r, digest: d}, nil
}
