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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	restartPollInterval = 500 * time.Millisecond
	restartPollTimeout  = 30 * time.Second
)

// OwnerKind identifies what, if anything, manages the target pod —
// determines how Restart brings a replacement pod up.
type OwnerKind string

const (
	OwnerNone       OwnerKind = "none" // bare pod, e.g. `kubectl run`
	OwnerDeployment OwnerKind = "Deployment"
)

// DetectOwner walks a pod's OwnerReferences (Pod -> ReplicaSet ->
// Deployment) to find a managing Deployment, or reports OwnerNone for a
// bare pod. Only the first owner reference is considered — a Pod having
// more than one meaningful owner isn't a realistic scenario in practice.
//
// StatefulSet/DaemonSet-owned pods (or anything else) return an error
// naming the actual kind, rather than being silently treated as
// unsupported-but-ignored: Restart only knows how to handle the two
// cases above.
func DetectOwner(ctx context.Context, client kubernetes.Interface, namespace string, pod *corev1.Pod) (OwnerKind, string, error) {
	if len(pod.OwnerReferences) == 0 {
		return OwnerNone, "", nil
	}

	ref := pod.OwnerReferences[0]
	if ref.Kind != "ReplicaSet" {
		return "", "", fmt.Errorf(
			"pod %s/%s is owned by a %s, which Restart doesn't support yet (only bare pods and Deployment-owned pods are — see internal/k8s/restart.go)",
			namespace, pod.Name, ref.Kind)
	}

	rs, err := client.AppsV1().ReplicaSets(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("fetching ReplicaSet %s/%s: %w", namespace, ref.Name, err)
	}
	if len(rs.OwnerReferences) == 0 || rs.OwnerReferences[0].Kind != "Deployment" {
		return "", "", fmt.Errorf(
			"ReplicaSet %s/%s has no Deployment owner, which Restart doesn't support",
			namespace, rs.Name)
	}

	return OwnerDeployment, rs.OwnerReferences[0].Name, nil
}

// Restart deletes-and-recreates a bare pod, or triggers a rollout
// restart for a Deployment-owned pod, then waits for the replacement pod
// to exist and returns an updated TargetPod pointing at it (same name
// for a bare pod; a new, controller-generated name for a Deployment).
//
// This exists to close docs/e2e-demo.md Finding 2: trace_open only
// observes openat(), not writes on an already-open fd, so resources a
// process opens once at startup (a pid file, a log fd held open for the
// process lifetime) are invisible to a trace that attaches to a
// container already running before the observation window started —
// the only way to see them is to actually observe a startup.
//
// The tracer is started as soon as the replacement pod *exists* (any
// phase), not once it's Running: the assumption is that Inspektor
// Gadget's KubeManager filter attaches at container-creation time,
// before the entrypoint's first syscalls — unconfirmed against a live
// cluster as of this writing, same as every other Inspektor Gadget
// behavioral assumption in this codebase before its first live test.
func Restart(ctx context.Context, client kubernetes.Interface, target *TargetPod) (*TargetPod, error) {
	pod, err := client.CoreV1().Pods(target.Namespace).Get(ctx, target.PodName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetching pod %s/%s: %w", target.Namespace, target.PodName, err)
	}

	owner, ownerName, err := DetectOwner(ctx, client, target.Namespace, pod)
	if err != nil {
		return nil, err
	}

	switch owner {
	case OwnerNone:
		return restartBarePod(ctx, client, target, pod)
	case OwnerDeployment:
		return restartDeployment(ctx, client, target, ownerName)
	default:
		return nil, fmt.Errorf("unhandled owner kind %q", owner)
	}
}

// restartBarePod deletes pod and recreates it under the same name with
// the same spec/labels — nothing else will bring a bare pod back.
func restartBarePod(ctx context.Context, client kubernetes.Interface, target *TargetPod, pod *corev1.Pod) (*TargetPod, error) {
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels:    pod.Labels,
		},
		Spec: pod.Spec,
	}
	// NodeName is runtime-assigned by the scheduler on the pod being
	// replaced; carrying it over would pin the new pod to that specific
	// node instead of letting it be scheduled fresh.
	newPod.Spec.NodeName = ""

	if err := client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
		return nil, fmt.Errorf("deleting pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	// A pod object must be fully gone (not just Terminating) before a
	// new one can be created under the same name.
	if err := waitForPodGone(ctx, client, pod.Namespace, pod.Name); err != nil {
		return nil, err
	}

	if _, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, newPod, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("recreating pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	updated := *target
	updated.Labels = pod.Labels
	return &updated, nil
}

// restartDeploymentAnnotation is the same annotation key `kubectl
// rollout restart` itself patches onto a Deployment's pod template to
// force a new rollout — a stable, documented part of kubectl's own
// behavior, not an Inspektor-Gadget-style guess.
const restartDeploymentAnnotation = "kubectl.kubernetes.io/restartedAt"

// restartDeployment triggers a rollout restart on deploymentName the
// same way `kubectl rollout restart` does, then waits for the new pod
// (a different, controller-generated name) to appear.
func restartDeployment(ctx context.Context, client kubernetes.Interface, target *TargetPod, deploymentName string) (*TargetPod, error) {
	patch := []byte(fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		restartDeploymentAnnotation, time.Now().Format(time.RFC3339)))

	deployment, err := client.AppsV1().Deployments(target.Namespace).Patch(
		ctx, deploymentName, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("patching deployment %s/%s to trigger a rollout restart: %w",
			target.Namespace, deploymentName, err)
	}

	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("deployment %s/%s has an invalid pod selector: %w",
			target.Namespace, deploymentName, err)
	}

	newName, newLabels, err := waitForNewPod(ctx, client, target.Namespace, selector.String(), target.PodName)
	if err != nil {
		return nil, err
	}

	updated := *target
	updated.PodName = newName
	updated.Labels = newLabels
	return &updated, nil
}

// waitForNewPod polls (no Watch, to keep this testable against a fake
// clientset the same way target_test.go tests Resolve) for a pod
// matching labelSelector whose name differs from oldPodName.
func waitForNewPod(ctx context.Context, client kubernetes.Interface, namespace, labelSelector, oldPodName string) (name string, labels map[string]string, err error) {
	ctx, cancel := context.WithTimeout(ctx, restartPollTimeout)
	defer cancel()

	for {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return "", nil, fmt.Errorf("listing pods in %s matching %q: %w", namespace, labelSelector, err)
		}
		for _, p := range pods.Items {
			if p.Name != oldPodName {
				return p.Name, p.Labels, nil
			}
		}

		select {
		case <-ctx.Done():
			return "", nil, fmt.Errorf("timed out waiting for a replacement pod in %s matching %q", namespace, labelSelector)
		case <-time.After(restartPollInterval):
		}
	}
}

// waitForPodGone polls until name no longer exists in namespace.
func waitForPodGone(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	ctx, cancel := context.WithTimeout(ctx, restartPollTimeout)
	defer cancel()

	for {
		_, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking deletion of pod %s/%s: %w", namespace, name, err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for pod %s/%s to be deleted", namespace, name)
		case <-time.After(restartPollInterval):
		}
	}
}
