// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package kubernetes contains the bounded Kubernetes persistence adapter for
// the Kubernetes-independent observation domain.
package kubernetes

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	apiVersion = "landlockgenprof.io/v1alpha1"
	kind       = "Observation"

	maxSources       = 16
	maxTargets       = 64
	maxRevisions     = 64
	maxTargetChanges = 256
	maxSourceResults = 16
	maxString        = 512
	maxSourceName    = 128
	maxReferences    = 64
	maxDuration      = 24 * time.Hour
)

type persistedGroupKind struct {
	Group string `json:"group,omitempty"`
	Kind  string `json:"kind"`
}
type persistedCluster struct {
	NamespaceUID string `json:"namespaceUID"`
}
type persistedWorkload struct {
	Cluster   persistedCluster   `json:"cluster"`
	Namespace string             `json:"namespace"`
	GroupKind persistedGroupKind `json:"groupKind"`
	Name      string             `json:"name"`
	UID       string             `json:"uid"`
}
type persistedSlot struct {
	Workload  persistedWorkload `json:"workload"`
	Container string            `json:"container"`
}
type persistedTarget struct {
	Slot persistedSlot `json:"slot"`
}
type persistedImageRevision struct {
	Slot        persistedSlot `json:"slot"`
	ImageDigest string        `json:"imageDigest"`
}
type persistedRuntimeInstance struct {
	Slot          persistedSlot           `json:"slot"`
	PodUID        string                  `json:"podUID"`
	ContainerID   string                  `json:"containerID,omitempty"`
	ImageRevision *persistedImageRevision `json:"imageRevision,omitempty"`
}
type persistedBackend struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
}
type persistedTargetChange struct {
	At     string `json:"at"`
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}

type persistedSpec struct {
	Target           persistedTarget `json:"target"`
	Sources          []string        `json:"sources"`
	Duration         string          `json:"duration"`
	RequesterSession string          `json:"requesterSession,omitempty"`
}
type persistedBinding struct {
	ResolvedTargets []persistedRuntimeInstance `json:"resolvedTargets"`
	Backend         persistedBackend           `json:"backend"`
	ImageRevisions  []persistedImageRevision   `json:"imageRevisions"`
	TargetChanges   []persistedTargetChange    `json:"targetChanges"`
}
type persistedExecution struct {
	State       string `json:"state"`
	Completion  string `json:"completion,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	// These fields are structurally reserved for G4. This adapter does not
	// interpret them as proof of executor ownership.
	ExecutorID          string            `json:"executorID,omitempty"`
	ClaimGeneration     uint64            `json:"claimGeneration,omitempty"`
	LeaseExpiry         string            `json:"leaseExpiry,omitempty"`
	StopRequestedAt     string            `json:"stopRequestedAt,omitempty"`
	StopRequester       string            `json:"stopRequester,omitempty"`
	StopContextVersion  uint64            `json:"stopContextVersion,omitempty"`
	StopExecutorID      string            `json:"stopExecutorID,omitempty"`
	StopClaimGeneration uint64            `json:"stopClaimGeneration,omitempty"`
	Failure             *persistedFailure `json:"failure,omitempty"`
}
type persistedFailure struct {
	Stage           string `json:"stage"`
	Code            string `json:"code"`
	Reason          string `json:"reason"`
	Source          string `json:"source"`
	OccurredAt      string `json:"occurredAt"`
	Retryable       bool   `json:"retryable"`
	ExecutorID      string `json:"executorID,omitempty"`
	ClaimGeneration uint64 `json:"claimGeneration,omitempty"`
}
type persistedQualification struct {
	BackendHealthConfirmed       bool   `json:"backendHealthConfirmed"`
	SourceAttachedForBoundWindow bool   `json:"sourceAttachedForBoundWindow"`
	FlushConfirmed               bool   `json:"flushConfirmed"`
	Attribution                  string `json:"attribution"`
	AttributedCount              uint64 `json:"attributedCount"`
	ExcludedCount                uint64 `json:"excludedCount"`
}
type persistedSource struct {
	Name          string                 `json:"name"`
	Backend       string                 `json:"backend,omitempty"`
	Version       string                 `json:"version,omitempty"`
	Qualification persistedQualification `json:"qualification"`
	Evidence      string                 `json:"evidence"`
	References    []string               `json:"references,omitempty"`
	Facts         persistedFacts         `json:"facts,omitempty"`
}
type persistedFilesystemFact struct {
	Path        string   `json:"path"`
	Permissions []string `json:"permissions"`
}
type persistedNetworkFact struct {
	Port      int    `json:"port"`
	Direction string `json:"direction"`
}
type persistedCapabilityFact struct {
	Name string `json:"name"`
}
type persistedExecFact struct {
	Path string `json:"path"`
}
type persistedFacts struct {
	Filesystem     []persistedFilesystemFact `json:"filesystem,omitempty"`
	Exec           []persistedExecFact       `json:"exec,omitempty"`
	NetworkConnect []persistedNetworkFact    `json:"networkConnect,omitempty"`
	NetworkBind    []persistedNetworkFact    `json:"networkBind,omitempty"`
	Capabilities   []persistedCapabilityFact `json:"capabilities,omitempty"`
}
type persistedResult struct {
	Sources []persistedSource `json:"sources"`
}
type persistedProvenance struct {
	ResolvedTargets  []persistedRuntimeInstance `json:"resolvedTargets"`
	ImageRevisions   []persistedImageRevision   `json:"imageRevisions"`
	Backend          persistedBackend           `json:"backend"`
	RequestedSources []string                   `json:"requestedSources"`
}
type persistedStatus struct {
	Binding    persistedBinding     `json:"binding"`
	Execution  persistedExecution   `json:"execution"`
	Result     persistedResult      `json:"result"`
	Provenance *persistedProvenance `json:"provenance,omitempty"`
}

func toMap(value any) (map[string]interface{}, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal observation: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("unmarshal observation: %w", err)
	}
	return result, nil
}

func fromMap(value interface{}, target any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal persisted observation: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode persisted observation: %w", err)
	}
	return nil
}

func encodeSlot(slot domain.ContainerSlot) persistedSlot {
	return persistedSlot{Workload: persistedWorkload{Cluster: persistedCluster{NamespaceUID: slot.Workload.Cluster.NamespaceUID}, Namespace: slot.Workload.Namespace, GroupKind: persistedGroupKind{Group: slot.Workload.GroupKind.Group, Kind: slot.Workload.GroupKind.Kind}, Name: slot.Workload.Name, UID: slot.Workload.UID}, Container: slot.Container}
}
func decodeSlot(slot persistedSlot) (domain.ContainerSlot, error) {
	workload := domain.WorkloadIdentity{Cluster: domain.ClusterIdentity{NamespaceUID: slot.Workload.Cluster.NamespaceUID}, Namespace: slot.Workload.Namespace, GroupKind: domain.GroupKind{Group: slot.Workload.GroupKind.Group, Kind: slot.Workload.GroupKind.Kind}, Name: slot.Workload.Name, UID: slot.Workload.UID}
	result := domain.ContainerSlot{Workload: workload, Container: slot.Container}
	if !result.Valid() {
		return domain.ContainerSlot{}, fmt.Errorf("%w: invalid container slot", domain.ErrInvalidDomainValue)
	}
	return result, nil
}
func encodeRuntime(item domain.RuntimeContainerInstance) persistedRuntimeInstance {
	result := persistedRuntimeInstance{Slot: encodeSlot(item.Slot), PodUID: item.PodUID, ContainerID: item.ContainerID}
	if item.ImageRevision != nil {
		value := encodeImageRevision(*item.ImageRevision)
		result.ImageRevision = &value
	}
	return result
}
func decodeRuntime(item persistedRuntimeInstance) (domain.RuntimeContainerInstance, error) {
	slot, err := decodeSlot(item.Slot)
	if err != nil {
		return domain.RuntimeContainerInstance{}, err
	}
	result := domain.RuntimeContainerInstance{Slot: slot, PodUID: item.PodUID, ContainerID: item.ContainerID}
	if item.ImageRevision != nil {
		revision, err := decodeImageRevision(*item.ImageRevision)
		if err != nil {
			return result, err
		}
		result.ImageRevision = &revision
	}
	if !result.Valid() {
		return domain.RuntimeContainerInstance{}, fmt.Errorf("%w: invalid runtime container instance", domain.ErrInvalidDomainValue)
	}
	return result, nil
}
func encodeImageRevision(item domain.ContainerImageRevision) persistedImageRevision {
	return persistedImageRevision{Slot: encodeSlot(item.Slot), ImageDigest: item.ImageDigest}
}
func decodeImageRevision(item persistedImageRevision) (domain.ContainerImageRevision, error) {
	slot, err := decodeSlot(item.Slot)
	if err != nil {
		return domain.ContainerImageRevision{}, err
	}
	return domain.NewContainerImageRevision(slot, item.ImageDigest)
}

func encodeSpec(spec domain.ObservationSpec) persistedSpec {
	return persistedSpec{Target: persistedTarget{Slot: encodeSlot(spec.Target.Slot)}, Sources: spec.SourceNames(), Duration: spec.Duration.String(), RequesterSession: spec.RequesterSession}
}
func decodeSpec(spec persistedSpec) (domain.ObservationSpec, error) {
	if len(spec.Sources) == 0 || len(spec.Sources) > maxSources || len(spec.RequesterSession) > maxString {
		return domain.ObservationSpec{}, fmt.Errorf("%w: observation spec exceeds bounds", domain.ErrInvalidDomainValue)
	}
	for _, source := range spec.Sources {
		if len(source) > maxSourceName {
			return domain.ObservationSpec{}, fmt.Errorf("%w: evidence source exceeds bounds", domain.ErrInvalidDomainValue)
		}
	}
	slot, err := decodeSlot(spec.Target.Slot)
	if err != nil {
		return domain.ObservationSpec{}, err
	}
	duration, err := time.ParseDuration(spec.Duration)
	if err != nil || duration <= 0 || duration > maxDuration {
		return domain.ObservationSpec{}, fmt.Errorf("%w: invalid observation duration", domain.ErrInvalidDomainValue)
	}
	return domain.NewObservationSpec(domain.RequestedTarget{Slot: slot}, spec.Sources, duration, spec.RequesterSession)
}

func encodeBinding(binding domain.ObservationBinding) persistedBinding {
	result := persistedBinding{Backend: persistedBackend{Kind: binding.Backend.Kind, Version: binding.Backend.Version}}
	for _, item := range binding.ResolvedTargets.Items() {
		result.ResolvedTargets = append(result.ResolvedTargets, encodeRuntime(item))
	}
	for _, item := range binding.ImageRevisionValues() {
		result.ImageRevisions = append(result.ImageRevisions, encodeImageRevision(item))
	}
	for _, item := range binding.TargetChangeEvents() {
		result.TargetChanges = append(result.TargetChanges, persistedTargetChange{At: item.At.UTC().Format(time.RFC3339Nano), Kind: string(item.Kind), Detail: item.Detail})
	}
	return result
}
func decodeBinding(binding persistedBinding) (domain.ObservationBinding, error) {
	if len(binding.ResolvedTargets) > maxTargets || len(binding.ImageRevisions) > maxRevisions || len(binding.TargetChanges) > maxTargetChanges {
		return domain.ObservationBinding{}, fmt.Errorf("%w: observation binding exceeds bounds", domain.ErrInvalidDomainValue)
	}
	instances := make([]domain.RuntimeContainerInstance, 0, len(binding.ResolvedTargets))
	for _, item := range binding.ResolvedTargets {
		decoded, err := decodeRuntime(item)
		if err != nil {
			return domain.ObservationBinding{}, err
		}
		instances = append(instances, decoded)
	}
	targets, err := domain.NewResolvedTargetSet(instances)
	if err != nil {
		return domain.ObservationBinding{}, err
	}
	revisions := make([]domain.ContainerImageRevision, 0, len(binding.ImageRevisions))
	for _, item := range binding.ImageRevisions {
		decoded, err := decodeImageRevision(item)
		if err != nil {
			return domain.ObservationBinding{}, err
		}
		revisions = append(revisions, decoded)
	}
	changes := make([]domain.TargetChangeEvent, 0, len(binding.TargetChanges))
	for _, item := range binding.TargetChanges {
		at, err := time.Parse(time.RFC3339Nano, item.At)
		if err != nil {
			return domain.ObservationBinding{}, fmt.Errorf("%w: invalid target change time", domain.ErrInvalidDomainValue)
		}
		decoded, err := domain.NewTargetChangeEvent(at, domain.TargetChangeKind(item.Kind), item.Detail)
		if err != nil {
			return domain.ObservationBinding{}, err
		}
		changes = append(changes, decoded)
	}
	return domain.ObservationBinding{ResolvedTargets: targets, Backend: domain.BackendIdentity{Kind: binding.Backend.Kind, Version: binding.Backend.Version}, ImageRevisions: revisions, TargetChanges: changes}, nil
}

func encodeExecution(execution domain.ObservationExecution) persistedExecution {
	result := persistedExecution{State: string(execution.State), Completion: string(execution.Completion)}
	if !execution.StartedAt.IsZero() {
		result.StartedAt = execution.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if !execution.CompletedAt.IsZero() {
		result.CompletedAt = execution.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	if execution.StopIntent != nil {
		result.StopRequester = execution.StopIntent.Requester
		result.StopContextVersion = execution.StopIntent.ContextVersion
		result.StopExecutorID = execution.StopIntent.ExecutorID
		result.StopClaimGeneration = execution.StopIntent.ClaimGeneration
		if !execution.StopIntent.RequestedAt.IsZero() {
			result.StopRequestedAt = execution.StopIntent.RequestedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	if execution.Failure != nil {
		result.Failure = &persistedFailure{Stage: execution.Failure.Stage, Code: execution.Failure.Code, Reason: execution.Failure.Reason, Source: execution.Failure.Source, Retryable: execution.Failure.Retryable, ExecutorID: execution.Failure.ExecutorID, ClaimGeneration: execution.Failure.ClaimGeneration}
		if !execution.Failure.OccurredAt.IsZero() {
			result.Failure.OccurredAt = execution.Failure.OccurredAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return result
}
func decodeExecution(execution persistedExecution) (domain.ObservationExecution, error) {
	result := domain.ObservationExecution{State: domain.ExecutionState(execution.State), Completion: domain.CompletionReason(execution.Completion)}
	if !result.State.Valid() {
		return result, fmt.Errorf("%w: invalid execution state", domain.ErrInvalidDomainValue)
	}
	for value, target := range map[string]*time.Time{execution.StartedAt: &result.StartedAt, execution.CompletedAt: &result.CompletedAt} {
		if value != "" {
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return result, fmt.Errorf("%w: invalid execution timestamp", domain.ErrInvalidDomainValue)
			}
			*target = parsed
		}
	}
	if execution.StopRequestedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, execution.StopRequestedAt)
		if err != nil {
			return result, fmt.Errorf("%w: invalid stop intent timestamp", domain.ErrInvalidDomainValue)
		}
		result.StopIntent = &domain.StopIntent{RequestedAt: parsed, Requester: execution.StopRequester, ContextVersion: execution.StopContextVersion, ExecutorID: execution.StopExecutorID, ClaimGeneration: execution.StopClaimGeneration}
	}
	if execution.Failure != nil {
		occurred, err := time.Parse(time.RFC3339Nano, execution.Failure.OccurredAt)
		if err != nil {
			return result, fmt.Errorf("%w: invalid failure timestamp", domain.ErrInvalidDomainValue)
		}
		result.Failure = &domain.FailureInfo{Stage: execution.Failure.Stage, Code: execution.Failure.Code, Reason: execution.Failure.Reason, Source: execution.Failure.Source, OccurredAt: occurred, Retryable: execution.Failure.Retryable, ExecutorID: execution.Failure.ExecutorID, ClaimGeneration: execution.Failure.ClaimGeneration}
	}
	if result.State == domain.ExecutionCompleted || result.State == domain.ExecutionFailed {
		if result.Completion == "" {
			return result, fmt.Errorf("%w: missing terminal reason", domain.ErrInvalidDomainValue)
		}
	}
	return result, nil
}

func encodeResult(result domain.ObservationResult) persistedResult {
	output := persistedResult{}
	for _, source := range result.Sources() {
		item := persistedSource{Name: source.Source.Name, Backend: source.Source.Backend, Version: source.Source.Version, Evidence: string(source.Evidence), References: append([]string(nil), source.References...), Facts: encodeFacts(source.Facts)}
		item.Qualification = persistedQualification{BackendHealthConfirmed: source.Qualification.BackendHealthConfirmed, SourceAttachedForBoundWindow: source.Qualification.SourceAttachedForBoundWindow, FlushConfirmed: source.Qualification.FlushConfirmed, Attribution: string(source.Qualification.Attribution), AttributedCount: source.Qualification.AttributedCount, ExcludedCount: source.Qualification.ExcludedCount}
		output.Sources = append(output.Sources, item)
	}
	return output
}

func encodeFacts(f domain.NormalizedFacts) persistedFacts {
	out := persistedFacts{}
	for _, item := range f.Filesystem {
		permissions := make([]string, len(item.Permissions))
		for i, p := range item.Permissions {
			permissions[i] = string(p)
		}
		out.Filesystem = append(out.Filesystem, persistedFilesystemFact{Path: item.Path, Permissions: permissions})
	}
	for _, item := range f.Exec {
		out.Exec = append(out.Exec, persistedExecFact{Path: item.Path})
	}
	for _, item := range f.NetworkConnect {
		out.NetworkConnect = append(out.NetworkConnect, persistedNetworkFact{Port: item.Port, Direction: string(item.Direction)})
	}
	for _, item := range f.NetworkBind {
		out.NetworkBind = append(out.NetworkBind, persistedNetworkFact{Port: item.Port, Direction: string(item.Direction)})
	}
	for _, item := range f.Capabilities {
		out.Capabilities = append(out.Capabilities, persistedCapabilityFact{Name: item.Name})
	}
	return out
}

func decodeFacts(f persistedFacts) domain.NormalizedFacts {
	out := domain.NormalizedFacts{}
	for _, item := range f.Filesystem {
		permissions := make([]profile.FilePermission, len(item.Permissions))
		for i, p := range item.Permissions {
			permissions[i] = profile.FilePermission(p)
		}
		out.Filesystem = append(out.Filesystem, domain.FilesystemFact{Path: item.Path, Permissions: permissions})
	}
	for _, item := range f.Exec {
		out.Exec = append(out.Exec, domain.ExecFact{Path: item.Path})
	}
	for _, item := range f.NetworkConnect {
		out.NetworkConnect = append(out.NetworkConnect, domain.NetworkFact{Port: item.Port, Direction: profile.NetworkDirection(item.Direction)})
	}
	for _, item := range f.NetworkBind {
		out.NetworkBind = append(out.NetworkBind, domain.NetworkFact{Port: item.Port, Direction: profile.NetworkDirection(item.Direction)})
	}
	for _, item := range f.Capabilities {
		out.Capabilities = append(out.Capabilities, domain.CapabilityFact{Name: item.Name})
	}
	return out
}
func decodeResult(result persistedResult) (domain.ObservationResult, error) {
	if len(result.Sources) > maxSourceResults {
		return domain.ObservationResult{}, fmt.Errorf("%w: too many source results", domain.ErrInvalidDomainValue)
	}
	sources := make([]domain.SourceResult, 0, len(result.Sources))
	for _, item := range result.Sources {
		if len(item.Name) > maxSourceName || len(item.References) > maxReferences {
			return domain.ObservationResult{}, fmt.Errorf("%w: source result exceeds bounds", domain.ErrInvalidDomainValue)
		}
		qualification := domain.SourceQualification{BackendHealthConfirmed: item.Qualification.BackendHealthConfirmed, SourceAttachedForBoundWindow: item.Qualification.SourceAttachedForBoundWindow, FlushConfirmed: item.Qualification.FlushConfirmed, Attribution: domain.AttributionState(item.Qualification.Attribution), AttributedCount: item.Qualification.AttributedCount, ExcludedCount: item.Qualification.ExcludedCount}
		source, err := domain.NewSourceResult(domain.EvidenceSource{Name: item.Name, Backend: item.Backend, Version: item.Version}, qualification, item.References, decodeFacts(item.Facts))
		if err != nil {
			return domain.ObservationResult{}, err
		}
		if item.Evidence != string(source.Evidence) {
			return domain.ObservationResult{}, fmt.Errorf("%w: persisted evidence state disagrees with qualification", domain.ErrInvalidDomainValue)
		}
		sources = append(sources, source)
	}
	return domain.NewObservationResult(sources)
}

func encodeProvenance(provenance domain.ObservationProvenance) *persistedProvenance {
	result := &persistedProvenance{Backend: persistedBackend{Kind: provenance.Backend.Kind, Version: provenance.Backend.Version}, RequestedSources: append([]string(nil), provenance.RequestedSources...)}
	for _, item := range provenance.ResolvedTargets.Items() {
		result.ResolvedTargets = append(result.ResolvedTargets, encodeRuntime(item))
	}
	for _, item := range provenance.ImageRevisions {
		result.ImageRevisions = append(result.ImageRevisions, encodeImageRevision(item))
	}
	return result
}
func decodeProvenance(provenance *persistedProvenance) (domain.ObservationProvenance, error) {
	if provenance == nil {
		return domain.ObservationProvenance{}, nil
	}
	if len(provenance.ResolvedTargets) > maxTargets || len(provenance.ImageRevisions) > maxRevisions || len(provenance.RequestedSources) > maxSources {
		return domain.ObservationProvenance{}, fmt.Errorf("%w: observation provenance exceeds bounds", domain.ErrInvalidDomainValue)
	}
	instances := make([]domain.RuntimeContainerInstance, 0, len(provenance.ResolvedTargets))
	for _, item := range provenance.ResolvedTargets {
		decoded, err := decodeRuntime(item)
		if err != nil {
			return domain.ObservationProvenance{}, err
		}
		instances = append(instances, decoded)
	}
	targets, err := domain.NewResolvedTargetSet(instances)
	if err != nil {
		return domain.ObservationProvenance{}, err
	}
	revisions := make([]domain.ContainerImageRevision, 0, len(provenance.ImageRevisions))
	for _, item := range provenance.ImageRevisions {
		decoded, err := decodeImageRevision(item)
		if err != nil {
			return domain.ObservationProvenance{}, err
		}
		revisions = append(revisions, decoded)
	}
	return domain.ObservationProvenance{ResolvedTargets: targets, ImageRevisions: revisions, Backend: domain.BackendIdentity{Kind: provenance.Backend.Kind, Version: provenance.Backend.Version}, RequestedSources: append([]string(nil), provenance.RequestedSources...)}, nil
}

func encodeStatus(observation domain.Observation) (map[string]interface{}, error) {
	status := persistedStatus{Binding: encodeBinding(observation.Binding()), Execution: encodeExecution(observation.Execution()), Result: encodeResult(observation.Result())}
	if observation.Frozen() {
		status.Provenance = encodeProvenance(observation.Provenance())
	}
	return toMap(status)
}

func encodeStatusWithClaim(observation domain.Observation, claim claimRecord) (map[string]interface{}, error) {
	status := persistedStatus{Binding: encodeBinding(observation.Binding()), Execution: encodeExecution(observation.Execution()), Result: encodeResult(observation.Result())}
	status.Execution.ExecutorID = claim.ExecutorID
	status.Execution.ClaimGeneration = claim.Generation
	status.Execution.LeaseExpiry = claim.LeaseExpiry.UTC().Format(time.RFC3339Nano)
	if observation.Frozen() {
		status.Provenance = encodeProvenance(observation.Provenance())
	}
	return toMap(status)
}
func decodeStatus(value interface{}) (domain.ObservationBinding, domain.ObservationExecution, domain.ObservationResult, domain.ObservationProvenance, error) {
	var status persistedStatus
	if err := fromMap(value, &status); err != nil {
		return domain.ObservationBinding{}, domain.ObservationExecution{}, domain.ObservationResult{}, domain.ObservationProvenance{}, err
	}
	binding, err := decodeBinding(status.Binding)
	if err != nil {
		return domain.ObservationBinding{}, domain.ObservationExecution{}, domain.ObservationResult{}, domain.ObservationProvenance{}, err
	}
	execution, err := decodeExecution(status.Execution)
	if err != nil {
		return domain.ObservationBinding{}, domain.ObservationExecution{}, domain.ObservationResult{}, domain.ObservationProvenance{}, err
	}
	result, err := decodeResult(status.Result)
	if err != nil {
		return domain.ObservationBinding{}, domain.ObservationExecution{}, domain.ObservationResult{}, domain.ObservationProvenance{}, err
	}
	provenance, err := decodeProvenance(status.Provenance)
	return binding, execution, result, provenance, err
}

// ToUnstructured converts a domain Observation to its bounded persistence
// shape. Status is included only for status updates; CreateObservation uses
// the spec-only helper in the store.
func ToUnstructured(observation domain.Observation, namespace string) (*unstructured.Unstructured, error) {
	if !observation.ID().Valid() || len(validation.IsDNS1123Subdomain(string(observation.ID()))) != 0 {
		return nil, fmt.Errorf("%w: ObservationID is not a Kubernetes name", domain.ErrInvalidDomainValue)
	}
	spec, err := toMap(encodeSpec(observation.Spec()))
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]interface{}{"name": string(observation.ID()), "namespace": namespace}, "spec": spec}}, nil
}

// FromUnstructured validates and reconstructs a domain Observation. Missing
// status means the initial REQUESTED execution created by the domain model.
func FromUnstructured(object *unstructured.Unstructured) (domain.Observation, error) {
	if object == nil {
		return domain.Observation{}, fmt.Errorf("%w: nil Observation", domain.ErrInvalidDomainValue)
	}
	if object.GetAPIVersion() != apiVersion || object.GetKind() != kind {
		return domain.Observation{}, fmt.Errorf("%w: unexpected Observation GVK", domain.ErrInvalidDomainValue)
	}
	name := object.GetName()
	if name == "" {
		return domain.Observation{}, fmt.Errorf("%w: missing Observation name", domain.ErrInvalidDomainValue)
	}
	var rawSpec interface{}
	var ok bool
	if rawSpec, ok, _ = unstructured.NestedFieldNoCopy(object.Object, "spec"); !ok {
		return domain.Observation{}, fmt.Errorf("%w: missing Observation spec", domain.ErrInvalidDomainValue)
	}
	var spec persistedSpec
	if err := fromMap(rawSpec, &spec); err != nil {
		return domain.Observation{}, err
	}
	decodedSpec, err := decodeSpec(spec)
	if err != nil {
		return domain.Observation{}, err
	}
	id := domain.ObservationID(name)
	if rawStatus, exists, _ := unstructured.NestedFieldNoCopy(object.Object, "status"); exists {
		binding, execution, result, provenance, err := decodeStatus(rawStatus)
		if err != nil {
			return domain.Observation{}, err
		}
		return domain.RestoreObservation(id, decodedSpec, binding, execution, result, provenance)
	}
	return domain.NewObservation(id, decodedSpec)
}

func sameTargetChangePrefix(old, current []domain.TargetChangeEvent) bool {
	if len(current) < len(old) {
		return false
	}
	for i := range old {
		if old[i] != current[i] {
			return false
		}
	}
	return true
}

// ValidateStatusMutation enforces adapter-visible invariants. Executor
// ownership/fencing is intentionally deferred to G4.
func ValidateStatusMutation(oldObservation, newObservation domain.Observation) error {
	if oldObservation.ID() != newObservation.ID() || !reflect.DeepEqual(oldObservation.Spec(), newObservation.Spec()) {
		return fmt.Errorf("%w: Observation identity/spec changed", domain.ErrInvalidDomainValue)
	}
	if !sameTargetChangePrefix(oldObservation.Binding().TargetChangeEvents(), newObservation.Binding().TargetChangeEvents()) {
		return fmt.Errorf("%w: target-change history is not append-only", domain.ErrInvalidDomainValue)
	}
	if oldObservation.Frozen() {
		if oldObservation.Execution() != newObservation.Execution() || oldObservation.Frozen() != newObservation.Frozen() {
			return domain.ErrObservationFrozen
		}
		if !reflect.DeepEqual(oldObservation.Binding(), newObservation.Binding()) || !reflect.DeepEqual(oldObservation.Result(), newObservation.Result()) || !reflect.DeepEqual(oldObservation.Provenance(), newObservation.Provenance()) {
			return domain.ErrObservationFrozen
		}
	}
	return nil
}
