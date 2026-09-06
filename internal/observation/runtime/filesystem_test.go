package runtime

import (
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func testRuntimeTarget(t *testing.T) domain.RuntimeContainerInstance {
	t.Helper()
	cluster, err := domain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	workload := domain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "deployment-uid"}
	slot := domain.ContainerSlot{Workload: workload, Container: "backend"}
	image, err := domain.NewContainerImageRevision(slot, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return domain.RuntimeContainerInstance{Slot: slot, PodUID: "pod-uid", ContainerID: "container-uid", ImageRevision: &image}
}

func testFilesystemEvent(at time.Time) tracer.Event {
	return tracer.Event{Timestamp: at, Path: "/etc/config", Mode: "read", Syscall: "openat"}
}

func TestAttributeFilesystemEventRequiresStrongIdentity(t *testing.T) {
	target := testRuntimeTarget(t)
	at := time.Now().UTC()
	missing := AttributeFilesystemEvent(testFilesystemEvent(at), tracer.RuntimeIdentity{Namespace: "default", Container: "backend"}, []domain.RuntimeContainerInstance{target}, at.Add(-time.Second), at.Add(time.Second))
	if missing.Found || missing.Reason != "runtime identity unavailable" {
		t.Fatalf("missing identity = %#v", missing)
	}
	matched := AttributeFilesystemEvent(testFilesystemEvent(at), tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "container-uid"}, []domain.RuntimeContainerInstance{target}, at.Add(-time.Second), at.Add(time.Second))
	if !matched.Found || matched.Target.PodUID != target.PodUID {
		t.Fatalf("matched = %#v", matched)
	}
}

func TestAttributeFilesystemEventAmbiguousIsExcluded(t *testing.T) {
	a := testRuntimeTarget(t)
	b := a
	// The same Pod identity with two otherwise-compatible candidates is
	// ambiguous when the runtime omits the container ID.
	a.ContainerID, b.ContainerID = "container-a", "container-b"
	at := time.Now().UTC()
	got := AttributeFilesystemEvent(testFilesystemEvent(at), tracer.RuntimeIdentity{Namespace: "default", Container: "backend", PodUID: "pod-uid"}, []domain.RuntimeContainerInstance{a, b}, at.Add(-time.Second), at.Add(time.Second))
	if got.Found || got.Reason != "runtime identity matched multiple bound targets" {
		t.Fatalf("ambiguous = %#v", got)
	}
	// A runtime Pod identity is strong enough to select exactly one subject.
	got = AttributeFilesystemEvent(testFilesystemEvent(at), tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "container-b"}, []domain.RuntimeContainerInstance{a, b}, at.Add(-time.Second), at.Add(time.Second))
	if !got.Found || got.Target.ContainerID != "container-b" {
		t.Fatalf("container-specific = %#v", got)
	}
}

func TestFilesystemQualificationPreservesPositiveFactsWhenUnknown(t *testing.T) {
	target := testRuntimeTarget(t)
	start := time.Now().UTC().Add(-time.Second)
	acc := NewFilesystemAccumulator([]domain.RuntimeContainerInstance{target}, start)
	acc.Add(testFilesystemEvent(time.Now().UTC()), tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "container-uid"})
	acc.Add(testFilesystemEvent(time.Now().UTC()), tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "wrong"})
	result, err := acc.SourceResult(true, true, false, domain.AttributionCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evidence != domain.EvidenceUnknown || result.Qualification.AttributedCount != 1 || result.Qualification.ExcludedCount != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestFilesystemQualificationZeroWithoutFlushIsUnknown(t *testing.T) {
	acc := NewFilesystemAccumulator(nil, time.Now().UTC().Add(-time.Second))
	result, err := acc.SourceResult(true, true, false, domain.AttributionCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evidence != domain.EvidenceUnknown || result.Qualification.AttributedCount != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestFilesystemQualificationAvailableWithPositiveAttributedFacts(t *testing.T) {
	target := testRuntimeTarget(t)
	acc := NewFilesystemAccumulator([]domain.RuntimeContainerInstance{target}, time.Now().UTC().Add(-time.Second))
	acc.Add(testFilesystemEvent(time.Now().UTC()), tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "container-uid"})
	result, err := acc.SourceResult(true, true, true, domain.AttributionCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evidence != domain.EvidenceAvailable {
		t.Fatalf("evidence = %s", result.Evidence)
	}
}
