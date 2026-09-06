// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package runtime contains the narrow, source-specific bridge between the
// Observation domain and supported runtime collectors. It owns attribution
// facts, but not Kubernetes serialization or policy generation.
package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
	k8sclient "k8s.io/client-go/kubernetes"
)

const (
	FilesystemSourceName = "filesystem"
	FilesystemBackend    = "trace_open"
	FilesystemVersion    = "v0.55.1"
	maxPositiveRefs      = 64
)

// SourceDescriptor is the closed, source-specific metadata seam for the
// supported runtime gadgets. It is intentionally not a plugin registry.
type SourceDescriptor interface {
	SourceName() string
	BackendName() string
	BackendVersion() string
}

// Attribution describes the outcome for one received runtime event.
type Attribution struct {
	Target domain.RuntimeContainerInstance
	Reason string
	Found  bool
}

// AttributeFilesystemEvent maps only strong runtime identity to a bound
// target. Names alone and missing runtime identity are never sufficient.
// RuntimeContainerInstance retains PodUID, but current trace_open attribution
// receives no PodUID and therefore depends specifically on runtime ContainerID
// plus available namespace/container constraints; no PodUID is fabricated.
func AttributeFilesystemEvent(event tracer.Event, identity tracer.RuntimeIdentity, targets []domain.RuntimeContainerInstance, start, end time.Time) Attribution {
	if event.Timestamp.IsZero() || event.Timestamp.Before(start) || !end.IsZero() && event.Timestamp.After(end) {
		return Attribution{Reason: "event outside qualified observation interval"}
	}
	if identity.ContainerID == "" && identity.PodUID == "" {
		return Attribution{Reason: "runtime identity unavailable"}
	}
	matches := make([]domain.RuntimeContainerInstance, 0, 1)
	for _, target := range targets {
		if identity.Namespace != "" && identity.Namespace != target.Slot.Workload.Namespace {
			continue
		}
		if identity.Container != "" && identity.Container != target.Slot.Container {
			continue
		}
		if identity.PodUID != "" && identity.PodUID != target.PodUID {
			continue
		}
		if identity.ContainerID != "" && (target.ContainerID == "" || identity.ContainerID != target.ContainerID) {
			continue
		}
		if identity.ImageDigest != "" && target.ImageRevision != nil && identity.ImageDigest != target.ImageRevision.ImageDigest {
			continue
		}
		matches = append(matches, target)
	}
	if len(matches) != 1 {
		if len(matches) == 0 {
			return Attribution{Reason: "runtime identity did not match a bound target"}
		}
		return Attribution{Reason: "runtime identity matched multiple bound targets"}
	}
	return Attribution{Target: matches[0], Found: true}
}

// FilesystemAccumulator retains bounded positive references and explicit
// exclusion accounting. It never stores raw event streams.
type FilesystemAccumulator struct {
	mu         sync.Mutex
	targets    []domain.RuntimeContainerInstance
	start      time.Time
	end        time.Time
	attributed uint64
	excluded   uint64
	references []string
}

func NewFilesystemAccumulator(targets []domain.RuntimeContainerInstance, start time.Time) *FilesystemAccumulator {
	return &FilesystemAccumulator{targets: append([]domain.RuntimeContainerInstance(nil), targets...), start: start}
}

func (a *FilesystemAccumulator) SetEnd(end time.Time) { a.mu.Lock(); a.end = end; a.mu.Unlock() }

func (a *FilesystemAccumulator) Add(event tracer.Event, identity tracer.RuntimeIdentity) Attribution {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := AttributeFilesystemEvent(event, identity, a.targets, a.start, a.end)
	if result.Found {
		a.attributed++
		if event.Path != "" && len(a.references) < maxPositiveRefs {
			a.references = append(a.references, event.Path)
		}
	} else {
		a.excluded++
	}
	return result
}

func (a *FilesystemAccumulator) Counts() (uint64, uint64, []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.attributed, a.excluded, append([]string(nil), a.references...)
}

// SourceResult derives EvidenceState through the canonical G2 function. The
// trace_open lifecycle currently supplies no drain/completeness proof, so the
// caller must pass flushConfirmed=false unless it has an actual proof signal.
func (a *FilesystemAccumulator) SourceResult(backendHealthy, attached, flushConfirmed bool, attribution domain.AttributionState) (domain.SourceResult, error) {
	return a.SourceResultFor(FilesystemSourceName, FilesystemBackend, FilesystemVersion, backendHealthy, attached, flushConfirmed, attribution)
}

func (a *FilesystemAccumulator) SourceResultFor(sourceName, backend, version string, backendHealthy, attached, flushConfirmed bool, attribution domain.AttributionState) (domain.SourceResult, error) {
	attributed, excluded, refs := a.Counts()
	return domain.NewSourceResult(domain.EvidenceSource{Name: sourceName, Backend: backend, Version: version}, domain.SourceQualification{
		BackendHealthConfirmed: backendHealthy, SourceAttachedForBoundWindow: attached, FlushConfirmed: flushConfirmed,
		Attribution: attribution, AttributedCount: attributed, ExcludedCount: excluded,
	}, refs)
}

// FilesystemSource is intentionally narrower than a generic plugin system.
type FilesystemSource interface {
	Run(context.Context, tracer.Options, func(error), func(tracer.Event, tracer.RuntimeIdentity)) error
}

type GadgetExecSource struct{}

func (GadgetExecSource) SourceName() string     { return "exec" }
func (GadgetExecSource) BackendName() string    { return "trace_exec" }
func (GadgetExecSource) BackendVersion() string { return FilesystemVersion }
func (GadgetExecSource) Run(ctx context.Context, opts tracer.Options, attached func(error), emit func(tracer.Event, tracer.RuntimeIdentity)) error {
	return tracer.TraceExecSourceWithIdentity(ctx, opts, attached, emit)
}

type GadgetNetworkConnectSource struct{}

func (GadgetNetworkConnectSource) SourceName() string     { return "networkConnect" }
func (GadgetNetworkConnectSource) BackendName() string    { return "trace_tcp" }
func (GadgetNetworkConnectSource) BackendVersion() string { return FilesystemVersion }
func (GadgetNetworkConnectSource) Run(ctx context.Context, opts tracer.Options, attached func(error), emit func(tracer.Event, tracer.RuntimeIdentity)) error {
	return tracer.TraceConnectSourceWithIdentity(ctx, opts, attached, emit)
}

type GadgetNetworkBindSource struct{}

func (GadgetNetworkBindSource) SourceName() string     { return "networkBind" }
func (GadgetNetworkBindSource) BackendName() string    { return "trace_bind" }
func (GadgetNetworkBindSource) BackendVersion() string { return FilesystemVersion }
func (GadgetNetworkBindSource) Run(ctx context.Context, opts tracer.Options, attached func(error), emit func(tracer.Event, tracer.RuntimeIdentity)) error {
	return tracer.TraceBindSourceWithIdentity(ctx, opts, attached, emit)
}

type GadgetCapabilitiesSource struct{}

func (GadgetCapabilitiesSource) SourceName() string     { return "capabilities" }
func (GadgetCapabilitiesSource) BackendName() string    { return "trace_capabilities" }
func (GadgetCapabilitiesSource) BackendVersion() string { return FilesystemVersion }
func (GadgetCapabilitiesSource) Run(ctx context.Context, opts tracer.Options, attached func(error), emit func(tracer.Event, tracer.RuntimeIdentity)) error {
	return tracer.TraceCapabilitiesSourceWithIdentity(ctx, opts, attached, emit)
}

// ObservationStore is the narrow persistence surface required by the
// filesystem runner. Keeping it local permits deterministic custody-failure
// tests without exposing or bypassing the Kubernetes Store's fenced methods.
type ObservationStore interface {
	ClaimObservation(context.Context, string, string, string) (obskube.ExecutorClaim, string, error)
	GetObservation(context.Context, string, string) (domain.Observation, string, error)
	UpdateExecutorStatus(context.Context, string, obskube.ExecutorClaim, string, domain.Observation) (string, error)
	TransitionExecution(context.Context, string, obskube.ExecutorClaim, string, domain.ExecutionState, domain.CompletionReason) (string, error)
}

// TargetMonitor emits facts only; the Runner persists them through G4.
type TargetMonitor interface {
	Watch(context.Context, []k8s.ObservationTarget, func(k8s.TargetChangeDecision)) error
}

// PollingTargetMonitor is the bounded relist fallback for the filesystem
// vertical and never mutates an Observation directly.
type PollingTargetMonitor struct {
	Client   k8sclient.Interface
	Cluster  domain.ClusterIdentity
	Target   domain.RequestedTarget
	Interval time.Duration
	Grace    time.Duration
}

func (m PollingTargetMonitor) Watch(ctx context.Context, initial []k8s.ObservationTarget, emit func(k8s.TargetChangeDecision)) error {
	interval := m.Interval
	if interval <= 0 {
		interval = time.Second
	}
	grace := m.Grace
	if grace <= 0 {
		grace = 2 * interval
	}
	previous := append([]k8s.ObservationTarget(nil), initial...)
	var absentSince time.Time
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case at := <-ticker.C:
			resolved, err := k8s.ResolveObservationTargets(ctx, m.Client, m.Cluster, m.Target)
			if err != nil && !errors.Is(err, k8s.ErrObservationTargetUnavailable) {
				continue
			}
			if errors.Is(err, k8s.ErrObservationTargetUnavailable) {
				if absentSince.IsZero() {
					absentSince = at
				}
				if at.Sub(absentSince) < grace {
					continue
				}
			} else {
				absentSince = time.Time{}
			}
			current := make([]k8s.ObservationTarget, 0, len(resolved))
			for _, target := range resolved {
				current = append(current, target.ObservationTarget)
			}
			decision, err := k8s.CompareObservationTargets(previous, current, at.UTC())
			if err != nil {
				return err
			}
			if len(decision.Events) > 0 || decision.Stop {
				emit(decision)
			}
			previous = current
		}
	}
}

type GadgetFilesystemSource struct{}

func (GadgetFilesystemSource) SourceName() string     { return FilesystemSourceName }
func (GadgetFilesystemSource) BackendName() string    { return FilesystemBackend }
func (GadgetFilesystemSource) BackendVersion() string { return FilesystemVersion }

func (GadgetFilesystemSource) Run(ctx context.Context, opts tracer.Options, attached func(error), emit func(tracer.Event, tracer.RuntimeIdentity)) error {
	var once sync.Once
	signal := func(err error) {
		once.Do(func() {
			if attached != nil {
				attached(err)
			}
		})
	}
	err := tracer.TraceFilesystemSourceWithIdentity(ctx, opts, signal, emit)
	if err != nil {
		signal(err)
	}
	return err
}

// Runner performs one bounded filesystem Observation using G4's fenced store
// operations. It is deliberately source-specific and does not expose a raw
// Kubernetes mutation escape hatch.
type Runner struct {
	Store   ObservationStore
	Client  k8sclient.Interface
	Cluster domain.ClusterIdentity
	Source  FilesystemSource
	Sources []FilesystemSource
	Monitor TargetMonitor
	Binary  string
	Lease   time.Duration
}

func (r *Runner) Run(ctx context.Context, namespace, name, executorID string) error {
	sources := r.Sources
	if len(sources) == 0 && r.Source != nil {
		sources = []FilesystemSource{r.Source}
	}
	if r.Store == nil || r.Client == nil || len(sources) == 0 {
		return errors.New("filesystem observation runner is incompletely configured")
	}
	claim, rv, err := r.Store.ClaimObservation(ctx, namespace, name, executorID)
	if err != nil {
		return err
	}
	observation, _, err := r.Store.GetObservation(ctx, namespace, name)
	if err != nil {
		return err
	}
	cluster := r.Cluster
	if cluster.NamespaceUID == "" {
		cluster, err = k8s.ResolveClusterIdentity(ctx, r.Client)
		if err != nil {
			return err
		}
	}
	targets, err := k8s.ResolveObservationTargets(ctx, r.Client, cluster, observation.Spec().Target)
	if err != nil {
		return err
	}
	instances := make([]domain.RuntimeContainerInstance, 0, len(targets))
	for _, target := range targets {
		instances = append(instances, target.Instance)
	}
	resolved, err := domain.NewResolvedTargetSet(instances)
	if err != nil {
		return err
	}
	backend := domain.BackendIdentity{Kind: FilesystemBackend, Version: FilesystemVersion}
	if descriptor, ok := sources[0].(SourceDescriptor); ok {
		backend = domain.BackendIdentity{Kind: descriptor.BackendName(), Version: descriptor.BackendVersion()}
	}
	if err := observation.Bind(resolved, backend, nil); err != nil {
		return err
	}
	rv, err = r.Store.UpdateExecutorStatus(ctx, namespace, claim, rv, observation)
	if err != nil {
		return err
	}

	windowCtx, cancel := context.WithTimeout(ctx, observation.Spec().Duration)
	defer cancel()
	accumulators := make([]*FilesystemAccumulator, len(sources))
	for i := range sources {
		accumulators[i] = NewFilesystemAccumulator(instances, time.Time{})
	}
	attached := make(chan error, len(targets)*len(sources))
	type sourceFailure struct {
		index int
		err   error
	}
	sourceErrors := make(chan sourceFailure, len(sources)*len(targets))
	var windowMu sync.RWMutex
	qualifiedWindow := false
	var wg sync.WaitGroup
	for sourceIndex, source := range sources {
		for _, target := range targets {
			target := target
			sourceIndex, source := sourceIndex, source
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := source.Run(windowCtx, tracer.Options{PodName: target.PodName, Namespace: namespace, Container: observation.Spec().Target.Slot.Container, Binary: r.Binary}, func(err error) { attached <- err }, func(event tracer.Event, identity tracer.RuntimeIdentity) {
					windowMu.RLock()
					ready := qualifiedWindow
					windowMu.RUnlock()
					if ready {
						accumulators[sourceIndex].Add(event, identity)
					}
				})
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					sourceErrors <- sourceFailure{index: sourceIndex, err: err}
				}
			}()
		}
	}
	attachOK := true
	for range targets {
		for range sources {
			if err := <-attached; err != nil {
				attachOK = false
			}
		}
	}
	if !attachOK {
		cancel()
		wg.Wait()
		_, transitionErr := r.Store.TransitionExecution(ctx, namespace, claim, rv, domain.ExecutionFailed, domain.BackendFailure)
		if transitionErr != nil {
			return transitionErr
		}
		return errors.New("observation source attachment failed")
	}
	windowStart := time.Now().UTC()
	// Events emitted before all attachments are acknowledged remain outside
	// the qualified window; the accumulator intentionally receives no start
	// time until this conjunction has been established.
	for _, acc := range accumulators {
		acc.mu.Lock()
		acc.start = windowStart
		acc.mu.Unlock()
	}
	windowMu.Lock()
	qualifiedWindow = true
	windowMu.Unlock()
	rv, err = r.Store.TransitionExecution(ctx, namespace, claim, rv, domain.ExecutionRunning, "")
	if err != nil {
		cancel()
		wg.Wait()
		return err
	}
	monitorCtx, stopMonitor := context.WithCancel(windowCtx)
	defer stopMonitor()
	decisions := make(chan k8s.TargetChangeDecision, 1)
	if r.Monitor != nil {
		initial := make([]k8s.ObservationTarget, 0, len(targets))
		for _, target := range targets {
			initial = append(initial, target.ObservationTarget)
		}
		go func() {
			_ = r.Monitor.Watch(monitorCtx, initial, func(decision k8s.TargetChangeDecision) {
				select {
				case decisions <- decision:
				default:
				}
			})
		}()
	}
	stopReason := domain.CompletedNormally
	var targetEvents []domain.TargetChangeEvent
	monitoring := true
	for monitoring {
		select {
		case decision := <-decisions:
			targetEvents = append(targetEvents, decision.Events...)
			if decision.Stop {
				stopReason = decision.Reason
				cancel()
				monitoring = false
			}
		case <-windowCtx.Done():
			monitoring = false
		}
	}
	windowEnd := time.Now().UTC()
	for _, acc := range accumulators {
		acc.SetEnd(windowEnd)
	}
	cancel()
	wg.Wait()
	sourceFailures := make([]error, len(sources))
	for {
		select {
		case failure := <-sourceErrors:
			if sourceFailures[failure.index] == nil {
				sourceFailures[failure.index] = failure.err
			}
		default:
			goto failuresCollected
		}
	}
failuresCollected:
	var sourceErr error
	for _, failure := range sourceFailures {
		if failure != nil {
			sourceErr = failure
			break
		}
	}
	observation, rv, err = r.Store.GetObservation(ctx, namespace, name)
	if err != nil {
		return err
	}
	for _, event := range targetEvents {
		if err := observation.AppendTargetChange(event); err != nil {
			return err
		}
	}
	for i, acc := range accumulators {
		descriptor, ok := sources[i].(SourceDescriptor)
		if !ok {
			descriptor = GadgetFilesystemSource{}
		}
		attribution := domain.AttributionCompleted
		if sourceFailures[i] != nil {
			attribution = domain.AttributionFailed
		}
		result, resultErr := acc.SourceResultFor(descriptor.SourceName(), descriptor.BackendName(), descriptor.BackendVersion(), sourceFailures[i] == nil, true, false, attribution)
		if resultErr != nil {
			return resultErr
		}
		if err := observation.RecordSourceResult(result); err != nil {
			return err
		}
	}
	rv, err = r.Store.UpdateExecutorStatus(ctx, namespace, claim, rv, observation)
	if err != nil {
		return err
	}
	rv, err = r.Store.TransitionExecution(ctx, namespace, claim, rv, domain.ExecutionCompleting, "")
	if err != nil {
		return err
	}
	// Re-read after the lifecycle transition so the result write is based on
	// the authoritative current object and is fenced by the same claim.
	observation, rv, err = r.Store.GetObservation(ctx, namespace, name)
	if err != nil {
		return err
	}
	for i, acc := range accumulators {
		descriptor, ok := sources[i].(SourceDescriptor)
		if !ok {
			descriptor = GadgetFilesystemSource{}
		}
		attribution := domain.AttributionCompleted
		if sourceFailures[i] != nil {
			attribution = domain.AttributionFailed
		}
		result, resultErr := acc.SourceResultFor(descriptor.SourceName(), descriptor.BackendName(), descriptor.BackendVersion(), sourceFailures[i] == nil, true, false, attribution)
		if resultErr != nil {
			return resultErr
		}
		if err := observation.RecordSourceResult(result); err != nil {
			return err
		}
	}
	rv, err = r.Store.UpdateExecutorStatus(ctx, namespace, claim, rv, observation)
	if err != nil {
		return err
	}
	finalReason := stopReason
	if sourceErr != nil {
		finalReason = domain.BackendFailure
	}
	_, err = r.Store.TransitionExecution(ctx, namespace, claim, rv, domain.ExecutionCompleted, finalReason)
	return err
}
