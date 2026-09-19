package authz

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProjectionCoalescerSameKeyRunsOnce(t *testing.T) {
	c := NewProjectionCoalescer()
	var executions atomic.Int32
	start := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	call := func() {
		defer wg.Done()
		_, _, err := c.Do(context.Background(), "same", func(context.Context) (map[Capability]bool, error) {
			executions.Add(1)
			close(start)
			<-release
			return map[Capability]bool{WorkloadView: true}, nil
		})
		if err != nil {
			t.Errorf("coalesced call: %v", err)
		}
	}
	wg.Add(1)
	go call()
	<-start
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go call()
	}
	for {
		c.mu.Lock()
		waiters := c.flights["same"].waiters
		c.mu.Unlock()
		if waiters == 9 {
			break
		}
		runtime.Gosched()
	}
	close(release)
	wg.Wait()
	if got := executions.Load(); got != 1 {
		t.Fatalf("executions=%d, want 1", got)
	}
}

func TestProjectionCoalescerSeparatesAuthorityKeys(t *testing.T) {
	c := NewProjectionCoalescer()
	var executions atomic.Int32
	var wg sync.WaitGroup
	for _, key := range []string{"user-a/payments", "user-b/payments", "user-a/security", "user-a/payments/v2", "user-a/payments/groups-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _, err := c.Do(context.Background(), key, func(context.Context) (map[Capability]bool, error) {
				executions.Add(1)
				return map[Capability]bool{WorkloadView: true}, nil
			})
			if err != nil {
				t.Errorf("coalesced call: %v", err)
			}
		}(key)
	}
	wg.Wait()
	if got := executions.Load(); got != 5 {
		t.Fatalf("executions=%d, want separate authority keys", got)
	}
}

func TestProjectionCoalescerPropagatesFailureToWaiters(t *testing.T) {
	c := NewProjectionCoalescer()
	waiting := make(chan struct{})
	release := make(chan struct{})
	want := errors.New("denied")
	go func() {
		_, _, _ = c.Do(context.Background(), "failure", func(context.Context) (map[Capability]bool, error) {
			close(waiting)
			<-release
			return nil, want
		})
	}()
	<-waiting
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resultCh := make(chan struct {
		result    map[Capability]bool
		coalesced bool
		err       error
	}, 1)
	go func() {
		result, coalesced, err := c.Do(ctx, "failure", func(context.Context) (map[Capability]bool, error) {
			return map[Capability]bool{WorkloadView: true}, nil
		})
		resultCh <- struct {
			result    map[Capability]bool
			coalesced bool
			err       error
		}{result, coalesced, err}
	}()
	for {
		c.mu.Lock()
		waiters := c.flights["failure"].waiters
		c.mu.Unlock()
		if waiters == 1 {
			break
		}
		runtime.Gosched()
	}
	close(release)
	result := <-resultCh
	if !errors.Is(result.err, want) || !result.coalesced || result.result != nil {
		t.Fatalf("waiter result=%v coalesced=%v err=%v", result.result, result.coalesced, result.err)
	}
}

func TestProjectionCoalescerWaiterCancellationDoesNotCancelLeader(t *testing.T) {
	c := NewProjectionCoalescer()
	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, _, _ = c.Do(context.Background(), "cancel", func(context.Context) (map[Capability]bool, error) {
			close(started)
			<-release
			return map[Capability]bool{WorkloadView: true}, nil
		})
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, coalesced, err := c.Do(ctx, "cancel", func(context.Context) (map[Capability]bool, error) { return nil, nil }); !coalesced || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter result coalesced=%v err=%v", coalesced, err)
	}
	close(release)
}

func TestProjectionCoalescerEvictsCompletedFlightsAndClonesResults(t *testing.T) {
	c := NewProjectionCoalescer()
	var executions atomic.Int32

	result, coalesced, err := c.Do(context.Background(), "evict", func(context.Context) (map[Capability]bool, error) {
		executions.Add(1)
		return map[Capability]bool{WorkloadView: true}, nil
	})
	if err != nil || coalesced || !result[WorkloadView] {
		t.Fatalf("first projection result=%v coalesced=%v err=%v", result, coalesced, err)
	}
	result[WorkloadView] = false

	second, coalesced, err := c.Do(context.Background(), "evict", func(context.Context) (map[Capability]bool, error) {
		executions.Add(1)
		return map[Capability]bool{WorkloadView: true}, nil
	})
	if err != nil || coalesced || !second[WorkloadView] {
		t.Fatalf("second projection result=%v coalesced=%v err=%v", second, coalesced, err)
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("executions=%d, want completed flight eviction", got)
	}
}

func TestProjectionCoalescerEvictsFailedFlights(t *testing.T) {
	c := NewProjectionCoalescer()
	want := errors.New("authorization unavailable")
	var executions atomic.Int32

	_, _, err := c.Do(context.Background(), "failed-evict", func(context.Context) (map[Capability]bool, error) {
		executions.Add(1)
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("first error=%v, want %v", err, want)
	}
	result, coalesced, err := c.Do(context.Background(), "failed-evict", func(context.Context) (map[Capability]bool, error) {
		executions.Add(1)
		return map[Capability]bool{WorkloadView: false}, nil
	})
	if err != nil || coalesced || result[WorkloadView] {
		t.Fatalf("second projection result=%v coalesced=%v err=%v", result, coalesced, err)
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("executions=%d, want failed flight eviction", got)
	}
}
