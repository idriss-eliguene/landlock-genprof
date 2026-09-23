package kubernetes

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time                 { return c.now }
func (c *testClock) advance(duration time.Duration) { c.now = c.now.Add(duration) }

func testStore(t *testing.T) (*Store, *testClock, string) {
	t.Helper()
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	clock := &testClock{now: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)}
	store, err := NewStoreWithClock(client, clock)
	if err != nil {
		t.Fatal(err)
	}
	observation := testObservation(t)
	object, err := ToUnstructured(observation, "default")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resource(GVR).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	raw, err := client.Resource(GVR).Namespace("default").Get(context.Background(), string(observation.ID()), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw.SetResourceVersion("1")
	if _, err := client.Resource(GVR).Namespace("default").Update(context.Background(), raw, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	return store, clock, string(observation.ID())
}

func TestClaimAndStaleExecutorFreshResourceVersion(t *testing.T) {
	store, clock, name := testStore(t)
	ctx := context.Background()
	a, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	if a.ClaimGeneration != 1 {
		t.Fatalf("generation=%d, want 1", a.ClaimGeneration)
	}
	stale := a
	stale.ClaimGeneration = 2
	if _, err := store.RenewLease(ctx, "default", stale, rv); !errors.Is(err, ErrStaleExecutor) {
		t.Fatalf("fresh-rv stale generation error=%v, want ErrStaleExecutor", err)
	}
	clock.advance(DefaultLeaseDuration + time.Nanosecond)
	freshRV, err := store.TerminalizeExecutorLost(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if freshRV != rv {
		t.Fatalf("fake resourceVersion changed unexpectedly: %q vs %q", freshRV, rv)
	}
	_, _, err = store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RenewLease(ctx, "default", a, freshRV); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("post-recovery write error=%v, want ErrTerminalObservation", err)
	}
	current, err := store.client.Resource(GVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	claimRecord, err := claimFromObject(current)
	if err != nil {
		t.Fatal(err)
	}
	if claimRecord.ExecutorID != a.ExecutorID || claimRecord.Generation != a.ClaimGeneration {
		t.Fatalf("stale write changed persisted claim: %#v", claimRecord)
	}
	terminal, _, err := store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Execution().State != domain.ExecutionFailed || terminal.Execution().Completion != domain.ExecutorLost {
		t.Fatalf("recovery state=%#v", terminal.Execution())
	}
	if failure := terminal.Execution().Failure; failure == nil || failure.Stage != "EXECUTOR_LOSS" || failure.Code != "EXECUTOR_LOST" || failure.ExecutorID != "executor-a" || failure.ClaimGeneration != 1 {
		t.Fatalf("recovery failure=%#v", failure)
	}
}

func TestConcurrentStopRequestsHaveOneDurableIntent(t *testing.T) {
	store, _, name := testStore(t)
	ctx := context.Background()
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, requester := range []string{"operator-a", "operator-b"} {
		wg.Add(1)
		go func(requester string) {
			defer wg.Done()
			_, _, err := store.RequestStop(ctx, "default", name, StopIntentInput{Requester: requester, ContextVersion: 7})
			results <- err
		}(requester)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, ErrConcurrentConflict) {
			t.Fatalf("concurrent stop error=%v", err)
		}
	}
	observation, _, err := store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Execution().StopRequested() {
		t.Fatal("concurrent stop requests did not persist an intent")
	}
}

func TestStopAfterFinalizationBoundaryIsRejected(t *testing.T) {
	store, _, name := testStore(t)
	ctx := context.Background()
	claim, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	rv, err = store.TransitionExecution(ctx, "default", claim, rv, domain.ExecutionRunning, "")
	if err != nil {
		t.Fatal(err)
	}
	rv, err = store.TransitionExecution(ctx, "default", claim, rv, domain.ExecutionCompleting, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RequestStop(ctx, "default", name, StopIntentInput{Requester: "operator", ContextVersion: 7}); !errors.Is(err, ErrStopNotEligible) {
		t.Fatalf("finalization stop error=%v, want ErrStopNotEligible", err)
	}
}

func TestDurableStopIntentIsCASProtectedAndClaimFenced(t *testing.T) {
	store, clock, name := testStore(t)
	ctx := context.Background()
	claim, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	requested, rv, err := store.RequestStop(ctx, "default", name, StopIntentInput{Requester: "operator", ContextVersion: 7})
	if err != nil {
		t.Fatal(err)
	}
	if !requested.Execution().StopRequested() || rv == "" {
		t.Fatalf("stop intent not persisted: stop=%v rv=%q", requested.Execution().StopRequested(), rv)
	}
	if ok, err := store.StopRequestedForClaim(ctx, "default", claim); err != nil || !ok {
		t.Fatalf("current claim did not consume intent: ok=%v err=%v", ok, err)
	}
	stale := claim
	stale.ClaimGeneration++
	if ok, err := store.StopRequestedForClaim(ctx, "default", stale); !errors.Is(err, ErrStaleExecutor) || ok {
		t.Fatalf("stale claim result ok=%v err=%v", ok, err)
	}
	duplicate, duplicateRV, err := store.RequestStop(ctx, "default", name, StopIntentInput{Requester: "operator", ContextVersion: 7})
	if err != nil || !duplicate.Execution().StopRequested() || duplicateRV != rv {
		t.Fatalf("duplicate stop changed intent: obs=%v rv=%q err=%v", duplicate.Execution().StopRequested(), duplicateRV, err)
	}
	clock.advance(DefaultLeaseDuration + time.Nanosecond)
	if ok, err := store.StopRequestedForClaim(ctx, "default", claim); !errors.Is(err, ErrStaleExecutor) || ok {
		t.Fatalf("expired claim result ok=%v err=%v", ok, err)
	}
}

func TestDurableStopMayBeRequestedBeforeExecutorClaim(t *testing.T) {
	store, _, name := testStore(t)
	ctx := context.Background()
	requested, _, err := store.RequestStop(ctx, "default", name, StopIntentInput{Requester: "operator", ContextVersion: 7})
	if err != nil || !requested.Execution().StopRequested() {
		t.Fatalf("pre-claim stop: observation=%v err=%v", requested.Execution().StopRequested(), err)
	}
	claim, _, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := store.StopRequestedForClaim(ctx, "default", claim); err != nil || !ok {
		t.Fatalf("pre-claim intent was not consumable by the fenced claim: ok=%v err=%v", ok, err)
	}
}

func TestAuthorityMatrix(t *testing.T) {
	store, clock, name := testStore(t)
	ctx := context.Background()
	claim, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	wrongID := claim
	wrongID.ExecutorID = "executor-b"
	oldGeneration := claim
	oldGeneration.ClaimGeneration = 0
	for _, tc := range []struct {
		name  string
		claim ExecutorClaim
		want  error
	}{
		{"wrong executor", wrongID, ErrStaleExecutor},
		{"old generation", oldGeneration, ErrStaleExecutor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.RenewLease(ctx, "default", tc.claim, rv); !errors.Is(err, tc.want) {
				t.Fatalf("error=%v, want %v", err, tc.want)
			}
		})
	}
	if _, err := store.RenewLease(ctx, "default", claim, "stale-rv"); err != nil {
		t.Fatalf("same-owner stale rv error=%v, want recovery", err)
	}
	clock.advance(DefaultLeaseDuration + time.Nanosecond)
	if _, err := store.RenewLease(ctx, "default", claim, rv); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired lease error=%v, want ErrLeaseExpired", err)
	}
}

func TestRenewLeaseReloadsSameOwnerAfterStaleResourceVersion(t *testing.T) {
	store, _, name := testStore(t)
	ctx := context.Background()
	claim, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RenewLease(ctx, "default", claim, "stale-resource-version"); err != nil {
		t.Fatalf("conflict error=%v, want same-owner recovery", err)
	}
	current, _, err := store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if current.Execution().State != domain.ExecutionStarting || rv == "" {
		t.Fatalf("conflict path changed lifecycle or lost resourceVersion: state=%s rv=%q", current.Execution().State, rv)
	}
}

func TestRenewLeaseRecoversSameOwnerAfterStatusConflict(t *testing.T) {
	store, _, name := testStore(t)
	ctx := context.Background()
	claim, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RequestStop(ctx, "default", name, StopIntentInput{Requester: "operator", ContextVersion: 7}); err != nil {
		t.Fatal(err)
	}
	current, err := store.client.Resource(GVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current.SetResourceVersion("2")
	if _, err := store.client.Resource(GVR).Namespace("default").Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	// RequestStop is a legitimate same-owner mutation which advances the
	// resourceVersion after the runner captured rv. Renewal must reload and
	// revalidate the unchanged claim instead of treating this as ownership loss.
	renewedRV, err := store.RenewLease(ctx, "default", claim, rv)
	if err != nil {
		t.Fatalf("same-owner renewal conflict = %v, want recovery", err)
	}
	if renewedRV == "" {
		t.Fatal("renewal returned an empty resourceVersion")
	}
	observation, currentRV, err := store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if currentRV != renewedRV || !observation.Execution().StopRequested() {
		t.Fatalf("renewal lost authoritative stop intent: rv=%q renewed=%q stop=%v", currentRV, renewedRV, observation.Execution().StopRequested())
	}
}

func TestAllExecutorWritesRequireCurrentClaim(t *testing.T) {
	store, clock, name := testStore(t)
	ctx := context.Background()
	claimA, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(DefaultLeaseDuration + time.Nanosecond)
	_, err = store.TerminalizeExecutorLost(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	starting := testObservation(t)
	if err := starting.Transition(domain.ExecutionStarting, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateExecutorStatus(ctx, "default", claimA, rv, starting); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("stale result update error=%v", err)
	}
	if _, err := store.TransitionExecution(ctx, "default", claimA, rv, domain.ExecutionRunning, ""); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("stale transition error=%v", err)
	}
	if _, err := store.RenewLease(ctx, "default", claimA, rv); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("stale heartbeat error=%v", err)
	}
}

func TestLostExecutorRecoveryIsTerminalAndReturnsNoClaim(t *testing.T) {
	store, clock, name := testStore(t)
	ctx := context.Background()
	claim, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(DefaultLeaseDuration + time.Nanosecond)
	recoveryRV, err := store.TerminalizeExecutorLost(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if recoveryRV != rv {
		t.Fatalf("recovery resourceVersion=%q, want %q", recoveryRV, rv)
	}
	observation, _, err := store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Execution().State != domain.ExecutionFailed || observation.Execution().Completion != domain.ExecutorLost || !observation.Frozen() {
		t.Fatalf("unexpected recovery result: %#v frozen=%v", observation.Execution(), observation.Frozen())
	}
	if _, err := store.RenewLease(ctx, "default", claim, recoveryRV); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("post-recovery heartbeat error=%v", err)
	}
	if _, err := store.TransitionExecution(ctx, "default", claim, recoveryRV, domain.ExecutionRunning, ""); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("post-recovery transition error=%v", err)
	}
	if _, err := store.UpdateExecutorStatus(ctx, "default", claim, recoveryRV, observation); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("post-recovery result error=%v", err)
	}
}

func TestLostExecutorRecoveryEligibility(t *testing.T) {
	store, clock, name := testStore(t)
	ctx := context.Background()
	_, _, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TerminalizeExecutorLost(ctx, "default", name); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("non-expired recovery error=%v", err)
	}
	clock.advance(DefaultLeaseDuration + time.Nanosecond)
	if _, err := store.TerminalizeExecutorLost(ctx, "default", name); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TerminalizeExecutorLost(ctx, "default", name); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("terminal recovery error=%v", err)
	}
}

func TestTerminalExecutorWritesAreDenied(t *testing.T) {
	store, _, name := testStore(t)
	ctx := context.Background()
	claim, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	rv, err = store.TransitionExecution(ctx, "default", claim, rv, domain.ExecutionRunning, "")
	if err != nil {
		t.Fatal(err)
	}
	rv, err = store.TransitionExecution(ctx, "default", claim, rv, domain.ExecutionCompleting, "")
	if err != nil {
		t.Fatal(err)
	}
	rv, err = store.TransitionExecution(ctx, "default", claim, rv, domain.ExecutionCompleted, domain.CompletedNormally)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RenewLease(ctx, "default", claim, rv); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("terminal heartbeat error=%v", err)
	}
	terminal, _, err := store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateExecutorStatus(ctx, "default", claim, rv, terminal); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("terminal result update error=%v", err)
	}
	if _, err := store.TransitionExecution(ctx, "default", claim, rv, domain.ExecutionFailed, domain.BackendFailure); !errors.Is(err, ErrTerminalObservation) {
		t.Fatalf("terminal transition error=%v", err)
	}
}

func TestExecutorIDIsOpaque(t *testing.T) {
	one, err := NewExecutorID()
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewExecutorID()
	if err != nil {
		t.Fatal(err)
	}
	if one == two || len(one) < 10 || len(two) < 10 {
		t.Fatalf("executor IDs are not opaque/unique: %q %q", one, two)
	}
}
