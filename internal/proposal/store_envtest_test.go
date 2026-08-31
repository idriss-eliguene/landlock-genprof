//go:build envtest

package proposal

import (
	"context"
	"fmt"
	"os"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg *rest.Config
	env *envtest.Environment
)

func TestMain(m *testing.M) {
	// Start envtest once per package.
	// All tests in this package share a single API server instance for efficiency.

	// Determine CRD path
	crdPath := "deploy/crd-securityprofileproposal.yaml"
	if _, err := os.Stat(crdPath); err != nil {
		crdPath = "../../deploy/crd-securityprofileproposal.yaml"
	}

	env = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths:              []string{crdPath},
			ErrorIfPathMissing: true,
		},
	}

	var err error
	cfg, err = env.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envtest.Start: %v\n", err)
		os.Exit(1)
	}

	// Run all tests
	code := m.Run()

	// Explicit cleanup
	if env != nil {
		env.Stop()
	}

	os.Exit(code)
}

func setupEnvtest(t *testing.T) dynamic.Interface {
	if cfg == nil {
		t.Fatal("envtest not initialized (TestMain may not have run)")
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}
	return dynamicClient
}

// TestUpdateCannotModifyStatus validates that normal Update cannot persist status changes.
func TestUpdateCannotModifyStatus(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()

	// Create a valid SecurityProfileProposal
	proposal := &unstructured.Unstructured{}
	proposal.SetAPIVersion("landlockgenprof.io/v1alpha1")
	proposal.SetKind("SecurityProfileProposal")
	proposal.SetName("test-proposal")
	proposal.SetNamespace("default")

	// Set spec fields
	spec := map[string]interface{}{
		"container":         "nginx",
		"binary":            "/usr/sbin/nginx",
		"generatedAt":       "2026-08-10T09:00:00Z",
		"historyUsed":       false,
		"podLock":           "test-podlock",
		"networkPolicy":     "test-networkpolicy",
		"patchedManifest":   "test-manifest",
		"spoSeccompProfile": "test-seccomp",
	}
	proposal.Object["spec"] = spec

	// Create the proposal
	created, err := client.Resource(securityProfileProposalGVR).Namespace("default").Create(ctx, proposal, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create proposal: %v", err)
	}

	// Set status through UpdateStatus subresource
	proposal.Object["status"] = map[string]interface{}{
		"approvalState": "Reviewed",
		"reason":        "initial review",
		"updatedAt":     "2026-08-10T09:01:00Z",
	}
	proposal.SetResourceVersion(created.GetResourceVersion())

	statusResource := client.Resource(securityProfileProposalGVR).Namespace("default")
	_, err = statusResource.UpdateStatus(ctx, proposal, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	// Fetch the proposal to verify status was set
	fetched, err := statusResource.Get(ctx, "test-proposal", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get after UpdateStatus: %v", err)
	}

	status, ok := fetched.Object["status"].(map[string]interface{})
	if !ok || status["approvalState"] != "Reviewed" {
		t.Errorf("Status not set by UpdateStatus: %v", fetched.Object["status"])
	}

	// Now attempt to modify status through normal Update (should fail or be ignored)
	fetched.Object["status"] = map[string]interface{}{
		"approvalState": "Approved", // Try to change through Update
		"reason":        "update attempt",
		"updatedAt":     "2026-08-10T09:02:00Z",
	}

	_, err = statusResource.Update(ctx, fetched, metav1.UpdateOptions{})
	if err != nil {
		// This is expected to fail on real API server
		if !apierrors.IsBadRequest(err) && !apierrors.IsInvalid(err) {
			t.Logf("Update with status field returned: %v", err)
		}
	}

	// Verify status was NOT changed by the Update
	refetched, err := statusResource.Get(ctx, "test-proposal", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}

	status, ok = refetched.Object["status"].(map[string]interface{})
	if !ok {
		t.Error("Status subresource missing after Update")
		return
	}

	if status["approvalState"] != "Reviewed" {
		t.Errorf("Status was changed by Update (should be immutable via Update): approvalState=%v, want Reviewed",
			status["approvalState"])
	}
}

func TestSecurityProfileProposalCanonicalTargetBindingRoundTrip(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "landlockgenprof.io/v1alpha1",
		"kind":       "SecurityProfileProposal",
		"metadata":   map[string]interface{}{"name": "bound-proposal", "namespace": "default"},
		"spec": map[string]interface{}{
			"container": "app", "binary": "/app/server",
			"targetBinding": map[string]interface{}{"namespace": "team-a", "group": "apps", "kind": "Deployment", "name": "api"},
		},
	}}
	resource := client.Resource(securityProfileProposalGVR).Namespace("default")
	if _, err := resource.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create SecurityProfileProposal: %v", err)
	}
	fetched, err := resource.Get(ctx, "bound-proposal", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get SecurityProfileProposal: %v", err)
	}
	binding, found, err := unstructured.NestedMap(fetched.Object, "spec", "targetBinding")
	if err != nil || !found || binding["namespace"] != "team-a" || binding["group"] != "apps" || binding["kind"] != "Deployment" || binding["name"] != "api" {
		t.Fatalf("targetBinding = %#v, found=%t, err=%v", binding, found, err)
	}
}

// TestUpdateStatusPreservesSpec validates that UpdateStatus changes status without altering spec.
func TestUpdateStatusPreservesSpec(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()

	// Create a proposal with known spec
	proposal := &unstructured.Unstructured{}
	proposal.SetAPIVersion("landlockgenprof.io/v1alpha1")
	proposal.SetKind("SecurityProfileProposal")
	proposal.SetName("test-proposal-2")
	proposal.SetNamespace("default")

	spec := map[string]interface{}{
		"container":         "nginx",
		"binary":            "/usr/sbin/nginx",
		"generatedAt":       "2026-08-10T09:00:00Z",
		"historyUsed":       false,
		"podLock":           "original-podlock",
		"networkPolicy":     "original-network",
		"patchedManifest":   "original-manifest",
		"spoSeccompProfile": "original-seccomp",
	}
	proposal.Object["spec"] = spec

	resource := client.Resource(securityProfileProposalGVR).Namespace("default")

	// Create the proposal
	created, err := resource.Create(ctx, proposal, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create proposal: %v", err)
	}

	// Update spec through normal Update
	created.Object["spec"] = map[string]interface{}{
		"container":         "nginx",
		"binary":            "/usr/sbin/nginx",
		"generatedAt":       "2026-08-10T09:05:00Z",
		"historyUsed":       true, // Changed
		"podLock":           "updated-podlock",
		"networkPolicy":     "updated-network",
		"patchedManifest":   "updated-manifest",
		"spoSeccompProfile": "updated-seccomp",
	}

	updated, err := resource.Update(ctx, created, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Update spec: %v", err)
	}

	// Now change status through UpdateStatus
	updated.Object["status"] = map[string]interface{}{
		"approvalState": "Approved",
		"reason":        "status change test",
		"updatedAt":     "2026-08-10T09:06:00Z",
	}

	statusUpdated, err := resource.UpdateStatus(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	// Verify status changed
	status, ok := statusUpdated.Object["status"].(map[string]interface{})
	if !ok || status["approvalState"] != "Approved" {
		t.Errorf("Status not updated: %v", statusUpdated.Object["status"])
	}

	// Verify spec is still the updated value (not reverted)
	finalSpec, ok := statusUpdated.Object["spec"].(map[string]interface{})
	if !ok {
		t.Error("Spec missing after UpdateStatus")
		return
	}

	if finalSpec["podLock"] != "updated-podlock" {
		t.Errorf("Spec was reverted by UpdateStatus: podLock=%v, want updated-podlock",
			finalSpec["podLock"])
	}

	if finalSpec["historyUsed"] != true {
		t.Errorf("Spec was reverted by UpdateStatus: historyUsed=%v, want true",
			finalSpec["historyUsed"])
	}
}

// TestStaleResourceVersionProduces409 validates that stale resourceVersion returns real Conflict.
func TestStaleResourceVersionProduces409(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()

	// Create a proposal
	proposal := &unstructured.Unstructured{}
	proposal.SetAPIVersion("landlockgenprof.io/v1alpha1")
	proposal.SetKind("SecurityProfileProposal")
	proposal.SetName("test-proposal-3")
	proposal.SetNamespace("default")

	spec := map[string]interface{}{
		"container":         "nginx",
		"binary":            "/usr/sbin/nginx",
		"generatedAt":       "2026-08-10T09:00:00Z",
		"historyUsed":       false,
		"podLock":           "test-podlock",
		"networkPolicy":     "test-network",
		"patchedManifest":   "test-manifest",
		"spoSeccompProfile": "test-seccomp",
	}
	proposal.Object["spec"] = spec

	resource := client.Resource(securityProfileProposalGVR).Namespace("default")

	// Fetch the same object twice
	_, err := resource.Create(ctx, proposal, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	copy1, err := resource.Get(ctx, "test-proposal-3", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get copy1: %v", err)
	}

	copy2, err := resource.Get(ctx, "test-proposal-3", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get copy2: %v", err)
	}

	// Verify both have same resourceVersion
	if copy1.GetResourceVersion() != copy2.GetResourceVersion() {
		t.Fatalf("Copies have different resourceVersions: %s vs %s",
			copy1.GetResourceVersion(), copy2.GetResourceVersion())
	}

	// Update copy1 (advances resourceVersion)
	copy1.Object["spec"] = map[string]interface{}{
		"container":         "nginx",
		"binary":            "/usr/sbin/nginx",
		"generatedAt":       "2026-08-10T09:10:00Z",
		"historyUsed":       true,
		"podLock":           "updated1",
		"networkPolicy":     "network1",
		"patchedManifest":   "manifest1",
		"spoSeccompProfile": "seccomp1",
	}

	updated1, err := resource.Update(ctx, copy1, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Update copy1: %v", err)
	}

	// Attempt to update copy2 with stale resourceVersion (should fail with 409)
	copy2.Object["spec"] = map[string]interface{}{
		"container":         "nginx",
		"binary":            "/usr/sbin/nginx",
		"generatedAt":       "2026-08-10T09:11:00Z",
		"historyUsed":       true,
		"podLock":           "updated2",
		"networkPolicy":     "network2",
		"patchedManifest":   "manifest2",
		"spoSeccompProfile": "seccomp2",
	}

	_, err = resource.Update(ctx, copy2, metav1.UpdateOptions{})
	if err == nil {
		t.Fatal("Update with stale resourceVersion should have failed with Conflict")
	}

	if !apierrors.IsConflict(err) {
		t.Fatalf("Expected Conflict error, got: %v (%T)", err, err)
	}

	// Verify the successful update's value is persisted
	verified, err := resource.Get(ctx, "test-proposal-3", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get after conflict: %v", err)
	}

	if verified.GetResourceVersion() != updated1.GetResourceVersion() {
		t.Logf("resourceVersion: %s (updated1) vs %s (verified)",
			updated1.GetResourceVersion(), verified.GetResourceVersion())
	}

	spec, ok := verified.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("Spec missing in verified object")
	}

	if spec["podLock"] != "updated1" {
		t.Errorf("Wrong value persisted: podLock=%v, want updated1 (from copy1)",
			spec["podLock"])
	}
}
