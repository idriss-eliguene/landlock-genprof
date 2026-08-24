package authority

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAuthorityRuleCanonicalIdentityIncludesFields(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	b, err := CanonicalAuthorityRuleBytes(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"issuer", "backend", "envelopeRef", "targetScope", "mandatoryCoverageDimensions", "acceptedEvidenceClasses", "acceptedCompletenessClasses", "adequacyRequirements", "baselineRef", "compatibilityRuleRef", "provenanceRequirements", "certificationProperties", "validity", "revocationRef", "registryRefs", "trustPolicyRef", "compositionRef"} {
		if !containsJSONField(b, field) {
			t.Fatalf("canonical bytes omit %s", field)
		}
	}
}

func containsJSONField(b []byte, field string) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	_, ok := m[field]
	return ok
}

func TestAuthorityRuleIdentityBindingAndMutation(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	d, err := AuthorityRuleDigestOf(r)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := NewSemanticReference(ObjectKindAuthorityRule, "rule-1", r.version, Digest(d))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := DecodeAndBindAuthorityRule(ref, []byte(completeAuthorityRuleJSON))
	if err != nil || !bound.Valid() {
		t.Fatal(err)
	}
	f, _ := StrictObject([]byte(completeAuthorityRuleJSON))
	var issuer string
	_ = json.Unmarshal(f["issuer"], &issuer)
	f["issuer"] = json.RawMessage(`"changed"`)
	changed, _ := NewAuthorityRule(f)
	if _, err := DecodeAndBindAuthorityRule(ref, mustJSON(f)); err == nil {
		t.Fatal("changed content accepted")
	}
	if changed.issuer == issuer {
		t.Fatal("mutation fixture did not change")
	}
	wrong, _ := NewSemanticReference(ObjectKindTrustPolicy, "rule-1", r.version, Digest(d))
	if _, err := DecodeAndBindAuthorityRule(wrong, []byte(completeAuthorityRuleJSON)); err == nil {
		t.Fatal("wrong kind accepted")
	}
}

func mustJSON(m map[string]json.RawMessage) []byte { b, _ := json.Marshal(m); return b }

func TestAuthorityRuleIdentityZeroRejected(t *testing.T) {
	if _, err := CanonicalAuthorityRuleBytes(AuthorityRule{}); err == nil {
		t.Fatal("zero rule canonicalized")
	}
	if _, err := AuthorityRuleDigestOf(AuthorityRule{}); err == nil {
		t.Fatal("zero rule digested")
	}
}

func TestAuthorityRuleLiteralGoldenDigest(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	const goldenDomain = "landlock-genprof/rfc0004/authority-rule/v1"
	const goldenCanonical = `{"acceptedCompletenessClasses":["STRUCTURAL_COMPLETENESS"],"acceptedEvidenceClasses":["COVERAGE_RECORD","OBSERVATION"],"adequacyRequirements":["STRUCTURAL_BASELINE"],"backend":"SECCOMP","baselineRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","id":"base","kind":"BASELINE","version":"1.0.0.0"},"certificationProperties":["SCOPE_COVERAGE"],"compatibilityRuleRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","id":"compat","kind":"COMPATIBILITY_RULE","version":"1.0.0.0"},"compositionRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","id":"compose","kind":"COMPOSITION_OPERATOR","version":"1.0.0.0"},"envelopeRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","id":"seccomp","kind":"BACKEND_ENVELOPE_REGISTRY","version":"1.0.0.0"},"id":"rule-1","issuer":"issuer-1","kind":"AUTHORITY_RULE","mandatoryCoverageDimensions":["ARCHITECTURE_ABI","CONTAINER_LIFETIME","EXECUTABLE_SET","IMAGE_IDENTITY","KERNEL_RUNTIME_COMPATIBILITY","PROCESS_TREE","STARTUP_BOOTSTRAP","WORKLOAD_STATE"],"provenanceRequirements":["CONTEXT_BOUND","SOURCE_IDENTITY"],"registryRefs":[{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","id":"fields","kind":"SECURITY_FIELD_REGISTRY","version":"1.0.0.0"}],"revocationRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","id":"rev","kind":"REVOCATION","version":"1.0.0.0"},"schemaId":"authority-rule.v1","schemaVersion":"1","targetScope":{"class":"CONTAINER_LIFETIME","dimensions":["CONTAINER_LIFETIME","PROCESS_TREE"]},"trustPolicyRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","id":"trust","kind":"TRUST_POLICY","version":"1.0.0.0"},"validity":{"notAfter":"2025-01-01T00:00:00Z","notBefore":"2024-01-01T00:00:00Z"},"version":"1.2.3.4"}`
	const expectedDigest AuthorityRuleDigest = "sha256:60d568671a34fda782ff2fd8aae4f8f265f02ffff514602b6648e7f4614936a5"
	b, err := CanonicalAuthorityRuleBytes(r)
	if err != nil || string(b) != goldenCanonical {
		t.Fatalf("canonical golden mismatch: %v\\n%s", err, b)
	}
	h := sha256.Sum256(append(append([]byte(goldenDomain), 0), []byte(goldenCanonical)...))
	independent := "sha256:" + hex.EncodeToString(h[:])
	if independent != string(expectedDigest) {
		t.Fatalf("independent digest mismatch: %s", independent)
	}
	d, err := AuthorityRuleDigestOf(r)
	if err != nil || d != expectedDigest {
		t.Fatalf("golden digest mismatch: %v %s", err, d)
	}
	if !strings.Contains(goldenCanonical, `"revocationRef"`) {
		t.Fatal("golden missing revocation")
	}
}

func TestAuthorityRuleOptionalPresenceChangesIdentity(t *testing.T) {
	r, base := decodeCompleteRule(t)
	without := map[string]json.RawMessage{}
	for k, v := range base {
		without[k] = v
	}
	delete(without, "trustPolicyRef")
	a, e := NewAuthorityRule(without)
	if e != nil {
		t.Fatal(e)
	}
	da, _ := AuthorityRuleDigestOf(a)
	db, _ := AuthorityRuleDigestOf(r)
	if da == db {
		t.Fatal("trust absence collided with presence")
	}
	without = map[string]json.RawMessage{}
	for k, v := range base {
		without[k] = v
	}
	delete(without, "compositionRef")
	c, e := NewAuthorityRule(without)
	if e != nil {
		t.Fatal(e)
	}
	dc, _ := AuthorityRuleDigestOf(c)
	if dc == db {
		t.Fatal("composition absence collided with presence")
	}
	for _, field := range []string{"trustPolicyRef", "compositionRef"} {
		m := cloneRaw(base)
		m[field] = json.RawMessage(`{"kind":"BASELINE","id":"bad","version":"1.0.0.0","digest":"bad"}`)
		if _, err := NewAuthorityRule(m); err == nil {
			t.Fatalf("invalid present %s accepted", field)
		}
	}
}

func TestAuthorityRuleContentMutationBattery(t *testing.T) {
	r, base := decodeCompleteRule(t)
	golden, _ := AuthorityRuleDigestOf(r)
	ref, _ := NewSemanticReference(ObjectKindAuthorityRule, r.id, r.version, Digest(golden))
	fields := []string{"issuer", "acceptedEvidenceClasses", "certificationProperties", "validity", "revocationRef", "baselineRef", "compatibilityRuleRef", "registryRefs"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			m := cloneRaw(base)
			switch field {
			case "issuer":
				m[field] = json.RawMessage(`"changed-issuer"`)
			case "acceptedEvidenceClasses":
				m[field] = json.RawMessage(`["VERIFICATION_RECORD"]`)
			case "certificationProperties":
				m[field] = json.RawMessage(`["PROVENANCE_VALIDITY"]`)
			case "validity":
				m[field] = json.RawMessage(`{"notBefore":"2024-02-01T00:00:00Z","notAfter":"2025-01-01T00:00:00Z"}`)
			case "revocationRef":
				m[field] = json.RawMessage(`{"kind":"REVOCATION","id":"other","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
			case "baselineRef":
				m[field] = json.RawMessage(`{"kind":"BASELINE","id":"other","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
			case "compatibilityRuleRef":
				m[field] = json.RawMessage(`{"kind":"COMPATIBILITY_RULE","id":"other","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
			case "registryRefs":
				m[field] = json.RawMessage(`[{"kind":"SECURITY_FIELD_REGISTRY","id":"other","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}]`)
			}
			x, e := NewAuthorityRule(m)
			if e != nil {
				t.Fatal(e)
			}
			d, e := AuthorityRuleDigestOf(x)
			if e != nil || d == golden {
				t.Fatal("mutation did not change digest")
			}
			if _, e = DecodeAndBindAuthorityRule(ref, mustJSON(m)); e == nil {
				t.Fatal("original reference accepted mutated content")
			}
		})
	}
}

func TestTypedResolvedAuthorityRuleTamper(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	d, _ := AuthorityRuleDigestOf(r)
	ref, _ := NewSemanticReference(ObjectKindAuthorityRule, r.id, r.version, Digest(d))
	good := TypedResolvedAuthorityRule{reference: ref, rule: r, digest: d}
	if !good.Valid() {
		t.Fatal("positive wrapper invalid")
	}
	bad := good
	bad.digest = ""
	if bad.Valid() {
		t.Fatal("zero digest accepted")
	}
	bad = good
	bad.reference, _ = NewSemanticReference(ObjectKindAuthorityRule, "other", r.version, Digest(d))
	if bad.Valid() {
		t.Fatal("ID tamper accepted")
	}
	bad = good
	bad.rule.id = "other"
	if bad.Valid() {
		t.Fatal("rule tamper accepted")
	}
	bad = good
	bad.reference, _ = NewSemanticReference(ObjectKindAuthorityRule, r.id, SemanticVersion{}, Digest(d))
	if bad.Valid() {
		t.Fatal("version tamper accepted")
	}
	bad = good
	bad.reference, _ = NewSemanticReference(ObjectKindTrustPolicy, r.id, r.version, Digest(d))
	if bad.Valid() {
		t.Fatal("kind tamper accepted")
	}
	bad = good
	bad.reference, _ = NewSemanticReference(ObjectKindAuthorityRule, r.id, r.version, Digest("sha256:"+strings.Repeat("0", 64)))
	if bad.Valid() {
		t.Fatal("reference digest tamper accepted")
	}
}

// Independently authored equivalent wire form: member and SET order differ.
const goldenEquivalentB = `{
  "kind":"AUTHORITY_RULE","schemaVersion":"1","schemaId":"authority-rule.v1",
  "version":"1.2.3.4","id":"rule-1","issuer":"issuer-1","backend":"SECCOMP",
  "envelopeRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","version":"1.0.0.0","id":"seccomp","kind":"BACKEND_ENVELOPE_REGISTRY"},
  "targetScope":{"dimensions":["PROCESS_TREE","CONTAINER_LIFETIME"],"class":"CONTAINER_LIFETIME"},
  "mandatoryCoverageDimensions":["KERNEL_RUNTIME_COMPATIBILITY","IMAGE_IDENTITY","ARCHITECTURE_ABI","WORKLOAD_STATE","EXECUTABLE_SET","PROCESS_TREE","CONTAINER_LIFETIME","STARTUP_BOOTSTRAP"],
  "acceptedEvidenceClasses":["COVERAGE_RECORD","OBSERVATION"],"acceptedCompletenessClasses":["STRUCTURAL_COMPLETENESS"],
  "adequacyRequirements":["STRUCTURAL_BASELINE"],
  "baselineRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","version":"1.0.0.0","id":"base","kind":"BASELINE"},
  "compatibilityRuleRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","version":"1.0.0.0","id":"compat","kind":"COMPATIBILITY_RULE"},
  "trustPolicyRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","version":"1.0.0.0","id":"trust","kind":"TRUST_POLICY"},
  "provenanceRequirements":["CONTEXT_BOUND","SOURCE_IDENTITY"],"certificationProperties":["SCOPE_COVERAGE"],
  "validity":{"notAfter":"2025-01-01T00:00:00Z","notBefore":"2024-01-01T00:00:00Z"},
  "revocationRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","version":"1.0.0.0","id":"rev","kind":"REVOCATION"},
  "registryRefs":[{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","version":"1.0.0.0","id":"fields","kind":"SECURITY_FIELD_REGISTRY"}],
  "compositionRef":{"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","version":"1.0.0.0","id":"compose","kind":"COMPOSITION_OPERATOR"}
}`

func TestAuthorityRuleLiteralDifferentialSets(t *testing.T) {
	a, _ := decodeCompleteRule(t)
	f, err := StrictObject([]byte(goldenEquivalentB))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewAuthorityRule(f)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := CanonicalAuthorityRuleBytes(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := CanonicalAuthorityRuleBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(ca) != string(cb) {
		t.Fatal("reordered semantic sets changed canonical bytes")
	}
	da, _ := AuthorityRuleDigestOf(a)
	db, _ := AuthorityRuleDigestOf(b)
	if da != db {
		t.Fatalf("reordered semantic sets changed digest: %s != %s", da, db)
	}
}

func TestAuthorityRuleSetFamiliesHaveLiteralCanonicalOrder(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	b, err := CanonicalAuthorityRuleBytes(r)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatal(err)
	}
	expected := map[string][]string{
		"mandatoryCoverageDimensions": {"ARCHITECTURE_ABI", "CONTAINER_LIFETIME", "EXECUTABLE_SET", "IMAGE_IDENTITY", "KERNEL_RUNTIME_COMPATIBILITY", "PROCESS_TREE", "STARTUP_BOOTSTRAP", "WORKLOAD_STATE"},
		"acceptedEvidenceClasses":     {"COVERAGE_RECORD", "OBSERVATION"},
		"acceptedCompletenessClasses": {"STRUCTURAL_COMPLETENESS"},
		"adequacyRequirements":        {"STRUCTURAL_BASELINE"},
		"provenanceRequirements":      {"CONTEXT_BOUND", "SOURCE_IDENTITY"},
		"certificationProperties":     {"SCOPE_COVERAGE"},
	}
	for field, want := range expected {
		var got []string
		if err := json.Unmarshal(obj[field], &got); err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("%s order %v, want literal %v", field, got, want)
		}
	}
}

func TestAuthorityRuleNonTrivialSetPermutations(t *testing.T) {
	_, base := decodeCompleteRule(t)
	refJSON := `{"kind":"SECURITY_FIELD_REGISTRY","id":"arch","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`
	fields := []struct {
		name, a, b, expected string
	}{
		{"completeness", `["EMPIRICAL_COMPLETENESS","STRUCTURAL_COMPLETENESS"]`, `["STRUCTURAL_COMPLETENESS","EMPIRICAL_COMPLETENESS"]`, `["EMPIRICAL_COMPLETENESS","STRUCTURAL_COMPLETENESS"]`},
		{"adequacy", `["EXTERNAL_CERTIFICATION","STRUCTURAL_BASELINE"]`, `["STRUCTURAL_BASELINE","EXTERNAL_CERTIFICATION"]`, `["EXTERNAL_CERTIFICATION","STRUCTURAL_BASELINE"]`},
		{"certification", `["BASELINE_COMPATIBILITY","SCOPE_COVERAGE"]`, `["SCOPE_COVERAGE","BASELINE_COMPATIBILITY"]`, `["BASELINE_COMPATIBILITY","SCOPE_COVERAGE"]`},
	}
	for _, tc := range fields {
		t.Run(tc.name, func(t *testing.T) {
			ma, mb := cloneRaw(base), cloneRaw(base)
			key := map[string]string{"completeness": "acceptedCompletenessClasses", "adequacy": "adequacyRequirements", "certification": "certificationProperties"}[tc.name]
			ma[key], mb[key] = json.RawMessage(tc.a), json.RawMessage(tc.b)
			ra, err := NewAuthorityRule(ma)
			if err != nil {
				t.Fatal(err)
			}
			rb, err := NewAuthorityRule(mb)
			if err != nil {
				t.Fatal(err)
			}
			ca, _ := CanonicalAuthorityRuleBytes(ra)
			cb, _ := CanonicalAuthorityRuleBytes(rb)
			if string(ca) != string(cb) {
				t.Fatal("permutation changed canonical bytes")
			}
			da, _ := AuthorityRuleDigestOf(ra)
			db, _ := AuthorityRuleDigestOf(rb)
			if da != db {
				t.Fatal("permutation changed digest")
			}
			var obj map[string]json.RawMessage
			_ = json.Unmarshal(ca, &obj)
			if string(obj[key]) != tc.expected {
				t.Fatalf("canonical order %s, want %s", obj[key], tc.expected)
			}
		})
	}
	t.Run("registry", func(t *testing.T) {
		first := `{"kind":"SECURITY_FIELD_REGISTRY","id":"fields","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`
		second := json.RawMessage(fmt.Sprintf(`[%s,%s]`, first, refJSON))
		reversed := json.RawMessage(fmt.Sprintf(`[%s,%s]`, refJSON, first))
		ma, mb := cloneRaw(base), cloneRaw(base)
		ma["registryRefs"], mb["registryRefs"] = second, reversed
		ra, err := NewAuthorityRule(ma)
		if err != nil {
			t.Fatal(err)
		}
		rb, err := NewAuthorityRule(mb)
		if err != nil {
			t.Fatal(err)
		}
		ca, _ := CanonicalAuthorityRuleBytes(ra)
		cb, _ := CanonicalAuthorityRuleBytes(rb)
		if string(ca) != string(cb) {
			t.Fatal("registry permutation changed canonical bytes")
		}
		da, _ := AuthorityRuleDigestOf(ra)
		db, _ := AuthorityRuleDigestOf(rb)
		if da != db {
			t.Fatal("registry permutation changed digest")
		}
		var obj map[string]json.RawMessage
		_ = json.Unmarshal(ca, &obj)
		var refs []map[string]any
		_ = json.Unmarshal(obj["registryRefs"], &refs)
		if refs[0]["id"] != "arch" || refs[1]["id"] != "fields" {
			t.Fatalf("unexpected literal registry order: %#v", refs)
		}
	})
}

func TestAuthorityRuleHTMLAndUnicodeStrings(t *testing.T) {
	_, base := decodeCompleteRule(t)
	for _, tc := range []struct {
		name, value string
		want        string
	}{
		{"html", "issuer<alpha>&beta", "issuer<alpha>&beta"},
		{"unicode", "émetteur", "émetteur"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := cloneRaw(base)
			m["issuer"] = json.RawMessage(fmt.Sprintf("%q", tc.value))
			r, err := NewAuthorityRule(m)
			if err != nil {
				t.Fatal(err)
			}
			b, err := CanonicalAuthorityRuleBytes(r)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), tc.want) {
				t.Fatalf("canonical output lost %s", tc.name)
			}
			if strings.Contains(string(b), `\u003c`) || strings.Contains(string(b), `\u003e`) || strings.Contains(string(b), `\u0026`) {
				t.Fatal("HTML escaping leaked into JCS")
			}
			b2, _ := CanonicalAuthorityRuleBytes(r)
			if string(b) != string(b2) {
				t.Fatal("non-deterministic string canonicalization")
			}
		})
	}
	decomp := cloneRaw(base)
	decomp["issuer"] = json.RawMessage(`"e\u0301"`)
	comp := cloneRaw(base)
	comp["issuer"] = json.RawMessage(`"é"`)
	r1, _ := NewAuthorityRule(decomp)
	r2, _ := NewAuthorityRule(comp)
	d1, _ := AuthorityRuleDigestOf(r1)
	d2, _ := AuthorityRuleDigestOf(r2)
	if d1 == d2 {
		t.Fatal("Unicode normalization collapsed distinct strings")
	}
}

func cloneRaw(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for k, v := range in {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}
