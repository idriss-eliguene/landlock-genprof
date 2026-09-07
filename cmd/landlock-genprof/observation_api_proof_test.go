package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func proofObservation(t *testing.T, id, capability string, qualification domain.SourceQualification) domain.Observation {
	t.Helper()
	cluster, err := domain.NewClusterIdentity("cluster-proof")
	if err != nil {
		t.Fatal(err)
	}
	workload := domain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "workload-proof"}
	slot := domain.ContainerSlot{Workload: workload, Container: "app"}
	spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: slot}, []string{"capabilities"}, time.Minute, "g8-proof")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewContainerImageRevision(slot, "sha256:"+strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	targets, err := domain.NewResolvedTargetSet([]domain.RuntimeContainerInstance{{Slot: slot, PodUID: id + "-pod", ContainerID: id + "-container"}})
	if err != nil {
		t.Fatal(err)
	}
	facts := domain.NormalizedFacts{}
	if capability != "" {
		facts.Capabilities = []domain.CapabilityFact{{Name: capability}}
	}
	result, err := domain.NewSourceResult(domain.EvidenceSource{Name: "capabilities", Backend: "proof", Version: "v1"}, qualification, nil, facts)
	if err != nil {
		t.Fatal(err)
	}
	observationResult, err := domain.NewObservationResult([]domain.SourceResult{result})
	if err != nil {
		t.Fatal(err)
	}
	binding := domain.ObservationBinding{ResolvedTargets: targets, Backend: domain.BackendIdentity{Kind: "proof", Version: "v1"}, ImageRevisions: []domain.ContainerImageRevision{revision}}
	provenance := domain.ObservationProvenance{ResolvedTargets: targets, ImageRevisions: []domain.ContainerImageRevision{revision}, Backend: binding.Backend, RequestedSources: []string{"capabilities"}}
	observation, err := domain.RestoreObservation(domain.ObservationID(id), spec, binding, domain.ObservationExecution{State: domain.ExecutionCompleted, Completion: domain.CompletedNormally}, observationResult, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func proofClients() (*kubefake.Clientset, *dynamicfake.FakeDynamicClient) {
	return kubefake.NewSimpleClientset(), dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
}

func seedProofObservation(t *testing.T, dyn *dynamicfake.FakeDynamicClient, observation domain.Observation) {
	t.Helper()
	object, err := obskube.ToUnstructured(observation, "default")
	if err != nil {
		t.Fatal(err)
	}
	if observation.Frozen() {
		slot := observation.Binding().ResolvedTargets.Items()[0]
		source := observation.Result().Sources()[0]
		capabilities := make([]interface{}, 0, len(source.Facts.Capabilities))
		for _, fact := range source.Facts.Capabilities {
			capabilities = append(capabilities, map[string]interface{}{"name": fact.Name})
		}
		workload := map[string]interface{}{"cluster": map[string]interface{}{"namespaceUID": string(slot.Slot.Workload.Cluster.NamespaceUID)}, "namespace": slot.Slot.Workload.Namespace, "groupKind": map[string]interface{}{"group": slot.Slot.Workload.GroupKind.Group, "kind": slot.Slot.Workload.GroupKind.Kind}, "name": slot.Slot.Workload.Name, "uid": slot.Slot.Workload.UID}
		slotMap := map[string]interface{}{"workload": workload, "container": slot.Slot.Container}
		status := map[string]interface{}{
			"binding": map[string]interface{}{
				"resolvedTargets": []interface{}{map[string]interface{}{
					"slot":   slotMap,
					"podUID": slot.PodUID, "containerID": slot.ContainerID,
				}},
				"backend": map[string]interface{}{"kind": "proof", "version": "v1"},
				"imageRevisions": []interface{}{map[string]interface{}{
					"slot":        slotMap,
					"imageDigest": "sha256:" + strings.Repeat("a", 64),
				}},
				"targetChanges": []interface{}{},
			},
			"execution": map[string]interface{}{"state": string(observation.Execution().State), "completion": string(observation.Execution().Completion)},
			"result": map[string]interface{}{"sources": []interface{}{map[string]interface{}{
				"name": source.Source.Name, "backend": source.Source.Backend, "version": source.Source.Version, "evidence": string(source.Evidence),
				"qualification": map[string]interface{}{"backendHealthConfirmed": source.Qualification.BackendHealthConfirmed, "sourceAttachedForBoundWindow": source.Qualification.SourceAttachedForBoundWindow, "flushConfirmed": source.Qualification.FlushConfirmed, "attribution": string(source.Qualification.Attribution), "attributedCount": float64(source.Qualification.AttributedCount), "excludedCount": float64(source.Qualification.ExcludedCount)},
				"facts":         map[string]interface{}{"capabilities": capabilities},
			}}},
			"provenance": map[string]interface{}{
				"resolvedTargets": []interface{}{}, "imageRevisions": []interface{}{}, "backend": map[string]interface{}{"kind": "proof", "version": "v1"}, "requestedSources": []interface{}{"capabilities"},
			},
		}
		object.Object["status"] = status
	}
	if _, err := dyn.Resource(obskube.GVR).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestObservationAPIProof_GenerateUsesCertifiedChain(t *testing.T) {
	core, dyn := proofClients()
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	q := domain.SourceQualification{SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted, AttributedCount: 1}
	observation := proofObservation(t, "g8-api-proof", "CAP_CHOWN", q)
	seedProofObservation(t, dyn, observation)
	result, err := api.generate(context.Background(), "default", string(observation.ID()), "g8-api-proposal")
	if err != nil {
		t.Fatal(err)
	}
	if result["approved"] != false || result["candidateVersion"] != "candidate-v2" {
		t.Fatalf("generation result = %#v", result)
	}
	got, err := proposal.Get(context.Background(), dyn, "default", "g8-api-proposal")
	if err != nil {
		t.Fatal(err)
	}
	if got.CandidateVersion != proposal.CandidateVersionV2 || got.Subject == nil || got.Subject.Container != "app" {
		t.Fatalf("proposal = %#v", got)
	}
	status, err := proposal.GetStatus(context.Background(), dyn, "default", "g8-api-proposal")
	if err != nil {
		t.Fatal(err)
	}
	if status.ApprovalState != proposal.ApprovalDraft || status.ApprovedCandidateDigest != "" || status.LastApprovalSnapshot != nil {
		t.Fatalf("generation changed authority: %#v", status)
	}
	if _, err := api.generate(context.Background(), "default", string(observation.ID()), "g8-api-proposal"); err != nil {
		t.Fatalf("repeated generation: %v", err)
	}
}

func TestObservationAPIProof_RejectsNonterminalAndNamespaceOverride(t *testing.T) {
	core, dyn := proofClients()
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	base := proofObservation(t, "g8-requested", "CAP_CHOWN", domain.SourceQualification{Attribution: domain.AttributionCompleted, AttributedCount: 1})
	observation, err := domain.NewObservation(domain.ObservationID("g8-requested"), base.Spec())
	if err != nil {
		t.Fatal(err)
	}
	seedProofObservation(t, dyn, observation)
	if _, err := api.generate(context.Background(), "default", string(observation.ID()), "never"); err == nil || !strings.Contains(err.Error(), "not a completed frozen result") {
		t.Fatalf("nonterminal generation error = %v", err)
	}
	if _, err := api.generate(context.Background(), "other", string(observation.ID()), "never"); err == nil || !strings.Contains(err.Error(), "outside the Workbench") {
		t.Fatalf("namespace override error = %v", err)
	}
}

func TestObservationAPIProof_StatusIsReadOnlyAndPreservesUnknown(t *testing.T) {
	core, dyn := proofClients()
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	q := domain.SourceQualification{SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted, AttributedCount: 1}
	observation := proofObservation(t, "g8-status-proof", "CAP_CHOWN", q)
	seedProofObservation(t, dyn, observation)
	before, _, err := api.get(context.Background(), "default", string(observation.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if got := before.Result().Sources()[0].Evidence; got != domain.EvidenceUnknown {
		t.Fatalf("evidence = %s, want UNKNOWN", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/observations/status?namespace=default&observationID="+string(observation.ID()), nil)
	req.Host = "127.0.0.1:18080"
	srv := &workbenchServer{observations: api, allowedHost: req.Host, allowedOrigin: "http://" + req.Host, sema: make(chan struct{}, 1)}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, body=%s", w.Code, w.Body.String())
	}
	var response observationStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Sources) != 1 || response.Sources[0].EvidenceState != "UNKNOWN" {
		t.Fatalf("API status = %#v", response)
	}
	after, _, err := api.get(context.Background(), "default", string(observation.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if after.Execution() != before.Execution() {
		t.Fatalf("status endpoint changed execution: %#v -> %#v", before.Execution(), after.Execution())
	}
}

func TestObservationAPIProof_ZeroPositiveProducesNoCandidate(t *testing.T) {
	core, dyn := proofClients()
	api, err := newObservationAPI(core, dyn, "default")
	if err != nil {
		t.Fatal(err)
	}
	observation := proofObservation(t, "g8-zero-proof", "", domain.SourceQualification{SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted})
	seedProofObservation(t, dyn, observation)
	if _, err := api.generate(context.Background(), "default", string(observation.ID()), "g8-zero-proposal"); err == nil || !strings.Contains(err.Error(), "no candidate") {
		t.Fatalf("zero-positive result = %v", err)
	}
	_, err = dyn.Resource(schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}).Namespace("default").Get(context.Background(), "g8-zero-proposal", metav1.GetOptions{})
	if err == nil {
		t.Fatal("zero-positive generation persisted a proposal")
	}
}
