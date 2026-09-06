package history

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

func testContributionObservation(t *testing.T, state domain.ExecutionState) domain.Observation {
	t.Helper()
	cluster, err := domain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	workload := domain.WorkloadIdentity{
		Cluster: cluster, Namespace: "default",
		GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"},
		Name:      "api", UID: "workload-uid",
	}
	slot := domain.ContainerSlot{Workload: workload, Container: "app"}
	spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: slot}, []string{"filesystem", "exec", "networkConnect", "networkBind", "capabilities"}, time.Minute, "test")
	if err != nil {
		t.Fatal(err)
	}
	if state == domain.ExecutionRequested {
		observation, err := domain.NewObservation(domain.ObservationID("observation-adapter-1"), spec)
		if err != nil {
			t.Fatal(err)
		}
		return observation
	}
	if state != domain.ExecutionCompleted && state != domain.ExecutionFailed {
		emptyResult, err := domain.NewObservationResult(nil)
		if err != nil {
			t.Fatal(err)
		}
		observation, err := domain.RestoreObservation(domain.ObservationID("observation-adapter-1"), spec, domain.ObservationBinding{}, domain.ObservationExecution{State: state}, emptyResult, domain.ObservationProvenance{})
		if err != nil {
			t.Fatal(err)
		}
		return observation
	}
	image, err := domain.NewContainerImageRevision(slot, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	runtimeTarget, err := domain.NewResolvedTargetSet([]domain.RuntimeContainerInstance{{Slot: slot, PodUID: "pod-uid", ContainerID: "container-id"}})
	if err != nil {
		t.Fatal(err)
	}
	qualification := domain.SourceQualification{
		BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true,
		FlushConfirmed: true, Attribution: domain.AttributionCompleted, AttributedCount: 1,
	}
	sources := []struct {
		name  string
		facts domain.NormalizedFacts
	}{
		{"filesystem", domain.NormalizedFacts{Filesystem: []domain.FilesystemFact{{Path: "/tmp/a", Permissions: []profile.FilePermission{profile.PermissionRead}}}}},
		{"exec", domain.NormalizedFacts{Exec: []domain.ExecFact{{Path: "/bin/a"}}}},
		{"networkConnect", domain.NormalizedFacts{NetworkConnect: []domain.NetworkFact{{Port: 443, Direction: profile.DirectionEgress}}}},
		{"networkBind", domain.NormalizedFacts{NetworkBind: []domain.NetworkFact{{Port: 8443, Direction: profile.DirectionIngress}}}},
		{"capabilities", domain.NormalizedFacts{Capabilities: []domain.CapabilityFact{{Name: "CAP_NET_RAW"}}}},
	}
	results := make([]domain.SourceResult, 0, len(sources))
	for _, source := range sources {
		result, err := domain.NewSourceResult(domain.EvidenceSource{Name: source.name, Backend: "test", Version: "v1"}, qualification, nil, source.facts)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, result)
	}
	result, err := domain.NewObservationResult(results)
	if err != nil {
		t.Fatal(err)
	}
	binding := domain.ObservationBinding{ResolvedTargets: runtimeTarget, ImageRevisions: []domain.ContainerImageRevision{image}, Backend: domain.BackendIdentity{Kind: "test", Version: "v1"}}
	provenance := domain.ObservationProvenance{ResolvedTargets: runtimeTarget, ImageRevisions: []domain.ContainerImageRevision{image}, Backend: binding.Backend, RequestedSources: spec.SourceNames()}
	completion := domain.CompletedNormally
	if state == domain.ExecutionFailed {
		completion = domain.BackendFailure
	}
	observation, err := domain.RestoreObservation(domain.ObservationID("observation-adapter-1"), spec, binding, domain.ObservationExecution{State: state, Completion: completion}, result, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func TestContributionFromCompletedObservationMapsFactsAndIdentity(t *testing.T) {
	contribution, err := ContributionFromObservation(testContributionObservation(t, domain.ExecutionCompleted))
	if err != nil {
		t.Fatal(err)
	}
	if contribution.Population.Scope != ScopeContainer || contribution.Population.Target != "Deployment/api" || contribution.Population.Container != "app" || contribution.Population.ImageIdentity == "" || contribution.Population.BinaryPath != "" {
		t.Fatalf("population = %#v", contribution.Population)
	}
	if contribution.ObservationID != "observation-adapter-1" || len(contribution.Filesystem) != 1 || len(contribution.NetworkConnect) != 1 || len(contribution.NetworkBind) != 1 || len(contribution.Capabilities) != 1 {
		t.Fatalf("contribution facts = %#v", contribution)
	}
	if len(contribution.Sources) != 5 {
		t.Fatalf("source summaries = %#v", contribution.Sources)
	}
	for _, source := range contribution.Sources {
		if source.Source == string(SourceExec) && source.NormalizedFactCount != 1 {
			t.Fatalf("exec provenance was not retained: %#v", source)
		}
	}
}

func TestContributionFromObservationRejectsNonEligibleStates(t *testing.T) {
	for _, state := range []domain.ExecutionState{domain.ExecutionRequested, domain.ExecutionStarting, domain.ExecutionRunning, domain.ExecutionCompleting, domain.ExecutionFailed} {
		observation := testContributionObservation(t, state)
		if _, err := ContributionFromObservation(observation); err == nil {
			t.Fatalf("state %s was accepted", state)
		}
	}
}

func TestApplyObservationContributionDelegatesToG63(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	observation := testContributionObservation(t, domain.ExecutionCompleted)
	first, err := ApplyObservationContribution(context.Background(), client, "default", observation)
	if err != nil || first != ContributionApplied {
		t.Fatalf("first application = %s, %v", first, err)
	}
	second, err := ApplyObservationContribution(context.Background(), client, "default", observation)
	if err != nil || second != ContributionAlreadyCommitted {
		t.Fatalf("replay = %s, %v", second, err)
	}
	identity := PopulationIdentity{Scope: ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	name, err := RecordNameContainerV2(identity)
	if err != nil {
		t.Fatal(err)
	}
	record, err := Get(context.Background(), client, "default", name)
	if err != nil || len(record.Populations) != 1 {
		t.Fatalf("record = %#v, %v", record, err)
	}
	population := record.Populations[0]
	if population.Scope != ScopeContainer || population.RunsRecorded != 0 || len(population.FilesystemAccesses) != 1 || len(population.NetworkAccesses) != 2 || len(population.CapabilityAccesses) != 1 || len(population.ObservationContributions) != 1 {
		t.Fatalf("population = %#v", population)
	}
}
