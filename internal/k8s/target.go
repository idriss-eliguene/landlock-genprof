// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package k8s locates and prepares the target pod for a training run
// (namespace/pod/container resolution, checking the RBAC permissions the
// tracer needs).
package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// WorkloadRef identifies a logical Kubernetes workload without binding it to
// an API version or a runtime incarnation.
type WorkloadRef struct {
	Group string `json:"group"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
}

// GovernedTarget is the canonical logical workload/container identity. API
// Version, UIDs, image content, and executable paths are deliberately outside
// this identity; those belong to read provenance or runtime/evidence models.
type GovernedTarget struct {
	Namespace string      `json:"namespace"`
	Workload  WorkloadRef `json:"workload"`
	Container string      `json:"container"`
}

// CanonicalTargetBinding is the additive persistence representation of a
// GovernedTarget. Container remains authoritative in the enclosing history
// population or proposal Spec, so this binding does not duplicate it.
// API Version and UIDs are intentionally absent.
type CanonicalTargetBinding struct {
	Namespace string `json:"namespace"`
	Group     string `json:"group,omitempty"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

func CanonicalTargetBindingFor(target GovernedTarget) CanonicalTargetBinding {
	return CanonicalTargetBinding{Namespace: target.Namespace, Group: target.Workload.Group, Kind: target.Workload.Kind, Name: target.Workload.Name}
}

// GovernedTarget reconstructs a target using the enclosing authoritative
// container field. Empty Group is valid for a core/v1 Pod only.
func (b CanonicalTargetBinding) GovernedTarget(container string) (GovernedTarget, error) {
	if b.Namespace == "" || b.Kind == "" || b.Name == "" || container == "" {
		return GovernedTarget{}, fmt.Errorf("incomplete canonical target binding")
	}
	if b.Group == "" && b.Kind != "Pod" {
		return GovernedTarget{}, fmt.Errorf("non-Pod canonical target binding requires a Group")
	}
	return GovernedTarget{Namespace: b.Namespace, Workload: WorkloadRef{Group: b.Group, Kind: b.Kind, Name: b.Name}, Container: container}, nil
}

// Equal compares the complete logical target deterministically.
func (t GovernedTarget) Equal(other GovernedTarget) bool { return t == other }

// CanonicalJSON returns a deterministic, collision-resistant representation
// for logs, tests, and future typed consumers. The legacy persistence form is
// intentionally separate and remains unchanged.
func (t GovernedTarget) CanonicalJSON() (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("marshaling governed target: %w", err)
	}
	return string(b), nil
}

// LegacyString preserves the v0.4 TrainingHistory population target field.
func (t GovernedTarget) LegacyString() string {
	if t.Workload.Kind == "Pod" {
		return t.Workload.Name
	}
	return t.Workload.Kind + "/" + t.Workload.Name
}

// RuntimeSubject identifies one observed runtime incarnation of a governed
// target. PodUID and ImageID are attribution/provenance, not logical target
// identity and not the TrainingHistory population key by themselves.
type RuntimeSubject struct {
	Target     GovernedTarget
	PodUID     string
	ImageID    string
	BinaryPath string
}

// TargetPod identifies the pod/container to observe.
type TargetPod struct {
	Namespace string
	PodName   string
	PodUID    string
	// GovernedTarget is the canonical owner identity used for evidence
	// populations. Bare pods fall back to PodName; owned pods use the
	// owner kind/name so replacement pods retain the same target.
	GovernedTarget GovernedTarget
	Container      string
	// ImageIdentity is the resolved immutable container image identity from
	// status, never the user-supplied image tag. Empty means unknown.
	ImageIdentity string
	// Labels are the traced pod's own labels, carried through so a
	// NetworkPolicy exporter can build spec.podSelector from real cluster
	// data instead of inventing a selector mechanism (see
	// internal/exporter/networkpolicy).
	Labels map[string]string
}

// Resolve checks that the target pod exists, is running, and that the
// requested container is present in it (or deduces it if there is only
// one). The client is injected rather than constructed here, so tests can
// use client-go's fake clientset (k8s.io/client-go/kubernetes/fake)
// without depending on a real cluster — see internal/k8s/target_test.go.
//
// Minimal RBAC required for this call alone: `get` on `pods` in the target
// namespace; an owned pod additionally requires `get` on its ReplicaSet when
// resolving a Deployment owner. See docs/threat-model.md for the tracer's
// full RBAC (beyond this resolution step).
func Resolve(ctx context.Context, client kubernetes.Interface, namespace, podName, container string) (*TargetPod, error) {
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("pod %s/%s not found: %w", namespace, podName, err)
	}
	if err != nil {
		return nil, fmt.Errorf("resolving pod %s/%s: %w", namespace, podName, err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("pod %s/%s is not running (current phase: %s)",
			namespace, podName, pod.Status.Phase)
	}

	resolvedContainer, err := resolveContainer(pod, container)
	if err != nil {
		return nil, err
	}

	imageIdentity := ""
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == resolvedContainer {
			imageIdentity = status.ImageID
			break
		}
	}
	owner, ownerName, err := DetectOwner(ctx, client, namespace, pod)
	if err != nil {
		return nil, fmt.Errorf("detecting pod owner: %w", err)
	}
	governedTarget := governedTargetFor(namespace, podName, owner, ownerName, resolvedContainer)

	return &TargetPod{
		Namespace:      namespace,
		PodName:        podName,
		PodUID:         string(pod.UID),
		GovernedTarget: governedTarget,
		Container:      resolvedContainer,
		ImageIdentity:  imageIdentity,
		Labels:         pod.Labels,
	}, nil
}

func governedTargetFor(namespace, podName string, owner OwnerKind, ownerName, container string) GovernedTarget {
	if owner == OwnerNone {
		return GovernedTarget{Namespace: namespace, Workload: WorkloadRef{Kind: "Pod", Name: podName}, Container: container}
	}
	return GovernedTarget{
		Namespace: namespace,
		Workload:  WorkloadRef{Group: "apps", Kind: string(owner), Name: ownerName},
		Container: container,
	}
}

// RuntimeSubject returns the minimal runtime attribution associated with this
// resolved target for an observation using binaryPath.
func (t TargetPod) RuntimeSubject(binaryPath string) RuntimeSubject {
	return RuntimeSubject{Target: t.GovernedTarget, PodUID: t.PodUID, ImageID: t.ImageIdentity, BinaryPath: binaryPath}
}

// resolveContainer validates the requested container, or deduces it if the
// pod has only one (mirrors `kubectl exec` without --container).
func resolveContainer(pod *corev1.Pod, container string) (string, error) {
	if len(pod.Spec.Containers) == 0 {
		return "", fmt.Errorf("pod %s/%s has no containers", pod.Namespace, pod.Name)
	}

	if container == "" {
		if len(pod.Spec.Containers) > 1 {
			return "", fmt.Errorf(
				"pod %s/%s has multiple containers (%d): specify --container",
				pod.Namespace, pod.Name, len(pod.Spec.Containers),
			)
		}
		return pod.Spec.Containers[0].Name, nil
	}

	for _, c := range pod.Spec.Containers {
		if c.Name == container {
			return container, nil
		}
	}
	return "", fmt.Errorf("container %q not found in pod %s/%s", container, pod.Namespace, pod.Name)
}
