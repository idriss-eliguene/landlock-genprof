package proposal

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func custodyClient(t *testing.T, name, uid string, spec Spec) *dynamicfake.FakeDynamicClient {
	t.Helper()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	if err := Save(ctx, client, "default", name, spec); err != nil {
		t.Fatal(err)
	}
	gvr := schema.GroupVersionResource{Group: apiGroup, Version: apiVersion, Resource: "securityprofileproposals"}
	obj, err := client.Resource(gvr).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	obj.SetUID(typesUID(uid))
	if _, err := client.Resource(gvr).Namespace("default").Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	return client
}

// typesUID keeps the fixture helper independent of the concrete UID alias in
// the production API while still exercising the real Kubernetes identity.
func typesUID(value string) types.UID { return types.UID(value) }

func TestApprovalCustodyIsMonotonicAndUIDBound(t *testing.T) {
	ctx := context.Background()
	specA := Spec{Container: "app", Binary: "/bin/app", PodLock: "a"}
	client := custodyClient(t, "proposal", "uid-a", specA)
	digestA, err := CandidateDigest(specA)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", "proposal", ApprovalApproved, "approve A", digestA); err != nil {
		t.Fatal(err)
	}
	status, err := GetStatus(ctx, client, "default", "proposal")
	if err != nil || status.LastApprovalSnapshot == nil {
		t.Fatalf("approval custody missing: status=%+v err=%v", status, err)
	}
	if status.LastApprovalSnapshot.ProposalUID != "uid-a" || status.LastApprovalSnapshot.ApprovedCandidateDigest != digestA {
		t.Fatalf("snapshot = %+v, want uid-a and digest A", status.LastApprovalSnapshot)
	}

	if err := SetApprovalState(ctx, client, "default", "proposal", ApprovalRejected, "revoke", ""); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", "proposal")
	if err != nil || status.ApprovalState != ApprovalRejected || status.ApprovedCandidateDigest != "" || status.LastApprovalSnapshot.ApprovedCandidateDigest != digestA {
		t.Fatalf("rejection lost custody or current revocation: status=%+v err=%v", status, err)
	}
	if err := ValidateApprovedCandidate(&specA, status); err == nil {
		t.Fatal("rejected proposal retained current approval")
	}

	specB := specA
	specB.PodLock = "b"
	if err := Save(ctx, client, "default", "proposal", specB); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", "proposal")
	if err != nil || status.LastApprovalSnapshot.ApprovedCandidateDigest != digestA {
		t.Fatalf("Spec mutation changed custody: status=%+v err=%v", status, err)
	}

	digestB, err := CandidateDigest(specB)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", "proposal", ApprovalApproved, "approve B", digestB); err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", "proposal", ApprovalRejected, "revoke B", ""); err != nil {
		t.Fatal(err)
	}
	status, err = GetStatus(ctx, client, "default", "proposal")
	if err != nil || status.LastApprovalSnapshot.ApprovedCandidateDigest != digestB || status.LastApprovalSnapshot.ProposalUID != "uid-a" {
		t.Fatalf("second snapshot not preserved: status=%+v err=%v", status, err)
	}
}

func TestFailedApprovalDoesNotCreateCustody(t *testing.T) {
	client := custodyClient(t, "proposal", "uid-failure", Spec{Container: "app", Binary: "/bin/app"})
	if err := SetApprovalState(context.Background(), client, "default", "proposal", ApprovalApproved, "bad", "sha256:0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("invalid approval unexpectedly succeeded")
	}
	status, err := GetStatus(context.Background(), client, "default", "proposal")
	if err != nil || status.LastApprovalSnapshot != nil {
		t.Fatalf("failed approval created custody: status=%+v err=%v", status, err)
	}
}

func TestProposalNameReuseDoesNotTransferCustody(t *testing.T) {
	ctx := context.Background()
	client := custodyClient(t, "proposal", "uid-old", Spec{Container: "app", Binary: "/bin/app"})
	spec := Spec{Container: "app", Binary: "/bin/app"}
	digest, err := CandidateDigest(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetApprovalState(ctx, client, "default", "proposal", ApprovalApproved, "approve", digest); err != nil {
		t.Fatal(err)
	}
	gvr := schema.GroupVersionResource{Group: apiGroup, Version: apiVersion, Resource: "securityprofileproposals"}
	if err := client.Resource(gvr).Namespace("default").Delete(ctx, "proposal", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Save(ctx, client, "default", "proposal", spec); err != nil {
		t.Fatal(err)
	}
	obj, err := client.Resource(gvr).Namespace("default").Get(ctx, "proposal", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	obj.SetUID(typesUID("uid-new"))
	if _, err := client.Resource(gvr).Namespace("default").Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	status, err := GetStatus(ctx, client, "default", "proposal")
	if err != nil || status.LastApprovalSnapshot != nil {
		t.Fatalf("name reuse transferred custody: status=%+v err=%v", status, err)
	}
}
