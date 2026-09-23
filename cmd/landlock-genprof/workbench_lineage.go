package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type lineageDiagnostics struct {
	ExcludedMalformed              int `json:"excludedMalformed"`
	ExcludedInsufficientProvenance int `json:"excludedInsufficientProvenance"`
	ExcludedMixedIdentity          int `json:"excludedMixedIdentity"`
	ExcludedNotAssociated          int `json:"excludedNotAssociated"`
}

type lineageResponse struct {
	Items       any                `json:"items"`
	Complete    bool               `json:"complete"`
	Continue    string             `json:"continue,omitempty"`
	Diagnostics lineageDiagnostics `json:"diagnostics"`
}

type workloadPolicyRead struct {
	Identity    observationIdentity `json:"identity"`
	Proposals   []proposalRead      `json:"proposals"`
	Complete    bool                `json:"complete"`
	Continue    string              `json:"continue,omitempty"`
	Diagnostics lineageDiagnostics  `json:"diagnostics"`
}

type lineageAssociation string

const (
	lineageExact         lineageAssociation = "EXACT"
	lineageInsufficient  lineageAssociation = "INSUFFICIENT_PROVENANCE"
	lineageMixed         lineageAssociation = "MIXED_IDENTITY"
	lineageNotAssociated lineageAssociation = "NOT_ASSOCIATED"
)

type lineagePager interface {
	ListObservationsPage(context.Context, string) (*unstructured.UnstructuredList, error)
	ListProposalsPage(context.Context, string) (*unstructured.UnstructuredList, error)
	ListApplyAttemptsPage(context.Context, string) (*unstructured.UnstructuredList, error)
	ListRollbackAttemptsPage(context.Context, string) (*unstructured.UnstructuredList, error)
}

type lineageAttemptRead struct {
	Namespace               string                   `json:"namespace"`
	Name                    string                   `json:"name"`
	UID                     string                   `json:"uid"`
	ResourceVersion         string                   `json:"resourceVersion,omitempty"`
	ProposalNamespace       string                   `json:"proposalNamespace,omitempty"`
	ProposalName            string                   `json:"proposalName,omitempty"`
	ProposalUID             string                   `json:"proposalUID,omitempty"`
	SourceNamespace         string                   `json:"sourceNamespace,omitempty"`
	SourceName              string                   `json:"sourceName,omitempty"`
	SourceUID               string                   `json:"sourceUID,omitempty"`
	ApprovedCandidateDigest string                   `json:"approvedCandidateDigest,omitempty"`
	Target                  string                   `json:"target,omitempty"`
	Operator                string                   `json:"operator,omitempty"`
	CustodyEpoch            string                   `json:"custodyEpoch,omitempty"`
	State                   string                   `json:"state,omitempty"`
	StartedAt               string                   `json:"startedAt,omitempty"`
	UpdatedAt               string                   `json:"updatedAt,omitempty"`
	CompletedAt             string                   `json:"completedAt,omitempty"`
	Failure                 any                      `json:"failure,omitempty"`
	Mutations               []attempt.MutationRecord `json:"mutations,omitempty"`
}

func (s *workbenchServer) requireLineageCapability(w http.ResponseWriter, r *http.Request, caps ...authz.Capability) bool {
	if s.discoverCaps == nil {
		return true
	}
	for _, cap := range caps {
		if !s.capabilityAllowed(r.Context(), cap) {
			writeWorkbenchJSON(w, http.StatusForbidden, map[string]string{"state": "AUTHORIZATION_DENIED", "reason": "required read capability is not granted"})
			return false
		}
	}
	return true
}

func parseLineageSelector(r *http.Request) (readModelSelector, string) {
	q := r.URL.Query()
	allowed := map[string]bool{"namespace": true, "group": true, "kind": true, "name": true, "workloadUID": true, "container": true, "continue": true}
	for key := range q {
		if !allowed[key] {
			return readModelSelector{}, fmt.Sprintf("unsupported query parameter %q", key)
		}
	}
	selectorQuery := make(map[string][]string)
	for _, key := range []string{"group", "kind", "name", "container", "workloadUID"} {
		if values, ok := q[key]; ok {
			selectorQuery[key] = values
		}
	}
	s, why := parseReadModelSelector(selectorQuery)
	if why != "" {
		return s, why
	}
	if q.Get("namespace") == "" {
		return s, "namespace is required"
	}
	return s, ""
}

func exactObservationIdentities(ctx context.Context, reads k8s.WorkbenchReadCapability) (map[string]observationIdentity, bool, error) {
	out := make(map[string]observationIdentity)
	token := ""
	complete := true
	for {
		var list *unstructured.UnstructuredList
		var err error
		if token == "" {
			list, err = reads.ListObservations(ctx)
		} else if pager, ok := reads.(lineagePager); ok {
			list, err = pager.ListObservationsPage(ctx, token)
		} else {
			complete = false
			break
		}
		if err != nil {
			return nil, false, err
		}
		for i := range list.Items {
			p, e := observationProjection(&list.Items[i])
			if e == nil {
				out[p.ID] = p.Identity
			}
		}
		token = list.GetContinue()
		if token == "" {
			break
		}
	}
	return out, complete, nil
}

func sameWorkloadIdentity(a, b observationIdentity) bool {
	return a.ClusterIdentity == b.ClusterIdentity && a.Namespace == b.Namespace && a.Group == b.Group && a.Kind == b.Kind && a.WorkloadName == b.WorkloadName && a.WorkloadUID == b.WorkloadUID && a.Container == b.Container
}

func classifyProposalLineage(p proposalRead, observations map[string]observationIdentity, selected observationIdentity) lineageAssociation {
	if p.Provenance == nil || len(p.Provenance.ObservationIDs) == 0 || p.Provenance.PopulationScope != "CONTAINER" {
		return lineageInsufficient
	}
	var first observationIdentity
	for _, id := range p.Provenance.ObservationIDs {
		identity, ok := observations[id]
		if !ok {
			return lineageInsufficient
		}
		if first.WorkloadUID == "" {
			first = identity
			continue
		}
		if !sameWorkloadIdentity(first, identity) {
			return lineageMixed
		}
	}
	if !sameWorkloadIdentity(first, selected) {
		return lineageNotAssociated
	}
	return lineageExact
}

func (s *workbenchServer) handleWorkloadPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLineageCapability(w, r, authz.WorkloadView, authz.ProposalView) {
		return
	}
	sel, why := parseLineageSelector(r)
	if why != "" {
		writeWorkbenchClientError(w, http.StatusBadRequest, why)
		return
	}
	if r.URL.Query().Get("namespace") != s.reads.SessionIdentity().Namespace {
		writeWorkbenchClientError(w, http.StatusConflict, "namespace is not the authoritative session namespace")
		return
	}
	if s.clusterIdentity == "" {
		writeWorkbenchClientError(w, http.StatusConflict, "authoritative ClusterIdentity is unavailable")
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
		writeWorkbenchClientError(w, http.StatusNotFound, "no discovered workload matches the requested identity")
		return
	}
	cluster := s.clusterIdentity
	identity := observationIdentity{ClusterIdentity: cluster, Namespace: r.URL.Query().Get("namespace"), Group: sel.group, Kind: sel.kind, WorkloadName: sel.name, WorkloadUID: sel.workloadUID, Container: sel.container}
	obs, obsComplete, err := exactObservationIdentities(ctx, s.reads)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	continueToken := r.URL.Query().Get("continue")
	var proposals *unstructured.UnstructuredList
	if continueToken == "" {
		proposals, err = s.reads.ListProposals(ctx)
	} else if pager, ok := s.reads.(lineagePager); ok {
		proposals, err = pager.ListProposalsPage(ctx, continueToken)
	} else {
		writeWorkbenchClientError(w, http.StatusBadRequest, "continuation is not supported by this read session")
		return
	}
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	out := make([]proposalRead, 0)
	diag := lineageDiagnostics{}
	for i := range proposals.Items {
		p, err := proposalProjection(&proposals.Items[i])
		if err != nil {
			diag.ExcludedMalformed++
			continue
		}
		switch classifyProposalLineage(p, obs, identity) {
		case lineageInsufficient:
			diag.ExcludedInsufficientProvenance++
		case lineageMixed:
			diag.ExcludedMixedIdentity++
		case lineageNotAssociated:
			diag.ExcludedNotAssociated++
		case lineageExact:
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeWorkbenchJSON(w, http.StatusOK, workloadPolicyRead{Identity: identity, Proposals: out, Complete: obsComplete && proposals.GetContinue() == "", Continue: proposals.GetContinue(), Diagnostics: diag})
}

func decodeLineageApply(obj *unstructured.Unstructured) (lineageAttemptRead, error) {
	var spec attempt.Spec
	var status attempt.Status
	if err := fromNested(obj, "spec", &spec); err != nil {
		return lineageAttemptRead{}, err
	}
	if err := fromNested(obj, "status", &status); err != nil {
		return lineageAttemptRead{}, err
	}
	return lineageAttemptRead{Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), ResourceVersion: obj.GetResourceVersion(), ProposalNamespace: spec.ProposalNamespace, ProposalName: spec.ProposalName, ProposalUID: spec.ProposalUID, ApprovedCandidateDigest: spec.ApprovedCandidateDigest, Target: targetText(spec.Target), Operator: spec.OperatorIdentity, CustodyEpoch: epochText(spec.CustodyEpoch), State: status.State, StartedAt: spec.StartedAt, UpdatedAt: status.UpdatedAt, CompletedAt: status.CompletedAt, Failure: status.Failure, Mutations: status.Mutations}, nil
}

func decodeLineageRollback(obj *unstructured.Unstructured) (lineageAttemptRead, error) {
	var spec attempt.RollbackSpec
	var status attempt.Status
	if err := fromNested(obj, "spec", &spec); err != nil {
		return lineageAttemptRead{}, err
	}
	if err := fromNested(obj, "status", &status); err != nil {
		return lineageAttemptRead{}, err
	}
	return lineageAttemptRead{Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), ResourceVersion: obj.GetResourceVersion(), ProposalNamespace: spec.ProposalNamespace, ProposalName: spec.ProposalName, ProposalUID: spec.ProposalUID, SourceNamespace: spec.SourceNamespace, SourceName: spec.SourceName, SourceUID: spec.SourceUID, ApprovedCandidateDigest: spec.ApprovedCandidateDigest, Target: targetText(spec.Target), Operator: spec.OperatorIdentity, CustodyEpoch: epochText(spec.CustodyEpoch), State: status.State, StartedAt: spec.StartedAt, UpdatedAt: status.UpdatedAt, CompletedAt: status.CompletedAt, Failure: status.Failure, Mutations: status.Mutations}, nil
}

func (s *workbenchServer) handleLineageAttempts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", 405)
		return
	}
	if !s.requireLineageCapability(w, r, authz.ProposalView, authz.HistoryView) {
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/proposals/")
	name = strings.TrimSuffix(name, "/attempts")
	uid := r.URL.Query().Get("proposalUID")
	if name == "" || uid == "" {
		writeWorkbenchClientError(w, 400, "proposal name and proposalUID are required")
		return
	}
	release, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()
	p, err := s.reads.GetProposal(ctx, name)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	if string(p.GetUID()) != uid {
		writeWorkbenchClientError(w, http.StatusConflict, "proposal UID does not match the current Proposal")
		return
	}
	list, err := s.reads.ListApplyAttempts(ctx)
	if token := r.URL.Query().Get("continue"); token != "" {
		if pager, ok := s.reads.(lineagePager); ok {
			list, err = pager.ListApplyAttemptsPage(ctx, token)
		} else {
			writeWorkbenchClientError(w, http.StatusBadRequest, "continuation is not supported by this read session")
			return
		}
	}
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	items := make([]lineageAttemptRead, 0)
	for i := range list.Items {
		a, e := decodeLineageApply(&list.Items[i])
		if e != nil {
			continue
		}
		if a.ProposalName == name && a.ProposalUID == uid {
			items = append(items, a)
		}
	}
	writeWorkbenchJSON(w, 200, lineageResponse{Items: items, Continue: list.GetContinue(), Complete: list.GetContinue() == ""})
}

func (s *workbenchServer) handleLineageAttemptRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", 405)
		return
	}
	if !s.requireLineageCapability(w, r, authz.HistoryView) {
		return
	}
	release, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if strings.HasPrefix(path, "apply-attempts/") {
		tail := strings.TrimPrefix(path, "apply-attempts/")
		if strings.HasSuffix(tail, "/rollbacks") {
			name := strings.TrimSuffix(tail, "/rollbacks")
			uid := r.URL.Query().Get("attemptUID")
			if name == "" || uid == "" {
				writeWorkbenchClientError(w, 400, "attempt name and attemptUID are required")
				return
			}
			obj, err := s.reads.GetApplyAttempt(ctx, name)
			if err != nil {
				writeWorkbenchTransportError(w, err)
				return
			}
			if string(obj.GetUID()) != uid {
				writeWorkbenchClientError(w, 409, "ApplyAttempt UID does not match the current attempt")
				return
			}
			list, err := s.reads.ListRollbackAttempts(ctx)
			if token := r.URL.Query().Get("continue"); token != "" {
				if pager, ok := s.reads.(lineagePager); ok {
					list, err = pager.ListRollbackAttemptsPage(ctx, token)
				} else {
					writeWorkbenchClientError(w, http.StatusBadRequest, "continuation is not supported by this read session")
					return
				}
			}
			if err != nil {
				writeWorkbenchTransportError(w, err)
				return
			}
			out := make([]lineageAttemptRead, 0)
			for i := range list.Items {
				a, e := decodeLineageRollback(&list.Items[i])
				if e == nil && a.SourceUID == uid {
					out = append(out, a)
				}
			}
			writeWorkbenchJSON(w, 200, lineageResponse{Items: out, Continue: list.GetContinue(), Complete: list.GetContinue() == ""})
			return
		}
		name := strings.TrimSuffix(tail, "/")
		uid := r.URL.Query().Get("attemptUID")
		if name == "" || uid == "" {
			writeWorkbenchClientError(w, 400, "attempt name and attemptUID are required")
			return
		}
		obj, err := s.reads.GetApplyAttempt(ctx, name)
		if err != nil {
			writeWorkbenchTransportError(w, err)
			return
		}
		if string(obj.GetUID()) != uid {
			writeWorkbenchClientError(w, 409, "ApplyAttempt UID does not match the current attempt")
			return
		}
		out, err := decodeLineageApply(obj)
		if err != nil {
			writeWorkbenchJSON(w, 422, map[string]string{"state": "MALFORMED_OBJECT"})
			return
		}
		writeWorkbenchJSON(w, 200, out)
		return
	}
	if strings.HasPrefix(path, "rollback-attempts/") {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "rollback-attempts/"), "/")
		uid := r.URL.Query().Get("attemptUID")
		if name == "" || uid == "" {
			writeWorkbenchClientError(w, 400, "attempt name and attemptUID are required")
			return
		}
		obj, err := s.reads.GetRollbackAttempt(ctx, name)
		if err != nil {
			writeWorkbenchTransportError(w, err)
			return
		}
		if string(obj.GetUID()) != uid {
			writeWorkbenchClientError(w, 409, "RollbackAttempt UID does not match the current attempt")
			return
		}
		out, err := decodeLineageRollback(obj)
		if err != nil {
			writeWorkbenchJSON(w, 422, map[string]string{"state": "MALFORMED_OBJECT"})
			return
		}
		writeWorkbenchJSON(w, 200, out)
		return
	}
	http.NotFound(w, r)
}
