package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

const operationalContextPath = "/api/v08/operations-context"

type operationalStatus string

const (
	operationalHealthy     operationalStatus = "HEALTHY"
	operationalDegraded    operationalStatus = "DEGRADED"
	operationalUnavailable operationalStatus = "UNAVAILABLE"
	operationalUnknown     operationalStatus = "UNKNOWN"
)

type operationalCapabilityState struct {
	Status       operationalStatus         `json:"status"`
	Capabilities map[authz.Capability]bool `json:"capabilities,omitempty"`
	Diagnostic   string                    `json:"diagnostic,omitempty"`
}

type operationalResourceState struct {
	Name   string            `json:"name"`
	Status operationalStatus `json:"status"`
	Reason string            `json:"reason,omitempty"`
}

type operationalDependencyState struct {
	Name   string            `json:"name"`
	Status operationalStatus `json:"status"`
	Scope  string            `json:"scope"`
}

type operationalContextResponse struct {
	ReadTime string `json:"readTime"`
	Context  struct {
		Cluster struct {
			Identity string            `json:"identity"`
			Status   operationalStatus `json:"status"`
		} `json:"cluster"`
		Namespace string `json:"namespace"`
		Actor     struct {
			Username string `json:"username"`
		} `json:"actor"`
	} `json:"context"`
	Authority operationalCapabilityState `json:"authority"`
	Platform  struct {
		Status           operationalStatus            `json:"status"`
		Backend          operationalStatus            `json:"backend"`
		KubernetesAPI    operationalStatus            `json:"kubernetesAPI"`
		ProductResources []operationalResourceState   `json:"productResources"`
		Dependencies     []operationalDependencyState `json:"dependencies"`
	} `json:"platform"`
	Projection projectionDiagnostics `json:"projection"`
}

func (s *workbenchServer) handleOperationalContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeWorkbenchClientError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	if !s.authenticated || s.requestIdentity.Username == "" || s.reads == nil {
		writeWorkbenchClientError(w, http.StatusUnauthorized, "operational context requires authenticated request mode")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()

	response := operationalContextResponse{ReadTime: time.Now().UTC().Format(time.RFC3339Nano)}
	response.Context.Cluster.Identity = s.clusterIdentity
	response.Context.Cluster.Status = operationalHealthy
	if response.Context.Cluster.Identity == "" {
		response.Context.Cluster.Status = operationalUnknown
	}
	response.Context.Namespace = s.reads.SessionIdentity().Namespace
	response.Context.Actor.Username = s.requestIdentity.Username

	response.Authority.Status = operationalHealthy
	if s.discoverCaps == nil {
		response.Authority.Status = operationalUnknown
		response.Authority.Diagnostic = "CAPABILITY_DISCOVERY_UNAVAILABLE"
	} else if capabilities, err := s.discoverCaps(ctx, response.Context.Namespace); err != nil {
		response.Authority.Status = operationalUnknown
		response.Authority.Diagnostic = "CAPABILITY_DISCOVERY_FAILED"
	} else {
		response.Authority.Capabilities = capabilities
	}

	response.Platform.Backend = operationalHealthy
	response.Platform.KubernetesAPI = operationalHealthy
	if response.Context.Cluster.Status != operationalHealthy {
		response.Platform.KubernetesAPI = operationalUnknown
	}
	response.Platform.Dependencies = []operationalDependencyState{{
		Name: "observation-executor", Scope: "observation-start",
		Status: operationalUnavailable,
	}}
	if s.observations != nil {
		response.Platform.Dependencies[0].Status = operationalHealthy
	}

	response.Platform.ProductResources = checkOperationalResources(ctx, s.reads)
	for _, resource := range response.Platform.ProductResources {
		if resource.Status == operationalUnavailable && resource.Reason != "REQUIRED_RESOURCE_MISSING" {
			response.Platform.KubernetesAPI = operationalUnavailable
		}
		if resource.Status == operationalUnavailable && resource.Reason == "REQUIRED_RESOURCE_MISSING" && response.Platform.KubernetesAPI == operationalHealthy {
			response.Platform.KubernetesAPI = operationalDegraded
		}
	}
	response.Platform.Status = operationalHealthy
	for _, resource := range response.Platform.ProductResources {
		if resource.Status != operationalHealthy {
			response.Platform.Status = operationalDegraded
		}
	}
	for _, dependency := range response.Platform.Dependencies {
		if dependency.Status == operationalUnavailable && response.Platform.Status == operationalHealthy {
			response.Platform.Status = operationalDegraded
		}
	}
	if response.Platform.KubernetesAPI == operationalUnavailable {
		response.Platform.Status = operationalUnavailable
	}

	loaded, err := s.loadV08Inputs(ctx)
	if err != nil {
		response.Projection.Status = projectionStatus(operationalUnavailable)
		response.Projection.Diagnostics = []projectionDiagnostic{{
			Category: "READ_ERROR", Reason: "projection source read failed",
			ProjectionImpact: "projection unavailable", SecurityDisposition: "NOT_ELIGIBLE",
		}}
	} else {
		response.Projection = loaded.diagnostics
		if s.metrics != nil {
			s.metrics.ProjectionExcludedCount(response.Projection.ExcludedObjectCount)
		}
	}
	writeWorkbenchJSON(w, http.StatusOK, response)
}

func checkOperationalResources(ctx context.Context, reads k8s.WorkbenchReadCapability) []operationalResourceState {
	checks := []struct {
		name string
		read func(context.Context) error
	}{
		{"traininghistories.landlockgenprof.io", func(ctx context.Context) error { _, err := reads.ListTrainingHistory(ctx); return err }},
		{"securityprofileproposals.landlockgenprof.io", func(ctx context.Context) error { _, err := reads.ListProposals(ctx); return err }},
		{"applyattempts.landlockgenprof.io", func(ctx context.Context) error { _, err := reads.ListApplyAttempts(ctx); return err }},
		{"rollbackattempts.landlockgenprof.io", func(ctx context.Context) error { _, err := reads.ListRollbackAttempts(ctx); return err }},
		{"observations.landlockgenprof.io", func(ctx context.Context) error { _, err := reads.ListObservations(ctx); return err }},
		{"observationcontributionreceipts.landlockgenprof.io", func(ctx context.Context) error { _, err := reads.ListContributionReceipts(ctx); return err }},
	}
	result := make([]operationalResourceState, 0, len(checks))
	for _, check := range checks {
		state := operationalResourceState{Name: check.name, Status: operationalHealthy}
		if err := check.read(ctx); err != nil {
			state.Status = operationalUnavailable
			state.Reason = operationalReadReason(err)
		}
		result = append(result, state)
	}
	return result
}

func operationalReadReason(err error) string {
	var readErr *k8s.ReadError
	if errors.As(err, &readErr) {
		switch readErr.State {
		case k8s.ReadBackendNotInstalled:
			return "REQUIRED_RESOURCE_MISSING"
		case k8s.ReadPermissionDenied:
			return "AUTHORIZED_READ_DENIED"
		case k8s.ReadTimeout:
			return "KUBERNETES_API_TIMEOUT"
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "timeout") {
		return "KUBERNETES_API_TIMEOUT"
	}
	return "KUBERNETES_API_UNAVAILABLE"
}
