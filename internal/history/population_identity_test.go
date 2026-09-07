package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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
	for _, name := range []string{"crd-traininghistory.yaml", "crd-observationcontributionreceipt.yaml"} {
		deploy, err := os.ReadFile(filepath.Join(root, "deploy", name))
		if err != nil {
			t.Fatal(err)
		}
		helm, err := os.ReadFile(filepath.Join(root, "deploy", "helm", "landlock-genprof", "crds", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(deploy, helm) {
			t.Fatalf("deploy and Helm %s differ", name)
		}
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

func TestContainerFingerprintAndNameAreDeterministicAndSeparated(t *testing.T) {
	identity := identityPopulation(ScopeContainer, "")
	fingerprint, err := ContainerPopulationFingerprint(mustIdentity(t, identity))
	if err != nil {
		t.Fatal(err)
	}
	if len(fingerprint) != len("sha256:")+64 || fingerprint[:7] != "sha256:" {
		t.Fatalf("fingerprint=%q", fingerprint)
	}
	if again, _ := ContainerPopulationFingerprint(mustIdentity(t, identity)); again != fingerprint {
		t.Fatal("container fingerprint is nondeterministic")
	}
	for _, changed := range []Population{
		identityPopulation(ScopeContainer, ""),
		{Scope: ScopeContainer, Target: "Deployment/other", Container: "app", ImageIdentity: "sha256:image"},
		{Scope: ScopeContainer, Target: "Deployment/api", Container: "sidecar", ImageIdentity: "sha256:image"},
		{Scope: ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:other"},
	} {
		if changed.Target == identity.Target && changed.Container == identity.Container && changed.ImageIdentity == identity.ImageIdentity {
			continue
		}
		other, err := ContainerPopulationFingerprint(mustIdentity(t, changed))
		if err != nil {
			t.Fatal(err)
		}
		if other == fingerprint {
			t.Fatalf("container fingerprint collision for %#v", changed)
		}
	}
	name, err := RecordNameContainerV2(mustIdentity(t, identity))
	if err != nil {
		t.Fatal(err)
	}
	if name != "container-v2-"+fingerprint[len("sha256:"):] || len(name) > 253 {
		t.Fatalf("container name=%q", name)
	}
	binary := mustIdentity(t, identityPopulation(ScopeBinary, "/app/server"))
	binaryFingerprint, err := FingerprintForPopulation(binary)
	if err != nil {
		t.Fatal(err)
	}
	if binaryFingerprint == fingerprint {
		t.Fatal("binary and container fingerprints collided")
	}
	first := mustIdentity(t, Population{Scope: ScopeContainer, Target: "ab", Container: "c", ImageIdentity: "sha256:image"})
	second := mustIdentity(t, Population{Scope: ScopeContainer, Target: "a", Container: "bc", ImageIdentity: "sha256:image"})
	firstBytes, err := ContainerPopulationCanonicalBytes(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := ContainerPopulationCanonicalBytes(second)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("length-prefixed container encodings collided at field boundary")
	}
}

func mustIdentity(t *testing.T, population Population) PopulationIdentity {
	t.Helper()
	identity, err := population.Identity()
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestContainerSnapshotPersistenceAndScopeLookup(t *testing.T) {
	identity := mustIdentity(t, identityPopulation(ScopeContainer, ""))
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := SavePopulationSnapshot(context.Background(), client, "default", identity, &Record{}); err != nil {
		t.Fatal(err)
	}
	got, err := GetPopulation(context.Background(), client, "default", identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Populations) != 1 || got.Populations[0].Scope != ScopeContainer || got.Populations[0].BinaryPath != "" {
		t.Fatalf("container persistence=%#v", got.Populations)
	}
	wrong := mustIdentity(t, Population{Scope: ScopeContainer, Target: "Deployment/wrong", Container: "app", ImageIdentity: "sha256:image"})
	locator, err := RecordNameContainerV2(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(context.Background(), client, "default", locator, &Record{Populations: []Population{{
		Scope:         wrong.Scope,
		Target:        wrong.Target,
		Container:     wrong.Container,
		ImageIdentity: wrong.ImageIdentity,
	}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := GetPopulation(context.Background(), client, "default", identity); err == nil {
		t.Fatal("wrong full identity unexpectedly matched")
	}
}
