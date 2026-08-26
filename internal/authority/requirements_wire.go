package authority

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

const (
	resolvedRequirementSetSchemaID = "landlock-genprof/rfc0004/resolved-mandatory-requirement-set/v1"
	resolvedRequirementSetKind     = "RESOLVED_MANDATORY_REQUIREMENT_SET"
)

// EncodeResolvedMandatoryRequirementSet emits the fixed RFC-0004 envelope.
func EncodeResolvedMandatoryRequirementSet(s ResolvedMandatoryRequirementSet) ([]byte, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("invalid resolved requirement set")
	}
	return canonical(map[string]any{
		"schemaId": resolvedRequirementSetSchemaID, "schemaVersion": "1",
		"kind": resolvedRequirementSetKind, "id": s.id, "version": "1",
		"ruleRef": referenceJSON(s.ruleRef), "resolutionAttempt": string(s.attempt),
		"requirements": func() []any {
			out := make([]any, len(s.requirements))
			for i, r := range s.requirements {
				out[i] = r.identityObject()
			}
			return out
		}(),
	})
}

// DecodeResolvedMandatoryRequirementSet validates the fixed envelope. Member
// decoding is deliberately strict; unsupported wire member shapes fail closed.
func DecodeResolvedMandatoryRequirementSet(data []byte, authorities ...TypedResolvedAuthorityRule) (ResolvedMandatoryRequirementSet, error) {
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return ResolvedMandatoryRequirementSet{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("trailing envelope data")
	}
	allowed := map[string]bool{"schemaId": true, "schemaVersion": true, "kind": true, "id": true, "version": true, "ruleRef": true, "resolutionAttempt": true, "requirements": true}
	for k := range raw {
		if !allowed[k] {
			return ResolvedMandatoryRequirementSet{}, fmt.Errorf("unknown envelope field %s", k)
		}
	}
	for _, k := range []string{"schemaId", "schemaVersion", "kind", "id", "version", "ruleRef", "resolutionAttempt", "requirements"} {
		if _, ok := raw[k]; !ok {
			return ResolvedMandatoryRequirementSet{}, fmt.Errorf("missing envelope field %s", k)
		}
	}
	var schema, sv, kind, id, ver, attempt string
	for k, p := range map[string]*string{"schemaId": &schema, "schemaVersion": &sv, "kind": &kind, "id": &id, "version": &ver, "resolutionAttempt": &attempt} {
		if json.Unmarshal(raw[k], p) != nil {
			return ResolvedMandatoryRequirementSet{}, fmt.Errorf("invalid %s", k)
		}
	}
	if schema != resolvedRequirementSetSchemaID || sv != "1" || kind != resolvedRequirementSetKind || ver != "1" {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("unsupported requirement-set envelope")
	}
	if id == "" {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("empty set id")
	}
	var members []json.RawMessage
	if json.Unmarshal(raw["requirements"], &members) != nil || len(members) == 0 {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("invalid requirements")
	}
	if attempt == "" {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("invalid resolution attempt")
	}
	if len(authorities) != 1 || !authorities[0].Valid() {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("authoritative rule binding required")
	}
	var rr map[string]any
	if err := decodeExactJSON(raw["ruleRef"], &rr); err != nil || exactKeys(rr, "kind", "id", "version", "digest") != nil {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("invalid ruleRef")
	}
	auth := authorities[0].Reference()
	if rr["kind"] != string(auth.Kind()) || rr["id"] != auth.ID() || rr["version"] != auth.Version().String() || rr["digest"] != auth.Digest().String() {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("rule reference mismatch")
	}
	var decoded []MandatoryRequirement
	for _, item := range members {
		var m map[string]any
		if decodeExactJSON(item, &m) != nil {
			return ResolvedMandatoryRequirementSet{}, fmt.Errorf("malformed member")
		}
		r, err := decodeMandatoryMember(m)
		if err != nil {
			return ResolvedMandatoryRequirementSet{}, err
		}
		decoded = append(decoded, r)
	}
	at, err := NewResolutionAttemptIdentity(attempt)
	if err != nil {
		return ResolvedMandatoryRequirementSet{}, err
	}
	s, err := NewResolvedMandatoryRequirementSet(authorities[0], at, decoded)
	if err != nil {
		return s, err
	}
	if s.id != id {
		return ResolvedMandatoryRequirementSet{}, fmt.Errorf("set identity mismatch")
	}
	return s, nil
}

func decodeExactJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

func exactKeys(m map[string]any, keys ...string) error {
	if len(m) != len(keys) {
		return fmt.Errorf("unexpected object fields")
	}
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			return fmt.Errorf("missing object field %s", key)
		}
	}
	return nil
}

func exactUnsigned(v any, bits int) (uint64, error) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, fmt.Errorf("integer required")
	}
	s := n.String()
	if s == "" || s[0] == '-' || (len(s) > 1 && s[0] == '0') {
		return 0, fmt.Errorf("canonical unsigned integer required")
	}
	u, err := strconv.ParseUint(s, 10, bits)
	if err != nil {
		return 0, fmt.Errorf("canonical unsigned integer required")
	}
	return u, nil
}

func str(m map[string]any, k string) (string, error) {
	v, ok := m[k].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("invalid %s", k)
	}
	return v, nil
}
func decodeMandatoryMember(m map[string]any) (MandatoryRequirement, error) {
	f, e := str(m, "family")
	if e != nil {
		return MandatoryRequirement{}, e
	}
	v, e := str(m, "schemaVersion")
	if e != nil || v != "1" {
		return MandatoryRequirement{}, fmt.Errorf("unsupported member version")
	}
	switch MandatoryRequirementFamily(f) {
	case RequirementTrust:
		if err := exactKeys(m, "family", "schemaVersion", "subject", "policyRef", "rootRef", "scope", "context"); err != nil {
			return MandatoryRequirement{}, err
		}
		sub, _ := str(m, "subject")
		p, _ := str(m, "policyRef")
		root, _ := str(m, "rootRef")
		sc, err := decodeScope(m["scope"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		c, err := decodeContext(m["context"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		return NewTrustRequirement(sub, p, root, sc, c), nil
	case RequirementVerification:
		if err := exactKeys(m, "family", "schemaVersion", "subject", "verifier", "property", "scope", "context"); err != nil {
			return MandatoryRequirement{}, err
		}
		sub, _ := str(m, "subject")
		ver, _ := str(m, "verifier")
		prop, _ := str(m, "property")
		sc, err := decodeScope(m["scope"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		c, err := decodeContext(m["context"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		return NewVerificationRequirement(sub, ver, prop, sc, c), nil
	case RequirementRevocation:
		if err := exactKeys(m, "family", "schemaVersion", "subject", "sourceRef"); err != nil {
			return MandatoryRequirement{}, err
		}
		s, e := str(m, "subject")
		if e != nil {
			return MandatoryRequirement{}, e
		}
		x, e := str(m, "sourceRef")
		if e != nil {
			return MandatoryRequirement{}, e
		}
		return NewRevocationStatusRequirement(s, x), nil
	case RequirementCompatibility:
		if err := exactKeys(m, "family", "schemaVersion", "schema", "predicate", "field", "candidate", "baseline", "requirementRef", "backend", "subject", "scope", "context"); err != nil {
			return MandatoryRequirement{}, err
		}
		schema, _ := str(m, "schema")
		pred, _ := str(m, "predicate")
		field, _ := str(m, "field")
		cand, _ := str(m, "candidate")
		base, _ := str(m, "baseline")
		ref, _ := str(m, "requirementRef")
		back, _ := str(m, "backend")
		sub, _ := str(m, "subject")
		sc, err := decodeScope(m["scope"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		c, err := decodeContext(m["context"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		return NewMandatoryCompatibilityRequirement(schema, pred, field, cand, base, ref, back, sub, sc, c), nil
	case RequirementCoverage:
		if err := exactKeys(m, "family", "schemaVersion", "subject", "backend", "sourceRef", "scope", "context"); err != nil {
			return MandatoryRequirement{}, err
		}
		sub, _ := str(m, "subject")
		back, _ := str(m, "backend")
		src, _ := str(m, "sourceRef")
		sc, err := decodeScope(m["scope"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		c, err := decodeContext(m["context"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		return NewCoverageRequirement(sub, back, src, sc, c), nil
	case RequirementCompleteness:
		if err := exactKeys(m, "family", "schemaVersion", "subject", "requiredClass", "scope"); err != nil {
			return MandatoryRequirement{}, err
		}
		sub, _ := str(m, "subject")
		class, err := decodeCompletenessClass(m["requiredClass"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		sc, err := decodeScope(m["scope"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		return NewCompletenessRequirement(sub, class, sc), nil
	case RequirementAdequacy:
		if err := exactKeys(m, "family", "schemaVersion", "subject", "requiredClass", "scope", "context", "verifier"); err != nil {
			return MandatoryRequirement{}, err
		}
		sub, _ := str(m, "subject")
		class, err := decodeAdequacyClass(m["requiredClass"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		sc, err := decodeScope(m["scope"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		c, err := decodeContext(m["context"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		v, err := decodeVerifier(m["verifier"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		return NewAdequacyRequirement(sub, class, sc, c, v), nil
	case RequirementCertification:
		if err := exactKeys(m, "family", "schemaVersion", "subject", "certificateRef", "property", "scope", "context", "verifier"); err != nil {
			return MandatoryRequirement{}, err
		}
		sub, _ := str(m, "subject")
		cert, _ := str(m, "certificateRef")
		prop, err := decodeCertificationProperty(m["property"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		sc, err := decodeScope(m["scope"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		c, err := decodeContext(m["context"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		v, err := decodeVerifier(m["verifier"])
		if err != nil {
			return MandatoryRequirement{}, err
		}
		return NewCertificationRequirement(sub, cert, prop, sc, c, v), nil
	default:
		return MandatoryRequirement{}, fmt.Errorf("unknown member family")
	}
}

func decodeScope(v any) (Scope, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return Scope{}, fmt.Errorf("invalid scope")
	}
	if err := exactKeys(m, "dimensions", "target", "context"); err != nil {
		return Scope{}, fmt.Errorf("invalid scope: %w", err)
	}
	t, ok := m["target"].(string)
	if !ok {
		return Scope{}, fmt.Errorf("invalid scope")
	}
	c, ok := m["context"].(string)
	if !ok {
		return Scope{}, fmt.Errorf("invalid scope")
	}
	arr, ok := m["dimensions"].([]any)
	if !ok {
		return Scope{}, fmt.Errorf("invalid scope")
	}
	ds := make([]ScopeDimensionResult, 0, len(arr))
	for _, x := range arr {
		z, ok := x.(map[string]any)
		if !ok {
			return Scope{}, fmt.Errorf("invalid scope")
		}
		if err := exactKeys(z, "dimension", "state"); err != nil {
			return Scope{}, fmt.Errorf("invalid scope: %w", err)
		}
		d, ok := z["dimension"].(string)
		if !ok {
			return Scope{}, fmt.Errorf("invalid scope")
		}
		n, err := exactUnsigned(z["state"], 8)
		if err != nil {
			return Scope{}, fmt.Errorf("invalid scope")
		}
		ds = append(ds, ScopeDimensionResult{ScopeDimension(d), ScopeCoverageState(n)})
	}
	return NewScope(ds, t, c)
}
func decodeContext(v any) (SecurityContextIdentity, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return SecurityContextIdentity{}, fmt.Errorf("invalid context")
	}
	if err := exactKeys(m, "image", "architecture", "abi", "kernel", "workload", "executable", "libc", "privilege", "namespace", "configuration", "environment", "features", "persistent"); err != nil {
		return SecurityContextIdentity{}, fmt.Errorf("invalid context: %w", err)
	}
	c := SecurityContextIdentity{}
	c.ImageIdentity, _ = m["image"].(string)
	c.Architecture, _ = m["architecture"].(string)
	c.ABI, _ = m["abi"].(string)
	c.KernelRuntimeClass, _ = m["kernel"].(string)
	c.WorkloadIdentity, _ = m["workload"].(string)
	c.ExecutableIdentity, _ = m["executable"].(string)
	c.LibcIdentity, _ = m["libc"].(string)
	c.PrivilegeContext, _ = m["privilege"].(string)
	c.NamespaceSecurity, _ = m["namespace"].(string)
	c.ConfigurationID, _ = m["configuration"].(string)
	c.EnvironmentID, _ = m["environment"].(string)
	c.FeatureSetID, _ = m["features"].(string)
	c.PersistentStateID, _ = m["persistent"].(string)
	return NewSecurityContextIdentity(c)
}
func decodeCompletenessClass(v any) (CompletenessClass, error) {
	n, err := exactUnsigned(v, 8)
	c := CompletenessClass(n)
	if err != nil || !c.Valid() {
		return 0, fmt.Errorf("invalid completeness class")
	}
	return c, nil
}
func decodeAdequacyClass(v any) (AdequacyClass, error) {
	n, err := exactUnsigned(v, 8)
	c := AdequacyClass(n)
	if err != nil || !c.Valid() {
		return 0, fmt.Errorf("invalid adequacy class")
	}
	return c, nil
}
func decodeCertificationProperty(v any) (CertificationProperty, error) {
	n, err := exactUnsigned(v, 8)
	p := CertificationProperty(n)
	if err != nil || !p.Valid() {
		return 0, fmt.Errorf("invalid certification property")
	}
	return p, nil
}
func decodeVerifier(v any) (VerifierSemanticIdentity, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return VerifierSemanticIdentity{}, fmt.Errorf("invalid verifier")
	}
	if err := exactKeys(m, "id", "version", "digest", "class", "inputSchema", "outputSchema", "property", "procedure", "constraints"); err != nil {
		return VerifierSemanticIdentity{}, fmt.Errorf("invalid verifier: %w", err)
	}
	id, _ := m["id"].(string)
	ver, _ := m["version"].(string)
	ds, _ := m["digest"].(string)
	d, e := NewDigest(ds)
	if e != nil {
		return VerifierSemanticIdentity{}, e
	}
	cl, _ := m["class"].(string)
	in, _ := m["inputSchema"].(string)
	out, _ := m["outputSchema"].(string)
	prop, _ := m["property"].(string)
	proc, _ := m["procedure"].(string)
	rawConstraints, ok := m["constraints"].([]any)
	if !ok {
		return VerifierSemanticIdentity{}, fmt.Errorf("invalid verifier constraints")
	}
	constraints := make([]string, len(rawConstraints))
	for i, raw := range rawConstraints {
		constraint, ok := raw.(string)
		if !ok {
			return VerifierSemanticIdentity{}, fmt.Errorf("invalid verifier constraint")
		}
		constraints[i] = constraint
	}
	return NewVerifierSemanticIdentity(VerifierSemanticIdentity{id: id, version: ver, digest: d, class: cl, inputSchema: in, outputSchema: out, property: prop, procedure: proc, constraints: constraints})
}
