// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"reflect"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func deploymentOwnedPod(namespace, name, rsName string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: rsName}},
		},
	}
}

func replicaSetOwnedByDeployment(namespace, name, deploymentName string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: deploymentName}},
		},
	}
}

func TestDetectOwner_BarePod(t *testing.T) {
	client := fake.NewSimpleClientset()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "nginx-demo", Namespace: "default"}}

	owner, name, err := DetectOwner(context.Background(), client, "default", pod)
	if err != nil {
		t.Fatalf("DetectOwner() error = %v", err)
	}
	if owner != OwnerNone || name != "" {
		t.Errorf("got (%q, %q), want (OwnerNone, \"\")", owner, name)
	}
}

func TestDetectOwner_DeploymentOwned(t *testing.T) {
	rs := replicaSetOwnedByDeployment("default", "nginx-rs-abc", "nginx-deploy")
	client := fake.NewSimpleClientset(rs)
	pod := deploymentOwnedPod("default", "nginx-deploy-abc-xyz", "nginx-rs-abc", nil)

	owner, name, err := DetectOwner(context.Background(), client, "default", pod)
	if err != nil {
		t.Fatalf("DetectOwner() error = %v", err)
	}
	if owner != OwnerDeployment || name != "nginx-deploy" {
		t.Errorf("got (%q, %q), want (OwnerDeployment, nginx-deploy)", owner, name)
	}
}

func TestDetectOwner_UnsupportedOwnerKind(t *testing.T) {
	client := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "web-0",
			Namespace:       "default",
			OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: "web"}},
		},
	}

	_, _, err := DetectOwner(context.Background(), client, "default", pod)
	if err == nil {
		t.Fatal("DetectOwner() error = nil, want an error (StatefulSet not supported)")
	}
	if !strings.Contains(err.Error(), "StatefulSet") {
		t.Errorf("err = %q, want it to mention StatefulSet", err)
	}
}

func TestDetectOwner_ReplicaSetWithoutDeploymentOwner(t *testing.T) {
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "orphan-rs", Namespace: "default"}}
	client := fake.NewSimpleClientset(rs)
	pod := deploymentOwnedPod("default", "orphan-rs-xyz", "orphan-rs", nil)

	_, _, err := DetectOwner(context.Background(), client, "default", pod)
	if err == nil {
		t.Fatal("DetectOwner() error = nil, want an error (ReplicaSet has no Deployment owner)")
	}
}

func TestRestart_BarePod_DeletesAndRecreatesWithSameSpec(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-demo",
			Namespace: "default",
			Labels:    map[string]string{"run": "nginx-demo"},
		},
		Spec: corev1.PodSpec{
			NodeName:   "some-node",
			Containers: []corev1.Container{{Name: "nginx", Image: "nginx:alpine"}},
		},
	}
	client := fake.NewSimpleClientset(pod)
	target := &TargetPod{Namespace: "default", PodName: "nginx-demo", Container: "nginx"}

	updated, err := Restart(context.Background(), client, target)
	if err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if updated.PodName != "nginx-demo" {
		t.Errorf("PodName = %q, want unchanged nginx-demo (a bare pod keeps its name)", updated.PodName)
	}
	if !reflect.DeepEqual(updated.Labels, map[string]string{"run": "nginx-demo"}) {
		t.Errorf("Labels = %v, want {run: nginx-demo}", updated.Labels)
	}

	got, err := client.CoreV1().Pods("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected the pod to have been recreated, Get error = %v", err)
	}
	if got.Spec.NodeName != "" {
		t.Errorf("NodeName = %q, want empty (not carried over to the recreated pod)", got.Spec.NodeName)
	}
	if len(got.Spec.Containers) != 1 || got.Spec.Containers[0].Image != "nginx:alpine" {
		t.Errorf("recreated pod spec = %+v, want the same container spec", got.Spec)
	}
}

// TestRestart_Deployment_TargetsReplacementPod simulates the
// already-replaced state (a fake clientset won't run a real ReplicaSet
// controller in response to the rollout-restart patch) to verify the
// selection logic: given both the old and a new pod matching the
// Deployment's selector, Restart must return the new one.
func TestRestart_Deployment_TargetsReplacementPod(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
		},
	}
	rs := replicaSetOwnedByDeployment("default", "nginx-deploy-abc", "nginx-deploy")
	oldPod := deploymentOwnedPod("default", "nginx-deploy-abc-old", "nginx-deploy-abc", map[string]string{"app": "nginx"})
	newPod := deploymentOwnedPod("default", "nginx-deploy-abc-new", "nginx-deploy-abc", map[string]string{"app": "nginx"})

	client := fake.NewSimpleClientset(deployment, rs, oldPod, newPod)
	target := &TargetPod{Namespace: "default", PodName: "nginx-deploy-abc-old", Container: "nginx"}

	updated, err := Restart(context.Background(), client, target)
	if err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if updated.PodName != "nginx-deploy-abc-new" {
		t.Errorf("PodName = %q, want nginx-deploy-abc-new (the replacement pod already present)", updated.PodName)
	}
}
