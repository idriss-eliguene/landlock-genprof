package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	observationruntime "github.com/idriss-eliguene/landlock-genprof/internal/observation/runtime"
	"github.com/idriss-eliguene/landlock-genprof/internal/observationapp"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposalapp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type observationExecutorFactory func() (kubernetes.Interface, dynamic.Interface, *rest.Config, error)

type observationAPI struct {
	client    kubernetes.Interface
	dynamic   dynamic.Interface
	namespace string
	mu        sync.Mutex
	stop      map[string]context.CancelFunc
	executor  observationExecutorFactory
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

func (a *observationAPI) withExecutor(factory observationExecutorFactory) *observationAPI {
	return &observationAPI{client: a.client, dynamic: a.dynamic, namespace: a.namespace, stop: make(map[string]context.CancelFunc), executor: factory}
}

type startObservationRequest struct {
	Namespace string        `json:"namespace"`
	Pod       string        `json:"pod"`
	Container string        `json:"container"`
	Sources   []string      `json:"sources"`
	Duration  time.Duration `json:"duration"`
}

type observationStatusResponse struct {
	ID            string                             `json:"observationID"`
	State         observationdomain.ExecutionState   `json:"executionState"`
	Completion    observationdomain.CompletionReason `json:"completion,omitempty"`
	Target        observationdomain.ContainerSlot    `json:"target"`
	Sources       []observationSourceStatus          `json:"sources"`
	Frozen        bool                               `json:"frozen"`
	StopRequested bool                               `json:"stopRequested"`
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
	for i := range request.Sources {
		request.Sources[i] = strings.TrimSpace(request.Sources[i])
	}
	prepared, err := observationapp.Prepare(ctx, observationapp.Clients{Core: a.client, Dynamic: a.dynamic}, observationapp.PrepareRequest{
		Namespace: request.Namespace, Pod: request.Pod, Container: request.Container,
		Sources: request.Sources, Duration: request.Duration, Requester: "workbench",
	})
	if err != nil {
		return observationStatusResponse{}, err
	}
	// Authenticated Operations Center requests only create the durable
	// REQUESTED record. A separately deployed executor owns Gadget execution;
	// keeping that process boundary here prevents human request handling from
	// acquiring runtime authority. The local/development adapter below retains
	// the historical in-process CLI-compatible behavior.
	if a.executor != nil {
		return observationStatusResponse{ID: string(prepared.ID), State: observationdomain.ExecutionRequested, Target: prepared.Target.Instance.Slot}, nil
	}
	executorID, err := obskube.NewExecutorID()
	if err != nil {
		return observationStatusResponse{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.stop[string(prepared.ID)] = cancel
	a.mu.Unlock()
	runnerClient, runnerDynamic, executorConfig := a.client, a.dynamic, (*rest.Config)(nil)
	runnerStore := prepared.Store
	if a.executor != nil {
		var err error
		runnerClient, runnerDynamic, executorConfig, err = a.executor()
		if err != nil {
			return observationStatusResponse{}, fmt.Errorf("observation executor unavailable: %w", err)
		}
		runnerStore, err = obskube.NewStore(runnerDynamic)
		if err != nil {
			return observationStatusResponse{}, err
		}
	}
	runner := &observationruntime.Runner{Store: runnerStore, Client: runnerClient, ExecutorConfig: executorConfig, Cluster: prepared.Cluster, Sources: prepared.Sources, Monitor: observationruntime.PollingTargetMonitor{Client: runnerClient, Cluster: prepared.Cluster, Target: prepared.Spec.Target}}
	go func() {
		_ = runner.Run(runCtx, request.Namespace, string(prepared.ID), executorID)
		a.mu.Lock()
		delete(a.stop, string(prepared.ID))
		a.mu.Unlock()
	}()
	return observationStatusResponse{ID: string(prepared.ID), State: observationdomain.ExecutionRequested, Target: prepared.Target.Instance.Slot}, nil
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

func (a *observationAPI) requestDurableStop(ctx context.Context, namespace, id string, input obskube.StopIntentInput) (observationStatusResponse, error) {
	if namespace != a.namespace {
		return observationStatusResponse{}, fmt.Errorf("invalid request: namespace is outside the Workbench read scope")
	}
	store, err := obskube.NewStore(a.dynamic)
	if err != nil {
		return observationStatusResponse{}, err
	}
	observation, _, err := store.RequestStop(ctx, namespace, id, input)
	if err != nil {
		return observationStatusResponse{}, err
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
	result := observationStatusResponse{ID: string(observation.ID()), State: observation.Execution().State, Completion: observation.Execution().Completion, Frozen: observation.Frozen(), StopRequested: observation.Execution().StopRequested()}
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
	result, err := proposalapp.Generate(ctx, a.dynamic, namespace, id, proposalName, func(ctx context.Context, namespace, id string) (observationdomain.Observation, error) {
		observation, _, err := a.get(ctx, namespace, id)
		return observation, err
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"proposalName": result.ProposalName, "candidateVersion": result.CandidateVersion, "scope": result.Scope, "target": result.Target, "container": result.Container, "imageIdentity": result.ImageIdentity, "approved": result.Approved}, nil
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
	case strings.Contains(message, "authorization denied"):
		code, class = http.StatusForbidden, "AUTHORIZATION_DENIED"
	case errors.Is(err, obskube.ErrStopNotEligible):
		code, class = http.StatusConflict, "STOP_NOT_ELIGIBLE"
	case errors.Is(err, obskube.ErrLeaseExpired):
		code, class = http.StatusConflict, "EXECUTOR_LEASE_EXPIRED"
	case errors.Is(err, proposal.ErrProposalPersistenceConflict):
		code, class = http.StatusConflict, "CONFLICT"
	case strings.Contains(message, "not found"):
		code, class = http.StatusNotFound, "NOT_FOUND"
	case strings.HasPrefix(message, "invalid request"):
		code, class = http.StatusBadRequest, "INVALID_REQUEST"
	case strings.Contains(message, "not a completed frozen result"):
		// A distinct class from CONFLICT: this is the Observation lifecycle
		// precondition for proposal generation, not a stale-resourceVersion
		// CAS conflict. Collapsing the two would make the client offer
		// "refresh and decide again" guidance for a failure that refreshing
		// cannot fix (the Observation must finish, not be re-read).
		code, class = http.StatusConflict, "OBSERVATION_NOT_COMPLETED"
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
	var result observationStatusResponse
	var err error
	if s.authenticated {
		if !s.capabilityAllowed(r.Context(), authz.ObservationOperate) {
			writeObservationAPIError(w, fmt.Errorf("authorization denied: authenticated identity lacks observation.operate"))
			return
		}
		observation, _, getErr := s.observations.get(r.Context(), request.Namespace, request.ObservationID)
		if getErr != nil {
			writeObservationAPIError(w, getErr)
			return
		}
		if observation.Spec().Target.Slot.Workload.Namespace != request.Namespace || (s.clusterIdentity != "" && string(observation.Spec().Target.Slot.Workload.Cluster.NamespaceUID) != s.clusterIdentity) {
			writeObservationAPIError(w, fmt.Errorf("authorization denied: Observation is outside the selected environment"))
			return
		}
		version, _ := strconv.ParseUint(r.Header.Get("X-Environment-Context-Version"), 10, 64)
		result, err = s.observations.requestDurableStop(r.Context(), request.Namespace, request.ObservationID, obskube.StopIntentInput{Requester: s.requestIdentity.Username, ContextVersion: version})
	} else {
		result, err = s.observations.stopObservation(r.Context(), request.Namespace, request.ObservationID)
	}
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
