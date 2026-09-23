// Package sphm defines the Security Profile Health Model used by the
// Operations Center. It intentionally separates missing proof (UNKNOWN) from
// missing product observability (NOT_ESTABLISHED).
package sphm

import "time"

type State string

const (
	Healthy        State = "HEALTHY"
	AttentionState State = "ATTENTION"
	Critical       State = "CRITICAL"
	Unknown        State = "UNKNOWN"
	NotEstablished State = "NOT_ESTABLISHED"
	NotApplicable  State = "NOT_APPLICABLE"
)

type Dimension struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	State       State     `json:"state"`
	Value       any       `json:"value,omitempty"`
	Unit        string    `json:"unit,omitempty"`
	Reason      string    `json:"reason"`
	Source      string    `json:"authoritativeSource"`
	Drilldown   string    `json:"drilldown,omitempty"`
	EvaluatedAt time.Time `json:"evaluatedAt"`
}

type Attention struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Reason        string `json:"reason"`
	ObservationID string `json:"observationID,omitempty"`
	ProposalName  string `json:"proposalName,omitempty"`
	Workload      string `json:"workload,omitempty"`
}

type Report struct {
	ModelVersion string      `json:"modelVersion"`
	Overall      Dimension   `json:"overall"`
	Dimensions   []Dimension `json:"dimensions"`
	Attention    []Attention `json:"attention"`
	Context      Context     `json:"context"`
}

type Context struct {
	ClusterIdentity string `json:"clusterIdentity"`
	Namespace       string `json:"namespace"`
	ContextVersion  string `json:"contextVersion,omitempty"`
}

type Observation struct {
	ID, Workload, State, Evidence string
	Frozen, Failed                bool
}

type Proposal struct {
	Name, Workload, Status string
}

// Exclusions reports objects that could not be projected and were therefore
// excluded before reaching Evaluate. M10.4: previously these vanished
// silently (dropped by the caller's loop, never counted, never surfaced
// anywhere in the Report). Per the invariant that malformed resources must
// not improve apparent health merely by disappearing from the denominator,
// their count is now a required input and is surfaced as an explicit
// Attention diagnostic whenever non-zero -- not invented as a new State,
// reusing the existing Attention mechanism.
type Exclusions struct {
	MalformedObservations int
	MalformedProposals    int
}

func Evaluate(now time.Time, ctx Context, observations []Observation, proposals []Proposal, excluded Exclusions) Report {
	unknownEvidence := 0
	failed := 0
	for _, o := range observations {
		if o.State == "COMPLETED" && o.Frozen && o.Evidence == "UNKNOWN" {
			unknownEvidence++
		}
		if o.Failed || o.State == "FAILED" {
			failed++
		}
	}
	pending := 0
	for _, p := range proposals {
		if p.Status == "DRAFT" || p.Status == "REVIEWED" {
			pending++
		}
	}
	dims := []Dimension{
		{ID: "authority", Name: "Authority", State: Healthy, Reason: "A request reached this handler only after passing namespace-scoped read authorization for the bound EnvironmentSession. This dimension does not independently monitor governance, apply, or rollback capability health.", Source: "EnvironmentSession / authenticated read session", Drilldown: "/api/v08/environment", EvaluatedAt: now},
		{ID: "coverage", Name: "Coverage", State: NotEstablished, Reason: "No authoritative eligible-workload denominator is defined by the current product.", Source: "Not established by current domain model", Drilldown: "workloads", EvaluatedAt: now},
		{ID: "evidence", Name: "Evidence qualification", State: Healthy, Value: len(observations), Unit: "observations", Reason: "Completed observations were evaluated from persisted evidence qualification.", Source: "Observation read model", Drilldown: "observations", EvaluatedAt: now},
		{ID: "freshness", Name: "Freshness", State: NotEstablished, Reason: "No product policy defines a freshness threshold for profiles or observations.", Source: "No authoritative threshold", Drilldown: "observations", EvaluatedAt: now},
		{ID: "drift", Name: "Drift", State: NotEstablished, Reason: "The current product cannot prove governed-state versus enforced-state drift.", Source: "No enforcement read model", Drilldown: "proposals", EvaluatedAt: now},
		{ID: "governance", Name: "Governance", State: Healthy, Value: pending, Unit: "pending proposals", Reason: "Governance state is read from authoritative proposal status.", Source: "SecurityProfileProposal read model", Drilldown: "proposals", EvaluatedAt: now},
		{ID: "pipeline", Name: "Observation pipeline", State: Healthy, Value: len(observations), Unit: "observations", Reason: "Observation lifecycle outcomes are read from the Observation read model.", Source: "Observation read model", Drilldown: "observations", EvaluatedAt: now},
		{ID: "enforcement", Name: "Enforcement visibility", State: NotEstablished, Reason: "Operations Center has no authoritative enforcement-state signal.", Source: "No enforcement read model", Drilldown: "proposals", EvaluatedAt: now},
	}
	// M10.4: absence of observations is not evidence of a healthy pipeline
	// (ADR-0035: "missing data never becomes HEALTHY; it hides uncertainty
	// and authority failure"). Zero observations means pipeline health has
	// not been established, not that it is confirmed healthy.
	if len(observations) == 0 {
		dims[6].State = NotEstablished
		dims[6].Reason = "No observations exist to evaluate pipeline health. Absence of data is not evidence of a healthy pipeline."
	}
	attention := []Attention{}
	if unknownEvidence > 0 {
		dims[2].State = Unknown
		dims[2].Value = unknownEvidence
		dims[2].Reason = "Terminal UNKNOWN evidence exists; missing qualification proof is not equivalent to processing or failure."
	}
	if failed > 0 {
		dims[6].State = AttentionState
		dims[6].Value = failed
		dims[6].Reason = "Failed observations require operator investigation."
	}
	if pending > 0 {
		dims[5].State = AttentionState
		dims[5].Reason = "Pending proposals require governance review."
	}
	for _, o := range observations {
		if o.State == "COMPLETED" && o.Frozen && o.Evidence == "UNKNOWN" {
			attention = append(attention, Attention{ID: "evidence-" + o.ID, Kind: "EVIDENCE_UNKNOWN", Title: "Evidence qualification inconclusive", Reason: "The completed observation does not establish all evidence proof conditions.", ObservationID: o.ID, Workload: o.Workload})
		}
		if o.Failed || o.State == "FAILED" {
			attention = append(attention, Attention{ID: "observation-" + o.ID, Kind: "OBSERVATION_FAILED", Title: "Observation failed", Reason: "Inspect the authoritative failure stage and reason.", ObservationID: o.ID, Workload: o.Workload})
		}
	}
	for _, p := range proposals {
		if p.Status == "DRAFT" || p.Status == "REVIEWED" {
			attention = append(attention, Attention{ID: "proposal-" + p.Name, Kind: "GOVERNANCE_PENDING", Title: "Proposal requires governance action", Reason: "Review the exact candidate and its evidence provenance.", ProposalName: p.Name, Workload: p.Workload})
		}
	}
	// M10.4: malformed objects must not silently vanish from the
	// denominator -- surface their exclusion as an explicit diagnostic.
	if excluded.MalformedObservations > 0 {
		attention = append(attention, Attention{ID: "excluded-observations", Kind: "MALFORMED_OBSERVATIONS_EXCLUDED", Title: "Malformed observations excluded from this report", Reason: "One or more Observation objects could not be projected and were excluded from evidence and pipeline evaluation. They are not counted as healthy, failed, or unknown."})
	}
	if excluded.MalformedProposals > 0 {
		attention = append(attention, Attention{ID: "excluded-proposals", Kind: "MALFORMED_PROPOSALS_EXCLUDED", Title: "Malformed proposals excluded from this report", Reason: "One or more Proposal objects could not be projected and were excluded from governance evaluation. They are not counted as pending, approved, or applied."})
	}
	overall := Dimension{ID: "overall", Name: "Security Profile Health", State: Healthy, Reason: "No actionable condition was found in the established dimensions.", Source: "SPHM v1 deterministic aggregation", EvaluatedAt: now}
	if unknownEvidence > 0 {
		overall.State = Unknown
		overall.Reason = "Evidence qualification is inconclusive for one or more completed observations."
	}
	if failed > 0 || pending > 0 {
		if overall.State == Healthy {
			overall.State = AttentionState
		}
		overall.Reason = "One or more operational conditions require investigation."
	}
	return Report{ModelVersion: "SPHM-v1", Overall: overall, Dimensions: dims, Attention: attention, Context: ctx}
}
