// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

// securityProfileProposalListGVR mirrors internal/proposal's own private
// securityProfileProposalGVR — duplicated here (not imported, it's
// unexported) since this package's tests need it for the same reason
// internal/proposal/store_test.go's own newFakeClientForList does: the
// fake dynamic client's List() panics without an explicit GVR -> List-
// Kind hint for a CRD with no generated Go type in the scheme.
var securityProfileProposalListGVR = schema.GroupVersionResource{
	Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals",
}

func newFakeClientForPolicyList() dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{securityProfileProposalListGVR: "SecurityProfileProposalList"},
	)
}

func withPolicyTestClient(t *testing.T, client dynamic.Interface) {
	t.Helper()
	old := newDynamicClientForPolicy
	newDynamicClientForPolicy = func() (dynamic.Interface, error) { return client, nil }
	t.Cleanup(func() { newDynamicClientForPolicy = old })
}

func TestRunPolicyList_Empty(t *testing.T) {
	client := newFakeClientForPolicyList()
	withPolicyTestClient(t, client)

	var buf bytes.Buffer
	if err := runPolicyList(context.Background(), &buf, policyListOptions{namespace: "default"}); err != nil {
		t.Fatalf("runPolicyList() error = %v", err)
	}
	if !strings.Contains(buf.String(), "No SecurityProfileProposals") {
		t.Errorf("output missing empty-namespace message:\n%s", buf.String())
	}
}

func TestRunPolicyList_ShowsApprovalState(t *testing.T) {
	ctx := context.Background()
	client := newFakeClientForPolicyList()
	withPolicyTestClient(t, client)

	if err := proposal.Save(ctx, client, "default", "nginx-demo", proposal.Spec{Container: "nginx"}); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	computed, err := proposal.CandidateDigest(proposal.Spec{Container: "nginx"})
	if err != nil {
		t.Fatalf("CandidateDigest error: %v", err)
	}
	if err := proposal.SetApprovalState(ctx, client, "default", "nginx-demo", proposal.ApprovalApproved, "looks good", computed); err != nil {
		t.Fatalf("proposal.SetApprovalState() error = %v", err)
	}

	var buf bytes.Buffer
	if err := runPolicyList(ctx, &buf, policyListOptions{namespace: "default"}); err != nil {
		t.Fatalf("runPolicyList() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "nginx-demo") || !strings.Contains(out, string(proposal.ApprovalApproved)) {
		t.Errorf("output missing name/state:\n%s", out)
	}
}

func TestRunPolicyStatus_Approved_ExitsZero(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	withPolicyTestClient(t, client)

	if err := proposal.Save(ctx, client, "default", "nginx-demo", proposal.Spec{Container: "nginx"}); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	computed, err := proposal.CandidateDigest(proposal.Spec{Container: "nginx"})
	if err != nil {
		t.Fatalf("CandidateDigest error: %v", err)
	}
	if err := proposal.SetApprovalState(ctx, client, "default", "nginx-demo", proposal.ApprovalApproved, "", computed); err != nil {
		t.Fatalf("proposal.SetApprovalState() error = %v", err)
	}

	var buf bytes.Buffer
	err = runPolicyStatus(ctx, &buf, policyStatusOptions{namespace: "default"}, "nginx-demo")
	if err != nil {
		t.Fatalf("runPolicyStatus() error = %v, want nil (Approved)", err)
	}
	if !strings.Contains(buf.String(), string(proposal.ApprovalApproved)) {
		t.Errorf("output missing approval state:\n%s", buf.String())
	}
}

func TestRunPolicyStatus_Draft_ExitsBlocking(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	withPolicyTestClient(t, client)

	if err := proposal.Save(ctx, client, "default", "nginx-demo", proposal.Spec{Container: "nginx"}); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}

	var buf bytes.Buffer
	err := runPolicyStatus(ctx, &buf, policyStatusOptions{namespace: "default"}, "nginx-demo")

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runPolicyStatus() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("ExitCode() = %d, want 2 (blocking: not yet Approved)", exitErr.ExitCode())
	}
}

func TestRunPolicyStatus_NotFound(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	withPolicyTestClient(t, client)

	var buf bytes.Buffer
	err := runPolicyStatus(context.Background(), &buf, policyStatusOptions{namespace: "default"}, "nginx-demo")
	if err == nil {
		t.Fatal("runPolicyStatus() error = nil, want an error for a nonexistent proposal")
	}
	var exitErr *exitCodeError
	if errors.As(err, &exitErr) {
		t.Error("a not-found proposal should be a plain usage error, not an exitCodeError")
	}
}

func TestRunPolicyStatus_Rejected_ShowsReason(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	withPolicyTestClient(t, client)

	if err := proposal.Save(ctx, client, "default", "nginx-demo", proposal.Spec{Container: "nginx"}); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	if err := proposal.SetApprovalState(ctx, client, "default", "nginx-demo", proposal.ApprovalRejected, "syscalls too broad", ""); err != nil {
		t.Fatalf("proposal.SetApprovalState() error = %v", err)
	}

	var buf bytes.Buffer
	_ = runPolicyStatus(ctx, &buf, policyStatusOptions{namespace: "default"}, "nginx-demo")
	if !strings.Contains(buf.String(), "syscalls too broad") {
		t.Errorf("output missing rejection reason:\n%s", buf.String())
	}
}
