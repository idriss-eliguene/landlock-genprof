package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/sphm"
)

func (s *workbenchServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Health is read-only", http.StatusMethodNotAllowed)
		return
	}
	release, ok := s.workbenchAcquireRead(w)
	if !ok {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), workbenchClusterReadDeadline)
	defer cancel()
	observations, err := s.reads.ListObservations(ctx)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	proposals, err := s.reads.ListProposals(ctx)
	if err != nil {
		writeWorkbenchTransportError(w, err)
		return
	}
	items := make([]sphm.Observation, 0, len(observations.Items))
	// Execution is intentionally projected as a domain value. The helper below
	// handles its stable JSON representation without granting the browser any
	// Kubernetes authority.
	for i := range observations.Items {
		o, e := observationProjection(&observations.Items[i])
		if e != nil {
			continue
		}
		state := observationStateString(o.Execution)
		evidence := "UNKNOWN"
		if len(o.Sources) > 0 {
			evidence = o.Sources[0].EvidenceState
		}
		items = append(items, sphm.Observation{ID: o.ID, Workload: o.Identity.Namespace + "/" + o.Identity.WorkloadName, State: state, Evidence: evidence, Frozen: o.Frozen, Failed: state == "FAILED"})
	}
	ps := make([]sphm.Proposal, 0, len(proposals.Items))
	for i := range proposals.Items {
		if p, e := proposalProjection(&proposals.Items[i]); e == nil {
			workload := ""
			if p.Subject != nil {
				workload = p.Subject.Target
			}
			ps = append(ps, sphm.Proposal{Name: p.Name, Workload: workload, Status: string(p.Status.ApprovalState)})
		}
	}
	identity := s.reads.SessionIdentity()
	ctxVersion := ""
	if rctx, err := s.requestContextForHealth(r); err == nil {
		ctxVersion = rctx
	}
	report := sphm.Evaluate(time.Now().UTC(), sphm.Context{ClusterIdentity: s.clusterIdentity, Namespace: identity.Namespace, ContextVersion: ctxVersion}, items, ps)
	writeWorkbenchJSON(w, http.StatusOK, report)
}

func observationStateString(v any) string {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m["State"].(string); ok {
			return s
		}
		if s, ok := m["state"].(string); ok {
			return s
		}
	}
	b, _ := json.Marshal(v)
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		if s, ok := m["State"].(string); ok {
			return s
		}
		if s, ok := m["state"].(string); ok {
			return s
		}
	}
	return "UNKNOWN"
}

func (s *workbenchServer) requestContextForHealth(r *http.Request) (string, error) {
	return r.Header.Get("X-Environment-Context-Version"), nil
}
