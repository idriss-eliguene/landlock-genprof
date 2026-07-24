// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package proposal

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	apiGroup   = "landlockgenprof.io"
	apiVersion = "v1alpha1"
	kind       = "SecurityProfileProposal"
)

// securityProfileProposalGVR must match
// deploy/crd-securityprofileproposal.yaml's group/version/plural
// exactly — there's no code-level link between them, this is it. Same
// group as internal/history's trainingHistoryGVR: SecurityProfileProposal
// is a second Kind under landlockgenprof.io, not a reason for a second
// API group.
var securityProfileProposalGVR = schema.GroupVersionResource{
	Group:    apiGroup,
	Version:  apiVersion,
	Resource: "securityprofileproposals",
}

// Save creates or updates the SecurityProfileProposal object for name in
// namespace — a plain overwrite-on-rerun snapshot, not an accumulation
// the way internal/history.Merge is: a proposal represents "the latest
// generated recommendation," matching how the CLI's local files already
// behave (trace overwrites <pod>-profile.yaml on every run too).
//
// Built via runtime.DefaultUnstructuredConverter, not a hand-rolled map
// the way internal/history/store.go's toUnstructured is — appropriate
// there for Record's flat, hand-rolled shape, but the wrong tool for
// Spec's nested real Kubernetes/PodLock/seccomp API types (confirmed via
// k8s.io/apimachinery/pkg/runtime/converter.go: this is the same
// converter client-go itself uses for this exact purpose).
func Save(ctx context.Context, client dynamic.Interface, namespace, name string, spec Spec) error {
	resource := client.Resource(securityProfileProposalGVR).Namespace(namespace)

	specMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		return fmt.Errorf("converting proposal spec for %s/%s: %w", namespace, name, err)
	}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiGroup + "/" + apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": specMap,
	}}

	existing, err := resource.Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, err := resource.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating SecurityProfileProposal %s/%s: %w", namespace, name, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("fetching SecurityProfileProposal %s/%s before update: %w", namespace, name, err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	if _, err := resource.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	return nil
}

// Get fetches the SecurityProfileProposal for name in namespace, or
// returns (nil, nil) if it doesn't exist yet. No CLI-facing read path
// uses this today — kept for round-trip testability and future reuse
// (e.g. a future `landlock-genprof proposal get` subcommand), mirroring
// internal/history.Get's own shape.
func Get(ctx context.Context, client dynamic.Interface, namespace, name string) (*Spec, error) {
	obj, err := client.Resource(securityProfileProposalGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}

	specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("reading spec from SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	if !found {
		return &Spec{}, nil
	}

	var spec Spec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &spec); err != nil {
		return nil, fmt.Errorf("converting spec from SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	return &spec, nil
}
