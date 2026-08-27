package authority

import (
	"encoding/json"
	"testing"
)

const completeAuthorityRuleJSON = `{"schemaId":"authority-rule.v1","schemaVersion":"1","kind":"AUTHORITY_RULE","id":"rule-1","version":"1.2.3.4","issuer":"issuer-1","backend":"SECCOMP","envelopeRef":{"kind":"BACKEND_ENVELOPE_REGISTRY","id":"seccomp","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"targetScope":{"class":"CONTAINER_LIFETIME","dimensions":["CONTAINER_LIFETIME","PROCESS_TREE"]},"mandatoryCoverageDimensions":["STARTUP_BOOTSTRAP","CONTAINER_LIFETIME","PROCESS_TREE","EXECUTABLE_SET","WORKLOAD_STATE","ARCHITECTURE_ABI","IMAGE_IDENTITY","KERNEL_RUNTIME_COMPATIBILITY"],"acceptedEvidenceClasses":["OBSERVATION","COVERAGE_RECORD"],"acceptedCompletenessClasses":["STRUCTURAL_COMPLETENESS"],"adequacyRequirements":["STRUCTURAL_BASELINE"],"baselineRef":{"kind":"BASELINE","id":"base","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"compatibilityRuleRef":{"kind":"COMPATIBILITY_RULE","id":"compat","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"trustPolicyRef":{"kind":"TRUST_POLICY","id":"trust","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"provenanceRequirements":["SOURCE_IDENTITY","CONTEXT_BOUND"],"certificationProperties":["SCOPE_COVERAGE"],"validity":{"notBefore":"2024-01-01T00:00:00Z","notAfter":"2025-01-01T00:00:00Z"},"revocationRef":{"kind":"REVOCATION","id":"rev","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"registryRefs":[{"kind":"SECURITY_FIELD_REGISTRY","id":"fields","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}],"compositionRef":{"kind":"COMPOSITION_OPERATOR","id":"compose","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}`

func decodeCompleteRule(t *testing.T) (AuthorityRule, map[string]json.RawMessage) {
	t.Helper()
	f, err := StrictObject([]byte(completeAuthorityRuleJSON))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewAuthorityRule(f)
	if err != nil {
		t.Fatal(err)
	}
	return r, f
}

func TestAuthorityRuleCompleteTypedFixture(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	if !r.Valid() || r.backend != BackendSeccomp || r.id != "rule-1" {
		t.Fatal("invalid typed rule")
	}
	if r.TargetScope().Class() != TargetScopeContainerLifetime {
		t.Fatal("scope class not preserved")
	}
	if len(r.MandatoryDimensions()) != 8 || len(r.EvidenceClasses()) != 2 || len(r.CompletenessClasses()) != 1 || len(r.AdequacyRequirements()) != 1 || len(r.ProvenanceRequirements()) != 2 || len(r.CertificationProperties()) != 1 {
		t.Fatal("typed fields not preserved")
	}
	if r.envelopeRef.Kind() != ObjectKindBackendEnvelopeRegistry || r.baselineRef.Kind() != ObjectKindBaseline || r.compatibilityRef.Kind() != ObjectKindCompatibilityRule || r.trustPolicyRef.Kind() != ObjectKindTrustPolicy || r.compositionRef.Kind() != ObjectKindCompositionOperator {
		t.Fatal("reference kind binding lost")
	}
	if !r.Validity().Valid() || !r.RevocationReference().Valid() {
		t.Fatal("validity/revocation missing")
	}
}

func TestAuthorityRuleRequiredFieldOmission(t *testing.T) {
	_, base := decodeCompleteRule(t)
	for _, field := range []string{"schemaId", "schemaVersion", "kind", "id", "version", "issuer", "backend", "envelopeRef", "targetScope", "mandatoryCoverageDimensions", "acceptedEvidenceClasses", "acceptedCompletenessClasses", "adequacyRequirements", "baselineRef", "compatibilityRuleRef", "provenanceRequirements", "certificationProperties", "validity", "revocationRef", "registryRefs"} {
		t.Run(field, func(t *testing.T) {
			cp := map[string]json.RawMessage{}
			for k, v := range base {
				cp[k] = v
			}
			delete(cp, field)
			if _, err := NewAuthorityRule(cp); err == nil {
				t.Fatalf("omitted %s accepted", field)
			}
		})
	}
}

func TestAuthorityRuleWrongReferenceKinds(t *testing.T) {
	_, base := decodeCompleteRule(t)
	for _, field := range []string{"envelopeRef", "baselineRef", "compatibilityRuleRef", "trustPolicyRef", "compositionRef", "revocationRef"} {
		t.Run(field, func(t *testing.T) {
			cp := map[string]json.RawMessage{}
			for k, v := range base {
				cp[k] = v
			}
			wrong := "TRUST_POLICY"
			if field == "trustPolicyRef" {
				wrong = "BASELINE"
			}
			cp[field] = json.RawMessage([]byte(`{"kind":"` + wrong + `","id":"wrong","version":"1.0.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`))
			if _, err := NewAuthorityRule(cp); err == nil {
				t.Fatal("wrong reference kind accepted")
			}
		})
	}
}

func TestAuthorityRuleSeccompDimensionBattery(t *testing.T) {
	_, base := decodeCompleteRule(t)
	for dim := range seccompMandatoryDimensions {
		t.Run(dim, func(t *testing.T) {
			var ds []string
			_ = json.Unmarshal(base["mandatoryCoverageDimensions"], &ds)
			out := ds[:0]
			for _, x := range ds {
				if x != dim {
					out = append(out, x)
				}
			}
			b, _ := json.Marshal(out)
			cp := map[string]json.RawMessage{}
			for k, v := range base {
				cp[k] = v
			}
			cp["mandatoryCoverageDimensions"] = b
			if _, err := NewAuthorityRule(cp); err == nil {
				t.Fatal("missing dimension accepted")
			}
		})
	}
	for _, raw := range []string{`["STARTUP_BOOTSTRAP","STARTUP_BOOTSTRAP","CONTAINER_LIFETIME","PROCESS_TREE","EXECUTABLE_SET","WORKLOAD_STATE","ARCHITECTURE_ABI","IMAGE_IDENTITY","KERNEL_RUNTIME_COMPATIBILITY"]`, `["startup_bootstrap","CONTAINER_LIFETIME","PROCESS_TREE","EXECUTABLE_SET","WORKLOAD_STATE","ARCHITECTURE_ABI","IMAGE_IDENTITY","KERNEL_RUNTIME_COMPATIBILITY"]`} {
		cp := map[string]json.RawMessage{}
		for k, v := range base {
			cp[k] = v
		}
		cp["mandatoryCoverageDimensions"] = json.RawMessage(raw)
		if _, err := NewAuthorityRule(cp); err == nil {
			t.Fatal("invalid dimensions accepted")
		}
	}
}

func TestAuthorityRuleCollectionImmutability(t *testing.T) {
	r, _ := decodeCompleteRule(t)
	got := r.MandatoryDimensions()
	got[0] = "bad"
	if r.MandatoryDimensions()[0] == "bad" {
		t.Fatal("getter alias")
	}
	scope := r.TargetScope().Dimensions()
	scope[0] = "bad"
	if r.TargetScope().Dimensions()[0] == "bad" {
		t.Fatal("nested getter alias")
	}
}

func TestNestedAuthorityRuleObjectsRejectUnknownFields(t *testing.T) {
	_, base := decodeCompleteRule(t)
	cases := []struct {
		name, field string
		mutate      func(json.RawMessage) json.RawMessage
	}{
		{"reference", "envelopeRef", func(p json.RawMessage) json.RawMessage {
			return json.RawMessage(string(p[:len(p)-1]) + `,"futureField":"x"}`)
		}},
		{"scope", "targetScope", func(p json.RawMessage) json.RawMessage {
			return json.RawMessage(string(p[:len(p)-1]) + `,"futureField":"x"}`)
		}},
		{"validity", "validity", func(p json.RawMessage) json.RawMessage {
			return json.RawMessage(string(p[:len(p)-1]) + `,"futureField":"x"}`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := map[string]json.RawMessage{}
			for k, v := range base {
				cp[k] = v
			}
			cp[tc.field] = tc.mutate(cp[tc.field])
			if _, err := NewAuthorityRule(cp); err == nil {
				t.Fatal("unknown nested field accepted")
			}
		})
	}
}

func TestAuthorityRuleSeccompDimensionsAreLiteralRFCSet(t *testing.T) {
	want := map[string]bool{"STARTUP_BOOTSTRAP": true, "CONTAINER_LIFETIME": true, "PROCESS_TREE": true, "EXECUTABLE_SET": true, "WORKLOAD_STATE": true, "ARCHITECTURE_ABI": true, "IMAGE_IDENTITY": true, "KERNEL_RUNTIME_COMPATIBILITY": true}
	if len(want) != 8 || len(seccompMandatoryDimensions) != 8 {
		t.Fatal("mandatory set cardinality changed")
	}
	for d := range want {
		if _, ok := seccompMandatoryDimensions[d]; !ok {
			t.Fatalf("missing RFC dimension %s", d)
		}
	}
}
