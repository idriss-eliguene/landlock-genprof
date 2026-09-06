//go:build envtest

package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
	crdPath := os.Getenv("OBSERVATION_CRD_PATH")
	if crdPath == "" {
		crdPath = "deploy/crd-observation.yaml"
		if _, err := os.Stat(crdPath); err != nil {
			crdPath = "../../../deploy/crd-observation.yaml"
		}
	}
	if _, err := os.Stat(crdPath); err != nil {
		fmt.Fprintf(os.Stderr, "Observation CRD source %q is unavailable: %v\n", crdPath, err)
		os.Exit(1)
	}
	if filepath.Base(crdPath) != "crd-observation.yaml" {
		fmt.Fprintf(os.Stderr, "unexpected Observation CRD source %q\n", crdPath)
		os.Exit(1)
	}
	deployCRD, err := os.ReadFile("../../../deploy/crd-observation.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read deploy Observation CRD: %v\n", err)
		os.Exit(1)
	}
	helmCRD, err := os.ReadFile("../../../deploy/helm/landlock-genprof/crds/crd-observation.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read Helm Observation CRD: %v\n", err)
		os.Exit(1)
	}
	if string(deployCRD) != string(helmCRD) {
		fmt.Fprintln(os.Stderr, "deploy and Helm Observation CRDs differ")
		os.Exit(1)
	}
	observationEnv = &envtest.Environment{CRDInstallOptions: envtest.CRDInstallOptions{Paths: []string{crdPath}, ErrorIfPathMissing: true}}
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

func TestDeployAndHelmObservationCRDParity(t *testing.T) {
	deployCRD, err := os.ReadFile("../../../deploy/crd-observation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	helmCRD, err := os.ReadFile("../../../deploy/helm/landlock-genprof/crds/crd-observation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(deployCRD, helmCRD) {
		t.Fatal("deploy and Helm Observation CRDs differ")
	}
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

func TestProvenanceResolvedTargetRoundTripsWithoutPruning(t *testing.T) {
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
	object.SetName("provenance-roundtrip")
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
	if _, err := client.Resource(GVR).Namespace("default").UpdateStatus(ctx, statusObject, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	fetched, err := client.Resource(GVR).Namespace("default").Get(ctx, "provenance-roundtrip", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	resolvedTargets, found, err := unstructured.NestedSlice(fetched.Object, "status", "provenance", "resolvedTargets")
	if err != nil || !found || len(resolvedTargets) != 1 {
		t.Fatalf("provenance resolved target missing: found=%v len=%d err=%v", found, len(resolvedTargets), err)
	}
	resolved, ok := resolvedTargets[0].(map[string]interface{})
	if !ok {
		t.Fatalf("provenance resolved target has unexpected type %T", resolvedTargets[0])
	}
	if got, _, _ := unstructured.NestedString(resolved, "slot", "container"); got != "backend" {
		t.Fatalf("container=%q, want backend", got)
	}
	workload, found, err := unstructured.NestedMap(resolved, "slot", "workload")
	if err != nil || !found {
		t.Fatalf("provenance workload missing: found=%v err=%v", found, err)
	}
	checks := map[string]string{"namespace": "workloads", "name": "api", "uid": "workload-uid"}
	for field, want := range checks {
		if got, _, _ := unstructured.NestedString(workload, field); got != want {
			t.Errorf("workload %s=%q, want %q", field, got, want)
		}
	}
	if got, _, _ := unstructured.NestedString(workload, "cluster", "namespaceUID"); got != "cluster-uid" {
		t.Errorf("cluster namespaceUID=%q", got)
	}
	if got, _, _ := unstructured.NestedString(workload, "groupKind", "group"); got != "apps" {
		t.Errorf("groupKind group=%q", got)
	}
	if got, _, _ := unstructured.NestedString(workload, "groupKind", "kind"); got != "Deployment" {
		t.Errorf("groupKind kind=%q", got)
	}
}

func TestConcurrentInitialClaims(t *testing.T) {
	client := observationEnvClient(t)
	object, err := ToUnstructured(testObservation(t), "default")
	if err != nil {
		t.Fatal(err)
	}
	object.SetName("claim-race")
	if _, err := client.Resource(GVR).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(client)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, executorID := range []string{"race-a", "race-b"} {
		wait.Add(1)
		go func(id string) {
			defer wait.Done()
			_, _, err := store.ClaimObservation(context.Background(), "default", "claim-race", id)
			results <- err
		}(executorID)
	}
	wait.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrAlreadyClaimed) && !errors.Is(err, ErrConcurrentConflict) {
			t.Fatalf("unexpected losing claim error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent claim winners=%d, want 1", wins)
	}
}

func TestConcurrentLostExecutorRecovery(t *testing.T) {
	client := observationEnvClient(t)
	ctx := context.Background()
	object, err := ToUnstructured(testObservation(t), "default")
	if err != nil {
		t.Fatal(err)
	}
	object.SetName("recovery-race")
	_, err = client.Resource(GVR).Namespace("default").Create(ctx, object, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(client)
	if err != nil {
		t.Fatal(err)
	}
	claim, _, err := store.ClaimObservation(ctx, "default", "recovery-race", "recovery-owner")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := client.Resource(GVR).Namespace("default").Get(ctx, "recovery-race", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := encodeStatusWithClaim(domainObservationStarting(t), claimRecord{ExecutorID: claim.ExecutorID, Generation: claim.ClaimGeneration, LeaseExpiry: time.Now().UTC().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	raw.Object["status"] = status
	if _, err := client.Resource(GVR).Namespace("default").UpdateStatus(ctx, raw, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.TerminalizeExecutorLost(ctx, "default", "recovery-race")
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrTerminalObservation) && !errors.Is(err, ErrConcurrentConflict) {
			t.Fatalf("unexpected recovery error: %v", err)
		}
	}
	if wins > 1 {
		t.Fatalf("concurrent recovery winners=%d, want at most one", wins)
	}
}

func domainObservationStarting(t *testing.T) domain.Observation {
	t.Helper()
	observation := testObservation(t)
	if err := observation.Transition(domain.ExecutionStarting, ""); err != nil {
		t.Fatal(err)
	}
	return observation
}
