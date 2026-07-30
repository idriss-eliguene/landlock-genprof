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

// TestRunApplyProposal_ContinuesPastAFailedArtifact is the exact
// real-world scenario this project's own kind reference cluster hits:
// PodLock isn't installed (no matching CRD), but NetworkPolicy is a
// builtin that should still apply. PodLock is first in
// proposalArtifacts' order, so this also checks that a failure isn't
// silently swallowed by continuing — it must still surface as an error.
func TestRunApplyProposal_ContinuesPastAFailedArtifact(t *testing.T) {
	const unrecognizedPodLockYAML = `apiVersion: podlock.example.invalid/v1
kind: NotARealCRD
metadata:
  name: nginx-demo
`
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:     "nginx",
		Binary:        "/usr/sbin/nginx",
		PodLock:       unrecognizedPodLockYAML,
		NetworkPolicy: testNetworkPolicyYAML,
	})

	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default", yes: true}
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want an error since PodLock failed to apply")
	}

	out := stdout.String()
	if !strings.Contains(out, "failed: PodLock") {
		t.Errorf("stdout = %q, want it to report PodLock failing", out)
	}
	if !strings.Contains(out, "applied: NetworkPolicy") {
		t.Errorf("stdout = %q, want NetworkPolicy to still have been applied despite PodLock failing first", out)
	}

	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err != nil {
		t.Errorf("NetworkPolicy was not actually applied despite PodLock failing first: %v", err)
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

func TestRunApplyProposal_PrintsFullReviewSummaryBeforePrompt(t *testing.T) {
	setUpApplyProposalTestClient(t, proposal.Spec{
		Container:     "nginx",
		Binary:        "/usr/sbin/nginx",
		GeneratedAt:   "2026-07-30T10:00:00Z",
		HistoryUsed:   true,
		NetworkPolicy: testNetworkPolicyYAML,
	})

	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default", yes: true}
	if err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"WORKLOAD SECURITY REVIEW",
		"Container: nginx",
		"Binary: /usr/sbin/nginx",
		"History used: true",
		"Artifacts available: 1/4",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRunApplyProposal_SkipExcludesArtifact(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:       "nginx",
		Binary:          "/usr/sbin/nginx",
		NetworkPolicy:   testNetworkPolicyYAML,
		PatchedManifest: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: nginx-demo\n  namespace: default\n",
	})

	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default", yes: true, skip: []string{"patched-manifest"}}
	if err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Skipping 1 artifact(s), per --skip:\n  - Patched Manifest") {
		t.Errorf("stdout = %q, want it to report skipping Patched Manifest", out)
	}
	if !strings.Contains(out, "applied: NetworkPolicy") {
		t.Errorf("stdout = %q, want NetworkPolicy to still be applied", out)
	}
	if strings.Contains(out, "applied: Patched Manifest") {
		t.Errorf("stdout = %q, Patched Manifest should not have been applied", out)
	}

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err == nil {
		t.Error("Patched Manifest's Pod was applied despite --skip=patched-manifest")
	}
}

func TestRunApplyProposal_SkipCommaSeparated(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:     "nginx",
		Binary:        "/usr/sbin/nginx",
		PodLock:       "apiVersion: podlock.kubewarden.io/v1alpha1\nkind: LandlockProfile\nmetadata:\n  name: nginx-demo\n",
		NetworkPolicy: testNetworkPolicyYAML,
	})

	var stdout bytes.Buffer
	// cobra's StringSliceVar splits "--skip=a,b" into ["a", "b"] at flag
	// parse time; runApplyProposal is called directly here, bypassing
	// that, so the slice must already be pre-split the same way.
	opts := applyProposalOptions{namespace: "default", yes: true, skip: []string{"podlock", "networkpolicy"}}
	if err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "Nothing left to apply") {
		t.Errorf("stdout = %q, want the nothing-left-to-apply message", stdout.String())
	}

	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err == nil {
		t.Error("NetworkPolicy was applied despite being in a comma-separated --skip")
	}
}

func TestRunApplyProposal_UnknownSkipValueIsRejected(t *testing.T) {
	// No cluster client set up on purpose: an invalid --skip must fail
	// before ever connecting, not after fetching the proposal.
	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default", yes: true, skip: []string{"podlok"}}
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want an error for an unknown --skip value")
	}
	if !strings.Contains(err.Error(), `--skip="podlok"`) {
		t.Errorf("error = %q, want it to quote the bad value", err.Error())
	}
}
