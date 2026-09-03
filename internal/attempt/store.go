// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package attempt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var GVR = schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "applyattempts"}

func newName() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating ApplyAttempt ID: %w", err)
	}
	return "apply-" + time.Now().UTC().Format("20060102t150405") + "-" + hex.EncodeToString(b), nil
}

// Create durably creates the ApplyAttempt main resource and immutable spec.
// Callers must persist the initial lifecycle state through SaveStatus before
// proceeding to any governed-target mutation.
func Create(ctx context.Context, client dynamic.Interface, namespace string, spec Spec) (string, *unstructured.Unstructured, error) {
	name, err := newName()
	if err != nil {
		return "", nil, err
	}
	specMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		return "", nil, fmt.Errorf("serializing ApplyAttempt spec: %w", err)
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "landlockgenprof.io/v1alpha1",
		"kind":       "ApplyAttempt",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec":       specMap,
	}}
	created, err := client.Resource(GVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("creating ApplyAttempt %s/%s: %w", namespace, name, err)
	}
	return name, created, nil
}

// SaveStatus replaces only the status subresource. It is intentionally
// explicit: status custody is durable and is not an authorization source.
func SaveStatus(ctx context.Context, client dynamic.Interface, namespace, name string, obj *unstructured.Unstructured, status Status) error {
	if err := status.Validate(); err != nil {
		return err
	}
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		return fmt.Errorf("serializing ApplyAttempt status %s/%s: %w", namespace, name, err)
	}
	copy := obj.DeepCopy()
	copy.Object["status"] = statusMap
	updated, err := client.Resource(GVR).Namespace(namespace).UpdateStatus(ctx, copy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("persisting ApplyAttempt status %s/%s: %w", namespace, name, err)
	}
	// Carry forward the API server's resourceVersion for the next status
	// checkpoint. Status writes are sequential, but they still participate in
	// Kubernetes optimistic concurrency on the same object.
	obj.Object = updated.Object
	return nil
}
