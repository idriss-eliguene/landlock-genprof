// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"strings"
	"testing"

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
	client := fake.NewSimpleClientset(runningPod("default", "nginx-demo", "nginx"))

	target, err := Resolve(context.Background(), client, "default", "nginx-demo", "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if target.Container != "nginx" {
		t.Errorf("Container = %q, want %q (déduit, un seul conteneur)", target.Container, "nginx")
	}
	if target.Namespace != "default" || target.PodName != "nginx-demo" {
		t.Errorf("TargetPod = %+v, want Namespace=default PodName=nginx-demo", target)
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

func TestResolve_AmbiguousWithoutContainerFlag(t *testing.T) {
	client := fake.NewSimpleClientset(runningPod("default", "multi-demo", "sidecar", "app"))

	_, err := Resolve(context.Background(), client, "default", "multi-demo", "")
	if err == nil {
		t.Fatal("Resolve() error = nil, want an error (plusieurs conteneurs, --container requis)")
	}
	if !strings.Contains(err.Error(), "plusieurs conteneurs") {
		t.Errorf("err = %q, want a message about multiple containers", err)
	}
}

func TestResolve_UnknownContainer(t *testing.T) {
	client := fake.NewSimpleClientset(runningPod("default", "nginx-demo", "nginx"))

	_, err := Resolve(context.Background(), client, "default", "nginx-demo", "does-not-exist")
	if err == nil {
		t.Fatal("Resolve() error = nil, want an error (conteneur introuvable)")
	}
	if !strings.Contains(err.Error(), "introuvable") {
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
		t.Fatal("Resolve() error = nil, want an error (pod pas Running)")
	}
	if !strings.Contains(err.Error(), "Pending") {
		t.Errorf("err = %q, want it to mention the actual phase (Pending)", err)
	}
}
