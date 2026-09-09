package executor

import (
	"context"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestConfigRequiresDedicatedInClusterClientsAndNamespaces(t *testing.T) {
	if err := (Config{}).validate(); err == nil {
		t.Fatal("empty executor configuration unexpectedly validated")
	}
}

func TestNextRequestedDoesNotClaimNonRequestedWork(t *testing.T) {
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		kubernetes.GVR: "ObservationList",
	})
	store, err := kubernetes.NewStore(client)
	if err != nil {
		t.Fatal(err)
	}
	observation, found, err := nextRequested(context.Background(), store, "default")
	if err != nil {
		t.Fatal(err)
	}
	if found || observation.ID() != "" {
		t.Fatalf("empty namespace returned work: found=%v observation=%#v", found, observation)
	}
}
