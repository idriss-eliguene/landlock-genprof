package attempt

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

func TestCreateAndSaveStatusRoundTrip(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	name, obj, err := Create(ctx, client, "default", Spec{
		ProposalNamespace: "default", ProposalName: "proposal", ProposalUID: "proposal-uid",
		ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Target:                  k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "app"}, Container: "app"}, StartedAt: "2026-09-03T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if name == "" || obj.GetName() != name {
		t.Fatalf("attempt identity = %q, object name = %q", name, obj.GetName())
	}
	if _, found, err := unstructured.NestedFieldCopy(obj.Object, "status"); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatalf("Create unexpectedly included inline status: %#v", obj.Object["status"])
	}
	status := Status{State: StateApplied, Mutations: []MutationRecord{{
		ID: "networkpolicy", Version: "v1", Kind: "NetworkPolicy", Namespace: "default", Name: "app",
		Operation: "CREATE", Before: "null", IntendedAfter: `{"kind":"NetworkPolicy"}`,
		Result: ResultSucceeded,
	}}}
	if err := SaveStatus(ctx, client, "default", name, obj, status); err != nil {
		t.Fatal(err)
	}
	got, err := client.Resource(GVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gotStatus, found, err := unstructured.NestedMap(got.Object, "status")
	if err != nil || !found {
		t.Fatalf("status missing: found=%t err=%v", found, err)
	}
	if gotStatus["state"] != StateApplied {
		t.Fatalf("state = %#v, want %q", gotStatus["state"], StateApplied)
	}
	if obj.GetResourceVersion() != got.GetResourceVersion() {
		t.Fatalf("status save did not carry forward resourceVersion: object=%q stored=%q", obj.GetResourceVersion(), got.GetResourceVersion())
	}
}

func TestStatusValidateLifecycleAndOutcomeUnknown(t *testing.T) {
	valid := []string{StateInProgress, StateApplied, StatePartiallyApplied, StateFailed, StateOutcomeUnknown}
	for _, state := range valid {
		if err := (Status{State: state}).Validate(); err != nil {
			t.Errorf("state %q rejected: %v", state, err)
		}
	}
	if err := (Status{State: "SECURE"}).Validate(); err == nil {
		t.Fatal("decorative lifecycle state accepted")
	}
	if err := (Status{State: StateOutcomeUnknown, Mutations: []MutationRecord{{Result: ResultUnknown}}}).Validate(); err != nil {
		t.Fatalf("OUTCOME_UNKNOWN rejected: %v", err)
	}
	if err := (Status{State: StateFailed, Mutations: []MutationRecord{{Result: "APPLIED"}}}).Validate(); err == nil {
		t.Fatal("unknown mutation result accepted")
	}
}

func TestMultipleAttemptsRemainIndependent(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	spec := Spec{
		ProposalNamespace: "default", ProposalName: "proposal", ProposalUID: "proposal-uid",
		ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Target:                  k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "app"}, Container: "app"},
		StartedAt:               "2026-09-03T00:00:00Z",
	}
	first, firstObj, err := Create(ctx, client, "default", spec)
	if err != nil {
		t.Fatal(err)
	}
	second, secondObj, err := Create(ctx, client, "default", spec)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || firstObj.GetName() == secondObj.GetName() {
		t.Fatalf("attempt names collide: %q and %q", first, second)
	}
	if err := SaveStatus(ctx, client, "default", first, firstObj, Status{State: StateFailed, Mutations: []MutationRecord{{ID: "first", Version: "v1", Kind: "Pod", Name: "app", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", Result: ResultFailed}}}); err != nil {
		t.Fatal(err)
	}
	firstLoaded, err := client.Resource(GVR).Namespace("default").Get(ctx, first, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	secondLoaded, err := client.Resource(GVR).Namespace("default").Get(ctx, second, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if firstLoaded.Object["status"].(map[string]interface{})["state"] != StateFailed {
		t.Fatalf("first state changed unexpectedly: %#v", firstLoaded.Object["status"])
	}
	if _, found, _ := unstructured.NestedFieldCopy(secondLoaded.Object, "status", "mutations"); found {
		t.Fatalf("second attempt received first attempt's mutation record: %#v", secondLoaded.Object["status"])
	}
}
