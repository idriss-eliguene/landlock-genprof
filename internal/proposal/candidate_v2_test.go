package proposal

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func candidateV2Fixture() CandidateV2 {
	return CandidateV2{
		Version:  CandidateV2Version,
		Subject:  SubjectV2{Scope: CandidateV2ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:" + strings.Repeat("a", 64)},
		Artifact: ArtifactV2{Type: CandidateV2ArtifactContainerCaps, ContainerCapabilities: ContainerCapabilitiesV2{Drop: []string{"ALL"}, Add: []string{"CAP_NET_ADMIN", "CAP_CHOWN"}}},
	}
}

func TestCandidateV2FixedFixture(t *testing.T) {
	c := candidateV2Fixture()
	b, err := c.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	const wantHex = "0000000c63616e6469646174652d76320000000c63616e6469646174652d763200000009434f4e5441494e45520000000e4465706c6f796d656e742f61706900000003617070000000477368613235363a6161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616161616100000016434f4e5441494e45525f4341504142494c49544945530000000100000003414c4c00000002000000094341505f43484f574e0000000d4341505f4e45545f41444d494e"
	if got := hex.EncodeToString(b); got != wantHex {
		t.Fatalf("canonical hex = %s", got)
	}
	const wantDigest = "sha256:46062013486c3c47ba3d092d002fa12eb86eeb2019eafcd8b1e51805a9e32609"
	got, err := CandidateDigestV2(c)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantDigest {
		t.Fatalf("digest = %s", got)
	}
}

func TestCandidateV2ValidationAndCanonicalization(t *testing.T) {
	valid := candidateV2Fixture()
	cases := []struct {
		name    string
		mutate  func(*CandidateV2)
		wantErr bool
	}{
		{"empty version", func(c *CandidateV2) { c.Version = "" }, true},
		{"unknown version", func(c *CandidateV2) { c.Version = "candidate-v3" }, true},
		{"binary scope", func(c *CandidateV2) { c.Subject.Scope = "BINARY" }, true},
		{"empty target", func(c *CandidateV2) { c.Subject.Target = "" }, true},
		{"empty container", func(c *CandidateV2) { c.Subject.Container = "" }, true},
		{"empty image", func(c *CandidateV2) { c.Subject.ImageIdentity = "" }, true},
		{"empty artifact", func(c *CandidateV2) { c.Artifact.Type = "" }, true},
		{"unknown artifact", func(c *CandidateV2) { c.Artifact.Type = "OTHER" }, true},
		{"bad drop", func(c *CandidateV2) { c.Artifact.ContainerCapabilities.Drop = []string{"NET_ADMIN"} }, true},
		{"unknown capability", func(c *CandidateV2) { c.Artifact.ContainerCapabilities.Add = []string{"CAP_NOT_A_REAL_CAPABILITY"} }, true},
		{"lowercase capability", func(c *CandidateV2) { c.Artifact.ContainerCapabilities.Add = []string{"cap_net_admin"} }, true},
		{"prefixless capability", func(c *CandidateV2) { c.Artifact.ContainerCapabilities.Add = []string{"NET_ADMIN"} }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.mutate(&c)
			if (c.Validate() != nil) != tc.wantErr {
				t.Fatalf("validation mismatch")
			}
		})
	}
	ordered := valid
	ordered.Artifact.ContainerCapabilities.Add = []string{"CAP_CHOWN", "CAP_NET_ADMIN", "CAP_CHOWN"}
	b1, err := ordered.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	canonical := valid
	canonical.Artifact.ContainerCapabilities.Add = []string{"CAP_CHOWN", "CAP_NET_ADMIN"}
	b2, err := canonical.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatal("Add set was not sorted/deduplicated")
	}
	emptyAdd := valid
	emptyAdd.Artifact.ContainerCapabilities.Add = nil
	if err := emptyAdd.Validate(); err != nil {
		t.Fatalf("empty Add should be valid: %v", err)
	}
}

func TestCandidateV2SubstitutionsChangeDigest(t *testing.T) {
	base := candidateV2Fixture()
	baseDigest, err := CandidateDigestV2(base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*CandidateV2)
	}{
		{"target", func(c *CandidateV2) { c.Subject.Target = "Deployment/other" }},
		{"container", func(c *CandidateV2) { c.Subject.Container = "sidecar" }},
		{"image", func(c *CandidateV2) { c.Subject.ImageIdentity = "sha256:" + strings.Repeat("b", 64) }},
		{"capability", func(c *CandidateV2) { c.Artifact.ContainerCapabilities.Add = []string{"CAP_SYS_ADMIN"} }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			got, err := CandidateDigestV2(c)
			if err != nil {
				t.Fatal(err)
			}
			if got == baseDigest {
				t.Fatal("semantic substitution did not change digest")
			}
		})
	}
}

func TestCandidateV1FixtureAndValidationRemainIndependent(t *testing.T) {
	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: "podlock", NetworkPolicy: "network", PatchedManifest: "manifest", SPOSeccompProfile: "seccomp"}
	wantJSON := `{"container":"nginx","binary":"/usr/sbin/nginx","podLock":"podlock","networkPolicy":"network","patchedManifest":"manifest","spoSeccompProfile":"seccomp"}`
	payload := struct {
		Container         string `json:"container"`
		Binary            string `json:"binary"`
		PodLock           string `json:"podLock"`
		NetworkPolicy     string `json:"networkPolicy"`
		PatchedManifest   string `json:"patchedManifest"`
		SPOSeccompProfile string `json:"spoSeccompProfile"`
	}{spec.Container, spec.Binary, spec.PodLock, spec.NetworkPolicy, spec.PatchedManifest, spec.SPOSeccompProfile}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != wantJSON {
		t.Fatalf("v1 canonical bytes changed: %s", b)
	}
	digest, err := CandidateDigest(spec)
	if err != nil {
		t.Fatal(err)
	}
	if digest != "sha256:07c84292f68ba339b68d781a1c876466611b7b322fa4e7a4d9108353a603dc34" {
		t.Fatalf("v1 digest changed: %s", digest)
	}
	status := Status{ApprovalState: ApprovalApproved, ApprovedCandidateDigest: digest, ApprovalMechanismVersion: "candidate-v1"}
	if err := ValidateApprovedCandidate(&spec, &status); err != nil {
		t.Fatalf("v1 validation changed: %v", err)
	}
}

func TestCandidateV2ExcludesV1AndNonCandidateFields(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(CandidateV2{}), reflect.TypeOf(SubjectV2{}), reflect.TypeOf(ArtifactV2{}), reflect.TypeOf(ContainerCapabilitiesV2{})} {
		for i := 0; i < typ.NumField(); i++ {
			name := typ.Field(i).Name
			if strings.Contains(strings.ToLower(name), "binarypath") || strings.Contains(strings.ToLower(name), "provenance") || strings.Contains(strings.ToLower(name), "qualification") || strings.Contains(strings.ToLower(name), "manifest") || strings.Contains(strings.ToLower(name), "event") {
				t.Fatalf("forbidden field %s.%s", typ.Name(), name)
			}
		}
	}
}
