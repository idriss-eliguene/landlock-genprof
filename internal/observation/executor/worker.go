// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package executor owns the process that consumes durable Observation work.
// It deliberately contains no governance operation and uses the fenced
// Observation store for every executor-authored mutation.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	observationruntime "github.com/idriss-eliguene/landlock-genprof/internal/observation/runtime"
	"github.com/idriss-eliguene/landlock-genprof/internal/observationapp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	DefaultPollInterval = time.Second
	DefaultShutdownWait = 5 * time.Second
)

type Config struct {
	Core         kubernetes.Interface
	Dynamic      dynamic.Interface
	Rest         *rest.Config
	Namespaces   []string
	Binary       string
	PollInterval time.Duration
	ShutdownWait time.Duration
	Logger       *observability.Logger
	Metrics      *observability.Metrics
}

func (c Config) validate() error {
	if c.Core == nil || c.Dynamic == nil || c.Rest == nil {
		return errors.New("observation executor requires Kubernetes clients")
	}
	if len(c.Namespaces) == 0 {
		return errors.New("observation executor requires at least one target namespace")
	}
	for _, namespace := range c.Namespaces {
		if strings.TrimSpace(namespace) == "" {
			return errors.New("observation executor target namespace must not be empty")
		}
	}
	return nil
}

// Run processes at most one observation at a time. Kubernetes CAS prevents a
// second executor from taking the same observation, while the process-level
// bound keeps the Gadget/resource envelope explicit for v0.8.
func Run(ctx context.Context, config Config) error {
	if err := config.validate(); err != nil {
		return err
	}
	interval := config.PollInterval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	shutdownWait := config.ShutdownWait
	if shutdownWait <= 0 {
		shutdownWait = DefaultShutdownWait
	}
	logger := config.Logger
	if logger == nil {
		logger, _ = observability.NewLogger(io.Discard, observability.DefaultLogLevel)
	}
	metrics := config.Metrics
	if metrics == nil {
		metrics = observability.NewMetrics()
	}
	logger.Info("executor_startup", map[string]interface{}{"component": "observation_executor", "namespaces": len(config.Namespaces)})
	metrics.ExecutorActive(0)
	store, err := obskube.NewStore(config.Dynamic)
	if err != nil {
		return err
	}
	executorID, err := obskube.NewExecutorID()
	if err != nil {
		return err
	}

	var activeCancel context.CancelFunc
	var activeDone <-chan error
	activeObservation := ""
	for {
		if activeDone == nil {
			if err := recoverExpired(ctx, store, config.Namespaces, logger, metrics); err != nil && ctx.Err() == nil {
				return err
			}
			for _, namespace := range config.Namespaces {
				observation, found, scanErr := nextRequested(ctx, store, namespace)
				if scanErr != nil {
					if ctx.Err() != nil {
						return nil
					}
					return scanErr
				}
				if !found {
					continue
				}
				if err := start(ctx, config, store, observation, namespace, executorID, &activeCancel, &activeDone, logger, metrics); err != nil {
					return err
				}
				activeObservation = string(observation.ID())
				break
			}
		}

		select {
		case <-ctx.Done():
			if activeCancel != nil {
				activeCancel()
				wait := time.NewTimer(shutdownWait)
				select {
				case <-activeDone:
					wait.Stop()
				case <-wait.C:
					// The durable lease expires and the next executor scan
					// terminalizes this work as EXECUTOR_LOST.
				}
			}
			logger.Info("executor_shutdown", map[string]interface{}{"component": "observation_executor"})
			return nil
		case err := <-activeDone:
			activeCancel = nil
			activeDone = nil
			metrics.ExecutorActive(0)
			if err == nil {
				metrics.ExecutorCompleted()
				logger.Info("observation_completed", map[string]interface{}{"component": "observation_executor", "observation": activeObservation})
			} else {
				if strings.Contains(strings.ToLower(err.Error()), "conflict") || strings.Contains(strings.ToLower(err.Error()), "already claimed") {
					metrics.ExecutorClaimConflict()
				}
				metrics.ExecutorFailed("RUN_ERROR")
				logger.Warn("observation_failed", map[string]interface{}{"component": "observation_executor", "observation": activeObservation, "reason": err})
			}
			activeObservation = ""
			if err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
				// Runner has already fenced all writes. A failed run is
				// observable through durable state; the worker remains alive.
				logger.Error("observation_failed", map[string]interface{}{"component": "observation_executor", "reason": err})
				metrics.ExecutorFailed("RUN_ERROR")
				continue
			}
		case <-time.After(interval):
		}
	}
}

func nextRequested(ctx context.Context, store *obskube.Store, namespace string) (observationdomain.Observation, bool, error) {
	items, err := store.ListObservations(ctx, namespace)
	if err != nil {
		return observationdomain.Observation{}, false, err
	}
	for _, observation := range items {
		if observation.Execution().State == observationdomain.ExecutionRequested {
			return observation, true, nil
		}
	}
	return observationdomain.Observation{}, false, nil
}

func recoverExpired(ctx context.Context, store *obskube.Store, namespaces []string, logger *observability.Logger, metrics *observability.Metrics) error {
	for _, namespace := range namespaces {
		items, err := store.ListObservations(ctx, namespace)
		if err != nil {
			return err
		}
		for _, observation := range items {
			state := observation.Execution().State
			if state == observationdomain.ExecutionCompleted || state == observationdomain.ExecutionFailed || state == observationdomain.ExecutionRequested {
				continue
			}
			_, err := store.TerminalizeExecutorLost(ctx, namespace, string(observation.ID()))
			if err == nil {
				logger.Warn("executor_lost", map[string]interface{}{"component": "observation_executor", "namespace": namespace, "observation": observation.ID()})
				metrics.ExecutorLost()
				metrics.ExecutorFailed("EXECUTOR_LOST")
			}
			if err != nil && !errors.Is(err, obskube.ErrLeaseExpired) && !errors.Is(err, obskube.ErrRecoveryNotEligible) && !errors.Is(err, obskube.ErrTerminalObservation) && !errors.Is(err, obskube.ErrConcurrentConflict) {
				return err
			}
		}
	}
	return nil
}

func start(parent context.Context, config Config, store *obskube.Store, observation observationdomain.Observation, namespace, executorID string, cancelOut *context.CancelFunc, doneOut *<-chan error, logger *observability.Logger, metrics *observability.Metrics) error {
	sources := make([]observationruntime.FilesystemSource, 0, len(observation.Spec().SourceNames()))
	for _, name := range observation.Spec().SourceNames() {
		source, err := observationapp.Source(name)
		if err != nil {
			return fmt.Errorf("observation %s has unsupported source: %w", observation.ID(), err)
		}
		sources = append(sources, source)
	}
	cluster, err := k8s.ResolveClusterIdentity(parent, config.Core)
	if err != nil {
		return fmt.Errorf("resolving executor cluster identity: %w", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	*cancelOut = cancel
	*doneOut = done
	claimCh := make(chan obskube.ExecutorClaim, 1)
	runner := &observationruntime.Runner{
		Store:          store,
		Client:         config.Core,
		ExecutorConfig: config.Rest,
		Cluster:        cluster,
		Sources:        sources,
		Monitor:        observationruntime.PollingTargetMonitor{Client: config.Core, Cluster: cluster, Target: observation.Spec().Target},
		Logger:         logger,
		Metrics:        metrics,
		OnClaim:        func(claim obskube.ExecutorClaim) { claimCh <- claim },
	}
	runner.Binary = config.Binary
	go func() {
		var durableStop atomic.Bool
		watchDone := make(chan struct{})
		go func() {
			select {
			case <-parent.Done():
				cancel()
			case <-watchDone:
			}
		}()
		stopDone := make(chan struct{})
		runner.StopRequested = durableStop.Load
		go watchDurableStop(runCtx, store, namespace, claimCh, func() { durableStop.Store(true); cancel() }, stopDone, config.PollInterval)
		done <- runner.Run(runCtx, namespace, string(observation.ID()), executorID)
		close(stopDone)
		close(watchDone)
	}()
	logger.Info("observation_execution_started", map[string]interface{}{"component": "observation_executor", "namespace": namespace, "observation": observation.ID(), "executorID": executorID})
	metrics.ExecutorClaim("started")
	metrics.ExecutorActive(1)
	return nil
}

// watchDurableStop is deliberately executor-local: the durable Observation
// contains intent, while only the executor holding the current fenced claim
// has the cancellation authority over its Runner context.
func watchDurableStop(ctx context.Context, store *obskube.Store, namespace string, claims <-chan obskube.ExecutorClaim, stop context.CancelFunc, done <-chan struct{}, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	var claim obskube.ExecutorClaim
	select {
	case claim = <-claims:
	case <-done:
		return
	case <-ctx.Done():
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		requested, err := store.StopRequestedForClaim(ctx, namespace, claim)
		if err == nil && requested {
			stop()
			return
		}
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
