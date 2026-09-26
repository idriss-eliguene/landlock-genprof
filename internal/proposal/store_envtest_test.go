//go:build envtest

package proposal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

var (
	cfg *rest.Config
	env *envtest.Environment
)

func TestMain(m *testing.M) {
	// Start envtest once per package.
	// All tests in this package share a single API server instance for efficiency.

	// Determine CRD paths. The derivation integration exercises both the
	// Proposal and TrainingHistory persistence boundaries against this API.
	crdRoot := "deploy"
	if _, err := os.Stat(filepath.Join(crdRoot, "crd-securityprofileproposal.yaml")); err != nil {
		crdRoot = filepath.Join("..", "..", "deploy")
	}
	proposalCRDPath, _ := filepath.Abs(filepath.Join(crdRoot, "crd-securityprofileproposal.yaml"))
	historyCRDPath, _ := filepath.Abs(filepath.Join(crdRoot, "crd-traininghistory.yaml"))
	receiptCRDPath, _ := filepath.Abs(filepath.Join(crdRoot, "crd-observationcontributionreceipt.yaml"))

	env = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths:              []string{proposalCRDPath, historyCRDPath, receiptCRDPath},
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

func TestContainerCapabilityDerivationEnvtest(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()
	identity := history.PopulationIdentity{
		Scope:         history.ScopeContainer,
		Target:        "Deployment/derived",
		Container:     "app",
		ImageIdentity: "sha256:" + strings.Repeat("c", 64),
	}
	historyRecord := &history.Record{Populations: []history.Population{{
		Scope:         identity.Scope,
		Target:        identity.Target,
		Container:     identity.Container,
		ImageIdentity: identity.ImageIdentity,
		CapabilityAccesses: []history.CapabilityAccessRecord{
			{Name: "CAP_NET_ADMIN"},
			{Name: "CAP_CHOWN"},
		},
		ObservationContributions: []history.ObservationContribution{
			{ObservationID: "envtest-observation-a", Sources: []history.ObservationSourceContribution{{Source: "capabilities", EvidenceState: "UNKNOWN", AttributionState: "COMPLETED", AttributedCount: 1, NormalizedFactCount: 1}}, CapabilityFacts: []string{"CAP_NET_ADMIN"}},
			{ObservationID: "envtest-observation-b", Sources: []history.ObservationSourceContribution{{Source: "capabilities", EvidenceState: "UNKNOWN", AttributionState: "COMPLETED", AttributedCount: 1, NormalizedFactCount: 1}}, CapabilityFacts: []string{"CAP_CHOWN"}},
		},
	}}}
	historyName, err := history.RecordNameForPopulation(identity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Resource(securityProfileProposalGVR).Namespace("default").Delete(ctx, "derived-container-capabilities", metav1.DeleteOptions{})
		_ = client.Resource(schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "traininghistories"}).Namespace("default").Delete(ctx, historyName, metav1.DeleteOptions{})
	})
	if err := history.SavePopulationSnapshot(ctx, client, "default", identity, historyRecord); err != nil {
		t.Fatal(err)
	}
	spec, err := GenerateContainerCapabilityProposal(ctx, client, "default", identity, "derived-container-capabilities")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Subject == nil || spec.Subject.Target != identity.Target || spec.Subject.Container != identity.Container || spec.Subject.ImageIdentity != identity.ImageIdentity {
		t.Fatalf("derived subject = %#v", spec.Subject)
	}
	if spec.Qualification.Capabilities != "UNKNOWN" || len(spec.Provenance.ObservationIDs) != 2 || spec.Provenance.ObservationIDs[0] != "envtest-observation-a" || spec.Provenance.ObservationIDs[1] != "envtest-observation-b" {
		t.Fatalf("derived review context = %#v %#v", spec.Qualification, spec.Provenance)
	}
	if len(spec.Provenance.CapabilityAttribution) != 2 || spec.Provenance.CapabilityAttribution[0].Capability != "CAP_CHOWN" || spec.Provenance.CapabilityAttribution[0].State != CapabilityAttributionUnknown || spec.Provenance.CapabilityAttribution[0].ObservationIDs[0] != "envtest-observation-b" || spec.Provenance.CapabilityAttribution[1].Capability != "CAP_NET_ADMIN" || spec.Provenance.CapabilityAttribution[1].ObservationIDs[0] != "envtest-observation-a" {
		t.Fatalf("per-capability attribution was not persisted and reloaded: %#v", spec.Provenance.CapabilityAttribution)
	}
	got, err := Get(ctx, client, "default", "derived-container-capabilities")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := got.CandidateV2(); err != nil {
		t.Fatalf("round-trip candidate: %v", err)
	}
	status, err := GetStatus(ctx, client, "default", "derived-container-capabilities")
	if err != nil || status.ApprovalState != ApprovalDraft || status.LastApprovalSnapshot != nil {
		t.Fatalf("generation changed governance status: %+v, err=%v", status, err)
	}
}

func TestIntegratedObservationHistoryProposalEnvtest(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()
	image := "sha256:" + strings.Repeat("d", 64)
	identity := history.PopulationIdentity{Scope: history.ScopeContainer, Target: "Deployment/integrated", Container: "app", ImageIdentity: image}
	historyName, err := history.RecordNameForPopulation(identity)
	if err != nil {
		t.Fatal(err)
	}
	proposalName := "integrated-container-proposal"
	t.Cleanup(func() {
		_ = client.Resource(securityProfileProposalGVR).Namespace("default").Delete(ctx, proposalName, metav1.DeleteOptions{})
		_ = client.Resource(schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "traininghistories"}).Namespace("default").Delete(ctx, historyName, metav1.DeleteOptions{})
	})

	for _, observation := range []observationdomain.Observation{
		integratedObservation(t, "integrated-a", "Deployment/integrated", "app", image, "CAP_CHOWN"),
		integratedObservation(t, "integrated-b", "Deployment/integrated", "app", image, "CAP_NET_ADMIN"),
	} {
		if _, err := history.ApplyObservationContribution(ctx, client, "default", observation); err != nil {
			t.Fatal(err)
		}
	}
	spec, err := GenerateContainerCapabilityProposal(ctx, client, "default", identity, proposalName)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := spec.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := CandidateDigestV2(candidate)
	if err != nil {
		t.Fatal(err)
	}
	review, err := spec.ReviewContextV2()
	if err != nil {
		t.Fatal(err)
	}
	reviewDigest, err := ReviewContextDigestV2(review)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Qualification.Capabilities != "UNKNOWN" || len(spec.Provenance.ObservationIDs) != 2 || len(candidate.Artifact.ContainerCapabilities.Add) != 2 {
		t.Fatalf("integrated derivation = subject=%#v artifact=%#v provenance=%#v qualification=%#v", candidate.Subject, candidate.Artifact, spec.Provenance, spec.Qualification)
	}
	if err := SetApprovalState(ctx, client, "default", proposalName, ApprovalApproved, "integrated approval", digest); err != nil {
		t.Fatal(err)
	}
	approved, err := GetStatus(ctx, client, "default", proposalName)
	if err != nil || approved.ApprovedCandidateDigest != digest || approved.ApprovedReviewContextDigest != reviewDigest || approved.LastApprovalSnapshot == nil {
		t.Fatalf("integrated approval = %+v, err=%v", approved, err)
	}
	current, err := Get(ctx, client, "default", proposalName)
	if err != nil || ValidateApprovedCandidate(current, approved) != nil {
		t.Fatalf("integrated current authority invalid: spec=%#v status=%#v err=%v", current, approved, err)
	}

	// A third contribution with an existing capability changes only the
	// provenance snapshot, not the candidate artifact.
	if _, err := history.ApplyObservationContribution(ctx, client, "default", integratedObservation(t, "integrated-c", "Deployment/integrated", "app", image, "CAP_CHOWN")); err != nil {
		t.Fatal(err)
	}
	regenerated, err := GenerateContainerCapabilityProposal(ctx, client, "default", identity, proposalName)
	if err != nil {
		t.Fatal(err)
	}
	regeneratedCandidate, err := regenerated.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	regeneratedReview, err := regenerated.ReviewContextV2()
	if err != nil {
		t.Fatal(err)
	}
	regeneratedDigest, err := CandidateDigestV2(regeneratedCandidate)
	if err != nil {
		t.Fatal(err)
	}
	regeneratedReviewDigest, err := ReviewContextDigestV2(regeneratedReview)
	if err != nil {
		t.Fatal(err)
	}
	if regeneratedDigest != digest || regeneratedReviewDigest == reviewDigest {
		t.Fatalf("review-only regeneration digests = candidate %s/%s review %s/%s", regeneratedDigest, digest, regeneratedReviewDigest, reviewDigest)
	}
	stale, err := GetStatus(ctx, client, "default", proposalName)
	if err != nil || stale.ApprovedCandidateDigest != digest || stale.ApprovedReviewContextDigest != reviewDigest || ValidateApprovedCandidate(&regenerated, stale) == nil {
		t.Fatalf("review-only regeneration authority/custody = status=%#v err=%v", stale, err)
	}
}

func integratedObservation(t *testing.T, id, workloadName, container, image, capability string) observationdomain.Observation {
	t.Helper()
	cluster, err := observationdomain.NewClusterIdentity("integrated-cluster")
	if err != nil {
		t.Fatal(err)
	}
	workload := observationdomain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: observationdomain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: strings.TrimPrefix(workloadName, "Deployment/"), UID: "integrated-workload-uid"}
	slot := observationdomain.ContainerSlot{Workload: workload, Container: container}
	spec, err := observationdomain.NewObservationSpec(observationdomain.RequestedTarget{Slot: slot}, []string{"capabilities"}, time.Minute, "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := observationdomain.NewContainerImageRevision(slot, image)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := observationdomain.NewResolvedTargetSet([]observationdomain.RuntimeContainerInstance{{Slot: slot, PodUID: id + "-pod", ContainerID: id + "-container"}})
	if err != nil {
		t.Fatal(err)
	}
	qualification := observationdomain.SourceQualification{SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: observationdomain.AttributionCompleted, AttributedCount: 1}
	result, err := observationdomain.NewSourceResult(observationdomain.EvidenceSource{Name: "capabilities", Backend: "test", Version: "v1"}, qualification, nil, observationdomain.NormalizedFacts{Capabilities: []observationdomain.CapabilityFact{{Name: capability}}})
	if err != nil {
		t.Fatal(err)
	}
	observationResult, err := observationdomain.NewObservationResult([]observationdomain.SourceResult{result})
	if err != nil {
		t.Fatal(err)
	}
	binding := observationdomain.ObservationBinding{ResolvedTargets: targets, Backend: observationdomain.BackendIdentity{Kind: "test", Version: "v1"}, ImageRevisions: []observationdomain.ContainerImageRevision{revision}}
	provenance := observationdomain.ObservationProvenance{ResolvedTargets: targets, ImageRevisions: []observationdomain.ContainerImageRevision{revision}, Backend: binding.Backend, RequestedSources: []string{"capabilities"}}
	observation, err := observationdomain.RestoreObservation(observationdomain.ObservationID(id), spec, binding, observationdomain.ObservationExecution{State: observationdomain.ExecutionCompleted, Completion: observationdomain.CompletedNormally}, observationResult, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return observation
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

func TestApprovalCustodyEnvtestLifecycle(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()
	name := "approval-custody-lifecycle"
	specA := Spec{Container: "app", Binary: "/bin/app", PodLock: "candidate-a"}
	t.Cleanup(func() {
		_ = client.Resource(securityProfileProposalGVR).Namespace("default").Delete(ctx, name, metav1.DeleteOptions{})
	})

	if err := Save(ctx, client, "default", name, specA); err != nil {
		t.Fatal(err)
	}
	status, err := GetStatus(ctx, client, "default", name)
	if err != nil || status.LastApprovalSnapshot != nil {
		t.Fatalf("new proposal custody = %+v, err=%v; want absent", status, err)
	}
	digestA, err := CandidateDigest(specA)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", name, ApprovalApproved, "approve A", digestA); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", name)
	if err != nil || status.LastApprovalSnapshot == nil || status.LastApprovalSnapshot.ProposalUID == "" || status.LastApprovalSnapshot.ApprovedCandidateDigest != digestA {
		t.Fatalf("approved custody = %+v, err=%v", status, err)
	}
	if err := SetApprovalState(ctx, client, "default", name, ApprovalRejected, "reject A", ""); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", name)
	if err != nil || status.ApprovalState != ApprovalRejected || status.ApprovedCandidateDigest != "" || status.LastApprovalSnapshot.ApprovedCandidateDigest != digestA {
		t.Fatalf("rejected custody = %+v, err=%v", status, err)
	}
	specB := specA
	specB.PodLock = "candidate-b"
	if err := Save(ctx, client, "default", name, specB); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", name)
	if err != nil || status.LastApprovalSnapshot.ApprovedCandidateDigest != digestA {
		t.Fatalf("mutated-spec custody = %+v, err=%v", status, err)
	}
	digestB, err := CandidateDigest(specB)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", name, ApprovalApproved, "approve B", digestB); err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", name, ApprovalRejected, "reject B", ""); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", name)
	if err != nil || status.LastApprovalSnapshot.ApprovedCandidateDigest != digestB || status.LastApprovalSnapshot.ProposalUID == "" {
		t.Fatalf("latest custody = %+v, err=%v", status, err)
	}
}

func TestCandidateV2ProposalEnvtest(t *testing.T) {
	client := setupEnvtest(t)
	ctx := context.Background()
	name := "candidate-v2-governance"
	spec := v2SpecFixture()
	t.Cleanup(func() {
		_ = client.Resource(securityProfileProposalGVR).Namespace("default").Delete(ctx, name, metav1.DeleteOptions{})
	})

	if err := Save(ctx, client, "default", name, spec); err != nil {
		t.Fatal(err)
	}
	got, err := Get(ctx, client, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := got.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := CandidateDigestV2(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if digest != "sha256:46062013486c3c47ba3d092d002fa12eb86eeb2019eafcd8b1e51805a9e32609" {
		t.Fatalf("v2 digest anchor = %s", digest)
	}
	review, err := got.ReviewContextV2()
	if err != nil {
		t.Fatal(err)
	}
	reviewDigest, err := ReviewContextDigestV2(review)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", name, ApprovalApproved, "approve v2", digest); err != nil {
		t.Fatal(err)
	}
	status, err := GetStatus(ctx, client, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if status.ApprovedReviewContextDigest != reviewDigest || status.LastApprovalSnapshot == nil || status.LastApprovalSnapshot.ReviewContextDigest != reviewDigest {
		t.Fatalf("v2 dual snapshot = %+v", status)
	}
	if err := ValidateApprovedCandidate(got, status); err != nil {
		t.Fatalf("v2 validation = %v", err)
	}

	mutated := *got
	mutated.Provenance = &ProposalProvenance{PopulationScope: CandidateV2ScopeContainer, ObservationIDs: []string{"later"}}
	if err := Save(ctx, client, "default", name, mutated); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateApprovedCandidate(&mutated, status); err == nil {
		t.Fatal("v2 review-context mutation remained authorized")
	}
	if status.LastApprovalSnapshot.ReviewContextDigest != reviewDigest {
		t.Fatal("v2 custody snapshot mutated")
	}
	if err := SetApprovalState(ctx, client, "default", name, ApprovalRejected, "reject v2", ""); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", name)
	if err != nil || status.LastApprovalSnapshot == nil || status.LastApprovalSnapshot.ReviewContextDigest != reviewDigest {
		t.Fatalf("v2 rejection custody = %+v, err=%v", status, err)
	}
	if err := ValidateProposalSpec(Spec{CandidateVersion: CandidateVersionV2}); err == nil {
		t.Fatal("malformed v2 accepted")
	}
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
