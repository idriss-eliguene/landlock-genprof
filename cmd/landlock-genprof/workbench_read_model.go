package main

// The read model is deliberately a projection over the existing durable CRDs.
// It contains no client-side authority and no write-capable Kubernetes client.

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	obsdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const workbenchReadModelLimit = 100

type readModelSelector struct{ group, kind, name, container, workloadUID, imageIdentity string }

func parseReadModelSelector(q map[string][]string) (readModelSelector, string) {
	if len(q) > workbenchMaxQueryParams {
		return readModelSelector{}, "too many query parameters"
	}
	var s readModelSelector
	for k, v := range q {
		if len(v) != 1 {
			return s, "each query parameter must have one value"
		}
		if len(v[0]) == 0 || len(v[0]) > workbenchMaxIdentifierLength {
			return s, "invalid query parameter value"
		}
		switch k {
		case "group":
			s.group = v[0]
		case "kind":
			s.kind = v[0]
		case "name":
			s.name = v[0]
		case "container":
			s.container = v[0]
		case "workloadUID":
			s.workloadUID = v[0]
		case "imageIdentity":
			s.imageIdentity = v[0]
		default:
			return s, fmt.Sprintf("unsupported query parameter %q", k)
		}
	}
	if s.kind == "" || s.name == "" || s.container == "" || s.workloadUID == "" {
		return s, "kind, name, container, and workloadUID are required"
	}
	if !workbenchKindPattern.MatchString(s.kind) || !workbenchContainerPattern.MatchString(s.container) || !workbenchIdentifierPattern.MatchString(s.name) {
		return s, "invalid workload selector"
	}
	return s, ""
}

type observationRead struct {
	ID           string                  `json:"observationID"`
	Identity     observationIdentity     `json:"identity"`
	Spec         observationSpecRead     `json:"spec"`
	Execution    any                     `json:"execution"`
	Sources      []observationSourceRead `json:"sources"`
	Frozen       bool                    `json:"frozen"`
	StopEligible bool                    `json:"stopEligible"`
	CreatedAt    string                  `json:"createdAt,omitempty"`
	UpdatedAt    string                  `json:"updatedAt,omitempty"`
}
type observationIdentity struct {
	ClusterIdentity string `json:"clusterIdentity"`
	Namespace       string `json:"namespace"`
	Group           string `json:"group"`
	Kind            string `json:"kind"`
	WorkloadName    string `json:"workloadName"`
	WorkloadUID     string `json:"workloadUID"`
	Container       string `json:"container"`
	ImageIdentity   string `json:"imageIdentity,omitempty"`
}
type observationSpecRead struct {
	Sources          []string `json:"sources"`
	Duration         string   `json:"duration"`
	RequesterSession string   `json:"requesterSession,omitempty"`
}
type observationSourceRead struct {
	Name                         string   `json:"name"`
	AttributionState             string   `json:"attributionState"`
	EvidenceState                string   `json:"evidenceState"`
	AttributedCount              uint64   `json:"attributedCount"`
	ExcludedCount                uint64   `json:"excludedCount"`
	BackendHealthConfirmed       bool     `json:"backendHealthConfirmed"`
	SourceAttachedForBoundWindow bool     `json:"sourceAttachedForBoundWindow"`
	FlushConfirmed               bool     `json:"flushConfirmed"`
	Facts                        any      `json:"facts,omitempty"`
	References                   []string `json:"references,omitempty"`
}

type proposalRead struct {
	Name                string                             `json:"name"`
	UID                 string                             `json:"uid,omitempty"`
	ResourceVersion     string                             `json:"resourceVersion,omitempty"`
	CandidateVersion    string                             `json:"candidateVersion"`
	Subject             *proposal.SubjectV2                `json:"subject,omitempty"`
	Artifact            *proposal.ArtifactV2               `json:"artifact,omitempty"`
	CandidateDigest     string                             `json:"candidateDigest,omitempty"`
	ReviewContextDigest string                             `json:"reviewContextDigest,omitempty"`
	Provenance          *proposal.ProposalProvenance       `json:"provenance,omitempty"`
	Qualification       *proposal.ProposalQualification    `json:"qualification,omitempty"`
	DerivationStatus    *proposal.ProposalDerivationStatus `json:"derivationStatus,omitempty"`
	CandidateYAML       string                             `json:"candidateYAML,omitempty"`
	Status              proposal.Status                    `json:"status"`
	CurrentAuthority    string                             `json:"currentAuthority,omitempty"`
	CreationTimestamp   string                             `json:"creationTimestamp,omitempty"`
}

func observationIdentityOf(o obsdomain.Observation) observationIdentity {
	slot := o.Spec().Target.Slot
	w := slot.Workload
	result := observationIdentity{ClusterIdentity: w.Cluster.NamespaceUID, Namespace: w.Namespace, Group: w.GroupKind.Group, Kind: w.GroupKind.Kind, WorkloadName: w.Name, WorkloadUID: w.UID, Container: slot.Container}
	binding := o.Binding()
	for _, rev := range binding.ImageRevisionValues() {
		if rev.Slot == slot {
			result.ImageIdentity = rev.ImageDigest
			break
		}
	}
	if result.ImageIdentity == "" {
		for _, target := range binding.ResolvedTargets.Items() {
			if target.Slot == slot && target.ImageRevision != nil {
				result.ImageIdentity = target.ImageRevision.ImageDigest
				break
			}
		}
	}
	return result
}
func observationProjection(obj *unstructured.Unstructured) (observationRead, error) {
	o, err := obskube.FromUnstructured(obj)
	if err != nil {
		return observationRead{}, err
	}
	s := o.Spec()
	p := observationRead{ID: string(o.ID()), Identity: observationIdentityOf(o), Spec: observationSpecRead{Sources: s.SourceNames(), Duration: s.Duration.String(), RequesterSession: s.RequesterSession}, Execution: o.Execution(), Frozen: o.Frozen(), StopEligible: o.CanRequestStop(), CreatedAt: obj.GetCreationTimestamp().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), UpdatedAt: obj.GetAnnotations()["landlockgenprof.io/updated-at"]}
	for _, src := range o.Result().Sources() {
		p.Sources = append(p.Sources, observationSourceRead{
			Name:                         src.Source.Name,
			AttributionState:             string(src.Qualification.Attribution),
			EvidenceState:                string(src.Evidence),
			AttributedCount:              src.Qualification.AttributedCount,
			ExcludedCount:                src.Qualification.ExcludedCount,
			BackendHealthConfirmed:       src.Qualification.BackendHealthConfirmed,
			SourceAttachedForBoundWindow: src.Qualification.SourceAttachedForBoundWindow,
			FlushConfirmed:               src.Qualification.FlushConfirmed,
			Facts:                        src.Facts,
			References:                   src.References,
		})
	}
	return p, nil
}
func selectorMatches(i observationIdentity, s readModelSelector) bool {
	return i.Group == s.group && i.Kind == s.kind && i.WorkloadName == s.name && i.WorkloadUID == s.workloadUID && i.Container == s.container && (s.imageIdentity == "" || i.ImageIdentity == s.imageIdentity)
}

func proposalMatches(p proposalRead, s readModelSelector) bool {
	if p.Subject == nil || p.Subject.Scope != proposal.CandidateV2ScopeContainer || p.Subject.Container != s.container || p.Subject.ImageIdentity != s.imageIdentity && s.imageIdentity != "" {
		return false
	}
	parts := strings.SplitN(p.Subject.Target, "/", 2)
	return len(parts) == 2 && parts[0] == s.kind && parts[1] == s.name
}

func proposalProjection(obj *unstructured.Unstructured) (proposalRead, error) {
	m, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return proposalRead{}, fmt.Errorf("proposal %s has invalid spec", obj.GetName())
	}
	var spec proposal.Spec
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(m, &spec); err != nil {
		return proposalRead{}, err
	}
	if err = proposal.ValidateProposalSpec(spec); err != nil {
		return proposalRead{}, err
	}
	status := proposal.Status{ApprovalState: proposal.ApprovalDraft}
	if sm, ok, _ := unstructured.NestedMap(obj.Object, "status"); ok {
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(sm, &status); err != nil {
			return proposalRead{}, err
		}
		if status.ApprovalState == "" {
			status.ApprovalState = proposal.ApprovalDraft
		}
	}
	v, err := spec.NormalizedCandidateVersion()
	if err != nil {
		return proposalRead{}, err
	}
	out := proposalRead{Name: obj.GetName(), UID: string(obj.GetUID()), ResourceVersion: obj.GetResourceVersion(), CandidateVersion: v, Status: status, CreationTimestamp: obj.GetCreationTimestamp().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")}
	if v == proposal.CandidateVersionV2 {
		c, e := spec.CandidateV2()
		if e != nil {
			return out, e
		}
		out.Subject = &c.Subject
		out.Artifact = &c.Artifact
		out.CandidateDigest, err = proposal.CandidateDigestV2(c)
		if err != nil {
			return out, err
		}
		rc, e := spec.ReviewContextV2()
		if e != nil {
			return out, e
		}
		out.Provenance = spec.Provenance
		out.Qualification = spec.Qualification
		out.DerivationStatus = spec.DerivationStatus
		out.ReviewContextDigest, err = proposal.ReviewContextDigestV2(rc)
		if err != nil {
			return out, err
		}
		candidateYAML, e := yaml.Marshal(c)
		if e != nil {
			return out, e
		}
		out.CandidateYAML = string(candidateYAML)
	} else {
		out.CandidateDigest, err = proposal.CandidateDigest(spec)
		if err != nil {
			return out, err
		}
	}
	if status.ApprovalState != proposal.ApprovalApproved {
		out.CurrentAuthority = "NOT_APPROVED"
	} else if err = proposal.ValidateApprovedCandidate(&spec, &status); err != nil {
		out.CurrentAuthority = "STALE"
	} else {
		out.CurrentAuthority = "VALID"
	}
	return out, nil
}

func (s *workbenchServer) handleObservationReadModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", 405)
		return
	}
	rel, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer rel()
	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()
	if r.URL.Path == "/api/observations" {
		sel, why := parseReadModelSelector(r.URL.Query())
		if why != "" {
			writeWorkbenchClientError(w, 400, why)
			return
		}
		list, err := s.reads.ListObservations(ctx)
		if err != nil {
			writeWorkbenchTransportError(w, err)
			return
		}
		out := make([]observationRead, 0)
		diagnostics := projectionDiagnostics{}
		for _, obj := range list.Items {
			p, e := observationProjection(&obj)
			if e != nil {
				diagnostics.addMalformed("Observation", "excluded from Observation read model", metadataOf(&obj), e)
				continue
			}
			diagnostics.addValid()
			if selectorMatches(p.Identity, sel) {
				out = append(out, p)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].CreatedAt != out[j].CreatedAt {
				return out[i].CreatedAt < out[j].CreatedAt
			}
			return out[i].ID < out[j].ID
		})
		if len(out) > workbenchReadModelLimit {
			out = out[:workbenchReadModelLimit]
		}
		diagnostics.finalize()
		writeWorkbenchJSON(w, 200, observationListResponse{Items: out, Limit: workbenchReadModelLimit, ProjectionDiagnostics: diagnostics})
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/observations/")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	obj, err := s.reads.GetObservation(ctx, name)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	p, err := observationProjection(obj)
	if err != nil {
		writeWorkbenchJSON(w, http.StatusUnprocessableEntity, malformedObjectResponse{State: "MALFORMED_OBJECT", Diagnostic: diagnosticForObject("Observation", obj, "excluded from Observation read model", err)})
		return
	}
	writeWorkbenchJSON(w, 200, p)
}

func (s *workbenchServer) handleProposalReadModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "read-only Workbench: GET only", 405)
		return
	}
	rel, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer rel()
	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()
	if r.URL.Path == "/api/proposals" {
		sel, why := parseReadModelSelector(r.URL.Query())
		if why != "" {
			writeWorkbenchClientError(w, 400, why)
			return
		}
		list, err := s.reads.ListProposals(ctx)
		if err != nil {
			writeWorkbenchTransportError(w, err)
			return
		}
		out := make([]proposalRead, 0)
		diagnostics := projectionDiagnostics{}
		for _, obj := range list.Items {
			p, e := proposalProjection(&obj)
			if e != nil {
				diagnostics.addMalformed("SecurityProfileProposal", "excluded from Proposal read model", metadataOf(&obj), e)
				continue
			}
			diagnostics.addValid()
			if proposalMatches(p, sel) {
				out = append(out, p)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].CreationTimestamp != out[j].CreationTimestamp {
				return out[i].CreationTimestamp < out[j].CreationTimestamp
			}
			return out[i].Name < out[j].Name
		})
		if len(out) > workbenchReadModelLimit {
			out = out[:workbenchReadModelLimit]
		}
		diagnostics.finalize()
		writeWorkbenchJSON(w, 200, proposalListResponse{Items: out, Limit: workbenchReadModelLimit, ProjectionDiagnostics: diagnostics})
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/proposals/")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	obj, err := s.reads.GetProposal(ctx, name)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	p, err := proposalProjection(obj)
	if err != nil {
		writeWorkbenchJSON(w, http.StatusUnprocessableEntity, malformedObjectResponse{State: "MALFORMED_OBJECT", Diagnostic: diagnosticForObject("SecurityProfileProposal", obj, "excluded from Proposal read model", err)})
		return
	}
	writeWorkbenchJSON(w, 200, p)
}

type observationListResponse struct {
	Items []observationRead `json:"items"`
	Limit int               `json:"limit"`
	ProjectionDiagnostics
}

type proposalListResponse struct {
	Items []proposalRead `json:"items"`
	Limit int            `json:"limit"`
	ProjectionDiagnostics
}
