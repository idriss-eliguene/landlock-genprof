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
	// The CRD carries image revisions on each resolved runtime instance. Keep
	// the fixture faithful to that authoritative schema shape; the legacy
	// aggregate imageRevisions field is intentionally a reduced compatibility
	// projection and cannot round-trip the full workload identity.
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
	binding.ImageRevisions = nil
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
