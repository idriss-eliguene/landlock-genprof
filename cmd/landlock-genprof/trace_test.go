// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/analysis"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func TestAddPodLockProfileLabel_PodManifest(t *testing.T) {
	in := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: nginx-demo
  namespace: default
spec:
  containers:
    - name: nginx
      image: nginx:alpine
`)

	out, err := addPodLockProfileLabel(in, "nginx-demo")
	if err != nil {
		t.Fatalf("addPodLockProfileLabel() error = %v", err)
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal(out, &obj); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	labels, found, err := unstructured.NestedStringMap(obj, "metadata", "labels")
	if err != nil {
		t.Fatalf("NestedStringMap(metadata.labels) error = %v", err)
	}
	if !found {
		t.Fatal("metadata.labels not found")
	}
	if labels[podLockProfileLabel] != "nginx-demo" {
		t.Fatalf("metadata.labels[%q] = %q, want nginx-demo", podLockProfileLabel, labels[podLockProfileLabel])
	}
}

func TestAddPodLockProfileLabel_DeploymentManifest(t *testing.T) {
	in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-demo
  namespace: default
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:alpine
`)

	out, err := addPodLockProfileLabel(in, "nginx-demo")
	if err != nil {
		t.Fatalf("addPodLockProfileLabel() error = %v", err)
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal(out, &obj); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	labels, found, err := unstructured.NestedStringMap(obj, "spec", "template", "metadata", "labels")
	if err != nil {
		t.Fatalf("NestedStringMap(spec.template.metadata.labels) error = %v", err)
	}
	if !found {
		t.Fatal("spec.template.metadata.labels not found")
	}
	if labels[podLockProfileLabel] != "nginx-demo" {
		t.Fatalf("template labels[%q] = %q, want nginx-demo", podLockProfileLabel, labels[podLockProfileLabel])
	}
}

func TestPrintSecurityRecommendationSummary(t *testing.T) {
	rec := analysis.SecurityRecommendation{
		Workload: analysis.WorkloadRef{Namespace: "default", Pod: "payment-api", Container: "app"},
		TrainingRuns:      14,
		OverallConfidence: 94,
		Domains: []analysis.DomainRecommendation{
			{Domain: "filesystem", RequiredCount: 23, Backend: "podlock", Available: true},
			{Domain: "network", RequiredCount: 4, Backend: "networkpolicy", Available: true},
		},
	}

	var out bytes.Buffer
	printSecurityRecommendationSummary(&out, rec)
	got := out.String()

	for _, want := range []string{
		"WORKLOAD SECURITY ANALYSIS",
		"Workload: default/payment-api",
		"Training runs: 14",
		"filesystem: 23 item(s) -> podlock",
		"Overall confidence: 94%",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q, got: %s", want, got)
		}
	}
}

func TestPublishProposal_SavesMandatoryProposal(t *testing.T) {
	dynClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	oldFactory := newDynamicClientForProposal
	newDynamicClientForProposal = func() (dynamic.Interface, error) { return dynClient, nil }
	defer func() { newDynamicClientForProposal = oldFactory }()

	target := &k8s.TargetPod{
		Namespace: "default",
		PodName:   "nginx-demo",
		Container: "nginx",
		Labels:    map[string]string{"app": "nginx"},
	}

	behavior := profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{{
			Path:        "/etc/nginx",
			Permissions: []profile.FilePermission{profile.PermissionRead},
			Confidence:  profile.ConfidenceHigh,
			SeenCount:   2,
		}}},
	}

	var stdout bytes.Buffer
	err := publishProposal(
		context.Background(),
		&stdout,
		k8sfake.NewSimpleClientset(),
		target,
		target,
		k8s.OwnerNone,
		traceOptions{binary: "/usr/sbin/nginx", history: true},
		behavior,
		"",
	)
	if err != nil {
		t.Fatalf("publishProposal() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "SecurityProfileProposal published: nginx-demo") {
		t.Fatalf("publishProposal() did not report publication, stdout = %q", stdout.String())
	}

	got, err := proposal.Get(context.Background(), dynClient, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("proposal.Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("proposal.Get() = nil, want stored proposal")
	}
	if got.Container != "nginx" {
		t.Fatalf("proposal.Container = %q, want nginx", got.Container)
	}
	if got.Binary != "/usr/sbin/nginx" {
		t.Fatalf("proposal.Binary = %q, want /usr/sbin/nginx", got.Binary)
	}
	if !got.HistoryUsed {
		t.Fatal("proposal.HistoryUsed = false, want true")
	}
	if got.GeneratedAt == "" {
		t.Fatal("proposal.GeneratedAt = empty, want RFC3339 timestamp")
	}
	if !strings.Contains(got.PodLock, "kind: LandlockProfile") {
		t.Fatalf("proposal.PodLock missing LandlockProfile YAML, got %q", got.PodLock)
	}
	if got.NetworkPolicy != "" {
		t.Fatalf("proposal.NetworkPolicy = %q, want empty (no network accesses)", got.NetworkPolicy)
	}
	if got.PatchedManifest != "" {
		t.Fatalf("proposal.PatchedManifest = %q, want empty (nothing to compose)", got.PatchedManifest)
	}
	if got.SPOSeccompProfile != "" {
		t.Fatalf("proposal.SPOSeccompProfile = %q, want empty (no syscalls)", got.SPOSeccompProfile)
	}
}
