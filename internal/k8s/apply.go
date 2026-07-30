// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

// applyGVRs maps every GroupVersionKind a SecurityProfileProposal's
// artifacts can ever contain to its GroupVersionResource, so Apply can
// dynamic.Interface.Resource(...) without a live discovery round-trip
// (RESTMapper) on every call. Two of these (LandlockProfile,
// SeccompProfile) are CRDs this project doesn't own — their group/version
// here must match the same constants the exporters that generate them
// hardcode (internal/exporter/podlock, internal/exporter/spo), which in
// turn must match the real operators' CRDs. If Apply fails with a
// "the server could not find the requested resource" error, that's the
// first thing to check — not a bug in the create/update logic below.
var applyGVRs = map[schema.GroupVersionKind]schema.GroupVersionResource{
	{Group: "podlock.kubewarden.io", Version: "v1alpha1", Kind: "LandlockProfile"}: {
		Group: "podlock.kubewarden.io", Version: "v1alpha1", Resource: "landlockprofiles",
	},
	{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}: {
		Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies",
	},
	{Group: "security-profiles-operator.x-k8s.io", Version: "v1", Kind: "SeccompProfile"}: {
		Group: "security-profiles-operator.x-k8s.io", Version: "v1", Resource: "seccompprofiles",
	},
	// PatchedManifest is whichever of these DetectOwner/PatchedManifest
	// picked when it was generated — Pod for a bare pod, its owner's kind
	// otherwise (see patch.go).
	{Version: "v1", Kind: "Pod"}: {Version: "v1", Resource: "pods"},
	{Group: "apps", Version: "v1", Kind: "Deployment"}: {
		Group: "apps", Version: "v1", Resource: "deployments",
	},
	{Group: "apps", Version: "v1", Kind: "StatefulSet"}: {
		Group: "apps", Version: "v1", Resource: "statefulsets",
	},
	{Group: "apps", Version: "v1", Kind: "DaemonSet"}: {
		Group: "apps", Version: "v1", Resource: "daemonsets",
	},
}

// Apply creates or updates the single resource described by yamlContent —
// one of a SecurityProfileProposal's four artifacts (see
// internal/proposal.Spec). Namespace is only used as a fallback: a
// PatchedManifest already carries its own real metadata.namespace (it's a
// live-fetched owner manifest, not authored fresh), while the other three
// artifacts are generated locally and may not set one.
//
// Create-or-update, same pattern as internal/proposal.Save: fetch first
// to get the ResourceVersion an Update needs, Create if that fetch 404s.
func Apply(ctx context.Context, client dynamic.Interface, namespace, yamlContent string) error {
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(yamlContent), &obj.Object); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	gvk := obj.GroupVersionKind()
	gvr, ok := applyGVRs[gvk]
	if !ok {
		return fmt.Errorf("unrecognized resource kind %q (apiVersion %q) in generated manifest — "+
			"this artifact type isn't one apply-proposal knows how to apply, please report this as a bug",
			gvk.Kind, gvk.GroupVersion())
	}

	ns := obj.GetNamespace()
	if ns == "" {
		ns = namespace
		obj.SetNamespace(ns)
	}
	resource := client.Resource(gvr).Namespace(ns)

	existing, err := resource.Get(ctx, obj.GetName(), metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, err := resource.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating %s %s/%s: %w", gvk.Kind, ns, obj.GetName(), err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("fetching %s %s/%s before update: %w", gvk.Kind, ns, obj.GetName(), err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	if _, err := resource.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating %s %s/%s: %w", gvk.Kind, ns, obj.GetName(), err)
	}
	return nil
}
