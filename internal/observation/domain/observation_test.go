package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

func testObservation(t *testing.T, sources []string) *Observation {
	t.Helper()
	spec, err := NewObservationSpec(RequestedTarget{Slot: testSlot("workload", "app")}, sources, time.Minute, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	o, err := NewObservation(ObservationID("observation-1"), spec)
	if err != nil {
		t.Fatal(err)
	}
	return &o
}

func testRuntimeInstance() RuntimeContainerInstance {
	return RuntimeContainerInstance{Slot: testSlot("workload", "app"), PodUID: "pod-1", ContainerID: "container-1"}
}

func qualified(attributed, excluded uint64) SourceQualification {
	return SourceQualification{BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: AttributionCompleted, AttributedCount: attributed, ExcludedCount: excluded}
}

func sourceResult(t *testing.T, name string, q SourceQualification) SourceResult {
	t.Helper()
	result, err := NewSourceResult(EvidenceSource{Name: name, Backend: "test", Version: "v1"}, q, []string{"sha256:evidence"})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestObservationIDIsIndependentOfMutableDomainState(t *testing.T) {
	first := testObservation(t, []string{"filesystem"})
	second := testObservation(t, []string{"network"})
	if first.ID() != second.ID() {
		t.Fatal("test IDs should be equal")
	}
	if err := first.Transition(ExecutionStarting, ""); err != nil {
		t.Fatal(err)
	}
	if first.ID() != ObservationID("observation-1") {
		t.Fatal("execution state changed ObservationID")
	}
	if _, err := NewObservationID(); err != nil {
		t.Fatal(err)
	}
	if ObservationID("observation-1") == ObservationID("observation-2") {
		t.Fatal("different opaque IDs compared equal")
	}
}

func TestObservationSpecIsValidatedAndCopied(t *testing.T) {
	sources := []string{"network", "filesystem"}
	spec, err := NewObservationSpec(RequestedTarget{Slot: testSlot("workload", "app")}, sources, time.Second, "session")
	if err != nil {
		t.Fatal(err)
	}
	sources[0] = "mutated"
	if got := spec.SourceNames(); got[0] != "filesystem" || got[1] != "network" {
		t.Fatalf("spec sources = %v", got)
	}
	if _, err := NewObservationSpec(RequestedTarget{}, []string{"filesystem"}, time.Second, "session"); err == nil {
		t.Fatal("invalid target was accepted")
	}
	if _, err := NewObservationSpec(RequestedTarget{Slot: testSlot("workload", "app")}, nil, time.Second, "session"); err == nil {
		t.Fatal("missing sources were accepted")
	}
}

func TestExecutionStateGraph(t *testing.T) {
	o := testObservation(t, []string{"filesystem"})
	for _, state := range []ExecutionState{ExecutionStarting, ExecutionRunning, ExecutionCompleting} {
		if err := o.Transition(state, ""); err != nil {
			t.Fatalf("transition to %s: %v", state, err)
		}
	}
	if err := o.Transition(ExecutionCompleted, CompletedNormally); err != nil {
		t.Fatal(err)
	}
	if o.Execution().State != ExecutionCompleted || !o.Frozen() {
		t.Fatalf("execution = %#v, frozen=%v", o.Execution(), o.Frozen())
	}
	for _, next := range []ExecutionState{ExecutionStarting, ExecutionRunning, ExecutionFailed} {
		if err := o.Transition(next, BackendFailure); !errors.Is(err, ErrObservationFrozen) {
			t.Fatalf("terminal transition to %s error = %v", next, err)
		}
	}
}

func TestForbiddenExecutionEdgesAndStates(t *testing.T) {
	if ExecutionState("UNKNOWN").Valid() || ExecutionState("CANCELLED").Valid() {
		t.Fatal("UNKNOWN/CANCELLED became execution states")
	}
	cases := []struct{ from, to ExecutionState }{{ExecutionRequested, ExecutionRunning}, {ExecutionRunning, ExecutionCompleted}, {ExecutionCompleted, ExecutionFailed}}
	for _, tc := range cases {
		o := testObservation(t, []string{"filesystem"})
		o.execution.State = tc.from
		if err := o.Transition(tc.to, BackendFailure); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("%s -> %s error = %v", tc.from, tc.to, err)
		}
	}
	starting := testObservation(t, []string{"filesystem"})
	starting.execution.State = ExecutionStarting
	if err := starting.Transition(ExecutionFailed, BackendFailure); err != nil {
		t.Fatal(err)
	}
}

func TestCanRequestStopCoversCancellableNonTerminalStates(t *testing.T) {
	for _, state := range []ExecutionState{ExecutionRequested, ExecutionStarting, ExecutionRunning, ExecutionCompleting} {
		o := testObservation(t, []string{"filesystem"})
		o.execution.State = state
		if !o.CanRequestStop() {
			t.Fatalf("%s should accept durable stop intent", state)
		}
	}
	for _, state := range []ExecutionState{ExecutionCompleted, ExecutionFailed} {
		o := testObservation(t, []string{"filesystem"})
		o.execution.State = state
		o.frozen = true
		if o.CanRequestStop() {
			t.Fatalf("%s should reject durable stop intent", state)
		}
	}
}

func TestAttributionTransitions(t *testing.T) {
	if err := AdvanceAttribution(AttributionNotStarted, AttributionInProgress); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceAttribution(AttributionInProgress, AttributionCompleted); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceAttribution(AttributionInProgress, AttributionFailed); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceAttribution(AttributionCompleted, AttributionInProgress); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("backwards attribution transition error = %v", err)
	}
}

func TestEvidenceQualificationRules(t *testing.T) {
	tests := []struct {
		name  string
		q     SourceQualification
		state EvidenceState
	}{
		{"qualified empty", qualified(0, 0), EvidenceEmpty},
		{"qualified available", qualified(2, 0), EvidenceAvailable},
		{"excluded only", qualified(0, 1), EvidenceUnknown},
		{"positive and excluded", qualified(12, 3), EvidenceUnknown},
		{"unhealthy", SourceQualification{SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: AttributionCompleted}, EvidenceUnknown},
		{"not attached", SourceQualification{BackendHealthConfirmed: true, FlushConfirmed: true, Attribution: AttributionCompleted}, EvidenceUnknown},
		{"not flushed", SourceQualification{BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true, Attribution: AttributionCompleted}, EvidenceUnknown},
		{"attribution incomplete", SourceQualification{BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: AttributionInProgress}, EvidenceUnknown},
	}
	for _, tt := range tests {
		if got := DeriveEvidenceState(tt.q); got != tt.state {
			t.Errorf("%s: state = %s, want %s", tt.name, got, tt.state)
		}
	}
}

func TestPositiveFactsSurviveUnknown(t *testing.T) {
	facts := NormalizedFacts{Filesystem: []FilesystemFact{{Path: "/etc/hosts", Permissions: []profile.FilePermission{profile.PermissionWrite, profile.PermissionRead}}}}
	result, err := NewSourceResult(EvidenceSource{Name: "filesystem"}, qualified(12, 3), nil, facts)
	if err != nil { t.Fatal(err) }
	if result.Evidence != EvidenceUnknown || result.Qualification.AttributedCount != 12 || len(result.References) != 0 {
		t.Fatalf("source result lost positive facts: %#v", result)
	}
	if len(result.Facts.Filesystem) != 1 { t.Fatalf("normalized facts lost: %#v", result.Facts) }
}

func TestNormalizedFactsAreClosedDeduplicatedAndBounded(t *testing.T) {
	facts := NormalizedFacts{NetworkConnect: []NetworkFact{{Port: 443, Direction: profile.DirectionEgress}}}
	result, err := NewSourceResult(EvidenceSource{Name: "networkConnect"}, qualified(2, 0), nil, facts)
	if err != nil || len(result.Facts.NetworkConnect) != 1 { t.Fatalf("network facts = %#v, err=%v", result.Facts, err) }
	if _, err := NewSourceResult(EvidenceSource{Name: "networkConnect"}, qualified(0, 0), nil, facts); err == nil { t.Fatal("facts with zero attributed count accepted") }
	if _, err := NewSourceResult(EvidenceSource{Name: "exec"}, qualified(1, 0), nil, NormalizedFacts{Capabilities: []CapabilityFact{{Name: "CAP_NET_RAW"}}}); err == nil { t.Fatal("facts for the wrong source accepted") }
	tooMany := NormalizedFacts{Capabilities: make([]CapabilityFact, 257)}
	if _, err := NewSourceResult(EvidenceSource{Name: "capabilities"}, qualified(257, 0), nil, tooMany); err == nil { t.Fatal("fact overflow accepted") }
}

func TestNormalizedFactsCanonicalOrderAndEvidenceIndependence(t *testing.T) {
	facts := NormalizedFacts{Filesystem: []FilesystemFact{{Path: "/z", Permissions: []profile.FilePermission{profile.PermissionWrite, profile.PermissionRead}}, {Path: "/a", Permissions: []profile.FilePermission{profile.PermissionExecute}}}}
	result, err := NewSourceResult(EvidenceSource{Name: "filesystem"}, qualified(2, 1), nil, facts)
	if err != nil { t.Fatal(err) }
	if result.Evidence != EvidenceUnknown || result.Facts.Filesystem[0].Path != "/a" || result.Facts.Filesystem[1].Permissions[0] != profile.PermissionRead { t.Fatalf("facts/evidence = %#v/%s", result.Facts, result.Evidence) }
}

func TestMixedSourceStatesRemainIndependent(t *testing.T) {
	o := testObservation(t, []string{"filesystem", "network", "exec"})
	for _, result := range []SourceResult{sourceResult(t, "filesystem", qualified(1, 0)), sourceResult(t, "network", qualified(0, 1)), sourceResult(t, "exec", qualified(0, 0))} {
		if err := o.RecordSourceResult(result); err != nil {
			t.Fatal(err)
		}
	}
	sources := o.Result().Sources()
	if len(sources) != 3 || sources[0].Source.Name != "exec" || sources[0].Evidence != EvidenceEmpty || sources[1].Source.Name != "filesystem" || sources[1].Evidence != EvidenceAvailable || sources[2].Source.Name != "network" || sources[2].Evidence != EvidenceUnknown {
		t.Fatalf("mixed source states = %#v", sources)
	}
}

func TestBindingAndResultFreezeAtTerminalization(t *testing.T) {
	o := testObservation(t, []string{"filesystem"})
	resolved, err := NewResolvedTargetSet([]RuntimeContainerInstance{testRuntimeInstance()})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Bind(resolved, BackendIdentity{Kind: "gadget", Version: "v1"}, nil); err != nil {
		t.Fatal(err)
	}
	event, err := NewTargetChangeEvent(time.Unix(1, 0).UTC(), PodAdded, "pod-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := o.AppendTargetChange(event); err != nil {
		t.Fatal(err)
	}
	if err := o.RecordSourceResult(sourceResult(t, "filesystem", qualified(1, 0))); err != nil {
		t.Fatal(err)
	}
	for _, state := range []ExecutionState{ExecutionStarting, ExecutionRunning, ExecutionCompleting} {
		if err := o.Transition(state, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := o.Transition(ExecutionCompleted, CompletedNormally); err != nil {
		t.Fatal(err)
	}
	if len(o.Provenance().ResolvedTargets.Items()) != 1 || len(o.Provenance().RequestedSources) != 1 {
		t.Fatal("terminal provenance was not derived from final facts")
	}
	if err := o.AppendTargetChange(event); !errors.Is(err, ErrObservationFrozen) {
		t.Fatalf("append after terminal error = %v", err)
	}
	if err := o.RecordSourceResult(sourceResult(t, "filesystem", qualified(2, 0))); !errors.Is(err, ErrObservationFrozen) {
		t.Fatalf("result update after terminal error = %v", err)
	}
	if err := o.Bind(resolved, BackendIdentity{}, nil); !errors.Is(err, ErrObservationFrozen) {
		t.Fatalf("binding update after terminal error = %v", err)
	}
}

func TestTargetChangeEventsPreserveAppendOrder(t *testing.T) {
	o := testObservation(t, []string{"filesystem"})
	first, _ := NewTargetChangeEvent(time.Unix(1, 0).UTC(), PodAdded, "a")
	second, _ := NewTargetChangeEvent(time.Unix(2, 0).UTC(), PodRemoved, "b")
	if err := o.AppendTargetChange(first); err != nil {
		t.Fatal(err)
	}
	if err := o.AppendTargetChange(second); err != nil {
		t.Fatal(err)
	}
	events := o.Binding().TargetChangeEvents()
	if len(events) != 2 || events[0] != first || events[1] != second {
		t.Fatalf("target changes = %#v", events)
	}
}
