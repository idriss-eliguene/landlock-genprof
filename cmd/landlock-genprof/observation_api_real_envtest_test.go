//go:build envtest

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

const realObservationHost = "127.0.0.1:18081"

func realObservationServer(t *testing.T) (*workbenchServer, kubernetes.Interface, dynamic.Interface) {
	t.Helper()
	core, err := kubernetes.NewForConfig(e2eConfig)
	if err != nil {
		t.Fatal(err)
	}
	dyn, err := dynamic.NewForConfig(e2eConfig)
	if err != nil {
		t.Fatal(err)
	}
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	return &workbenchServer{observations: api, allowedHost: realObservationHost, allowedOrigin: "http://" + realObservationHost, sema: make(chan struct{}, workbenchMaxConcurrentReads)}, core, dyn
}

func realObservationRequest(t *testing.T, server http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Host = realObservationHost
	req.Header.Set("Origin", "http://"+realObservationHost)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	return response
}

func seedRealObservation(t *testing.T, client dynamic.Interface, observation observationdomain.Observation) {
	t.Helper()
	store, err := obskube.NewStore(client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateObservation(context.Background(), "default", observation); err != nil {
		t.Fatal(err)
	}
	// Use the public executor persistence seam to encode the completed
	// result. This keeps the real-API fixture on the same status shape and
	// transition path as production execution.
	binding := observation.Binding()
	// Keep both the resolved runtime instance and the aggregate image revision
	// in the fixture. The read model uses the latter for its immutable image
	// identity projection, while the executor status shape preserves the former.
	instances := binding.ResolvedTargets.Items()
	if len(instances) != 1 || len(binding.ImageRevisions) != 1 {
		t.Fatalf("fixture binding instances=%d revisions=%d", len(instances), len(binding.ImageRevisions))
	}
	revision := binding.ImageRevisions[0]
	instances[0].ImageRevision = &revision
	resolved, err := observationdomain.NewResolvedTargetSet(instances)
	if err != nil {
		t.Fatal(err)
	}
	binding.ResolvedTargets = resolved
	starting, err := observationdomain.RestoreObservation(observation.ID(), observation.Spec(), binding, observationdomain.ObservationExecution{State: observationdomain.ExecutionStarting}, observation.Result(), observationdomain.ObservationProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	claim, rv, err := store.ClaimObservation(context.Background(), "default", string(observation.ID()), "g8-fixture-executor")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if rv, err = store.UpdateExecutorStatus(context.Background(), "default", claim, rv, starting); err != nil {
		t.Fatalf("starting status: %v", err)
	}
	if _, err := store.TransitionExecution(context.Background(), "default", claim, rv, observationdomain.ExecutionRunning, ""); err != nil {
		t.Fatalf("running transition: %v", err)
	}
	_, rv, err = store.GetObservation(context.Background(), "default", string(observation.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if rv, err = store.TransitionExecution(context.Background(), "default", claim, rv, observationdomain.ExecutionCompleting, ""); err != nil {
		t.Fatalf("completing transition: %v", err)
	}
	completing, err := observationdomain.RestoreObservation(observation.ID(), observation.Spec(), binding, observationdomain.ObservationExecution{State: observationdomain.ExecutionCompleting}, observation.Result(), observationdomain.ObservationProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	if rv, err = store.UpdateExecutorStatus(context.Background(), "default", claim, rv, completing); err != nil {
		t.Fatalf("completing status: %v", err)
	}
	_, rv, err = store.GetObservation(context.Background(), "default", string(observation.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionExecution(context.Background(), "default", claim, rv, observationdomain.ExecutionCompleted, observationdomain.CompletedNormally); err != nil {
		t.Fatalf("completed transition: %v", err)
	}
}

func TestObservationAPIRealEnvtestStartStatusStop(t *testing.T) {
	server, core, dyn := realObservationServer(t)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "g8-real-start-pod", Namespace: "default"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}}}}
	created, err := core.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status.Phase = corev1.PodRunning
	created.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "app", ContainerID: "containerd://g8-real-container", ImageID: "registry.example/app@sha256:" + strings.Repeat("a", 64)}}
	if _, err := core.CoreV1().Pods("default").UpdateStatus(context.Background(), created, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	start := realObservationRequest(t, server, http.MethodPost, "/api/observations/start", map[string]interface{}{"namespace": "default", "pod": pod.Name, "container": "app", "sources": []string{"capabilities"}, "duration": int64(time.Minute)})
	if start.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	var started observationStatusResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.ID == "" || started.State != observationdomain.ExecutionRequested {
		t.Fatalf("start response=%#v", started)
	}
	stored, err := obskube.NewStore(dyn)
	if err != nil {
		t.Fatal(err)
	}
	observation, _, err := stored.GetObservation(context.Background(), "default", started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(observation.ID()) != started.ID || observation.Spec().Target.Slot.Container != "app" || string(observation.Spec().Target.Slot.Workload.Cluster.NamespaceUID) == "" {
		t.Fatalf("stored identity=%#v", observation)
	}
	status := realObservationRequest(t, server, http.MethodGet, "/api/observations/status?namespace=default&observationID="+started.ID, nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code=%d body=%s", status.Code, status.Body.String())
	}
	stop := realObservationRequest(t, server, http.MethodPost, "/api/observations/stop", map[string]string{"namespace": "default", "observationID": started.ID})
	if stop.Code != http.StatusOK && stop.Code != http.StatusConflict {
		t.Fatalf("stop code=%d body=%s", stop.Code, stop.Body.String())
	}
	bad := realObservationRequest(t, server, http.MethodPost, "/api/observations/start", map[string]interface{}{"namespace": "other", "pod": pod.Name, "container": "app", "duration": int64(time.Minute)})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("namespace override code=%d body=%s", bad.Code, bad.Body.String())
	}
}

func TestObservationAPIRealEnvtestGenerateAndSameProposalConcurrency(t *testing.T) {
	server, _, dyn := realObservationServer(t)
	observation := proofObservation(t, "g8-real-generate", "CAP_CHOWN", observationdomain.SourceQualification{Attribution: observationdomain.AttributionCompleted, AttributedCount: 1, SourceAttachedForBoundWindow: true, FlushConfirmed: true})
	seedRealObservation(t, dyn, observation)
	for _, callers := range []int{2, 4, 8, 16} {
		for iteration := 0; iteration < 50; iteration++ {
			name := fmt.Sprintf("g8-real-generate-%d-%d", callers, iteration)
			errs := make([]error, callers)
			var wg sync.WaitGroup
			for i := 0; i < callers; i++ {
				i := i
				wg.Add(1)
				go func() {
					defer wg.Done()
					response := realObservationRequest(t, server, http.MethodPost, "/api/observations/generate-proposal", map[string]string{"namespace": "default", "observationID": string(observation.ID()), "proposalName": name})
					if response.Code != http.StatusOK {
						errs[i] = fmt.Errorf("caller %d status=%d body=%s", i, response.Code, response.Body.String())
					}
				}()
			}
			wg.Wait()
			for _, err := range errs {
				if err != nil {
					t.Fatalf("callers=%d iteration=%d: %v", callers, iteration, err)
				}
			}
			got, err := proposal.Get(context.Background(), dyn, "default", name)
			if err != nil || got.CandidateVersion != proposal.CandidateVersionV2 {
				t.Fatalf("proposal %s=%#v err=%v", name, got, err)
			}
			status, err := proposal.GetStatus(context.Background(), dyn, "default", name)
			if err != nil {
				t.Fatal(err)
			}
			if status.ApprovedCandidateDigest != "" || status.LastApprovalSnapshot != nil {
				t.Fatalf("proposal %s gained authority: %#v", name, status)
			}
		}
	}
}

// TestG10IntegratedObservationToWorkbenchProposalRealEnvtest composes the
// already-certified persistence seams without replacing any of them: a
// completed Observation is durably restored through the production executor
// store seam, read through the Workbench API, contributed through the real
// Observation API, derived into a candidate-v2 Proposal, and rediscovered
// through the Workbench read model.
func TestG10IntegratedObservationToWorkbenchProposalRealEnvtest(t *testing.T) {
	server, _, dyn := realObservationServer(t)
	reads, err := k8s.NewReadSession(e2eConfig, "default")
	if err != nil {
		t.Fatal(err)
	}
	server.reads = reads
	image := "sha256:" + strings.Repeat("a", 64)
	observation := proofObservation(t, "g10-integrated", "CAP_CHOWN", observationdomain.SourceQualification{
		BackendHealthConfirmed:       true,
		SourceAttachedForBoundWindow: true,
		FlushConfirmed:               true,
		Attribution:                  observationdomain.AttributionCompleted,
		AttributedCount:              1,
	})
	seedRealObservation(t, dyn, observation)
	storedObject, err := dyn.Resource(obskube.GVR).Namespace("default").Get(context.Background(), string(observation.ID()), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obskube.FromUnstructured(storedObject); err != nil {
		t.Fatalf("real Observation decode=%v object=%#v", err, storedObject.Object)
	}

	observations := realObservationRequest(t, server, http.MethodGet, "/api/observations/"+string(observation.ID()), nil)
	if observations.Code != http.StatusOK {
		t.Fatalf("Observation discovery status=%d body=%s", observations.Code, observations.Body.String())
	}
	var item observationRead
	if err := json.Unmarshal(observations.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID != string(observation.ID()) || item.Identity.WorkloadUID != "workload-proof" || item.Identity.Container != "app" || item.Identity.ImageIdentity != image {
		t.Fatalf("identity continuity=%#v", item.Identity)
	}
	if item.Frozen != true || !strings.Contains(fmt.Sprint(item.Execution), string(observationdomain.ExecutionCompleted)) || len(item.Sources) != 1 || item.Sources[0].EvidenceState != "AVAILABLE" || item.Sources[0].AttributedCount != 1 {
		t.Fatalf("Observation projection=%#v", item)
	}

	proposalName := "g10-integrated-proposal"
	generated := realObservationRequest(t, server, http.MethodPost, "/api/observations/generate-proposal", map[string]string{
		"namespace":     "default",
		"observationID": string(observation.ID()),
		"proposalName":  proposalName,
	})
	if generated.Code != http.StatusOK {
		t.Fatalf("GenerateProposal status=%d body=%s", generated.Code, generated.Body.String())
	}

	identity := history.PopulationIdentity{Scope: history.ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: image}
	historyName, err := history.RecordNameForPopulation(identity)
	if err != nil {
		t.Fatal(err)
	}
	record, err := history.Get(context.Background(), dyn, "default", historyName)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Populations) == 0 {
		t.Fatalf("contribution continuity=%#v", record.Populations)
	}
	pop := record.Populations[0]
	if pop.Scope != history.ScopeContainer || pop.BinaryPath != "" || len(pop.CapabilityAccesses) == 0 || pop.CapabilityAccesses[0].Name != "CAP_CHOWN" {
		t.Fatalf("contributed capabilities=%#v", record.Populations[0].CapabilityAccesses)
	}
	contributions := 0
	for _, contribution := range pop.ObservationContributions {
		if contribution.ObservationID == string(observation.ID()) {
			contributions++
		}
	}
	if contributions != 1 {
		t.Fatalf("observation contribution count=%d, populations=%#v", contributions, record.Populations)
	}

	proposalResponse := realObservationRequest(t, server, http.MethodGet, "/api/proposals/"+proposalName, nil)
	if proposalResponse.Code != http.StatusOK {
		t.Fatalf("Proposal discovery status=%d body=%s", proposalResponse.Code, proposalResponse.Body.String())
	}
	var projected proposalRead
	if err := json.Unmarshal(proposalResponse.Body.Bytes(), &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Subject == nil || projected.Subject.Scope != proposal.CandidateV2ScopeContainer || projected.Subject.Target != "Deployment/api" || projected.Subject.Container != "app" || projected.Subject.ImageIdentity != image {
		t.Fatalf("Proposal subject=%#v", projected.Subject)
	}
	if projected.Artifact == nil || projected.Artifact.Type != proposal.CandidateV2ArtifactContainerCaps || len(projected.Artifact.ContainerCapabilities.Add) != 1 || projected.Artifact.ContainerCapabilities.Add[0] != "CAP_CHOWN" || len(projected.Artifact.ContainerCapabilities.Drop) != 1 || projected.Artifact.ContainerCapabilities.Drop[0] != "ALL" {
		t.Fatalf("Proposal artifact=%#v", projected.Artifact)
	}
	containsObservation := false
	if projected.Provenance != nil {
		for _, id := range projected.Provenance.ObservationIDs {
			if id == string(observation.ID()) {
				containsObservation = true
			}
		}
	}
	if projected.CandidateDigest == "" || projected.ReviewContextDigest == "" || !containsObservation || projected.Status.ApprovalState != proposal.ApprovalDraft || projected.CurrentAuthority != "NOT_APPROVED" {
		t.Fatalf("Proposal governance/provenance=%#v", projected)
	}
	if strings.Contains(proposalResponse.Body.String(), "workloadUID") {
		t.Fatal("Proposal projection fabricated workload UID binding")
	}

	// Replaying the integrated path and discarding the prior response must
	// rediscover the same durable objects without duplicating the history effect.
	replay := realObservationRequest(t, server, http.MethodPost, "/api/observations/generate-proposal", map[string]string{
		"namespace":     "default",
		"observationID": string(observation.ID()),
		"proposalName":  proposalName,
	})
	if replay.Code != http.StatusOK {
		t.Fatalf("replayed GenerateProposal status=%d body=%s", replay.Code, replay.Body.String())
	}
	reloadedObservation := realObservationRequest(t, server, http.MethodGet, "/api/observations/"+string(observation.ID()), nil)
	reloadedProposal := realObservationRequest(t, server, http.MethodGet, "/api/proposals/"+proposalName, nil)
	if reloadedObservation.Code != http.StatusOK || reloadedProposal.Code != http.StatusOK {
		t.Fatalf("session-loss rediscovery observations=%d proposals=%d", reloadedObservation.Code, reloadedProposal.Code)
	}
	reloadedRecord, err := history.Get(context.Background(), dyn, "default", historyName)
	if err != nil {
		t.Fatal(err)
	}
	contributions = 0
	for _, contribution := range reloadedRecord.Populations[0].ObservationContributions {
		if contribution.ObservationID == string(observation.ID()) {
			contributions++
		}
	}
	if contributions != 1 {
		t.Fatalf("replay duplicated/lost contribution=%#v", reloadedRecord.Populations)
	}
}

func TestObservationAPIRealEnvtestDistinctObservationAccumulation(t *testing.T) {
	server, _, dyn := realObservationServer(t)
	ids := make([]string, 0, 200)
	for iteration := 0; iteration < 100; iteration++ {
		a := proofObservation(t, fmt.Sprintf("g8-real-accumulate-a-%d", iteration), "CAP_CHOWN", observationdomain.SourceQualification{Attribution: observationdomain.AttributionCompleted, AttributedCount: 1})
		b := proofObservation(t, fmt.Sprintf("g8-real-accumulate-b-%d", iteration), "CAP_CHOWN", observationdomain.SourceQualification{Attribution: observationdomain.AttributionCompleted, AttributedCount: 1})
		seedRealObservation(t, dyn, a)
		seedRealObservation(t, dyn, b)
		ids = append(ids, string(a.ID()), string(b.ID()))
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i, observation := range []observationdomain.Observation{a, b} {
			wg.Add(1)
			go func(i int, observation observationdomain.Observation) {
				defer wg.Done()
				response := realObservationRequest(t, server, http.MethodPost, "/api/observations/generate-proposal", map[string]string{"namespace": "default", "observationID": string(observation.ID()), "proposalName": fmt.Sprintf("g8-real-accumulate-%d-%d", iteration, i)})
				if response.Code != http.StatusOK {
					errs <- fmt.Errorf("iteration=%d caller=%d status=%d body=%s", iteration, i, response.Code, response.Body.String())
				}
			}(i, observation)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
	}
	identity := history.PopulationIdentity{Scope: history.ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:" + strings.Repeat("a", 64)}
	name, err := history.RecordNameForPopulation(identity)
	if err != nil {
		t.Fatal(err)
	}
	record, err := history.Get(context.Background(), dyn, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, pop := range record.Populations {
		for _, contribution := range pop.ObservationContributions {
			seen[contribution.ObservationID] = true
		}
	}
	for _, id := range ids {
		if !seen[id] {
			t.Fatalf("lost contribution %s", id)
		}
	}
}
