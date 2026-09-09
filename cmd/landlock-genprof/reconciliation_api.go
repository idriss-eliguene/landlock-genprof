package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/reconciliation"
)

const (
	v08EnvironmentPath = "/api/v08/environment"
	v08HistoryPath     = "/api/v08/history"
	v08MaxLimit        = 100
)

var v08ImageDigest = regexp.MustCompile(`^sha256:[0-9a-fA-F]{64}$`)

type v08Loaded struct {
	environment reconciliation.EnvironmentInputs
	history     reconciliation.HistoryInputs
	diagnostics projectionDiagnostics
}

func (l v08Loaded) diagnosticFor(kind, namespace, name, uid string) (projectionDiagnostic, bool) {
	return l.diagnostics.diagnosticFor(kind, namespace, name, uid)
}

func (s *workbenchServer) handleV08Environment(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == v08EnvironmentPath {
		if len(r.URL.Query()) > 1 {
			writeWorkbenchClientError(w, http.StatusBadRequest, "only limit is accepted")
			return
		}
		limit, err := parseV08Limit(r.URL.Query().Get("limit"))
		if err != nil {
			writeWorkbenchClientError(w, http.StatusBadRequest, err.Error())
			return
		}
		loaded, err := s.loadV08Inputs(r.Context())
		if err != nil {
			writeWorkbenchTransportError(w, err)
			return
		}
		loaded.environment.Limit = limit
		projection, err := reconciliation.ProjectEnvironment(loaded.environment)
		if err != nil {
			writeWorkbenchTransportError(w, err)
			return
		}
		writeWorkbenchJSON(w, http.StatusOK, v08EnvironmentResponse{Items: projection.Entries, TotalCount: projection.TotalCount, Truncated: projection.Truncated, UnattributedFailedObservationCount: projection.UnattributedFailedObservationCount, Limitation: "BEST_EFFORT_MULTI_OBJECT_READ", ProjectionDiagnostics: loaded.diagnostics})
		return
	}
	if r.URL.Path != v08EnvironmentPath+"/detail" {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query()
	query.Del("limit")
	subject, err := parseV08Subject(query, s.reads.SessionIdentity().Namespace)
	if err != nil {
		writeWorkbenchClientError(w, http.StatusBadRequest, err.Error())
		return
	}
	loaded, err := s.loadV08Inputs(r.Context())
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	loaded.environment.Limit = 0
	projection, err := reconciliation.ProjectEnvironment(loaded.environment)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	for _, entry := range projection.Entries {
		if reconciliation.SubjectsEqual(entry.Subject, subject) {
			writeWorkbenchJSON(w, http.StatusOK, v08EnvironmentDetailResponse{Entry: entry, Limitation: "BEST_EFFORT_MULTI_OBJECT_READ", ProjectionDiagnostics: loaded.diagnostics})
			return
		}
	}
	writeWorkbenchClientError(w, http.StatusNotFound, "environment subject not found")
}

func (s *workbenchServer) handleV08History(w http.ResponseWriter, r *http.Request) {
	limit, err := parseV08Limit(r.URL.Query().Get("limit"))
	if err != nil {
		writeWorkbenchClientError(w, http.StatusBadRequest, err.Error())
		return
	}
	loaded, err := s.loadV08Inputs(r.Context())
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	loaded.history.Limit = limit
	if r.URL.Path == v08HistoryPath+"/proposal" {
		name := r.URL.Query().Get("name")
		uid := r.URL.Query().Get("uid")
		if name == "" || strings.Contains(name, "/") || len(r.URL.Query()) > 3 {
			writeWorkbenchClientError(w, http.StatusBadRequest, "name and optional uid are required")
			return
		}
		ref := reconciliation.ProposalRef{Namespace: s.reads.SessionIdentity().Namespace, Name: name, UID: uid}
		found := false
		for _, p := range loaded.history.Proposals {
			if p.Ref == ref || (uid == "" && p.Ref.Namespace == ref.Namespace && p.Ref.Name == ref.Name) {
				ref = p.Ref
				found = true
				break
			}
		}
		if !found {
			if diagnostic, ok := loaded.diagnosticFor("SecurityProfileProposal", s.reads.SessionIdentity().Namespace, name, uid); ok {
				writeWorkbenchJSON(w, http.StatusUnprocessableEntity, malformedObjectResponse{State: "MALFORMED_OBJECT", Diagnostic: diagnostic})
				return
			}
			writeWorkbenchClientError(w, http.StatusNotFound, "proposal not found")
			return
		}
		projection, err := reconciliation.ProjectProposalHistory(ref, loaded.history)
		if err != nil {
			writeWorkbenchTransportError(w, err)
			return
		}
		writeWorkbenchJSON(w, http.StatusOK, v08HistoryResponse{Projection: projection, Limitation: "BEST_EFFORT_MULTI_OBJECT_READ", ProjectionDiagnostics: loaded.diagnostics})
		return
	}
	query := r.URL.Query()
	query.Del("limit")
	subject, err := parseV08Subject(query, s.reads.SessionIdentity().Namespace)
	if err != nil {
		writeWorkbenchClientError(w, http.StatusBadRequest, err.Error())
		return
	}
	projection, err := reconciliation.ProjectPopulationHistory(subject, loaded.history)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	writeWorkbenchJSON(w, http.StatusOK, v08HistoryResponse{Projection: projection, Limitation: "BEST_EFFORT_MULTI_OBJECT_READ", ProjectionDiagnostics: loaded.diagnostics})
}

type v08EnvironmentResponse struct {
	Items                              []reconciliation.EnvironmentEntry `json:"items"`
	TotalCount                         int                               `json:"totalCount"`
	Truncated                          bool                              `json:"truncated"`
	UnattributedFailedObservationCount int                               `json:"unattributedFailedObservationCount"`
	Limitation                         string                            `json:"limitation"`
	ProjectionDiagnostics
}
type v08EnvironmentDetailResponse struct {
	Entry      reconciliation.EnvironmentEntry `json:"entry"`
	Limitation string                          `json:"limitation"`
	ProjectionDiagnostics
}
type v08HistoryResponse struct {
	Projection reconciliation.HistoryProjection `json:"history"`
	Limitation string                           `json:"limitation"`
	ProjectionDiagnostics
}

type ProjectionDiagnostics = projectionDiagnostics

type malformedObjectResponse struct {
	State      string               `json:"state"`
	Diagnostic projectionDiagnostic `json:"diagnostic"`
}

func parseV08Limit(value string) (int, error) {
	if value == "" {
		return v08MaxLimit, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > v08MaxLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", v08MaxLimit)
	}
	return n, nil
}

func parseV08Subject(q map[string][]string, namespace string) (reconciliation.EnvironmentSubject, error) {
	allowed := map[string]bool{"scope": true, "target": true, "container": true, "imageIdentity": true, "binaryPath": true}
	if len(q) != 5 {
		return reconciliation.EnvironmentSubject{}, fmt.Errorf("scope, target, container, imageIdentity, and binaryPath are required")
	}
	get := func(k string) (string, error) {
		values, ok := q[k]
		if !ok || len(values) != 1 || (k != "binaryPath" && values[0] == "") {
			return "", fmt.Errorf("%s is required exactly once", k)
		}
		return values[0], nil
	}
	for key := range q {
		if !allowed[key] {
			return reconciliation.EnvironmentSubject{}, fmt.Errorf("unsupported subject parameter %q", key)
		}
	}
	scope, err := get("scope")
	if err != nil {
		return reconciliation.EnvironmentSubject{}, err
	}
	target, err := get("target")
	if err != nil {
		return reconciliation.EnvironmentSubject{}, err
	}
	container, err := get("container")
	if err != nil {
		return reconciliation.EnvironmentSubject{}, err
	}
	image, err := get("imageIdentity")
	if err != nil {
		return reconciliation.EnvironmentSubject{}, err
	}
	binary, err := get("binaryPath")
	if err != nil {
		return reconciliation.EnvironmentSubject{}, err
	}
	if scope != string(history.ScopeContainer) && scope != string(history.ScopeBinary) {
		return reconciliation.EnvironmentSubject{}, fmt.Errorf("invalid scope")
	}
	if !v08ImageDigest.MatchString(image) {
		return reconciliation.EnvironmentSubject{}, fmt.Errorf("invalid imageIdentity")
	}
	if strings.ContainsAny(target, "\r\n") || strings.Contains(target, "//") || strings.HasPrefix(target, "/") {
		return reconciliation.EnvironmentSubject{}, fmt.Errorf("invalid target")
	}
	if strings.TrimSpace(namespace) == "" {
		return reconciliation.EnvironmentSubject{}, fmt.Errorf("read namespace is unresolved")
	}
	if scope == string(history.ScopeContainer) && binary != "" || scope == string(history.ScopeBinary) && binary == "" {
		return reconciliation.EnvironmentSubject{}, fmt.Errorf("binaryPath does not match scope")
	}
	subject := reconciliation.EnvironmentSubject{Scope: history.PopulationScope(scope), Target: target, Container: container, ImageIdentity: image, BinaryPath: binary}
	if err := subject.Validate(); err != nil {
		return reconciliation.EnvironmentSubject{}, fmt.Errorf("invalid subject: %w", err)
	}
	return subject, nil
}

func (s *workbenchServer) loadV08Inputs(ctx context.Context) (v08Loaded, error) {
	ctx, cancel := context.WithTimeout(ctx, workbenchClusterReadDeadline)
	defer cancel()
	observations, err := s.reads.ListObservations(ctx)
	if err != nil {
		return v08Loaded{}, err
	}
	proposals, err := s.reads.ListProposals(ctx)
	if err != nil {
		return v08Loaded{}, err
	}
	histories, err := s.reads.ListTrainingHistory(ctx)
	if err != nil {
		return v08Loaded{}, err
	}
	applies, err := s.reads.ListApplyAttempts(ctx)
	if err != nil {
		return v08Loaded{}, err
	}
	rollbacks, err := s.reads.ListRollbackAttempts(ctx)
	if err != nil {
		return v08Loaded{}, err
	}
	loaded := v08Loaded{environment: reconciliation.EnvironmentInputs{}, history: reconciliation.HistoryInputs{}}
	for i := range observations.Items {
		value, e := decodeV08Observation(&observations.Items[i])
		if e != nil {
			loaded.diagnostics.addMalformed("Observation", "excluded from environment and history projection", metadataOf(&observations.Items[i]), e)
			continue
		}
		loaded.diagnostics.addValid()
		loaded.environment.Observations = append(loaded.environment.Observations, value.Observation)
		loaded.history.Observations = append(loaded.history.Observations, value)
	}
	for i := range proposals.Items {
		value, e := decodeV08Proposal(&proposals.Items[i])
		if e != nil {
			loaded.diagnostics.addMalformed("SecurityProfileProposal", "excluded from environment and history projection", metadataOf(&proposals.Items[i]), e)
			continue
		}
		loaded.diagnostics.addValid()
		loaded.environment.Proposals = append(loaded.environment.Proposals, value)
		loaded.history.Proposals = append(loaded.history.Proposals, value)
	}
	for i := range histories.Items {
		values, e := decodeV08History(&histories.Items[i])
		if e != nil {
			loaded.diagnostics.addMalformed("TrainingHistory", "excluded from environment and history projection", metadataOf(&histories.Items[i]), e)
			continue
		}
		loaded.diagnostics.addValid()
		loaded.environment.Populations = append(loaded.environment.Populations, values...)
		for _, value := range values {
			loaded.history.Populations = append(loaded.history.Populations, reconciliation.HistoryPopulationInput{Population: value, Namespace: histories.Items[i].GetNamespace(), Name: histories.Items[i].GetName(), UID: string(histories.Items[i].GetUID())})
		}
	}
	for i := range applies.Items {
		value, e := decodeV08Apply(&applies.Items[i])
		if e != nil {
			loaded.diagnostics.addMalformed("ApplyAttempt", "excluded from environment and history projection", metadataOf(&applies.Items[i]), e)
			continue
		}
		loaded.diagnostics.addValid()
		loaded.environment.ApplyAttempts = append(loaded.environment.ApplyAttempts, value)
		loaded.history.ApplyAttempts = append(loaded.history.ApplyAttempts, value)
	}
	for i := range rollbacks.Items {
		value, e := decodeV08Rollback(&rollbacks.Items[i])
		if e != nil {
			loaded.diagnostics.addMalformed("RollbackAttempt", "excluded from environment and history projection", metadataOf(&rollbacks.Items[i]), e)
			continue
		}
		loaded.diagnostics.addValid()
		loaded.environment.RollbackAttempts = append(loaded.environment.RollbackAttempts, value)
		loaded.history.RollbackAttempts = append(loaded.history.RollbackAttempts, value)
	}
	loaded.diagnostics.finalize()
	return loaded, nil
}

func decodeV08Observation(obj *unstructured.Unstructured) (reconciliation.HistoryObservationInput, error) {
	value, err := obskube.FromUnstructured(obj)
	if err != nil {
		return reconciliation.HistoryObservationInput{}, err
	}
	return reconciliation.HistoryObservationInput{Observation: value, Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), CreatedAt: obj.GetCreationTimestamp().Time}, nil
}
func decodeV08Proposal(obj *unstructured.Unstructured) (reconciliation.ProposalInput, error) {
	specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return reconciliation.ProposalInput{}, fmt.Errorf("proposal %s has invalid spec", obj.GetName())
	}
	var spec proposal.Spec
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &spec); err != nil {
		return reconciliation.ProposalInput{}, err
	}
	if err = proposal.ValidateProposalSpec(spec); err != nil {
		return reconciliation.ProposalInput{}, err
	}
	var status proposal.Status
	if statusMap, ok, _ := unstructured.NestedMap(obj.Object, "status"); ok {
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(statusMap, &status); err != nil {
			return reconciliation.ProposalInput{}, err
		}
	}
	return reconciliation.ProposalInput{CandidateProposal: reconciliation.CandidateProposal{Ref: reconciliation.ProposalRef{Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID())}, Spec: spec, Status: status}, CreatedAt: obj.GetCreationTimestamp().Time}, nil
}
func decodeV08History(obj *unstructured.Unstructured) ([]history.Population, error) {
	specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("history %s has invalid spec", obj.GetName())
	}
	var record history.Record
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &record); err != nil {
		return nil, err
	}
	for i := range record.Populations {
		normalized, e := history.NormalizeLegacyScope(record.Populations[i])
		if e != nil {
			return nil, e
		}
		record.Populations[i] = normalized
	}
	return record.Populations, nil
}
func decodeV08Apply(obj *unstructured.Unstructured) (reconciliation.ApplyAttemptInput, error) {
	return decodeV08Attempt[attempt.Spec](obj, "ApplyAttempt")
}
func decodeV08Rollback(obj *unstructured.Unstructured) (reconciliation.RollbackAttemptInput, error) {
	var spec attempt.RollbackSpec
	status, err := decodeV08Status(obj, &spec)
	if err != nil {
		return reconciliation.RollbackAttemptInput{}, err
	}
	return reconciliation.RollbackAttemptInput{Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), CreatedAt: obj.GetCreationTimestamp().Time, Spec: spec, Status: status}, nil
}
func decodeV08Attempt[T any](obj *unstructured.Unstructured, kind string) (reconciliation.ApplyAttemptInput, error) {
	var spec T
	status, err := decodeV08Status(obj, &spec)
	if err != nil {
		return reconciliation.ApplyAttemptInput{}, fmt.Errorf("decoding %s: %w", kind, err)
	}
	typed, ok := any(spec).(attempt.Spec)
	if !ok {
		return reconciliation.ApplyAttemptInput{}, fmt.Errorf("invalid %s spec", kind)
	}
	return reconciliation.ApplyAttemptInput{Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), CreatedAt: obj.GetCreationTimestamp().Time, Spec: typed, Status: status}, nil
}
func decodeV08Status(obj *unstructured.Unstructured, spec any) (attempt.Status, error) {
	specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return attempt.Status{}, fmt.Errorf("invalid spec")
	}
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, spec); err != nil {
		return attempt.Status{}, err
	}
	statusMap, _, err := unstructured.NestedMap(obj.Object, "status")
	if err != nil {
		return attempt.Status{}, err
	}
	var status attempt.Status
	if statusMap != nil {
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(statusMap, &status); err != nil {
			return attempt.Status{}, err
		}
	}
	return status, nil
}
