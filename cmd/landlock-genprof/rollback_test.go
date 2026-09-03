package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
)

func rollbackTestClient(t *testing.T, mutation attempt.MutationRecord, epoch string) *dynamicfake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	specMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&attempt.Spec{
		ProposalNamespace: "default", ProposalName: "proposal", ProposalUID: "proposal-uid",
		ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Target:                  k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "web"}, Container: "app"},
		StartedAt:               "2026-09-03T00:00:00Z", CustodyEpoch: epoch,
	})
	if err != nil {
		t.Fatal(err)
	}
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&attempt.Status{State: attempt.StateApplied, Mutations: []attempt.MutationRecord{mutation}})
	if err != nil {
		t.Fatal(err)
	}
	source := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "ApplyAttempt",
		"metadata": map[string]interface{}{"name": "source", "namespace": "default", "uid": "source-uid", "resourceVersion": "1"},
		"spec":     specMap, "status": statusMap,
	}}
	crd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
		"metadata": map[string]interface{}{"name": "applyattempts.landlockgenprof.io", "annotations": map[string]interface{}{attempt.CustodyEpochAnnotation: epoch}},
		"spec":     map[string]interface{}{"versions": []interface{}{map[string]interface{}{"schema": map[string]interface{}{"openAPIV3Schema": map[string]interface{}{"properties": map[string]interface{}{"spec": map[string]interface{}{"x-kubernetes-validations": []interface{}{map[string]interface{}{"rule": "self == oldSelf"}}}, "status": map[string]interface{}{"x-kubernetes-validations": []interface{}{map[string]interface{}{"rule": "terminal"}}}}}}}}},
	}}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{attempt.RollbackGVR: "RollbackAttemptList"}, source, crd)
	return client
}

func TestRunRollbackRefusesBarePodBeforeCustody(t *testing.T) {
	client := rollbackTestClient(t, attempt.MutationRecord{ID: "pod", Kind: "Pod", Version: "v1", Name: "web", Operation: "DELETE_THEN_CREATE", Before: "{}", IntendedAfter: "{}", Result: attempt.ResultSucceeded}, "0123456789abcdef0123456789abcdef")
	oldClient, oldCreate := newDynamicClientForRollback, createRollbackAttempt
	created := 0
	newDynamicClientForRollback = func() (dynamic.Interface, error) { return client, nil }
	createRollbackAttempt = func(context.Context, dynamic.Interface, string, attempt.RollbackSpec) (string, *unstructured.Unstructured, error) {
		created++
		return "", nil, nil
	}
	t.Cleanup(func() { newDynamicClientForRollback, createRollbackAttempt = oldClient, oldCreate })
	if err := runRollback(context.Background(), &bytes.Buffer{}, bytes.NewBufferString(""), rollbackOptions{namespace: "default", yes: true}, "source"); err == nil {
		t.Fatal("runRollback() error = nil, want bare-Pod refusal")
	}
	if created != 0 {
		t.Fatalf("RollbackAttempt creations = %d, want 0", created)
	}
}

func TestRunRollbackInitialStatusFailurePreventsInverse(t *testing.T) {
	object := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "web", "namespace": "default", "uid": "uid-1", "resourceVersion": "7"}, "spec": map[string]interface{}{"replicas": int64(1)}}}
	record := attempt.MutationRecord{ID: "m1", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid-1", AttributableAfterRV: "7", Operation: "UPDATE", Before: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid-1","resourceVersion":"6"},"spec":{"replicas":1}}`, IntendedAfter: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid-1","resourceVersion":"7"},"spec":{"replicas":2}}`, ObservedAfter: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid-1","resourceVersion":"7"},"spec":{"replicas":2}}`, Result: attempt.ResultSucceeded}
	client := rollbackTestClient(t, record, "0123456789abcdef0123456789abcdef")
	if _, err := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	oldClient, oldSave := newDynamicClientForRollback, saveRollbackAttemptStatus
	saves := 0
	newDynamicClientForRollback = func() (dynamic.Interface, error) { return client, nil }
	saveRollbackAttemptStatus = func(context.Context, dynamic.Interface, string, string, *unstructured.Unstructured, attempt.Status) error {
		saves++
		return errors.New("injected status failure")
	}
	t.Cleanup(func() { newDynamicClientForRollback, saveRollbackAttemptStatus = oldClient, oldSave })
	if err := runRollback(context.Background(), &bytes.Buffer{}, bytes.NewBufferString(""), rollbackOptions{namespace: "default", yes: true}, "source"); err == nil {
		t.Fatal("runRollback() error = nil, want initial status failure")
	}
	if saves != 1 {
		t.Fatalf("status writes = %d, want one failed initial write and no later writes", saves)
	}
}

func TestRollbackStrictRVRefusesInterveningWrite(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	current := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "web", "namespace": "default", "uid": "uid-1", "resourceVersion": "8"},
		"spec":     map[string]interface{}{"replicas": int64(2)},
	}}
	if _, err := client.Resource(gvr).Namespace("default").Create(context.Background(), current, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	record := attempt.MutationRecord{
		Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web",
		UID: "uid-1", AttributableAfterRV: "7", Operation: "UPDATE",
		Before:        `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid-1","resourceVersion":"6"},"spec":{"replicas":1}}`,
		ObservedAfter: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid-1","resourceVersion":"7"},"spec":{"replicas":2}}`,
	}
	err := executeInverse(context.Background(), client, &record, k8s.GovernedTarget{})
	if err == nil {
		t.Fatal("executeInverse() error = nil, want strict resourceVersion refusal")
	}
	got, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Object["spec"].(map[string]interface{})["replicas"] != int64(2) {
		t.Fatalf("object changed after stale rollback: %#v", got.Object["spec"])
	}
}

func TestRollbackCreatedObjectConfirmsDelete(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "web", "namespace": "default", "uid": "uid-1", "resourceVersion": "7"},
		"spec":     map[string]interface{}{"replicas": int64(1)},
	}}
	if _, err := client.Resource(gvr).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	record := attempt.MutationRecord{
		Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web",
		AttributableAfterRV: "7", Operation: "CREATE",
		ObservedAfter: string(mutationSnapshot(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, object)),
	}
	if err := executeInverse(context.Background(), client, &record, k8s.GovernedTarget{}); err != nil {
		t.Fatal(err)
	}
	if record.Result != attempt.ResultSucceeded {
		t.Fatalf("inverse result = %q, want %q", record.Result, attempt.ResultSucceeded)
	}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "web", metav1.GetOptions{}); err == nil {
		t.Fatal("created object still exists after confirmed inverse delete")
	}
}

func TestMergeWorkloadBeforeRemovesControlledFieldsAndPreservesOthers(t *testing.T) {
	object := map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{
			"metadata": map[string]interface{}{"labels": map[string]interface{}{"landlockgenprof.io/podlock-profile": "old", "unrelated": "keep"}},
			"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{
				"name": "app", "image": "current", "securityContext": map[string]interface{}{
					"capabilities": map[string]interface{}{"drop": []interface{}{"ALL"}}, "privileged": true,
				},
			}},
			}}},
	}
	before := map[string]interface{}{"template": map[string]interface{}{
		"metadata": map[string]interface{}{"labels": map[string]interface{}{}},
		"spec":     map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "app"}}},
	}}
	if err := mergeWorkloadBefore(object, before); err != nil {
		t.Fatal(err)
	}
	template, _, _ := unstructured.NestedMap(object, "spec", "template")
	labels, _, _ := unstructured.NestedStringMap(template, "metadata", "labels")
	if labels["unrelated"] != "keep" || labels[podLockProfileLabel] != "" {
		t.Fatalf("labels = %#v, want unrelated label preserved and controlled label removed", labels)
	}
	containers, _, _ := unstructured.NestedSlice(template, "spec", "containers")
	sc := containers[0].(map[string]interface{})["securityContext"].(map[string]interface{})
	if _, ok := sc["capabilities"]; ok || sc["privileged"] != true {
		t.Fatalf("securityContext = %#v, want controlled fields removed and unrelated field preserved", sc)
	}
}

func TestRollbackHistoryTracksIndependentSourceMutations(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{attempt.RollbackGVR: "RollbackAttemptList"})
	spec := attempt.RollbackSpec{SourceNamespace: "default", SourceName: "a", SourceUID: "source", ProposalNamespace: "default", ProposalName: "p", ProposalUID: "puid", ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CustodyEpoch: "0123456789abcdef0123456789abcdef", StartedAt: "now", Target: k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "web"}, Container: "app"}}
	name, object, err := attempt.CreateRollback(context.Background(), client, "default", spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.SaveRollbackStatus(context.Background(), client, "default", name, object, attempt.Status{State: attempt.StateApplied, Mutations: []attempt.MutationRecord{{ID: "m1", SourceMutationID: "m1", Result: attempt.ResultSucceeded}}}); err != nil {
		t.Fatal(err)
	}
	completed, unknown, previous, err := rollbackHistory(context.Background(), client, "default", "source")
	if err != nil || previous == nil {
		t.Fatalf("rollbackHistory() = (%v, %v), want a prior attempt", err, previous)
	}
	if !completed["m1"] {
		t.Fatalf("completed mutations = %#v, want m1", completed)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown mutations = %#v, want none", unknown)
	}
}

func TestInverseFailureClassificationDistinguishesDispatch(t *testing.T) {
	if got := inverseFailureResult(preDispatchFailure(errors.New("guard"))); got != attempt.ResultFailed {
		t.Fatalf("pre-dispatch result = %q, want FAILED", got)
	}
	if got := inverseFailureResult(postDispatchFailure(errors.New("lost response"))); got != attempt.ResultUnknown {
		t.Fatalf("post-dispatch result = %q, want OUTCOME_UNKNOWN", got)
	}
}

func TestRollbackHistoryBlocksUnknownDescendant(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{attempt.RollbackGVR: "RollbackAttemptList"})
	spec := attempt.RollbackSpec{SourceNamespace: "default", SourceName: "a", SourceUID: "source", ProposalNamespace: "default", ProposalName: "p", ProposalUID: "puid", ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CustodyEpoch: "0123456789abcdef0123456789abcdef", StartedAt: "now", Target: k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "web"}, Container: "app"}}
	name, object, err := attempt.CreateRollback(context.Background(), client, "default", spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.SaveRollbackStatus(context.Background(), client, "default", name, object, attempt.Status{State: attempt.StateOutcomeUnknown, Mutations: []attempt.MutationRecord{{ID: "m1-inverse", SourceMutationID: "m1", Version: "v1", Kind: "Deployment", Name: "web", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", Result: attempt.ResultUnknown}}}); err != nil {
		t.Fatal(err)
	}
	_, unknown, _, err := rollbackHistory(context.Background(), client, "default", "source")
	if err != nil || !unknown["m1"] {
		t.Fatalf("unknown descendants = %#v, err=%v; want m1 blocked", unknown, err)
	}
}

func TestSeccompReferenceIdentity(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	workload := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "web", "namespace": "default"},
		"spec": map[string]interface{}{"template": map[string]interface{}{
			"spec": map[string]interface{}{"containers": []interface{}{
				map[string]interface{}{"name": "app", "securityContext": map[string]interface{}{
					"seccompProfile": map[string]interface{}{"type": "Localhost", "localhostProfile": spobackend.LocalhostProfilePath("profile-a")},
				}},
			}},
		}},
	}}
	if _, err := client.Resource(gvr).Namespace("default").Create(context.Background(), workload, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	base := attempt.MutationRecord{Kind: "SeccompProfile", Name: "profile-a", ObservedAfter: `{"spec":{"defaultAction":"SCMP_ACT_ERRNO"}}`}
	target := k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "web"}}
	if !targetReferencesPolicy(context.Background(), &base, target, client) {
		t.Fatal("same SeccompProfile reference did not block deletion")
	}
	different := base
	different.Name = "profile-b"
	if targetReferencesPolicy(context.Background(), &different, target, client) {
		t.Fatal("different SeccompProfile reference falsely blocked deletion")
	}
	noReference := workload.DeepCopy()
	delete(noReference.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})[0].(map[string]interface{}), "securityContext")
	if _, err := client.Resource(gvr).Namespace("default").Update(context.Background(), noReference, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if targetReferencesPolicy(context.Background(), &base, target, client) {
		t.Fatal("no SeccompProfile reference blocked deletion")
	}
}

func TestCustodyEpochActivationRefusesUnhardenedCRDWithoutPublishing(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
		"metadata": map[string]interface{}{"name": "applyattempts.landlockgenprof.io", "annotations": map[string]interface{}{attempt.CustodyEpochAnnotation: "old-epoch"}},
		"spec":     map[string]interface{}{"versions": []interface{}{map[string]interface{}{"schema": map[string]interface{}{"openAPIV3Schema": map[string]interface{}{"properties": map[string]interface{}{}}}}}},
		"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Established", "status": "True"}}},
	}})
	old := newDynamicClientForCustodyEpoch
	newDynamicClientForCustodyEpoch = func() (dynamic.Interface, error) { return client, nil }
	t.Cleanup(func() { newDynamicClientForCustodyEpoch = old })
	if err := runCustodyEpochActivate(context.Background(), &bytes.Buffer{}); err == nil {
		t.Fatal("runCustodyEpochActivate() error = nil, want hardening refusal")
	}
	crd, err := client.Resource(attempt.CRDGVR).Get(context.Background(), "applyattempts.landlockgenprof.io", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := crd.GetAnnotations()[attempt.CustodyEpochAnnotation]; got != "old-epoch" {
		t.Fatalf("custody epoch = %q, want unchanged old epoch", got)
	}
}

func TestRunRollbackEpochMismatchBeforeCustody(t *testing.T) {
	record := attempt.MutationRecord{ID: "m1", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid-1", AttributableAfterRV: "7", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", ObservedAfter: "{}", Result: attempt.ResultSucceeded}
	client := rollbackTestClient(t, record, "")
	oldClient, oldCreate := newDynamicClientForRollback, createRollbackAttempt
	created := 0
	newDynamicClientForRollback = func() (dynamic.Interface, error) { return client, nil }
	createRollbackAttempt = func(context.Context, dynamic.Interface, string, attempt.RollbackSpec) (string, *unstructured.Unstructured, error) {
		created++
		return "", nil, nil
	}
	t.Cleanup(func() { newDynamicClientForRollback, createRollbackAttempt = oldClient, oldCreate })
	if err := runRollback(context.Background(), &bytes.Buffer{}, bytes.NewBuffer(nil), rollbackOptions{namespace: "default", yes: true}, "source"); err == nil {
		t.Fatal("runRollback() error = nil, want epoch mismatch")
	}
	if created != 0 {
		t.Fatalf("RollbackAttempt creations = %d, want 0", created)
	}
}

func TestRunRollbackAlreadyRolledBackBeforeNewCustody(t *testing.T) {
	record := attempt.MutationRecord{ID: "m1", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid-1", AttributableAfterRV: "7", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", ObservedAfter: "{}", Result: attempt.ResultSucceeded}
	client := rollbackTestClient(t, record, "current-epoch-0123456789abcdef0123456789")
	rbSpec := attempt.RollbackSpec{SourceNamespace: "default", SourceName: "source", SourceUID: "source-uid", ProposalNamespace: "default", ProposalName: "proposal", ProposalUID: "proposal-uid", ApprovedCandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CustodyEpoch: "current-epoch-0123456789abcdef0123456789", StartedAt: "now", Target: k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "web"}, Container: "app"}}
	name, object, err := attempt.CreateRollback(context.Background(), client, "default", rbSpec)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.SaveRollbackStatus(context.Background(), client, "default", name, object, attempt.Status{State: attempt.StateApplied, Mutations: []attempt.MutationRecord{{ID: "m1-inverse", SourceMutationID: "m1", Version: "v1", Kind: "Deployment", Name: "web", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", Result: attempt.ResultSucceeded}}}); err != nil {
		t.Fatal(err)
	}
	oldClient, oldCreate := newDynamicClientForRollback, createRollbackAttempt
	created := 0
	newDynamicClientForRollback = func() (dynamic.Interface, error) { return client, nil }
	createRollbackAttempt = func(context.Context, dynamic.Interface, string, attempt.RollbackSpec) (string, *unstructured.Unstructured, error) {
		created++
		return "", nil, nil
	}
	t.Cleanup(func() { newDynamicClientForRollback, createRollbackAttempt = oldClient, oldCreate })
	if err := runRollback(context.Background(), &bytes.Buffer{}, bytes.NewBuffer(nil), rollbackOptions{namespace: "default", yes: true}, "source"); err == nil {
		t.Fatal("runRollback() error = nil, want already-rolled-back refusal")
	}
	if created != 0 {
		t.Fatalf("RollbackAttempt creations = %d, want 0", created)
	}
}

func TestRunRollbackPreMutationPersistenceFailurePreventsInverse(t *testing.T) {
	record := attempt.MutationRecord{ID: "m1", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid-1", AttributableAfterRV: "7", Operation: "UPDATE", Before: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid-1","resourceVersion":"6"},"spec":{"replicas":1}}`, IntendedAfter: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid-1","resourceVersion":"7"},"spec":{"replicas":2}}`, ObservedAfter: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default","uid":"uid-1","resourceVersion":"7"},"spec":{"replicas":2}}`, Result: attempt.ResultSucceeded}
	client := rollbackTestClient(t, record, "current-epoch-0123456789abcdef0123456789")
	object := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "web", "namespace": "default", "uid": "uid-1", "resourceVersion": "7"}, "spec": map[string]interface{}{"replicas": int64(2)}}}
	if _, err := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	oldClient, oldSave := newDynamicClientForRollback, saveRollbackAttemptStatus
	saves := 0
	newDynamicClientForRollback = func() (dynamic.Interface, error) { return client, nil }
	saveRollbackAttemptStatus = func(_ context.Context, _ dynamic.Interface, _ string, _ string, _ *unstructured.Unstructured, _ attempt.Status) error {
		saves++
		if saves == 2 {
			return errors.New("injected pre-mutation custody failure")
		}
		return nil
	}
	t.Cleanup(func() { newDynamicClientForRollback, saveRollbackAttemptStatus = oldClient, oldSave })
	if err := runRollback(context.Background(), &bytes.Buffer{}, bytes.NewBuffer(nil), rollbackOptions{namespace: "default", yes: true}, "source"); err == nil {
		t.Fatal("runRollback() error = nil, want pre-mutation persistence failure")
	}
	if saves != 2 {
		t.Fatalf("status writes = %d, want initial plus one failed pre-mutation write", saves)
	}
	got, err := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Object["spec"].(map[string]interface{})["replicas"] != int64(2) {
		t.Fatal("inverse mutation reached target after custody persistence failure")
	}
}

func TestRunRollbackPartialApplicationPreservesPriorSuccess(t *testing.T) {
	record := attempt.MutationRecord{ID: "m1", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid-1", AttributableAfterRV: "7", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", ObservedAfter: "{}", Result: attempt.ResultSucceeded}
	client := rollbackTestClient(t, record, "current-epoch-0123456789abcdef0123456789")
	addRollbackTarget(t, client, "web", "uid-1", "7")
	addRollbackTarget(t, client, "web-2", "uid-2", "8")
	record.ObservedAfter = rollbackTargetSnapshot(t, client, "web")
	record2 := attempt.MutationRecord{ID: "m2", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web-2", UID: "uid-2", AttributableAfterRV: "8", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", ObservedAfter: rollbackTargetSnapshot(t, client, "web-2"), Result: attempt.ResultSucceeded}
	setRollbackSourceMutations(t, client, []attempt.MutationRecord{record, record2})
	oldClient, oldCreate, oldSave, oldExecute := newDynamicClientForRollback, createRollbackAttempt, saveRollbackAttemptStatus, executeInverseForRollback
	var rbName string
	var final attempt.Status
	calls := 0
	newDynamicClientForRollback = func() (dynamic.Interface, error) { return client, nil }
	createRollbackAttempt = func(ctx context.Context, c dynamic.Interface, ns string, spec attempt.RollbackSpec) (string, *unstructured.Unstructured, error) {
		name, obj, err := oldCreate(ctx, c, ns, spec)
		rbName = name
		return name, obj, err
	}
	saveRollbackAttemptStatus = func(ctx context.Context, c dynamic.Interface, ns, name string, obj *unstructured.Unstructured, status attempt.Status) error {
		final = status
		return oldSave(ctx, c, ns, name, obj, status)
	}
	executeInverseForRollback = func(_ context.Context, _ dynamic.Interface, record *attempt.MutationRecord, _ k8s.GovernedTarget) error {
		calls++
		if calls == 1 {
			record.Result = attempt.ResultSucceeded
			return nil
		}
		record.Result = attempt.ResultFailed
		return preDispatchFailure(errors.New("CAS conflict"))
	}
	t.Cleanup(func() {
		newDynamicClientForRollback, createRollbackAttempt, saveRollbackAttemptStatus, executeInverseForRollback = oldClient, oldCreate, oldSave, oldExecute
	})
	if err := runRollback(context.Background(), &bytes.Buffer{}, bytes.NewBuffer(nil), rollbackOptions{namespace: "default", yes: true}, "source"); err == nil {
		t.Fatal("runRollback() error = nil, want partial rollback failure")
	}
	if calls != 2 || final.State != attempt.StatePartiallyApplied || len(final.Mutations) != 2 || final.Mutations[0].Result != attempt.ResultSucceeded || final.Mutations[1].Result != attempt.ResultFailed || rbName == "" {
		t.Fatalf("calls=%d state=%q mutations=%#v rollback=%q, want preserved partial custody", calls, final.State, final.Mutations, rbName)
	}
}

func TestRunRollbackUnknownOutcomeIsDurable(t *testing.T) {
	record := attempt.MutationRecord{ID: "m1", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid-1", AttributableAfterRV: "7", Operation: "UPDATE", Before: "{}", IntendedAfter: "{}", ObservedAfter: "{}", Result: attempt.ResultSucceeded}
	client := rollbackTestClient(t, record, "current-epoch-0123456789abcdef0123456789")
	addRollbackTarget(t, client, "web", "uid-1", "7")
	record.ObservedAfter = rollbackTargetSnapshot(t, client, "web")
	setRollbackSourceMutations(t, client, []attempt.MutationRecord{record})
	oldClient, oldSave, oldExecute := newDynamicClientForRollback, saveRollbackAttemptStatus, executeInverseForRollback
	var final attempt.Status
	newDynamicClientForRollback = func() (dynamic.Interface, error) { return client, nil }
	saveRollbackAttemptStatus = func(ctx context.Context, c dynamic.Interface, ns, name string, obj *unstructured.Unstructured, status attempt.Status) error {
		final = status
		return oldSave(ctx, c, ns, name, obj, status)
	}
	executeInverseForRollback = func(_ context.Context, _ dynamic.Interface, record *attempt.MutationRecord, _ k8s.GovernedTarget) error {
		record.Result = attempt.ResultUnknown
		return postDispatchFailure(errors.New("response lost"))
	}
	t.Cleanup(func() {
		newDynamicClientForRollback, saveRollbackAttemptStatus, executeInverseForRollback = oldClient, oldSave, oldExecute
	})
	if err := runRollback(context.Background(), &bytes.Buffer{}, bytes.NewBuffer(nil), rollbackOptions{namespace: "default", yes: true}, "source"); err == nil {
		t.Fatal("runRollback() error = nil, want unknown outcome")
	}
	if final.State != attempt.StateOutcomeUnknown || len(final.Mutations) != 1 || final.Mutations[0].Result != attempt.ResultUnknown {
		t.Fatalf("state=%q mutations=%#v, want durable OUTCOME_UNKNOWN", final.State, final.Mutations)
	}
}

func setRollbackSourceMutations(t *testing.T, client dynamic.Interface, mutations []attempt.MutationRecord) {
	t.Helper()
	obj, err := client.Resource(attempt.GVR).Namespace("default").Get(context.Background(), "source", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&attempt.Status{State: attempt.StateApplied, Mutations: mutations})
	if err != nil {
		t.Fatal(err)
	}
	obj.Object["status"] = statusMap
	if _, err := client.Resource(attempt.GVR).Namespace("default").Update(context.Background(), obj, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func addRollbackTarget(t *testing.T, client dynamic.Interface, name, uid, rv string) {
	t.Helper()
	_, err := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace("default").Create(context.Background(), &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": name, "namespace": "default", "uid": uid, "resourceVersion": rv}, "spec": map[string]interface{}{"replicas": int64(2)}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func rollbackTargetSnapshot(t *testing.T, client dynamic.Interface, name string) string {
	t.Helper()
	obj, err := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace("default").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return string(mutationSnapshot(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, obj))
}

func TestCustodyEpochActivationPublishesOnlyAfterProbe(t *testing.T) {
	crd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
		"metadata": map[string]interface{}{"name": "applyattempts.landlockgenprof.io"},
		"spec":     map[string]interface{}{"versions": []interface{}{map[string]interface{}{"schema": map[string]interface{}{"openAPIV3Schema": map[string]interface{}{"properties": map[string]interface{}{"spec": map[string]interface{}{"x-kubernetes-validations": []interface{}{map[string]interface{}{"rule": "self == oldSelf"}}}, "status": map[string]interface{}{"x-kubernetes-validations": []interface{}{map[string]interface{}{"rule": "terminal"}}}}}}}}},
		"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Established", "status": "True"}}},
	}}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), crd)
	statusUpdates := 0
	client.PrependReactor("update", "applyattempts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		update, ok := action.(k8stesting.UpdateAction)
		if !ok || update.GetSubresource() != "status" {
			return false, nil, nil
		}
		statusUpdates++
		if statusUpdates == 3 {
			return true, nil, apierrors.NewInvalid(schema.GroupKind{Group: "landlockgenprof.io", Kind: "ApplyAttempt"}, "probe", field.ErrorList{})
		}
		return false, nil, nil
	})
	old := newDynamicClientForCustodyEpoch
	newDynamicClientForCustodyEpoch = func() (dynamic.Interface, error) { return client, nil }
	t.Cleanup(func() { newDynamicClientForCustodyEpoch = old })
	if err := runCustodyEpochActivate(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if statusUpdates != 3 {
		t.Fatalf("probe status updates = %d, want initial, terminal, and rejected rewrite", statusUpdates)
	}
	updated, err := client.Resource(attempt.CRDGVR).Get(context.Background(), "applyattempts.landlockgenprof.io", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.GetAnnotations()[attempt.CustodyEpochAnnotation]) < 32 {
		t.Fatalf("published custody epoch = %q, want at least 128 bits", updated.GetAnnotations()[attempt.CustodyEpochAnnotation])
	}
}
