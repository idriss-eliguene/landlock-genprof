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
	OwnerNone        OwnerKind = "none" // bare pod, e.g. `kubectl run`
	OwnerDeployment  OwnerKind = "Deployment"
	OwnerStatefulSet OwnerKind = "StatefulSet"
	OwnerDaemonSet   OwnerKind = "DaemonSet"
)

// KeepsStableName reports whether owner's pods keep the same name across
// a restart — true for a bare pod (this code recreates it under the same
// name itself) and StatefulSet (deterministic `<name>-<ordinal>` names,
// unchanged by a rolling restart), false for Deployment/DaemonSet (both
// create pods via `generateName`, a new random suffix every time).
// cmd/landlock-genprof/trace.go's traceWithRestart uses this to decide
// whether the tracer can be pre-attached to the existing name before
// restarting (relying on Inspektor Gadget's KubeManager filter to
// dynamically re-attach to the replacement) or must restart first and
// discover the new name afterward.
func KeepsStableName(owner OwnerKind) bool {
	return owner == OwnerNone || owner == OwnerStatefulSet
}

// DetectOwner walks a pod's OwnerReferences to find a managing
// Deployment (via the intermediate ReplicaSet), StatefulSet, or
// DaemonSet — or reports OwnerNone for a bare pod. Only the first owner
// reference is considered — a Pod having more than one meaningful owner
// isn't a realistic scenario in practice.
//
// Anything else returns an error naming the actual kind, rather than
// being silently treated as unsupported-but-ignored: Restart only knows
// how to handle the cases above.
func DetectOwner(ctx context.Context, client kubernetes.Interface, namespace string, pod *corev1.Pod) (OwnerKind, string, error) {
	if len(pod.OwnerReferences) == 0 {
		return OwnerNone, "", nil
	}

	ref := pod.OwnerReferences[0]
	switch ref.Kind {
	case "StatefulSet":
		return OwnerStatefulSet, ref.Name, nil
	case "DaemonSet":
		return OwnerDaemonSet, ref.Name, nil
	case "ReplicaSet":
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
	default:
		return "", "", fmt.Errorf(
			"pod %s/%s is owned by a %s, which Restart doesn't support yet (only bare pods, Deployment-, StatefulSet-, and DaemonSet-owned pods are — see internal/k8s/restart.go)",
			namespace, pod.Name, ref.Kind)
	}
}

// Restart deletes-and-recreates a bare pod, or triggers a rollout
// restart for a Deployment/StatefulSet/DaemonSet-owned pod.
//
// This exists to close docs/e2e-demo.md Finding 2: trace_open only
// observes openat(), not writes on an already-open fd, so resources a
// process opens once at startup (a pid file, a log fd held open for the
// process lifetime) are invisible to a trace that attaches to a
// container already running before the observation window started —
// the only way to see them is to actually observe a startup.
//
// Restart itself doesn't start the tracer, and doesn't need to report
// back a replacement pod's identity for the caller to do so either —
// cmd/landlock-genprof/trace.go's traceWithRestart decides that
// *before* calling Restart, and always attaches the tracer first, then
// calls this:
//   - Stable name (bare pod, StatefulSet — see KeepsStableName): the
//     tracer is pre-targeted by the unchanging pod name, relying on
//     Inspektor Gadget's KubeManager filter to dynamically re-attach to
//     whichever container matches it. Confirmed live for bare pods (see
//     docs/e2e-demo.md Finding 2): restarting first and only then
//     attaching reliably lost the startup activity, since gadget
//     attachment (a real gRPC handshake per gadget) is slower than an
//     already-cached image's container start.
//   - Unstable name (Deployment, DaemonSet): the tracer is pre-targeted
//     by the owning workload's label selector (PodSelectorFor) instead
//     of an exact pod name, since the replacement gets a new,
//     unpredictable one — closing the same class of miss the bare-pod
//     case had, confirmed live after label-selector filtering replaced
//     the original restart-then-discover-the-name approach (which
//     itself was confirmed broken: an empty profile for a DaemonSet,
//     same root cause as the original bare-pod bug).
//
// Because identity/targeting is fully decided before this is called,
// Restart itself only ever needs to trigger the right mechanism and
// report success or failure — not discover or return anything.
func Restart(ctx context.Context, client kubernetes.Interface, target *TargetPod) error {
	pod, err := client.CoreV1().Pods(target.Namespace).Get(ctx, target.PodName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("fetching pod %s/%s: %w", target.Namespace, target.PodName, err)
	}

	owner, ownerName, err := DetectOwner(ctx, client, target.Namespace, pod)
	if err != nil {
		return err
	}

	switch owner {
	case OwnerNone:
		return restartBarePod(ctx, client, pod)
	case OwnerDeployment:
		return restartDeployment(ctx, client, target.Namespace, ownerName)
	case OwnerStatefulSet:
		return restartStatefulSet(ctx, client, target.Namespace, ownerName)
	case OwnerDaemonSet:
		return restartDaemonSet(ctx, client, target.Namespace, ownerName)
	default:
		return fmt.Errorf("unhandled owner kind %q", owner)
	}
}

// PodSelectorFor returns the label selector matching owner's pods —
// Deployment or DaemonSet only (the two !KeepsStableName kinds; a bare
// pod has no controller-defined selector, and StatefulSet doesn't need
// one since its pods keep a stable name). Called by
// cmd/landlock-genprof/trace.go's traceWithRestart *before* triggering a
// restart, so the tracer can be pre-attached via this selector
// (tracer.Options.Selector) instead of an exact pod name that's about to
// stop existing.
func PodSelectorFor(ctx context.Context, client kubernetes.Interface, namespace string, owner OwnerKind, ownerName string) (*metav1.LabelSelector, error) {
	switch owner {
	case OwnerDeployment:
		d, err := client.AppsV1().Deployments(namespace).Get(ctx, ownerName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("fetching deployment %s/%s: %w", namespace, ownerName, err)
		}
		return d.Spec.Selector, nil
	case OwnerDaemonSet:
		d, err := client.AppsV1().DaemonSets(namespace).Get(ctx, ownerName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("fetching daemonset %s/%s: %w", namespace, ownerName, err)
		}
		return d.Spec.Selector, nil
	default:
		return nil, fmt.Errorf("PodSelectorFor: unsupported owner kind %q (only Deployment/DaemonSet have one)", owner)
	}
}

// restartBarePod deletes pod and recreates it under the same name with
// the same spec/labels — nothing else will bring a bare pod back.
func restartBarePod(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod) error {
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
		return fmt.Errorf("deleting pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	// A pod object must be fully gone (not just Terminating) before a
	// new one can be created under the same name.
	if err := waitForPodGone(ctx, client, pod.Namespace, pod.Name); err != nil {
		return err
	}

	if _, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, newPod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("recreating pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	return nil
}

// rolloutRestartAnnotation is the same annotation key `kubectl rollout
// restart` itself patches onto a Deployment/StatefulSet/DaemonSet's pod
// template to force a new rollout — a stable, documented part of
// kubectl's own behavior (all three resource kinds support `kubectl
// rollout restart`), not an Inspektor-Gadget-style guess.
const rolloutRestartAnnotation = "kubectl.kubernetes.io/restartedAt"

func rolloutRestartPatch() []byte {
	return []byte(fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		rolloutRestartAnnotation, time.Now().Format(time.RFC3339)))
}

// restartDeployment triggers a rollout restart on deploymentName the
// same way `kubectl rollout restart` does. The caller has already
// pre-attached the tracer via PodSelectorFor before calling this — no
// wait, no replacement to discover here.
func restartDeployment(ctx context.Context, client kubernetes.Interface, namespace, deploymentName string) error {
	_, err := client.AppsV1().Deployments(namespace).Patch(
		ctx, deploymentName, types.StrategicMergePatchType, rolloutRestartPatch(), metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patching deployment %s/%s to trigger a rollout restart: %w", namespace, deploymentName, err)
	}
	return nil
}

// restartStatefulSet triggers a rollout restart on statefulSetName the
// same way `kubectl rollout restart` does. A StatefulSet's pods keep
// their deterministic `<name>-<ordinal>` names across a rolling
// restart, so — like restartDeployment now, but for a different reason
// — there's nothing to discover: the caller already has the tracer
// attached to that stable name before calling this (see
// KeepsStableName).
func restartStatefulSet(ctx context.Context, client kubernetes.Interface, namespace, statefulSetName string) error {
	_, err := client.AppsV1().StatefulSets(namespace).Patch(
		ctx, statefulSetName, types.StrategicMergePatchType, rolloutRestartPatch(), metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patching statefulset %s/%s to trigger a rollout restart: %w", namespace, statefulSetName, err)
	}
	return nil
}

// restartDaemonSet triggers a rollout restart on daemonSetName the same
// way `kubectl rollout restart` does. Like restartDeployment, the caller
// has already pre-attached the tracer via PodSelectorFor — DaemonSet
// pods use `generateName` like ReplicaSet-owned ones (a new random
// suffix every recreation, not a stable identity like StatefulSet), but
// that no longer matters here since nothing needs the new name anymore.
func restartDaemonSet(ctx context.Context, client kubernetes.Interface, namespace, daemonSetName string) error {
	_, err := client.AppsV1().DaemonSets(namespace).Patch(
		ctx, daemonSetName, types.StrategicMergePatchType, rolloutRestartPatch(), metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patching daemonset %s/%s to trigger a rollout restart: %w", namespace, daemonSetName, err)
	}
	return nil
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
