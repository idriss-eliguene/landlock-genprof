package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// lifecycleObservation builds a domain Observation pinned at an arbitrary
// point in the REQUESTED/STARTING/RUNNING/COMPLETING/COMPLETED/FAILED
// lifecycle, so Stop can be proven against every state without depending on
// the asynchronous Runner to actually drive a real transition.
func lifecycleObservation(t *testing.T, id string, state domain.ExecutionState, completion domain.CompletionReason) domain.Observation {
	t.Helper()
	cluster, err := domain.NewClusterIdentity("cluster-stop")
	if err != nil {
		t.Fatal(err)
	}
	workload := domain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "workload-stop"}
	slot := domain.ContainerSlot{Workload: workload, Container: "app"}
	spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: slot}, []string{"capabilities"}, time.Minute, "g8-stop")
	if err != nil {
		t.Fatal(err)
	}
	execution := domain.ObservationExecution{State: state, Completion: completion}

	var binding domain.ObservationBinding
	var result domain.ObservationResult
	var provenance domain.ObservationProvenance

	if state == domain.ExecutionCompleted || state == domain.ExecutionFailed {
		revision, err := domain.NewContainerImageRevision(slot, "sha256:"+strings.Repeat("a", 64))
		if err != nil {
			t.Fatal(err)
		}
		targets, err := domain.NewResolvedTargetSet([]domain.RuntimeContainerInstance{{Slot: slot, PodUID: id + "-pod", ContainerID: id + "-container"}})
		if err != nil {
			t.Fatal(err)
		}
		q := domain.SourceQualification{SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted, AttributedCount: 1}
		sourceResult, err := domain.NewSourceResult(domain.EvidenceSource{Name: "capabilities", Backend: "proof", Version: "v1"}, q, nil, domain.NormalizedFacts{Capabilities: []domain.CapabilityFact{{Name: "CAP_CHOWN"}}})
		if err != nil {
			t.Fatal(err)
		}
		result, err = domain.NewObservationResult([]domain.SourceResult{sourceResult})
		if err != nil {
			t.Fatal(err)
		}
		binding = domain.ObservationBinding{ResolvedTargets: targets, Backend: domain.BackendIdentity{Kind: "proof", Version: "v1"}, ImageRevisions: []domain.ContainerImageRevision{revision}}
		provenance = domain.ObservationProvenance{Backend: binding.Backend, RequestedSources: []string{"capabilities"}}
	}

	observation, err := domain.RestoreObservation(domain.ObservationID(id), spec, binding, execution, result, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

// seedLifecycleObservation persists a lifecycleObservation directly into the
// fake dynamic store, bypassing the executor layer, mirroring the seeding
// approach already established in observation_api_proof_test.go but
// generalized across every non-terminal state as well.
func seedLifecycleObservation(t *testing.T, dyn *dynamicfake.FakeDynamicClient, namespace string, observation domain.Observation) {
	t.Helper()
	object, err := obskube.ToUnstructured(observation, namespace)
	if err != nil {
		t.Fatal(err)
	}

	resolvedTargets := []interface{}{}
	imageRevisions := []interface{}{}
	sources := []interface{}{}
	backend := map[string]interface{}{"kind": "", "version": ""}
	requestedSources := []interface{}{}

	if observation.Frozen() {
		slot := observation.Binding().ResolvedTargets.Items()[0]
		source := observation.Result().Sources()[0]
		capabilities := make([]interface{}, 0, len(source.Facts.Capabilities))
		for _, fact := range source.Facts.Capabilities {
			capabilities = append(capabilities, map[string]interface{}{"name": fact.Name})
		}
		workloadMap := map[string]interface{}{"cluster": map[string]interface{}{"namespaceUID": string(slot.Slot.Workload.Cluster.NamespaceUID)}, "namespace": slot.Slot.Workload.Namespace, "groupKind": map[string]interface{}{"group": slot.Slot.Workload.GroupKind.Group, "kind": slot.Slot.Workload.GroupKind.Kind}, "name": slot.Slot.Workload.Name, "uid": slot.Slot.Workload.UID}
		slotMap := map[string]interface{}{"workload": workloadMap, "container": slot.Slot.Container}
		resolvedTargets = []interface{}{map[string]interface{}{"slot": slotMap, "podUID": slot.PodUID, "containerID": slot.ContainerID}}
		imageRevisions = []interface{}{map[string]interface{}{"slot": slotMap, "imageDigest": "sha256:" + strings.Repeat("a", 64)}}
		backend = map[string]interface{}{"kind": "proof", "version": "v1"}
		requestedSources = []interface{}{"capabilities"}
		sources = []interface{}{map[string]interface{}{
			"name": source.Source.Name, "backend": source.Source.Backend, "version": source.Source.Version, "evidence": string(source.Evidence),
			"qualification": map[string]interface{}{"backendHealthConfirmed": source.Qualification.BackendHealthConfirmed, "sourceAttachedForBoundWindow": source.Qualification.SourceAttachedForBoundWindow, "flushConfirmed": source.Qualification.FlushConfirmed, "attribution": string(source.Qualification.Attribution), "attributedCount": float64(source.Qualification.AttributedCount), "excludedCount": float64(source.Qualification.ExcludedCount)},
			"facts":         map[string]interface{}{"capabilities": capabilities},
		}}
	}

	status := map[string]interface{}{
		"binding": map[string]interface{}{
			"resolvedTargets": resolvedTargets,
			"backend":         backend,
			"imageRevisions":  imageRevisions,
			"targetChanges":   []interface{}{},
		},
		"execution": map[string]interface{}{"state": string(observation.Execution().State), "completion": string(observation.Execution().Completion)},
		"result":    map[string]interface{}{"sources": sources},
		"provenance": map[string]interface{}{
			"resolvedTargets": []interface{}{}, "imageRevisions": []interface{}{}, "backend": backend, "requestedSources": requestedSources,
		},
	}
	object.Object["status"] = status
	if _, err := dyn.Resource(obskube.GVR).Namespace(namespace).Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func newStopFixtureClients() (*dynamicfake.FakeDynamicClient, *observationAPI) {
	dyn := newObservationDynamicFakeClient()
	api := &observationAPI{client: nil, dynamic: dyn, namespace: "default", stop: make(map[string]context.CancelFunc)}
	return dyn, api
}

// registerCancel wires a counting CancelFunc into the API's internal stop
// registry, the same map the real start() populates, so Stop's cancel-only-
// if-not-frozen behavior can be observed deterministically without running
// the asynchronous Runner goroutine.
func registerCancel(api *observationAPI, id string) *int32 {
	var calls int32
	api.mu.Lock()
	api.stop[id] = func() { atomic.AddInt32(&calls, 1) }
	api.mu.Unlock()
	return &calls
}

// STOP-1: a valid Stop on a RUNNING (non-frozen) Observation invokes the
// registered cancel function.
func TestObservationAPIProof_StopCancelsRunningObservation(t *testing.T) {
	dyn, api := newStopFixtureClients()
	observation := lifecycleObservation(t, "g8-stop-running", domain.ExecutionRunning, "")
	seedLifecycleObservation(t, dyn, "default", observation)
	calls := registerCancel(api, "g8-stop-running")
	if _, err := api.stopObservation(context.Background(), "default", "g8-stop-running"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(calls) == 0 {
		t.Fatal("STOP-1: cancel was not invoked for a non-frozen RUNNING Observation")
	}
}

// STOP-2: Stop on an already-frozen (COMPLETED) Observation must NOT invoke
// cancel, even if a stale registration is still present — frozen evidence
// must never be disturbed by a late Stop call.
func TestObservationAPIProof_StopDoesNotCancelFrozenObservation(t *testing.T) {
	dyn, api := newStopFixtureClients()
	observation := lifecycleObservation(t, "g8-stop-completed", domain.ExecutionCompleted, domain.CompletedNormally)
	seedLifecycleObservation(t, dyn, "default", observation)
	calls := registerCancel(api, "g8-stop-completed")
	resp, err := api.stopObservation(context.Background(), "default", "g8-stop-completed")
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(calls) != 0 {
		t.Fatal("STOP-2: cancel was invoked for an already-frozen Observation")
	}
	if resp.State != domain.ExecutionCompleted || !resp.Frozen {
		t.Fatalf("STOP-2: response = %#v, want unchanged frozen COMPLETED state", resp)
	}
}

// STOP-3: Stop against an unknown ObservationID is rejected as not found.
func TestObservationAPIProof_StopUnknownIDNotFound(t *testing.T) {
	_, api := newStopFixtureClients()
	_, err := api.stopObservation(context.Background(), "default", "does-not-exist")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("STOP-3: error = %v, want not found", err)
	}
}

// STOP-4: Stop rejects a caller-supplied namespace outside the Workbench's
// own pinned read scope.
func TestObservationAPIProof_StopRejectsNamespaceOverride(t *testing.T) {
	dyn, api := newStopFixtureClients()
	observation := lifecycleObservation(t, "g8-stop-ns", domain.ExecutionRunning, "")
	seedLifecycleObservation(t, dyn, "default", observation)
	_, err := api.stopObservation(context.Background(), "other-namespace", "g8-stop-ns")
	if err == nil || !strings.Contains(err.Error(), "outside the Workbench") {
		t.Fatalf("STOP-4: error = %v, want outside the Workbench", err)
	}
}

func TestObservationAPIProof_AuthenticatedStopRejectsSessionNamespaceMismatch(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/observations/stop", nil)
	req.Header.Set("X-Environment-Namespace", "security")
	if authenticatedRequestNamespaceMatches(req, "payments") {
		t.Fatal("authenticated stop accepted a namespace outside its bound environment context")
	}
}

// STOP-5: repeated Stop calls against the same RUNNING Observation are safe
// — context.CancelFunc is itself idempotent, so Stop may invoke it more than
// once without harm; this deliberately does not assert exactly-once.
func TestObservationAPIProof_RepeatedStopIsSafe(t *testing.T) {
	dyn, api := newStopFixtureClients()
	observation := lifecycleObservation(t, "g8-stop-repeat", domain.ExecutionRunning, "")
	seedLifecycleObservation(t, dyn, "default", observation)
	calls := registerCancel(api, "g8-stop-repeat")
	for i := 0; i < 3; i++ {
		if _, err := api.stopObservation(context.Background(), "default", "g8-stop-repeat"); err != nil {
			t.Fatalf("repeat #%d: %v", i, err)
		}
	}
	if atomic.LoadInt32(calls) == 0 {
		t.Fatal("STOP-5: repeated Stop never invoked cancel at all")
	}
}

// Full 6-state lifecycle table: Stop must succeed (truthfully reflecting
// accumulated evidence) against every state, cancel only for non-frozen
// states, and must never mutate the persisted Observation.
func TestObservationAPIProof_StopAcrossFullLifecycle(t *testing.T) {
	cases := []struct {
		name       string
		state      domain.ExecutionState
		completion domain.CompletionReason
		frozen     bool
	}{
		{"REQUESTED", domain.ExecutionRequested, "", false},
		{"STARTING", domain.ExecutionStarting, "", false},
		{"RUNNING", domain.ExecutionRunning, "", false},
		{"COMPLETING", domain.ExecutionCompleting, "", false},
		{"COMPLETED", domain.ExecutionCompleted, domain.CompletedNormally, true},
		{"FAILED", domain.ExecutionFailed, domain.ExecutorLost, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dyn, api := newStopFixtureClients()
			id := "g8-stop-lifecycle-" + strings.ToLower(tc.name)
			observation := lifecycleObservation(t, id, tc.state, tc.completion)
			seedLifecycleObservation(t, dyn, "default", observation)
			calls := registerCancel(api, id)

			resp, err := api.stopObservation(context.Background(), "default", id)
			if err != nil {
				t.Fatalf("Stop from %s: unexpected error: %v", tc.name, err)
			}
			if resp.State != tc.state {
				t.Fatalf("Stop from %s: response state = %s, want %s (accumulated evidence must be preserved)", tc.name, resp.State, tc.state)
			}
			if resp.Frozen != tc.frozen {
				t.Fatalf("Stop from %s: response frozen = %v, want %v", tc.name, resp.Frozen, tc.frozen)
			}
			invoked := atomic.LoadInt32(calls) != 0
			if invoked == tc.frozen {
				t.Fatalf("Stop from %s: cancel invoked = %v, want invoked iff not frozen (frozen=%v)", tc.name, invoked, tc.frozen)
			}

			after, _, err := api.get(context.Background(), "default", id)
			if err != nil {
				t.Fatal(err)
			}
			if after.Execution() != observation.Execution() {
				t.Fatalf("Stop from %s: persisted execution changed: %#v -> %#v", tc.name, observation.Execution(), after.Execution())
			}
		})
	}
}

// CON-1: concurrent Start calls against distinct targets create distinct,
// uncorrupted Observations. Run with -race.
func TestObservationAPIProof_ConcurrentStartCreatesDistinctObservations(t *testing.T) {
	core, dyn := startFixture(t)
	// Give each goroutine its own resolvable Pod name sharing the same
	// container, all under the same Deployment-owned chain.
	const n = 8
	for i := 1; i < n; i++ {
		podName := fmt.Sprintf("api-pod-%d", i)
		pod := core.CoreV1().Pods("default")
		base, err := pod.Get(context.Background(), "api-pod", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		clone := base.DeepCopy()
		clone.ObjectMeta.Name = podName
		clone.ObjectMeta.UID = types.UID(fmt.Sprintf("pod-uid-%d", i))
		if _, err := pod.Create(context.Background(), clone, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			podName := "api-pod"
			if i > 0 {
				podName = fmt.Sprintf("api-pod-%d", i)
			}
			resp, err := api.start(context.Background(), startObservationRequest{Pod: podName, Container: "app", Duration: time.Minute})
			ids[i] = resp.ID
			errs[i] = err
		}()
	}
	wg.Wait()
	seen := make(map[string]bool, n)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("CON-1: goroutine %d: %v", i, err)
		}
		if ids[i] == "" || seen[ids[i]] {
			t.Fatalf("CON-1: goroutine %d produced a duplicate or empty ID: %q", i, ids[i])
		}
		seen[ids[i]] = true
		if _, err := api.stopObservation(context.Background(), "default", ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	if countObservations(t, dyn, "default") != n {
		t.Fatalf("CON-1: expected %d distinct durable Observations, got %d", n, countObservations(t, dyn, "default"))
	}
}

// CON-2: concurrent Stop calls against the SAME Observation ID never race
// (guarded by observationAPI.mu) and never panic. context.CancelFunc is
// itself idempotent, so this deliberately does not assert exactly-once
// invocation of cancel.
func TestObservationAPIProof_ConcurrentStopSameIDIsRaceFree(t *testing.T) {
	dyn, api := newStopFixtureClients()
	observation := lifecycleObservation(t, "g8-con-stop", domain.ExecutionRunning, "")
	seedLifecycleObservation(t, dyn, "default", observation)
	calls := registerCancel(api, "g8-con-stop")

	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := api.stopObservation(context.Background(), "default", "g8-con-stop")
			errs[i] = err
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("CON-2: goroutine %d: %v", i, err)
		}
	}
	if atomic.LoadInt32(calls) == 0 {
		t.Fatal("CON-2: cancel was never invoked across concurrent Stop calls")
	}
}

// CON-3: concurrent Stop and Status(get) reads against the same Observation
// never race with each other.
func TestObservationAPIProof_ConcurrentStopAndStatusIsRaceFree(t *testing.T) {
	dyn, api := newStopFixtureClients()
	observation := lifecycleObservation(t, "g8-con-stop-status", domain.ExecutionRunning, "")
	seedLifecycleObservation(t, dyn, "default", observation)
	registerCancel(api, "g8-con-stop-status")

	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, 0, n*2)
	var mu sync.Mutex
	record := func(err error) {
		mu.Lock()
		errs = append(errs, err)
		mu.Unlock()
	}
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := api.stopObservation(context.Background(), "default", "g8-con-stop-status")
			record(err)
		}()
		go func() {
			defer wg.Done()
			_, _, err := api.get(context.Background(), "default", "g8-con-stop-status")
			record(err)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("CON-3: call %d: %v", i, err)
		}
	}
}
