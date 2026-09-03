// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
)

const testNetworkPolicyYAML = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nginx-demo
  namespace: default
`

func setUpApplyProposalTestClient(t *testing.T, spec proposal.Spec) dynamic.Interface {
	t.Helper()
	if spec.TargetBinding == nil {
		spec.TargetBinding = &k8s.CanonicalTargetBinding{Namespace: "default", Kind: "Pod", Name: "nginx-demo"}
	}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	proposalGVR := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}
	proposalObj, err := client.Resource(proposalGVR).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("proposal Get() error = %v", err)
	}
	proposalObj.SetUID("proposal-test-uid")
	if _, err := client.Resource(proposalGVR).Namespace("default").Update(context.Background(), proposalObj, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("proposal UID update error = %v", err)
	}

	// Automatically approve the candidate used in tests so apply-proposal
	// preflight (which is now fail-closed) will succeed for the common
	// happy-path tests. Individual hostile tests can override this if
	// needed.
	computed, err := proposal.CandidateDigest(spec)
	if err != nil {
		t.Fatalf("CandidateDigest error: %v", err)
	}
	if err := proposal.SetApprovalState(context.Background(), client, "default", "nginx-demo", proposal.ApprovalApproved, "test auto-approve", computed); err != nil {
		t.Fatalf("proposal.SetApprovalState() error = %v", err)
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

func TestMutationRecordFor_WorkloadCustodyIsFieldScoped(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "web", "namespace": "default", "uid": "uid-1", "resourceVersion": "7"},
		"spec": map[string]interface{}{
			"replicas": int64(9),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{"labels": map[string]interface{}{podLockProfileLabel: "profile", "unrelated": "keep-out"}},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "web",
							"image": "example:v2",
							"securityContext": map[string]interface{}{
								"capabilities":   map[string]interface{}{"add": []interface{}{"NET_ADMIN"}},
								"seccompProfile": map[string]interface{}{"type": "Localhost", "localhostProfile": "profile.json"},
								"runAsUser":      int64(1000),
							},
						},
					},
				},
			},
		},
	}}
	p := plannedArtifact{slug: patchedManifestSlug, gvk: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, ns: "default", nameStr: "web", obj: obj}
	record := mutationRecordFor(p, obj)
	for _, unwanted := range []string{"replicas", "example:v2", "unrelated", "runAsUser"} {
		if strings.Contains(record.Before, unwanted) {
			t.Errorf("field-scoped Before contains unrelated field %q: %s", unwanted, record.Before)
		}
	}
	for _, wanted := range []string{podLockProfileLabel, "capabilities", "seccompProfile", "resourceVersion", "uid"} {
		if !strings.Contains(record.Before, wanted) {
			t.Errorf("field-scoped Before lacks controlled/identity field %q: %s", wanted, record.Before)
		}
	}
}

func TestRunApplyProposal_InitialStatusFailurePreventsMutation(t *testing.T) {
	setUpApplyProposalTestClient(t, proposal.Spec{
		Container:     "nginx",
		Binary:        "/usr/sbin/nginx",
		NetworkPolicy: testNetworkPolicyYAML,
	})
	oldSave, oldApply := saveAttemptStatus, applyManifest
	t.Cleanup(func() { saveAttemptStatus, applyManifest = oldSave, oldApply })
	saveAttemptStatus = func(context.Context, dynamic.Interface, string, string, *unstructured.Unstructured, attempt.Status) error {
		return errors.New("injected ApplyAttempt persistence failure")
	}
	mutations := 0
	applyManifest = func(ctx context.Context, client dynamic.Interface, namespace, content string) error {
		mutations++
		return k8s.Apply(ctx, client, namespace, content)
	}
	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{namespace: "default", yes: true}, "nginx-demo")
	if err == nil || !strings.Contains(err.Error(), "IN_PROGRESS status") {
		t.Fatalf("runApplyProposal() error = %v, want initial status failure", err)
	}
	if mutations != 0 {
		t.Fatalf("mutation count = %d, want zero when Before persistence fails", mutations)
	}
}

func TestRunApplyProposal_AttemptCreationFailurePreventsMutation(t *testing.T) {
	setUpApplyProposalTestClient(t, proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", NetworkPolicy: testNetworkPolicyYAML})
	oldCreate, oldApply := createAttempt, applyManifest
	t.Cleanup(func() { createAttempt, applyManifest = oldCreate, oldApply })
	createAttempt = func(context.Context, dynamic.Interface, string, attempt.Spec) (string, *unstructured.Unstructured, error) {
		return "", nil, errors.New("injected ApplyAttempt create failure")
	}
	mutations := 0
	applyManifest = func(ctx context.Context, client dynamic.Interface, namespace, content string) error {
		mutations++
		return k8s.Apply(ctx, client, namespace, content)
	}
	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{namespace: "default", yes: true}, "nginx-demo")
	if err == nil || !strings.Contains(err.Error(), "creation is required") {
		t.Fatalf("runApplyProposal() error = %v, want creation failure", err)
	}
	if mutations != 0 {
		t.Fatalf("mutation count = %d, want zero when attempt creation fails", mutations)
	}
}

func TestRunApplyProposal_PreStatePersistenceFailurePreventsMutation(t *testing.T) {
	setUpApplyProposalTestClient(t, proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", NetworkPolicy: testNetworkPolicyYAML})
	oldSave, oldApply := saveAttemptStatus, applyManifest
	t.Cleanup(func() { saveAttemptStatus, applyManifest = oldSave, oldApply })
	saves := 0
	saveAttemptStatus = func(ctx context.Context, client dynamic.Interface, namespace, name string, obj *unstructured.Unstructured, status attempt.Status) error {
		saves++
		if saves == 2 {
			return errors.New("injected Before persistence failure")
		}
		return attempt.SaveStatus(ctx, client, namespace, name, obj, status)
	}
	mutations := 0
	applyManifest = func(ctx context.Context, client dynamic.Interface, namespace, content string) error {
		mutations++
		return k8s.Apply(ctx, client, namespace, content)
	}
	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{namespace: "default", yes: true}, "nginx-demo")
	if err == nil || !strings.Contains(err.Error(), "pre-state persistence failed") {
		t.Fatalf("runApplyProposal() error = %v, want pre-state persistence failure", err)
	}
	if mutations != 0 {
		t.Fatalf("mutation count = %d, want zero when Before persistence fails", mutations)
	}
}

func getApplyAttempt(t *testing.T, client dynamic.Interface, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := client.Resource(attempt.GVR).Namespace("default").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("getting ApplyAttempt %q: %v", name, err)
	}
	return obj
}

func TestRunApplyProposal_UnknownOutcomeIsPersisted(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", NetworkPolicy: testNetworkPolicyYAML})
	oldRead, oldApply, oldCreate := readApplyResource, applyManifest, createAttempt
	t.Cleanup(func() { readApplyResource, applyManifest, createAttempt = oldRead, oldApply, oldCreate })
	attemptName := ""
	createAttempt = func(ctx context.Context, client dynamic.Interface, namespace string, spec attempt.Spec) (string, *unstructured.Unstructured, error) {
		name, obj, err := attempt.Create(ctx, client, namespace, spec)
		attemptName = name
		return name, obj, err
	}
	reads := 0
	readApplyResource = func(ctx context.Context, client dynamic.Interface, namespace string, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
		reads++
		if reads == 1 {
			return nil, nil
		}
		return nil, errors.New("injected ambiguous outcome read failure")
	}
	applyManifest = func(context.Context, dynamic.Interface, string, string) error {
		return errors.New("request response lost after dispatch")
	}
	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{namespace: "default", yes: true}, "nginx-demo")
	if err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("runApplyProposal() error = %v, want unknown outcome", err)
	}
	attemptObj := getApplyAttempt(t, client, attemptName)
	status, found, err := unstructured.NestedMap(attemptObj.Object, "status")
	if err != nil || !found || status["state"] != attempt.StateOutcomeUnknown {
		t.Fatalf("attempt status = %#v, found=%t err=%v, want OUTCOME_UNKNOWN", status, found, err)
	}
	mutations, found, err := unstructured.NestedSlice(attemptObj.Object, "status", "mutations")
	if err != nil || !found || len(mutations) != 1 {
		t.Fatalf("mutations = %#v, found=%t err=%v, want one durable record", mutations, found, err)
	}
	if !strings.Contains(string(mustJSON(mutations[0])), attempt.ResultUnknown) {
		t.Fatalf("mutation record = %#v, want OUTCOME_UNKNOWN", mutations[0])
	}
	if attemptObj.Object["spec"].(map[string]interface{})["proposalUID"] != "proposal-test-uid" {
		t.Fatalf("proposal custody missing: %#v", attemptObj.Object["spec"])
	}
	spec := attemptObj.Object["spec"].(map[string]interface{})
	if digest, ok := spec["approvedCandidateDigest"].(string); !ok || digest == "" {
		t.Fatalf("approved digest custody = %#v, want non-empty proposal digest", spec["approvedCandidateDigest"])
	}
	target := spec["target"].(map[string]interface{})
	if target["namespace"] != "default" || target["container"] != "nginx" || target["workload"].(map[string]interface{})["name"] != "nginx-demo" {
		t.Fatalf("target custody = %#v, want canonical nginx-demo/default/nginx target", target)
	}
	before, found, err := unstructured.NestedString(mutations[0].(map[string]interface{}), "before")
	if err != nil || !found || before == "" {
		t.Fatalf("mutation Before = %q, found=%t err=%v; want durable pre-state", before, found, err)
	}
}

func TestRunApplyProposal_PartialApplicationIsPersisted(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container: "nginx", Binary: "/usr/sbin/nginx",
		PodLock:       "apiVersion: podlock.kubewarden.io/v1alpha1\nkind: LandlockProfile\nmetadata:\n  name: nginx-profile\n",
		NetworkPolicy: testNetworkPolicyYAML,
	})
	oldApply, oldCreate := applyManifest, createAttempt
	t.Cleanup(func() { applyManifest, createAttempt = oldApply, oldCreate })
	attemptName := ""
	createAttempt = func(ctx context.Context, client dynamic.Interface, namespace string, spec attempt.Spec) (string, *unstructured.Unstructured, error) {
		name, obj, err := attempt.Create(ctx, client, namespace, spec)
		attemptName = name
		return name, obj, err
	}
	applications := 0
	applyManifest = func(ctx context.Context, client dynamic.Interface, namespace, content string) error {
		applications++
		if applications == 1 {
			return k8s.Apply(ctx, client, namespace, content)
		}
		return errors.New("known second mutation failure")
	}
	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{namespace: "default", yes: true}, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want second mutation failure")
	}
	if applications != 2 {
		t.Fatalf("mutation count = %d, want two and no third", applications)
	}
	attemptObj := getApplyAttempt(t, client, attemptName)
	status := attemptObj.Object["status"].(map[string]interface{})
	if status["state"] != attempt.StatePartiallyApplied {
		t.Fatalf("attempt state = %#v, want PARTIALLY_APPLIED", status["state"])
	}
	mutations := status["mutations"].([]interface{})
	if len(mutations) != 2 {
		t.Fatalf("mutation records = %d, want two", len(mutations))
	}
	if mutations[0].(map[string]interface{})["result"] != attempt.ResultSucceeded || mutations[1].(map[string]interface{})["result"] != attempt.ResultFailed {
		t.Fatalf("mutation results = %#v, want SUCCEEDED then FAILED", mutations)
	}
	if mutations[0].(map[string]interface{})["observedAfter"] == nil || mutations[0].(map[string]interface{})["observedAfter"] == "" {
		t.Fatalf("first mutation lacks durable observed result: %#v", mutations[0])
	}
}

func mustJSON(value interface{}) []byte {
	b, _ := json.Marshal(value)
	return b
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
		t.Fatal("runApplyProposal() error = nil, want an error since preflight should reject unsupported GVKs")
	}

	// Preflight should detect the unrecognized PodLock GVK and fail before
	// any cluster mutation. The error should name the offending artifact.
	if !strings.Contains(err.Error(), "apply preflight failed for PodLock") {
		t.Errorf("error = %q, want it to mention preflight failure for PodLock", err.Error())
	}

	// Ensure NetworkPolicy was NOT applied due to fail-closed preflight.
	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err == nil {
		t.Errorf("NetworkPolicy was applied despite preflight failure; expected no mutations")
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

func TestRunApplyProposal_RejectsLegacyApprovedWithoutDigest(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	// create proposal
	spec := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", TargetBinding: &k8s.CanonicalTargetBinding{Namespace: "default", Kind: "Pod", Name: "nginx-demo"}}
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	// Manually set a legacy Approved status with empty ApprovedCandidateDigest
	securityProfileProposalGVR := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}
	resource := client.Resource(securityProfileProposalGVR).Namespace("default")
	obj, err := resource.Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fetching object: %v", err)
	}
	obj.SetUID("proposal-test-uid")
	if _, err := resource.Update(context.Background(), obj, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("setting proposal UID: %v", err)
	}
	obj.Object["status"] = map[string]interface{}{
		"approvalState": "Approved",
	}
	if _, err := resource.UpdateStatus(context.Background(), obj, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}

	old := newDynamicClientForApplyProposal
	newDynamicClientForApplyProposal = func() (dynamic.Interface, error) { return client, nil }
	defer func() { newDynamicClientForApplyProposal = old }()

	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default", yes: true}
	err = runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want an error for legacy Approved without digest")
	}
	if !strings.Contains(err.Error(), "legacy approval") {
		t.Errorf("error = %q, want it to mention legacy approval", err.Error())
	}
}

func TestRunApplyProposal_RejectsAfterSpecMutation(t *testing.T) {
	// Create spec A and approve it
	targetBinding := &k8s.CanonicalTargetBinding{Namespace: "default", Kind: "Pod", Name: "nginx-demo"}
	specA := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", NetworkPolicy: testNetworkPolicyYAML, TargetBinding: targetBinding}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", specA); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	proposalGVR := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}
	proposalObj, err := client.Resource(proposalGVR).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("proposal Get() error = %v", err)
	}
	proposalObj.SetUID("proposal-test-uid")
	if _, err := client.Resource(proposalGVR).Namespace("default").Update(context.Background(), proposalObj, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("proposal UID update error = %v", err)
	}
	computedA, err := proposal.CandidateDigest(specA)
	if err != nil {
		t.Fatalf("CandidateDigest error: %v", err)
	}
	if err := proposal.SetApprovalState(context.Background(), client, "default", "nginx-demo", proposal.ApprovalApproved, "test", computedA); err != nil {
		t.Fatalf("proposal.SetApprovalState() error = %v", err)
	}

	// Mutate spec to B (same name) — Save preserves status but changes spec
	specB := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", NetworkPolicy: "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: nginx-demo-b\n  namespace: default\n", TargetBinding: targetBinding}
	if err := proposal.Save(context.Background(), client, "default", "nginx-demo", specB); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	proposalObj, err = client.Resource(proposalGVR).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("proposal Get() after mutation = %v", err)
	}
	proposalObj.SetUID("proposal-test-uid")
	if _, err := client.Resource(proposalGVR).Namespace("default").Update(context.Background(), proposalObj, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("proposal UID restore = %v", err)
	}

	old := newDynamicClientForApplyProposal
	newDynamicClientForApplyProposal = func() (dynamic.Interface, error) { return client, nil }
	defer func() { newDynamicClientForApplyProposal = old }()

	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default", yes: true}
	err = runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want an error due to candidate digest mismatch after spec mutation")
	}
	if !strings.Contains(err.Error(), "approved candidate digest mismatch") {
		t.Errorf("error = %q, want it to mention digest mismatch", err.Error())
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

func TestAlignBindingWithArtifactPlan_PodLockSkipIsolation(t *testing.T) {
	const manifest = `apiVersion: v1
kind: Pod
metadata:
  name: governed
  namespace: team-a
  labels:
    podlock.kubewarden.io/profile: governed
    app: preserved
  annotations:
    example.test/preserved: "true"
spec:
  containers:
    - name: tools
      image: example.invalid/tools@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
      securityContext:
        seccompProfile:
          type: Localhost
          localhostProfile: operator/governed-seccomp.json
`
	artifact := proposalArtifact{name: "Patched Manifest", slug: patchedManifestSlug, content: manifest, available: true}

	t.Run("skipped", func(t *testing.T) {
		planned, err := buildPlannedArtifact(artifact, "fallback")
		if err != nil {
			t.Fatal(err)
		}
		if err := alignBindingWithArtifactPlan(&planned, map[string]bool{"podlock": true}); err != nil {
			t.Fatal(err)
		}
		if _, found := podLockBindingName(planned.obj); found {
			t.Fatal("PodLock binding survived explicit skip")
		}
		if planned.obj.GetNamespace() != "team-a" || planned.obj.GetLabels()["app"] != "preserved" || planned.obj.GetAnnotations()["example.test/preserved"] != "true" {
			t.Fatalf("unrelated metadata changed: %#v", planned.obj.Object)
		}
		profiles := referencedLocalhostProfiles(planned.obj)
		if len(profiles) != 1 || profiles[0] != "operator/governed-seccomp.json" {
			t.Fatalf("Seccomp binding changed: %v", profiles)
		}
		var roundTrip unstructured.Unstructured
		if err := json.Unmarshal([]byte(planned.content), &roundTrip.Object); err != nil {
			t.Fatal(err)
		}
		if _, found := podLockBindingName(&roundTrip); found || !reflect.DeepEqual(referencedLocalhostProfiles(&roundTrip), profiles) {
			t.Fatal("serialized apply payload differs from transformed plan")
		}
	})

	t.Run("active", func(t *testing.T) {
		planned, err := buildPlannedArtifact(artifact, "fallback")
		if err != nil {
			t.Fatal(err)
		}
		before := planned.content
		if err := alignBindingWithArtifactPlan(&planned, map[string]bool{}); err != nil {
			t.Fatal(err)
		}
		if name, found := podLockBindingName(planned.obj); !found || name != "governed" {
			t.Fatalf("active PodLock binding changed: name=%q found=%v", name, found)
		}
		if planned.content != before {
			t.Fatal("active manifest was rewritten")
		}
	})
}

func TestAlignBindingWithArtifactPlan_SeccompSkipIsolation(t *testing.T) {
	const manifest = `apiVersion: v1
kind: Pod
metadata:
  name: governed
  namespace: team-a
  labels:
    podlock.kubewarden.io/profile: governed
    app: preserved
spec:
  containers:
    - name: tools
      image: example.invalid/tools
      securityContext:
        seccompProfile:
          type: Localhost
          localhostProfile: operator/governed-seccomp.json
`
	planned, err := buildPlannedArtifact(proposalArtifact{name: "Patched Manifest", slug: patchedManifestSlug, content: manifest, available: true}, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if err := alignBindingWithArtifactPlan(&planned, map[string]bool{"spo-seccompprofile": true}); err != nil {
		t.Fatal(err)
	}
	if got := referencedLocalhostProfiles(planned.obj); len(got) != 0 {
		t.Fatalf("skipped Seccomp binding survived: %v", got)
	}
	if planned.obj.GetLabels()[podLockProfileLabel] != "governed" || planned.obj.GetLabels()["app"] != "preserved" {
		t.Fatalf("unrelated or PodLock metadata changed: %#v", planned.obj.GetLabels())
	}
}

func TestValidateCompositionCompatibility_SelectedPairRejected(t *testing.T) {
	plans := []plannedArtifact{{slug: "spo-seccompprofile"}, {slug: "podlock"}, {slug: patchedManifestSlug}}
	if err := validateCompositionCompatibility(plans); err == nil {
		t.Fatal("validateCompositionCompatibility() = nil, want unsupported pair rejection")
	}
	// Selection order is not authority: the same pair in reverse order must
	// remain rejected.
	if err := validateCompositionCompatibility([]plannedArtifact{plans[1], plans[0]}); err == nil {
		t.Fatal("reordered pair was accepted")
	}
}

func TestApplyProposal_CompositionRejectedBeforeMutation(t *testing.T) {
	spec := proposal.Spec{
		Container:         "nginx",
		Binary:            "/usr/sbin/nginx",
		PodLock:           testPodLockYAMLA,
		SPOSeccompProfile: testSeccompProfileYAML,
		PatchedManifest:   testPatchedManifestWithSeccompYAML,
	}
	setUpApplyProposalTestClient(t, spec)
	var mutations int
	oldApply := applyManifest
	applyManifest = func(ctx context.Context, c dynamic.Interface, namespace, content string) error {
		mutations++
		return oldApply(ctx, c, namespace, content)
	}
	t.Cleanup(func() { applyManifest = oldApply })
	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{namespace: "default", yes: true, restart: true}, "nginx-demo")
	if err == nil || !strings.Contains(err.Error(), "composition is unsupported") {
		t.Fatalf("runApplyProposal() error = %v, want composition rejection", err)
	}
	if mutations != 0 {
		t.Fatalf("mutations = %d, want zero before compatibility rejection", mutations)
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

// TestRunApplyProposal_PatchedManifestExcludedByDefault checks the
// inverted default: Patched Manifest is the only artifact whose apply
// deletes and recreates the target pod (internal/k8s.applyPod), so
// unlike the other three it's left out unless --restart is passed —
// confirmed live (2026-07-30): the opt-out version of this (--skip) let
// nginx-demo get force-restarted by every apply-proposal run regardless
// of whether its enforcement side (SPO/PodLock) was actually ready,
// racking up a 73-minute, 15-restart CrashLoopBackOff with no single
// explicit decision to restart it.
func TestRunApplyProposal_PatchedManifestExcludedByDefault(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:       "nginx",
		Binary:          "/usr/sbin/nginx",
		NetworkPolicy:   testNetworkPolicyYAML,
		PatchedManifest: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: nginx-demo\n  namespace: default\n",
	})

	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default", yes: true}
	if err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Leaving out 1 artifact(s) that would restart the target pod — pass --restart to include:\n  - Patched Manifest") {
		t.Errorf("stdout = %q, want it to report leaving out Patched Manifest pending --restart", out)
	}
	if !strings.Contains(out, "applied: NetworkPolicy") {
		t.Errorf("stdout = %q, want NetworkPolicy to still be applied", out)
	}
	if strings.Contains(out, "applied: Patched Manifest") {
		t.Errorf("stdout = %q, Patched Manifest should not have been applied without --restart", out)
	}

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err == nil {
		t.Error("Patched Manifest's Pod was applied despite --restart not being passed")
	}
}

// TestRunApplyProposal_RestartFlagIncludesPatchedManifest is the
// opt-in half of the previous test: --restart is how you get the old
// (now non-default) apply-everything-available behavior for this one
// artifact back.
func TestRunApplyProposal_RestartFlagIncludesPatchedManifest(t *testing.T) {
	client := setUpApplyProposalTestClient(t, proposal.Spec{
		Container:       "nginx",
		Binary:          "/usr/sbin/nginx",
		PatchedManifest: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: nginx-demo\n  namespace: default\n",
	})

	var stdout bytes.Buffer
	opts := applyProposalOptions{namespace: "default", yes: true, restart: true}
	if err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), opts, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "applied: Patched Manifest") {
		t.Errorf("stdout = %q, want Patched Manifest applied with --restart", out)
	}
	if strings.Contains(out, "Leaving out") {
		t.Errorf("stdout = %q, --restart should mean nothing gets left out for that reason", out)
	}

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err != nil {
		t.Errorf("Patched Manifest's Pod was not applied despite --restart: %v", err)
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

const testPodLockYAMLA = `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: landlock-a
  namespace: default
spec:
  profilesByContainer: {}
`

const testPodLockYAMLB = `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: landlock-b
  namespace: default
spec:
  profilesByContainer: {}
`

func TestRunApplyProposal_RejectsMutationAfterPlanningBeforeRevalidation(t *testing.T) {
	specA := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: testPodLockYAMLA}
	specB := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: testPodLockYAMLB}
	client := setUpApplyProposalTestClient(t, specA)

	oldHook := afterApplyProposalPlanBuilt
	oldApply := applyManifest
	t.Cleanup(func() {
		afterApplyProposalPlanBuilt = oldHook
		applyManifest = oldApply
	})

	afterApplyProposalPlanBuilt = func() {
		if err := proposal.Save(context.Background(), client, "default", "nginx-demo", specB); err != nil {
			t.Fatalf("proposal.Save() mutation error = %v", err)
		}
	}

	applyCount := 0
	applyManifest = func(ctx context.Context, c dynamic.Interface, namespace, content string) error {
		applyCount++
		return k8s.Apply(ctx, c, namespace, content)
	}

	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{
		namespace: "default",
		yes:       true,
	}, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want rejection after post-plan spec mutation")
	}
	if !strings.Contains(err.Error(), "authorization changed before apply") &&
		!strings.Contains(err.Error(), "candidate changed since plan creation") {
		t.Fatalf("error = %q, want post-plan authorization/candidate continuity rejection", err.Error())
	}
	if applyCount != 0 {
		t.Fatalf("applyCount = %d, want 0 after rejection", applyCount)
	}

	landlockGVR := schema.GroupVersionResource{Group: "podlock.kubewarden.io", Version: "v1alpha1", Resource: "landlockprofiles"}
	if _, getErr := client.Resource(landlockGVR).Namespace("default").Get(context.Background(), "landlock-a", metav1.GetOptions{}); !apierrors.IsNotFound(getErr) {
		t.Fatalf("landlock-a get error = %v, want NotFound after rejection", getErr)
	}
	if _, getErr := client.Resource(landlockGVR).Namespace("default").Get(context.Background(), "landlock-b", metav1.GetOptions{}); !apierrors.IsNotFound(getErr) {
		t.Fatalf("landlock-b get error = %v, want NotFound after rejection", getErr)
	}
}

func TestRunApplyProposal_RejectsRevocationAfterPlanningBeforeRevalidation(t *testing.T) {
	specA := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: testPodLockYAMLA}
	client := setUpApplyProposalTestClient(t, specA)

	oldHook := afterApplyProposalPlanBuilt
	oldApply := applyManifest
	t.Cleanup(func() {
		afterApplyProposalPlanBuilt = oldHook
		applyManifest = oldApply
	})

	afterApplyProposalPlanBuilt = func() {
		if err := proposal.SetApprovalState(context.Background(), client, "default", "nginx-demo", proposal.ApprovalRejected, "revoked during apply", ""); err != nil {
			t.Fatalf("SetApprovalState(revoke) error = %v", err)
		}
	}

	applyCount := 0
	applyManifest = func(ctx context.Context, c dynamic.Interface, namespace, content string) error {
		applyCount++
		return k8s.Apply(ctx, c, namespace, content)
	}

	var stdout bytes.Buffer
	err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{
		namespace: "default",
		yes:       true,
	}, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want rejection after post-plan approval revocation")
	}
	if !strings.Contains(err.Error(), "authorization changed before apply") ||
		!strings.Contains(err.Error(), "not Approved") {
		t.Fatalf("error = %q, want second validation to reject non-Approved state", err.Error())
	}
	if applyCount != 0 {
		t.Fatalf("applyCount = %d, want 0 after rejection", applyCount)
	}
}

func TestRunApplyProposal_RejectsApprovalDigestReplacementAfterPlanning(t *testing.T) {
	specA := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: testPodLockYAMLA}
	specB := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: testPodLockYAMLB}
	client := setUpApplyProposalTestClient(t, specA)

	digestB, err := proposal.CandidateDigest(specB)
	if err != nil {
		t.Fatalf("CandidateDigest(specB) error = %v", err)
	}

	oldHook := afterApplyProposalPlanBuilt
	oldApply := applyManifest
	t.Cleanup(func() {
		afterApplyProposalPlanBuilt = oldHook
		applyManifest = oldApply
	})

	afterApplyProposalPlanBuilt = func() {
		proposalGVR := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}
		resource := client.Resource(proposalGVR).Namespace("default")
		obj, getErr := resource.Get(context.Background(), "nginx-demo", metav1.GetOptions{})
		if getErr != nil {
			t.Fatalf("fetching proposal for status tamper: %v", getErr)
		}
		obj.Object["status"] = map[string]interface{}{
			"approvalState":            "Approved",
			"approvedCandidateDigest":  digestB,
			"approvalMechanismVersion": "candidate-v1",
			"reason":                   "tampered",
		}
		if _, updErr := resource.UpdateStatus(context.Background(), obj, metav1.UpdateOptions{}); updErr != nil {
			t.Fatalf("UpdateStatus tamper error = %v", updErr)
		}
	}

	applyCount := 0
	applyManifest = func(ctx context.Context, c dynamic.Interface, namespace, content string) error {
		applyCount++
		return k8s.Apply(ctx, c, namespace, content)
	}

	var stdout bytes.Buffer
	err = runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{
		namespace: "default",
		yes:       true,
	}, "nginx-demo")
	if err == nil {
		t.Fatal("runApplyProposal() error = nil, want rejection after digest replacement")
	}
	if !strings.Contains(err.Error(), "authorization changed before apply") ||
		!strings.Contains(err.Error(), "approved candidate digest mismatch") {
		t.Fatalf("error = %q, want digest mismatch rejection", err.Error())
	}
	if applyCount != 0 {
		t.Fatalf("applyCount = %d, want 0 after rejection", applyCount)
	}
}

func TestRunApplyProposal_UsesPlannedPayloadEvenIfProposalMutatesAtApplyTime(t *testing.T) {
	specA := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: testPodLockYAMLA}
	specB := proposal.Spec{Container: "nginx", Binary: "/usr/sbin/nginx", PodLock: testPodLockYAMLB}
	client := setUpApplyProposalTestClient(t, specA)

	oldHook := afterApplyProposalPlanBuilt
	oldApply := applyManifest
	t.Cleanup(func() {
		afterApplyProposalPlanBuilt = oldHook
		applyManifest = oldApply
	})

	afterApplyProposalPlanBuilt = nil

	applyCount := 0
	var appliedContent string
	applyManifest = func(ctx context.Context, c dynamic.Interface, namespace, content string) error {
		applyCount++
		appliedContent = content
		if err := proposal.Save(context.Background(), client, "default", "nginx-demo", specB); err != nil {
			t.Fatalf("proposal.Save() mutation during apply error = %v", err)
		}
		return k8s.Apply(ctx, c, namespace, content)
	}

	var stdout bytes.Buffer
	if err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{
		namespace: "default",
		yes:       true,
	}, "nginx-demo"); err != nil {
		t.Fatalf("runApplyProposal() error = %v", err)
	}

	if applyCount != 1 {
		t.Fatalf("applyCount = %d, want 1", applyCount)
	}
	if !strings.Contains(appliedContent, "name: landlock-a") {
		t.Fatalf("applied payload does not contain planned candidate A content:\n%s", appliedContent)
	}
	if strings.Contains(appliedContent, "name: landlock-b") {
		t.Fatalf("applied payload unexpectedly contains candidate B content:\n%s", appliedContent)
	}

	landlockGVR := schema.GroupVersionResource{Group: "podlock.kubewarden.io", Version: "v1alpha1", Resource: "landlockprofiles"}
	if _, err := client.Resource(landlockGVR).Namespace("default").Get(context.Background(), "landlock-a", metav1.GetOptions{}); err != nil {
		t.Fatalf("landlock-a not applied: %v", err)
	}
	if _, err := client.Resource(landlockGVR).Namespace("default").Get(context.Background(), "landlock-b", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("landlock-b get error = %v, want NotFound (must not be applied)", err)
	}
}

func TestRunApplyProposal_PreservesPlannedLandlockSemantics(t *testing.T) {
	expected := podlock.LandlockProfile{
		APIVersion: "podlock.kubewarden.io/v1alpha1",
		Kind:       "LandlockProfile",
		Metadata:   podlock.Metadata{Name: "worker-profile", Namespace: "team-a"},
		Spec: podlock.LandlockProfileSpec{ProfilesByContainer: map[string]podlock.ProfileByBinary{
			"api": {
				"/usr/bin/api": {
					ReadOnly:      []string{"/etc/ssl/certs", "/usr/share/zoneinfo"},
					ReadWrite:     []string{"/tmp/api-write"},
					ReadExec:      []string{"/usr/bin/helper"},
					ReadWriteExec: []string{"/opt/runtime/tool"},
				},
			},
			"worker": {
				"/usr/local/bin/worker": {
					ReadExec:      []string{"/usr/local/libexec/worker-helper"},
					ReadWriteExec: []string{"/opt/worker/runtime"},
				},
			},
		}},
	}
	manifest, err := yaml.Marshal(&expected)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	spec := proposal.Spec{Container: "api", Binary: "/usr/bin/api", PodLock: string(manifest), TargetBinding: &k8s.CanonicalTargetBinding{Namespace: "team-a", Kind: "Pod", Name: "worker-proposal"}}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := proposal.Save(context.Background(), client, "team-a", "worker-proposal", spec); err != nil {
		t.Fatalf("proposal.Save() error = %v", err)
	}
	proposalGVR := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}
	proposalObj, err := client.Resource(proposalGVR).Namespace("team-a").Get(context.Background(), "worker-proposal", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("proposal Get() error = %v", err)
	}
	proposalObj.SetUID("proposal-worker-uid")
	if _, err := client.Resource(proposalGVR).Namespace("team-a").Update(context.Background(), proposalObj, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("proposal UID update error = %v", err)
	}
	digest, err := proposal.CandidateDigest(spec)
	if err != nil {
		t.Fatalf("CandidateDigest() error = %v", err)
	}
	if err := proposal.SetApprovalState(context.Background(), client, "team-a", "worker-proposal", proposal.ApprovalApproved, "test", digest); err != nil {
		t.Fatalf("SetApprovalState() error = %v", err)
	}

	oldClient := newDynamicClientForApplyProposal
	newDynamicClientForApplyProposal = func() (dynamic.Interface, error) { return client, nil }
	t.Cleanup(func() { newDynamicClientForApplyProposal = oldClient })

	var stdout bytes.Buffer
	if err := runApplyProposal(context.Background(), &stdout, strings.NewReader(""), applyProposalOptions{
		namespace: "team-a",
		yes:       true,
	}, "worker-proposal"); err != nil {
		t.Fatalf("runApplyProposal() error = %v\noutput:\n%s", err, stdout.String())
	}

	gvr := schema.GroupVersionResource{Group: "podlock.kubewarden.io", Version: "v1alpha1", Resource: "landlockprofiles"}
	got, err := client.Resource(gvr).Namespace("team-a").Get(context.Background(), "worker-profile", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fetching persisted LandlockProfile: %v", err)
	}
	var persisted podlock.LandlockProfile
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(got.Object, &persisted); err != nil {
		t.Fatalf("converting persisted LandlockProfile: %v", err)
	}
	if got, want := normalizeLandlockProfile(persisted), normalizeLandlockProfile(expected); !reflect.DeepEqual(got, want) {
		t.Fatalf("planned/persisted LandlockProfile semantics differ:\n got: %#v\nwant: %#v", got, want)
	}
}

func normalizeLandlockProfile(profile podlock.LandlockProfile) podlock.LandlockProfile {
	normalized := podlock.LandlockProfile{
		APIVersion: profile.APIVersion,
		Kind:       profile.Kind,
		Metadata:   profile.Metadata,
		Spec:       podlock.LandlockProfileSpec{ProfilesByContainer: map[string]podlock.ProfileByBinary{}},
	}
	for container, binaries := range profile.Spec.ProfilesByContainer {
		normalized.Spec.ProfilesByContainer[container] = podlock.ProfileByBinary{}
		for binary, p := range binaries {
			normalized.Spec.ProfilesByContainer[container][binary] = podlock.Profile{
				ReadOnly:      sortedPaths(p.ReadOnly),
				ReadWrite:     sortedPaths(p.ReadWrite),
				ReadExec:      sortedPaths(p.ReadExec),
				ReadWriteExec: sortedPaths(p.ReadWriteExec),
			}
		}
	}
	return normalized
}

func sortedPaths(paths []string) []string {
	result := append([]string(nil), paths...)
	sort.Strings(result)
	return result
}
