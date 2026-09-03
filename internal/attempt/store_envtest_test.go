//go:build envtest

package attempt

import (
	"context"
	"fmt"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

var (
	attemptCfg *rest.Config
	attemptEnv *envtest.Environment
)

func TestMain(m *testing.M) {
	crdPath := "deploy/crd-applyattempt.yaml"
	if _, err := os.Stat(crdPath); err != nil {
		crdPath = "../../deploy/crd-applyattempt.yaml"
	}
	attemptEnv = &envtest.Environment{CRDInstallOptions: envtest.CRDInstallOptions{Paths: []string{crdPath}, ErrorIfPathMissing: true}}
	var err error
	attemptCfg, err = attemptEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envtest.Start: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = attemptEnv.Stop()
	os.Exit(code)
}

func attemptEnvClient(t *testing.T) dynamic.Interface {
	t.Helper()
	client, err := dynamic.NewForConfig(attemptCfg)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestApplyAttemptCRDRoundTrip(t *testing.T) {
	client := attemptEnvClient(t)
	ctx := context.Background()
	name, obj, err := Create(ctx, client, "default", Spec{
		ProposalNamespace: "default", ProposalName: "proposal", ProposalUID: "proposal-uid",
		ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Target:                  k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "web"}, Container: "app"},
		StartedAt:               "2026-09-03T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := unstructured.NestedFieldCopy(obj.Object, "status"); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatalf("Create unexpectedly persisted status inline: %#v", obj.Object["status"])
	}
	created, err := client.Resource(GVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := unstructured.NestedFieldCopy(created.Object, "status"); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatalf("normal Create unexpectedly persisted status: %#v", created.Object["status"])
	}
	if err := SaveStatus(ctx, client, "default", name, obj, Status{State: StateInProgress}); err != nil {
		t.Fatal(err)
	}
	inProgress, err := client.Resource(GVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if inProgress.Object["status"].(map[string]interface{})["state"] != StateInProgress {
		t.Fatalf("initial state = %#v, want %q", inProgress.Object["status"], StateInProgress)
	}
	if err := SaveStatus(ctx, client, "default", name, obj, Status{State: StateOutcomeUnknown, Mutations: []MutationRecord{{ID: "m1", Version: "v1", Kind: "Pod", Name: "web", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", Result: ResultUnknown}}}); err != nil {
		t.Fatal(err)
	}
	got, err := client.Resource(GVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Object["spec"].(map[string]interface{})["proposalUID"] != "proposal-uid" {
		t.Fatalf("proposal UID was not retained: %#v", got.Object["spec"])
	}
	if got.Object["status"].(map[string]interface{})["state"] != StateOutcomeUnknown {
		t.Fatalf("state = %#v, want %q", got.Object["status"].(map[string]interface{})["state"], StateOutcomeUnknown)
	}
}

func validAttemptSpec() Spec {
	return Spec{ProposalNamespace: "default", ProposalName: "immutability", ProposalUID: "proposal-uid",
		ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Target:                  k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "web"}, Container: "app"},
		StartedAt:               "2026-09-03T00:00:00Z"}
}

func TestApplyAttemptCRDEnforcesSpecAndTerminalStatusImmutability(t *testing.T) {
	client := attemptEnvClient(t)
	ctx := context.Background()
	name, obj, err := Create(ctx, client, "default", validAttemptSpec())
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveStatus(ctx, client, "default", name, obj, Status{State: StateInProgress}); err != nil {
		t.Fatal(err)
	}
	terminal := Status{State: StateApplied, Mutations: []MutationRecord{{ID: "m1", Version: "v1", Kind: "Deployment", Name: "web", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", Result: ResultSucceeded}}}
	if err := SaveStatus(ctx, client, "default", name, obj, terminal); err != nil {
		t.Fatalf("transition into terminal failed: %v", err)
	}
	if err := SaveStatus(ctx, client, "default", name, obj, Status{State: StateFailed}); err == nil {
		t.Fatal("terminal status rewrite was accepted")
	}
	current, err := client.Resource(GVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current.Object["spec"].(map[string]interface{})["proposalName"] = "changed"
	if _, err := client.Resource(GVR).Namespace("default").Update(ctx, current, metav1.UpdateOptions{}); err == nil {
		t.Fatal("ApplyAttempt Spec mutation was accepted")
	}
}

func TestRollbackAttemptCRDEnforcesCustody(t *testing.T) {
	client := attemptEnvClient(t)
	ctx := context.Background()
	name, obj, err := CreateRollback(ctx, client, "default", RollbackSpec{SourceNamespace: "default", SourceName: "source", SourceUID: "source-uid", ProposalNamespace: "default", ProposalName: "proposal", ProposalUID: "proposal-uid", ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CustodyEpoch: "0123456789abcdef0123456789abcdef", Target: k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "web"}, Container: "app"}, StartedAt: "2026-09-03T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveRollbackStatus(ctx, client, "default", name, obj, Status{State: StateInProgress}); err != nil {
		t.Fatal(err)
	}
	if err := SaveRollbackStatus(ctx, client, "default", name, obj, Status{State: StateApplied}); err != nil {
		t.Fatal(err)
	}
	if err := SaveRollbackStatus(ctx, client, "default", name, obj, Status{State: StateFailed}); err == nil {
		t.Fatal("terminal RollbackAttempt status rewrite was accepted")
	}
	current, err := client.Resource(RollbackGVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current.Object["spec"].(map[string]interface{})["sourceName"] = "changed"
	if _, err := client.Resource(RollbackGVR).Namespace("default").Update(ctx, current, metav1.UpdateOptions{}); err == nil {
		t.Fatal("RollbackAttempt Spec mutation was accepted")
	}
}

func TestCustodyEpochActivationAfterHardeningProbe(t *testing.T) {
	client := attemptEnvClient(t)
	ctx := context.Background()
	epoch, hardened, err := CustodyEpochAndHardening(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	if epoch != "" || !hardened {
		t.Fatalf("initial custody epoch/hardening = %q/%t, want empty epoch and hardened CRD", epoch, hardened)
	}
	fresh, err := NewCustodyEpoch()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PatchCustodyEpoch(ctx, client, fresh); err != nil {
		t.Fatal(err)
	}
	got, hardened, err := CustodyEpochAndHardening(ctx, client)
	if err != nil || !hardened || got != fresh {
		t.Fatalf("published custody epoch/hardening = %q/%t/%v, want %q/true/nil", got, hardened, err, fresh)
	}
}
