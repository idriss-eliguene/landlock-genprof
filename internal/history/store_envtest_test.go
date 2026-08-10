//go:build envtest

package history

import (
	"context"
	"os"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg *rest.Config
	env *envtest.Environment
	setupOnce sync.Once
)

func setupEnvtest(t *testing.T) dynamic.Interface {
	var setupErr error
	setupOnce.Do(func() {
		// Initialize envtest once per package
		
		// Determine CRD path
		crdPath := "deploy/crd-traininghistory.yaml"
		if _, err := os.Stat(crdPath); err != nil {
			crdPath = "../../deploy/crd-traininghistory.yaml"
		}
		
		env = &envtest.Environment{
			CRDInstallOptions: envtest.CRDInstallOptions{
				Paths:              []string{crdPath},
				ErrorIfPathMissing: true,
			},
		}

		var err error
		cfg, err = env.Start()
		if err != nil {
			setupErr = err
			return
		}
	})

	if setupErr != nil {
		t.Fatalf("envtest.Start: %v", setupErr)
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
	}
	history.Object["spec"] = spec

	gvr := schema.GroupVersionResource{
		Group:    "landlockgenprof.io",
		Version:  "v1alpha1",
		Resource: "traininghistories",
	}

	resource := client.Resource(gvr).Namespace("default")

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
}
