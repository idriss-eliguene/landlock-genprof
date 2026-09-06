// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrObservationFrozen  = errors.New("observation is terminal and frozen")
	ErrInvalidTransition  = errors.New("invalid observation state transition")
	ErrInvalidDomainValue = errors.New("invalid observation domain value")
)

// ObservationID is opaque and is the sole identity of an Observation.
type ObservationID string

func NewObservationID() (ObservationID, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating ObservationID: %w", err)
	}
	return ObservationID(hex.EncodeToString(b)), nil
}

func (id ObservationID) Valid() bool { return id != "" }

// RequestedTarget is immutable observation intent, not a governance target.
type RequestedTarget struct{ Slot ContainerSlot }

func (t RequestedTarget) Valid() bool { return t.Slot.Valid() }

// ObservationSpec is immutable requested intent.
type ObservationSpec struct {
	Target           RequestedTarget
	Sources          []string
	Duration         time.Duration
	RequesterSession string
}

func NewObservationSpec(target RequestedTarget, sources []string, duration time.Duration, requesterSession string) (ObservationSpec, error) {
	if !target.Valid() || duration <= 0 || len(sources) == 0 {
		return ObservationSpec{}, fmt.Errorf("%w: incomplete ObservationSpec", ErrInvalidDomainValue)
	}
	copySources := append([]string(nil), sources...)
	for _, source := range copySources {
		if strings.TrimSpace(source) == "" {
			return ObservationSpec{}, fmt.Errorf("%w: empty evidence source", ErrInvalidDomainValue)
		}
	}
	sort.Strings(copySources)
	return ObservationSpec{Target: target, Sources: copySources, Duration: duration, RequesterSession: requesterSession}, nil
}

func (s ObservationSpec) SourceNames() []string { return append([]string(nil), s.Sources...) }

// ResolvedTargetSet, ObservedTargetSet, and ExcludedTargetSet are distinct
// value types so resolution and attribution cannot be conflated.
type ResolvedTargetSet struct{ items []RuntimeContainerInstance }
type ObservedTargetSet struct{ items []RuntimeContainerInstance }
type ExcludedTargetSet struct{ items []RuntimeContainerInstance }

func newTargetItems(items []RuntimeContainerInstance) ([]RuntimeContainerInstance, error) {
	copyItems := append([]RuntimeContainerInstance(nil), items...)
	for _, item := range copyItems {
		if !item.Valid() {
			return nil, fmt.Errorf("%w: invalid runtime target", ErrInvalidDomainValue)
		}
	}
	sort.SliceStable(copyItems, func(i, j int) bool {
		return runtimeInstanceKey(copyItems[i]) < runtimeInstanceKey(copyItems[j])
	})
	return copyItems, nil
}

func NewResolvedTargetSet(items []RuntimeContainerInstance) (ResolvedTargetSet, error) {
	copyItems, err := newTargetItems(items)
	return ResolvedTargetSet{items: copyItems}, err
}

func NewObservedTargetSet(items []RuntimeContainerInstance) (ObservedTargetSet, error) {
	copyItems, err := newTargetItems(items)
	return ObservedTargetSet{items: copyItems}, err
}

func NewExcludedTargetSet(items []RuntimeContainerInstance) (ExcludedTargetSet, error) {
	copyItems, err := newTargetItems(items)
	return ExcludedTargetSet{items: copyItems}, err
}

func (s ResolvedTargetSet) Items() []RuntimeContainerInstance {
	return append([]RuntimeContainerInstance(nil), s.items...)
}
func (s ObservedTargetSet) Items() []RuntimeContainerInstance {
	return append([]RuntimeContainerInstance(nil), s.items...)
}
func (s ExcludedTargetSet) Items() []RuntimeContainerInstance {
	return append([]RuntimeContainerInstance(nil), s.items...)
}

type BackendIdentity struct {
	Kind    string
	Version string
}

// TargetChangeKind is deliberately small; it records facts, not actions.
type TargetChangeKind string

const (
	PodAdded                TargetChangeKind = "POD_ADDED"
	PodRemoved              TargetChangeKind = "POD_REMOVED"
	ImageChanged            TargetChangeKind = "IMAGE_CHANGED"
	WorkloadRevisionChanged TargetChangeKind = "WORKLOAD_REVISION_CHANGED"
)

type TargetChangeEvent struct {
	At     time.Time
	Kind   TargetChangeKind
	Detail string
}

func NewTargetChangeEvent(at time.Time, kind TargetChangeKind, detail string) (TargetChangeEvent, error) {
	if at.IsZero() || strings.TrimSpace(string(kind)) == "" {
		return TargetChangeEvent{}, fmt.Errorf("%w: incomplete target change event", ErrInvalidDomainValue)
	}
	switch kind {
	case PodAdded, PodRemoved, ImageChanged, WorkloadRevisionChanged:
	default:
		return TargetChangeEvent{}, fmt.Errorf("%w: unsupported target change kind %q", ErrInvalidDomainValue, kind)
	}
	return TargetChangeEvent{At: at, Kind: kind, Detail: detail}, nil
}

// ObservationBinding records runtime facts separately from requested intent.
type ObservationBinding struct {
	ResolvedTargets ResolvedTargetSet
	Backend         BackendIdentity
	ImageRevisions  []ContainerImageRevision
	TargetChanges   []TargetChangeEvent
}

func (b ObservationBinding) ImageRevisionValues() []ContainerImageRevision {
	return append([]ContainerImageRevision(nil), b.ImageRevisions...)
}
func (b ObservationBinding) TargetChangeEvents() []TargetChangeEvent {
	return append([]TargetChangeEvent(nil), b.TargetChanges...)
}

type ExecutionState string

const (
	ExecutionRequested  ExecutionState = "REQUESTED"
	ExecutionStarting   ExecutionState = "STARTING"
	ExecutionRunning    ExecutionState = "RUNNING"
	ExecutionCompleting ExecutionState = "COMPLETING"
	ExecutionCompleted  ExecutionState = "COMPLETED"
	ExecutionFailed     ExecutionState = "FAILED"
)

type CompletionReason string

const (
	CompletedNormally     CompletionReason = "COMPLETED"
	StoppedByRequest      CompletionReason = "STOPPED_BY_REQUEST"
	TargetRevisionChanged CompletionReason = "TARGET_REVISION_CHANGED"
	TargetUnavailable     CompletionReason = "TARGET_UNAVAILABLE"
	ExecutorLost          CompletionReason = "EXECUTOR_LOST"
	// BackendFailure is an Observation-level terminal hint. It may represent
	// a partial source failure; the per-source SourceResult qualification is
	// authoritative for each requested source and does not imply that all
	// sources failed.
	BackendFailure CompletionReason = "BACKEND_FAILURE"
)

func (s ExecutionState) Valid() bool {
	switch s {
	case ExecutionRequested, ExecutionStarting, ExecutionRunning, ExecutionCompleting, ExecutionCompleted, ExecutionFailed:
		return true
	default:
		return false
	}
}

type ObservationExecution struct {
	State       ExecutionState
	Completion  CompletionReason
	StartedAt   time.Time
	CompletedAt time.Time
}

type AttributionState string

const (
	AttributionNotStarted AttributionState = "NOT_STARTED"
	AttributionInProgress AttributionState = "IN_PROGRESS"
	AttributionCompleted  AttributionState = "COMPLETED"
	AttributionFailed     AttributionState = "FAILED"
)

func (s AttributionState) Valid() bool {
	switch s {
	case AttributionNotStarted, AttributionInProgress, AttributionCompleted, AttributionFailed:
		return true
	default:
		return false
	}
}

func AdvanceAttribution(from, to AttributionState) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("%w: invalid attribution state", ErrInvalidDomainValue)
	}
	valid := (from == AttributionNotStarted && to == AttributionInProgress) ||
		(from == AttributionInProgress && (to == AttributionCompleted || to == AttributionFailed))
	if !valid {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	return nil
}

type EvidenceState string

const (
	EvidenceEmpty     EvidenceState = "EMPTY"
	EvidenceAvailable EvidenceState = "AVAILABLE"
	EvidenceUnknown   EvidenceState = "UNKNOWN"
)

// SourceQualification contains only generic proof facts. Backend-specific
// details belong in source adapters, not in this domain value.
type SourceQualification struct {
	BackendHealthConfirmed       bool
	SourceAttachedForBoundWindow bool
	FlushConfirmed               bool
	Attribution                  AttributionState
	AttributedCount              uint64
	ExcludedCount                uint64
}

func DeriveEvidenceState(q SourceQualification) EvidenceState {
	if !q.BackendHealthConfirmed || !q.SourceAttachedForBoundWindow || !q.FlushConfirmed || q.Attribution != AttributionCompleted || q.ExcludedCount > 0 {
		return EvidenceUnknown
	}
	if q.AttributedCount > 0 {
		return EvidenceAvailable
	}
	return EvidenceEmpty
}

type EvidenceSource struct {
	Name    string
	Backend string
	Version string
}

type SourceResult struct {
	Source        EvidenceSource
	Qualification SourceQualification
	Evidence      EvidenceState
	References    []string
	Facts         NormalizedFacts
}

func NewSourceResult(source EvidenceSource, qualification SourceQualification, references []string, facts ...NormalizedFacts) (SourceResult, error) {
	if strings.TrimSpace(source.Name) == "" || !qualification.Attribution.Valid() {
		return SourceResult{}, fmt.Errorf("%w: incomplete source result", ErrInvalidDomainValue)
	}
	if len(references) > 64 {
		return SourceResult{}, fmt.Errorf("%w: too many evidence references", ErrInvalidDomainValue)
	}
	copyReferences := append([]string(nil), references...)
	for _, ref := range copyReferences {
		if strings.TrimSpace(ref) == "" || len(ref) > 512 {
			return SourceResult{}, fmt.Errorf("%w: invalid evidence reference", ErrInvalidDomainValue)
		}
	}
	var normalized NormalizedFacts
	if len(facts) > 1 {
		return SourceResult{}, fmt.Errorf("%w: multiple normalized fact sets", ErrInvalidDomainValue)
	}
	if len(facts) == 1 {
		normalized = facts[0].Copy()
	}
	if err := normalized.ValidateForSource(source.Name); err != nil {
		return SourceResult{}, err
	}
	sortFacts(&normalized)
	if qualification.AttributedCount == 0 && normalized.Count() > 0 {
		return SourceResult{}, fmt.Errorf("%w: facts require attributed evidence", ErrInvalidDomainValue)
	}
	return SourceResult{Source: source, Qualification: qualification, Evidence: DeriveEvidenceState(qualification), References: copyReferences, Facts: normalized}, nil
}

// ObservationResult contains bounded per-source facts and no raw events.
type ObservationResult struct{ sources []SourceResult }

// NewObservationResult validates and copies a persisted set of source facts.
// It is intentionally a domain constructor rather than a persistence API.
func NewObservationResult(sources []SourceResult) (ObservationResult, error) {
	result := ObservationResult{sources: make([]SourceResult, 0, len(sources))}
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		validated, err := NewSourceResult(source.Source, source.Qualification, source.References, source.Facts)
		if err != nil {
			return ObservationResult{}, err
		}
		if source.Evidence != "" && source.Evidence != validated.Evidence {
			return ObservationResult{}, fmt.Errorf("%w: source evidence does not match qualification", ErrInvalidDomainValue)
		}
		if _, exists := seen[source.Source.Name]; exists {
			return ObservationResult{}, fmt.Errorf("%w: duplicate evidence source", ErrInvalidDomainValue)
		}
		seen[source.Source.Name] = struct{}{}
		result.sources = append(result.sources, validated)
	}
	sort.SliceStable(result.sources, func(i, j int) bool { return result.sources[i].Source.Name < result.sources[j].Source.Name })
	return result, nil
}

func (r ObservationResult) Sources() []SourceResult {
	result := append([]SourceResult(nil), r.sources...)
	for i := range result {
		result[i].References = append([]string(nil), result[i].References...)
		result[i].Facts = result[i].Facts.Copy()
	}
	return result
}

// ObservationProvenance is a frozen explanation of the binding/result facts.
type ObservationProvenance struct {
	ResolvedTargets  ResolvedTargetSet
	ImageRevisions   []ContainerImageRevision
	Backend          BackendIdentity
	RequestedSources []string
}

// Observation is the pure domain aggregate. Only ID is identity; all other
// fields are accessed through copies and controlled transitions.
type Observation struct {
	id         ObservationID
	spec       ObservationSpec
	binding    ObservationBinding
	execution  ObservationExecution
	result     ObservationResult
	provenance ObservationProvenance
	bound      bool
	frozen     bool
}

func NewObservation(id ObservationID, spec ObservationSpec) (Observation, error) {
	if !id.Valid() || !spec.Target.Valid() || spec.Duration <= 0 || len(spec.Sources) == 0 {
		return Observation{}, fmt.Errorf("%w: invalid Observation", ErrInvalidDomainValue)
	}
	copySpec := spec
	copySpec.Sources = append([]string(nil), spec.Sources...)
	return Observation{id: id, spec: copySpec, execution: ObservationExecution{State: ExecutionRequested}}, nil
}

// RestoreObservation reconstructs a domain aggregate from a trusted,
// validated persistence adapter. It deliberately accepts domain values only;
// Kubernetes serialization and resource-version handling stay outside this
// package.
func RestoreObservation(id ObservationID, spec ObservationSpec, binding ObservationBinding, execution ObservationExecution, result ObservationResult, provenance ObservationProvenance) (Observation, error) {
	observation, err := NewObservation(id, spec)
	if err != nil {
		return Observation{}, err
	}
	if !execution.State.Valid() {
		return Observation{}, fmt.Errorf("%w: invalid execution state", ErrInvalidDomainValue)
	}
	if execution.State == ExecutionCompleted || execution.State == ExecutionFailed {
		if strings.TrimSpace(string(execution.Completion)) == "" {
			return Observation{}, fmt.Errorf("%w: terminal execution requires a completion reason", ErrInvalidDomainValue)
		}
		observation.frozen = true
	} else if len(provenance.RequestedSources) != 0 || len(provenance.ImageRevisions) != 0 || len(provenance.ResolvedTargets.Items()) != 0 || provenance.Backend != (BackendIdentity{}) {
		return Observation{}, fmt.Errorf("%w: non-terminal observation has provenance", ErrInvalidDomainValue)
	}
	observation.binding = binding
	observation.bound = binding.Backend != (BackendIdentity{}) || len(binding.ImageRevisions) > 0 || len(binding.TargetChanges) > 0 || len(binding.ResolvedTargets.Items()) > 0
	observation.execution = execution
	observation.result = result
	observation.provenance = provenance
	return observation, nil
}

func (o Observation) ID() ObservationID { return o.id }
func (o Observation) Spec() ObservationSpec {
	spec := o.spec
	spec.Sources = append([]string(nil), o.spec.Sources...)
	return spec
}
func (o Observation) Binding() ObservationBinding {
	binding := o.binding
	binding.ImageRevisions = append([]ContainerImageRevision(nil), o.binding.ImageRevisions...)
	binding.TargetChanges = append([]TargetChangeEvent(nil), o.binding.TargetChanges...)
	return binding
}
func (o Observation) Execution() ObservationExecution { return o.execution }
func (o Observation) Result() ObservationResult {
	return ObservationResult{sources: o.result.Sources()}
}
func (o Observation) Provenance() ObservationProvenance {
	provenance := o.provenance
	provenance.ImageRevisions = append([]ContainerImageRevision(nil), o.provenance.ImageRevisions...)
	provenance.RequestedSources = append([]string(nil), o.provenance.RequestedSources...)
	return provenance
}
func (o Observation) Frozen() bool { return o.frozen }

func (o *Observation) Bind(resolved ResolvedTargetSet, backend BackendIdentity, revisions []ContainerImageRevision) error {
	if o.frozen {
		return ErrObservationFrozen
	}
	if o.bound {
		return fmt.Errorf("%w: binding already established", ErrInvalidTransition)
	}
	o.binding = ObservationBinding{ResolvedTargets: resolved, Backend: backend, ImageRevisions: append([]ContainerImageRevision(nil), revisions...)}
	o.bound = true
	return nil
}

func (o *Observation) AppendTargetChange(event TargetChangeEvent) error {
	if o.frozen {
		return ErrObservationFrozen
	}
	o.binding.TargetChanges = append(o.binding.TargetChanges, event)
	return nil
}

func (o *Observation) RecordSourceResult(source SourceResult) error {
	if o.frozen {
		return ErrObservationFrozen
	}
	if strings.TrimSpace(source.Source.Name) == "" || !source.Qualification.Attribution.Valid() {
		return fmt.Errorf("%w: incomplete source result", ErrInvalidDomainValue)
	}
	source.Evidence = DeriveEvidenceState(source.Qualification)
	source.References = append([]string(nil), source.References...)
	for i := range o.result.sources {
		if o.result.sources[i].Source == source.Source {
			o.result.sources[i] = source
			return nil
		}
	}
	o.result.sources = append(o.result.sources, source)
	sort.SliceStable(o.result.sources, func(i, j int) bool { return o.result.sources[i].Source.Name < o.result.sources[j].Source.Name })
	return nil
}

func (o *Observation) Transition(next ExecutionState, reason CompletionReason) error {
	if o.frozen {
		return ErrObservationFrozen
	}
	if !next.Valid() || !validExecutionTransition(o.execution.State, next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, o.execution.State, next)
	}
	if (next == ExecutionCompleted || next == ExecutionFailed) && strings.TrimSpace(string(reason)) == "" {
		return fmt.Errorf("%w: terminal execution requires a completion reason", ErrInvalidDomainValue)
	}
	now := time.Now().UTC()
	if next == ExecutionRunning && o.execution.StartedAt.IsZero() {
		o.execution.StartedAt = now
	}
	o.execution.State = next
	if next == ExecutionCompleted || next == ExecutionFailed {
		o.execution.Completion = reason
		o.execution.CompletedAt = now
		o.frozen = true
		o.provenance = ObservationProvenance{ResolvedTargets: o.binding.ResolvedTargets, ImageRevisions: append([]ContainerImageRevision(nil), o.binding.ImageRevisions...), Backend: o.binding.Backend, RequestedSources: append([]string(nil), o.spec.Sources...)}
	}
	return nil
}

func validExecutionTransition(from, to ExecutionState) bool {
	switch from {
	case ExecutionRequested:
		return to == ExecutionStarting
	case ExecutionStarting:
		return to == ExecutionRunning || to == ExecutionFailed
	case ExecutionRunning:
		return to == ExecutionCompleting
	case ExecutionCompleting:
		return to == ExecutionCompleted || to == ExecutionFailed
	default:
		return false
	}
}

func runtimeInstanceKey(item RuntimeContainerInstance) string {
	return item.Slot.Workload.Namespace + "\x00" + item.Slot.Workload.Name + "\x00" + item.Slot.Container + "\x00" + item.PodUID + "\x00" + item.ContainerID
}
