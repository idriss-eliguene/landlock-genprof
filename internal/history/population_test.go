package history

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

func populationTestBehavior(path string) profile.BehaviorProfile {
	return profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{{Path: path}}}}
}

func populationTestFingerprint(image string) PopulationFingerprint {
	return PopulationFingerprint{Target: "payments-api", Container: "app", ImageIdentity: image, BinaryPath: "/bin/app"}
}

func TestMergePopulationSeparatesImageIdentities(t *testing.T) {
	a := populationTestFingerprint("docker-pullable://app@sha256:a")
	b := populationTestFingerprint("docker-pullable://app@sha256:b")
	record := MergePopulation(nil, a, "pod-a", populationTestBehavior("/a"))
	record = MergePopulation(record, b, "pod-b", populationTestBehavior("/a"))
	if len(record.Populations) != 2 {
		t.Fatalf("populations = %d, want 2", len(record.Populations))
	}
	for _, p := range record.Populations {
		if p.RunsRecorded != 1 || p.FilesystemAccesses[0].SeenInRuns != 1 {
			t.Fatalf("population was aggregated across image identities: %+v", p)
		}
	}
}

func TestMergePopulationCompatibleRunsAndSubjects(t *testing.T) {
	f := populationTestFingerprint("docker-pullable://app@sha256:a")
	record := MergePopulation(nil, f, "pod-a", populationTestBehavior("/a"))
	record = MergePopulation(record, f, "pod-b", populationTestBehavior("/a"))
	if len(record.Populations) != 1 || record.Populations[0].RunsRecorded != 2 {
		t.Fatalf("compatible runs did not aggregate: %+v", record.Populations)
	}
	if record.Container != f.Container || record.Binary != f.BinaryPath || record.RunsRecorded != 2 {
		t.Fatalf("fresh record metadata = %q/%q runs=%d", record.Container, record.Binary, record.RunsRecorded)
	}
	if !reflect.DeepEqual(record.Populations[0].Contributors, []string{"pod-a", "pod-b"}) {
		t.Fatalf("contributors = %v", record.Populations[0].Contributors)
	}
	got := ApplyPopulationConfidence(record, f, populationTestBehavior("/a"))
	if got.Filesystem.Accesses[0].Confidence != profile.ConfidenceHigh {
		t.Fatalf("2/2 confidence = %q, want high", got.Filesystem.Accesses[0].Confidence)
	}
}

func TestMergePopulationUnknownImageDoesNotUseQualifiedPopulation(t *testing.T) {
	qualified := populationTestFingerprint("docker-pullable://app@sha256:a")
	unknown := populationTestFingerprint("")
	record := MergePopulation(nil, qualified, "pod-a", populationTestBehavior("/a"))
	record = MergePopulation(record, unknown, "pod-b", populationTestBehavior("/a"))
	if len(record.Populations) != 2 {
		t.Fatalf("unknown image merged into qualified population: %+v", record.Populations)
	}
	if record.Populations[0].Qualified == record.Populations[1].Qualified {
		t.Fatalf("expected exactly one unqualified population: %+v", record.Populations)
	}
	if got := ApplyPopulationConfidence(record, unknown, populationTestBehavior("/a")); got.Filesystem.Accesses[0].Confidence == profile.ConfidenceHigh {
		t.Fatal("unknown image emitted qualified high confidence")
	}
}

func TestMergePopulationInvalidFingerprintDoesNotQualifyOrAggregate(t *testing.T) {
	unknown := populationTestFingerprint("")
	record := MergePopulation(nil, unknown, "pod-a", populationTestBehavior("/a"))
	record = MergePopulation(record, unknown, "pod-b", populationTestBehavior("/a"))
	if len(record.Populations) != 1 || record.Populations[0].Qualified {
		t.Fatalf("unknown observations became qualified: %+v", record.Populations)
	}
	if record.Populations[0].RunsRecorded != 2 {
		t.Fatalf("raw unqualified observations were not preserved: %+v", record.Populations[0])
	}
	if got := ApplyPopulationConfidence(record, unknown, populationTestBehavior("/a")); got.Filesystem.Accesses[0].Confidence == profile.ConfidenceHigh {
		t.Fatal("unqualified observations emitted qualified confidence")
	}
}

func TestConfidenceForHistoryFailsClosedForInvalidCounts(t *testing.T) {
	if got := confidenceForHistory(3, 2); got != profile.ConfidenceLow {
		t.Fatalf("3/2 = %q, want low", got)
	}
}

func TestConfidenceForHistoryRequiresTwoRunsForHigh(t *testing.T) {
	if got := confidenceForHistory(1, 1); got != profile.ConfidenceLow {
		t.Fatalf("1/1 = %q, want low", got)
	}
	if got := confidenceForHistory(2, 2); got != profile.ConfidenceHigh {
		t.Fatalf("2/2 = %q, want high", got)
	}
	if got := confidenceForHistory(1, 2); got != profile.ConfidenceMedium {
		t.Fatalf("1/2 = %q, want medium", got)
	}
}

func TestConfidenceForHistoryRejectsNegativeCounts(t *testing.T) {
	for _, counts := range [][2]int{{-1, 1}, {0, -1}} {
		if got := confidenceForHistory(counts[0], counts[1]); got != profile.ConfidenceLow {
			t.Fatalf("%d/%d = %q, want low", counts[0], counts[1], got)
		}
	}
}

func TestMergePopulationSeparatesTargets(t *testing.T) {
	a := populationTestFingerprint("image@sha256:a")
	b := a
	b.Target = "Deployment/other"
	record := MergePopulation(nil, a, "pod-a", populationTestBehavior("/a"))
	record = MergePopulation(record, b, "pod-b", populationTestBehavior("/a"))
	if len(record.Populations) != 2 {
		t.Fatalf("different governed targets pooled: %+v", record.Populations)
	}
}

func TestMergePopulationPodUIDIsAttributionOnly(t *testing.T) {
	f := populationTestFingerprint("image@sha256:a")
	record := MergePopulation(nil, f, "pod-a", populationTestBehavior("/a"))
	record = MergePopulation(record, f, "pod-b", populationTestBehavior("/a"))
	if len(record.Populations) != 1 || record.Populations[0].RunsRecorded != 2 {
		t.Fatalf("PodUID changed population identity: %+v", record.Populations)
	}
}

func TestMergePopulationSerializationIsDeterministic(t *testing.T) {
	a := populationTestFingerprint("image@sha256:a")
	b := populationTestFingerprint("image@sha256:b")
	left := MergePopulation(MergePopulation(nil, a, "pod-a", populationTestBehavior("/a")), b, "pod-b", populationTestBehavior("/b"))
	right := MergePopulation(MergePopulation(nil, b, "pod-b", populationTestBehavior("/b")), a, "pod-a", populationTestBehavior("/a"))
	lb, err := json.Marshal(left)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := json.Marshal(right)
	if err != nil {
		t.Fatal(err)
	}
	if string(lb) != string(rb) {
		t.Fatalf("serialization differs:\n%s\n%s", lb, rb)
	}
}

func TestConfidenceInvariantForProductionPopulationMerge(t *testing.T) {
	f := populationTestFingerprint("image@sha256:a")
	record := (*Record)(nil)
	for i := 0; i < 4; i++ {
		record = MergePopulation(record, f, "pod-a", populationTestBehavior("/a"))
	}
	for _, p := range record.Populations {
		for _, a := range p.FilesystemAccesses {
			if a.SeenInRuns < 0 || a.SeenInRuns > p.RunsRecorded {
				t.Fatalf("invalid population counts: %+v", p)
			}
		}
	}
}

func TestSavePopulationMergeDoesNotPoolDifferentImageIdentities(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	a := populationTestFingerprint("image@sha256:a")
	b := populationTestFingerprint("image@sha256:b")
	if _, err := SaveWithPopulationMerge(ctx, client, "default", a, "pod-a", populationTestBehavior("/a")); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveWithPopulationMerge(ctx, client, "default", b, "pod-b", populationTestBehavior("/a")); err != nil {
		t.Fatal(err)
	}
	got, err := Get(ctx, client, "default", RecordNameV2("app", "/bin/app"))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Populations) != 2 {
		t.Fatalf("production persistence pooled image identities: %+v", got)
	}
	for _, p := range got.Populations {
		if p.RunsRecorded != 1 {
			t.Fatalf("pooled confidence counters: %+v", got.Populations)
		}
	}
}

func TestMergePopulationIsDeterministicAcrossOrder(t *testing.T) {
	a := populationTestFingerprint("docker-pullable://app@sha256:a")
	b := populationTestFingerprint("docker-pullable://app@sha256:b")
	left := MergePopulation(MergePopulation(nil, a, "pod-a", populationTestBehavior("/a")), b, "pod-b", populationTestBehavior("/b"))
	right := MergePopulation(MergePopulation(nil, b, "pod-b", populationTestBehavior("/b")), a, "pod-a", populationTestBehavior("/a"))
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("merge order changed result:\nleft=%+v\nright=%+v", left, right)
	}
}

func TestSavePopulationMergePreservesLegacyAndRoundTrips(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	legacy := &Record{Container: "app", Binary: "/bin/app", RunsRecorded: 5, FilesystemAccesses: []FileAccessRecord{{Path: "/legacy", SeenInRuns: 5}}}
	if err := Save(context.Background(), client, "default", RecordNameLegacy("app", "/bin/app"), legacy); err != nil {
		t.Fatal(err)
	}
	f := populationTestFingerprint("docker-pullable://app@sha256:a")
	if _, err := SaveWithPopulationMerge(context.Background(), client, "default", f, "pod-a", populationTestBehavior("/new")); err != nil {
		t.Fatal(err)
	}
	record, err := Get(context.Background(), client, "default", RecordNameLegacy("app", "/bin/app"))
	if err != nil {
		t.Fatal(err)
	}
	if record.RunsRecorded != 5 || record.FilesystemAccesses[0].SeenInRuns != 5 {
		t.Fatalf("legacy aggregate changed: %+v", record)
	}
	if len(record.Populations) != 1 || record.Populations[0].RunsRecorded != 1 {
		t.Fatalf("qualified population missing: %+v", record.Populations)
	}
	if got := ApplyPopulationConfidence(record, f, populationTestBehavior("/new")); got.Filesystem.Accesses[0].Confidence == profile.ConfidenceHigh {
		t.Fatal("single qualified run emitted high confidence")
	}
	if _, err := SaveWithPopulationMerge(context.Background(), client, "default", f, "pod-b", populationTestBehavior("/new")); err != nil {
		t.Fatal(err)
	}
	record, err = Get(context.Background(), client, "default", RecordNameLegacy("app", "/bin/app"))
	if err != nil {
		t.Fatal(err)
	}
	if record.Populations[0].RunsRecorded != 2 || len(record.Populations[0].Contributors) != 2 {
		t.Fatalf("population did not round-trip/aggregate: %+v", record.Populations[0])
	}
}
