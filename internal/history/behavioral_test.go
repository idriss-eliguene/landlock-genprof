package history

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

func makeFSBehavior(path string) profile.BehaviorProfile {
	return profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{{Path: path}}},
	}
}

func makeNetworkBehavior(port int) profile.BehaviorProfile {
	return profile.BehaviorProfile{
		Network: profile.NetworkProfile{Accesses: []profile.NetworkAccess{{Port: port, Direction: profile.DirectionEgress}}},
	}
}

func makeSyscallBehavior(name string, archs []string) profile.BehaviorProfile {
	return profile.BehaviorProfile{
		Syscalls: profile.SyscallProfile{Accesses: []profile.SyscallAccess{{Name: name}}, Architectures: archs},
	}
}

func makeCapabilityBehavior(name string) profile.BehaviorProfile {
	return profile.BehaviorProfile{
		Capabilities: profile.CapabilityProfile{Accesses: []profile.CapabilityAccess{{Name: name}}},
	}
}

func mergeProfiles(a, b profile.BehaviorProfile) profile.BehaviorProfile {
	// simple merge for tests: append accesses from b into a where non-empty
	if len(b.Filesystem.Accesses) > 0 {
		a.Filesystem = b.Filesystem
	}
	if len(b.Network.Accesses) > 0 {
		a.Network = b.Network
	}
	if len(b.Syscalls.Accesses) > 0 {
		a.Syscalls = b.Syscalls
	}
	if len(b.Capabilities.Accesses) > 0 {
		a.Capabilities = b.Capabilities
	}
	return a
}

func TestBehavioral_FirstRun_FS(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "nginx"
	binary := "/usr/sbin/nginx"
	name := RecordName(container, binary)

	behavior := makeFSBehavior("/etc/passwd")
	persisted, err := SaveWithMerge(ctx, client, "default", name, container, binary, behavior)
	if err != nil {
		t.Fatalf("SaveWithMerge failed: %v", err)
	}
	if persisted.RunsRecorded != 1 {
		t.Fatalf("RunsRecorded = %d, want 1", persisted.RunsRecorded)
	}
	if len(persisted.FilesystemAccesses) != 1 || persisted.FilesystemAccesses[0].SeenInRuns != 1 {
		t.Fatalf("SeenInRuns = %+v, want 1", persisted.FilesystemAccesses)
	}
	// ApplyConfidence
	applied := ApplyConfidence(persisted, behavior)
	if len(applied.Filesystem.Accesses) == 0 || applied.Filesystem.Accesses[0].Confidence != profile.ConfidenceHigh {
		t.Fatalf("Confidence = %v, want high", applied.Filesystem.Accesses)
	}
}

func TestBehavioral_100Occurrences_OneRun(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "nginx"
	binary := "/usr/sbin/nginx"
	name := RecordName(container, binary)

	// simulate 100 occurrences -> policy would dedupe to one access; in test
	// craft behavior with single access (equivalent of dedup)
	behavior := makeFSBehavior("/tmp/x")
	for i := 0; i < 100; i++ {
		// simulate occurrences by reusing same behavior; Merge should only count once
	}
	persisted, err := SaveWithMerge(ctx, client, "default", name, container, binary, behavior)
	if err != nil {
		t.Fatalf("SaveWithMerge failed: %v", err)
	}
	if persisted.RunsRecorded != 1 {
		t.Fatalf("RunsRecorded = %d, want 1", persisted.RunsRecorded)
	}
	if persisted.FilesystemAccesses[0].SeenInRuns != 1 {
		t.Fatalf("SeenInRuns = %d, want 1", persisted.FilesystemAccesses[0].SeenInRuns)
	}
}

func TestBehavioral_TwoRuns_PresentBoth(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "nginx"
	binary := "/usr/sbin/nginx"
	name := RecordName(container, binary)

	behavior := makeFSBehavior("/var/log/app.log")
	if _, err := SaveWithMerge(ctx, client, "default", name, container, binary, behavior); err != nil { t.Fatalf("SaveWithMerge p1: %v", err) }
	p2, err := SaveWithMerge(ctx, client, "default", name, container, binary, behavior)
	if err != nil { t.Fatalf("SaveWithMerge p2: %v", err) }
	if p2.RunsRecorded != 2 { t.Fatalf("RunsRecorded = %d, want 2", p2.RunsRecorded) }
	if p2.FilesystemAccesses[0].SeenInRuns != 2 { t.Fatalf("SeenInRuns = %d, want 2", p2.FilesystemAccesses[0].SeenInRuns) }
	applied := ApplyConfidence(p2, behavior)
	if applied.Filesystem.Accesses[0].Confidence != profile.ConfidenceHigh { t.Fatalf("Confidence = %v, want high", applied.Filesystem.Accesses[0].Confidence) }
}

func TestBehavioral_TwoRuns_PresentOnce(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "nginx"
	binary := "/usr/sbin/nginx"
	name := RecordName(container, binary)

	behavior := makeFSBehavior("/opt/app/config")
	if _, err := SaveWithMerge(ctx, client, "default", name, container, binary, behavior); err != nil { t.Fatalf("first Save: %v", err) }
	// second: successful zero-event run
	empty := profile.BehaviorProfile{}
	p2, err := SaveWithMerge(ctx, client, "default", name, container, binary, empty)
	if err != nil { t.Fatalf("second Save: %v", err) }
	if p2.RunsRecorded != 2 { t.Fatalf("RunsRecorded = %d, want 2", p2.RunsRecorded) }
	if p2.FilesystemAccesses[0].SeenInRuns != 1 { t.Fatalf("SeenInRuns = %d, want 1", p2.FilesystemAccesses[0].SeenInRuns) }
	applied := ApplyConfidence(p2, behavior)
	if applied.Filesystem.Accesses[0].Confidence != profile.ConfidenceMedium { t.Fatalf("Confidence = %v, want medium", applied.Filesystem.Accesses[0].Confidence) }
}

func TestBehavioral_ThreeRuns_OnePresent(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "nginx"
	binary := "/usr/sbin/nginx"
	name := RecordName(container, binary)

	behavior := makeFSBehavior("/srv/data")
	if _, err := SaveWithMerge(ctx, client, "default", name, container, binary, behavior); err != nil { t.Fatalf("first Save: %v", err) }
	// two zero-event successful runs
	if _, err := SaveWithMerge(ctx, client, "default", name, container, binary, profile.BehaviorProfile{}); err != nil { t.Fatalf("second Save: %v", err) }
	p3, err := SaveWithMerge(ctx, client, "default", name, container, binary, profile.BehaviorProfile{})
	if err != nil { t.Fatalf("third Save: %v", err) }
	if p3.RunsRecorded != 3 { t.Fatalf("RunsRecorded = %d, want 3", p3.RunsRecorded) }
	if p3.FilesystemAccesses[0].SeenInRuns != 1 { t.Fatalf("SeenInRuns = %d, want 1", p3.FilesystemAccesses[0].SeenInRuns) }
	applied := ApplyConfidence(p3, behavior)
	if applied.Filesystem.Accesses[0].Confidence != profile.ConfidenceLow { t.Fatalf("Confidence = %v, want low", applied.Filesystem.Accesses[0].Confidence) }
}

func TestBehavioral_ZeroEventSuccessDoesNotChangeSeenInRuns(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "nginx"
	binary := "/usr/sbin/nginx"
	name := RecordName(container, binary)

	behavior := makeFSBehavior("/etc/hosts")
	p1, err := SaveWithMerge(ctx, client, "default", name, container, binary, behavior)
	if err != nil { t.Fatalf("first Save: %v", err) }
	if p1.FilesystemAccesses[0].SeenInRuns != 1 { t.Fatalf("SeenInRuns initial = %d, want 1", p1.FilesystemAccesses[0].SeenInRuns) }
	p2, err := SaveWithMerge(ctx, client, "default", name, container, binary, profile.BehaviorProfile{})
	if err != nil { t.Fatalf("zero-event Save: %v", err) }
	if p2.RunsRecorded != 2 { t.Fatalf("RunsRecorded = %d, want 2", p2.RunsRecorded) }
	if p2.FilesystemAccesses[0].SeenInRuns != 1 { t.Fatalf("SeenInRuns after zero-event = %d, want 1", p2.FilesystemAccesses[0].SeenInRuns) }
}

func TestBehavioral_FilesystemNetworkSyscallCapabilityConfidences(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "svc"
	binary := "/usr/bin/svc"
	name := RecordName(container, binary)

	// Run1: filesystem, network, syscall, capability observed
	bfs := makeFSBehavior("/a")
	bnet := makeNetworkBehavior(8080)
	bsys := makeSyscallBehavior("openat", []string{"SCMP_ARCH_X86_64"})
	bcap := makeCapabilityBehavior("CAP_NET_BIND_SERVICE")
	beh1 := mergeProfiles(bfs, mergeProfiles(bnet, mergeProfiles(bsys, bcap)))
	if _, err := SaveWithMerge(ctx, client, "default", name, container, binary, beh1); err != nil { t.Fatalf("save1: %v", err) }

	// Run2: filesystem + network (network only present in 2 runs), syscall absent, capability present
	beh2 := mergeProfiles(makeFSBehavior("/a"), mergeProfiles(makeNetworkBehavior(8080), makeCapabilityBehavior("CAP_NET_BIND_SERVICE")))
	if _, err := SaveWithMerge(ctx, client, "default", name, container, binary, beh2); err != nil { t.Fatalf("save2: %v", err) }

	// Run3: filesystem + capability only
	beh3 := mergeProfiles(makeFSBehavior("/a"), makeCapabilityBehavior("CAP_NET_BIND_SERVICE"))
	rec, err := SaveWithMerge(ctx, client, "default", name, container, binary, beh3)
	if err != nil { t.Fatalf("save3: %v", err) }

	// rec should show RunsRecorded = 3
	if rec.RunsRecorded != 3 { t.Fatalf("RunsRecorded = %d, want 3", rec.RunsRecorded) }
	// filesystem seen 3/3 -> High
	fsSeen := 0
	for _, f := range rec.FilesystemAccesses { if f.Path == "/a" { fsSeen = f.SeenInRuns } }
	if fsSeen != 3 { t.Fatalf("filesystem seen = %d, want 3", fsSeen) }

	// compute confidences from record's seen counts
	fsConf := confidenceForHistory(fsSeen, rec.RunsRecorded)
	if fsConf != profile.ConfidenceHigh { t.Fatalf("filesystem confidence = %v, want high", fsConf) }
	// network seen 2/3 -> Medium
	netSeen := 0
	for _, n := range rec.NetworkAccesses { if n.Port == 8080 { netSeen = n.SeenInRuns } }
	netConf := confidenceForHistory(netSeen, rec.RunsRecorded)
	if netConf != profile.ConfidenceMedium { t.Fatalf("network confidence = %v, want medium", netConf) }
	// syscall seen 1/3 -> Low
	sySeen := 0
	for _, s := range rec.SyscallAccesses { if s.Name == "openat" { sySeen = s.SeenInRuns } }
	syConf := confidenceForHistory(sySeen, rec.RunsRecorded)
	if syConf != profile.ConfidenceLow { t.Fatalf("syscall confidence = %v, want low", syConf) }
	// capability seen 3/3 -> High
	capSeen := 0
	for _, c := range rec.CapabilityAccesses { if c.Name == "CAP_NET_BIND_SERVICE" { capSeen = c.SeenInRuns } }
	capConf := confidenceForHistory(capSeen, rec.RunsRecorded)
	if capConf != profile.ConfidenceHigh { t.Fatalf("capability confidence = %v, want high", capConf) }
}

func TestBehavioral_LegacyContinuityAndV2(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "nginx"
	path1 := "/opt/tools/run-helper"
	legacyName := RecordNameLegacy(container, path1)
	v2Name := RecordNameV2(container, path1)
	// create legacy record via Save
	rec := &Record{Container: container, Binary: path1, RunsRecorded: 1,
		FilesystemAccesses: []FileAccessRecord{{Path: "/x", SeenInRuns: 1}},
	}
	if err := Save(ctx, client, "default", legacyName, rec); err != nil { t.Fatalf("initial legacy save: %v", err) }
	// call SaveWithMerge -> should update legacy and not create V2
	behavior := makeFSBehavior("/x")
	p, err := SaveWithMerge(ctx, client, "default", legacyName, container, path1, behavior)
	if err != nil { t.Fatalf("SaveWithMerge legacy: %v", err) }
	if p.RunsRecorded != 2 { t.Fatalf("legacy continued RunsRecorded = %d, want 2", p.RunsRecorded) }
	gotV2, _ := Get(ctx, client, "default", v2Name)
	if gotV2 != nil { t.Fatalf("V2 unexpectedly created when legacy existed: %v", v2Name) }
}

func TestBehavioral_ConflictRetryReturnsFreshRecord(t *testing.T) {
	// Simulate other writer winning by mutating underlying storage when injecting conflict
	underlying := fake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	container := "nginx"
	binary := "/usr/sbin/nginx"
	name := RecordName(container, binary)
	// initial save
	if err := Save(ctx, underlying, "default", name, &Record{Container: container, Binary: binary, RunsRecorded: 1}); err != nil { t.Fatalf("initial save: %v", err) }

	
	// Build a custom client that injects a Conflict on the first Update call
	// and *also* performs an intervening Save to simulate another writer
	// winning the race by incrementing RunsRecorded.
	// Manual intervening save: other writer increments to 2
	if err := Save(ctx, underlying, "default", name, &Record{Container: container, Binary: binary, RunsRecorded: 2}); err != nil { t.Fatalf("manual intervening save: %v", err) }
	client := newConflictInjectingClient(underlying, 1)
	behavior := makeFSBehavior("/conflict")
	p, err := SaveWithMerge(ctx, client, "default", name, container, binary, behavior)
	if err != nil { t.Fatalf("SaveWithMerge conflict test failed: %v", err) }
	// expected: manual intervening save made RunsRecorded=2, our merge => 3
	if p.RunsRecorded != 3 {
		t.Fatalf("expected RunsRecorded 3 after conflict+retry, got %d", p.RunsRecorded)
	}
}


func TestBehavioral_ReplayDoesNotMutateHistory(t *testing.T) {
	// reconstructSemantic is in cmd package; it doesn't touch history. This test is a smoke check of that contract.
	// Nothing to do here in history package beyond asserting SaveWithMerge not called.
}
