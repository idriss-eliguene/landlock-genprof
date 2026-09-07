package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	observationruntime "github.com/idriss-eliguene/landlock-genprof/internal/observation/runtime"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type observationAPI struct {
	client    kubernetes.Interface
	dynamic   dynamic.Interface
	namespace string
	mu        sync.Mutex
	stop      map[string]context.CancelFunc
}

func newObservationAPI(client kubernetes.Interface, dynamicClient dynamic.Interface, namespace string) (*observationAPI, error) {
	if client == nil || dynamicClient == nil {
		return nil, fmt.Errorf("observation API requires Kubernetes clients")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("observation API requires a namespace")
	}
	return &observationAPI{client: client, dynamic: dynamicClient, namespace: namespace, stop: make(map[string]context.CancelFunc)}, nil
}

type startObservationRequest struct {
	Namespace string        `json:"namespace"`
	Pod       string        `json:"pod"`
	Container string        `json:"container"`
	Sources   []string      `json:"sources"`
	Duration  time.Duration `json:"duration"`
}

type observationStatusResponse struct {
	ID         string                             `json:"observationID"`
	State      observationdomain.ExecutionState   `json:"executionState"`
	Completion observationdomain.CompletionReason `json:"completion,omitempty"`
	Target     observationdomain.ContainerSlot    `json:"target"`
	Sources    []observationSourceStatus          `json:"sources"`
	Frozen     bool                               `json:"frozen"`
}

type observationSourceStatus struct {
	Name             string `json:"name"`
	AttributionState string `json:"attributionState"`
	EvidenceState    string `json:"evidenceState"`
	AttributedCount  uint64 `json:"attributedCount"`
	ExcludedCount    uint64 `json:"excludedCount"`
}

func (a *observationAPI) start(ctx context.Context, request startObservationRequest) (observationStatusResponse, error) {
	if request.Namespace == "" {
		request.Namespace = a.namespace
	}
	if request.Namespace != a.namespace {
		return observationStatusResponse{}, fmt.Errorf("invalid request: namespace is outside the Workbench read scope")
	}
	if strings.TrimSpace(request.Namespace) == "" || strings.TrimSpace(request.Pod) == "" || strings.TrimSpace(request.Container) == "" {
		return observationStatusResponse{}, fmt.Errorf("invalid request: namespace, pod, and container are required")
	}
	if request.Duration <= 0 || request.Duration > 24*time.Hour {
		return observationStatusResponse{}, fmt.Errorf("invalid request: duration must be between 1ns and 24h")
	}
	cluster, err := k8s.ResolveClusterIdentity(ctx, a.client)
	if err != nil {
		return observationStatusResponse{}, fmt.Errorf("cluster identity unresolved: %w", err)
	}
	pod, err := a.client.CoreV1().Pods(request.Namespace).Get(ctx, request.Pod, metav1.GetOptions{})
	if err != nil {
		return observationStatusResponse{}, fmt.Errorf("invalid target: %w", err)
	}
	target, err := k8s.ResolveObservationTarget(ctx, a.client, cluster, pod, request.Container)
	if err != nil {
		return observationStatusResponse{}, fmt.Errorf("invalid target: %w", err)
	}
	if len(request.Sources) == 0 {
		request.Sources = []string{observationruntime.FilesystemSourceName}
	}
	sources := make([]observationruntime.FilesystemSource, 0, len(request.Sources))
	for i := range request.Sources {
		name := strings.TrimSpace(request.Sources[i])
		source, sourceErr := observationSource(name)
		if sourceErr != nil {
			return observationStatusResponse{}, fmt.Errorf("invalid request: %w", sourceErr)
		}
		sources = append(sources, source)
		request.Sources[i] = name
	}
	spec, err := observationdomain.NewObservationSpec(observationdomain.RequestedTarget{Slot: target.Instance.Slot}, request.Sources, request.Duration, "workbench")
	if err != nil {
		return observationStatusResponse{}, fmt.Errorf("invalid request: %w", err)
	}
	id, err := observationdomain.NewObservationID()
	if err != nil {
		return observationStatusResponse{}, err
	}
	observation, err := observationdomain.NewObservation(id, spec)
	if err != nil {
		return observationStatusResponse{}, err
	}
	store, err := obskube.NewStore(a.dynamic)
	if err != nil {
		return observationStatusResponse{}, err
	}
	if _, err := store.CreateObservation(ctx, request.Namespace, observation); err != nil {
		return observationStatusResponse{}, err
	}
	executorID, err := obskube.NewExecutorID()
	if err != nil {
		return observationStatusResponse{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.stop[string(id)] = cancel
	a.mu.Unlock()
	runner := &observationruntime.Runner{Store: store, Client: a.client, Cluster: cluster, Sources: sources, Monitor: observationruntime.PollingTargetMonitor{Client: a.client, Cluster: cluster, Target: spec.Target}}
	go func() {
		_ = runner.Run(runCtx, request.Namespace, string(id), executorID)
		a.mu.Lock()
		delete(a.stop, string(id))
		a.mu.Unlock()
	}()
	return observationStatusResponse{ID: string(id), State: observationdomain.ExecutionRequested, Target: target.Instance.Slot}, nil
}

func (a *observationAPI) stopObservation(ctx context.Context, namespace, id string) (observationStatusResponse, error) {
	if namespace != a.namespace {
		return observationStatusResponse{}, fmt.Errorf("invalid request: namespace is outside the Workbench read scope")
	}
	observation, _, err := a.get(ctx, namespace, id)
	if err != nil {
		return observationStatusResponse{}, err
	}
	a.mu.Lock()
	cancel := a.stop[id]
	a.mu.Unlock()
	if cancel != nil && !observation.Frozen() {
		cancel()
	}
	return observationResponse(observation), nil
}

func (a *observationAPI) get(ctx context.Context, namespace, id string) (observationdomain.Observation, string, error) {
	store, err := obskube.NewStore(a.dynamic)
	if err != nil {
		return observationdomain.Observation{}, "", err
	}
	return store.GetObservation(ctx, namespace, id)
}

func observationResponse(observation observationdomain.Observation) observationStatusResponse {
	result := observationStatusResponse{ID: string(observation.ID()), State: observation.Execution().State, Completion: observation.Execution().Completion, Frozen: observation.Frozen()}
	result.Target = observation.Spec().Target.Slot
	for _, source := range observation.Result().Sources() {
		result.Sources = append(result.Sources, observationSourceStatus{Name: source.Source.Name, AttributionState: string(source.Qualification.Attribution), EvidenceState: string(source.Evidence), AttributedCount: source.Qualification.AttributedCount, ExcludedCount: source.Qualification.ExcludedCount})
	}
	return result
}

func (a *observationAPI) generate(ctx context.Context, namespace, id, proposalName string) (map[string]interface{}, error) {
	if namespace != a.namespace {
		return nil, fmt.Errorf("invalid request: namespace is outside the Workbench read scope")
	}
	observation, _, err := a.get(ctx, namespace, id)
	if err != nil {
		return nil, err
	}
	if !observation.Frozen() || observation.Execution().State != observationdomain.ExecutionCompleted {
		return nil, fmt.Errorf("conflict: Observation is not a completed frozen result")
	}
	contribution, err := history.ContributionFromObservation(observation)
	if err != nil {
		return nil, fmt.Errorf("conflict: Observation is not eligible: %w", err)
	}
	if _, err := history.ApplyObservationContribution(ctx, a.dynamic, namespace, observation); err != nil {
		return nil, fmt.Errorf("contributing Observation: %w", err)
	}
	identity, err := contribution.Population.Identity()
	if err != nil {
		return nil, err
	}
	spec, err := proposal.GenerateContainerCapabilityProposal(ctx, a.dynamic, namespace, identity, proposalName)
	if err != nil {
		if errors.Is(err, proposal.ErrNoCandidate) {
			return nil, fmt.Errorf("no candidate: %w", err)
		}
		return nil, fmt.Errorf("generating proposal: %w", err)
	}
	return map[string]interface{}{"proposalName": proposalName, "candidateVersion": spec.CandidateVersion, "scope": spec.Subject.Scope, "target": spec.Subject.Target, "container": spec.Subject.Container, "imageIdentity": spec.Subject.ImageIdentity, "approved": false}, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, workbenchMaxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

type stopObservationRequest struct {
	Namespace     string `json:"namespace"`
	ObservationID string `json:"observationID"`
}

type generateProposalRequest struct {
	Namespace     string `json:"namespace"`
	ObservationID string `json:"observationID"`
	ProposalName  string `json:"proposalName"`
}

func writeObservationAPIError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	class := "INTERNAL"
	message := err.Error()
	switch {
	case errors.Is(err, proposal.ErrProposalPersistenceConflict):
		code, class = http.StatusConflict, "CONFLICT"
	case strings.Contains(message, "not found"):
		code, class = http.StatusNotFound, "NOT_FOUND"
	case strings.HasPrefix(message, "invalid request"):
		code, class = http.StatusBadRequest, "INVALID_REQUEST"
	case strings.Contains(message, "conflict"):
		code, class = http.StatusConflict, "CONFLICT"
	case strings.Contains(message, "no candidate"):
		code, class = http.StatusUnprocessableEntity, "NO_CANDIDATE"
	case strings.Contains(message, "unsupported"):
		code, class = http.StatusNotImplemented, "UNSUPPORTED"
	case strings.Contains(message, "cluster identity unresolved"):
		code, class = http.StatusFailedDependency, "CLUSTER_IDENTITY_UNRESOLVED"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": class, "message": message})
}

func (s *workbenchServer) handleObservationStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || s.observations == nil {
		if s.observations == nil {
			http.Error(w, "observation API unavailable", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	var request startObservationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeObservationAPIError(w, fmt.Errorf("invalid request: %w", err))
		return
	}
	result, err := s.observations.start(r.Context(), request)
	if err != nil {
		writeObservationAPIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *workbenchServer) handleObservationStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || s.observations == nil {
		if s.observations == nil {
			http.Error(w, "observation API unavailable", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	var request stopObservationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeObservationAPIError(w, fmt.Errorf("invalid request: %w", err))
		return
	}
	result, err := s.observations.stopObservation(r.Context(), request.Namespace, request.ObservationID)
	if err != nil {
		writeObservationAPIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *workbenchServer) handleObservationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || s.observations == nil {
		if s.observations == nil {
			http.Error(w, "observation API unavailable", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	query := r.URL.Query()
	namespace, id := query.Get("namespace"), query.Get("observationID")
	if namespace == "" || id == "" {
		writeObservationAPIError(w, fmt.Errorf("invalid request: namespace and observationID are required"))
		return
	}
	observation, _, err := s.observations.get(r.Context(), namespace, id)
	if err != nil {
		writeObservationAPIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(observationResponse(observation))
}

func (s *workbenchServer) handleObservationGenerateProposal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || s.observations == nil {
		if s.observations == nil {
			http.Error(w, "observation API unavailable", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	var request generateProposalRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeObservationAPIError(w, fmt.Errorf("invalid request: %w", err))
		return
	}
	if request.ProposalName == "" {
		request.ProposalName = request.ObservationID
	}
	result, err := s.observations.generate(r.Context(), request.Namespace, request.ObservationID, request.ProposalName)
	if err != nil {
		writeObservationAPIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
