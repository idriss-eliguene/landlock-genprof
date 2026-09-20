package main

import "testing"

// M10.4 Finding A: observationEvidenceVerdict replaces an arbitrary
// Sources[0]-by-alphabetical-name collapse that could mask a genuinely
// UNKNOWN source behind a co-existing AVAILABLE source. These vectors pin
// the conservative dominance order the fix establishes: UNKNOWN always
// wins, AVAILABLE beats EMPTY, EMPTY only when every source is EMPTY.

func TestObservationEvidenceVerdict_NoSources(t *testing.T) {
	if got := observationEvidenceVerdict(nil); got != "UNKNOWN" {
		t.Fatalf("got %s, want UNKNOWN for zero sources", got)
	}
}

func TestObservationEvidenceVerdict_UnknownDominatesRegardlessOfNameOrder(t *testing.T) {
	// "aardvark" sorts before "zzz" -- the old Sources[0] collapse would have
	// picked "aardvark" (AVAILABLE) and hidden the genuinely UNKNOWN source.
	sources := []observationSourceRead{
		{Name: "aardvark", EvidenceState: "AVAILABLE"},
		{Name: "zzz", EvidenceState: "UNKNOWN"},
	}
	if got := observationEvidenceVerdict(sources); got != "UNKNOWN" {
		t.Fatalf("got %s, want UNKNOWN: any UNKNOWN source must dominate", got)
	}
}

func TestObservationEvidenceVerdict_AvailableDominatesEmpty(t *testing.T) {
	sources := []observationSourceRead{
		{Name: "a", EvidenceState: "EMPTY"},
		{Name: "b", EvidenceState: "AVAILABLE"},
	}
	if got := observationEvidenceVerdict(sources); got != "AVAILABLE" {
		t.Fatalf("got %s, want AVAILABLE: real evidence from any source must not be hidden", got)
	}
}

func TestObservationEvidenceVerdict_EmptyOnlyWhenAllSourcesEmpty(t *testing.T) {
	sources := []observationSourceRead{
		{Name: "a", EvidenceState: "EMPTY"},
		{Name: "b", EvidenceState: "EMPTY"},
	}
	if got := observationEvidenceVerdict(sources); got != "EMPTY" {
		t.Fatalf("got %s, want EMPTY", got)
	}
}

func TestObservationEvidenceVerdict_SingleUnknownSource(t *testing.T) {
	sources := []observationSourceRead{{Name: "only", EvidenceState: "UNKNOWN"}}
	if got := observationEvidenceVerdict(sources); got != "UNKNOWN" {
		t.Fatalf("got %s, want UNKNOWN", got)
	}
}
