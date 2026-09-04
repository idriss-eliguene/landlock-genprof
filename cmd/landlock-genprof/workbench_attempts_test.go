package main

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

type attemptReadStub struct {
	k8s.WorkbenchReadCapability
	apply    *unstructured.UnstructuredList
	rollback *unstructured.UnstructuredList
	epoch    string
}

func (s attemptReadStub) GetApplyAttempt(context.Context, string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (s attemptReadStub) ListApplyAttempts(context.Context) (*unstructured.UnstructuredList, error) {
	return s.apply, nil
}
func (s attemptReadStub) GetRollbackAttempt(context.Context, string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (s attemptReadStub) ListRollbackAttempts(context.Context) (*unstructured.UnstructuredList, error) {
	return s.rollback, nil
}
func (s attemptReadStub) GetCustodyEpoch(context.Context) (string, error) { return s.epoch, nil }

func TestWorkbenchAttemptProjectionPreservesCustodyAndRelationships(t *testing.T) {
	apply := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"namespace": "default", "name": "apply-1", "uid": "a-uid"},
		"spec":     map[string]interface{}{"proposalNamespace": "default", "proposalName": "proposal", "proposalUID": "p-uid", "approvedCandidateDigest": "sha256:" + strings.Repeat("a", 64), "startedAt": "2026-09-04T00:00:00Z", "custodyEpoch": strings.Repeat("b", 32), "target": map[string]interface{}{"namespace": "default", "workload": map[string]interface{}{"kind": "Deployment", "name": "api"}, "container": "app"}},
		"status":   map[string]interface{}{"state": attempt.StatePartiallyApplied, "updatedAt": "2026-09-04T00:01:00Z", "mutations": []interface{}{map[string]interface{}{"id": "m1", "version": "v1", "kind": "Deployment", "name": "api", "operation": "UPDATE", "before": "<before>", "intendedAfter": "<after>", "observedAfter": "<observed>", "attributableAfterRV": "12", "result": attempt.ResultSucceeded}}},
	}}}}
	rb := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"namespace": "default", "name": "rollback-1", "uid": "r-uid"},
		"spec":     map[string]interface{}{"sourceNamespace": "default", "sourceName": "apply-1", "sourceUID": "a-uid", "previousNamespace": "default", "previousName": "rollback-0", "previousUID": "r0", "target": map[string]interface{}{"namespace": "default", "workload": map[string]interface{}{"kind": "Deployment", "name": "api"}, "container": "app"}, "custodyEpoch": strings.Repeat("b", 32), "startedAt": "2026-09-04T00:02:00Z"},
		"status":   map[string]interface{}{"state": attempt.StateOutcomeUnknown, "mutations": []interface{}{map[string]interface{}{"id": "m1", "version": "v1", "kind": "Deployment", "name": "api", "operation": "UPDATE", "before": "<before>", "intendedAfter": "<restored>", "result": attempt.ResultUnknown, "sourceMutationID": "m1"}}},
	}}}}
	view := loadWorkbenchAttemptVisibility(context.Background(), attemptReadStub{apply: apply, rollback: rb, epoch: strings.Repeat("b", 32)})
	if view.State != "AVAILABLE" || view.CustodyEpoch != strings.Repeat("b", 32) || len(view.ApplyAttempts) != 1 || len(view.RollbackAttempts) != 1 {
		t.Fatalf("attempt view = %+v", view)
	}
	if view.ApplyAttempts[0].State != attempt.StatePartiallyApplied || view.ApplyAttempts[0].Mutations[0].AttributableAfterRV != "12" {
		t.Fatalf("apply custody lost: %+v", view.ApplyAttempts[0])
	}
	if view.RollbackAttempts[0].Source != "default/apply-1 (a-uid)" || view.RollbackAttempts[0].Mutations[0].Result != attempt.ResultUnknown {
		t.Fatalf("rollback relationship/result lost: %+v", view.RollbackAttempts[0])
	}
}
