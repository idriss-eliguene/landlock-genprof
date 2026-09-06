package history

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const contributionReceiptAPIVersion = "landlockgenprof.io/v1alpha1"

var contributionReceiptGVR = schema.GroupVersionResource{Group: apiGroup, Version: apiVersion, Resource: "observationcontributionreceipts"}

const receiptInitializationReadAttempts = 5

var errReceiptInitializing = errors.New("contribution receipt status initialization pending")

type ReceiptState string

const (
	ReceiptPrepared  ReceiptState = "PREPARED"
	ReceiptCommitted ReceiptState = "COMMITTED"
)

type ObservationContributionReceipt struct {
	ObservationID            string                `json:"observationID"`
	Population               PopulationFingerprint `json:"populationFingerprint"`
	TrainingHistoryNamespace string                `json:"trainingHistoryNamespace"`
	TrainingHistoryName      string                `json:"trainingHistoryName"`
	ContributionKeyDigest    string                `json:"contributionKeyDigest"`
	ContentDigest            string                `json:"contentDigest,omitempty"`
	State                    ReceiptState          `json:"state"`
}

func (r ObservationContributionReceipt) Validate() error {
	key := ContributionKey{ObservationID: r.ObservationID, Population: r.Population}
	if !key.Valid() || strings.TrimSpace(r.TrainingHistoryNamespace) == "" || strings.TrimSpace(r.TrainingHistoryName) == "" || r.State != ReceiptPrepared && r.State != ReceiptCommitted {
		return fmt.Errorf("%w: invalid receipt", ErrInvalidContribution)
	}
	digest, err := key.Digest()
	if err != nil || digest != r.ContributionKeyDigest {
		return fmt.Errorf("%w: contribution key digest mismatch", ErrReceiptIdentityMismatch)
	}
	if r.ContentDigest != "" && !isSHA256(r.ContentDigest) {
		return fmt.Errorf("%w: invalid content digest", ErrInvalidContribution)
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func receiptToUnstructured(namespace, name string, receipt ObservationContributionReceipt) *unstructured.Unstructured {
	population := map[string]interface{}{"target": receipt.Population.Target, "container": receipt.Population.Container, "imageIdentity": receipt.Population.ImageIdentity}
	if receipt.Population.Scope != "" {
		population["scope"] = string(receipt.Population.Scope)
	}
	if receipt.Population.BinaryPath != "" {
		population["binaryPath"] = receipt.Population.BinaryPath
	}
	spec := map[string]interface{}{
		"observationID":            receipt.ObservationID,
		"populationFingerprint":    population,
		"trainingHistoryNamespace": receipt.TrainingHistoryNamespace, "trainingHistoryName": receipt.TrainingHistoryName,
		"contributionKeyDigest": receipt.ContributionKeyDigest,
	}
	if receipt.ContentDigest != "" {
		spec["contentDigest"] = receipt.ContentDigest
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": contributionReceiptAPIVersion, "kind": "ObservationContributionReceipt",
		"metadata": map[string]interface{}{"name": name, "namespace": namespace},
		"spec":     spec,
		"status":   map[string]interface{}{"state": string(receipt.State)},
	}}
}

func receiptFromUnstructured(obj *unstructured.Unstructured) (ObservationContributionReceipt, error) {
	var r ObservationContributionReceipt
	r.ObservationID, _, _ = unstructured.NestedString(obj.Object, "spec", "observationID")
	r.TrainingHistoryNamespace, _, _ = unstructured.NestedString(obj.Object, "spec", "trainingHistoryNamespace")
	r.TrainingHistoryName, _, _ = unstructured.NestedString(obj.Object, "spec", "trainingHistoryName")
	r.ContributionKeyDigest, _, _ = unstructured.NestedString(obj.Object, "spec", "contributionKeyDigest")
	r.ContentDigest, _, _ = unstructured.NestedString(obj.Object, "spec", "contentDigest")
	fingerprint, found, err := unstructured.NestedMap(obj.Object, "spec", "populationFingerprint")
	if err != nil || !found {
		return r, fmt.Errorf("%w: missing population fingerprint", ErrInvalidContribution)
	}
	r.Population.Target, _, _ = unstructured.NestedString(fingerprint, "target")
	r.Population.Container, _, _ = unstructured.NestedString(fingerprint, "container")
	r.Population.ImageIdentity, _, _ = unstructured.NestedString(fingerprint, "imageIdentity")
	r.Population.BinaryPath, _, _ = unstructured.NestedString(fingerprint, "binaryPath")
	scope, _, _ := unstructured.NestedString(fingerprint, "scope")
	r.Population.Scope = PopulationScope(scope)
	if normalized, normalizeErr := r.Population.normalized(); normalizeErr != nil {
		return r, normalizeErr
	} else {
		r.Population = normalized
	}
	state, foundState, _ := unstructured.NestedString(obj.Object, "status", "state")
	if !foundState || strings.TrimSpace(state) == "" {
		// CREATE and the status-subresource initialization are separate API
		// operations.  Treat only an otherwise valid receipt with no state as
		// transient initialization; malformed spec fields remain fatal.
		initializing := r
		initializing.State = ReceiptPrepared
		if initializing.Validate() == nil {
			return r, errReceiptInitializing
		}
	}
	r.State = ReceiptState(state)
	if err := r.Validate(); err != nil {
		return r, err
	}
	return r, nil
}

type ReceiptStore struct{ client dynamic.Interface }

func NewReceiptStore(client dynamic.Interface) (*ReceiptStore, error) {
	if client == nil {
		return nil, fmt.Errorf("receipt store requires client")
	}
	return &ReceiptStore{client: client}, nil
}

func (s *ReceiptStore) CreatePrepared(ctx context.Context, namespace string, key ContributionKey, historyNamespace, historyName, contentDigest string) (ObservationContributionReceipt, string, error) {
	name, err := key.ReceiptName()
	if err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	normalizedPopulation, err := key.Population.normalized()
	if err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	key.Population = normalizedPopulation
	digest, _ := key.Digest()
	receipt := ObservationContributionReceipt{ObservationID: key.ObservationID, Population: key.Population, TrainingHistoryNamespace: historyNamespace, TrainingHistoryName: historyName, ContributionKeyDigest: digest, ContentDigest: contentDigest, State: ReceiptPrepared}
	if err := receipt.Validate(); err != nil {
		return receipt, "", err
	}
	obj, err := s.client.Resource(contributionReceiptGVR).Namespace(namespace).Create(ctx, receiptToUnstructured(namespace, name, receipt), metav1.CreateOptions{})
	if err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	// Status is a subresource and API servers may discard it during CREATE;
	// establish PREPARED explicitly before exposing the receipt to callers.
	if err := unstructured.SetNestedField(obj.Object, string(ReceiptPrepared), "status", "state"); err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	obj, err = s.client.Resource(contributionReceiptGVR).Namespace(namespace).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	created, err := receiptFromUnstructured(obj)
	if err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	return created, obj.GetResourceVersion(), nil
}

func (s *ReceiptStore) Get(ctx context.Context, namespace string, key ContributionKey) (*ObservationContributionReceipt, string, error) {
	name, err := key.ReceiptName()
	if err != nil {
		return nil, "", err
	}
	obj, err := s.client.Resource(contributionReceiptGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	receipt, err := receiptFromUnstructured(obj)
	if err != nil {
		return nil, "", err
	}
	if receipt.ObservationID != key.ObservationID || !receipt.Population.Equal(key.Population) {
		return nil, "", ErrReceiptIdentityMismatch
	}
	return &receipt, obj.GetResourceVersion(), nil
}

// GetAfterInitialization retries only the API-server visibility window
// between receipt CREATE and its PREPARED status initialization.  It never
// converts an incomplete receipt into a valid state and remains bounded.
func (s *ReceiptStore) GetAfterInitialization(ctx context.Context, namespace string, key ContributionKey) (*ObservationContributionReceipt, string, error) {
	var lastErr error
	for attempt := 0; attempt < receiptInitializationReadAttempts; attempt++ {
		receipt, resourceVersion, err := s.Get(ctx, namespace, key)
		if !errors.Is(err, errReceiptInitializing) {
			return receipt, resourceVersion, err
		}
		lastErr = err
		if attempt+1 < receiptInitializationReadAttempts {
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, "", ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, "", fmt.Errorf("%w after %d reads", lastErr, receiptInitializationReadAttempts)
}

// Commit transitions a PREPARED receipt to COMMITTED. If a concurrent caller
// holding the same ContributionKey has already committed an equivalent
// receipt (same ContentDigest) by the time this call performs its fresh
// read, that is benign convergence, not a failure: Commit returns the
// existing COMMITTED receipt with a nil error rather than erroring solely
// because another equivalent caller won the race. A COMMITTED receipt whose
// ContentDigest does not match expectedContentDigest is a genuine identity
// mismatch and remains a hard, fail-closed error.
func (s *ReceiptStore) Commit(ctx context.Context, namespace string, key ContributionKey, resourceVersion, expectedContentDigest string) (ObservationContributionReceipt, string, error) {
	name, err := key.ReceiptName()
	if err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	obj, err := s.client.Resource(contributionReceiptGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	receipt, err := receiptFromUnstructured(obj)
	if err != nil {
		return ObservationContributionReceipt{}, "", err
	}
	if receipt.ObservationID != key.ObservationID || !receipt.Population.Equal(key.Population) {
		return ObservationContributionReceipt{}, "", ErrReceiptIdentityMismatch
	}
	if receipt.State == ReceiptCommitted {
		if receipt.ContentDigest != expectedContentDigest {
			return receipt, obj.GetResourceVersion(), fmt.Errorf("%w: committed receipt content mismatch", ErrContributionContentMismatch)
		}
		return receipt, obj.GetResourceVersion(), nil
	}
	if receipt.State != ReceiptPrepared {
		return receipt, obj.GetResourceVersion(), fmt.Errorf("%w: receipt is not prepared", ErrInvalidContribution)
	}
	if resourceVersion != "" {
		obj.SetResourceVersion(resourceVersion)
	}
	if err := unstructured.SetNestedField(obj.Object, string(ReceiptCommitted), "status", "state"); err != nil {
		return receipt, "", err
	}
	updated, err := s.client.Resource(contributionReceiptGVR).Namespace(namespace).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsConflict(err) {
			// Another equivalent caller may have committed after our fresh
			// PREPARED read.  Conflict alone is never authority; accept only
			// an exact COMMITTED receipt from a new authoritative read.
			fresh, freshRV, freshErr := s.Get(ctx, namespace, key)
			if freshErr == nil && fresh != nil && fresh.State == ReceiptCommitted {
				if fresh.ContentDigest != expectedContentDigest {
					return *fresh, freshRV, fmt.Errorf("%w: committed receipt content mismatch", ErrContributionContentMismatch)
				}
				return *fresh, freshRV, nil
			}
		}
		return receipt, "", err
	}
	committed, err := receiptFromUnstructured(updated)
	if err != nil {
		return receipt, "", err
	}
	return committed, updated.GetResourceVersion(), nil
}
