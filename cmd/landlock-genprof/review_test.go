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
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func TestRunReview_PrintsProposalSummary(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", proposal.Spec{
		Container:   "nginx",
		Binary:      "/usr/sbin/nginx",
		GeneratedAt: "2026-07-24T10:00:00Z",
		HistoryUsed: true,
		PodLock: `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
`,
		NetworkPolicy: `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
`,
		PatchedManifest: `apiVersion: v1
kind: Pod
metadata:
  name: nginx-demo
  labels:
    podlock.kubewarden.io/profile: nginx-demo
`,
	}); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}

	old := newDynamicClientForReview
	newDynamicClientForReview = func() (dynamic.Interface, error) { return client, nil }
	defer func() { newDynamicClientForReview = old }()

	var stdout bytes.Buffer
	if err := runReview(context.Background(), &stdout, reviewOptions{namespace: "default"}, "nginx-demo"); err != nil {
		t.Fatalf("runReview() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"WORKLOAD SECURITY REVIEW",
		"Proposal: default/nginx-demo",
		"Container: nginx",
		"Binary: /usr/sbin/nginx",
		"Artifacts available: 3/4",
		"- PodLock: available",
		"- NetworkPolicy: available",
		"- Patched Manifest: available",
		"- SPO SeccompProfile: not generated",
		"Patched manifest PodLock label: present",
		"make export-proposal PROPOSAL=nginx-demo NS=default",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("review output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRunReview_NotFound(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	old := newDynamicClientForReview
	newDynamicClientForReview = func() (dynamic.Interface, error) { return client, nil }
	defer func() { newDynamicClientForReview = old }()

	err := runReview(context.Background(), &bytes.Buffer{}, reviewOptions{namespace: "default"}, "missing")
	if err == nil {
		t.Fatal("runReview() error = nil, want not found error")
	}
	if !strings.Contains(err.Error(), "securityprofileproposal default/missing not found") {
		t.Fatalf("runReview() error = %v, want not found message", err)
	}
}