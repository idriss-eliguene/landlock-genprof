// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package runtime contains the narrow, source-specific bridge between the
// Observation domain and supported runtime collectors. It owns attribution
// facts, but not Kubernetes serialization or policy generation.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	FilesystemSourceName      = "filesystem"
	FilesystemBackend         = "trace_open"
	FilesystemVersion         = "v0.55.1"
	maxPositiveRefs           = 64
	maxNormalizedFactsRuntime = 256
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
	facts      domain.NormalizedFacts
	overflow   bool
}

func NewFilesystemAccumulator(targets []domain.RuntimeContainerInstance, start time.Time) *FilesystemAccumulator {
	return &FilesystemAccumulator{targets: append([]domain.RuntimeContainerInstance(nil), targets...), start: start}
}

func (a *FilesystemAccumulator) SetEnd(end time.Time) { a.mu.Lock(); a.end = end; a.mu.Unlock() }

func (a *FilesystemAccumulator) Add(event tracer.Event, identity tracer.RuntimeIdentity) Attribution {
	return a.AddFor(FilesystemSourceName, event, identity)
}

// AddFor records only positive facts derived from an already-attributed event.
// It never uses counts or references to reconstruct a fact.
func (a *FilesystemAccumulator) AddFor(sourceName string, event tracer.Event, identity tracer.RuntimeIdentity) Attribution {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := AttributeFilesystemEvent(event, identity, a.targets, a.start, a.end)
	if result.Found {
		a.attributed++
		a.addFactLocked(sourceName, event)
		if event.Path != "" && len(a.references) < maxPositiveRefs {
			a.references = append(a.references, event.Path)
		}
	} else {
		a.excluded++
	}
	return result
}

func (a *FilesystemAccumulator) addFactLocked(sourceName string, event tracer.Event) {
	switch sourceName {
	case FilesystemSourceName:
		permissions := []profile.FilePermission{}
		switch event.Mode {
		case "read":
			permissions = []profile.FilePermission{profile.PermissionRead}
		case "write":
			permissions = []profile.FilePermission{profile.PermissionWrite}
		case "read_write":
			permissions = []profile.FilePermission{profile.PermissionRead, profile.PermissionWrite}
		case "exec":
			permissions = []profile.FilePermission{profile.PermissionExecute}
		default:
			return
		}
		for i := range a.facts.Filesystem {
			if a.facts.Filesystem[i].Path == event.Path {
				for _, permission := range permissions {
					found := false
					for _, existing := range a.facts.Filesystem[i].Permissions {
						if existing == permission {
							found = true
						}
					}
					if !found {
						a.facts.Filesystem[i].Permissions = append(a.facts.Filesystem[i].Permissions, permission)
					}
				}
				return
			}
		}
		if len(a.facts.Filesystem) >= maxNormalizedFactsRuntime {
			a.overflow = true
			return
		}
		a.facts.Filesystem = append(a.facts.Filesystem, domain.FilesystemFact{Path: event.Path, Permissions: permissions})
	case "exec":
		for _, fact := range a.facts.Exec {
			if fact.Path == event.Path {
				return
			}
		}
		if len(a.facts.Exec) >= maxNormalizedFactsRuntime {
			a.overflow = true
			return
		}
		a.facts.Exec = append(a.facts.Exec, domain.ExecFact{Path: event.Path})
	case "networkConnect", "networkBind":
		fact := domain.NetworkFact{Port: event.Port, Direction: profile.DirectionEgress}
		if sourceName == "networkBind" {
			fact.Direction = profile.DirectionIngress
		}
		list := &a.facts.NetworkConnect
		if sourceName == "networkBind" {
			list = &a.facts.NetworkBind
		}
		for _, existing := range *list {
			if existing == fact {
				return
			}
		}
		if len(*list) >= maxNormalizedFactsRuntime {
			a.overflow = true
			return
		}
		*list = append(*list, fact)
	case "capabilities":
		for _, fact := range a.facts.Capabilities {
			if fact.Name == event.Syscall {
				return
			}
		}
		if len(a.facts.Capabilities) >= maxNormalizedFactsRuntime {
			a.overflow = true
			return
		}
		a.facts.Capabilities = append(a.facts.Capabilities, domain.CapabilityFact{Name: event.Syscall})
	}
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
	a.mu.Lock()
	facts := a.facts.Copy()
	overflow := a.overflow
	a.mu.Unlock()
	if overflow {
		return domain.SourceResult{}, fmt.Errorf("%w: normalized fact limit exceeded", domain.ErrInvalidDomainValue)
	}
	return domain.NewSourceResult(domain.EvidenceSource{Name: sourceName, Backend: backend, Version: version}, domain.SourceQualification{
		BackendHealthConfirmed: backendHealthy, SourceAttachedForBoundWindow: attached, FlushConfirmed: flushConfirmed,
		Attribution: attribution, AttributedCount: attributed, ExcludedCount: excluded,
	}, refs, facts)
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

// LeaseRenewingStore is implemented by the Kubernetes adapter. Keeping lease
// renewal optional preserves the deterministic runner seam used by the
// source-level tests while production execution can keep long observations
// fenced for their whole lifetime.
type LeaseRenewingStore interface {
	ObservationStore
	RenewLease(context.Context, string, obskube.ExecutorClaim, string) (string, error)
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
	// ExecutorConfig is the technical Kubernetes identity used by tracer
	// sources. It is nil for the legacy CLI path, which retains its existing
	// kubeconfig behavior.
	ExecutorConfig *rest.Config
	Binary         string
	Lease          time.Duration
	// LeaseRenewInterval controls the renewal cadence after an observation
	// enters RUNNING. The production default is derived from the durable
	// lease duration; tests may use a shorter cadence to exercise lifecycle
	// boundaries without waiting for the full lease interval.
	LeaseRenewInterval time.Duration
	Logger             *observability.Logger
	Metrics            *observability.Metrics
	// OnClaim notifies the owning executor after the store has acquired its
	// fenced claim. It is not a second authority mechanism.
	OnClaim func(obskube.ExecutorClaim)
	// StopRequested is set only after the executor observes durable intent for
	// its current fenced claim. Context cancellation alone has other meanings.
	StopRequested func() bool
}

func (r *Runner) finalizeDurableStop(ctx context.Context, namespace, name string, claim obskube.ExecutorClaim) error {
	_, rv, err := r.Store.GetObservation(ctx, namespace, name)
	if err != nil {
		return err
	}
	rv, err = r.Store.TransitionExecution(ctx, namespace, claim, rv, domain.ExecutionCompleting, "")
	if err != nil {
		return err
	}
	_, err = r.Store.TransitionExecution(ctx, namespace, claim, rv, domain.ExecutionCompleted, domain.StoppedByRequest)
	return err
}

func (r *Runner) persistFailure(ctx context.Context, namespace, name string, claim obskube.ExecutorClaim, stage, code, reason string) (string, error) {
	observation, rv, err := r.Store.GetObservation(ctx, namespace, name)
	if err != nil {
		return rv, err
	}
	if err := observation.RecordFailure(domain.FailureInfo{Stage: stage, Code: code, Reason: reason, Source: "executor", OccurredAt: time.Now().UTC(), Retryable: true, ExecutorID: claim.ExecutorID, ClaimGeneration: claim.ClaimGeneration}); err != nil {
		return rv, err
	}
	return r.Store.UpdateExecutorStatus(ctx, namespace, claim, rv, observation)
}

func (r *Runner) Run(ctx context.Context, namespace, name, executorID string) error {
	// A cancellation stops collection, but must not cancel the bounded status
	// writes which make shutdown truthful. Kubernetes request deadlines still
	// bound each client call through the client configuration.
	persistCtx := context.WithoutCancel(ctx)
	sources := r.Sources
	if len(sources) == 0 && r.Source != nil {
		sources = []FilesystemSource{r.Source}
	}
	if r.Store == nil || r.Client == nil || len(sources) == 0 {
		return errors.New("filesystem observation runner is incompletely configured")
	}
	claim, rv, err := r.Store.ClaimObservation(persistCtx, namespace, name, executorID)
	if err != nil {
		return err
	}
	if r.OnClaim != nil {
		r.OnClaim(claim)
	}
	if r.Logger != nil {
		r.Logger.Info("observation_claim_acquired", map[string]interface{}{"component": "observation_executor", "namespace": namespace, "observation": name, "executorID": claim.ExecutorID, "claimGeneration": claim.ClaimGeneration, "phase": "CLAIMED"})
	}
	observation, _, err := r.Store.GetObservation(persistCtx, namespace, name)
	if err != nil {
		return err
	}
	cluster := r.Cluster
	if cluster.NamespaceUID == "" {
		cluster, err = k8s.ResolveClusterIdentity(persistCtx, r.Client)
		if err != nil {
			return err
		}
	}
	targets, err := k8s.ResolveObservationTargets(persistCtx, r.Client, cluster, observation.Spec().Target)
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
	rv, err = r.Store.UpdateExecutorStatus(persistCtx, namespace, claim, rv, observation)
	if err != nil {
		if r.StopRequested != nil && r.StopRequested() {
			return r.finalizeDurableStop(persistCtx, namespace, name, claim)
		}
		return err
	}

	windowCtx, cancel := context.WithTimeout(ctx, observation.Spec().Duration)
	defer cancel()
	startupTimeout := r.Lease
	if startupTimeout <= 0 {
		startupTimeout = obskube.DefaultLeaseDuration
	}
	startupCtx, cancelStartup := context.WithTimeout(windowCtx, startupTimeout)
	defer cancelStartup()
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
				err := source.Run(windowCtx, tracer.Options{KubeConfig: r.ExecutorConfig, PodName: target.PodName, Namespace: namespace, Container: observation.Spec().Target.Slot.Container, Binary: r.Binary, Scope: tracer.ContainerScoped}, func(err error) { attached <- err }, func(event tracer.Event, identity tracer.RuntimeIdentity) {
					windowMu.RLock()
					ready := qualifiedWindow
					windowMu.RUnlock()
					if ready {
						sourceName := FilesystemSourceName
						if descriptor, ok := source.(SourceDescriptor); ok {
							sourceName = descriptor.SourceName()
						}
						accumulators[sourceIndex].AddFor(sourceName, event, identity)
					}
				})
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					sourceErrors <- sourceFailure{index: sourceIndex, err: err}
				}
			}()
		}
	}
	attachOK := true
	startupFailed := false
	for range targets {
		for range sources {
			select {
			case err := <-attached:
				if err != nil {
					attachOK = false
				}
			case <-startupCtx.Done():
				attachOK = false
				startupFailed = true
			}
			if startupFailed {
				break
			}
		}
		if startupFailed {
			break
		}
	}
	if !attachOK {
		cancel()
		cancelStartup()
		wg.Wait()
		if r.StopRequested != nil && r.StopRequested() {
			// No qualified window exists yet. Do not synthesize source results;
			// finalization will preserve the unknown evidence state. Re-read the
			// object because persisting the intent may have advanced its RV.
			return r.finalizeDurableStop(persistCtx, namespace, name, claim)
		}
		failureCode := "TRACE_ATTACH_FAILED"
		failureReason := "evidence source attachment failed"
		if startupFailed {
			failureCode = "TRACE_ATTACH_TIMEOUT"
			failureReason = "evidence source attachment did not become ready before the startup deadline"
		}
		failureRV, failureErr := r.persistFailure(persistCtx, namespace, name, claim, "GADGET_ATTACH", failureCode, failureReason)
		if failureErr != nil {
			return failureErr
		}
		_, transitionErr := r.Store.TransitionExecution(persistCtx, namespace, claim, failureRV, domain.ExecutionFailed, domain.BackendFailure)
		if transitionErr != nil {
			return transitionErr
		}
		if startupFailed {
			return fmt.Errorf("observation source attachment timed out: %w", startupCtx.Err())
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
	rv, err = r.Store.TransitionExecution(persistCtx, namespace, claim, rv, domain.ExecutionRunning, "")
	if err != nil {
		cancel()
		wg.Wait()
		if r.StopRequested != nil && r.StopRequested() {
			return r.finalizeDurableStop(persistCtx, namespace, name, claim)
		}
		return err
	}
	if r.Logger != nil {
		r.Logger.Info("observation_running", map[string]interface{}{"component": "observation_executor", "namespace": namespace, "observation": name, "executorID": claim.ExecutorID, "claimGeneration": claim.ClaimGeneration, "phase": "RUNNING"})
	}
	var renewCancel context.CancelFunc
	var renewDone <-chan struct{}
	leaseErrors := make(chan error, 1)
	if leaseStore, ok := r.Store.(LeaseRenewingStore); ok {
		rvMu := &sync.Mutex{}
		currentRV := rv
		renewCtx, cancel := context.WithCancel(context.Background())
		renewCancel = cancel
		done := make(chan struct{})
		renewDone = done
		renewInterval := r.LeaseRenewInterval
		if renewInterval <= 0 {
			renewInterval = obskube.DefaultLeaseDuration / 3
		}
		go func() {
			defer close(done)
			ticker := time.NewTicker(renewInterval)
			defer ticker.Stop()
			for {
				select {
				case <-renewCtx.Done():
					return
				case <-ticker.C:
					rvMu.Lock()
					nextRV, renewErr := leaseStore.RenewLease(persistCtx, namespace, claim, currentRV)
					if renewErr == nil {
						currentRV = nextRV
					}
					rvMu.Unlock()
					if renewErr != nil {
						if r.Metrics != nil {
							r.Metrics.LeaseRenewalFailure()
						}
						if r.Logger != nil {
							r.Logger.Warn("lease_renewal_failed", map[string]interface{}{"component": "observation_executor", "namespace": namespace, "observation": name, "reason": renewErr})
						}
						select {
						case leaseErrors <- renewErr:
						default:
						}
						return
					}
				}
			}
		}()
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
	var leaseFailure error
	monitoring := true
	for monitoring {
		select {
		case leaseFailure = <-leaseErrors:
			cancel()
			monitoring = false
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
	if leaseFailure != nil {
		if renewCancel != nil {
			renewCancel()
			<-renewDone
		}
		return leaseFailure
	}
	if ctx.Err() != nil && stopReason == domain.CompletedNormally {
		stopReason = domain.StoppedByRequest
	}
	windowEnd := time.Now().UTC()
	for _, acc := range accumulators {
		acc.SetEnd(windowEnd)
	}
	cancel()
	wg.Wait()
	// Keep the durable lease alive through source cancellation and stream
	// draining. Stop is an intent to terminate collection, not proof that
	// finalization has completed. Once all source goroutines have exited, stop
	// renewal before the fenced terminal writes and re-read the authoritative
	// resource version for those writes below.
	if renewCancel != nil {
		renewCancel()
		<-renewDone
	}
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
	observation, rv, err = r.Store.GetObservation(persistCtx, namespace, name)
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
	rv, err = r.Store.UpdateExecutorStatus(persistCtx, namespace, claim, rv, observation)
	if err != nil {
		return err
	}
	rv, err = r.Store.TransitionExecution(persistCtx, namespace, claim, rv, domain.ExecutionCompleting, "")
	if err != nil {
		return err
	}
	if r.Logger != nil {
		r.Logger.Info("observation_completing", map[string]interface{}{"component": "observation_executor", "namespace": namespace, "observation": name, "executorID": claim.ExecutorID, "claimGeneration": claim.ClaimGeneration, "phase": "COMPLETING"})
	}
	// Re-read after the lifecycle transition so the result write is based on
	// the authoritative current object and is fenced by the same claim.
	observation, rv, err = r.Store.GetObservation(persistCtx, namespace, name)
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
	rv, err = r.Store.UpdateExecutorStatus(persistCtx, namespace, claim, rv, observation)
	if err != nil {
		return err
	}
	finalReason := stopReason
	if sourceErr != nil {
		finalReason = domain.BackendFailure
	}
	_, err = r.Store.TransitionExecution(persistCtx, namespace, claim, rv, domain.ExecutionCompleted, finalReason)
	if err == nil && r.Logger != nil {
		r.Logger.Info("observation_terminal", map[string]interface{}{"component": "observation_executor", "namespace": namespace, "observation": name, "executorID": claim.ExecutorID, "claimGeneration": claim.ClaimGeneration, "phase": "COMPLETED", "reason": finalReason})
	}
	return err
}
