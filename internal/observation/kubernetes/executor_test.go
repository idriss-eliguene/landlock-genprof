package kubernetes

import (
	"context"
	"errors"
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
	clock.advance(DefaultLeaseDuration + time.Nanosecond)
	b, freshRV, err := store.ReclaimExpired(ctx, "default", name, "executor-b")
	if err != nil {
		t.Fatal(err)
	}
	if b.ClaimGeneration <= a.ClaimGeneration {
		t.Fatalf("generation=%d did not exceed %d", b.ClaimGeneration, a.ClaimGeneration)
	}
	if freshRV != rv {
		t.Fatalf("fake resourceVersion changed unexpectedly: %q vs %q", freshRV, rv)
	}
	_, _, err = store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RenewLease(ctx, "default", a, freshRV); !errors.Is(err, ErrStaleExecutor) {
		t.Fatalf("fresh-rv stale write error=%v, want ErrStaleExecutor", err)
	}
	current, err := store.client.Resource(GVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	claimRecord, err := claimFromObject(current)
	if err != nil {
		t.Fatal(err)
	}
	if claimRecord.ExecutorID != b.ExecutorID || claimRecord.Generation != b.ClaimGeneration {
		t.Fatalf("stale write changed persisted claim: %#v", claimRecord)
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
	if _, err := store.RenewLease(ctx, "default", claim, "stale-rv"); !errors.Is(err, ErrConcurrentConflict) {
		t.Fatalf("stale rv error=%v, want conflict", err)
	}
	clock.advance(DefaultLeaseDuration + time.Nanosecond)
	if _, err := store.RenewLease(ctx, "default", claim, rv); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired lease error=%v, want ErrLeaseExpired", err)
	}
}

func TestConflictDoesNotRetryAnExecutorMutation(t *testing.T) {
	store, _, name := testStore(t)
	ctx := context.Background()
	claim, rv, err := store.ClaimObservation(ctx, "default", name, "executor-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RenewLease(ctx, "default", claim, "stale-resource-version"); !errors.Is(err, ErrConcurrentConflict) {
		t.Fatalf("conflict error=%v, want ErrConcurrentConflict", err)
	}
	current, _, err := store.GetObservation(ctx, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if current.Execution().State != domain.ExecutionStarting || rv == "" {
		t.Fatalf("conflict path changed lifecycle or lost resourceVersion: state=%s rv=%q", current.Execution().State, rv)
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
	claimB, rv, err := store.ReclaimExpired(ctx, "default", name, "executor-b")
	if err != nil {
		t.Fatal(err)
	}
	starting := testObservation(t)
	if err := starting.Transition(domain.ExecutionStarting, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateExecutorStatus(ctx, "default", claimA, rv, starting); !errors.Is(err, ErrStaleExecutor) {
		t.Fatalf("stale result update error=%v", err)
	}
	if _, err := store.TransitionExecution(ctx, "default", claimA, rv, domain.ExecutionRunning, ""); !errors.Is(err, ErrStaleExecutor) {
		t.Fatalf("stale transition error=%v", err)
	}
	if _, err := store.RenewLease(ctx, "default", claimA, rv); !errors.Is(err, ErrStaleExecutor) {
		t.Fatalf("stale heartbeat error=%v", err)
	}
	if _, err := store.TransitionExecution(ctx, "default", claimB, rv, domain.ExecutionRunning, ""); err != nil {
		t.Fatalf("current owner transition failed: %v", err)
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
