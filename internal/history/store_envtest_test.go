//go:build envtest

package history

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

var (
	cfg *rest.Config
	env *envtest.Environment
)

func TestMain(m *testing.M) {
	// Start envtest once per package.
	// All tests in this package share a single API server instance for efficiency.

	// Install the packaged Helm copies explicitly. The deploy copies are
	// checked for parity by the non-envtest regression test.
	crdRoot := filepath.Join("deploy", "helm", "landlock-genprof", "crds")
	if _, err := os.Stat(crdRoot); err != nil {
		crdRoot = filepath.Join("..", "..", "deploy", "helm", "landlock-genprof", "crds")
	}
	crdPaths := []string{
		filepath.Join(crdRoot, "crd-traininghistory.yaml"),
		filepath.Join(crdRoot, "crd-observationcontributionreceipt.yaml"),
	}

	env = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths:              crdPaths,
			ErrorIfPathMissing: true,
		},
	}

	var err error
	cfg, err = env.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envtest.Start: %v\n", err)
		os.Exit(1)
	}

	// Run all tests
	code := m.Run()

	// Explicit cleanup
	if env != nil {
		env.Stop()
	}

	os.Exit(code)
}

func TestTrainingHistoryAndReceiptCRDParity(t *testing.T) {
	root := "deploy"
	if _, err := os.Stat(root); err != nil {
		root = "../../deploy"
	}
	paths := []string{
		filepath.Join(root, "crd-traininghistory.yaml"),
		filepath.Join(root, "helm/landlock-genprof/crds/crd-traininghistory.yaml"),
		filepath.Join(root, "crd-observationcontributionreceipt.yaml"),
		filepath.Join(root, "helm/landlock-genprof/crds/crd-observationcontributionreceipt.yaml"),
	}
	for i := 0; i < len(paths); i += 2 {
		left, err := os.ReadFile(paths[i])
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(paths[i+1])
		if err != nil {
			t.Fatal(err)
		}
		if string(left) != string(right) {
			t.Fatalf("CRD copies differ: %s and %s", paths[i], paths[i+1])
		}
	}
}

func TestReceiptCRDRoundTrip(t *testing.T) {
	client := setupEnvtest(t)
	store, err := NewReceiptStore(client)
	if err != nil {
		t.Fatal(err)
	}
	key := provenanceKey()
	created, rv, err := store.CreatePrepared(context.Background(), "default", key, "default", "history", "")
	if err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	if created.State != ReceiptPrepared || created.ContributionKeyDigest == "" {
		t.Fatalf("created receipt = %#v", created)
	}
	fetched, fetchedRV, err := store.Get(context.Background(), "default", key)
	if err != nil || fetched == nil || fetchedRV != rv {
		t.Fatalf("Get: %#v, %s, %v", fetched, fetchedRV, err)
	}
	committed, _, err := store.Commit(context.Background(), "default", key, rv)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.State != ReceiptCommitted {
		t.Fatalf("committed receipt = %#v", committed)
	}
}

func TestTrainingHistoryMetadataCRDRoundTrip(t *testing.T) {
	client := setupEnvtest(t)
	key := provenanceKey()
	digest, err := key.Digest()
	if err != nil {
		t.Fatal(err)
	}
	record := &Record{Populations: []Population{{Scope: ScopeBinary, Target: key.Population.Target, Container: key.Population.Container, ImageIdentity: key.Population.ImageIdentity, BinaryPath: key.Population.BinaryPath, ObservationContributions: []ObservationContribution{{ObservationID: key.ObservationID, Sources: []ObservationSourceContribution{{Source: "exec", EvidenceState: "UNKNOWN", AttributionState: "COMPLETED", AttributedCount: 1}}}}, PendingContributionMarkers: []ContributionMarker{{ObservationID: key.ObservationID, Population: key.Population, KeyDigest: digest}}}}}
	obj := toUnstructured("default", "metadata-history", record)
	resource := client.Resource(trainingHistoryGVR).Namespace("default")
	if _, err := resource.Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	fetched, err := resource.Get(context.Background(), "metadata-history", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := fromUnstructured(fetched)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Populations[0].ObservationContributions) != 1 || len(decoded.Populations[0].PendingContributionMarkers) != 1 {
		t.Fatalf("metadata lost: %#v", decoded.Populations[0])
	}
}

func TestContributionProtocolEnvtest(t *testing.T) {
	client := setupEnvtest(t)
	c := testContribution()
	c.ObservationID = "envtest-observation"
	first, err := ApplyContribution(context.Background(), client, "default", c)
	if err != nil || first != ContributionApplied {
		t.Fatalf("first contribution = %s, %v", first, err)
	}
	replay, err := ApplyContribution(context.Background(), client, "default", c)
	if err != nil || replay != ContributionAlreadyCommitted {
		t.Fatalf("replay = %s, %v", replay, err)
	}
	receipts := client.Resource(contributionReceiptGVR).Namespace("default")
	key := ContributionKey{ObservationID: c.ObservationID, Population: c.Population}
	name, err := key.ReceiptName()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := receipts.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Object["status"].(map[string]interface{})["state"] != string(ReceiptCommitted) {
		t.Fatalf("receipt = %#v", receipt.Object["status"])
	}
	historyName := RecordNameV2(c.Population.Container, c.Population.BinaryPath)
	history, err := client.Resource(trainingHistoryGVR).Namespace("default").Get(context.Background(), historyName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	populations, found, err := unstructured.NestedSlice(history.Object, "spec", "populations")
	if err != nil || !found || len(populations) != 1 {
		t.Fatalf("populations = %#v, found=%t err=%v", populations, found, err)
	}
	population := populations[0].(map[string]interface{})
	if _, found := population["observationContributions"]; !found {
		t.Fatal("audit provenance missing")
	}
	if _, found := population["pendingContributionMarkers"]; found {
		t.Fatal("marker was not cleaned after commit")
	}
}

func TestObservationContributionEnvtest(t *testing.T) {
	client := setupEnvtest(t)
	observation := testContributionObservation(t, observationdomain.ExecutionCompleted)
	first, err := ApplyObservationContribution(context.Background(), client, "default", observation)
	if err != nil || first != ContributionApplied {
		t.Fatalf("first observation contribution = %s, %v", first, err)
	}
	replay, err := ApplyObservationContribution(context.Background(), client, "default", observation)
	if err != nil || replay != ContributionAlreadyCommitted {
		t.Fatalf("observation replay = %s, %v", replay, err)
	}
	identity := PopulationIdentity{Scope: ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	historyName, err := RecordNameContainerV2(identity)
	if err != nil {
		t.Fatal(err)
	}
	record, err := Get(context.Background(), client, "default", historyName)
	if err != nil || record == nil || len(record.Populations) != 1 {
		t.Fatalf("observation history = %#v, %v", record, err)
	}
	if record.Populations[0].Scope != ScopeContainer || record.Populations[0].BinaryPath != "" || record.Populations[0].RunsRecorded != 0 || len(record.Populations[0].FilesystemAccesses) != 1 || len(record.Populations[0].NetworkAccesses) != 2 || len(record.Populations[0].CapabilityAccesses) != 1 {
		t.Fatalf("persisted observation population = %#v", record.Populations[0])
	}
	key := ContributionKey{ObservationID: string(observation.ID()), Population: PopulationFingerprint{Scope: ScopeContainer, Target: identity.Target, Container: identity.Container, ImageIdentity: identity.ImageIdentity}}
	receiptName, err := key.ReceiptName()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.Resource(contributionReceiptGVR).Namespace("default").Get(context.Background(), receiptName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	population, found, err := unstructured.NestedMap(receipt.Object, "spec", "populationFingerprint")
	if err != nil || !found || population["scope"] != string(ScopeContainer) || population["binaryPath"] != nil {
		t.Fatalf("persisted receipt identity = %#v, found=%t, err=%v", population, found, err)
	}
}

func setupEnvtest(t *testing.T) dynamic.Interface {
	if cfg == nil {
		t.Fatal("envtest not initialized (TestMain may not have run)")
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}
	return dynamicClient
}

// TestTrainingHistoryCRDRoundTrip validates that TrainingHistory objects persist through the real CRD.
func TestTrainingHistoryCRDRoundTrip(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()

	// Create a TrainingHistory with known spec fields
	history := &unstructured.Unstructured{}
	history.SetAPIVersion("landlockgenprof.io/v1alpha1")
	history.SetKind("TrainingHistory")
	history.SetName("test-history")
	history.SetNamespace("default")

	// Set spec fields according to CRD schema
	spec := map[string]interface{}{
		"container":    "nginx",
		"binary":       "/usr/sbin/nginx",
		"runsRecorded": int64(5),
		"filesystemAccesses": []interface{}{
			map[string]interface{}{
				"path":        "/etc/nginx",
				"permissions": []interface{}{"read"},
				"seenInRuns":  int64(5),
			},
		},
		"populations": []interface{}{
			map[string]interface{}{
				"qualified": true,
				"target":    "nginx-demo", "container": "nginx",
				"imageIdentity": "docker-pullable://nginx@sha256:abc", "binaryPath": "/usr/sbin/nginx",
				"runsRecorded": int64(2), "contributors": []interface{}{"pod-1", "pod-2"},
				"filesystemAccesses": []interface{}{map[string]interface{}{
					"path": "/etc/nginx", "permissions": []interface{}{"read"}, "seenInRuns": int64(2),
				}},
				"networkAccesses": []interface{}{map[string]interface{}{
					"port": int64(443), "direction": "egress", "seenInRuns": int64(1),
				}},
				"syscallAccesses": []interface{}{map[string]interface{}{
					"name": "openat", "seenInRuns": int64(2),
				}},
				"capabilityAccesses": []interface{}{map[string]interface{}{
					"name": "NET_BIND_SERVICE", "seenInRuns": int64(1),
				}},
			},
		},
	}
	history.Object["spec"] = spec

	resource := client.Resource(trainingHistoryGVR).Namespace("default")

	// Create the TrainingHistory
	created, err := resource.Create(ctx, history, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create TrainingHistory: %v", err)
	}

	if created.GetName() != "test-history" {
		t.Errorf("Created object name: got %s, want test-history", created.GetName())
	}

	// Fetch it back from the real API server
	fetched, err := resource.Get(ctx, "test-history", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get TrainingHistory: %v", err)
	}

	if fetched.GetName() != "test-history" {
		t.Errorf("Fetched object name: got %s, want test-history", fetched.GetName())
	}

	// Verify spec is present (proves the real API server validated and stored the object)
	fetchedSpec, ok := fetched.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("Spec missing after round-trip through real API server")
	}

	// Verify key spec fields are present
	if _, hasContainer := fetchedSpec["container"]; !hasContainer {
		t.Error("container field missing from spec after round-trip")
	}

	if _, hasBinary := fetchedSpec["binary"]; !hasBinary {
		t.Error("binary field missing from spec after round-trip")
	}
	populations, hasPopulations := fetchedSpec["populations"].([]interface{})
	if !hasPopulations || len(populations) != 1 {
		t.Fatalf("populations field missing from spec after round-trip: %#v", fetchedSpec["populations"])
	}
	population, ok := populations[0].(map[string]interface{})
	if !ok {
		t.Fatal("population has unexpected type after round-trip")
	}
	assertNested := func(field, key string, want interface{}) {
		t.Helper()
		items, ok := population[field].([]interface{})
		if !ok || len(items) != 1 {
			t.Fatalf("%s missing after round-trip: %#v", field, population[field])
		}
		item, ok := items[0].(map[string]interface{})
		if !ok || item[key] != want {
			t.Fatalf("%s.%s = %#v, want %#v", field, key, item[key], want)
		}
	}
	assertNested("filesystemAccesses", "path", "/etc/nginx")
	assertNested("networkAccesses", "port", int64(443))
	assertNested("syscallAccesses", "name", "openat")
	assertNested("capabilityAccesses", "name", "NET_BIND_SERVICE")
}

func TestTrainingHistoryCanonicalTargetBindingRoundTrip(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "landlockgenprof.io/v1alpha1",
		"kind":       "TrainingHistory",
		"metadata":   map[string]interface{}{"name": "bound-history", "namespace": "default"},
		"spec": map[string]interface{}{
			"populations": []interface{}{map[string]interface{}{
				"qualified": true, "target": "Deployment/api", "container": "app",
				"imageIdentity": "sha256:a", "binaryPath": "/app/server",
				"targetBinding": map[string]interface{}{"namespace": "team-a", "group": "apps", "kind": "Deployment", "name": "api"},
			}},
		},
	}}
	resource := client.Resource(trainingHistoryGVR).Namespace("default")
	if _, err := resource.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create TrainingHistory: %v", err)
	}
	fetched, err := resource.Get(ctx, "bound-history", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get TrainingHistory: %v", err)
	}
	populations, found, err := unstructured.NestedSlice(fetched.Object, "spec", "populations")
	if err != nil || !found || len(populations) != 1 {
		t.Fatalf("populations = %#v, found=%t, err=%v", populations, found, err)
	}
	binding, found, err := unstructured.NestedMap(populations[0].(map[string]interface{}), "targetBinding")
	if err != nil || !found || binding["namespace"] != "team-a" || binding["group"] != "apps" || binding["kind"] != "Deployment" || binding["name"] != "api" {
		t.Fatalf("targetBinding = %#v, found=%t, err=%v", binding, found, err)
	}
}
