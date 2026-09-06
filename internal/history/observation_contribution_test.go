package history

import (
	"context"
	"sync"
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

func testVariantObservation(t *testing.T, id, workloadName, container, image string, path string, port int) domain.Observation {
	t.Helper()
	cluster, err := domain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	workload := domain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: workloadName, UID: workloadName + "-uid"}
	slot := domain.ContainerSlot{Workload: workload, Container: container}
	spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: slot}, []string{"filesystem", "networkConnect", "networkBind", "capabilities"}, time.Minute, "test")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewContainerImageRevision(slot, image)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := domain.NewResolvedTargetSet([]domain.RuntimeContainerInstance{{Slot: slot, PodUID: workloadName + "-pod", ContainerID: workloadName + "-container"}})
	if err != nil {
		t.Fatal(err)
	}
	q := domain.SourceQualification{BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted, AttributedCount: 1}
	makeSource := func(name string, facts domain.NormalizedFacts) domain.SourceResult {
		result, err := domain.NewSourceResult(domain.EvidenceSource{Name: name, Backend: "test", Version: "v1"}, q, nil, facts)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	results := []domain.SourceResult{
		makeSource("filesystem", domain.NormalizedFacts{Filesystem: []domain.FilesystemFact{{Path: path, Permissions: []profile.FilePermission{profile.PermissionRead}}}}),
		makeSource("networkConnect", domain.NormalizedFacts{NetworkConnect: []domain.NetworkFact{{Port: port, Direction: profile.DirectionEgress}}}),
		makeSource("networkBind", domain.NormalizedFacts{NetworkBind: []domain.NetworkFact{{Port: port + 1, Direction: profile.DirectionIngress}}}),
		makeSource("capabilities", domain.NormalizedFacts{Capabilities: []domain.CapabilityFact{{Name: "CAP_NET_RAW"}}}),
	}
	result, err := domain.NewObservationResult(results)
	if err != nil {
		t.Fatal(err)
	}
	binding := domain.ObservationBinding{ResolvedTargets: targets, ImageRevisions: []domain.ContainerImageRevision{revision}, Backend: domain.BackendIdentity{Kind: "test", Version: "v1"}}
	provenance := domain.ObservationProvenance{ResolvedTargets: targets, ImageRevisions: []domain.ContainerImageRevision{revision}, Backend: binding.Backend, RequestedSources: spec.SourceNames()}
	observation, err := domain.RestoreObservation(domain.ObservationID(id), spec, binding, domain.ObservationExecution{State: domain.ExecutionCompleted, Completion: domain.CompletedNormally}, result, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func TestObservationAdapterValidationFailsClosedWithoutMutation(t *testing.T) {
	valid := testContributionObservation(t, domain.ExecutionCompleted)
	missingBinding, err := domain.RestoreObservation(valid.ID(), valid.Spec(), domain.ObservationBinding{}, valid.Execution(), valid.Result(), domain.ObservationProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ContributionFromObservation(missingBinding); err == nil {
		t.Fatal("missing binding was accepted")
	}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := ApplyObservationContribution(context.Background(), client, "default", missingBinding); err == nil {
		t.Fatal("invalid observation was applied")
	}
	if objects := client.Actions(); len(objects) != 0 {
		t.Fatalf("validation performed durable actions: %#v", objects)
	}

	missingImage := valid.Binding()
	missingImage.ImageRevisions = nil
	missingImageObservation, err := domain.RestoreObservation(valid.ID(), valid.Spec(), missingImage, valid.Execution(), valid.Result(), domain.ObservationProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ContributionFromObservation(missingImageObservation); err == nil {
		t.Fatal("missing immutable image was accepted")
	}
}

func TestObservationAdapterPreservesUnknownAndEmptySemantics(t *testing.T) {
	observation := testContributionObservation(t, domain.ExecutionCompleted)
	var unknown domain.SourceResult
	for _, source := range observation.Result().Sources() {
		if source.Source.Name == "filesystem" {
			unknown = source
			break
		}
	}
	unknown.Qualification.ExcludedCount = 1
	unknown.Evidence = domain.DeriveEvidenceState(unknown.Qualification)
	result, err := domain.NewObservationResult([]domain.SourceResult{unknown})
	if err != nil {
		t.Fatal(err)
	}
	observation, err = domain.RestoreObservation(observation.ID(), observation.Spec(), observation.Binding(), observation.Execution(), result, observation.Provenance())
	if err != nil {
		t.Fatal(err)
	}
	contribution, err := ContributionFromObservation(observation)
	if err != nil || contribution.Sources[0].EvidenceState != string(domain.EvidenceUnknown) || len(contribution.Filesystem) != 1 {
		t.Fatalf("UNKNOWN positive mapping = %#v, %v", contribution, err)
	}

	q := domain.SourceQualification{BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: domain.AttributionCompleted}
	emptyResult, err := domain.NewSourceResult(domain.EvidenceSource{Name: "filesystem", Backend: "test", Version: "v1"}, q, nil)
	if err != nil {
		t.Fatal(err)
	}
	emptyObservation, err := domain.RestoreObservation(observation.ID(), observation.Spec(), observation.Binding(), observation.Execution(), mustObservationResult(t, emptyResult), observation.Provenance())
	if err != nil {
		t.Fatal(err)
	}
	emptyContribution, err := ContributionFromObservation(emptyObservation)
	if err != nil || len(emptyContribution.Filesystem) != 0 || emptyContribution.Sources[0].EvidenceState != string(domain.EvidenceEmpty) {
		t.Fatalf("EMPTY mapping = %#v, %v", emptyContribution, err)
	}
}

func TestObservationAdapterPreservesUnknownNetworkFacts(t *testing.T) {
	base := testContributionObservation(t, domain.ExecutionCompleted)
	var network domain.SourceResult
	for _, source := range base.Result().Sources() {
		if source.Source.Name == "networkConnect" {
			network = source
			break
		}
	}
	network.Qualification.ExcludedCount = 1
	network.Evidence = domain.DeriveEvidenceState(network.Qualification)
	result := mustObservationResult(t, network)
	observation, err := domain.RestoreObservation(base.ID(), base.Spec(), base.Binding(), base.Execution(), result, base.Provenance())
	if err != nil {
		t.Fatal(err)
	}
	contribution, err := ContributionFromObservation(observation)
	if err != nil || contribution.Sources[0].EvidenceState != string(domain.EvidenceUnknown) || len(contribution.NetworkConnect) != 1 {
		t.Fatalf("UNKNOWN network mapping = %#v, %v", contribution, err)
	}
}

func mustObservationResult(t *testing.T, source domain.SourceResult) domain.ObservationResult {
	t.Helper()
	result, err := domain.NewObservationResult([]domain.SourceResult{source})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestObservationAdapterConcurrentReplayHasOneEffect(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	observation := testContributionObservation(t, domain.ExecutionCompleted)
	start := make(chan struct{})
	var wait sync.WaitGroup
	results := make([]ContributionApplyResult, 2)
	errors := make([]error, 2)
	for i := range results {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			<-start
			results[i], errors[i] = ApplyObservationContribution(context.Background(), client, "default", observation)
		}(i)
	}
	close(start)
	wait.Wait()
	if errors[0] != nil && errors[1] != nil {
		t.Fatalf("both concurrent adapter calls failed: %v / %v", errors[0], errors[1])
	}
	identity := PopulationIdentity{Scope: ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	name, err := RecordNameContainerV2(identity)
	if err != nil {
		t.Fatal(err)
	}
	record, err := Get(context.Background(), client, "default", name)
	if err != nil || record == nil || len(record.Populations) != 1 || len(record.Populations[0].ObservationContributions) != 1 {
		t.Fatalf("concurrent effect = %#v, %v", record, err)
	}
	retry, err := ApplyObservationContribution(context.Background(), client, "default", observation)
	if err != nil || retry != ContributionAlreadyCommitted {
		t.Fatalf("retry after concurrent calls = %s, %v", retry, err)
	}
}

func TestObservationAdapterMultiplePopulationsRemainIsolated(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	first := testVariantObservation(t, "observation-a", "api", "app", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "/tmp/a", 4101)
	second := testVariantObservation(t, "observation-b", "other", "app", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "/tmp/b", 4201)
	var wait sync.WaitGroup
	errors := make(chan error, 2)
	for _, observation := range []domain.Observation{first, second} {
		wait.Add(1)
		go func(observation domain.Observation) {
			defer wait.Done()
			_, err := ApplyObservationContribution(context.Background(), client, "default", observation)
			errors <- err
		}(observation)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	firstIdentity := PopulationIdentity{Scope: ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	secondIdentity := PopulationIdentity{Scope: ScopeContainer, Target: "Deployment/other", Container: "app", ImageIdentity: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	firstRecord, err := GetPopulation(context.Background(), client, "default", firstIdentity)
	if err != nil || firstRecord == nil || len(firstRecord.Populations[0].FilesystemAccesses) != 1 || firstRecord.Populations[0].FilesystemAccesses[0].Path != "/tmp/a" {
		t.Fatalf("first population crossed: %#v, %v", firstRecord, err)
	}
	secondRecord, err := GetPopulation(context.Background(), client, "default", secondIdentity)
	if err != nil || secondRecord == nil || secondRecord.Populations[0].FilesystemAccesses[0].Path != "/tmp/b" {
		t.Fatalf("second population crossed: %#v, %v", secondRecord, err)
	}
}

func TestObservationAdapterConcurrentDifferentObservationsAccumulate(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	image := "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	first := testVariantObservation(t, "observation-concurrent-a", "concurrent", "app", image, "/tmp/concurrent-a", 6101)
	second := testVariantObservation(t, "observation-concurrent-b", "concurrent", "app", image, "/tmp/concurrent-b", 6201)
	start := make(chan struct{})
	var wait sync.WaitGroup
	errors := make([]error, 2)
	for i, observation := range []domain.Observation{first, second} {
		wait.Add(1)
		go func(i int, observation domain.Observation) {
			defer wait.Done()
			<-start
			_, errors[i] = ApplyObservationContribution(context.Background(), client, "default", observation)
		}(i, observation)
	}
	close(start)
	wait.Wait()
	if errors[0] != nil || errors[1] != nil {
		t.Fatalf("different observations failed: %v / %v", errors[0], errors[1])
	}
	identity := PopulationIdentity{Scope: ScopeContainer, Target: "Deployment/concurrent", Container: "app", ImageIdentity: image}
	record, err := GetPopulation(context.Background(), client, "default", identity)
	if err != nil || record == nil || len(record.Populations) != 1 || len(record.Populations[0].ObservationContributions) != 2 || len(record.Populations[0].FilesystemAccesses) != 2 {
		t.Fatalf("concurrent accumulation = %#v, %v", record, err)
	}
}
