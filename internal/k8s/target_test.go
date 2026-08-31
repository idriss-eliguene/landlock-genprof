// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func runningPod(namespace, name string, containers ...string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	for _, c := range containers {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: c})
	}
	return pod
}

func TestResolve_SingleContainerDefaultsWithoutFlag(t *testing.T) {
	pod := runningPod("default", "nginx-demo", "nginx")
	pod.UID = "pod-uid"
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "nginx", ImageID: "docker-pullable://nginx@sha256:abc"}}
	client := fake.NewSimpleClientset(pod)

	target, err := Resolve(context.Background(), client, "default", "nginx-demo", "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if target.Container != "nginx" {
		t.Errorf("Container = %q, want %q (deduced, single container)", target.Container, "nginx")
	}
	if target.Namespace != "default" || target.PodName != "nginx-demo" {
		t.Errorf("TargetPod = %+v, want Namespace=default PodName=nginx-demo", target)
	}
	if target.PodUID != "pod-uid" || target.ImageIdentity != "docker-pullable://nginx@sha256:abc" {
		t.Errorf("resolved execution identity = %+v", target)
	}
}

func TestResolve_ExplicitContainerMatch(t *testing.T) {
	client := fake.NewSimpleClientset(runningPod("default", "multi-demo", "sidecar", "app"))

	target, err := Resolve(context.Background(), client, "default", "multi-demo", "app")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if target.Container != "app" {
		t.Errorf("Container = %q, want app", target.Container)
	}
}

func TestResolve_UsesCanonicalDeploymentTargetAcrossPodReplacement(t *testing.T) {
	controller := true
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: "payments-abc", Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "payments"}},
	}}
	first := runningPod("default", "payments-abc-1", "app")
	first.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", Name: rs.Name, Controller: &controller}}
	first.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "app", ImageID: "image@sha256:a"}}
	second := runningPod("default", "payments-abc-2", "app")
	second.OwnerReferences = first.OwnerReferences
	second.Status.ContainerStatuses = first.Status.ContainerStatuses
	client := fake.NewSimpleClientset(rs, first, second)

	a, err := Resolve(context.Background(), client, "default", first.Name, "app")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Resolve(context.Background(), client, "default", second.Name, "app")
	if err != nil {
		t.Fatal(err)
	}
	if a.GovernedTarget.LegacyString() != "Deployment/payments" || !b.GovernedTarget.Equal(a.GovernedTarget) {
		t.Fatalf("replacement target identities = %+v/%+v", a.GovernedTarget, b.GovernedTarget)
	}
}

func TestGovernedTargetIdentityBoundaries(t *testing.T) {
	base := GovernedTarget{Namespace: "default", Workload: WorkloadRef{Group: "apps", Kind: "Deployment", Name: "payments"}, Container: "api"}
	tests := []struct {
		name      string
		mutate    func(*GovernedTarget)
		wantEqual bool
	}{
		{"api version is excluded", func(_ *GovernedTarget) {}, true},
		{"namespace separates", func(v *GovernedTarget) { v.Namespace = "other" }, false},
		{"kind separates", func(v *GovernedTarget) { v.Workload.Kind = "StatefulSet" }, false},
		{"group separates", func(v *GovernedTarget) { v.Workload.Group = "custom.example" }, false},
		{"name separates", func(v *GovernedTarget) { v.Workload.Name = "other" }, false},
		{"container separates", func(v *GovernedTarget) { v.Container = "sidecar" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := base
			tt.mutate(&got)
			if got.Equal(base) != tt.wantEqual {
				t.Fatalf("Equal() = %v, want %v: %+v", got.Equal(base), tt.wantEqual, got)
			}
		})
	}
}

func TestGovernedTargetCanonicalJSONIsDeterministicAndDistinct(t *testing.T) {
	a := GovernedTarget{Namespace: "default", Workload: WorkloadRef{Group: "apps", Kind: "Deployment", Name: "payments"}, Container: "api"}
	b := a
	b.Workload.Name = "payments-v2"
	left, err := a.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	right, err := a.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("canonical form changed: %q != %q", left, right)
	}
	other, err := b.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if left == other {
		t.Fatalf("distinct targets share canonical form: %q", left)
	}
}

func TestGovernedTargetSeparatesRuntimeAndEvidenceIdentity(t *testing.T) {
	target := GovernedTarget{Namespace: "default", Workload: WorkloadRef{Group: "apps", Kind: "Deployment", Name: "payments"}, Container: "api"}
	a := RuntimeSubject{Target: target, PodUID: "pod-a", ImageID: "image-a", BinaryPath: "/bin/api"}
	b := RuntimeSubject{Target: target, PodUID: "pod-b", ImageID: "image-b", BinaryPath: "/bin/api-v2"}
	if !a.Target.Equal(b.Target) {
		t.Fatal("runtime incarnation changed governed target")
	}
	if a.PodUID == b.PodUID || a.ImageID == b.ImageID || a.BinaryPath == b.BinaryPath {
		t.Fatal("runtime subjects did not retain distinct attribution")
	}
}

func TestResolve_AmbiguousWithoutContainerFlag(t *testing.T) {
	client := fake.NewSimpleClientset(runningPod("default", "multi-demo", "sidecar", "app"))

	_, err := Resolve(context.Background(), client, "default", "multi-demo", "")
	if err == nil {
		t.Fatal("Resolve() error = nil, want an error (multiple containers, --container required)")
	}
	if !strings.Contains(err.Error(), "multiple containers") {
		t.Errorf("err = %q, want a message about multiple containers", err)
	}
}

func TestResolve_UnknownContainer(t *testing.T) {
	client := fake.NewSimpleClientset(runningPod("default", "nginx-demo", "nginx"))

	_, err := Resolve(context.Background(), client, "default", "nginx-demo", "does-not-exist")
	if err == nil {
		t.Fatal("Resolve() error = nil, want an error (container not found)")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %q, want a message about the missing container", err)
	}
}

func TestResolve_PodNotFound(t *testing.T) {
	client := fake.NewSimpleClientset()

	_, err := Resolve(context.Background(), client, "default", "ghost", "")
	if err == nil {
		t.Fatal("Resolve() error = nil, want a not-found error")
	}
}

func TestResolve_PodNotRunning(t *testing.T) {
	pod := runningPod("default", "starting-up", "nginx")
	pod.Status.Phase = corev1.PodPending
	client := fake.NewSimpleClientset(pod)

	_, err := Resolve(context.Background(), client, "default", "starting-up", "")
	if err == nil {
		t.Fatal("Resolve() error = nil, want an error (pod not Running)")
	}
	if !strings.Contains(err.Error(), "Pending") {
		t.Errorf("err = %q, want it to mention the actual phase (Pending)", err)
	}
}
