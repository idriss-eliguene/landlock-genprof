// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// The G3 local Workbench HTTP trust boundary.
//
// Every route here is read-only and depends only on
// k8s.WorkbenchReadCapability (bounded GET/LIST, no client-go interface, no
// dynamic client, no write verb). That is a structural property, not a
// behavioral promise: this file cannot construct a write-capable client
// because it never imports one, and workbenchServer's only Kubernetes field
// is the capability interface. See docs/adr/0023 for the full trust-boundary
// analysis, including the explicit non-goal: loopback binding and the
// browser-origin controls below defend against a browser page and against
// DNS rebinding, never against an arbitrary local process, which can already
// forge any HTTP header this server reads.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/association"
	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/environment"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
	"github.com/idriss-eliguene/landlock-genprof/internal/projection"
	"github.com/idriss-eliguene/landlock-genprof/internal/workload"
	operationscenter "github.com/idriss-eliguene/landlock-genprof/web/operations-center"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

const (
	// workbenchClusterReadDeadline bounds every Kubernetes-backed request.
	// Smaller than WriteTimeout so a request that hits its own deadline can
	// still render an error before the server would forcibly close it.
	workbenchClusterReadDeadline = 8 * time.Second

	// workbenchMaxConcurrentReads bounds simultaneous cluster-backed
	// requests. A saturated server fails fast (503) rather than queuing
	// unboundedly; see workbenchAcquireRead.
	workbenchMaxConcurrentReads = 8

	// workbenchMaxResponseBytes is a defense-in-depth cap on one JSON
	// response body. The primary boundedness proof is structural: every
	// read is scoped to the one namespace pinned into ReadSession
	// (ADR-0021), so response size is already bounded by that namespace's
	// object count. This cap only stops a pathological single response
	// from consuming unbounded memory; it is not expected to trigger.
	workbenchMaxResponseBytes = 4 << 20

	// workbenchMaxRequestBodyBytes bounds a rejected request body read.
	workbenchMaxRequestBodyBytes = 1 << 10

	workbenchReadTimeout    = 10 * time.Second
	workbenchWriteTimeout   = 15 * time.Second
	workbenchIdleTimeout    = 60 * time.Second
	workbenchMaxHeaderBytes = 1 << 16

	// workbenchMaxIdentifierLength matches the Kubernetes DNS-1123 subdomain
	// bound (RFC 1123 hostname length).
	workbenchMaxIdentifierLength = 253
	// workbenchMaxQueryParams bounds how many query keys a request may
	// carry before validation even inspects their values.
	workbenchMaxQueryParams = 8

	// workbenchCSP is restrictive by default: no framing, no externally
	// loaded resources, no plugins. 'unsafe-inline' remains only for
	// style-src because the rendered page's <style> block is fixed
	// server-authored CSS with no attacker-controlled or user-supplied
	// content interpolated into it — every dynamic value in the page goes
	// through html/template's contextual text/attribute escaping, not into
	// a style context, so inline-style injection is not a reachable path
	// here. script-src is 'self': the page's JavaScript (workbench.js) is
	// same-origin, server-authored, and loaded from this handler's own
	// route, never inline or externally hosted.
	workbenchCSP = "default-src 'none'; connect-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; " +
		"img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'"
)

// workbenchIdentifierPattern matches a Kubernetes name or namespace: a
// lowercase RFC 1123 DNS subdomain. It intentionally does not accept
// uppercase letters, whitespace, or path/URL metacharacters.
var workbenchIdentifierPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// workbenchKindPattern matches a Kubernetes Kind: an upper-camel Go-style
// identifier. Group may be empty (the core group) or a DNS-1123 subdomain.
var workbenchKindPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,62}$`)

// workbenchContainerPattern matches a container name: an RFC 1123 DNS
// label (no dots), which is what the Kubernetes API itself requires.
var workbenchContainerPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// workbenchServer is the entire G3 HTTP surface. Its only Kubernetes
// dependency is the bounded read capability; it holds no write-capable
// client and exposes none. legacyProposal, when non-empty, serves the v0.4
// single-proposal review page at "/"; it is a display selector, not
// authority — every read it triggers still goes through reads.
type workbenchServer struct {
	reads           k8s.WorkbenchReadCapability
	discovery       *workload.Service
	projector       *projection.Service
	observations    *observationAPI
	dynamic         dynamic.Interface
	requestContext  func(*http.Request) (workbenchRequestContext, error)
	requestIdentity authn.Identity
	discoverCaps    workbenchCapabilityDiscovery
	environment     environment.ClusterConnector
	authenticated   bool
	clusterIdentity string
	legacyProposal  string
	allowedHost     string
	allowedOrigin   string
	sema            chan struct{}
	lifecycle       *workbenchLifecycle
	logger          *observability.Logger
	metrics         *observability.Metrics
}

func newWorkbenchServer(reads k8s.WorkbenchReadCapability, legacyProposal string, port int) (*workbenchServer, error) {
	if reads == nil {
		return nil, fmt.Errorf("workbench server requires a read capability")
	}
	discovery, err := workload.NewService(reads)
	if err != nil {
		return nil, fmt.Errorf("constructing workload discovery: %w", err)
	}
	projector, err := projection.NewService(reads)
	if err != nil {
		return nil, fmt.Errorf("constructing projection service: %w", err)
	}
	host := workbenchAllowedHost(port)
	return &workbenchServer{
		reads:          reads,
		discovery:      discovery,
		projector:      projector,
		legacyProposal: legacyProposal,
		environment:    newEnvironmentConnector(),
		allowedHost:    host,
		allowedOrigin:  "http://" + host,
		sema:           make(chan struct{}, workbenchMaxConcurrentReads),
		lifecycle:      &workbenchLifecycle{},
		logger:         discardObservabilityLogger(),
		metrics:        observability.NewMetrics(),
	}, nil
}

func discardObservabilityLogger() *observability.Logger {
	logger, _ := observability.NewLogger(io.Discard, observability.DefaultLogLevel)
	return logger
}

func (s *workbenchServer) mux() *http.ServeMux {
	// http.NewServeMux is used explicitly rather than http.DefaultServeMux:
	// nothing in this process ever registers on the default mux, so
	// net/http/pprof (or any other package that self-registers there) can
	// never become reachable through this listener even transitively.
	mux := http.NewServeMux()
	// The migration frontend is deliberately additive. The server-rendered
	// Workbench at / remains the reference oracle until parity is qualified.
	mux.Handle("/next/", operationscenter.Handler())
	mux.HandleFunc(workbenchStartupPath, s.lifecycle.serveHTTP)
	mux.HandleFunc(workbenchLivenessPath, s.lifecycle.serveHTTP)
	mux.HandleFunc(workbenchReadinessPath, s.lifecycle.serveHTTP)
	mux.HandleFunc("/", s.handleLegacyProposal)
	mux.HandleFunc("/api/workloads", s.handleWorkloads)
	mux.HandleFunc("/api/workloads/detail", s.handleWorkloadDetail)
	mux.HandleFunc("/api/projection", s.handleProjection)
	mux.HandleFunc("/api/observations/start", s.handleObservationStart)
	mux.HandleFunc("/api/observations/stop", s.handleObservationStop)
	mux.HandleFunc("/api/observations/status", s.handleObservationStatus)
	mux.HandleFunc("/api/observations/generate-proposal", s.handleObservationGenerateProposal)
	mux.HandleFunc("/api/observations", s.handleObservationReadModel)
	mux.HandleFunc("/api/observations/", s.handleObservationReadModel)
	mux.HandleFunc("/api/proposals", s.handleProposalReadModel)
	mux.HandleFunc("/api/proposals/", s.handleProposalReadModel)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/v08/environment/detail", s.handleV08Environment)
	mux.HandleFunc("/api/v08/environment", s.handleV08Environment)
	mux.HandleFunc("/api/v08/overview", s.handleV08Overview)
	mux.HandleFunc("/api/v08/history/proposal", s.handleV08History)
	mux.HandleFunc("/api/v08/history", s.handleV08History)
	mux.HandleFunc("/api/v08/capabilities", s.handleCapabilities)
	mux.HandleFunc("/api/v09/environments", s.handleEnvironments)
	mux.HandleFunc("/api/v09/environments/", s.handleEnvironmentSession)
	mux.HandleFunc(operationalContextPath, s.handleOperationalContext)
	mux.HandleFunc("/api/governance/proposals/", s.handleGovernanceProposal)
	mux.HandleFunc("/api/governance/apply-attempts/", s.handleGovernanceRollback)
	mux.HandleFunc("/workbench.js", handleWorkbenchScript)
	return mux
}

// ServeHTTP is the single entrypoint. It applies, in order: panic
// containment, security response headers, Host validation, browser-origin
// validation, and body rejection — all before any handler runs. A request
// that fails any of these never reaches a Kubernetes read.
func (s *workbenchServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := observability.RequestID(r.Header.Get(observability.RequestIDHeader))
	w.Header().Set(observability.RequestIDHeader, requestID)
	recorder := &observability.ResponseRecorder{ResponseWriter: w}
	started := time.Now()
	actor := ""
	defer func() {
		status := recorder.Status()
		route := observability.Route(r.URL.Path)
		fields := map[string]interface{}{"component": "operations_center", "request_id": requestID, "http_method": r.Method, "route": route, "status_code": status, "duration_ms": time.Since(started).Milliseconds()}
		if actor != "" {
			fields["actor"] = actor
		}
		if s.reads != nil {
			fields["namespace"] = s.reads.SessionIdentity().Namespace
		}
		if s.logger != nil {
			s.logger.Info("http_request", fields)
		}
		if s.metrics != nil {
			s.metrics.HTTP(r.Method, route, status)
			s.metrics.HTTPDuration(time.Since(started).Seconds(), r.Method, route)
			if strings.HasPrefix(r.URL.Path, "/api/governance/") {
				operation := "unknown"
				parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
				if len(parts) > 0 {
					operation = parts[len(parts)-1]
				}
				result := "SUCCESS"
				reason := "NONE"
				if status >= 400 {
					result = "FAILURE"
					reason = "HTTP_" + strconv.Itoa(status)
				}
				s.metrics.Governance(operation, result, reason)
			}
			if status == http.StatusForbidden {
				s.metrics.AuthorizationDenied("AUTHZ_DENIED")
			}
		}
	}()
	w = recorder
	defer s.workbenchRecover(w, r)

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", workbenchCSP)
	// No CORS headers are ever set. Their absence is what stops a
	// cross-origin browser page from reading a response; emitting
	// Access-Control-Allow-Origin (permissive or reflected) would defeat
	// that entirely, so this server never does.

	lifecycleRequest := isWorkbenchLifecyclePath(r.URL.Path)
	if !lifecycleRequest && !workbenchValidHost(r.Host, s.allowedHost) {
		http.Error(w, "invalid Host", http.StatusForbidden)
		return
	}
	if !lifecycleRequest && !workbenchValidBrowserOrigin(r, s.allowedOrigin) {
		http.Error(w, "cross-origin request rejected", http.StatusForbidden)
		return
	}
	// Every route this server has is GET-only, so the method is checked
	// once here rather than once per handler. This must run before the body
	// check below: a non-GET request is a method-contract violation (405)
	// first, whether or not it also happens to carry a body.
	if r.Method != http.MethodGet && !workbenchObservationMutationPath(r.URL.Path) && !workbenchEnvironmentMutationPath(r.URL.Path) {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodGet && !workbenchRejectBody(w, r) {
		return
	}
	// Lifecycle endpoints are exact, data-free process checks. They bypass
	// human identity only here; protected API routes still pass through the
	// request-context authorization path below.
	if lifecycleRequest {
		s.mux().ServeHTTP(w, r)
		return
	}

	// Local M3 sessions are selected through the environment API and carried
	// back only as opaque identifiers. Rebind every operational request to the
	// selected immutable session; never mutate the process-wide startup read
	// session. Production trusted-proxy requests continue through the identity
	// resolver below, which remains the authorization boundary.
	if s.requestContext == nil && r.Header.Get("X-Environment-Session") != "" && !strings.HasPrefix(r.URL.Path, "/api/v09/environments") {
		requestServer, err := s.forEnvironmentRequest(r)
		if err != nil {
			if errors.Is(err, environment.ErrStaleContext) || errors.Is(err, environment.ErrSessionNotFound) {
				writeWorkbenchJSON(w, http.StatusConflict, workbenchErrorBody{State: "STALE_ENVIRONMENT_CONTEXT", Reason: "the selected environment context is no longer valid; select it again"})
			} else {
				writeWorkbenchJSON(w, http.StatusForbidden, workbenchErrorBody{State: "ENVIRONMENT_UNAVAILABLE", Reason: "the selected environment cannot authorize this request"})
			}
			return
		}
		requestServer.mux().ServeHTTP(w, r)
		return
	}

	if s.requestContext != nil {
		request, err := s.requestContext(r)
		if err != nil {
			reason := classifyAuthenticationFailure(err)
			if s.metrics != nil {
				s.metrics.AuthFailure(reason)
			}
			if s.logger != nil {
				s.logger.Warn("authentication_failed", map[string]interface{}{"component": "operations_center", "request_id": requestID, "reason": reason})
			}
			http.Error(w, "request identity is not authorized", http.StatusUnauthorized)
			return
		}
		actor = request.identity.Username
		// Trusted-proxy mode owns the authenticated Kubernetes context. An
		// EnvironmentSession header may describe the browser's attempted
		// rebinding, but it must never redirect this request to a different
		// namespace or cluster while the request-context resolver remains the
		// authority. Validate the opaque session and fail closed on mismatch;
		// otherwise the UI could display one namespace while this fixed client
		// reads another.
		if r.Header.Get("X-Environment-Session") != "" && !strings.HasPrefix(r.URL.Path, "/api/v09/environments") {
			environmentRequest, environmentErr := s.forEnvironmentRequest(r)
			if environmentErr != nil {
				if errors.Is(environmentErr, environment.ErrStaleContext) || errors.Is(environmentErr, environment.ErrSessionNotFound) {
					writeWorkbenchJSON(w, http.StatusConflict, workbenchErrorBody{State: "STALE_ENVIRONMENT_CONTEXT", Reason: "the selected environment context is no longer valid; select it again"})
				} else {
					writeWorkbenchJSON(w, http.StatusForbidden, workbenchErrorBody{State: "ENVIRONMENT_UNAVAILABLE", Reason: "the selected environment cannot authorize this request"})
				}
				return
			}
			if environmentRequest.reads.SessionIdentity().Namespace != request.reads.SessionIdentity().Namespace ||
				(environmentRequest.clusterIdentity != "" && request.clusterIdentity != "" && environmentRequest.clusterIdentity != request.clusterIdentity) {
				writeWorkbenchJSON(w, http.StatusConflict, workbenchErrorBody{State: "STALE_ENVIRONMENT_CONTEXT", Reason: "the selected environment does not match the authenticated server context; select the authorized context again"})
				return
			}
		}
		requestServer := *s
		requestServer.reads = request.reads
		requestServer.requestIdentity = request.identity
		requestServer.dynamic = request.dynamic
		requestServer.discoverCaps = request.discoverCaps
		requestServer.clusterIdentity = request.clusterIdentity
		requestServer.authenticated = true
		var errBuild error
		requestServer.discovery, errBuild = workload.NewService(request.reads)
		if errBuild != nil {
			http.Error(w, "request read session unavailable", http.StatusBadGateway)
			return
		}
		requestServer.projector, errBuild = projection.NewService(request.reads)
		if errBuild != nil {
			http.Error(w, "request read session unavailable", http.StatusBadGateway)
			return
		}
		requestServer.observations = request.observations
		requestServer.mux().ServeHTTP(w, r)
		return
	}

	s.mux().ServeHTTP(w, r)
}

func (s *workbenchServer) forEnvironmentRequest(r *http.Request) (*workbenchServer, error) {
	sessionID := r.Header.Get("X-Environment-Session")
	version, err := strconv.ParseUint(r.Header.Get("X-Environment-Context-Version"), 10, 64)
	if err != nil || version == 0 {
		return nil, environment.ErrStaleContext
	}
	namespace := strings.TrimSpace(r.Header.Get("X-Environment-Namespace"))
	if namespace == "" {
		return nil, environment.ErrStaleContext
	}
	session, err := s.environment.Session(sessionID)
	if err != nil {
		return nil, err
	}
	selected, err := session.SelectNamespace(r.Context(), namespace)
	if err != nil {
		return nil, err
	}
	if selected.Context().ContextVersion() != version {
		return nil, environment.ErrStaleContext
	}
	reads, core, dyn, err := session.WorkbenchClients(namespace)
	if err != nil {
		return nil, err
	}
	requestServer := *s
	requestServer.reads = reads
	requestServer.dynamic = dyn
	requestServer.authenticated = true
	requestServer.requestIdentity = authn.Identity{Username: "local-kubeconfig"}
	requestServer.clusterIdentity = string(session.Context().ClusterIdentity().NamespaceUID)
	requestServer.discoverCaps = func(ctx context.Context, namespace string) (map[authz.Capability]bool, error) {
		return authz.DiscoverCapabilities(ctx, core, namespace)
	}
	requestServer.discovery, err = workload.NewService(reads)
	if err != nil {
		return nil, err
	}
	requestServer.projector, err = projection.NewService(reads)
	if err != nil {
		return nil, err
	}
	requestServer.observations, err = newObservationAPI(core, dyn, namespace)
	if err != nil {
		return nil, err
	}
	if s.observations != nil && s.observations.executor != nil {
		requestServer.observations = requestServer.observations.withExecutor(s.observations.executor)
	}
	return &requestServer, nil
}

func classifyAuthenticationFailure(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "timestamp"):
		return "AUTH_STALE"
	case strings.Contains(message, "signature"):
		return "AUTH_INVALID_SIGNATURE"
	case strings.Contains(message, "allowlist") || strings.Contains(message, "permitted"):
		return "AUTH_IDENTITY_DENIED"
	case strings.Contains(message, "identity"):
		return "AUTH_MISSING"
	default:
		return "AUTH_INVALID"
	}
}

func workbenchObservationMutationPath(path string) bool {
	return path == "/api/observations/start" || path == "/api/observations/stop" || path == "/api/observations/generate-proposal" ||
		strings.HasPrefix(path, "/api/governance/proposals/") || strings.HasPrefix(path, "/api/governance/apply-attempts/")
}

func (s *workbenchServer) workbenchRecover(w http.ResponseWriter, r *http.Request) {
	if rec := recover(); rec != nil {
		if s.logger != nil {
			s.logger.Error("http_panic", map[string]interface{}{
				"component":   "operations_center",
				"http_method": r.Method,
				"route":       observability.Route(r.URL.Path),
				"reason":      fmt.Sprint(rec),
			})
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// workbenchValidHost is the primary DNS-rebinding defense: a browser page
// served from an attacker domain that resolves to 127.0.0.1 still sends that
// domain (or nothing) in Host, never this server's own loopback:port.
func workbenchValidHost(host, allowed string) bool {
	return host != "" && host == allowed
}

// workbenchValidBrowserOrigin enforces the fixed G3 browser-origin policy.
// Fetch Metadata is authoritative when present (browser-set, unforgeable by
// page script); Origin is the fallback for browsers that omit it. A request
// with neither header is treated as a non-browser local client and is
// admitted here — Host validation above already ran for every request.
func workbenchValidBrowserOrigin(r *http.Request, allowedOrigin string) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin == allowedOrigin
	}
	return true
}

// workbenchRejectBody enforces the read-only contract at the transport
// level: any request carrying a body is rejected outright rather than
// silently parsed or ignored. It writes its own error response and reports
// whether the caller should continue.
func workbenchRejectBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	body := http.MaxBytesReader(w, r.Body, workbenchMaxRequestBodyBytes)
	var probe [1]byte
	n, err := body.Read(probe[:])
	if n > 0 {
		http.Error(w, "read-only Workbench: request body not allowed", http.StatusBadRequest)
		return false
	}
	if err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "read-only Workbench: request body not allowed", http.StatusBadRequest)
		return false
	}
	return true
}

// workbenchAcquireRead enforces the concurrency bound around every
// Kubernetes-backed handler. On saturation it fails fast rather than
// queuing: a bounded wait risks stacking requests past their own deadlines
// under load, while fail-fast gives the caller an immediate, explicit signal
// and keeps the bound trivially testable.
func (s *workbenchServer) workbenchAcquireRead(w http.ResponseWriter) (release func(), ok bool) {
	select {
	case s.sema <- struct{}{}:
		return func() { <-s.sema }, true
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "too many concurrent requests", http.StatusServiceUnavailable)
		return nil, false
	}
}

func (s *workbenchServer) handleLegacyProposal(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", http.StatusMethodNotAllowed)
		return
	}
	release, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()

	var selector *targetSelector
	if len(r.URL.Query()) > 0 {
		parsed, reason := parseTargetSelector(r.URL.Query())
		if reason != "" {
			writeWorkbenchClientError(w, http.StatusBadRequest, reason)
			return
		}
		selector = &parsed
	}
	page, err := workbenchClusterPage(ctx, s.reads, s.legacyProposal, selector)
	if err != nil {
		var notFound *workbenchTargetNotFoundError
		if errors.As(err, &notFound) {
			writeWorkbenchClientError(w, http.StatusNotFound, "no discovered workload matches the requested target")
			return
		}
		writeWorkbenchTransportError(w, err)
		return
	}
	newWorkbenchClusterHandler(page).ServeHTTP(w, r)
}

type workbenchTargetNotFoundError struct{ target targetSelector }

func (e *workbenchTargetNotFoundError) Error() string {
	return fmt.Sprintf("workbench target %s/%s/%s/%s was not discovered", e.target.group, e.target.kind, e.target.name, e.target.container)
}

func (s *workbenchServer) handleWorkloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", http.StatusMethodNotAllowed)
		return
	}
	if len(r.URL.Query()) > 0 {
		writeWorkbenchClientError(w, http.StatusBadRequest, "no query parameters are accepted")
		return
	}
	release, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()

	result, err := s.discovery.Discover(ctx)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	writeWorkbenchJSON(w, http.StatusOK, dtoFromDiscoveryResult(result))
}

// handleWorkloadDetail returns a deliberately small, authoritative manifest
// projection for the exact discovered workload. It uses the same bounded read
// capability and discovery binding as /api/workloads; the browser never gets a
// Kubernetes client or credentials, and server-managed metadata is not exposed
// by default.
func (s *workbenchServer) handleWorkloadDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", http.StatusMethodNotAllowed)
		return
	}
	sel, why := parseReadModelSelector(r.URL.Query())
	if why != "" {
		writeWorkbenchClientError(w, http.StatusBadRequest, why)
		return
	}
	release, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()
	result, err := s.discovery.Discover(ctx)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	_, item, _, found := resolveGovernedTarget(result, targetSelector{group: sel.group, kind: sel.kind, name: sel.name, container: sel.container})
	if !found || item.UID != sel.workloadUID {
		writeWorkbenchClientError(w, http.StatusNotFound, "no discovered workload matches the requested target")
		return
	}
	var object *unstructured.Unstructured
	switch {
	case sel.group == "apps" && sel.kind == "Deployment":
		object, err = s.reads.GetDeployment(ctx, sel.name)
	case sel.group == "apps" && sel.kind == "StatefulSet":
		object, err = s.reads.GetStatefulSet(ctx, sel.name)
	case sel.group == "apps" && sel.kind == "DaemonSet":
		object, err = s.reads.GetDaemonSet(ctx, sel.name)
	case sel.group == "apps" && sel.kind == "ReplicaSet":
		object, err = s.reads.GetReplicaSet(ctx, sel.name)
	default:
		writeWorkbenchClientError(w, http.StatusNotFound, "the selected workload kind is not supported by this engineering view")
		return
	}
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	if object == nil || string(object.GetUID()) != sel.workloadUID || object.GetNamespace() != s.reads.SessionIdentity().Namespace {
		writeWorkbenchClientError(w, http.StatusNotFound, "the selected workload is no longer the discovered workload")
		return
	}
	manifest := map[string]any{
		"apiVersion": object.GetAPIVersion(),
		"kind":       object.GetKind(),
		"metadata": map[string]any{
			"name":      object.GetName(),
			"namespace": object.GetNamespace(),
		},
	}
	if labels := object.GetLabels(); len(labels) > 0 {
		manifest["metadata"].(map[string]any)["labels"] = labels
	}
	if spec, found, specErr := unstructured.NestedMap(object.Object, "spec"); specErr != nil {
		writeWorkbenchTransportError(w, specErr)
		return
	} else if found {
		manifest["spec"] = spec
	}
	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	writeWorkbenchJSON(w, http.StatusOK, map[string]any{"workload": manifest, "yaml": string(manifestYAML), "source": "authoritative Kubernetes read projection"})
}

func (s *workbenchServer) handleProjection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", http.StatusMethodNotAllowed)
		return
	}
	selector, verr := parseTargetSelector(r.URL.Query())
	if verr != "" {
		writeWorkbenchClientError(w, http.StatusBadRequest, verr)
		return
	}
	release, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()

	result, err := s.discovery.Discover(ctx)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}

	target, item, subjects, found := resolveGovernedTarget(result, selector)
	if !found {
		writeWorkbenchClientError(w, http.StatusNotFound, "no discovered workload matches the requested target")
		return
	}

	view, err := s.projector.Project(ctx, target, item, projection.Inputs{
		// Evidence is deliberately nil: G3 introduces no TrainingHistory
		// loader over the bounded read capability. Runtime therefore
		// reports EMPTY, an honest statement that no evidence was supplied
		// — not that none exists. A bounded evidence loader is separate,
		// later work.
		Evidence: nil,
		// RuntimeSubjects reuses exactly what discovery already returned
		// for this target in this same bounded read; it introduces no new
		// authority or additional cluster access.
		RuntimeSubjects: subjects,
		// Proposals left nil: projection owns loading and records its own
		// NOT_INTERPRETED exclusions, which a caller-supplied list cannot.
		Proposals: nil,
	})
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	writeWorkbenchJSON(w, http.StatusOK, dtoFromProjection(view))
}

// targetSelector is the untrusted, syntax-validated request input. It is
// never used to construct a GovernedTarget directly — see
// resolveGovernedTarget, which only ever returns a target that already
// exists as a Container.Target in bounded discovery output.
type targetSelector struct {
	group     string
	kind      string
	name      string
	container string
}

// parseTargetSelector validates every field before any cluster read. It
// returns a non-empty reason on the first violation.
func parseTargetSelector(query map[string][]string) (targetSelector, string) {
	if len(query) > workbenchMaxQueryParams {
		return targetSelector{}, "too many query parameters"
	}
	for key, values := range query {
		switch key {
		case "group", "kind", "name", "container":
		default:
			return targetSelector{}, fmt.Sprintf("unsupported query parameter %q", key)
		}
		if len(values) != 1 {
			return targetSelector{}, fmt.Sprintf("query parameter %q must appear exactly once", key)
		}
	}
	get := func(key string) string {
		if v, ok := query[key]; ok {
			return v[0]
		}
		return ""
	}
	selector := targetSelector{group: get("group"), kind: get("kind"), name: get("name"), container: get("container")}

	if selector.kind == "" || selector.name == "" || selector.container == "" {
		return targetSelector{}, "kind, name, and container are required"
	}
	if len(selector.group) > workbenchMaxIdentifierLength || len(selector.kind) > workbenchMaxIdentifierLength ||
		len(selector.name) > workbenchMaxIdentifierLength || len(selector.container) > workbenchMaxIdentifierLength {
		return targetSelector{}, "identifier too long"
	}
	if selector.group != "" && !workbenchIdentifierPattern.MatchString(selector.group) {
		return targetSelector{}, "invalid group"
	}
	if !workbenchKindPattern.MatchString(selector.kind) {
		return targetSelector{}, "invalid kind"
	}
	if !workbenchIdentifierPattern.MatchString(selector.name) {
		return targetSelector{}, "invalid name"
	}
	if !workbenchContainerPattern.MatchString(selector.container) {
		return targetSelector{}, "invalid container"
	}
	return selector, ""
}

// resolveGovernedTarget selects an existing canonical Container.Target from
// bounded discovery output. It never assembles a GovernedTarget from request
// strings: the request only narrows which already-discovered container is
// selected. A request that matches no discovered, governable container is
// indistinguishable from "unknown workload" — the domain does not
// distinguish "wrong kind" from "does not exist" at this boundary, and this
// function does not invent that distinction either.
func resolveGovernedTarget(result workload.Result, selector targetSelector) (k8s.GovernedTarget, workload.Workload, []k8s.RuntimeSubject, bool) {
	wanted := k8s.WorkloadRef{Group: selector.group, Kind: selector.kind, Name: selector.name}
	for _, w := range result.Workloads {
		if w.Target != wanted {
			continue
		}
		var subjects []k8s.RuntimeSubject
		var target k8s.GovernedTarget
		found := false
		for _, pod := range w.Pods {
			for _, c := range pod.Containers {
				if c.Target == nil || c.Target.Container != selector.container || c.Target.Workload != wanted {
					continue
				}
				if !found {
					target = *c.Target
					found = true
				}
				if c.Runtime != nil {
					subjects = append(subjects, *c.Runtime)
				}
			}
		}
		if found {
			return target, w, subjects, true
		}
		return k8s.GovernedTarget{}, workload.Workload{}, nil, false
	}
	return k8s.GovernedTarget{}, workload.Workload{}, nil, false
}

// --- Error translation boundary ---
//
// This is the one place a domain read error becomes an HTTP response. The
// typed ReadState is preserved in the response body alongside a fixed,
// generic reason per state — never the wrapped Kubernetes/client-go error
// text, which can name RBAC subjects, resource details, or other cluster
// internals a browser must not see. This is deliberately more conservative
// than Section.Reason inside a successful projection body, which is
// hand-authored G2 text and is passed through verbatim (see dtoFromSection).

type workbenchErrorBody struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

func writeWorkbenchTransportError(w http.ResponseWriter, err error) {
	var readErr *k8s.ReadError
	if errors.As(err, &readErr) {
		status, reason := workbenchReadStateHTTP(readErr.State)
		writeWorkbenchJSON(w, status, workbenchErrorBody{State: string(readErr.State), Reason: reason})
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeWorkbenchJSON(w, http.StatusGatewayTimeout, workbenchErrorBody{State: "TIMEOUT", Reason: "the bounded cluster read exceeded its deadline"})
		return
	}
	writeWorkbenchJSON(w, http.StatusInternalServerError, workbenchErrorBody{State: "UNKNOWN", Reason: "an internal error occurred"})
}

func workbenchReadStateHTTP(state k8s.ReadState) (int, string) {
	switch state {
	case k8s.ReadNotFound:
		return http.StatusNotFound, "the requested object was not found"
	case k8s.ReadPermissionDenied:
		return http.StatusForbidden, "the configured credentials do not permit this read"
	case k8s.ReadBackendNotInstalled:
		return http.StatusServiceUnavailable, "a required backend is not installed in this cluster"
	case k8s.ReadTimeout:
		return http.StatusGatewayTimeout, "the bounded cluster read exceeded its deadline"
	case k8s.ReadUnsupported:
		return http.StatusNotImplemented, "the requested resource is not supported"
	default:
		return http.StatusInternalServerError, "an internal error occurred"
	}
}

func writeWorkbenchClientError(w http.ResponseWriter, status int, reason string) {
	writeWorkbenchJSON(w, status, workbenchErrorBody{State: "INVALID_REQUEST", Reason: reason})
}

// writeWorkbenchJSON is the single JSON writer for this server. It buffers
// before writing so a response that would exceed the defense-in-depth cap
// never partially reaches the client — see workbenchMaxResponseBytes.
func writeWorkbenchJSON(w http.ResponseWriter, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		log.Printf("workbench: encoding response: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(encoded) > workbenchMaxResponseBytes {
		log.Printf("workbench: response exceeded %d bytes, refusing to send", workbenchMaxResponseBytes)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

// --- Explicit, lossless wire DTOs ---
//
// Every field below is an explicit copy of one projection/workload/
// association/k8s field. Nothing here is computed, summarized, or inferred;
// State/Reason/association verdicts/exclusion reasons are copied verbatim.
// This is a transport representation, not a second semantic model — see
// TestProjectionDTO_* for the losslessness proof.

type dtoSourceRef struct {
	Kind            string `json:"kind"`
	Namespace       string `json:"namespace,omitempty"`
	Name            string `json:"name"`
	UID             string `json:"uid,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

func dtoFromSourceRef(s projection.SourceRef) dtoSourceRef {
	return dtoSourceRef{Kind: s.Kind, Namespace: s.Namespace, Name: s.Name, UID: s.UID, ResourceVersion: s.ResourceVersion}
}

func dtoFromSourceRefs(sources []projection.SourceRef) []dtoSourceRef {
	if sources == nil {
		return nil
	}
	out := make([]dtoSourceRef, len(sources))
	for i, s := range sources {
		out[i] = dtoFromSourceRef(s)
	}
	return out
}

type dtoWorkloadRef struct {
	Group string `json:"group,omitempty"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
}

func dtoFromWorkloadRef(r k8s.WorkloadRef) dtoWorkloadRef {
	return dtoWorkloadRef{Group: r.Group, Kind: r.Kind, Name: r.Name}
}

type dtoGovernedTarget struct {
	Namespace string         `json:"namespace"`
	Workload  dtoWorkloadRef `json:"workload"`
	Container string         `json:"container"`
}

func dtoFromGovernedTarget(t k8s.GovernedTarget) dtoGovernedTarget {
	return dtoGovernedTarget{Namespace: t.Namespace, Workload: dtoFromWorkloadRef(t.Workload), Container: t.Container}
}

type dtoSection struct {
	State   string         `json:"state"`
	Reason  string         `json:"reason,omitempty"`
	Sources []dtoSourceRef `json:"sources,omitempty"`
}

func dtoFromSection(s projection.Section) dtoSection {
	return dtoSection{State: string(s.State), Reason: s.Reason, Sources: dtoFromSourceRefs(s.Sources)}
}

type dtoPodReadObservation struct {
	Pod         dtoSourceRef `json:"pod"`
	State       string       `json:"state"`
	Reason      string       `json:"reason,omitempty"`
	Contributed bool         `json:"contributed"`
}

type dtoDeclaredContainer struct {
	PodName          string   `json:"podName"`
	Container        string   `json:"container"`
	CapabilitiesAdd  []string `json:"capabilitiesAdd,omitempty"`
	CapabilitiesDrop []string `json:"capabilitiesDrop,omitempty"`
	SeccompType      string   `json:"seccompType,omitempty"`
	LocalhostProfile string   `json:"localhostProfile,omitempty"`
}

type dtoDeclaredConfiguration struct {
	dtoSection
	Containers   []dtoDeclaredContainer  `json:"containers,omitempty"`
	Observations []dtoPodReadObservation `json:"observations,omitempty"`
}

func dtoFromDeclared(d projection.DeclaredConfiguration) dtoDeclaredConfiguration {
	out := dtoDeclaredConfiguration{dtoSection: dtoFromSection(d.Section)}
	for _, c := range d.Containers {
		out.Containers = append(out.Containers, dtoDeclaredContainer{
			PodName: c.PodName, Container: c.Container,
			CapabilitiesAdd: c.CapabilitiesAdd, CapabilitiesDrop: c.CapabilitiesDrop,
			SeccompType: c.SeccompType, LocalhostProfile: c.LocalhostProfile,
		})
	}
	for _, o := range d.Observations {
		out.Observations = append(out.Observations, dtoPodReadObservation{
			Pod: dtoFromSourceRef(o.Pod), State: string(o.State), Reason: o.Reason, Contributed: o.Contributed,
		})
	}
	return out
}

type dtoOptionalBackendObservation struct {
	Pod     dtoSourceRef   `json:"pod"`
	State   string         `json:"state"`
	Reason  string         `json:"reason,omitempty"`
	Sources []dtoSourceRef `json:"sources,omitempty"`
}

func dtoFromOptionalBackendObservations(obs []projection.OptionalBackendObservation) []dtoOptionalBackendObservation {
	if obs == nil {
		return nil
	}
	out := make([]dtoOptionalBackendObservation, len(obs))
	for i, o := range obs {
		out[i] = dtoOptionalBackendObservation{Pod: dtoFromSourceRef(o.Pod), State: string(o.State), Reason: o.Reason, Sources: dtoFromSourceRefs(o.Sources)}
	}
	return out
}

type dtoNetworkPolicy struct {
	Source       dtoSourceRef   `json:"source"`
	MatchedPods  []dtoSourceRef `json:"matchedPods,omitempty"`
	PolicyTypes  []string       `json:"policyTypes,omitempty"`
	IngressRules int            `json:"ingressRules"`
	EgressRules  int            `json:"egressRules"`
}

type dtoMaterializedPolicy struct {
	dtoSection
	PodLockState        string                          `json:"podLockState"`
	SPOState            string                          `json:"spoState"`
	PodLocks            []dtoSourceRef                  `json:"podLocks,omitempty"`
	SPOProfiles         []dtoSourceRef                  `json:"spoProfiles,omitempty"`
	NetworkPolicies     []dtoNetworkPolicy              `json:"networkPolicies,omitempty"`
	PodLockObservations []dtoOptionalBackendObservation `json:"podLockObservations,omitempty"`
	SPOObservations     []dtoOptionalBackendObservation `json:"spoObservations,omitempty"`
}

func dtoFromMaterialized(m projection.MaterializedPolicy) dtoMaterializedPolicy {
	out := dtoMaterializedPolicy{
		dtoSection: dtoFromSection(m.Section), PodLockState: string(m.PodLockState), SPOState: string(m.SPOState),
		PodLocks: dtoFromSourceRefs(m.PodLocks), SPOProfiles: dtoFromSourceRefs(m.SPOProfiles),
		PodLockObservations: dtoFromOptionalBackendObservations(m.PodLockObservations),
		SPOObservations:     dtoFromOptionalBackendObservations(m.SPOObservations),
	}
	for _, np := range m.NetworkPolicies {
		out.NetworkPolicies = append(out.NetworkPolicies, dtoNetworkPolicy{
			Source: dtoFromSourceRef(np.Source), MatchedPods: dtoFromSourceRefs(np.MatchedPods),
			PolicyTypes: np.PolicyTypes, IngressRules: np.IngressRules, EgressRules: np.EgressRules,
		})
	}
	return out
}

type dtoBinding struct {
	Backend string            `json:"backend"`
	Source  dtoSourceRef      `json:"source"`
	Target  dtoGovernedTarget `json:"target"`
	Detail  string            `json:"detail,omitempty"`
}

type dtoBindingEvidence struct {
	dtoSection
	Bindings []dtoBinding `json:"bindings,omitempty"`
}

func dtoFromBinding(b projection.BindingEvidence) dtoBindingEvidence {
	out := dtoBindingEvidence{dtoSection: dtoFromSection(b.Section)}
	for _, item := range b.Bindings {
		out.Bindings = append(out.Bindings, dtoBinding{
			Backend: item.Backend, Source: dtoFromSourceRef(item.Source), Target: dtoFromGovernedTarget(item.Target), Detail: item.Detail,
		})
	}
	return out
}

type dtoAssociationResult struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

func dtoFromAssociationResult(r association.Result) dtoAssociationResult {
	return dtoAssociationResult{State: string(r.State), Reason: r.Reason}
}

type dtoRuntimeSubject struct {
	Target     dtoGovernedTarget `json:"target"`
	PodUID     string            `json:"podUID,omitempty"`
	ImageID    string            `json:"imageID,omitempty"`
	BinaryPath string            `json:"binaryPath,omitempty"`
}

func dtoFromRuntimeSubject(s k8s.RuntimeSubject) dtoRuntimeSubject {
	return dtoRuntimeSubject{Target: dtoFromGovernedTarget(s.Target), PodUID: s.PodUID, ImageID: s.ImageID, BinaryPath: s.BinaryPath}
}

type dtoRuntimeCompatibility struct {
	Subject dtoRuntimeSubject `json:"subject"`
	State   string            `json:"state"`
	Reason  string            `json:"reason,omitempty"`
}

type dtoEvidenceSource struct {
	Target        *dtoGovernedTarget `json:"target,omitempty"`
	Container     string             `json:"container,omitempty"`
	ImageIdentity string             `json:"imageIdentity,omitempty"`
	BinaryPath    string             `json:"binaryPath,omitempty"`
}

func dtoFromEvidenceSource(e association.Evidence) dtoEvidenceSource {
	out := dtoEvidenceSource{Container: e.Population.Container, ImageIdentity: e.Population.ImageIdentity, BinaryPath: e.Population.BinaryPath}
	if e.Target != nil {
		t := dtoFromGovernedTarget(*e.Target)
		out.Target = &t
	}
	return out
}

type dtoEvidence struct {
	Source        dtoEvidenceSource         `json:"source"`
	Association   dtoAssociationResult      `json:"association"`
	Compatibility []dtoRuntimeCompatibility `json:"compatibility,omitempty"`
}

type dtoExcludedEvidence struct {
	Source      dtoEvidenceSource    `json:"source"`
	Association dtoAssociationResult `json:"association"`
}

type dtoRuntimeEvidence struct {
	dtoSection
	Evidence []dtoEvidence         `json:"evidence,omitempty"`
	Excluded []dtoExcludedEvidence `json:"excluded,omitempty"`
}

func dtoFromRuntime(r projection.RuntimeEvidence) dtoRuntimeEvidence {
	out := dtoRuntimeEvidence{dtoSection: dtoFromSection(r.Section)}
	for _, e := range r.Evidence {
		item := dtoEvidence{Source: dtoFromEvidenceSource(e.Source), Association: dtoFromAssociationResult(e.Association)}
		for _, c := range e.Compatibility {
			item.Compatibility = append(item.Compatibility, dtoRuntimeCompatibility{
				Subject: dtoFromRuntimeSubject(c.Subject), State: string(c.Compatibility.State), Reason: c.Compatibility.Reason,
			})
		}
		out.Evidence = append(out.Evidence, item)
	}
	for _, e := range r.Excluded {
		out.Excluded = append(out.Excluded, dtoExcludedEvidence{Source: dtoFromEvidenceSource(e.Source), Association: dtoFromAssociationResult(e.Association)})
	}
	return out
}

type dtoDerivedArtifact struct {
	Proposal string `json:"proposal"`
	Backend  string `json:"backend"`
	Present  bool   `json:"present"`
}

type dtoDerivedPolicy struct {
	dtoSection
	Artifacts []dtoDerivedArtifact `json:"artifacts,omitempty"`
}

func dtoFromDerived(d projection.DerivedPolicy) dtoDerivedPolicy {
	out := dtoDerivedPolicy{dtoSection: dtoFromSection(d.Section)}
	for _, a := range d.Artifacts {
		out.Artifacts = append(out.Artifacts, dtoDerivedArtifact{Proposal: a.Proposal, Backend: a.Backend, Present: a.Present})
	}
	return out
}

type dtoProposalState struct {
	Source                  dtoSourceRef         `json:"source"`
	Association             dtoAssociationResult `json:"association"`
	CandidateDigest         string               `json:"candidateDigest,omitempty"`
	ApprovedCandidateDigest string               `json:"approvedCandidateDigest,omitempty"`
	ApprovalBindingValid    bool                 `json:"approvalBindingValid"`
	ApprovalBindingReason   string               `json:"approvalBindingReason,omitempty"`
	ApprovalState           string               `json:"approvalState,omitempty"`
	Applied                 string               `json:"applied,omitempty"`
}

type dtoExcludedProposal struct {
	Source      dtoSourceRef          `json:"source"`
	Exclusion   string                `json:"exclusion"`
	Association *dtoAssociationResult `json:"association,omitempty"`
	Reason      string                `json:"reason,omitempty"`
}

type dtoProposalGovernance struct {
	dtoSection
	Proposals []dtoProposalState    `json:"proposals,omitempty"`
	Excluded  []dtoExcludedProposal `json:"excluded,omitempty"`
}

func dtoFromGovernance(g projection.ProposalGovernance) dtoProposalGovernance {
	out := dtoProposalGovernance{dtoSection: dtoFromSection(g.Section)}
	for _, p := range g.Proposals {
		out.Proposals = append(out.Proposals, dtoProposalState{
			Source: dtoFromSourceRef(p.Source), Association: dtoFromAssociationResult(p.Association),
			CandidateDigest: p.CandidateDigest, ApprovedCandidateDigest: p.ApprovedCandidateDigest,
			ApprovalBindingValid: p.ApprovalBindingValid, ApprovalBindingReason: p.ApprovalBindingReason,
			ApprovalState: p.ApprovalState, Applied: string(p.Applied),
		})
	}
	for _, e := range g.Excluded {
		item := dtoExcludedProposal{Source: dtoFromSourceRef(e.Source), Exclusion: string(e.Exclusion), Reason: e.Reason}
		if e.Association != nil {
			assoc := dtoFromAssociationResult(*e.Association)
			item.Association = &assoc
		}
		out.Excluded = append(out.Excluded, item)
	}
	return out
}

type dtoProjection struct {
	Target                 dtoGovernedTarget        `json:"target"`
	Declared               dtoDeclaredConfiguration `json:"declared"`
	Materialized           dtoMaterializedPolicy    `json:"materialized"`
	Binding                dtoBindingEvidence       `json:"binding"`
	Enforcement            dtoSection               `json:"enforcement"`
	BehavioralVerification dtoSection               `json:"behavioralVerification"`
	Runtime                dtoRuntimeEvidence       `json:"runtime"`
	Derived                dtoDerivedPolicy         `json:"derived"`
	Governance             dtoProposalGovernance    `json:"governance"`
}

func dtoFromProjection(p projection.WorkloadSecurityProjection) dtoProjection {
	return dtoProjection{
		Target:                 dtoFromGovernedTarget(p.Target),
		Declared:               dtoFromDeclared(p.Declared),
		Materialized:           dtoFromMaterialized(p.Materialized),
		Binding:                dtoFromBinding(p.Binding),
		Enforcement:            dtoFromSection(p.Enforcement.Section),
		BehavioralVerification: dtoFromSection(p.BehavioralVerification.Section),
		Runtime:                dtoFromRuntime(p.Runtime),
		Derived:                dtoFromDerived(p.Derived),
		Governance:             dtoFromGovernance(p.Governance),
	}
}

// --- Discovery DTO for /api/workloads ---

type dtoContainer struct {
	Name            string             `json:"name"`
	Category        string             `json:"category"`
	SupportedTarget bool               `json:"supportedTarget"`
	Target          *dtoGovernedTarget `json:"target,omitempty"`
	Runtime         *dtoRuntimeSubject `json:"runtime,omitempty"`
	RuntimeState    string             `json:"runtimeState"`
}

type dtoPod struct {
	Name                   string         `json:"name"`
	UID                    string         `json:"uid,omitempty"`
	Containers             []dtoContainer `json:"containers"`
	UnmatchedRuntimeStatus []string       `json:"unmatchedRuntimeStatus,omitempty"`
}

type dtoWorkload struct {
	Target    dtoWorkloadRef `json:"target"`
	UID       string         `json:"uid,omitempty"`
	Owner     string         `json:"owner"`
	OwnerNote string         `json:"ownerNote,omitempty"`
	Pods      []dtoPod       `json:"pods"`
}

type dtoDiscoveryResult struct {
	State     string        `json:"state"`
	Namespace string        `json:"namespace"`
	Workloads []dtoWorkload `json:"workloads,omitempty"`
}

func dtoFromDiscoveryResult(result workload.Result) dtoDiscoveryResult {
	out := dtoDiscoveryResult{State: string(result.State), Namespace: result.Namespace}
	for _, w := range result.Workloads {
		item := dtoWorkload{Target: dtoFromWorkloadRef(w.Target), UID: w.UID, Owner: string(w.Owner), OwnerNote: w.OwnerNote}
		for _, pod := range w.Pods {
			podItem := dtoPod{Name: pod.Name, UID: pod.UID, UnmatchedRuntimeStatus: pod.UnmatchedRuntimeStatus}
			for _, c := range pod.Containers {
				containerItem := dtoContainer{
					Name: c.Name, Category: string(c.Category), SupportedTarget: c.SupportedTarget, RuntimeState: string(c.RuntimeState),
				}
				if c.Target != nil {
					t := dtoFromGovernedTarget(*c.Target)
					containerItem.Target = &t
				}
				if c.Runtime != nil {
					rt := dtoFromRuntimeSubject(*c.Runtime)
					containerItem.Runtime = &rt
				}
				podItem.Containers = append(podItem.Containers, containerItem)
			}
			item.Pods = append(item.Pods, podItem)
		}
		out.Workloads = append(out.Workloads, item)
	}
	return out
}
