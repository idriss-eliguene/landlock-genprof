package runtime

import (
	"context"
	"errors"
	"sync"
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

func TestRemainingSourceAttributionAndExclusion(t *testing.T) {
	target := testRuntimeTarget(t)
	at := time.Now().UTC()
	for _, source := range []struct {
		name  string
		event tracer.Event
	}{
		{name: "exec", event: tracer.Event{Timestamp: at, Syscall: "execve", Mode: "exec"}},
		{name: "networkConnect", event: tracer.Event{Timestamp: at, Syscall: "connect", Mode: "egress", Port: 443}},
		{name: "networkBind", event: tracer.Event{Timestamp: at, Syscall: "bind", Mode: "ingress", Port: 8080}},
		{name: "capabilities", event: tracer.Event{Timestamp: at, Syscall: "CAP_NET_RAW", Mode: "capability"}},
	} {
		matched := AttributeFilesystemEvent(source.event, tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "container-uid"}, []domain.RuntimeContainerInstance{target}, at.Add(-time.Second), at.Add(time.Second))
		if !matched.Found {
			t.Fatalf("%s attribution = %#v", source.name, matched)
		}
		excluded := AttributeFilesystemEvent(source.event, tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "unknown"}, []domain.RuntimeContainerInstance{target}, at.Add(-time.Second), at.Add(time.Second))
		if excluded.Found {
			t.Fatalf("%s exclusion unexpectedly attributed", source.name)
		}
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

func TestNormalizedFactsBySource(t *testing.T) {
	target := testRuntimeTarget(t)
	at := time.Now().UTC()
	cases := []struct {
		name  string
		event tracer.Event
		want  int
	}{
		{"filesystem", testFilesystemEvent(at), 1},
		{"exec", tracer.Event{Timestamp: at, Syscall: "execve", Path: "/bin/sh", Mode: "exec"}, 1},
		{"networkConnect", tracer.Event{Timestamp: at, Syscall: "connect", Port: 443, Mode: "egress"}, 1},
		{"networkBind", tracer.Event{Timestamp: at, Syscall: "bind", Port: 8080, Mode: "ingress"}, 1},
		{"capabilities", tracer.Event{Timestamp: at, Syscall: "CAP_NET_RAW", Mode: "capability"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acc := NewFilesystemAccumulator([]domain.RuntimeContainerInstance{target}, at.Add(-time.Second))
			acc.AddFor(tc.name, tc.event, tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "container-uid"})
			acc.AddFor(tc.name, tc.event, tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "container-uid"})
			result, err := acc.SourceResultFor(tc.name, "gadget", "v1", true, true, false, domain.AttributionCompleted)
			if err != nil {
				t.Fatal(err)
			}
			if result.Qualification.AttributedCount != 2 || result.Facts.Count() != tc.want {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

type runnerFailureStore struct {
	observation domain.Observation
	rv          string
	failUpdate  int
	updates     int
	failRunning bool
	mu          sync.Mutex
	renewals    int
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

func (s *runnerFailureStore) RenewLease(_ context.Context, _ string, _ obskube.ExecutorClaim, expectedRV string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewals++
	return expectedRV, nil
}

func (s *runnerFailureStore) renewalCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.renewals
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

type silentStartupSource struct{}

func (silentStartupSource) SourceName() string     { return FilesystemSourceName }
func (silentStartupSource) BackendName() string    { return FilesystemBackend }
func (silentStartupSource) BackendVersion() string { return FilesystemVersion }
func (silentStartupSource) Run(ctx context.Context, _ tracer.Options, _ func(error), _ func(tracer.Event, tracer.RuntimeIdentity)) error {
	<-ctx.Done()
	return ctx.Err()
}

type gatedObservationSource struct {
	name    string
	backend string
	gate    <-chan struct{}
	started chan struct{}
}

func (s gatedObservationSource) SourceName() string     { return s.name }
func (s gatedObservationSource) BackendName() string    { return s.backend }
func (s gatedObservationSource) BackendVersion() string { return FilesystemVersion }
func (s gatedObservationSource) Run(ctx context.Context, _ tracer.Options, attached func(error), emit func(tracer.Event, tracer.RuntimeIdentity)) error {
	close(s.started)
	attached(nil)
	select {
	case <-s.gate:
		event := tracer.Event{Timestamp: time.Now().UTC(), Syscall: s.name, Mode: "runtime"}
		emit(event, tracer.RuntimeIdentity{Namespace: "default", Container: "backend", ContainerID: "container-uid"})
	case <-ctx.Done():
		return ctx.Err()
	}
	<-ctx.Done()
	return ctx.Err()
}

type gatedStopMonitor struct {
	started <-chan struct{}
	gate    chan struct{}
}

func (m gatedStopMonitor) Watch(ctx context.Context, _ []k8s.ObservationTarget, emit func(k8s.TargetChangeDecision)) error {
	select {
	case <-m.started:
		close(m.gate)
		emit(k8s.TargetChangeDecision{Stop: true, Reason: domain.StoppedByRequest})
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

type delayedCancellationSource struct {
	started chan struct{}
	release <-chan struct{}
}

func (s delayedCancellationSource) SourceName() string     { return FilesystemSourceName }
func (s delayedCancellationSource) BackendName() string    { return FilesystemBackend }
func (s delayedCancellationSource) BackendVersion() string { return FilesystemVersion }
func (s delayedCancellationSource) Run(ctx context.Context, _ tracer.Options, attached func(error), _ func(tracer.Event, tracer.RuntimeIdentity)) error {
	close(s.started)
	attached(nil)
	<-ctx.Done()
	<-s.release
	return ctx.Err()
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

func TestRunnerMissingAttachmentCallbackFailsBoundedly(t *testing.T) {
	observation, client, cluster := runnerFixture(t)
	store := &runnerFailureStore{observation: observation, rv: "1"}
	runner := &Runner{Store: store, Client: client, Cluster: cluster, Source: silentStartupSource{}, Lease: 20 * time.Millisecond}
	started := time.Now()
	err := runner.Run(context.Background(), "default", "runner-observation", "executor-test")
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("missing callback result=%v elapsed=%s", err, time.Since(started))
	}
	if got := store.observation.Execution().State; got != domain.ExecutionFailed {
		t.Fatalf("execution state = %s, want FAILED", got)
	}
}

func TestRunnerDurableStopDuringAttachmentFinalizesWithoutFabricatingEvidence(t *testing.T) {
	observation, client, cluster := runnerFixture(t)
	store := &runnerFailureStore{observation: observation, rv: "1"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopRequested := false
	runner := &Runner{
		Store: store, Client: client, Cluster: cluster, Source: silentStartupSource{},
		Lease:         200 * time.Millisecond,
		OnClaim:       func(obskube.ExecutorClaim) { stopRequested = true; cancel() },
		StopRequested: func() bool { return stopRequested },
	}
	if err := runner.Run(ctx, "default", "runner-observation", "executor-test"); err != nil {
		t.Fatal(err)
	}
	if got := store.observation.Execution().State; got != domain.ExecutionCompleted {
		t.Fatalf("execution state = %s, want COMPLETED", got)
	}
	if got := store.observation.Execution().Completion; got != domain.StoppedByRequest {
		t.Fatalf("completion = %s, want %s", got, domain.StoppedByRequest)
	}
	if !store.observation.Frozen() {
		t.Fatal("stopped observation was not frozen")
	}
	if len(store.observation.Result().Sources()) != 0 {
		t.Fatal("early stop fabricated source evidence")
	}
}

func TestRunnerKeepsLeaseAliveThroughStopDrainBeforeFinalization(t *testing.T) {
	observation, client, cluster := runnerFixture(t)
	store := &runnerFailureStore{observation: observation, rv: "1"}
	started := make(chan struct{})
	release := make(chan struct{})
	source := delayedCancellationSource{started: started, release: release}
	runner := &Runner{
		Store: store, Client: client, Cluster: cluster, Source: source,
		Monitor:            stopImmediatelyMonitor{sourceStarted: started},
		LeaseRenewInterval: 10 * time.Millisecond,
	}
	done := make(chan error, 1)
	go func() { done <- runner.Run(context.Background(), "default", "runner-observation", "executor-test") }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("source did not start")
	}
	deadline := time.After(500 * time.Millisecond)
	for store.renewalCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("lease renewal stopped before source drain completed")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not finalize after source drain")
	}
	if got := store.observation.Execution().State; got != domain.ExecutionCompleted {
		t.Fatalf("execution state = %s, want COMPLETED", got)
	}
	if !store.observation.Frozen() {
		t.Fatal("stopped observation was not frozen")
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

func TestRunnerMultiSourceKeepsIndependentResults(t *testing.T) {
	observation, client, cluster := runnerFixture(t)
	gate := make(chan struct{})
	started := make(chan struct{})
	store := &runnerFailureStore{observation: observation, rv: "1"}
	sources := []FilesystemSource{
		gatedObservationSource{name: "exec", backend: "trace_exec", gate: gate, started: started},
		gatedObservationSource{name: "networkConnect", backend: "trace_tcp", gate: gate, started: make(chan struct{})},
	}
	runner := &Runner{Store: store, Client: client, Cluster: cluster, Sources: sources, Monitor: gatedStopMonitor{started: started, gate: gate}}
	if err := runner.Run(context.Background(), "default", "runner-observation", "executor-test"); err != nil {
		t.Fatal(err)
	}
	if got := len(store.observation.Result().Sources()); got != 2 {
		t.Fatalf("source result count = %d", got)
	}
	seen := map[string]bool{}
	for _, result := range store.observation.Result().Sources() {
		seen[result.Source.Name] = true
	}
	if !seen["exec"] || !seen["networkConnect"] {
		t.Fatalf("independent results = %#v", store.observation.Result().Sources())
	}
}
