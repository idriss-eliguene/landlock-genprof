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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

const testNetworkPolicyYAML = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nginx-demo
  namespace: default
`

func setUpApplyProposalTestClient(t *testing.T, spec proposal.Spec) dynamic.Interface {
	t.Helper()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}

	old := newDynamicClientForApplyProposal
	newDynamicClientForApplyProposal = func() (dynamic.Interface, error) { return client, nil }
	t.Cleanup(func() { newDynamicClientForApplyProposal = old })

	return client
}

func TestRunApplyProposal_ConfirmedApplies(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:     "nginx",
		Binary:        "/usr/sbin/nginx",
		NetworkPolicy: testNetworkPolicyYAML,
	})

	var stdout bytes.Buffer
	stdin := strings.NewReader("y\n")
	opts := applyProposalOptions{namespace: "default"}
	if err := runApplyProposal(context.Background(), &stdout, stdin, opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "applied: NetworkPolicy") {
		t.Errorf("stdout = %q, want it to report applying NetworkPolicy", out)
	}
	if !strings.Contains(out, "Done.") {
		t.Errorf("stdout = %q, want a final Done. line", out)
	}

	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err != nil {
		t.Errorf("NetworkPolicy was not actually applied to the cluster: %v", err)
	}
}

func TestRunApplyProposal_DeclinedDoesNotApply(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:     "nginx",
		Binary:        "/usr/sbin/nginx",
		NetworkPolicy: testNetworkPolicyYAML,
	})

	var stdout bytes.Buffer
	stdin := strings.NewReader("n\n")
	opts := applyProposalOptions{namespace: "default"}
	if err := runApplyProposal(context.Background(), &stdout, stdin, opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "Aborted") {
		t.Errorf("stdout = %q, want an Aborted message", stdout.String())
	}

	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err == nil {
		t.Error("NetworkPolicy was applied despite declining the prompt")
	}
}

func TestRunApplyProposal_YesFlagSkipsPrompt(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:     "nginx",
		Binary:        "/usr/sbin/nginx",
		NetworkPolicy: testNetworkPolicyYAML,
	})

	var stdout bytes.Buffer
	// Empty stdin: if the prompt were reached, ReadString would return ""
	// and the default no/empty branch would abort — proves --yes really
	// bypasses the prompt entirely rather than happening to read a blank
	// line as an accidental "yes".
	stdin := strings.NewReader("")
	opts := applyProposalOptions{namespace: "default", yes: true}
	if err := runApplyProposal(context.Background(), &stdout, stdin, opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	if strings.Contains(stdout.String(), "Apply these to the cluster?") {
		t.Errorf("stdout = %q, --yes should have skipped the prompt", stdout.String())
	}

	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err != nil {
		t.Errorf("NetworkPolicy was not applied with --yes: %v", err)
	}
}

func TestRunApplyProposal_NotFound(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	old := newDynamicClientForApplyProposal
	newDynamicClientForApplyProposal = func() (dynamic.Interface, error) { return client, nil }
	defer func() { newDynamicClientForApplyProposal = old }()

	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{namespace: "default"}, "missing")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want an error for a nonexistent proposal")
	}
}

func TestRunApplyProposal_NoArtifactsSkipsPrompt(t *testing.T) {
	setUpApplyProposalTestClient(t, proposal.Spec{
		Container: "nginx",
		Binary:    "/usr/sbin/nginx",
		// No artifacts: empty training run.
	})

	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default"}
	if err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "No artifacts to apply") {
		t.Errorf("stdout = %q, want the no-artifacts message", stdout.String())
	}
}
