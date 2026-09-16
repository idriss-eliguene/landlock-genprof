package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

// newGenerateFixtureClients registers both the Observation and ApplyAttempt
// GVRs against the fake dynamic client, since AUTH proofs must list
// ApplyAttempts (a completely separate CRD/store from proposal generation)
// to prove Generate never creates one.
func newGenerateFixtureClients() (*kubefake.Clientset, *dynamicfake.FakeDynamicClient) {
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		obskube.GVR: "ObservationList",
		attempt.GVR: "ApplyAttemptList",
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}: "SecurityProfileProposalList",
	})
	return kubefake.NewSimpleClientset(), dyn
}

func countApplyAttempts(t *testing.T, dyn *dynamicfake.FakeDynamicClient, namespace string) int {
	t.Helper()
	list, err := dyn.Resource(attempt.GVR).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return len(list.Items)
}

// generateFixtureObservation mirrors lifecycleObservation/proofObservation
// but is parameterized on capability so GEN tests can build both a positive
// and a zero-positive completed Observation, plus every intermediate state.
func generateFixtureObservation(t *testing.T, id string, state domain.ExecutionState, completion domain.CompletionReason, capability string) domain.Observation {
	t.Helper()
	cluster, err := domain.NewClusterIdentity("cluster-gen")
	if err != nil {
		t.Fatal(err)
	}
	workload := domain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "workload-gen"}
	slot := domain.ContainerSlot{Workload: workload, Container: "app"}
	spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: slot}, []string{"capabilities"}, time.Minute, "g8-gen")
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
		q := domain.SourceQualification{SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted}
		facts := domain.NormalizedFacts{}
		if capability != "" {
			facts.Capabilities = []domain.CapabilityFact{{Name: capability}}
			q.AttributedCount = 1
		}
		sourceResult, err := domain.NewSourceResult(domain.EvidenceSource{Name: "capabilities", Backend: "proof", Version: "v1"}, q, nil, facts)
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

// seedGenerateObservation is seedLifecycleObservation's twin for this file's
// fixture builder (kept file-local so this proof file has no ordering
// dependency on observation_api_stop_test.go's helpers).
func seedGenerateObservation(t *testing.T, dyn *dynamicfake.FakeDynamicClient, namespace string, observation domain.Observation) {
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

// GEN full 6-state lifecycle table: mutable/nonterminal evidence must never
// be usable as if frozen, and FAILED (frozen but not COMPLETED) must be
// rejected too — Generate is only eligible from a completed frozen result.
func TestObservationAPIProof_GenerateAcrossFullLifecycle(t *testing.T) {
	cases := []struct {
		name     string
		state    domain.ExecutionState
		reason   domain.CompletionReason
		eligible bool
	}{
		{"REQUESTED", domain.ExecutionRequested, "", false},
		{"STARTING", domain.ExecutionStarting, "", false},
		{"RUNNING", domain.ExecutionRunning, "", false},
		{"COMPLETING", domain.ExecutionCompleting, "", false},
		{"FAILED", domain.ExecutionFailed, domain.ExecutorLost, false},
		{"COMPLETED", domain.ExecutionCompleted, domain.CompletedNormally, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, dyn := newGenerateFixtureClients()
			api, err := newObservationAPI(core, dyn, "default")
			if err != nil {
				t.Fatal(err)
			}
			id := "g8-gen-lifecycle-" + strings.ToLower(tc.name)
			observation := generateFixtureObservation(t, id, tc.state, tc.reason, "CAP_CHOWN")
			seedGenerateObservation(t, dyn, "default", observation)

			result, err := api.generate(context.Background(), "default", id, "proposal-"+strings.ToLower(tc.name))
			if tc.eligible {
				if err != nil {
					t.Fatalf("GEN: expected COMPLETED to be eligible, got error: %v", err)
				}
				if result["approved"] != false {
					t.Fatalf("GEN: result = %#v, must never fabricate approval", result)
				}
				return
			}
			if err == nil {
				t.Fatalf("GEN: state %s must be rejected (mutable/nonterminal evidence used as frozen); result = %#v", tc.name, result)
			}
			if !strings.Contains(err.Error(), "not a completed frozen result") {
				t.Fatalf("GEN: state %s error = %v, want not a completed frozen result", tc.name, err)
			}
			if got, getErr := proposal.Get(context.Background(), dyn, "default", "proposal-"+strings.ToLower(tc.name)); getErr != nil {
				t.Fatalf("GEN: state %s: unexpected error checking for a persisted proposal: %v", tc.name, getErr)
			} else if got != nil {
				t.Fatalf("GEN: state %s must not persist a proposal on rejection, got %#v", tc.name, got)
			}
		})
	}
}

// AUTH-1, AUTH-2: Generate never approves, never writes an approval digest,
// never writes LastApprovalSnapshot, never creates an ApplyAttempt, and never
// mutates the observed workload object — proven by direct inspection of
// every distinct persisted-state surface Generate could have touched.
func TestObservationAPIProof_GenerateNeverWritesApprovalAuthority(t *testing.T) {
	core, dyn := newGenerateFixtureClients()
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", UID: "workload-gen"}}
	created, err := core.AppsV1().Deployments("default").Create(context.Background(), deployment, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	beforeResourceVersion := created.ResourceVersion

	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	observation := generateFixtureObservation(t, "g8-auth-proof", domain.ExecutionCompleted, domain.CompletedNormally, "CAP_CHOWN")
	seedGenerateObservation(t, dyn, "default", observation)

	if _, err := api.generate(context.Background(), "default", "g8-auth-proof", "g8-auth-proposal"); err != nil {
		t.Fatal(err)
	}

	status, err := proposal.GetStatus(context.Background(), dyn, "default", "g8-auth-proposal")
	if err != nil {
		t.Fatal(err)
	}
	if status.ApprovalState != proposal.ApprovalDraft {
		t.Fatalf("AUTH-1: ApprovalState = %q, want Draft (Generate must never approve)", status.ApprovalState)
	}
	if status.ApprovedCandidateDigest != "" {
		t.Fatalf("AUTH-1: ApprovedCandidateDigest = %q, want empty (Generate must never write an approval digest)", status.ApprovedCandidateDigest)
	}
	if status.LastApprovalSnapshot != nil {
		t.Fatalf("AUTH-1: LastApprovalSnapshot = %#v, want nil (Generate must never write approval custody)", status.LastApprovalSnapshot)
	}

	if n := countApplyAttempts(t, dyn, "default"); n != 0 {
		t.Fatalf("AUTH-2: %d ApplyAttempt objects exist, want 0 (Generate must never create an ApplyAttempt)", n)
	}

	after, err := core.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if after.ResourceVersion != beforeResourceVersion {
		t.Fatalf("AUTH-2: workload resourceVersion changed %s -> %s; Generate must never mutate a workload", beforeResourceVersion, after.ResourceVersion)
	}
}

// CON-5: concurrent Generate and concurrent Status(get)/Stop calls against
// the same completed Observation never race with each other.
func TestObservationAPIProof_ConcurrentGenerateAndStatusIsRaceFree(t *testing.T) {
	core, dyn := newGenerateFixtureClients()
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	observation := generateFixtureObservation(t, "g8-con-gen-status", domain.ExecutionCompleted, domain.CompletedNormally, "CAP_CHOWN")
	seedGenerateObservation(t, dyn, "default", observation)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, 0, n*3)
	var mu sync.Mutex
	record := func(err error) {
		mu.Lock()
		errs = append(errs, err)
		mu.Unlock()
	}
	for i := 0; i < n; i++ {
		i := i
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, err := api.generate(context.Background(), "default", "g8-con-gen-status", fmt.Sprintf("g8-con-gen-status-proposal-%d", i))
			record(err)
		}()
		go func() {
			defer wg.Done()
			_, _, err := api.get(context.Background(), "default", "g8-con-gen-status")
			record(err)
		}()
		go func() {
			defer wg.Done()
			_, err := api.stopObservation(context.Background(), "default", "g8-con-gen-status")
			record(err)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("CON-5: call %d: %v", i, err)
		}
	}
}

// CON-6: concurrent Generate calls against the SAME Observation but DISTINCT
// proposal names each succeed independently with no cross-contamination
// between the resulting proposals.
func TestObservationAPIProof_ConcurrentGenerateDistinctProposalNames(t *testing.T) {
	core, dyn := newGenerateFixtureClients()
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	observation := generateFixtureObservation(t, "g8-con-gen-distinct", domain.ExecutionCompleted, domain.CompletedNormally, "CAP_CHOWN")
	seedGenerateObservation(t, dyn, "default", observation)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := api.generate(context.Background(), "default", "g8-con-gen-distinct", fmt.Sprintf("g8-con-gen-distinct-proposal-%d", i))
			errs[i] = err
		}()
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("CON-6: goroutine %d: %v", i, errs[i])
		}
		name := fmt.Sprintf("g8-con-gen-distinct-proposal-%d", i)
		got, err := proposal.Get(context.Background(), dyn, "default", name)
		if err != nil {
			t.Fatalf("CON-6: proposal %s missing: %v", name, err)
		}
		if got.Subject == nil || got.Subject.Container != "app" {
			t.Fatalf("CON-6: proposal %s corrupted: %#v", name, got)
		}
	}
}
