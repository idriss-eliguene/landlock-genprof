// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package kubernetes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrAlreadyClaimed      = errors.New("observation already has an executor claim")
	ErrConcurrentConflict  = errors.New("observation persistence conflict")
	ErrStaleExecutor       = errors.New("stale executor authority")
	ErrLeaseExpired        = errors.New("executor lease expired")
	ErrTerminalObservation = errors.New("terminal observation cannot be mutated")
	ErrObservationNotFound = errors.New("observation not found")
	ErrInvalidExecutor     = errors.New("invalid executor identity")
	ErrRecoveryNotEligible = errors.New("observation is not eligible for lost-executor recovery")
	ErrStopNotEligible     = errors.New("observation is not eligible for stopping")
)

const DefaultLeaseDuration = 30 * time.Second

// ExecutorClaim is the complete authority token held by an executor. It is
// not Observation identity and never contains resourceVersion.
type ExecutorClaim struct {
	ObservationID   domain.ObservationID
	ExecutorID      string
	ClaimGeneration uint64
}

type Clock interface{ Now() time.Time }
type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// NewExecutorID creates an opaque per-executor-instance identifier.
func NewExecutorID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating executor ID: %w", err)
	}
	return "exec-" + hex.EncodeToString(bytes), nil
}

type claimRecord struct {
	ExecutorID  string
	Generation  uint64
	LeaseExpiry time.Time
}

func claimFromObject(object *unstructured.Unstructured) (claimRecord, error) {
	status, exists, err := nestedField(object.UnstructuredContent(), "status", "execution")
	if err != nil {
		return claimRecord{}, err
	}
	if !exists {
		return claimRecord{}, nil
	}
	values, ok := status.(map[string]interface{})
	if !ok {
		return claimRecord{}, fmt.Errorf("%w: malformed execution status", domain.ErrInvalidDomainValue)
	}
	var claim claimRecord
	if value, ok := values["executorID"].(string); ok {
		claim.ExecutorID = value
	}
	if value, ok := values["claimGeneration"]; ok {
		switch number := value.(type) {
		case int64:
			if number < 0 {
				return claimRecord{}, fmt.Errorf("%w: invalid claim generation", domain.ErrInvalidDomainValue)
			}
			claim.Generation = uint64(number)
		case int:
			if number < 0 {
				return claimRecord{}, fmt.Errorf("%w: invalid claim generation", domain.ErrInvalidDomainValue)
			}
			claim.Generation = uint64(number)
		case float64:
			if number < 0 || number != float64(uint64(number)) {
				return claimRecord{}, fmt.Errorf("%w: invalid claim generation", domain.ErrInvalidDomainValue)
			}
			claim.Generation = uint64(number)
		default:
			return claimRecord{}, fmt.Errorf("%w: invalid claim generation", domain.ErrInvalidDomainValue)
		}
	}
	if value, ok := values["leaseExpiry"].(string); ok && value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return claimRecord{}, fmt.Errorf("%w: invalid lease expiry", domain.ErrInvalidDomainValue)
		}
		claim.LeaseExpiry = parsed
	}
	if claim.ExecutorID == "" && claim.Generation == 0 && claim.LeaseExpiry.IsZero() {
		return claimRecord{}, nil
	}
	if claim.ExecutorID == "" || claim.Generation == 0 || claim.LeaseExpiry.IsZero() {
		return claimRecord{}, fmt.Errorf("%w: incomplete executor claim", domain.ErrInvalidDomainValue)
	}
	return claim, nil
}

func nestedField(object map[string]interface{}, fields ...string) (interface{}, bool, error) {
	var current interface{} = object
	for _, field := range fields {
		values, ok := current.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("%w: malformed persisted object", domain.ErrInvalidDomainValue)
		}
		current, ok = values[field]
		if !ok {
			return nil, false, nil
		}
	}
	return current, true, nil
}

func (s *Store) currentRecord(ctx context.Context, namespace, name string) (*unstructuredRecord, error) {
	object, err := s.client.Resource(GVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s/%s", ErrObservationNotFound, namespace, name)
		}
		return nil, fmt.Errorf("reading Observation %s/%s: %w", namespace, name, err)
	}
	observation, err := FromUnstructured(object)
	if err != nil {
		return nil, err
	}
	claim, err := claimFromObject(object)
	if err != nil {
		return nil, err
	}
	return &unstructuredRecord{object: object, observation: observation, claim: claim, resourceVersion: object.GetResourceVersion()}, nil
}

type unstructuredRecord struct {
	object          *unstructured.Unstructured
	observation     domain.Observation
	claim           claimRecord
	resourceVersion string
}

func (s *Store) casStatus(ctx context.Context, record *unstructuredRecord, expectedRV string, status map[string]interface{}) (string, error) {
	if expectedRV == "" {
		return "", fmt.Errorf("resourceVersion is required")
	}
	copy := record.object.DeepCopy()
	copy.SetResourceVersion(expectedRV)
	copy.Object["status"] = status
	updated, err := s.client.Resource(GVR).Namespace(copy.GetNamespace()).UpdateStatus(ctx, copy, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsConflict(err) {
			return "", ErrConcurrentConflict
		}
		return "", fmt.Errorf("updating Observation status: %w", err)
	}
	return updated.GetResourceVersion(), nil
}

func (s *Store) authority(ctx context.Context, namespace string, claim ExecutorClaim, expectedRV string, now time.Time) (*unstructuredRecord, error) {
	if claim.ExecutorID == "" || claim.ClaimGeneration == 0 {
		return nil, ErrStaleExecutor
	}
	record, err := s.currentRecord(ctx, namespace, string(claim.ObservationID))
	if err != nil {
		return nil, err
	}
	if expectedRV == "" || record.resourceVersion != expectedRV {
		return nil, ErrConcurrentConflict
	}
	if record.claim.ExecutorID != claim.ExecutorID || record.claim.Generation != claim.ClaimGeneration {
		return nil, ErrStaleExecutor
	}
	if record.observation.Frozen() {
		return nil, ErrTerminalObservation
	}
	if record.claim.LeaseExpiry.IsZero() || !now.Before(record.claim.LeaseExpiry) {
		return nil, ErrLeaseExpired
	}
	return record, nil
}

// ClaimObservation atomically claims a REQUESTED Observation and advances it
// to STARTING. A CAS loser receives ErrAlreadyClaimed or ErrConcurrentConflict;
// it is never retried as a takeover.
func (s *Store) ClaimObservation(ctx context.Context, namespace, name, executorID string) (ExecutorClaim, string, error) {
	if executorID == "" {
		return ExecutorClaim{}, "", ErrInvalidExecutor
	}
	record, err := s.currentRecord(ctx, namespace, name)
	if err != nil {
		return ExecutorClaim{}, "", err
	}
	if record.observation.Execution().State != domain.ExecutionRequested || record.claim.ExecutorID != "" {
		return ExecutorClaim{}, "", ErrAlreadyClaimed
	}
	if err := record.observation.Transition(domain.ExecutionStarting, ""); err != nil {
		return ExecutorClaim{}, "", err
	}
	claim := ExecutorClaim{ObservationID: domain.ObservationID(name), ExecutorID: executorID, ClaimGeneration: record.claim.Generation + 1}
	lease := s.clock.Now().Add(DefaultLeaseDuration)
	status, err := encodeStatusWithClaim(record.observation, claimRecord{ExecutorID: executorID, Generation: claim.ClaimGeneration, LeaseExpiry: lease})
	if err != nil {
		return ExecutorClaim{}, "", err
	}
	rv, err := s.casStatus(ctx, record, record.resourceVersion, status)
	if err != nil {
		return ExecutorClaim{}, "", err
	}
	return claim, rv, nil
}

// StopIntentInput is authenticated request context. The store persists only
// bounded audit data and the current claim fence; credentials never enter it.
type StopIntentInput struct {
	Requester      string
	ContextVersion uint64
}

// RequestStop durably records cancellation intent with a resourceVersion CAS.
// It deliberately does not change lifecycle state or result data. An intent
// requested before a claim is unbound; the executor that successfully claims
// that REQUESTED observation is then the only process allowed to consume it.
func (s *Store) RequestStop(ctx context.Context, namespace, name string, input StopIntentInput) (domain.Observation, string, error) {
	if input.Requester == "" {
		return domain.Observation{}, "", ErrInvalidExecutor
	}
	record, err := s.currentRecord(ctx, namespace, name)
	if err != nil {
		return domain.Observation{}, "", err
	}
	if record.observation.Execution().StopRequested() {
		return record.observation, record.resourceVersion, nil
	}
	if !record.observation.CanRequestStop() {
		return record.observation, record.resourceVersion, ErrStopNotEligible
	}
	intent := domain.StopIntent{RequestedAt: s.clock.Now(), Requester: input.Requester, ContextVersion: input.ContextVersion}
	if record.claim.ExecutorID != "" {
		if record.claim.LeaseExpiry.IsZero() || !s.clock.Now().Before(record.claim.LeaseExpiry) {
			return domain.Observation{}, "", ErrLeaseExpired
		}
		intent.ExecutorID, intent.ClaimGeneration = record.claim.ExecutorID, record.claim.Generation
	}
	if err := record.observation.RequestStop(intent); err != nil {
		return domain.Observation{}, "", err
	}
	status, err := encodeStatusWithClaim(record.observation, record.claim)
	if err != nil {
		return domain.Observation{}, "", err
	}
	rv, err := s.casStatus(ctx, record, record.resourceVersion, status)
	if err != nil {
		return domain.Observation{}, "", err
	}
	return record.observation, rv, nil
}

// StopRequestedForClaim is the executor control-plane read. A stale or
// expired claim can never consume another execution's intent.
func (s *Store) StopRequestedForClaim(ctx context.Context, namespace string, claim ExecutorClaim) (bool, error) {
	record, err := s.currentRecord(ctx, namespace, string(claim.ObservationID))
	if err != nil {
		return false, err
	}
	if record.observation.Frozen() {
		return false, nil
	}
	if record.claim.ExecutorID != claim.ExecutorID || record.claim.Generation != claim.ClaimGeneration || record.claim.LeaseExpiry.IsZero() || !s.clock.Now().Before(record.claim.LeaseExpiry) {
		return false, ErrStaleExecutor
	}
	intent := record.observation.Execution().StopIntent
	if intent == nil {
		return false, nil
	}
	if intent.ExecutorID != "" && (intent.ExecutorID != claim.ExecutorID || intent.ClaimGeneration != claim.ClaimGeneration) {
		return false, ErrStaleExecutor
	}
	return true, nil
}

// TerminalizeExecutorLost is the only recovery operation. It transfers no
// live authority: an expired non-terminal observation is terminalized as
// FAILED/EXECUTOR_LOST using one resourceVersion-bound status CAS.
func (s *Store) TerminalizeExecutorLost(ctx context.Context, namespace, name string) (string, error) {
	record, err := s.currentRecord(ctx, namespace, name)
	if err != nil {
		return "", err
	}
	if record.observation.Frozen() {
		return "", ErrTerminalObservation
	}
	if record.claim.ExecutorID == "" {
		return "", ErrRecoveryNotEligible
	}
	if s.clock.Now().Before(record.claim.LeaseExpiry) {
		return "", ErrLeaseExpired
	}
	execution := record.observation.Execution()
	execution.State = domain.ExecutionFailed
	execution.Completion = domain.ExecutorLost
	execution.CompletedAt = s.clock.Now()
	execution.Failure = &domain.FailureInfo{Stage: "EXECUTOR_LOSS", Code: "EXECUTOR_LOST", Reason: "the executor lease expired before observation finalization", Source: "executor", OccurredAt: s.clock.Now(), Retryable: true, ExecutorID: record.claim.ExecutorID, ClaimGeneration: record.claim.Generation}
	binding := record.observation.Binding()
	provenance := domain.ObservationProvenance{ResolvedTargets: binding.ResolvedTargets, ImageRevisions: binding.ImageRevisionValues(), Backend: binding.Backend, RequestedSources: record.observation.Spec().SourceNames()}
	lost, err := domain.RestoreObservation(record.observation.ID(), record.observation.Spec(), binding, execution, record.observation.Result(), provenance)
	if err != nil {
		return "", err
	}
	status, err := encodeStatusWithClaim(lost, record.claim)
	if err != nil {
		return "", err
	}
	rv, err := s.casStatus(ctx, record, record.resourceVersion, status)
	if err != nil {
		return "", err
	}
	return rv, nil
}

func (s *Store) RenewLease(ctx context.Context, namespace string, claim ExecutorClaim, expectedRV string) (string, error) {
	// Lease renewal is allowed to recover one resourceVersion race caused by a
	// legitimate same-owner mutation (for example, durable Stop intent). The
	// fresh read below revalidates the executor fence before retrying; this is
	// not a blind retry and never recovers a changed executor or generation.
	for attempt := 0; attempt < 2; attempt++ {
		record, err := s.currentRecord(ctx, namespace, string(claim.ObservationID))
		if err != nil {
			return "", err
		}
		if record.claim.ExecutorID != claim.ExecutorID || record.claim.Generation != claim.ClaimGeneration {
			return "", ErrStaleExecutor
		}
		if record.observation.Frozen() {
			return "", ErrTerminalObservation
		}
		if record.claim.LeaseExpiry.IsZero() || !s.clock.Now().Before(record.claim.LeaseExpiry) {
			return "", ErrLeaseExpired
		}
		lease := s.clock.Now().Add(DefaultLeaseDuration)
		status, err := encodeStatusWithClaim(record.observation, claimRecord{ExecutorID: claim.ExecutorID, Generation: claim.ClaimGeneration, LeaseExpiry: lease})
		if err != nil {
			return "", err
		}
		nextRV, err := s.casStatus(ctx, record, record.resourceVersion, status)
		if err == nil {
			return nextRV, nil
		}
		if !errors.Is(err, ErrConcurrentConflict) || attempt == 1 {
			return "", err
		}
		// A concurrent writer advanced the object after the authoritative read.
		// The next iteration reloads and revalidates the complete fence.
	}
	return "", ErrConcurrentConflict
}

func (s *Store) TransitionExecution(ctx context.Context, namespace string, claim ExecutorClaim, expectedRV string, next domain.ExecutionState, reason domain.CompletionReason) (string, error) {
	record, err := s.authority(ctx, namespace, claim, expectedRV, s.clock.Now())
	if err != nil {
		return "", err
	}
	if err := record.observation.Transition(next, reason); err != nil {
		return "", err
	}
	status, err := encodeStatusWithClaim(record.observation, record.claim)
	if err != nil {
		return "", err
	}
	return s.casStatus(ctx, record, expectedRV, status)
}

// UpdateExecutorStatus is the fenced result/binding mutation. Execution state
// transitions use TransitionExecution so the domain graph remains central.
func (s *Store) UpdateExecutorStatus(ctx context.Context, namespace string, claim ExecutorClaim, expectedRV string, observation domain.Observation) (string, error) {
	record, err := s.authority(ctx, namespace, claim, expectedRV, s.clock.Now())
	if err != nil {
		return "", err
	}
	if observation.ID() != record.observation.ID() || observation.Execution().State != record.observation.Execution().State || observation.Frozen() {
		return "", domain.ErrInvalidTransition
	}
	if err := ValidateStatusMutation(record.observation, observation); err != nil {
		return "", err
	}
	status, err := encodeStatusWithClaim(observation, record.claim)
	if err != nil {
		return "", err
	}
	return s.casStatus(ctx, record, expectedRV, status)
}
