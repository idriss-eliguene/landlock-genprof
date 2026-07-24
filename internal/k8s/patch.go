// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// PatchedManifest fetches target's live owner (Deployment/StatefulSet/
// DaemonSet — see DetectOwner) or, for a bare pod, the pod itself, and
// returns a clean, minimal, ready-to-apply YAML manifest with sc merged
// into the target container's securityContext.
//
// Most container-spec fields, including securityContext, are immutable
// on an already-running Pod — kubectl apply can't change them on a live
// Pod directly. For an owned pod, the artifact that's actually useful is
// the *owner's* manifest (patched on
// spec.template.spec.containers[].securityContext): applying that
// triggers a rollout, the real supported way to change this. Only a bare
// pod's own manifest is the right target, and even then applying it
// requires delete+recreate (see Restart's own restartBarePod). This
// mirrors the exact OwnerKind distinction restart.go already encodes.
//
// Merge, not replace: only sc.Capabilities and sc.SeccompProfile are
// ever set on the target container's securityContext — every other
// field the live object already has (RunAsUser, RunAsNonRoot,
// Privileged, ReadOnlyRootFilesystem, ...) is left untouched. Silently
// replacing the whole securityContext would risk wiping out hardening
// the user already has; this only ever contributes what was actually
// generated, same principle applied everywhere else in this project.
//
// Returns the resource's own identity (the owner's name, or the pod's
// own name for a bare pod) alongside the manifest — not always
// target.PodName, e.g. after --restart substituted a Deployment/
// DaemonSet's own name for tracer targeting purposes (see
// cmd/landlock-genprof/trace.go's traceWithRestart), so callers must
// pass the *original*, still-real pod identity here, not a substituted
// one.
//
// Only safe to call with a target whose pod is still guaranteed to
// exist — i.e. no --restart happened, or owner keeps a stable name
// (KeepsStableName: bare pod, StatefulSet). For a Deployment/DaemonSet
// after --restart, the original pod has already been deleted by the
// rollout by the time this typically runs — use PatchedManifestForOwner
// instead, with the owner/name --restart already determined.
func PatchedManifest(ctx context.Context, client kubernetes.Interface, target *TargetPod, sc *corev1.SecurityContext) (identity string, manifest []byte, err error) {
	pod, err := client.CoreV1().Pods(target.Namespace).Get(ctx, target.PodName, metav1.GetOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("fetching pod %s/%s: %w", target.Namespace, target.PodName, err)
	}

	owner, ownerName, err := DetectOwner(ctx, client, target.Namespace, pod)
	if err != nil {
		return "", nil, err
	}

	if owner == OwnerNone {
		if err := mergeContainerSecurityContext(pod.Spec.Containers, target.Container, sc); err != nil {
			return "", nil, err
		}
		out, err := yaml.Marshal(cleanPod(pod))
		return pod.Name, out, err
	}
	return patchOwner(ctx, client, target.Namespace, owner, ownerName, target.Container, sc)
}

// PatchedManifestForOwner is PatchedManifest for a caller that already
// knows owner/ownerName — e.g. cmd/landlock-genprof/trace.go's runTrace
// after --restart already ran k8s.DetectOwner once itself. Skips
// re-fetching a pod entirely, which matters for Deployment/DaemonSet
// (!KeepsStableName): --restart's rollout deletes the original pod and
// replaces it under a new, unpredictable generateName, so by the time a
// caller gets around to building the patched manifest (after the whole
// training run), the exact pod PatchedManifest would try to Get by name
// may already be gone — confirmed live: "pods \"<name>\" not found"
// against a DaemonSet. ownerName here plays the same role as
// PodSelectorFor's own ownerName parameter, for the same reason.
//
// OwnerNone isn't accepted — a bare pod's own manifest is the target
// there, which needs an actual pod object (labels, etc.), not just a
// name; use PatchedManifest for that case, safe since a bare pod always
// keeps its name (KeepsStableName).
func PatchedManifestForOwner(ctx context.Context, client kubernetes.Interface, namespace string, owner OwnerKind, ownerName, containerName string, sc *corev1.SecurityContext) (identity string, manifest []byte, err error) {
	if owner == OwnerNone {
		return "", nil, fmt.Errorf("PatchedManifestForOwner: owner %q not supported, use PatchedManifest for a bare pod", owner)
	}
	return patchOwner(ctx, client, namespace, owner, ownerName, containerName, sc)
}

// patchOwner fetches owner/ownerName's live manifest, merges sc into
// containerName's securityContext, and returns a clean, ready-to-apply
// manifest — the shared Deployment/StatefulSet/DaemonSet logic behind
// both PatchedManifest and PatchedManifestForOwner.
func patchOwner(ctx context.Context, client kubernetes.Interface, namespace string, owner OwnerKind, ownerName, containerName string, sc *corev1.SecurityContext) (identity string, manifest []byte, err error) {
	switch owner {
	case OwnerDeployment:
		d, err := client.AppsV1().Deployments(namespace).Get(ctx, ownerName, metav1.GetOptions{})
		if err != nil {
			return "", nil, fmt.Errorf("fetching deployment %s/%s: %w", namespace, ownerName, err)
		}
		if err := mergeContainerSecurityContext(d.Spec.Template.Spec.Containers, containerName, sc); err != nil {
			return "", nil, err
		}
		out, err := yaml.Marshal(cleanDeployment(d))
		return d.Name, out, err
	case OwnerStatefulSet:
		s, err := client.AppsV1().StatefulSets(namespace).Get(ctx, ownerName, metav1.GetOptions{})
		if err != nil {
			return "", nil, fmt.Errorf("fetching statefulset %s/%s: %w", namespace, ownerName, err)
		}
		if err := mergeContainerSecurityContext(s.Spec.Template.Spec.Containers, containerName, sc); err != nil {
			return "", nil, err
		}
		out, err := yaml.Marshal(cleanStatefulSet(s))
		return s.Name, out, err
	case OwnerDaemonSet:
		ds, err := client.AppsV1().DaemonSets(namespace).Get(ctx, ownerName, metav1.GetOptions{})
		if err != nil {
			return "", nil, fmt.Errorf("fetching daemonset %s/%s: %w", namespace, ownerName, err)
		}
		if err := mergeContainerSecurityContext(ds.Spec.Template.Spec.Containers, containerName, sc); err != nil {
			return "", nil, err
		}
		out, err := yaml.Marshal(cleanDaemonSet(ds))
		return ds.Name, out, err
	default:
		return "", nil, fmt.Errorf("patchOwner: unhandled owner kind %q", owner)
	}
}

// mergeContainerSecurityContext finds containerName in containers and
// merges sc into its SecurityContext (creating one if absent) — see
// PatchedManifest's own doc comment for why this is a merge, not a
// replace. Returns an error, not a silent no-op, if containerName isn't
// found: a container that used to exist but doesn't anymore (spec
// drifted since the training run) shouldn't fail invisibly.
func mergeContainerSecurityContext(containers []corev1.Container, containerName string, sc *corev1.SecurityContext) error {
	for i := range containers {
		if containers[i].Name != containerName {
			continue
		}
		existing := containers[i].SecurityContext
		if existing == nil {
			existing = &corev1.SecurityContext{}
		}
		if sc.Capabilities != nil {
			existing.Capabilities = sc.Capabilities
		}
		if sc.SeccompProfile != nil {
			existing.SeccompProfile = sc.SeccompProfile
		}
		containers[i].SecurityContext = existing
		return nil
	}
	return fmt.Errorf("container %q not found", containerName)
}

// cleanPod/cleanDeployment/cleanStatefulSet/cleanDaemonSet each build a
// *new* object with only TypeMeta, a minimal ObjectMeta (Name/Namespace/
// Labels), and the (already patched) Spec — explicitly dropping Status,
// ResourceVersion, UID, CreationTimestamp, Generation, ManagedFields,
// and OwnerReferences from the live-fetched object. A raw dump of a
// fetched object carries a lot of server-populated noise that doesn't
// belong in a ready-to-apply manifest (and a stale ResourceVersion could
// even cause a spurious conflict on re-apply) — same "don't dump raw
// server state" discipline internal/proposal's rendered artifacts
// already follow.

// cleanManifest is the shape every clean* constructor marshals through —
// deliberately its own minimal type, not the real corev1.Pod/appsv1.
// Deployment/etc. types themselves: those always carry a Status field
// with no `omitempty` (apiserver-populated types are never optional
// about it), so even an explicitly zero-valued Status still serializes
// as `status: {}` if a real API type is marshaled directly — confirmed
// by a failing test before this fix. Omitting the field from this type
// entirely is the only way to actually leave it out of the output.
type cleanManifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              interface{} `json:"spec,omitempty"`
}

func cleanPod(p *corev1.Pod) cleanManifest {
	return cleanManifest{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: p.Name, Namespace: p.Namespace, Labels: p.Labels},
		Spec:       p.Spec,
	}
}

func cleanDeployment(d *appsv1.Deployment) cleanManifest {
	return cleanManifest{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: d.Namespace, Labels: d.Labels},
		Spec:       d.Spec,
	}
}

func cleanStatefulSet(s *appsv1.StatefulSet) cleanManifest {
	return cleanManifest{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"},
		ObjectMeta: metav1.ObjectMeta{Name: s.Name, Namespace: s.Namespace, Labels: s.Labels},
		Spec:       s.Spec,
	}
}

func cleanDaemonSet(ds *appsv1.DaemonSet) cleanManifest {
	return cleanManifest{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "DaemonSet"},
		ObjectMeta: metav1.ObjectMeta{Name: ds.Name, Namespace: ds.Namespace, Labels: ds.Labels},
		Spec:       ds.Spec,
	}
}
