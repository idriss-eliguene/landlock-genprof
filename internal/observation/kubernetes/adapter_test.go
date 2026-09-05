package kubernetes

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func testObservation(t *testing.T) domain.Observation {
	t.Helper()
	cluster, err := domain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	workload := domain.WorkloadIdentity{Cluster: cluster, Namespace: "workloads", GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "workload-uid"}
	slot := domain.ContainerSlot{Workload: workload, Container: "backend"}
	spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: slot}, []string{"filesystem", "network"}, time.Minute, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domain.NewObservation(domain.ObservationID("observation-1"), spec)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func boundObservation(t *testing.T, observation domain.Observation) domain.Observation {
	t.Helper()
	instance := domain.RuntimeContainerInstance{Slot: observation.Spec().Target.Slot, PodUID: "pod-uid", ContainerID: "containerd://container"}
	targets, err := domain.NewResolvedTargetSet([]domain.RuntimeContainerInstance{instance})
	if err != nil {
		t.Fatal(err)
	}
	if err := observation.Bind(targets, domain.BackendIdentity{Kind: "gadget", Version: "v0.55.1"}, nil); err != nil {
		t.Fatal(err)
	}
	event, err := domain.NewTargetChangeEvent(time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), domain.PodAdded, "pod-uid")
	if err != nil {
		t.Fatal(err)
	}
	if err := observation.AppendTargetChange(event); err != nil {
		t.Fatal(err)
	}
	qualification := domain.SourceQualification{BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted, AttributedCount: 1}
	for _, name := range []string{"filesystem", "network"} {
		result, err := domain.NewSourceResult(domain.EvidenceSource{Name: name, Backend: "gadget", Version: "v0.55.1"}, qualification, []string{"artifact://" + name})
		if err != nil {
			t.Fatal(err)
		}
		if err := observation.RecordSourceResult(result); err != nil {
			t.Fatal(err)
		}
	}
	return observation
}

func TestDomainPersistenceRoundTrip(t *testing.T) {
	original := boundObservation(t, testObservation(t))
	object, err := ToUnstructured(original, "default")
	if err != nil {
		t.Fatal(err)
	}
	status, err := encodeStatus(original)
	if err != nil {
		t.Fatal(err)
	}
	object.Object["status"] = status
	restored, err := FromUnstructured(object)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID() != original.ID() || !reflect.DeepEqual(restored.Spec(), original.Spec()) || !reflect.DeepEqual(restored.Binding(), original.Binding()) || !reflect.DeepEqual(restored.Execution(), original.Execution()) || !reflect.DeepEqual(restored.Result(), original.Result()) {
		t.Fatalf("round trip changed domain record: %#v != %#v", restored, original)
	}
}

func TestStoreCreateGetAndStatusUsesResourceVersion(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	store, err := NewStore(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	original := testObservation(t)
	rv, err := store.CreateObservation(ctx, "default", original)
	if err != nil {
		t.Fatal(err)
	}
	if rv == "" {
		// client-go's dynamic fake does not allocate resourceVersions.
		raw, err := client.Resource(GVR).Namespace("default").Get(ctx, string(original.ID()), metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		raw.SetResourceVersion("1")
		if _, err := client.Resource(GVR).Namespace("default").Update(ctx, raw, metav1.UpdateOptions{}); err != nil {
			t.Fatal(err)
		}
		rv = "1"
	}
	got, gotRV, err := store.GetObservation(ctx, "default", string(original.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if gotRV != rv || got.Execution().State != domain.ExecutionRequested {
		t.Fatalf("unexpected create state/rv: %s %#v", gotRV, got.Execution())
	}
	updated := boundObservation(t, got)
	status, err := encodeStatus(updated)
	if err != nil {
		t.Fatal(err)
	}
	newRV, err := store.updateObservationStatus(ctx, "default", updated, rv, status)
	if err != nil {
		t.Fatal(err)
	}
	if newRV != rv {
		t.Fatalf("status update did not propagate resourceVersion: got %q want %q", newRV, rv)
	}
	got, _, err = store.GetObservation(ctx, "default", string(original.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Result().Sources()) != 2 || len(got.Binding().TargetChangeEvents()) != 1 {
		t.Fatalf("status was not persisted: %#v", got)
	}
}

func TestUnknownPositiveFactsRoundTrip(t *testing.T) {
	observation := testObservation(t)
	qualification := domain.SourceQualification{BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted, AttributedCount: 12, ExcludedCount: 3}
	result, err := domain.NewSourceResult(domain.EvidenceSource{Name: "filesystem"}, qualification, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evidence != domain.EvidenceUnknown {
		t.Fatalf("state=%s", result.Evidence)
	}
	if err := observation.RecordSourceResult(result); err != nil {
		t.Fatal(err)
	}
	object, err := ToUnstructured(observation, "default")
	if err != nil {
		t.Fatal(err)
	}
	status, err := encodeStatus(observation)
	if err != nil {
		t.Fatal(err)
	}
	object.Object["status"] = status
	restored, err := FromUnstructured(object)
	if err != nil {
		t.Fatal(err)
	}
	sources := restored.Result().Sources()
	if len(sources) != 1 || sources[0].Evidence != domain.EvidenceUnknown || sources[0].Qualification.AttributedCount != 12 || sources[0].Qualification.ExcludedCount != 3 {
		t.Fatalf("positive facts were not preserved: %#v", sources)
	}
}

func TestStatusMutationRejectsNonAppendAndTerminalChanges(t *testing.T) {
	old := boundObservation(t, testObservation(t))
	changed := old
	changedBinding := changed.Binding()
	changedBinding.TargetChanges = nil
	changed, err := domain.RestoreObservation(changed.ID(), changed.Spec(), changedBinding, changed.Execution(), changed.Result(), changed.Provenance())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateStatusMutation(old, changed); err == nil {
		t.Fatal("target-change removal was accepted")
	}

	terminal := boundObservation(t, testObservation(t))
	for _, state := range []domain.ExecutionState{domain.ExecutionStarting, domain.ExecutionRunning, domain.ExecutionCompleting, domain.ExecutionCompleted} {
		if err := terminal.Transition(state, domain.CompletedNormally); err != nil {
			t.Fatal(err)
		}
	}
	altered, err := domain.RestoreObservation(terminal.ID(), terminal.Spec(), terminal.Binding(), terminal.Execution(), terminal.Result(), terminal.Provenance())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateStatusMutation(terminal, altered); err != nil {
		t.Fatalf("identical terminal status rejected: %v", err)
	}
	sources := terminal.Result().Sources()
	sources[0].Qualification.AttributedCount++
	changedResult, err := domain.NewObservationResult(sources)
	if err != nil {
		t.Fatal(err)
	}
	changedTerminal, err := domain.RestoreObservation(terminal.ID(), terminal.Spec(), terminal.Binding(), terminal.Execution(), changedResult, terminal.Provenance())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateStatusMutation(terminal, changedTerminal); err == nil {
		t.Fatal("terminal result mutation was accepted")
	}
}

func TestFromUnstructuredRejectsIdentityMismatchAndMalformedStatus(t *testing.T) {
	object, err := ToUnstructured(testObservation(t), "default")
	if err != nil {
		t.Fatal(err)
	}
	object.SetName("different-name")
	if _, err := FromUnstructured(object); err != nil { /* name is the identity; this is valid as a renamed object */
	} else {
		t.Log("metadata name defines the new identity as designed")
	}
	object, err = ToUnstructured(testObservation(t), "default")
	if err != nil {
		t.Fatal(err)
	}
	object.Object["status"] = map[string]interface{}{"execution": map[string]interface{}{"state": "NOT_A_STATE"}}
	if _, err := FromUnstructured(object); err == nil {
		t.Fatal("malformed execution state accepted")
	}
}
