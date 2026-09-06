package history

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func identityPopulation(scope PopulationScope, binary string) Population {
	return Population{Scope: scope, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:image", BinaryPath: binary}
}

func TestPopulationScopeValidationAndLegacyNormalization(t *testing.T) {
	tests := []struct {
		name  string
		input Population
		want  PopulationScope
		err   bool
	}{
		{"absent scope with binary", identityPopulation("", "/app/server"), ScopeBinary, false},
		{"absent scope empty binary", identityPopulation("", ""), "", true},
		{"explicit binary", identityPopulation(ScopeBinary, "/app/server"), ScopeBinary, false},
		{"explicit binary empty", identityPopulation(ScopeBinary, ""), "", true},
		{"explicit container absent binary", identityPopulation(ScopeContainer, ""), ScopeContainer, false},
		{"explicit container binary", identityPopulation(ScopeContainer, "/app/server"), "", true},
		{"unknown scope", identityPopulation("PROCESS", ""), "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeLegacyScope(tc.input)
			if (err != nil) != tc.err {
				t.Fatalf("error=%v, want error=%t", err, tc.err)
			}
			if err == nil && got.Scope != tc.want {
				t.Fatalf("scope=%q, want %q", got.Scope, tc.want)
			}
		})
	}
}

func TestPopulationIdentityMatchingIsScopeSeparated(t *testing.T) {
	binary := identityPopulation(ScopeBinary, "/app/server")
	container := identityPopulation(ScopeContainer, "")
	if !PopulationIdentityEqual(binary, binary) {
		t.Fatal("identical binary populations did not match")
	}
	if PopulationIdentityEqual(binary, container) || PopulationIdentityEqual(container, binary) {
		t.Fatal("binary and container populations cross-matched")
	}
	if !PopulationIdentityEqual(container, identityPopulation(ScopeContainer, "")) {
		t.Fatal("identical container populations did not match")
	}
}

func TestLegacyBinaryFingerprintAndNamesRemainUnchanged(t *testing.T) {
	fingerprint := PopulationFingerprint{Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:image", BinaryPath: "/app/server"}
	population := identityPopulation(ScopeBinary, fingerprint.BinaryPath)
	if populationFingerprint(population) != fingerprint {
		t.Fatal("binary fingerprint changed after adding scope")
	}
	if got := RecordNameLegacy("app", "/app/server"); got != "app-server" {
		t.Fatalf("legacy record name=%q", got)
	}
	sum := sha256.Sum256([]byte("/app/server"))
	wantV2 := "app-server-" + hex.EncodeToString(sum[:])[:8]
	if got := RecordNameV2("app", "/app/server"); got != wantV2 {
		t.Fatalf("V2 record name=%q, want %q", got, wantV2)
	}
}

func TestDeployHelmTrainingHistoryCRDParity(t *testing.T) {
	root := filepath.Join("..", "..")
	deploy, err := os.ReadFile(filepath.Join(root, "deploy", "crd-traininghistory.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	helm, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "landlock-genprof", "crds", "crd-traininghistory.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(deploy, helm) {
		t.Fatal("deploy and Helm TrainingHistory CRDs differ")
	}
}

func TestLegacyPopulationDecodeNormalizesWithoutRewrite(t *testing.T) {
	record := &Record{Populations: []Population{{Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:image", BinaryPath: "/app/server"}}}
	decoded, err := fromUnstructured(toUnstructured("default", "legacy", record))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Populations) != 1 || decoded.Populations[0].Scope != ScopeBinary {
		t.Fatalf("legacy scope=%q, want %q", decoded.Populations[0].Scope, ScopeBinary)
	}
	if record.Populations[0].Scope != "" {
		t.Fatal("legacy record was rewritten during decode")
	}
}
