package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
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

type runnerFailureStore struct {
	observation domain.Observation
	rv          string
	failUpdate  int
	updates     int
	failRunning bool
}

func (s *runnerFailureStore) ClaimObservation(_ context.Context, _ string, name string, executorID string) (obskube.ExecutorClaim, string, error) {
	if err := s.observation.Transition(domain.ExecutionStarting, ""); err != nil {
		return obskube.ExecutorClaim{}, "", err
	}
	return obskube.ExecutorClaim{ObservationID: domain.ObservationID(name), ExecutorID: executorID, ClaimGeneration: 1}, s.rv, nil
}

func (s *runnerFailureStore) GetObservation(context.Context, string, string) (domain.Observation, string, error) {
	return s.observation, s.rv, nil
}

func (s *runnerFailureStore) UpdateExecutorStatus(_ context.Context, _ string, _ obskube.ExecutorClaim, _ string, observation domain.Observation) (string, error) {
	s.updates++
	if s.failUpdate == s.updates {
		return "", errors.New("injected status persistence failure")
	}
	s.observation = observation
	return s.rv, nil
}

func (s *runnerFailureStore) TransitionExecution(_ context.Context, _ string, _ obskube.ExecutorClaim, _ string, next domain.ExecutionState, reason domain.CompletionReason) (string, error) {
	if s.failRunning && next == domain.ExecutionRunning {
		return "", errors.New("injected running transition failure")
	}
	if err := s.observation.Transition(next, reason); err != nil {
		return "", err
	}
	return s.rv, nil
}

type blockingFilesystemSource struct {
	started chan struct{}
	exited  chan struct{}
}

func (s *blockingFilesystemSource) Run(ctx context.Context, _ tracer.Options, attached func(error), _ func(tracer.Event, tracer.RuntimeIdentity)) error {
	close(s.started)
	attached(nil)
	<-ctx.Done()
	close(s.exited)
	return ctx.Err()
}

type stopImmediatelyMonitor struct{ sourceStarted <-chan struct{} }

func (m stopImmediatelyMonitor) Watch(ctx context.Context, _ []k8s.ObservationTarget, emit func(k8s.TargetChangeDecision)) error {
	select {
	case <-m.sourceStarted:
		emit(k8s.TargetChangeDecision{Stop: true, Reason: domain.StoppedByRequest})
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func runnerFixture(t *testing.T) (domain.Observation, *fake.Clientset, domain.ClusterIdentity) {
	t.Helper()
	cluster, err := domain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	uid := k8stypes.UID("workload-uid")
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", UID: uid}}
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-rs", Namespace: "default", UID: k8stypes.UID("rs-uid"), Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: uid, Controller: boolPtr(true)}}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "default", UID: k8stypes.UID("pod-uid"), OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-rs", UID: rs.UID, Controller: boolPtr(true)}}, Labels: map[string]string{}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "backend", ContainerID: "containerd://container-id", ImageID: "docker.io/library/busybox@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}}
	client := fake.NewSimpleClientset(deployment, rs, pod)
	workload := domain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: domain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: string(uid)}
	spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: domain.ContainerSlot{Workload: workload, Container: "backend"}}, []string{FilesystemSourceName}, 30*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domain.NewObservation(domain.ObservationID("runner-observation"), spec)
	if err != nil {
		t.Fatal(err)
	}
	return observation, client, cluster
}

func boolPtr(value bool) *bool { return &value }

func TestRunnerBindingPersistenceFailurePreventsCollectorLaunch(t *testing.T) {
	observation, client, cluster := runnerFixture(t)
	store := &runnerFailureStore{observation: observation, rv: "1", failUpdate: 1}
	source := &blockingFilesystemSource{started: make(chan struct{}), exited: make(chan struct{})}
	runner := &Runner{Store: store, Client: client, Cluster: cluster, Source: source}
	err := runner.Run(context.Background(), "default", "runner-observation", "executor-test")
	if err == nil || sourceStarted(source.started) {
		t.Fatalf("error=%v sourceStarted=%v", err, sourceStarted(source.started))
	}
}

func sourceStarted(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestRunnerRunningTransitionFailureStopsCollectorBeforeReturn(t *testing.T) {
	observation, client, cluster := runnerFixture(t)
	store := &runnerFailureStore{observation: observation, rv: "1", failRunning: true}
	source := &blockingFilesystemSource{started: make(chan struct{}), exited: make(chan struct{})}
	runner := &Runner{Store: store, Client: client, Cluster: cluster, Source: source}
	err := runner.Run(context.Background(), "default", "runner-observation", "executor-test")
	if err == nil {
		t.Fatal("expected RUNNING transition failure")
	}
	select {
	case <-source.exited:
	default:
		t.Fatal("collector was still active when Run returned")
	}
}

func TestRunnerResultPersistenceFailureStopsCollectorBeforeReturn(t *testing.T) {
	observation, client, cluster := runnerFixture(t)
	store := &runnerFailureStore{observation: observation, rv: "1", failUpdate: 2}
	source := &blockingFilesystemSource{started: make(chan struct{}), exited: make(chan struct{})}
	runner := &Runner{Store: store, Client: client, Cluster: cluster, Source: source, Monitor: stopImmediatelyMonitor{sourceStarted: source.started}}
	err := runner.Run(context.Background(), "default", "runner-observation", "executor-test")
	if err == nil {
		t.Fatal("expected result persistence failure")
	}
	select {
	case <-source.exited:
	default:
		t.Fatal("collector was still active when Run returned")
	}
	if store.observation.Execution().State == domain.ExecutionCompleted {
		t.Fatal("persistence failure fabricated terminal success")
	}
}
