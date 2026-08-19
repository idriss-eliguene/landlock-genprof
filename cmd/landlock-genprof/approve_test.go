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

func withApprovalTestClient(t *testing.T, client dynamic.Interface) {
	t.Helper()
	old := newDynamicClientForApproval
	newDynamicClientForApproval = func() (dynamic.Interface, error) { return client, nil }
	t.Cleanup(func() { newDynamicClientForApproval = old })
}

func TestRunSetApprovalState_Approve(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	spec := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	withApprovalTestClient(t, client)

	var stdout bytes.Buffer
	// Compute the expected digest the reviewer would see and assert to
	// protect against stale-reviewer misbinding.
	digest, err := proposal.CandidateDigest(spec)
	if err != nil {
		t.Fatalf("CandidateDigest error: %v", err)
	}
	opts := approveRejectOptions{namespace: "default", reason: "looks right", expectedDigest: digest}
	if err := runSetApprovalState(context.Background(), &stdout, opts, "nginx-demo", proposal.ApprovalApproved); err != nil {
		t.Fatalf("runSetApprovalState() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "Approved") {
		t.Errorf("stdout = %q, want it to mention Approved", stdout.String())
	}
	if !strings.Contains(stdout.String(), "looks right") {
		t.Errorf("stdout = %q, want it to echo the reason", stdout.String())
	}

	status, err := proposal.GetStatus(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.ApprovalState != proposal.ApprovalApproved {
		t.Errorf("ApprovalState = %q, want %q", status.ApprovalState, proposal.ApprovalApproved)
	}
	if status.Reason != "looks right" {
		t.Errorf("Reason = %q, want %q", status.Reason, "looks right")
	}
}

func TestRunSetApprovalState_Reject(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", proposal.Spec{
		Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z",
	}); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	withApprovalTestClient(t, client)

	var stdout bytes.Buffer
	opts := approveRejectOptions{namespace: "default"}
	if err := runSetApprovalState(context.Background(), &stdout, opts, "nginx-demo", proposal.ApprovalRejected); err != nil {
		t.Fatalf("runSetApprovalState() error = %v", err)
	}

	status, err := proposal.GetStatus(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.ApprovalState != proposal.ApprovalRejected {
		t.Errorf("ApprovalState = %q, want %q", status.ApprovalState, proposal.ApprovalRejected)
	}
}

func TestRunSetApprovalState_NotFound(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	withApprovalTestClient(t, client)

	var stdout bytes.Buffer
	opts := approveRejectOptions{namespace: "default"}
	err := runSetApprovalState(context.Background(), &stdout, opts, "does-not-exist", proposal.ApprovalApproved)
	if err == nil {
		t.Fatal("runSetApprovalState() on a nonexistent proposal: error = nil, want an error")
	}
}
