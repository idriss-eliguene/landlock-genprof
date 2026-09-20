package sphm

import (
	"testing"
	"time"
)

func TestEvaluateKeepsTerminalUnknownDistinctFromFailure(t *testing.T) {
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, []Observation{{ID: "obs-1", Workload: "payments/api", State: "COMPLETED", Frozen: true, Evidence: "UNKNOWN"}}, nil, Exclusions{})
	if r.Overall.State != Unknown {
		t.Fatalf("overall=%s, want UNKNOWN", r.Overall.State)
	}
	if r.Dimensions[2].State != Unknown {
		t.Fatalf("evidence=%s, want UNKNOWN", r.Dimensions[2].State)
	}
	if r.Attention[0].ObservationID != "obs-1" {
		t.Fatalf("drilldown=%+v", r.Attention[0])
	}
	if r.Dimensions[1].State != NotEstablished || r.Dimensions[7].State != NotEstablished {
		t.Fatal("unsupported dimensions must remain NOT_ESTABLISHED")
	}
}

func TestEvaluateUsesExplicitOperationalAttentionWithoutWeightedScore(t *testing.T) {
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, []Observation{{ID: "failed", Workload: "payments/api", State: "FAILED", Failed: true}}, []Proposal{{Name: "p", Workload: "Deployment/api", Status: "DRAFT"}}, Exclusions{})
	if r.Overall.State != AttentionState {
		t.Fatalf("overall=%s, want ATTENTION", r.Overall.State)
	}
	if r.Dimensions[5].State != AttentionState || r.Dimensions[6].State != AttentionState {
		t.Fatal("governance and pipeline attention not established")
	}
	if len(r.Attention) != 2 {
		t.Fatalf("attention=%d, want two exact drilldowns", len(r.Attention))
	}
}

func TestEvaluateEmptyPopulationDoesNotBecomeHealthyCoverage(t *testing.T) {
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, nil, nil, Exclusions{})
	if r.Dimensions[1].State != NotEstablished {
		t.Fatalf("coverage=%s, want NOT_ESTABLISHED", r.Dimensions[1].State)
	}
	if r.Dimensions[4].State != NotEstablished || r.Dimensions[7].State != NotEstablished {
		t.Fatal("unsupported health dimensions were marked healthy")
	}
	// M10.4: the dimension this test is actually named for. Zero observations
	// must not be read as a healthy pipeline -- absence of data is not
	// evidence of a healthy pipeline (ADR-0035).
	if r.Dimensions[6].State != NotEstablished {
		t.Fatalf("pipeline=%s, want NOT_ESTABLISHED for zero observations", r.Dimensions[6].State)
	}
	if r.Overall.State != Healthy {
		t.Fatalf("overall=%s, want HEALTHY (no failed/pending/unknown conditions exist to evaluate)", r.Overall.State)
	}
}

func TestEvaluateNonEmptyPopulationWithNoFailuresEstablishesPipelineHealthy(t *testing.T) {
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, []Observation{{ID: "obs-1", Workload: "payments/api", State: "COMPLETED"}}, nil, Exclusions{})
	if r.Dimensions[6].State != Healthy {
		t.Fatalf("pipeline=%s, want HEALTHY once at least one observation exists and none failed", r.Dimensions[6].State)
	}
}

func TestEvaluateSurfacesMalformedObservationExclusionAsAttentionNotAsImprovedHealth(t *testing.T) {
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, nil, nil, Exclusions{MalformedObservations: 2})
	found := false
	for _, a := range r.Attention {
		if a.Kind == "MALFORMED_OBSERVATIONS_EXCLUDED" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected an explicit MALFORMED_OBSERVATIONS_EXCLUDED attention item")
	}
	// Exclusion must not be miscounted as evidence of a healthy pipeline --
	// it is still zero observed, just with a known reason.
	if r.Dimensions[6].State != NotEstablished {
		t.Fatalf("pipeline=%s, want NOT_ESTABLISHED: excluded objects must not count toward pipeline health", r.Dimensions[6].State)
	}
}

func TestEvaluateSurfacesMalformedProposalExclusionAsAttention(t *testing.T) {
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, nil, nil, Exclusions{MalformedProposals: 1})
	found := false
	for _, a := range r.Attention {
		if a.Kind == "MALFORMED_PROPOSALS_EXCLUDED" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected an explicit MALFORMED_PROPOSALS_EXCLUDED attention item")
	}
}
