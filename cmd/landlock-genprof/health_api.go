package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
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
	projectionStarted := time.Now()
	defer func() {
		if stats := observability.RequestStatsFromContext(r.Context()); stats != nil {
			stats.SetProjectionDuration(time.Since(projectionStarted))
		}
	}()
	items := make([]sphm.Observation, 0, len(observations.Items))
	// Execution is intentionally projected as a domain value. The helper below
	// handles its stable JSON representation without granting the browser any
	// Kubernetes authority.
	malformedObservations := 0
	for i := range observations.Items {
		o, e := observationProjection(&observations.Items[i])
		if e != nil {
			malformedObservations++
			continue
		}
		state := observationStateString(o.Execution)
		evidence := observationEvidenceVerdict(o.Sources)
		items = append(items, sphm.Observation{ID: o.ID, Workload: o.Identity.Namespace + "/" + o.Identity.WorkloadName, State: state, Evidence: evidence, Frozen: o.Frozen, Failed: state == "FAILED"})
	}
	ps := make([]sphm.Proposal, 0, len(proposals.Items))
	malformedProposals := 0
	for i := range proposals.Items {
		if p, e := proposalProjection(&proposals.Items[i]); e == nil {
			workload := ""
			if p.Subject != nil {
				workload = p.Subject.Target
			}
			ps = append(ps, sphm.Proposal{Name: p.Name, Workload: workload, Status: string(p.Status.ApprovalState)})
		} else {
			malformedProposals++
		}
	}
	identity := s.reads.SessionIdentity()
	ctxVersion := ""
	if rctx, err := s.requestContextForHealth(r); err == nil {
		ctxVersion = rctx
	}
	report := sphm.Evaluate(time.Now().UTC(), sphm.Context{ClusterIdentity: s.clusterIdentity, Namespace: identity.Namespace, ContextVersion: ctxVersion}, items, ps, sphm.Exclusions{MalformedObservations: malformedObservations, MalformedProposals: malformedProposals})
	writeWorkbenchJSON(w, http.StatusOK, report)
}

// observationEvidenceVerdict derives a single per-observation evidence
// verdict from all of an Observation's sources, for SPHM's counting
// purposes. M10.4: previously this was o.Sources[0].EvidenceState, an
// arbitrary collapse to whichever source's name sorted alphabetically
// first (per NewObservationResult's deterministic-by-name ordering) --
// meaning a genuinely UNKNOWN source could be masked by a co-existing
// AVAILABLE source purely because of source-name ordering. The canonical
// domain model (internal/observation/domain) defines no aggregate evidence
// state, so this is a conservative, explicitly-scoped derivation for this
// one consumer, not a new domain concept: UNKNOWN in any source always
// dominates (missing qualification proof is never masked by a co-existing
// AVAILABLE source), AVAILABLE dominates over EMPTY (real attributable
// evidence from any source is not hidden by another source finding
// nothing), and EMPTY only when every source is EMPTY.
func observationEvidenceVerdict(sources []observationSourceRead) string {
	if len(sources) == 0 {
		return "UNKNOWN"
	}
	sawAvailable := false
	for _, s := range sources {
		switch s.EvidenceState {
		case "UNKNOWN":
			return "UNKNOWN"
		case "AVAILABLE":
			sawAvailable = true
		}
	}
	if sawAvailable {
		return "AVAILABLE"
	}
	return "EMPTY"
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
