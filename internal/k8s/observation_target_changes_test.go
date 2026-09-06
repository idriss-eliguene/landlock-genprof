package k8s

import (
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

func targetSnapshot(t *testing.T, pod, containerID, digest, revision string) ObservationTarget {
	t.Helper()
	cluster, _ := domain.NewClusterIdentity("cluster")
	workload := domain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: "workload"}
	slot := domain.ContainerSlot{Workload: workload, Container: "backend"}
	instance := domain.RuntimeContainerInstance{Slot: slot, PodUID: pod, ContainerID: containerID}
	if digest != "" {
		image, err := domain.NewContainerImageRevision(slot, digest)
		if err != nil {
			t.Fatal(err)
		}
		instance.ImageRevision = &image
	}
	result := ObservationTarget{Workload: workload, Revision: domain.WorkloadRevision{Value: revision}, HasRevision: revision != "", Instance: instance}
	return result
}

func TestCompareObservationTargetsPodReplacementContinues(t *testing.T) {
	old := targetSnapshot(t, "pod-a", "container-a", "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "1")
	next := targetSnapshot(t, "pod-b", "container-b", "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "1")
	decision, err := CompareObservationTargets([]ObservationTarget{old}, []ObservationTarget{next}, time.Now().UTC())
	if err != nil || decision.Stop || len(decision.Events) != 2 {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
}

func TestCompareObservationTargetsRevisionStops(t *testing.T) {
	old := targetSnapshot(t, "pod", "container", "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "1")
	next := targetSnapshot(t, "pod", "container", "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "2")
	decision, err := CompareObservationTargets([]ObservationTarget{old}, []ObservationTarget{next}, time.Now().UTC())
	if err != nil || !decision.Stop || decision.Reason != domain.TargetRevisionChanged {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
}

func TestCompareObservationTargetsImageChangeStops(t *testing.T) {
	old := targetSnapshot(t, "pod", "container", "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "1")
	next := targetSnapshot(t, "pod", "container", "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", "1")
	decision, err := CompareObservationTargets([]ObservationTarget{old}, []ObservationTarget{next}, time.Now().UTC())
	if err != nil || !decision.Stop || decision.Reason != domain.TargetRevisionChanged {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
}
