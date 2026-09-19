package authz

import (
	"context"
	"errors"
	"sync"

	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
)

// ProjectionCoalescer suppresses duplicate in-flight capability projections.
// The caller supplies an authority-complete key; this type never derives or
// broadens authority and never retains a completed result.
type ProjectionCoalescer struct {
	mu      sync.Mutex
	flights map[string]*projectionFlight
}

type projectionFlight struct {
	done         chan struct{}
	capabilities map[Capability]bool
	err          error
	waiters      int
}

func NewProjectionCoalescer() *ProjectionCoalescer {
	return &ProjectionCoalescer{flights: make(map[string]*projectionFlight)}
}

// Do runs fn once for a key while it is in flight. Waiters may cancel their
// own wait without cancelling the computation used by other callers. The
// computation inherits the leader's deadline, but not its cancellation, so a
// disconnected leader cannot poison a still-valid waiter.
func (c *ProjectionCoalescer) Do(ctx context.Context, key string, fn func(context.Context) (map[Capability]bool, error)) (map[Capability]bool, bool, error) {
	if fn == nil {
		return nil, false, errors.New("authorization projection function is required")
	}
	if c == nil || key == "" {
		result, err := fn(ctx)
		return cloneCapabilities(result), false, err
	}

	c.mu.Lock()
	if existing, ok := c.flights[key]; ok {
		existing.waiters++
		c.mu.Unlock()
		if stats := observability.RequestStatsFromContext(ctx); stats != nil {
			stats.AddAuthorizationProjectionCoalesced()
		}
		select {
		case <-existing.done:
			return cloneCapabilities(existing.capabilities), true, existing.err
		case <-ctx.Done():
			return nil, true, ctx.Err()
		}
	}
	flight := &projectionFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	if stats := observability.RequestStatsFromContext(ctx); stats != nil {
		stats.AddAuthorizationProjectionExecution()
	}
	computationCtx := context.Background()
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		computationCtx, cancel = context.WithDeadline(computationCtx, deadline)
		defer cancel()
	}
	computationCtx = observability.WithRequestStats(computationCtx, observability.RequestStatsFromContext(ctx))
	flight.capabilities, flight.err = fn(computationCtx)

	c.mu.Lock()
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
	return cloneCapabilities(flight.capabilities), false, flight.err
}

func cloneCapabilities(input map[Capability]bool) map[Capability]bool {
	if input == nil {
		return nil
	}
	output := make(map[Capability]bool, len(input))
	for capability, allowed := range input {
		output[capability] = allowed
	}
	return output
}
