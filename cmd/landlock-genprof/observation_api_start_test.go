package main

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"

	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
)

func newObservationDynamicFakeClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		obskube.GVR: "ObservationList",
	})
}

// startFixture builds a real Deployment -> ReplicaSet -> Pod owner chain
// (matching the already-certified G5 resolver's expected shape) plus a
// kube-system Namespace for ClusterIdentity resolution.
func startFixture(t *testing.T) (*kubefake.Clientset, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: "api-rs", Namespace: "default", UID: "rs-uid",
		Annotations:     map[string]string{"deployment.kubernetes.io/revision": "1"},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "deployment-uid", Controller: boolPtrStart(true)}},
	}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-pod", Namespace: "default", UID: "pod-uid",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-rs", UID: "rs-uid", Controller: boolPtrStart(true)}},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", ContainerID: "containerd://container-id",
			ImageID: "docker.io/library/api@sha256:" + strings.Repeat("b", 64),
		}}},
	}
	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: "cluster-uid"}}
	core := kubefake.NewSimpleClientset(rs, pod, kubeSystem)
	dyn := newObservationDynamicFakeClient()
	return core, dyn
}

func boolPtrStart(v bool) *bool { return &v }

func countObservations(t *testing.T, dyn *dynamicfake.FakeDynamicClient, namespace string) int {
	t.Helper()
	list, err := dyn.Resource(obskube.GVR).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return len(list.Items)
}

// START-1, START-2, START-7: a valid Start creates a durable Observation,
// the returned ID retrieves that exact Observation, and the response does
// not fabricate RUNNING.
func TestObservationAPIProof_StartCreatesDurableObservationAsRequested(t *testing.T) {
	core, dyn := startFixture(t)
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := api.start(context.Background(), startObservationRequest{Pod: "api-pod", Container: "app", Sources: []string{"capabilities"}, Duration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if resp.State != observationdomain.ExecutionRequested {
		t.Fatalf("START-7: returned state = %s, want REQUESTED (must not fabricate RUNNING)", resp.State)
	}
	if resp.ID == "" {
		t.Fatal("START-1: no ObservationID returned")
	}
	// Stop immediately so the background Runner goroutine (which will fail
	// fast against the fake client's incomplete gadget backend) does not
	// race the assertions below; the durable object it already created is
	// what START-1/2 are about, not the run's eventual outcome.
	if _, err := api.stopObservation(context.Background(), "default", resp.ID); err != nil {
		t.Fatal(err)
	}
	got, _, err := api.get(context.Background(), "default", resp.ID)
	if err != nil {
		t.Fatalf("START-2: could not retrieve the created Observation by its returned ID: %v", err)
	}
	if string(got.ID()) != resp.ID {
		t.Fatalf("START-2: retrieved Observation ID = %q, want %q", got.ID(), resp.ID)
	}
	if got.Spec().Target.Slot.Container != "app" {
		t.Fatalf("START-2: retrieved target mismatch: %#v", got.Spec().Target)
	}
	if countObservations(t, dyn, "default") != 1 {
		t.Fatal("START-1: exactly one durable Observation must exist")
	}
}

// START-3: ClusterIdentity resolution failure fails closed, before any
// Observation is created.
func TestObservationAPIProof_StartFailsClosedOnClusterIdentityFailure(t *testing.T) {
	core := kubefake.NewSimpleClientset() // no kube-system Namespace at all
	dyn := newObservationDynamicFakeClient()
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.start(context.Background(), startObservationRequest{Pod: "api-pod", Container: "app", Duration: time.Minute})
	if err == nil || !strings.Contains(err.Error(), "cluster identity unresolved") {
		t.Fatalf("START-3: error = %v, want cluster identity unresolved", err)
	}
	if countObservations(t, dyn, "default") != 0 {
		t.Fatal("START-3: a failed ClusterIdentity resolution must not create an Observation")
	}
}

// START-4: an unresolvable workload target (Pod does not exist) is rejected.
func TestObservationAPIProof_StartRejectsUnresolvableTarget(t *testing.T) {
	core, dyn := startFixture(t)
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.start(context.Background(), startObservationRequest{Pod: "does-not-exist", Container: "app", Duration: time.Minute})
	if err == nil || !strings.Contains(err.Error(), "invalid target") {
		t.Fatalf("START-4: error = %v, want invalid target", err)
	}
	if countObservations(t, dyn, "default") != 0 {
		t.Fatal("START-4: an unresolvable Pod must not create an Observation")
	}
}

// START-5: an unresolvable container (Pod exists, container name does not
// match any ContainerStatus) is rejected.
func TestObservationAPIProof_StartRejectsUnresolvableContainer(t *testing.T) {
	core, dyn := startFixture(t)
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.start(context.Background(), startObservationRequest{Pod: "api-pod", Container: "sidecar-does-not-exist", Duration: time.Minute})
	if err == nil || !strings.Contains(err.Error(), "invalid target") {
		t.Fatalf("START-5: error = %v, want invalid target", err)
	}
	if countObservations(t, dyn, "default") != 0 {
		t.Fatal("START-5: an unresolvable container must not create an Observation")
	}
}

// START-6: invalid sources and invalid duration are each independently
// rejected before any Observation is created.
func TestObservationAPIProof_StartRejectsInvalidSourcesAndDuration(t *testing.T) {
	t.Run("invalid source", func(t *testing.T) {
		core, dyn := startFixture(t)
		api, err := newObservationAPI(core, dyn, "default")
		if err != nil {
			t.Fatal(err)
		}
		_, err = api.start(context.Background(), startObservationRequest{Pod: "api-pod", Container: "app", Sources: []string{"not-a-real-source"}, Duration: time.Minute})
		if err == nil || !strings.Contains(err.Error(), "invalid request") {
			t.Fatalf("SEC-3/START-6: error = %v, want invalid request for an unsupported source", err)
		}
		if countObservations(t, dyn, "default") != 0 {
			t.Fatal("START-6: an invalid source must not create an Observation")
		}
	})
	t.Run("zero duration", func(t *testing.T) {
		core, dyn := startFixture(t)
		api, err := newObservationAPI(core, dyn, "default")
		if err != nil {
			t.Fatal(err)
		}
		_, err = api.start(context.Background(), startObservationRequest{Pod: "api-pod", Container: "app", Duration: 0})
		if err == nil || !strings.Contains(err.Error(), "invalid request") {
			t.Fatalf("START-6: error = %v, want invalid request for zero duration", err)
		}
		if countObservations(t, dyn, "default") != 0 {
			t.Fatal("START-6: zero duration must not create an Observation")
		}
	})
	t.Run("excessive duration", func(t *testing.T) {
		core, dyn := startFixture(t)
		api, err := newObservationAPI(core, dyn, "default")
		if err != nil {
			t.Fatal(err)
		}
		_, err = api.start(context.Background(), startObservationRequest{Pod: "api-pod", Container: "app", Duration: 48 * time.Hour})
		if err == nil || !strings.Contains(err.Error(), "invalid request") {
			t.Fatalf("START-6: error = %v, want invalid request for duration > 24h", err)
		}
		if countObservations(t, dyn, "default") != 0 {
			t.Fatal("START-6: excessive duration must not create an Observation")
		}
	})
}

// SEC-1: a name-based display locator cannot substitute for immutable
// target identity — recreating a Pod under the same name with a new UID
// must resolve to a distinct WorkloadIdentity/RuntimeContainerInstance,
// proving the API does not cache or trust the name alone.
func TestObservationAPIProof_DisplayLocatorCannotSubstituteImmutableIdentity(t *testing.T) {
	core, dyn := startFixture(t)
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	first, err := api.start(context.Background(), startObservationRequest{Pod: "api-pod", Container: "app", Duration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	firstObs, _, err := api.get(context.Background(), "default", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.stopObservation(context.Background(), "default", first.ID); err != nil {
		t.Fatal(err)
	}
	// Recreate "api-pod" (same name) with a different UID and a different
	// owning ReplicaSet/Deployment UID chain, and a different image digest.
	if err := core.CoreV1().Pods("default").Delete(context.Background(), "api-pod", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	rs2 := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: "api-rs-2", Namespace: "default", UID: "rs-uid-2",
		Annotations:     map[string]string{"deployment.kubernetes.io/revision": "2"},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "deployment-uid-2", Controller: boolPtrStart(true)}},
	}}
	if _, err := core.AppsV1().ReplicaSets("default").Create(context.Background(), rs2, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-pod", Namespace: "default", UID: "pod-uid-2",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-rs-2", UID: "rs-uid-2", Controller: boolPtrStart(true)}},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", ContainerID: "containerd://container-id-2",
			ImageID: "docker.io/library/api@sha256:" + strings.Repeat("c", 64),
		}}},
	}
	if _, err := core.CoreV1().Pods("default").Create(context.Background(), pod2, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	second, err := api.start(context.Background(), startObservationRequest{Pod: "api-pod", Container: "app", Duration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	secondObs, _, err := api.get(context.Background(), "default", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.stopObservation(context.Background(), "default", second.ID); err != nil {
		t.Fatal(err)
	}
	firstUID := firstObs.Spec().Target.Slot.Workload.UID
	secondUID := secondObs.Spec().Target.Slot.Workload.UID
	if firstUID == secondUID {
		t.Fatalf("SEC-1: same Pod name resolved to the same WorkloadIdentity UID across a delete/recreate: %q", firstUID)
	}
}
