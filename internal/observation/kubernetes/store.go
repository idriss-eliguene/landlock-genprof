// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package kubernetes

import (
	"context"
	"fmt"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var GVR = schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observations"}

// Store is the narrow domain-oriented persistence adapter. Executor writes
// are exposed only through the fenced operations in executor.go.
type Store struct {
	client dynamic.Interface
	clock  Clock
}

func NewStore(client dynamic.Interface) (*Store, error) {
	return NewStoreWithClock(client, realClock{})
}

func NewStoreWithClock(client dynamic.Interface, clock Clock) (*Store, error) {
	if client == nil {
		return nil, fmt.Errorf("observation store requires a Kubernetes client")
	}
	if clock == nil {
		return nil, fmt.Errorf("observation store requires a clock")
	}
	return &Store{client: client, clock: clock}, nil
}

func (s *Store) CreateObservation(ctx context.Context, namespace string, observation domain.Observation) (string, error) {
	object, err := ToUnstructured(observation, namespace)
	if err != nil {
		return "", err
	}
	created, err := s.client.Resource(GVR).Namespace(namespace).Create(ctx, object, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("creating Observation %s/%s: %w", namespace, observation.ID(), err)
	}
	return created.GetResourceVersion(), nil
}

func (s *Store) GetObservation(ctx context.Context, namespace, name string) (domain.Observation, string, error) {
	object, err := s.client.Resource(GVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return domain.Observation{}, "", fmt.Errorf("getting Observation %s/%s: %w", namespace, name, err)
	}
	observation, err := FromUnstructured(object)
	if err != nil {
		return domain.Observation{}, "", fmt.Errorf("decoding Observation %s/%s: %w", namespace, name, err)
	}
	return observation, object.GetResourceVersion(), nil
}

// updateObservationStatus is the internal persistence primitive. Executor
// callers must use the claim-aware operations in executor.go; keeping this
// method private prevents an unfenced status-write escape hatch.
func (s *Store) updateObservationStatus(ctx context.Context, namespace string, observation domain.Observation, expectedResourceVersion string, status map[string]interface{}) (string, error) {
	if expectedResourceVersion == "" {
		return "", fmt.Errorf("resourceVersion is required for Observation status update")
	}
	resource := s.client.Resource(GVR).Namespace(namespace)
	currentObject, err := resource.Get(ctx, string(observation.ID()), metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("reading Observation status base %s/%s: %w", namespace, observation.ID(), err)
	}
	current, err := FromUnstructured(currentObject)
	if err != nil {
		return "", fmt.Errorf("decoding Observation status base %s/%s: %w", namespace, observation.ID(), err)
	}
	if err := ValidateStatusMutation(current, observation); err != nil {
		return "", err
	}
	copy := currentObject.DeepCopy()
	copy.SetResourceVersion(expectedResourceVersion)
	copy.Object["status"] = status
	updated, err := resource.UpdateStatus(ctx, copy, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsConflict(err) {
			return "", fmt.Errorf("updating Observation status %s/%s: resourceVersion conflict: %w", namespace, observation.ID(), err)
		}
		return "", fmt.Errorf("updating Observation status %s/%s: %w", namespace, observation.ID(), err)
	}
	return updated.GetResourceVersion(), nil
}
