//go:build envtest

package kubernetes

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var observationEnvConfig *rest.Config
var observationEnv *envtest.Environment

func TestMain(m *testing.M) {
	crdPath := "deploy/crd-observation.yaml"
	if _, err := os.Stat(crdPath); err != nil {
		crdPath = "../../../deploy/crd-observation.yaml"
	}
	observationEnv = &envtest.Environment{CRDInstallOptions: envtest.CRDInstallOptions{Paths: []string{crdPath}, ErrorIfPathMissing: true}}
	var err error
	observationEnvConfig, err = observationEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envtest.Start: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	if err := observationEnv.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "envtest.Stop: %v\n", err)
	}
	os.Exit(code)
}

func observationEnvClient(t *testing.T) dynamic.Interface {
	t.Helper()
	client, err := dynamic.NewForConfig(observationEnvConfig)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestObservationSchemaAndStatusSubresource(t *testing.T) {
	client := observationEnvClient(t)
	ctx := context.Background()
	object, err := ToUnstructured(testObservation(t), "default")
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.Resource(GVR).Namespace("default").Create(ctx, object, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("valid Observation rejected: %v", err)
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(created.Object, "status"); found {
		t.Fatal("create unexpectedly persisted status")
	}

	mutated := created.DeepCopy()
	mutated.Object["spec"].(map[string]interface{})["duration"] = "2m"
	if _, err := client.Resource(GVR).Namespace("default").Update(ctx, mutated, metav1.UpdateOptions{}); err == nil || (!apierrors.IsInvalid(err) && !apierrors.IsBadRequest(err)) {
		t.Fatalf("spec mutation error=%v, want validation failure", err)
	}

	updated := boundObservation(t, testObservation(t))
	status, err := encodeStatus(updated)
	if err != nil {
		t.Fatal(err)
	}
	statusObject := created.DeepCopy()
	statusObject.Object["status"] = status
	if _, err := client.Resource(GVR).Namespace("default").UpdateStatus(ctx, statusObject, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("status subresource update rejected: %v", err)
	}
}

func TestObservationSchemaRejectsOversizedSpec(t *testing.T) {
	client := observationEnvClient(t)
	object, err := ToUnstructured(testObservation(t), "default")
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]interface{}, 17)
	for i := range sources {
		sources[i] = fmt.Sprintf("source-%d", i)
	}
	object.Object["metadata"].(map[string]interface{})["name"] = "oversized-observation"
	object.Object["spec"].(map[string]interface{})["sources"] = sources
	if _, err := client.Resource(GVR).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err == nil || (!apierrors.IsInvalid(err) && !apierrors.IsBadRequest(err)) {
		t.Fatalf("oversized source list error=%v, want validation failure", err)
	}
}

func TestObservationSchemaRejectsTerminalStatusMutation(t *testing.T) {
	client := observationEnvClient(t)
	ctx := context.Background()
	observation := boundObservation(t, testObservation(t))
	for _, state := range []domain.ExecutionState{domain.ExecutionStarting, domain.ExecutionRunning, domain.ExecutionCompleting, domain.ExecutionCompleted} {
		if err := observation.Transition(state, domain.CompletedNormally); err != nil {
			t.Fatal(err)
		}
	}
	object, err := ToUnstructured(observation, "default")
	if err != nil {
		t.Fatal(err)
	}
	object.SetName("terminal-observation")
	created, err := client.Resource(GVR).Namespace("default").Create(ctx, object, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := encodeStatus(observation)
	if err != nil {
		t.Fatal(err)
	}
	statusObject := created.DeepCopy()
	statusObject.Object["status"] = status
	created, err = client.Resource(GVR).Namespace("default").UpdateStatus(ctx, statusObject, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("terminal status rejected: %v", err)
	}
	mutated := created.DeepCopy()
	mutated.Object["status"].(map[string]interface{})["execution"].(map[string]interface{})["completion"] = "BACKEND_FAILURE"
	if _, err := client.Resource(GVR).Namespace("default").UpdateStatus(ctx, mutated, metav1.UpdateOptions{}); err == nil || (!apierrors.IsInvalid(err) && !apierrors.IsBadRequest(err)) {
		t.Fatalf("terminal status mutation error=%v, want validation failure", err)
	}
}
