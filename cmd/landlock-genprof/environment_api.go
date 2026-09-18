package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/environment"
)

const environmentKubeconfigEnv = "LANDLOCK_GENPROF_ENVIRONMENT_KUBECONFIG"

func newEnvironmentConnector() environment.ClusterConnector {
	path := strings.TrimSpace(os.Getenv(environmentKubeconfigEnv))
	return environment.NewLocalKubeconfigConnector(path, environment.ProductInstallationIdentity("operations-center"))
}

type environmentOpenRequest struct {
	ContextName string `json:"contextName"`
	Namespace   string `json:"namespace,omitempty"`
}

func (s *workbenchServer) handleEnvironments(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v09/environments" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		items, err := s.environment.Discover(r.Context())
		if err != nil {
			writeEnvironmentError(w, err)
			return
		}
		writeWorkbenchJSON(w, http.StatusOK, struct {
			Contexts []environment.DiscoveredContext `json:"contexts"`
		}{Contexts: items})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
		return
	}
	var request environmentOpenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, workbenchMaxRequestBodyBytes)).Decode(&request); err != nil || strings.TrimSpace(request.ContextName) == "" {
		writeWorkbenchClientError(w, http.StatusBadRequest, "contextName is required")
		return
	}
	session, err := s.environment.Open(r.Context(), environment.OpenRequest{ContextName: request.ContextName, Namespace: request.Namespace})
	if err != nil {
		writeEnvironmentError(w, err)
		return
	}
	writeWorkbenchJSON(w, http.StatusCreated, session.Metadata())
}

func (s *workbenchServer) handleEnvironmentSession(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v09/environments/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	sessionID := parts[0]
	switch {
	case len(parts) == 2 && parts[1] == "namespaces" && r.Method == http.MethodGet:
		session, err := s.environment.Session(sessionID)
		if err != nil {
			writeEnvironmentError(w, err)
			return
		}
		result, err := session.DiscoverNamespaces(r.Context())
		if err != nil {
			writeEnvironmentError(w, err)
			return
		}
		writeWorkbenchJSON(w, http.StatusOK, result)
	case len(parts) == 2 && parts[1] == "capabilities" && r.Method == http.MethodGet:
		namespace := strings.TrimSpace(r.URL.Query().Get("namespace"))
		if namespace == "" {
			writeWorkbenchClientError(w, http.StatusBadRequest, "namespace is required")
			return
		}
		// Trusted-proxy requests are bound to the authenticated startup
		// namespace. Do not return a capability projection for another
		// namespace or let the browser present that namespace as active while
		// subsequent reads remain pinned to the authenticated one.
		if s.requestContext != nil {
			if strings.TrimSpace(s.reads.SessionIdentity().Namespace) != namespace ||
				(r.Header.Get("X-Environment-Namespace") != "" && r.Header.Get("X-Environment-Namespace") != namespace) {
				writeWorkbenchJSON(w, http.StatusConflict, workbenchErrorBody{State: "STALE_ENVIRONMENT_CONTEXT", Reason: "the selected environment does not match the authenticated server context; select the authorized context again"})
				return
			}
			session, err := s.environment.Session(sessionID)
			if err != nil {
				writeEnvironmentError(w, err)
				return
			}
			if session.Context().ClusterIdentity().NamespaceUID != s.clusterIdentity {
				writeWorkbenchJSON(w, http.StatusConflict, workbenchErrorBody{State: "STALE_ENVIRONMENT_CONTEXT", Reason: "the selected environment does not match the authenticated server context; select the authorized context again"})
				return
			}
		}
		session, err := s.environment.Session(sessionID)
		if err != nil {
			writeEnvironmentError(w, err)
			return
		}
		selected, err := session.SelectNamespace(r.Context(), namespace)
		if err != nil {
			writeEnvironmentError(w, err)
			return
		}
		writeWorkbenchJSON(w, http.StatusOK, struct {
			Namespace      string                    `json:"namespace"`
			ContextVersion uint64                    `json:"contextVersion"`
			Capabilities   map[authz.Capability]bool `json:"capabilities"`
		}{namespace, selected.Context().ContextVersion(), selected.Capabilities()})
	default:
		http.NotFound(w, r)
	}
}

func workbenchEnvironmentMutationPath(path string) bool { return path == "/api/v09/environments" }

func writeEnvironmentError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, environment.ErrSessionNotFound) {
		status = http.StatusNotFound
	}
	if errors.Is(err, environment.ErrStaleContext) {
		status = http.StatusConflict
	}
	if errors.Is(err, environment.ErrExecPluginNotAllowed) {
		status = http.StatusForbidden
	}
	writeWorkbenchJSON(w, status, workbenchErrorBody{State: "ENVIRONMENT_UNAVAILABLE", Reason: "environment connector could not establish the requested context"})
}
