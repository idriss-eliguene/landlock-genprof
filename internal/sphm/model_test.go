package sphm

import (
	"testing"
	"time"
)

func TestEvaluateKeepsTerminalUnknownDistinctFromFailure(t *testing.T) {
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, []Observation{{ID: "obs-1", Workload: "payments/api", State: "COMPLETED", Frozen: true, Evidence: "UNKNOWN"}}, nil)
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
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, []Observation{{ID: "failed", Workload: "payments/api", State: "FAILED", Failed: true}}, []Proposal{{Name: "p", Workload: "Deployment/api", Status: "DRAFT"}})
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
	r := Evaluate(time.Unix(1, 0), Context{Namespace: "payments"}, nil, nil)
	if r.Dimensions[1].State != NotEstablished {
		t.Fatalf("coverage=%s, want NOT_ESTABLISHED", r.Dimensions[1].State)
	}
	if r.Dimensions[4].State != NotEstablished || r.Dimensions[7].State != NotEstablished {
		t.Fatal("unsupported health dimensions were marked healthy")
	}
}
