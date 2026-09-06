// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package k8s

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	domain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

// ErrObservationTargetUnresolved marks a target fact that real Kubernetes
// state does not currently establish. Callers must surface this explicitly
// (e.g. as an unresolved binding/UNKNOWN qualification) and must never
// substitute a name, tag, or invented token in its place.
var ErrObservationTargetUnresolved = errors.New("OBSERVATION_TARGET_UNRESOLVED")

// imageDigestPattern extracts the immutable "sha256:<64 lowercase/uppercase
// hex>" suffix from a container runtime's ImageID, which may be prefixed by
// a registry/repository path (e.g. "docker.io/library/nginx@sha256:...").
var imageDigestPattern = regexp.MustCompile(`sha256:[0-9a-fA-F]{64}$`)

// ResolveContainerImageRevision extracts an immutable digest from a Pod
// container status's ImageID. It never falls back to the mutable image tag
// in Pod.Spec.Containers[].Image, and it never fabricates a digest when the
// runtime has not reported one (e.g. the container has not started yet).
func ResolveContainerImageRevision(slot domain.ContainerSlot, status corev1.ContainerStatus) (domain.ContainerImageRevision, error) {
	digest := imageDigestPattern.FindString(strings.TrimSpace(status.ImageID))
	if digest == "" {
		return domain.ContainerImageRevision{}, fmt.Errorf("%w: no immutable image digest reported for container %q", ErrObservationTargetUnresolved, slot.Container)
	}
	return domain.NewContainerImageRevision(slot, digest)
}

// containerStatus returns the ContainerStatus entry for the named container,
// searching both regular and init containers (an init container can be a
// legitimate Observation target).
func containerStatus(pod *corev1.Pod, container string) (corev1.ContainerStatus, error) {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == container {
			return status, nil
		}
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if status.Name == container {
			return status, nil
		}
	}
	return corev1.ContainerStatus{}, fmt.Errorf("%w: container %q not found in Pod %s/%s status", ErrObservationTargetUnresolved, container, pod.Namespace, pod.Name)
}

// ResolveWorkloadIdentity resolves the stable WorkloadIdentity and, where the
// owning kind's revision facts are actually available, a WorkloadRevision,
// from a Pod's real controller ownership chain.
//
// Supported kinds, exactly as planned:
//   - Deployment (via the owning ReplicaSet's "deployment.kubernetes.io/revision"
//     annotation)
//   - StatefulSet / DaemonSet (via the Pod's "controller-revision-hash" label,
//     the same mechanism both controller kinds use)
//   - bare ReplicaSet (identity only; no revision concept beyond its own UID)
//   - any other single controller owner reference (identity only)
//   - no controller owner reference at all: the Pod itself is the workload
//
// hasRevision is false, never a fabricated value, whenever the owning kind's
// revision facts are not actually available. This is not an error.
func ResolveWorkloadIdentity(ctx context.Context, client kubernetes.Interface, cluster domain.ClusterIdentity, pod *corev1.Pod) (domain.WorkloadIdentity, domain.WorkloadRevision, bool, error) {
	owner := controllerOwner(pod.OwnerReferences)
	if owner == nil {
		identity := domain.WorkloadIdentity{Cluster: cluster, Namespace: pod.Namespace, GroupKind: domain.GroupKind{Group: "", Kind: "Pod"}, Name: pod.Name, UID: string(pod.UID)}
		if !identity.Valid() {
			return domain.WorkloadIdentity{}, domain.WorkloadRevision{}, false, fmt.Errorf("%w: Pod %s/%s is not a valid workload identity", ErrObservationTargetUnresolved, pod.Namespace, pod.Name)
		}
		return identity, domain.WorkloadRevision{}, false, nil
	}

	group, _ := groupFromAPIVersion(owner.APIVersion)

	switch owner.Kind {
	case "ReplicaSet":
		replicaSet, err := client.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		if err != nil {
			return domain.WorkloadIdentity{}, domain.WorkloadRevision{}, false, fmt.Errorf("%w: reading owning ReplicaSet %s/%s: %v", ErrObservationTargetUnresolved, pod.Namespace, owner.Name, err)
		}
		if deploymentOwner := controllerOwner(replicaSet.OwnerReferences); deploymentOwner != nil && deploymentOwner.Kind == "Deployment" {
			deploymentGroup, _ := groupFromAPIVersion(deploymentOwner.APIVersion)
			identity := domain.WorkloadIdentity{Cluster: cluster, Namespace: pod.Namespace, GroupKind: domain.GroupKind{Group: deploymentGroup, Kind: "Deployment"}, Name: deploymentOwner.Name, UID: string(deploymentOwner.UID)}
			if !identity.Valid() {
				return domain.WorkloadIdentity{}, domain.WorkloadRevision{}, false, fmt.Errorf("%w: Deployment owner reference for Pod %s/%s is incomplete", ErrObservationTargetUnresolved, pod.Namespace, pod.Name)
			}
			revisionValue := strings.TrimSpace(replicaSet.Annotations["deployment.kubernetes.io/revision"])
			if revisionValue == "" {
				return identity, domain.WorkloadRevision{}, false, nil
			}
			revision, err := domain.NewWorkloadRevision(revisionValue)
			if err != nil {
				return identity, domain.WorkloadRevision{}, false, nil
			}
			return identity, revision, true, nil
		}
		// A bare ReplicaSet not owned by a Deployment: identity only.
		identity := domain.WorkloadIdentity{Cluster: cluster, Namespace: pod.Namespace, GroupKind: domain.GroupKind{Group: group, Kind: "ReplicaSet"}, Name: owner.Name, UID: string(owner.UID)}
		if !identity.Valid() {
			return domain.WorkloadIdentity{}, domain.WorkloadRevision{}, false, fmt.Errorf("%w: ReplicaSet owner reference for Pod %s/%s is incomplete", ErrObservationTargetUnresolved, pod.Namespace, pod.Name)
		}
		return identity, domain.WorkloadRevision{}, false, nil

	case "StatefulSet", "DaemonSet":
		identity := domain.WorkloadIdentity{Cluster: cluster, Namespace: pod.Namespace, GroupKind: domain.GroupKind{Group: group, Kind: owner.Kind}, Name: owner.Name, UID: string(owner.UID)}
		if !identity.Valid() {
			return domain.WorkloadIdentity{}, domain.WorkloadRevision{}, false, fmt.Errorf("%w: %s owner reference for Pod %s/%s is incomplete", ErrObservationTargetUnresolved, owner.Kind, pod.Namespace, pod.Name)
		}
		revisionValue := strings.TrimSpace(pod.Labels["controller-revision-hash"])
		if revisionValue == "" {
			return identity, domain.WorkloadRevision{}, false, nil
		}
		revision, err := domain.NewWorkloadRevision(revisionValue)
		if err != nil {
			return identity, domain.WorkloadRevision{}, false, nil
		}
		return identity, revision, true, nil

	default:
		// Any other single controller owner (e.g. Job): identity is a real,
		// authoritative fact from the owner reference itself; that kind's
		// revision semantics are not implemented, so no revision is claimed.
		identity := domain.WorkloadIdentity{Cluster: cluster, Namespace: pod.Namespace, GroupKind: domain.GroupKind{Group: group, Kind: owner.Kind}, Name: owner.Name, UID: string(owner.UID)}
		if !identity.Valid() {
			return domain.WorkloadIdentity{}, domain.WorkloadRevision{}, false, fmt.Errorf("%w: %s owner reference for Pod %s/%s is incomplete", ErrObservationTargetUnresolved, owner.Kind, pod.Namespace, pod.Name)
		}
		return identity, domain.WorkloadRevision{}, false, nil
	}
}

// controllerOwner returns the single owner reference with Controller==true,
// or nil if there is none (ambiguous or absent controller ownership is
// treated identically to no ownership: the Pod itself is the workload).
func controllerOwner(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	return nil
}

func groupFromAPIVersion(apiVersion string) (string, string) {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", apiVersion
}

// ObservationTarget is the fully resolved, ready-to-bind result for one
// container in one running Pod: identity facts only, never mutable
// execution/result state.
type ObservationTarget struct {
	Workload domain.WorkloadIdentity
	Revision domain.WorkloadRevision
	HasRevision bool
	Instance domain.RuntimeContainerInstance
}

// ResolveObservationTarget resolves everything Observation binding needs
// for one container in one currently-running Pod, from real, current
// Kubernetes/runtime facts only. It never substitutes a context name for
// ClusterIdentity, a workload name for a workload UID, a Pod name for a Pod
// UID, or an image tag for an image digest — each is either a genuine fact
// or the corresponding value is left unresolved.
func ResolveObservationTarget(ctx context.Context, client kubernetes.Interface, cluster domain.ClusterIdentity, pod *corev1.Pod, container string) (ObservationTarget, error) {
	if pod == nil {
		return ObservationTarget{}, fmt.Errorf("%w: nil Pod", ErrObservationTargetUnresolved)
	}
	if string(pod.UID) == "" {
		return ObservationTarget{}, fmt.Errorf("%w: Pod %s/%s has no UID", ErrObservationTargetUnresolved, pod.Namespace, pod.Name)
	}
	workload, revision, hasRevision, err := ResolveWorkloadIdentity(ctx, client, cluster, pod)
	if err != nil {
		return ObservationTarget{}, err
	}
	slot := domain.ContainerSlot{Workload: workload, Container: container}
	if !slot.Valid() {
		return ObservationTarget{}, fmt.Errorf("%w: invalid container slot for %s/%s container %q", ErrObservationTargetUnresolved, pod.Namespace, pod.Name, container)
	}
	status, err := containerStatus(pod, container)
	if err != nil {
		return ObservationTarget{}, err
	}
	instance := domain.RuntimeContainerInstance{Slot: slot, PodUID: string(pod.UID)}
	if id := runtimeContainerID(status.ContainerID); id != "" {
		instance.ContainerID = id
	}
	if revisionValue, err := ResolveContainerImageRevision(slot, status); err == nil {
		instance.ImageRevision = &revisionValue
	}
	if !instance.Valid() {
		return ObservationTarget{}, fmt.Errorf("%w: invalid runtime container instance for %s/%s container %q", ErrObservationTargetUnresolved, pod.Namespace, pod.Name, container)
	}
	return ObservationTarget{Workload: workload, Revision: revision, HasRevision: hasRevision, Instance: instance}, nil
}

// runtimeContainerID strips the "<runtime>://" prefix Kubernetes reports
// (e.g. "containerd://<id>") to retain only the runtime-native identifier;
// an empty/unset ContainerID (container not yet started) stays empty.
func runtimeContainerID(reported string) string {
	if _, id, found := strings.Cut(reported, "://"); found {
		return id
	}
	return reported
}
