// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package workload provides the bounded, read-only workload discovery model
// used by the future Cluster Workbench.
package workload

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

type DiscoveryState string

const (
	StateReady DiscoveryState = "READY"
	StateEmpty DiscoveryState = "EMPTY"
)

type OwnerState string

const (
	OwnerSupported   OwnerState = "SUPPORTED"
	OwnerBarePod     OwnerState = "BARE_POD"
	OwnerUnsupported OwnerState = "UNSUPPORTED"
	OwnerUnresolved  OwnerState = "UNRESOLVED"
)

type ContainerCategory string

const (
	ContainerRegular    ContainerCategory = "REGULAR"
	ContainerInit       ContainerCategory = "INIT"
	ContainerNativeSide ContainerCategory = "NATIVE_SIDECAR"
	ContainerEphemeral  ContainerCategory = "EPHEMERAL"
)

type RuntimeState string

const (
	RuntimeAvailable         RuntimeState = "AVAILABLE"
	RuntimeImageUnknown      RuntimeState = "IMAGE_ID_UNKNOWN"
	RuntimeStatusUnavailable RuntimeState = "STATUS_UNAVAILABLE"
)

// Result is a successful bounded discovery result. StateEmpty is distinct
// from a read error such as permission denied.
type Result struct {
	State     DiscoveryState
	Namespace string
	Workloads []Workload
}

// Workload is one logical workload in the session namespace. Pods are runtime
// incarnations and remain nested rather than becoming separate workloads.
type Workload struct {
	Target    k8s.WorkloadRef
	Owner     OwnerState
	OwnerNote string
	Pods      []Pod
}

type Pod struct {
	Name                   string
	UID                    string
	ResourceVersion        string
	Containers             []Container
	UnmatchedRuntimeStatus []string
}

type Container struct {
	Name            string
	Category        ContainerCategory
	SupportedTarget bool
	Target          *k8s.GovernedTarget
	Runtime         *k8s.RuntimeSubject
	RuntimeState    RuntimeState
}

// Service discovers supported workloads from the fixed namespace of a
// WorkbenchReadCapability. It performs one bounded Pod LIST and memoized
// owner GETs; it never constructs or exposes a Kubernetes client.
type Service struct{ reads k8s.WorkbenchReadCapability }

func NewService(reads k8s.WorkbenchReadCapability) (*Service, error) {
	if reads == nil {
		return nil, fmt.Errorf("workload discovery requires a read capability")
	}
	return &Service{reads: reads}, nil
}

func (s *Service) Discover(ctx context.Context) (Result, error) {
	identity := s.reads.SessionIdentity()
	result := Result{State: StateReady, Namespace: identity.Namespace}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	pods, err := s.reads.ListPods(ctx)
	if err != nil {
		return result, err
	}
	if len(pods) == 0 {
		result.State = StateEmpty
		return result, nil
	}

	resolved := make(map[string]ownerResolution)
	workloads := make(map[string]*Workload)
	for i := range pods {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		pod := &pods[i]
		owner, err := s.resolveOwner(ctx, pod, resolved)
		if err != nil {
			return result, err
		}
		key := owner.key(identity.Namespace, pod.Name, string(pod.UID))
		workload := workloads[key]
		if workload == nil {
			workload = &Workload{Target: owner.target, Owner: owner.state, OwnerNote: owner.note}
			workloads[key] = workload
		}
		workload.Pods = append(workload.Pods, s.enumeratePod(identity.Namespace, pod, owner))
	}

	result.Workloads = make([]Workload, 0, len(workloads))
	for _, workload := range workloads {
		for i := range workload.Pods {
			sort.SliceStable(workload.Pods[i].Containers, func(a, b int) bool {
				return containerOrder(workload.Pods[i].Containers[a]) < containerOrder(workload.Pods[i].Containers[b])
			})
			sort.Strings(workload.Pods[i].UnmatchedRuntimeStatus)
		}
		sort.SliceStable(workload.Pods, func(a, b int) bool {
			if workload.Pods[a].Name != workload.Pods[b].Name {
				return workload.Pods[a].Name < workload.Pods[b].Name
			}
			return workload.Pods[a].UID < workload.Pods[b].UID
		})
		result.Workloads = append(result.Workloads, *workload)
	}
	sort.SliceStable(result.Workloads, func(a, b int) bool {
		return workloadKey(result.Workloads[a].Target) < workloadKey(result.Workloads[b].Target)
	})
	return result, nil
}

type ownerResolution struct {
	target k8s.WorkloadRef
	state  OwnerState
	note   string
	keyRef string
}

func (o ownerResolution) key(namespace, podName, podUID string) string {
	if o.keyRef != "" {
		return namespace + "/" + o.keyRef
	}
	return namespace + "/Pod/" + podName + "/" + podUID
}

func (s *Service) resolveOwner(ctx context.Context, pod *corev1.Pod, cache map[string]ownerResolution) (ownerResolution, error) {
	ref, ok := controllingOwner(pod.OwnerReferences)
	if !ok {
		if len(pod.OwnerReferences) != 0 {
			return ownerResolution{state: OwnerUnsupported, note: "owner reference is not controlling", target: k8s.WorkloadRef{Kind: "Pod", Name: pod.Name}, keyRef: "unsupported/" + pod.Name}, nil
		}
		return ownerResolution{state: OwnerBarePod, target: k8s.WorkloadRef{Kind: "Pod", Name: pod.Name}}, nil
	}
	cacheKey := ref.APIVersion + "/" + ref.Kind + "/" + ref.Name
	if cached, found := cache[cacheKey]; found {
		return cached, nil
	}
	group := apiGroup(ref.APIVersion)
	resolution := ownerResolution{state: OwnerUnsupported, target: k8s.WorkloadRef{Group: group, Kind: ref.Kind, Name: ref.Name}, keyRef: group + "/" + ref.Kind + "/" + ref.Name}
	if group != "apps" {
		resolution.note = "owner group is outside the supported apps group"
		cache[cacheKey] = resolution
		return resolution, nil
	}
	var obj interface{}
	var err error
	switch ref.Kind {
	case "Deployment":
		obj, err = s.reads.GetDeployment(ctx, ref.Name)
	case "StatefulSet":
		obj, err = s.reads.GetStatefulSet(ctx, ref.Name)
	case "DaemonSet":
		obj, err = s.reads.GetDaemonSet(ctx, ref.Name)
	case "ReplicaSet":
		obj, err = s.reads.GetReplicaSet(ctx, ref.Name)
	default:
		resolution.note = "owner kind is outside the supported workload set"
		cache[cacheKey] = resolution
		return resolution, nil
	}
	if err != nil {
		var readErr *k8s.ReadError
		if errors.As(err, &readErr) && readErr.State == k8s.ReadNotFound {
			resolution.state = OwnerUnresolved
			resolution.note = "owner object was not found"
			cache[cacheKey] = resolution
			return resolution, nil
		}
		return resolution, err
	}
	ownerObject, ok := obj.(*unstructured.Unstructured)
	if !ok || ref.UID == "" || ownerObject.GetUID() != ref.UID {
		resolution.state = OwnerUnresolved
		resolution.note = "owner reference UID does not match the current owner object"
		cache[cacheKey] = resolution
		return resolution, nil
	}
	if ref.Kind == "ReplicaSet" {
		ownerRefs := ownerObject.GetOwnerReferences()
		parent, parentOK := controllingOwner(ownerRefs)
		if !parentOK || parent.Kind != "Deployment" {
			resolution.note = "ReplicaSet has no supported Deployment controller"
			cache[cacheKey] = resolution
			return resolution, nil
		}
		resolution.target = k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: parent.Name}
		resolution.state = OwnerSupported
		resolution.keyRef = workloadKey(resolution.target)
	} else {
		resolution.target.Group = "apps"
		resolution.state = OwnerSupported
		resolution.note = "supported workload owner"
		resolution.keyRef = workloadKey(resolution.target)
	}
	cache[cacheKey] = resolution
	return resolution, nil
}

func controllingOwner(refs []metav1.OwnerReference) (metav1.OwnerReference, bool) {
	for _, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			return ref, true
		}
	}
	return metav1.OwnerReference{}, false
}

func (s *Service) enumeratePod(namespace string, pod *corev1.Pod, owner ownerResolution) Pod {
	result := Pod{Name: pod.Name, UID: string(pod.UID), ResourceVersion: pod.ResourceVersion}
	regularStatus := statusByName(pod.Status.ContainerStatuses)
	initStatus := statusByName(pod.Status.InitContainerStatuses)
	ephemeralStatus := statusByName(pod.Status.EphemeralContainerStatuses)
	for _, c := range pod.Spec.Containers {
		result.Containers = append(result.Containers, makeContainer(namespace, pod, owner, c.Name, ContainerRegular, regularStatus[c.Name]))
	}
	for _, c := range pod.Spec.InitContainers {
		category := ContainerInit
		if c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			category = ContainerNativeSide
		}
		result.Containers = append(result.Containers, makeContainer(namespace, pod, owner, c.Name, category, initStatus[c.Name]))
	}
	for _, c := range pod.Spec.EphemeralContainers {
		result.Containers = append(result.Containers, makeContainer(namespace, pod, owner, c.Name, ContainerEphemeral, ephemeralStatus[c.Name]))
	}
	known := map[string]bool{}
	for _, c := range pod.Spec.Containers {
		known["regular/"+c.Name] = true
	}
	for _, c := range pod.Spec.InitContainers {
		known["init/"+c.Name] = true
	}
	for _, c := range pod.Spec.EphemeralContainers {
		known["ephemeral/"+c.Name] = true
	}
	for _, status := range pod.Status.ContainerStatuses {
		if !known["regular/"+status.Name] {
			result.UnmatchedRuntimeStatus = append(result.UnmatchedRuntimeStatus, status.Name)
		}
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if !known["init/"+status.Name] {
			result.UnmatchedRuntimeStatus = append(result.UnmatchedRuntimeStatus, status.Name)
		}
	}
	for _, status := range pod.Status.EphemeralContainerStatuses {
		if !known["ephemeral/"+status.Name] {
			result.UnmatchedRuntimeStatus = append(result.UnmatchedRuntimeStatus, status.Name)
		}
	}
	return result
}

func makeContainer(namespace string, pod *corev1.Pod, owner ownerResolution, name string, category ContainerCategory, status *corev1.ContainerStatus) Container {
	container := Container{Name: name, Category: category, SupportedTarget: category == ContainerRegular && owner.state == OwnerSupported || category == ContainerRegular && owner.state == OwnerBarePod, RuntimeState: RuntimeStatusUnavailable}
	if container.SupportedTarget {
		target := k8s.GovernedTarget{Namespace: namespace, Workload: owner.target, Container: name}
		container.Target = &target
	}
	if status != nil && container.Target != nil {
		runtime := k8s.RuntimeSubject{Target: *container.Target, PodUID: string(pod.UID), ImageID: status.ImageID}
		container.Runtime = &runtime
		container.RuntimeState = RuntimeAvailable
		if status.ImageID == "" {
			container.RuntimeState = RuntimeImageUnknown
		}
	}
	return container
}

func statusByName(statuses []corev1.ContainerStatus) map[string]*corev1.ContainerStatus {
	result := make(map[string]*corev1.ContainerStatus, len(statuses))
	for i := range statuses {
		result[statuses[i].Name] = &statuses[i]
	}
	return result
}

func apiGroup(apiVersion string) string {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

func workloadKey(ref k8s.WorkloadRef) string { return ref.Group + "/" + ref.Kind + "/" + ref.Name }

func containerOrder(c Container) string {
	order := map[ContainerCategory]string{ContainerRegular: "0", ContainerInit: "1", ContainerNativeSide: "2", ContainerEphemeral: "3"}
	return order[c.Category] + "/" + c.Name
}
