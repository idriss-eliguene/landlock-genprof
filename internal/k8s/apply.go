// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
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
	// Cluster-scoped v1 — see internal/spobackend, which owns the SPO API
	// shape so it is stated once rather than restated at every use.
	spobackend.SeccompProfileGVK(): spobackend.SeccompProfileGVR(),
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

// ApplyGuard is the live identity/version observation made immediately
// before a mutation. Present=false describes a create path.
type ApplyGuard struct {
	Present         bool
	UID             types.UID
	ResourceVersion string
}

// ReadApplyResource reads the object targeted by a rendered apply manifest.
// It shares Apply's GVK/GVR and scope table so custody cannot drift from the
// actual mutation path.
func ReadApplyResource(ctx context.Context, client dynamic.Interface, namespace string, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	gvk := desired.GroupVersionKind()
	gvr, ok := applyGVRs[gvk]
	if !ok {
		return nil, fmt.Errorf("unrecognized resource kind %q (apiVersion %q)", gvk.Kind, gvk.GroupVersion())
	}
	var resource dynamic.ResourceInterface
	if !clusterScoped(gvk) {
		ns := desired.GetNamespace()
		if ns == "" {
			ns = namespace
		}
		resource = client.Resource(gvr).Namespace(ns)
	} else {
		resource = client.Resource(gvr)
	}
	current, err := resource.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return current, nil
}

// ApplyWithGuard refuses to enter the existing Apply operation unless the
// live object still matches the custody observation. Apply remains the sole
// mutation implementation; this function only adds the pre-dispatch guard.
func ApplyWithGuard(ctx context.Context, client dynamic.Interface, namespace, yamlContent string, guard ApplyGuard) error {
	_, err := ApplyWithGuardObserved(ctx, client, namespace, yamlContent, guard)
	return err
}

// MutationObservation is the identity returned by the Kubernetes mutation
// response. It must be compared with the immediate reread before custody can
// attribute an After resourceVersion to this mutation.
type MutationObservation struct {
	UID             types.UID
	ResourceVersion string
}

func ApplyWithGuardObserved(ctx context.Context, client dynamic.Interface, namespace, yamlContent string, guard ApplyGuard) (MutationObservation, error) {
	desired := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(yamlContent), &desired.Object); err != nil {
		return MutationObservation{}, fmt.Errorf("parsing manifest: %w", err)
	}
	return applyParsedObserved(ctx, client, namespace, desired, &guard)
}

// Apply creates or updates the single resource described by yamlContent —
// one of a SecurityProfileProposal's four artifacts (see
// internal/proposal.Spec). Namespace is only used as a fallback: a
// PatchedManifest already carries its own real metadata.namespace (it's a
// live-fetched owner manifest, not authored fresh), while the other three
// artifacts are generated locally and may not set one.
//
// Create-or-update for everything except Pod (see applyPod for why that
// one's different), same pattern as internal/proposal.Save: fetch first
// to get the ResourceVersion an Update needs, Create if that fetch 404s.
func Apply(ctx context.Context, client dynamic.Interface, namespace, yamlContent string) error {
	_, err := ApplyObserved(ctx, client, namespace, yamlContent)
	return err
}

func ApplyObserved(ctx context.Context, client dynamic.Interface, namespace, yamlContent string) (MutationObservation, error) {
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(yamlContent), &obj.Object); err != nil {
		return MutationObservation{}, fmt.Errorf("parsing manifest: %w", err)
	}
	return applyParsedObserved(ctx, client, namespace, obj, nil)
}

// applyParsed contains the sole mutation path. A non-nil guard is checked
// against the same GET whose ResourceVersion is then used by Update/Delete;
// guarded callers therefore cannot silently re-read a newer object and
// mutate it under an observation they did not persist.
func applyParsed(ctx context.Context, client dynamic.Interface, namespace string, obj *unstructured.Unstructured, guard *ApplyGuard) error {
	_, err := applyParsedObserved(ctx, client, namespace, obj, guard)
	return err
}

func applyParsedObserved(ctx context.Context, client dynamic.Interface, namespace string, obj *unstructured.Unstructured, guard *ApplyGuard) (MutationObservation, error) {

	gvk := obj.GroupVersionKind()
	gvr, ok := applyGVRs[gvk]
	if !ok {
		return MutationObservation{}, fmt.Errorf("unrecognized resource kind %q (apiVersion %q) in generated manifest — "+
			"this artifact type isn't one apply-proposal knows how to apply, please report this as a bug",
			gvk.Kind, gvk.GroupVersion())
	}

	// Scope-aware, because not every artifact is namespaced. SPO's
	// SeccompProfile is cluster-scoped on the API this project targets
	// (internal/spobackend), and forcing a namespace onto a cluster-scoped
	// object is rejected by the API server. The scope question is asked of
	// the backend rather than answered here, so generic apply logic never
	// encodes which resources are cluster-scoped.
	var (
		ns       string
		resource dynamic.ResourceInterface
	)
	if clusterScoped(gvk) {
		obj.SetNamespace("")
		resource = client.Resource(gvr)
	} else {
		ns = obj.GetNamespace()
		if ns == "" {
			ns = namespace
			obj.SetNamespace(ns)
		}
		resource = client.Resource(gvr).Namespace(ns)
	}
	target := describeTarget(ns, obj.GetName())

	// Most Pod spec fields, including securityContext, are immutable on
	// an already-running Pod — confirmed live: a generic Update fails
	// with "Forbidden: pod updates may not change fields other than
	// spec.containers[*].image, ...". PatchedManifest only ever contains
	// a bare Pod's own manifest when the pod has no owner (patch.go's
	// cleanPod) — an owned pod's PatchedManifest is its Deployment/
	// StatefulSet/DaemonSet instead, which *does* support this update
	// (triggers a rollout, handled by the generic path below). A bare
	// pod has to be deleted and recreated, same as restart.go's own
	// restartBarePod — cleanPod already produces a minimal manifest
	// (NodeName cleared, no ResourceVersion/UID, injected token volume
	// stripped), so it's already safe to Create as-is post-delete.
	if gvk.Kind == "Pod" {
		return applyPodObserved(ctx, resource, obj, guard)
	}

	existing, err := resource.Get(ctx, obj.GetName(), metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		created, err := resource.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return MutationObservation{}, fmt.Errorf("creating %s %s: %w", gvk.Kind, target, err)
		}
		return MutationObservation{UID: types.UID(created.GetUID()), ResourceVersion: created.GetResourceVersion()}, nil
	case err != nil:
		return MutationObservation{}, fmt.Errorf("fetching %s %s before update: %w", gvk.Kind, target, err)
	}
	if guard != nil {
		if !guard.Present || types.UID(existing.GetUID()) != guard.UID || existing.GetResourceVersion() != guard.ResourceVersion {
			return MutationObservation{}, fmt.Errorf("live resource changed before mutation: expected present=%t UID=%q resourceVersion=%q, got present=true UID=%q resourceVersion=%q", guard.Present, guard.UID, guard.ResourceVersion, existing.GetUID(), existing.GetResourceVersion())
		}
	}

	// Cluster-scoped names are cluster-wide, so an existing object of the
	// same name may belong to somebody else — something a namespaced
	// resource could never do. Refuse rather than overwrite unless the
	// existing object records that it is ours and governs the same
	// workload. See docs/adr/0008, "Ownership marker" and "Collision
	// policy": this is collision protection under RBAC, not authentication.
	if clusterScoped(gvk) {
		if err := checkGovernedOwnership(existing, obj); err != nil {
			return MutationObservation{}, err
		}
	}

	// Retry on conflict: confirmed live against a real SPO install — its
	// own spod controller actively reconciles SeccompProfile objects
	// (writes status.localhostProfile, conditions, etc.), so the
	// ResourceVersion fetched above can go stale between this Get and the
	// Update below purely from spod's own writes, not a real editing
	// conflict from the user. A stale-RV Update fails with "the object has
	// been modified; please apply your changes to the latest version and
	// try again" (metav1.StatusReasonConflict) — re-fetching and retrying
	// a few times clears it without surfacing a spurious failure for
	// something that was never actually a conflict with any human edit.
	const maxConflictRetries = 3
	for attempt := 0; ; attempt++ {
		obj.SetResourceVersion(existing.GetResourceVersion())
		updated, err := resource.Update(ctx, obj, metav1.UpdateOptions{})
		if err == nil {
			return MutationObservation{UID: types.UID(updated.GetUID()), ResourceVersion: updated.GetResourceVersion()}, nil
		}
		if !apierrors.IsConflict(err) || attempt >= maxConflictRetries {
			return MutationObservation{}, fmt.Errorf("updating %s %s: %w", gvk.Kind, target, err)
		}
		existing, err = resource.Get(ctx, obj.GetName(), metav1.GetOptions{})
		if err != nil {
			return MutationObservation{}, fmt.Errorf("re-fetching %s %s/%s after conflict: %w", gvk.Kind, ns, obj.GetName(), err)
		}
	}
}

// applyPod is Apply's Pod-specific path: delete the existing pod (if
// any), wait for it to be fully gone — not just Terminating, a new pod
// can't be created under the same name until then — then create obj
// fresh. See Apply's own doc comment for why a generic update can't
// work here.
func applyPod(ctx context.Context, resource dynamic.ResourceInterface, obj *unstructured.Unstructured, guard *ApplyGuard) error {
	_, err := applyPodObserved(ctx, resource, obj, guard)
	return err
}

func applyPodObserved(ctx context.Context, resource dynamic.ResourceInterface, obj *unstructured.Unstructured, guard *ApplyGuard) (MutationObservation, error) {
	ns, name := obj.GetNamespace(), obj.GetName()

	existing, err := resource.Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		created, err := resource.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return MutationObservation{}, fmt.Errorf("creating Pod %s/%s: %w", ns, name, err)
		}
		return MutationObservation{UID: types.UID(created.GetUID()), ResourceVersion: created.GetResourceVersion()}, nil
	case err != nil:
		return MutationObservation{}, fmt.Errorf("fetching Pod %s/%s before recreate: %w", ns, name, err)
	}
	if guard != nil {
		if !guard.Present || types.UID(existing.GetUID()) != guard.UID || existing.GetResourceVersion() != guard.ResourceVersion {
			return MutationObservation{}, fmt.Errorf("live Pod changed before mutation: expected present=%t UID=%q resourceVersion=%q, got present=true UID=%q resourceVersion=%q", guard.Present, guard.UID, guard.ResourceVersion, existing.GetUID(), existing.GetResourceVersion())
		}
	}

	graceSeconds, found, err := unstructured.NestedInt64(existing.Object, "spec", "terminationGracePeriodSeconds")
	if err != nil {
		return MutationObservation{}, fmt.Errorf("reading termination grace period for Pod %s/%s: %w", ns, name, err)
	}
	var grace *int64
	if found {
		grace = &graceSeconds
	}

	deleteOptions := metav1.DeleteOptions{}
	originalUID := types.UID(existing.GetUID())
	if originalUID != "" {
		deleteOptions.Preconditions = &metav1.Preconditions{UID: &originalUID}
	}
	if err := resource.Delete(ctx, name, deleteOptions); err != nil {
		return MutationObservation{}, fmt.Errorf("deleting Pod %s/%s before recreate: %w", ns, name, err)
	}
	if err := waitForDynamicPodGone(ctx, resource, ns, name, originalUID, podDeletionBudget(grace), restartPollInterval); err != nil {
		return MutationObservation{}, err
	}
	created, err := resource.Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return MutationObservation{}, fmt.Errorf("recreating Pod %s/%s: %w", ns, name, err)
	}
	return MutationObservation{UID: types.UID(created.GetUID()), ResourceVersion: created.GetResourceVersion()}, nil
}

// waitForDynamicPodGone polls until the original Pod UID is fully gone from
// the API (not just Terminating). It uses the dynamic client because Apply
// stays generic across every kind a proposal artifact can contain.
func waitForDynamicPodGone(ctx context.Context, resource dynamic.ResourceInterface, namespace, name string, originalUID types.UID, timeout, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		pod, err := resource.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking deletion of Pod %s/%s: %w", namespace, name, err)
		}
		if originalUID != "" && types.UID(pod.GetUID()) != originalUID {
			return fmt.Errorf("Pod %s/%s changed identity while being deleted (original UID %s, current UID %s)", namespace, name, originalUID, pod.GetUID())
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for Pod %s/%s to be deleted", namespace, name)
		case <-time.After(interval):
		}
	}
}

// clusterScoped reports whether gvk names a cluster-scoped resource. Only
// the backends this project applies to are listed; anything else is
// namespaced, which is the Kubernetes default and the behavior every other
// artifact relies on.
func clusterScoped(gvk schema.GroupVersionKind) bool {
	return gvk == spobackend.SeccompProfileGVK() && spobackend.SeccompProfileClusterScoped()
}

// describeTarget renders "namespace/name" for a namespaced object and just
// the name for a cluster-scoped one, so error messages don't invent a
// namespace that does not exist.
func describeTarget(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

// checkGovernedOwnership refuses to overwrite a cluster-scoped object that
// this project does not own, or that it owns on behalf of a different
// workload — the digest-collision and name-scheme-migration cases.
//
// The identity tuple is read from the incoming object's own annotations,
// so this needs no extra plumbing: a governed artifact carries the tuple it
// was generated for.
func checkGovernedOwnership(existing, incoming *unstructured.Unstructured) error {
	want := incoming.GetAnnotations()
	if want[spobackend.ManagedByAnnotation] != spobackend.ManagedByValue {
		// Not a governed artifact; nothing to assert.
		return nil
	}

	verdict := spobackend.ClassifyOwnership(
		existing.GetAnnotations(),
		want[spobackend.TargetNamespaceAnnotation],
		want[spobackend.TargetPodAnnotation],
		want[spobackend.TargetContainerAnnotation],
	)
	if verdict == spobackend.OwnedSameTarget {
		return nil
	}
	return fmt.Errorf(
		"refusing to overwrite cluster-scoped %s %q: it is %s; "+
			"apply nothing rather than replace an enforcement resource this candidate does not govern",
		incoming.GetKind(), incoming.GetName(), verdict)
}
