// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package domain contains policy-neutral, Kubernetes-independent Observation
// identity values. Kubernetes adapters resolve these values but do not define
// their semantics.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrClusterIdentityUnresolved = errors.New("CLUSTER_IDENTITY_UNRESOLVED")

// ClusterIdentity is the opaque Kubernetes-local identity anchor selected by
// ADR-0027: the UID of the kube-system Namespace.
type ClusterIdentity struct{ NamespaceUID string }

func NewClusterIdentity(namespaceUID string) (ClusterIdentity, error) {
	if strings.TrimSpace(namespaceUID) == "" {
		return ClusterIdentity{}, fmt.Errorf("%w: kube-system Namespace UID is empty", ErrClusterIdentityUnresolved)
	}
	return ClusterIdentity{NamespaceUID: namespaceUID}, nil
}

// ClusterLocator describes how a cluster was reached. It is never part of
// ClusterIdentity and is not an authority identifier.
type ClusterLocator struct {
	KubeconfigSource string
	Context          string
	APIURL           string
}

type GroupKind struct {
	Group string
	Kind  string
}

// WorkloadIdentity identifies the Kubernetes workload object, including its
// object UID so delete/recreate is distinguishable from a stable name.
type WorkloadIdentity struct {
	Cluster   ClusterIdentity
	Namespace string
	GroupKind GroupKind
	Name      string
	UID       string
}

func (w WorkloadIdentity) Valid() bool {
	return w.Cluster.NamespaceUID != "" && w.Namespace != "" &&
		w.GroupKind.Kind != "" && w.Name != "" && w.UID != ""
}

// WorkloadRevision is an opaque revision token. The current repository does
// not expose one universal rollout-revision algorithm, so callers must supply
// a platform-derived token and must not manufacture one from a name or tag.
type WorkloadRevision struct{ Value string }

func NewWorkloadRevision(value string) (WorkloadRevision, error) {
	if strings.TrimSpace(value) == "" {
		return WorkloadRevision{}, fmt.Errorf("workload revision is unresolved")
	}
	return WorkloadRevision{Value: value}, nil
}

// ContainerSlot identifies a logical container position in a workload.
// Image and runtime-instance facts are intentionally outside this value.
type ContainerSlot struct {
	Workload  WorkloadIdentity
	Container string
}

func (s ContainerSlot) Valid() bool {
	return s.Workload.Valid() && s.Container != ""
}

var digestPattern = regexp.MustCompile(`(?:^|[@/:])sha256:[0-9a-fA-F]{64}$`)

// ContainerImageRevision identifies immutable image content for a slot.
// A mutable tag is not accepted as a digest-equivalent identity.
type ContainerImageRevision struct {
	Slot        ContainerSlot
	ImageDigest string
}

func NewContainerImageRevision(slot ContainerSlot, imageDigest string) (ContainerImageRevision, error) {
	if !slot.Valid() {
		return ContainerImageRevision{}, fmt.Errorf("container image revision has invalid container slot")
	}
	if !digestPattern.MatchString(imageDigest) {
		return ContainerImageRevision{}, fmt.Errorf("container image revision requires an immutable sha256 digest")
	}
	return ContainerImageRevision{Slot: slot, ImageDigest: imageDigest}, nil
}

// RuntimeContainerInstance identifies one concrete running Pod/container.
// ContainerID is optional because Kubernetes discovery does not always expose
// the runtime ID; an absent value remains unknown rather than fabricated.
type RuntimeContainerInstance struct {
	Slot          ContainerSlot
	PodUID        string
	ContainerID   string
	ImageRevision *ContainerImageRevision
}

func (r RuntimeContainerInstance) Valid() bool {
	return r.Slot.Valid() && r.PodUID != ""
}
