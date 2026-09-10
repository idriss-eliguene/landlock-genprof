// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package k8s

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	domain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

func testCluster(t *testing.T) domain.ClusterIdentity {
	t.Helper()
	identity, err := domain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func boolPtr(v bool) *bool { return &v }

// T1: UID-based WorkloadIdentity target resolution — a Deployment-owned Pod
// resolves to the Deployment's real object UID (via the ReplicaSet's own
// controller owner reference), not merely its name.
func TestResolveWorkloadIdentityDeploymentUsesRealUID(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-6f9d", Namespace: "workloads", UID: "rs-uid",
			Annotations:     map[string]string{"deployment.kubernetes.io/revision": "3"},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "deployment-uid", Controller: boolPtr(true)}},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-6f9d-x1", Namespace: "workloads", UID: "pod-uid",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-6f9d", UID: "rs-uid", Controller: boolPtr(true)}},
		},
	}
	client := fake.NewSimpleClientset(rs)
	workload, revision, hasRevision, err := ResolveWorkloadIdentity(context.Background(), client, testCluster(t), pod)
	if err != nil {
		t.Fatal(err)
	}
	if workload.UID != "deployment-uid" || workload.Name != "api" || workload.GroupKind.Kind != "Deployment" || workload.GroupKind.Group != "apps" {
		t.Fatalf("workload = %#v, want Deployment/api with real UID", workload)
	}
	if !hasRevision || revision.Value != "3" {
		t.Fatalf("revision = %#v hasRevision=%v, want value=3", revision, hasRevision)
	}
}

// T1 (StatefulSet path): revision comes from the Pod's own
// controller-revision-hash label, matching real StatefulSet/DaemonSet
// semantics; no separate ControllerRevision fetch is required.
func TestResolveWorkloadIdentityStatefulSetUsesControllerRevisionHashLabel(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cache-0", Namespace: "workloads", UID: "pod-uid",
			Labels:          map[string]string{"controller-revision-hash": "cache-7c8"},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "cache", UID: "sts-uid", Controller: boolPtr(true)}},
		},
	}
	client := fake.NewSimpleClientset()
	workload, revision, hasRevision, err := ResolveWorkloadIdentity(context.Background(), client, testCluster(t), pod)
	if err != nil {
		t.Fatal(err)
	}
	if workload.UID != "sts-uid" || workload.GroupKind.Kind != "StatefulSet" {
		t.Fatalf("workload = %#v, want StatefulSet with real UID", workload)
	}
	if !hasRevision || revision.Value != "cache-7c8" {
		t.Fatalf("revision = %#v hasRevision=%v, want cache-7c8", revision, hasRevision)
	}
}

// A bare Pod (no controller owner) is the workload itself; no revision is
// fabricated because none exists.
func TestResolveWorkloadIdentityBarePodHasNoRevision(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "workloads", UID: "pod-uid"}}
	client := fake.NewSimpleClientset()
	workload, _, hasRevision, err := ResolveWorkloadIdentity(context.Background(), client, testCluster(t), pod)
	if err != nil {
		t.Fatal(err)
	}
	if workload.GroupKind.Kind != "Pod" || workload.UID != "pod-uid" {
		t.Fatalf("workload = %#v, want the Pod itself", workload)
	}
	if hasRevision {
		t.Fatal("bare Pod must never report a fabricated revision")
	}
}

// Deployment-owned but the ReplicaSet carries no revision annotation: the
// workload still resolves; the revision is honestly absent, not invented.
func TestResolveWorkloadIdentityMissingRevisionAnnotationIsUnresolvedNotFabricated(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-6f9d", Namespace: "workloads", UID: "rs-uid",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "deployment-uid", Controller: boolPtr(true)}},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-6f9d-x1", Namespace: "workloads", UID: "pod-uid",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-6f9d", UID: "rs-uid", Controller: boolPtr(true)}},
		},
	}
	client := fake.NewSimpleClientset(rs)
	workload, _, hasRevision, err := ResolveWorkloadIdentity(context.Background(), client, testCluster(t), pod)
	if err != nil {
		t.Fatal(err)
	}
	if workload.Name != "api" {
		t.Fatalf("workload identity should still resolve: %#v", workload)
	}
	if hasRevision {
		t.Fatal("missing revision annotation must not be fabricated into a revision")
	}
}

// T3: an image tag must never substitute an immutable digest.
func TestResolveContainerImageRevisionRejectsTagOnlyImageID(t *testing.T) {
	slot := domain.ContainerSlot{Workload: domain.WorkloadIdentity{Cluster: testCluster(t), Namespace: "workloads", GroupKind: domain.GroupKind{Kind: "Pod"}, Name: "p", UID: "pod-uid"}, Container: "backend"}
	status := corev1.ContainerStatus{Name: "backend", ImageID: "", Image: "nginx:latest"}
	if _, err := ResolveContainerImageRevision(slot, status); !errors.Is(err, ErrObservationTargetUnresolved) {
		t.Fatalf("error = %v, want ErrObservationTargetUnresolved for an unset ImageID", err)
	}
}

func TestResolveContainerImageRevisionAcceptsRealDigest(t *testing.T) {
	slot := domain.ContainerSlot{Workload: domain.WorkloadIdentity{Cluster: testCluster(t), Namespace: "workloads", GroupKind: domain.GroupKind{Kind: "Pod"}, Name: "p", UID: "pod-uid"}, Container: "backend"}
	status := corev1.ContainerStatus{Name: "backend", ImageID: "docker.io/library/nginx@sha256:" + fortyEightZeroes()}
	revision, err := ResolveContainerImageRevision(slot, status)
	if err != nil {
		t.Fatal(err)
	}
	if revision.ImageDigest != "sha256:"+fortyEightZeroes() {
		t.Fatalf("digest = %q", revision.ImageDigest)
	}
}

func TestCanonicalImageDigestNormalizesRuntimeIdentity(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, input := range []string{digest, "docker.io/library/nginx@" + digest, "docker-pullable://nginx@" + digest} {
		got, err := CanonicalImageDigest(input)
		if err != nil || got != digest {
			t.Fatalf("CanonicalImageDigest(%q) = %q, %v; want %q", input, got, err, digest)
		}
	}
}

func TestCanonicalImageDigestRejectsAmbiguousOrMutableIdentity(t *testing.T) {
	for _, input := range []string{"nginx:latest", "sha256:" + strings.Repeat("a", 63), "sha512:" + strings.Repeat("a", 64), "repo@sha256:" + strings.Repeat("a", 64) + "@x", "@sha256:" + strings.Repeat("a", 64), " sha256:" + strings.Repeat("a", 64)} {
		if got, err := CanonicalImageDigest(input); err == nil || got != "" {
			t.Errorf("CanonicalImageDigest(%q) = %q, %v; want rejection", input, got, err)
		}
	}
}

func fortyEightZeroes() string {
	digest := make([]byte, 64)
	for i := range digest {
		digest[i] = '0'
	}
	return string(digest)
}

// T2 (documented at this layer, not this package): ClusterIdentity here is
// always the caller-supplied domain.ClusterIdentity produced by
// ResolveClusterIdentity — this resolver has no context-name/locator input
// at all, so there is no code path by which a context name could reach
// WorkloadIdentity.Cluster.
func TestResolveObservationTargetEndToEnd(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-6f9d", Namespace: "workloads", UID: "rs-uid",
			Annotations:     map[string]string{"deployment.kubernetes.io/revision": "3"},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "deployment-uid", Controller: boolPtr(true)}},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-6f9d-x1", Namespace: "workloads", UID: "pod-uid",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-6f9d", UID: "rs-uid", Controller: boolPtr(true)}},
		},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "backend", ContainerID: "containerd://runtime-id", ImageID: "docker.io/library/nginx@sha256:" + fortyEightZeroes()}}
	client := fake.NewSimpleClientset(rs)
	target, err := ResolveObservationTarget(context.Background(), client, testCluster(t), pod, "backend")
	if err != nil {
		t.Fatal(err)
	}
	if target.Instance.PodUID != "pod-uid" {
		t.Fatalf("PodUID = %q, want the real Pod UID, not the Pod name", target.Instance.PodUID)
	}
	if target.Instance.ContainerID != "runtime-id" {
		t.Fatalf("ContainerID = %q, want the runtime-native ID without the containerd:// prefix", target.Instance.ContainerID)
	}
	if target.Instance.ImageRevision == nil || target.Instance.ImageRevision.ImageDigest != "sha256:"+fortyEightZeroes() {
		t.Fatalf("ImageRevision = %#v, want the real digest", target.Instance.ImageRevision)
	}
	if !target.HasRevision || target.Revision.Value != "3" {
		t.Fatalf("Revision = %#v HasRevision=%v", target.Revision, target.HasRevision)
	}
}

// A container that has not yet reported an ImageID still resolves as a
// target (its PodUID/ContainerSlot are real and sufficient for binding),
// but the image revision stays explicitly absent rather than fabricated —
// callers must treat ImageRevision == nil as "unresolved", never as a
// license to skip the revision-boundary check.
func TestResolveObservationTargetLeavesImageRevisionUnresolvedWithoutImageID(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "workloads", UID: "pod-uid"}}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "backend"}}
	client := fake.NewSimpleClientset()
	target, err := ResolveObservationTarget(context.Background(), client, testCluster(t), pod, "backend")
	if err != nil {
		t.Fatal(err)
	}
	if target.Instance.ImageRevision != nil {
		t.Fatalf("ImageRevision = %#v, want nil (unresolved) rather than fabricated", target.Instance.ImageRevision)
	}
	if target.Instance.PodUID != "pod-uid" {
		t.Fatalf("PodUID = %q, resolution of the real Pod UID must not depend on the image digest", target.Instance.PodUID)
	}
}
