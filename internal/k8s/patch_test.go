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
	"sigs.k8s.io/yaml"
)

func exampleSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Add:  []corev1.Capability{"SETUID"},
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeLocalhost,
		},
	}
}

// TestPatchedManifest_BarePod checks the merge-not-replace safety
// property directly: an existing securityContext field (RunAsNonRoot)
// must survive alongside the newly-set Capabilities/SeccompProfile, and
// the output must be a clean Pod manifest (no status/resourceVersion),
// identified by the pod's own name.
func TestPatchedManifest_BarePod(t *testing.T) {
	runAsNonRoot := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "nginx-demo",
			Namespace:       "default",
			Labels:          map[string]string{"run": "nginx-demo"},
			ResourceVersion: "12345",
			UID:             "some-uid",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "nginx",
				Image: "nginx:alpine",
				SecurityContext: &corev1.SecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	client := fake.NewSimpleClientset(pod)
	target := &TargetPod{Namespace: "default", PodName: "nginx-demo", Container: "nginx"}

	identity, manifest, err := PatchedManifest(context.Background(), client, target, exampleSecurityContext())
	if err != nil {
		t.Fatalf("PatchedManifest() error = %v", err)
	}
	if identity != "nginx-demo" {
		t.Errorf("identity = %q, want nginx-demo (the pod's own name)", identity)
	}

	var got corev1.Pod
	if err := yaml.Unmarshal(manifest, &got); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if got.APIVersion != "v1" || got.Kind != "Pod" {
		t.Errorf("TypeMeta = {%q %q}, want {v1 Pod}", got.APIVersion, got.Kind)
	}
	if got.ResourceVersion != "" || got.UID != "" {
		t.Errorf("expected server-populated metadata stripped, got ResourceVersion=%q UID=%q", got.ResourceVersion, got.UID)
	}

	sc := got.Spec.Containers[0].SecurityContext
	if sc == nil {
		t.Fatalf("Containers[0].SecurityContext = nil, want set")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Errorf("RunAsNonRoot = %v, want true (existing field must survive the merge)", sc.RunAsNonRoot)
	}
	if sc.Capabilities == nil || sc.Capabilities.Add[0] != "SETUID" {
		t.Errorf("Capabilities = %+v, want the generated ones set", sc.Capabilities)
	}
	if sc.SeccompProfile == nil {
		t.Errorf("SeccompProfile = nil, want set")
	}

	if strings.Contains(string(manifest), "status:") {
		t.Errorf("manifest contains a status: section, want it stripped:\n%s", manifest)
	}
}

// TestPatchedManifest_DeploymentOwned checks that an owned pod produces
// the *owner's* manifest, not the pod's own — the whole point being that
// applying it triggers a rollout, since the pod's own securityContext
// can't be changed on a live pod directly.
func TestPatchedManifest_DeploymentOwned(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx-deploy", Namespace: "default", ResourceVersion: "999"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx:alpine"}},
				},
			},
		},
	}
	rs := replicaSetOwnedByDeployment("default", "nginx-deploy-abc", "nginx-deploy")
	pod := deploymentOwnedPod("default", "nginx-deploy-abc-old", "nginx-deploy-abc", map[string]string{"app": "nginx"})

	client := fake.NewSimpleClientset(deployment, rs, pod)
	target := &TargetPod{Namespace: "default", PodName: "nginx-deploy-abc-old", Container: "nginx"}

	identity, manifest, err := PatchedManifest(context.Background(), client, target, exampleSecurityContext())
	if err != nil {
		t.Fatalf("PatchedManifest() error = %v", err)
	}
	if identity != "nginx-deploy" {
		t.Errorf("identity = %q, want nginx-deploy (the owner's name, not the ephemeral pod's)", identity)
	}

	var got appsv1.Deployment
	if err := yaml.Unmarshal(manifest, &got); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if got.APIVersion != "apps/v1" || got.Kind != "Deployment" {
		t.Errorf("TypeMeta = {%q %q}, want {apps/v1 Deployment}", got.APIVersion, got.Kind)
	}
	if got.ResourceVersion != "" {
		t.Errorf("expected ResourceVersion stripped, got %q", got.ResourceVersion)
	}
	sc := got.Spec.Template.Spec.Containers[0].SecurityContext
	if sc == nil || sc.Capabilities == nil {
		t.Fatalf("template container SecurityContext = %+v, want Capabilities set", sc)
	}
}

// TestPatchedManifest_StatefulSetOwned and
// TestPatchedManifest_DaemonSetOwned mirror the Deployment case for the
// two other owner kinds — same container-patch path via the shared
// PodTemplateSpec shape.
func TestPatchedManifest_StatefulSetOwned(t *testing.T) {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}}},
			},
		},
	}
	pod := podOwnedBy("default", "web-0", "StatefulSet", "web", map[string]string{"app": "web"})
	client := fake.NewSimpleClientset(statefulSet, pod)
	target := &TargetPod{Namespace: "default", PodName: "web-0", Container: "web"}

	identity, manifest, err := PatchedManifest(context.Background(), client, target, exampleSecurityContext())
	if err != nil {
		t.Fatalf("PatchedManifest() error = %v", err)
	}
	if identity != "web" {
		t.Errorf("identity = %q, want web", identity)
	}

	var got appsv1.StatefulSet
	if err := yaml.Unmarshal(manifest, &got); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if got.APIVersion != "apps/v1" || got.Kind != "StatefulSet" {
		t.Errorf("TypeMeta = {%q %q}, want {apps/v1 StatefulSet}", got.APIVersion, got.Kind)
	}
	if got.Spec.Template.Spec.Containers[0].SecurityContext == nil {
		t.Errorf("template container SecurityContext not set")
	}
}

func TestPatchedManifest_DaemonSetOwned(t *testing.T) {
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "fluentd", Namespace: "default"},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "fluentd"}}},
			},
		},
	}
	pod := podOwnedBy("default", "fluentd-old", "DaemonSet", "fluentd", map[string]string{"app": "fluentd"})
	client := fake.NewSimpleClientset(daemonSet, pod)
	target := &TargetPod{Namespace: "default", PodName: "fluentd-old", Container: "fluentd"}

	identity, manifest, err := PatchedManifest(context.Background(), client, target, exampleSecurityContext())
	if err != nil {
		t.Fatalf("PatchedManifest() error = %v", err)
	}
	if identity != "fluentd" {
		t.Errorf("identity = %q, want fluentd", identity)
	}

	var got appsv1.DaemonSet
	if err := yaml.Unmarshal(manifest, &got); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if got.APIVersion != "apps/v1" || got.Kind != "DaemonSet" {
		t.Errorf("TypeMeta = {%q %q}, want {apps/v1 DaemonSet}", got.APIVersion, got.Kind)
	}
	if got.Spec.Template.Spec.Containers[0].SecurityContext == nil {
		t.Errorf("template container SecurityContext not set")
	}
}

// TestPatchedManifest_ContainerNotFound checks that a container name
// mismatch is a real error, not a silent no-op.
func TestPatchedManifest_ContainerNotFound(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx-demo", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx"}}},
	}
	client := fake.NewSimpleClientset(pod)
	target := &TargetPod{Namespace: "default", PodName: "nginx-demo", Container: "does-not-exist"}

	_, _, err := PatchedManifest(context.Background(), client, target, exampleSecurityContext())
	if err == nil {
		t.Fatal("PatchedManifest() error = nil, want an error (container not found)")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("err = %q, want it to mention the missing container name", err)
	}
}
