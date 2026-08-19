// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Proof obligations for
// docs/adr/0007-governed-apply-ordering-and-enforcement-readiness.md.
//
// Most of these prove a negative — that the workload binding did NOT
// happen — because that is the safety property the ADR exists for. A
// test that only proved the happy path would pass just as well against
// the defect this contract fixes.

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
)

// The patched manifest binds the tools container to the profile the
// SeccompProfile artifact below materializes — the same
// operator/<ns>/<name>.json convention internal/exporter/spo generates.
// The governed profile is cluster-scoped and its name is derived, so both
// fixtures are built from the adapter rather than hardcoded — hardcoding
// them would silently drift from the naming contract they are meant to
// exercise.
var (
	testGovernedProfileName = spobackend.GovernedProfileName("default", "nginx-demo", "nginx")
	testGovernedProfilePath = spobackend.LocalhostProfilePath(testGovernedProfileName)
)

var testPatchedManifestWithSeccompYAML = `apiVersion: v1
kind: Pod
metadata:
  name: nginx-demo
  namespace: default
spec:
  containers:
    - name: nginx
      image: nginx:1.25
      securityContext:
        seccompProfile:
          type: Localhost
          localhostProfile: ` + testGovernedProfilePath + `
`

var testSeccompProfileYAML = `apiVersion: ` + spobackend.APIVersion + `
kind: SeccompProfile
metadata:
  name: ` + testGovernedProfileName + `
  annotations:
    ` + spobackend.ManagedByAnnotation + `: ` + spobackend.ManagedByValue + `
    ` + spobackend.NameSchemeAnnotation + `: "` + spobackend.NameScheme + `"
    ` + spobackend.TargetNamespaceAnnotation + `: default
    ` + spobackend.TargetPodAnnotation + `: nginx-demo
    ` + spobackend.TargetContainerAnnotation + `: nginx
spec:
  defaultAction: SCMP_ACT_ERRNO
  architectures:
    - SCMP_ARCH_X86_64
  syscalls:
    - names:
        - openat
        - read
      action: SCMP_ACT_ALLOW
`

var testPodGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

// specWithSeccompBinding is the full four-artifact shape the readiness
// gate is meant to govern.
func specWithSeccompBinding() proposal.Spec {
	return proposal.Spec{
		Container:         "nginx",
		Binary:            "/usr/sbin/nginx",
		NetworkPolicy:     testNetworkPolicyYAML,
		PatchedManifest:   testPatchedManifestWithSeccompYAML,
		SPOSeccompProfile: testSeccompProfileYAML,
	}
}

func fastReadinessPolling(t *testing.T) {
	t.Helper()
	old := readinessPollInterval
	readinessPollInterval = time.Millisecond
	t.Cleanup(func() { readinessPollInterval = old })
}

// recordApplyOrder replaces the apply seam with one that records the
// order artifacts were submitted in, and optionally fails one of them.
func recordApplyOrder(t *testing.T, failNamed string) *[]string {
	t.Helper()
	var applied []string
	old := applyManifest
	applyManifest = func(ctx context.Context, c dynamic.Interface, namespace, content string) error {
		name := artifactNameFromContent(content)
		if failNamed != "" && name == failNamed {
			return context.DeadlineExceeded
		}
		applied = append(applied, name)
		return k8s.Apply(ctx, c, namespace, content)
	}
	t.Cleanup(func() { applyManifest = old })
	return &applied
}

// artifactNameFromContent identifies an artifact by its Kind, which is
// enough to assert ordering without coupling the test to display names.
func artifactNameFromContent(content string) string {
	switch {
	case strings.Contains(content, "kind: SeccompProfile"):
		return "SeccompProfile"
	case strings.Contains(content, "kind: NetworkPolicy"):
		return "NetworkPolicy"
	case strings.Contains(content, "kind: Pod"):
		return "Pod"
	default:
		return "unknown"
	}
}

func podExists(t *testing.T, client dynamic.Interface) bool {
	t.Helper()
	_, err := client.Resource(testPodGVR).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err == nil {
		return true
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("unexpected error checking for the bound Pod: %v", err)
	}
	return false
}

func runApplyWithBinding(t *testing.T, timeout time.Duration) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{
		namespace:        "default",
		yes:              true,
		restart:          true,
		readinessTimeout: timeout,
	}, "nginx-demo")
	return stdout.String(), err
}

// 1 — the enforcement artifact is applied before the workload binding.
// This is the ordering defect the ADR exists for: the shipped order put
// the Patched Manifest before the SeccompProfile it references.
func TestApplyReadiness_SeccompProfilePrecedesWorkloadBinding(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "")

	// Reconcile as soon as the profile lands, the way SPO would.
	oldHook := afterApplyProposalPlanBuilt
	t.Cleanup(func() { afterApplyProposalPlanBuilt = oldHook })
	afterApplyProposalPlanBuilt = nil

	startFakeSPOReconciler(t, client, testGovernedProfilePath, nil)

	if _, err := runApplyWithBinding(t, 10*time.Second); err != nil {
		t.Fatalf("runApplyProposal() error = %v, want success", err)
	}

	order := *applied
	seccompIdx, podIdx := -1, -1
	for i, name := range order {
		switch name {
		case "SeccompProfile":
			seccompIdx = i
		case "Pod":
			podIdx = i
		}
	}
	if seccompIdx == -1 || podIdx == -1 {
		t.Fatalf("applied = %v, want both SeccompProfile and Pod", order)
	}
	if seccompIdx > podIdx {
		t.Errorf("applied = %v, want SeccompProfile before Pod", order)
	}
}

// 2 and 3 — the binding waits for readiness, and when readiness never
// arrives the workload is not bound.
func TestApplyReadiness_TimeoutDoesNotBindWorkload(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "")

	// No reconciler: status.localhostProfile is never populated.
	stdout, err := runApplyWithBinding(t, 30*time.Millisecond)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want a readiness timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, want a readiness timeout", err.Error())
	}
	if !strings.Contains(err.Error(), "workload binding not applied") {
		t.Errorf("error = %q, want it to say the binding was not applied", err.Error())
	}
	assertExitCode(t, err, 2)

	for _, name := range *applied {
		if name == "Pod" {
			t.Fatalf("applied = %v, want no Pod after a readiness timeout", *applied)
		}
	}
	if podExists(t, client) {
		t.Error("the workload was bound despite the readiness timeout")
	}
	if !strings.Contains(stdout, "waiting for SPO to reconcile") {
		t.Errorf("stdout = %q, want the wait to be visible to the operator", stdout)
	}
}

// 4 — an enforcement artifact that fails to apply stops the sequence
// before the binding (INV-APPLY-ORDER-05).
func TestApplyReadiness_EnforcementApplyFailureDoesNotBindWorkload(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "SeccompProfile")

	_, err := runApplyWithBinding(t, 30*time.Millisecond)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want the SeccompProfile failure to stop the sequence")
	}
	if !strings.Contains(err.Error(), "stopping before any further artifact") {
		t.Errorf("error = %q, want fail-stop wording", err.Error())
	}
	for _, name := range *applied {
		if name == "Pod" {
			t.Fatalf("applied = %v, want no Pod after an enforcement failure", *applied)
		}
	}
	if podExists(t, client) {
		t.Error("the workload was bound despite an enforcement artifact failing")
	}
}

// 5 — the backend materialized the profile somewhere other than where
// the workload will look for it. Waiting cannot fix that, so it is fatal
// immediately rather than at the timeout.
func TestApplyReadiness_PathMismatchDoesNotBindWorkload(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "")

	startFakeSPOReconciler(t, client, "operator/somewhere-else.json", nil)

	_, err := runApplyWithBinding(t, 5*time.Second)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want a path mismatch rejection")
	}
	if !strings.Contains(err.Error(), "materialized at") {
		t.Errorf("error = %q, want it to name the mismatched path", err.Error())
	}
	assertExitCode(t, err, 2)
	for _, name := range *applied {
		if name == "Pod" {
			t.Fatalf("applied = %v, want no Pod after a path mismatch", *applied)
		}
	}
	if podExists(t, client) {
		t.Error("the workload was bound despite a profile path mismatch")
	}
}

// 6 — identity drift. The approved profile was applied, then another
// actor mutated the live object, then the backend reported ready. Ready
// for the wrong content is not ready (INV-APPLY-ORDER-02).
func TestApplyReadiness_IdentityDriftDoesNotBindWorkload(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "")

	// Widen the live profile behind our back, then report it ready.
	startFakeSPOReconciler(t, client, testGovernedProfilePath, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, "SCMP_ACT_ALLOW", "spec", "defaultAction")
	})

	_, err := runApplyWithBinding(t, 5*time.Second)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want an identity-drift rejection")
	}
	if !strings.Contains(err.Error(), "no longer carries the approved enforcement content") {
		t.Errorf("error = %q, want an approved-content mismatch", err.Error())
	}
	assertExitCode(t, err, 2)
	for _, name := range *applied {
		if name == "Pod" {
			t.Fatalf("applied = %v, want no Pod after identity drift", *applied)
		}
	}
	if podExists(t, client) {
		t.Error("the workload was bound to a profile that is not the approved one")
	}
}

// 7 — Gate 3. Approval is revoked during the readiness wait; a wait must
// not launder a revoked approval into permission to recreate the
// workload (INV-APPLY-ORDER-08).
func TestApplyReadiness_ApprovalRevokedDuringWaitDoesNotBindWorkload(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "")

	startFakeSPOReconciler(t, client, testGovernedProfilePath, nil)

	oldHook := afterEnforcementReady
	t.Cleanup(func() { afterEnforcementReady = oldHook })
	afterEnforcementReady = func() {
		if err := proposal.SetApprovalState(context.Background(), client, "default", "nginx-demo",
			proposal.ApprovalRejected, "revoked while waiting for SPO", ""); err != nil {
			t.Errorf("SetApprovalState(revoke) error = %v", err)
		}
	}

	_, err := runApplyWithBinding(t, 5*time.Second)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want Gate 3 to reject a revoked approval")
	}
	if !strings.Contains(err.Error(), "authorization changed before workload binding") {
		t.Errorf("error = %q, want a Gate 3 authorization rejection", err.Error())
	}
	for _, name := range *applied {
		if name == "Pod" {
			t.Fatalf("applied = %v, want no Pod after the approval was revoked", *applied)
		}
	}
	if podExists(t, client) {
		t.Error("the workload was bound on a revoked approval")
	}
}

// 8 — Gate 3, candidate mutation. The candidate changed during the wait,
// so the plan no longer corresponds to what is approved.
func TestApplyReadiness_CandidateChangedDuringWaitDoesNotBindWorkload(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "")

	startFakeSPOReconciler(t, client, testGovernedProfilePath, nil)

	oldHook := afterEnforcementReady
	t.Cleanup(func() { afterEnforcementReady = oldHook })
	afterEnforcementReady = func() {
		changed := specWithSeccompBinding()
		changed.Binary = "/usr/sbin/nginx-next"
		if err := proposal.Save(context.Background(), client, "default", "nginx-demo", changed); err != nil {
			t.Errorf("proposal.Save() error = %v", err)
		}
	}

	_, err := runApplyWithBinding(t, 5*time.Second)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want a candidate-change rejection")
	}
	if !strings.Contains(err.Error(), "before workload binding") &&
		!strings.Contains(err.Error(), "during enforcement readiness wait") {
		t.Errorf("error = %q, want a Gate 3 candidate/authority rejection", err.Error())
	}
	for _, name := range *applied {
		if name == "Pod" {
			t.Fatalf("applied = %v, want no Pod after the candidate changed", *applied)
		}
	}
	if podExists(t, client) {
		t.Error("the workload was bound after the candidate changed during the wait")
	}
}

// 9 — a live-policy artifact failing also stops the sequence.
func TestApplyReadiness_NetworkPolicyFailureDoesNotBindWorkload(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "NetworkPolicy")

	_, err := runApplyWithBinding(t, 30*time.Millisecond)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want the NetworkPolicy failure to stop the sequence")
	}
	for _, name := range *applied {
		if name == "Pod" {
			t.Fatalf("applied = %v, want no Pod after a NetworkPolicy failure", *applied)
		}
	}
	if podExists(t, client) {
		t.Error("the workload was bound despite a NetworkPolicy failure")
	}
}

// 10 — retrying after a timeout re-runs every gate and succeeds once the
// backend is healthy, without manual cleanup (INV-APPLY-ORDER-06).
func TestApplyReadiness_RetryAfterTimeoutSucceedsOnceReady(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	recordApplyOrder(t, "")

	if _, err := runApplyWithBinding(t, 20*time.Millisecond); err == nil {
		t.Fatal("first run: error = nil, want a readiness timeout")
	}
	if podExists(t, client) {
		t.Fatal("first run bound the workload despite timing out")
	}

	// The backend catches up; the operator retries.
	startFakeSPOReconciler(t, client, testGovernedProfilePath, nil)

	if _, err := runApplyWithBinding(t, 5*time.Second); err != nil {
		t.Fatalf("retry: error = %v, want success once the profile is ready", err)
	}
	if !podExists(t, client) {
		t.Error("retry did not bind the workload even though the profile was ready")
	}
}

// 11 — the standalone path. No seccomp artifact means nothing to wait
// for, and behavior is the pre-existing one.
func TestApplyReadiness_NoSeccompArtifactSkipsGateEntirely(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:     "nginx",
		Binary:        "/usr/sbin/nginx",
		NetworkPolicy: testNetworkPolicyYAML,
	})
	applied := recordApplyOrder(t, "")

	stdout, err := runApplyWithBinding(t, time.Millisecond)
	if err != nil {
		t.Fatalf("runApplyProposal() error = %v, want success with no enforcement artifact", err)
	}
	if strings.Contains(stdout, "waiting for SPO") {
		t.Errorf("stdout = %q, want no readiness wait when nothing references a profile", stdout)
	}
	if len(*applied) == 0 {
		t.Error("nothing was applied")
	}
	_ = client
}

// 12 — no silent fallback: a workload bound to a profile this run is not
// applying is still gated on that profile actually being ready, so
// --skip cannot quietly produce an unstartable pod.
func TestApplyReadiness_SkippedSeccompStillGatesBinding(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	applied := recordApplyOrder(t, "")

	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{
		namespace:        "default",
		yes:              true,
		restart:          true,
		skip:             []string{"spo-seccompprofile"},
		readinessTimeout: 20 * time.Millisecond,
	}, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want the binding gated on the referenced profile")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, want a readiness timeout for the referenced profile", err.Error())
	}
	for _, name := range *applied {
		if name == "SeccompProfile" {
			t.Errorf("applied = %v, want the skipped SeccompProfile not to be applied", *applied)
		}
		if name == "Pod" {
			t.Fatalf("applied = %v, want no Pod when the referenced profile is absent", *applied)
		}
	}
	if podExists(t, client) {
		t.Error("the workload was bound to a profile that was never applied")
	}
}

// startFakeSPOReconciler models SPO's daemon: it keeps
// status.localhostProfile populated for as long as the test runs,
// including after an apply replaces the object and clears status.
// mutate, when non-nil, also drifts the live spec — the "another actor
// changed it behind our back" case.
func startFakeSPOReconciler(t *testing.T, client dynamic.Interface, path string, mutate func(*unstructured.Unstructured)) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		resource := client.Resource(spobackend.SeccompProfileGVR())
		for {
			select {
			case <-done:
				return
			default:
			}
			if obj, err := resource.Get(context.Background(), testGovernedProfileName, metav1.GetOptions{}); err == nil {
				installed, _, _ := unstructured.NestedString(obj.Object, "status", "localhostProfile")
				if installed != path {
					if mutate != nil {
						mutate(obj)
					}
					_ = unstructured.SetNestedField(obj.Object, path, "status", "localhostProfile")
					_, _ = resource.Update(context.Background(), obj, metav1.UpdateOptions{})
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()
	t.Cleanup(func() { close(done) })
}

// 13 — INV-APPLY-ORDER-04: readiness is an execution precondition, not
// authority. A readiness wait, and its failure, must leave the approval
// and the candidate digest exactly as they were.
func TestApplyReadiness_ReadinessDoesNotAlterAuthority(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	recordApplyOrder(t, "")

	before, err := proposal.GetStatus(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() before = %v", err)
	}

	if _, err := runApplyWithBinding(t, 20*time.Millisecond); err == nil {
		t.Fatal("runApplyProposal() error = nil, want a readiness timeout")
	}

	after, err := proposal.GetStatus(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() after = %v", err)
	}
	if before.ApprovalState != after.ApprovalState {
		t.Errorf("approvalState = %q after a readiness failure, want it unchanged at %q",
			after.ApprovalState, before.ApprovalState)
	}
	if before.ApprovedCandidateDigest != after.ApprovedCandidateDigest {
		t.Errorf("approvedCandidateDigest changed across a readiness failure: %q -> %q",
			before.ApprovedCandidateDigest, after.ApprovedCandidateDigest)
	}
	if before.ApprovalMechanismVersion != after.ApprovalMechanismVersion {
		t.Errorf("approvalMechanismVersion changed across a readiness failure: %q -> %q",
			before.ApprovalMechanismVersion, after.ApprovalMechanismVersion)
	}

	// And the candidate itself is untouched, so a retry binds to the same
	// approved content rather than a silently re-derived one.
	spec, err := proposal.Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() after = %v", err)
	}
	digest, err := proposal.CandidateDigest(*spec)
	if err != nil {
		t.Fatalf("CandidateDigest() after = %v", err)
	}
	if digest != after.ApprovedCandidateDigest {
		t.Errorf("candidate digest = %q, want it to still match the approved digest %q",
			digest, after.ApprovedCandidateDigest)
	}
}

// injectSeccompGetError makes every readiness Get for seccompprofiles
// fail with err. The tests using it also --skip the SeccompProfile, so
// the only caller reaching this reactor is the readiness gate.
func injectSeccompGetError(t *testing.T, client dynamic.Interface, err error) {
	t.Helper()
	fake, ok := client.(*dynamicfake.FakeDynamicClient)
	if !ok {
		t.Fatalf("client is %T, want *dynamicfake.FakeDynamicClient", client)
	}
	fake.PrependReactor("get", "seccompprofiles", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
}

func runApplySkippingSeccomp(t *testing.T, timeout time.Duration) (time.Duration, error) {
	t.Helper()
	var stdout bytes.Buffer
	start := time.Now()
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{
		namespace:        "default",
		yes:              true,
		restart:          true,
		skip:             []string{"spo-seccompprofile"},
		readinessTimeout: timeout,
	}, "nginx-demo")
	return time.Since(start), err
}

// P2-1 — the SeccompProfile resource type is not served at all (SPO is
// not installed). This must fail immediately, not consume the readiness
// budget: it is exactly the shape the SPO-less Core E2E produces.
//
// The injected error reproduces what a real cluster returns, verified
// live: 404 / reason NotFound / "the server could not find the requested
// resource" / empty Details — indistinguishable from a missing object by
// apierrors.IsNotFound alone.
func TestApplyReadiness_MissingSeccompAPIFailsFast(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	recordApplyOrder(t, "")
	injectSeccompGetError(t, client, apierrors.NewGenericServerResponse(
		404, "get", schema.GroupResource{}, "", "the server could not find the requested resource", 0, false))

	// A budget far larger than the test could tolerate if it were consumed.
	elapsed, err := runApplySkippingSeccomp(t, 30*time.Second)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want an immediate API-unavailable failure")
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, want fail-fast rather than a readiness timeout", err.Error())
	}
	if !strings.Contains(err.Error(), "SeccompProfile API is not available") {
		t.Errorf("error = %q, want it to name the unavailable API", err.Error())
	}
	if !strings.Contains(err.Error(), "security-profiles-operator") {
		t.Errorf("error = %q, want it to point at the missing operator", err.Error())
	}
	assertExitCode(t, err, 2)
	if elapsed > 5*time.Second {
		t.Errorf("took %s, want an immediate failure well inside the 30s budget", elapsed)
	}
	if podExists(t, client) {
		t.Error("the workload was bound despite the SeccompProfile API being unavailable")
	}
}

// P2-1 companion — a missing *object* on an available API is still
// retryable, so the fix above must not have made every 404 fatal.
func TestApplyReadiness_MissingObjectStillRetriesUntilTimeout(t *testing.T) {
	fastReadinessPolling(t)
	client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
	recordApplyOrder(t, "")
	injectSeccompGetError(t, client, apierrors.NewNotFound(
		spobackend.SeccompProfileGVR().GroupResource(), testGovernedProfileName))

	_, err := runApplySkippingSeccomp(t, 30*time.Millisecond)
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want a readiness timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, want a missing object to be retried until timeout", err.Error())
	}
	if !strings.Contains(err.Error(), "does not exist yet") {
		t.Errorf("error = %q, want the last reason to be the absent object", err.Error())
	}
	if podExists(t, client) {
		t.Error("the workload was bound despite the profile never existing")
	}
}

// P2-2 — non-transient API failures fail immediately.
func TestApplyReadiness_NonTransientAPIErrorsFailFast(t *testing.T) {
	gr := spobackend.SeccompProfileGVR().GroupResource()
	cases := []struct {
		name string
		err  error
	}{
		{"Unauthorized", apierrors.NewUnauthorized("no credentials")},
		{"Forbidden", apierrors.NewForbidden(gr, testGovernedProfileName, context.DeadlineExceeded)},
		{"Invalid", apierrors.NewInvalid(schema.GroupKind{Group: gr.Group, Kind: "SeccompProfile"}, testGovernedProfileName, nil)},
		{"BadRequest", apierrors.NewBadRequest("malformed")},
		{"MethodNotSupported", apierrors.NewMethodNotSupported(gr, "get")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastReadinessPolling(t)
			client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
			recordApplyOrder(t, "")
			injectSeccompGetError(t, client, tc.err)

			elapsed, err := runApplySkippingSeccomp(t, 30*time.Second)
			if err == nil {
				t.Fatalf("runApplyProposal() error = nil, want %s to fail fast", tc.name)
			}
			if strings.Contains(err.Error(), "timed out") {
				t.Errorf("error = %q, want %s to fail fast rather than time out", err.Error(), tc.name)
			}
			assertExitCode(t, err, 2)
			if elapsed > 5*time.Second {
				t.Errorf("took %s, want %s to fail immediately", elapsed, tc.name)
			}
			if podExists(t, client) {
				t.Errorf("the workload was bound despite %s", tc.name)
			}
		})
	}
}

// P2-2 companion — genuinely transient failures stay retryable.
func TestApplyReadiness_TransientAPIErrorsAreRetried(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ServiceUnavailable", apierrors.NewServiceUnavailable("try later")},
		{"InternalError", apierrors.NewInternalError(context.DeadlineExceeded)},
		{"TooManyRequests", apierrors.NewTooManyRequestsError("slow down")},
		{"ServerTimeout", apierrors.NewServerTimeout(spobackend.SeccompProfileGVR().GroupResource(), "get", 1)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastReadinessPolling(t)
			client := setUpApplyProposalTestClient(t, specWithSeccompBinding())
			recordApplyOrder(t, "")
			injectSeccompGetError(t, client, tc.err)

			_, err := runApplySkippingSeccomp(t, 30*time.Millisecond)
			if err == nil {
				t.Fatalf("runApplyProposal() error = nil, want a readiness timeout for %s", tc.name)
			}
			if !strings.Contains(err.Error(), "timed out") {
				t.Errorf("error = %q, want %s retried until the budget expired", err.Error(), tc.name)
			}
			if podExists(t, client) {
				t.Errorf("the workload was bound despite %s", tc.name)
			}
		})
	}
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	coder, ok := err.(interface{ ExitCode() int })
	if !ok {
		t.Errorf("error %q carries no exit code, want %d", err.Error(), want)
		return
	}
	if got := coder.ExitCode(); got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
}
