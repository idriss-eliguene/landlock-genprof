// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

const workbenchAttemptDisplayCap = 100

type workbenchAttemptVisibility struct {
	State             string
	Reason            string
	CustodyEpoch      string
	ApplyAttempts     []workbenchApplyAttempt
	ApplyTotal        int
	ApplyTruncated    bool
	RollbackAttempts  []workbenchRollbackAttempt
	RollbackTotal     int
	RollbackTruncated bool
}

type workbenchApplyAttempt struct {
	Namespace   string
	Name        string
	UID         string
	Proposal    string
	ProposalUID string
	Digest      string
	Target      string
	Epoch       string
	State       string
	StartedAt   string
	UpdatedAt   string
	Mutations   []attempt.MutationRecord
}

type workbenchRollbackAttempt struct {
	Namespace string
	Name      string
	UID       string
	Source    string
	Previous  string
	Target    string
	Epoch     string
	State     string
	StartedAt string
	UpdatedAt string
	Mutations []attempt.MutationRecord
}

func loadWorkbenchAttemptVisibility(ctx context.Context, reads k8s.WorkbenchReadCapability) workbenchAttemptVisibility {
	view := workbenchAttemptVisibility{State: "AVAILABLE", Reason: "bounded namespace-scoped attempt reads"}
	if epoch, err := reads.GetCustodyEpoch(ctx); err != nil {
		view.State = readStateText(err)
		view.Reason = err.Error()
	} else if epoch == "" {
		view.CustodyEpoch = "NOT_AVAILABLE — no custody epoch is published"
	} else {
		view.CustodyEpoch = epoch
	}
	applyList, err := reads.ListApplyAttempts(ctx)
	if err != nil {
		if view.State == "AVAILABLE" {
			view.State = readStateText(err)
		}
		view.Reason = appendReadReason(view.Reason, err)
	} else {
		view.ApplyTotal = len(applyList.Items)
		view.ApplyTruncated = view.ApplyTotal > workbenchAttemptDisplayCap
		view.ApplyAttempts = decodeWorkbenchApplyAttempts(applyList)
		if view.ApplyTruncated {
			view.Reason = appendDisplayCapReason(view.Reason, "ApplyAttempt", len(view.ApplyAttempts), view.ApplyTotal)
		}
	}
	rollbackList, err := reads.ListRollbackAttempts(ctx)
	if err != nil {
		if view.State == "AVAILABLE" {
			view.State = readStateText(err)
		}
		view.Reason = appendReadReason(view.Reason, err)
	} else {
		view.RollbackTotal = len(rollbackList.Items)
		view.RollbackTruncated = view.RollbackTotal > workbenchAttemptDisplayCap
		view.RollbackAttempts = decodeWorkbenchRollbackAttempts(rollbackList)
		if view.RollbackTruncated {
			view.Reason = appendDisplayCapReason(view.Reason, "RollbackAttempt", len(view.RollbackAttempts), view.RollbackTotal)
		}
	}
	return view
}

func appendDisplayCapReason(existing, kind string, shown, total int) string {
	message := fmt.Sprintf("%s display capped: %d of %d shown", kind, shown, total)
	if existing == "" {
		return message
	}
	return existing + "; " + message
}

func readStateText(err error) string {
	var readErr *k8s.ReadError
	if errors.As(err, &readErr) {
		return string(readErr.State)
	}
	return "UNKNOWN"
}

func appendReadReason(existing string, err error) string {
	if existing == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s; %s", existing, err.Error())
}

func decodeWorkbenchApplyAttempts(list *unstructured.UnstructuredList) []workbenchApplyAttempt {
	if list == nil {
		return nil
	}
	items := list.Items
	if len(items) > workbenchAttemptDisplayCap {
		items = items[:workbenchAttemptDisplayCap]
	}
	out := make([]workbenchApplyAttempt, 0, len(items))
	for i := range items {
		var spec attempt.Spec
		var status attempt.Status
		_ = fromNested(&items[i], "spec", &spec)
		_ = fromNested(&items[i], "status", &status)
		out = append(out, workbenchApplyAttempt{
			Namespace: items[i].GetNamespace(), Name: items[i].GetName(), UID: string(items[i].GetUID()),
			Proposal: spec.ProposalNamespace + "/" + spec.ProposalName, ProposalUID: spec.ProposalUID,
			Digest: spec.ApprovedCandidateDigest, Target: targetText(spec.Target), Epoch: epochText(spec.CustodyEpoch),
			State: status.State, StartedAt: spec.StartedAt, UpdatedAt: status.UpdatedAt, Mutations: status.Mutations,
		})
	}
	return out
}

func decodeWorkbenchRollbackAttempts(list *unstructured.UnstructuredList) []workbenchRollbackAttempt {
	if list == nil {
		return nil
	}
	items := list.Items
	if len(items) > workbenchAttemptDisplayCap {
		items = items[:workbenchAttemptDisplayCap]
	}
	out := make([]workbenchRollbackAttempt, 0, len(items))
	for i := range items {
		var spec attempt.RollbackSpec
		var status attempt.Status
		_ = fromNested(&items[i], "spec", &spec)
		_ = fromNested(&items[i], "status", &status)
		previous := ""
		if spec.PreviousName != "" {
			previous = spec.PreviousNamespace + "/" + spec.PreviousName + " (" + spec.PreviousUID + ")"
		}
		out = append(out, workbenchRollbackAttempt{
			Namespace: items[i].GetNamespace(), Name: items[i].GetName(), UID: string(items[i].GetUID()),
			Source: spec.SourceNamespace + "/" + spec.SourceName + " (" + spec.SourceUID + ")", Previous: previous,
			Target: targetText(spec.Target), Epoch: spec.CustodyEpoch, State: status.State,
			StartedAt: spec.StartedAt, UpdatedAt: status.UpdatedAt, Mutations: status.Mutations,
		})
	}
	return out
}

func fromNested(obj *unstructured.Unstructured, field string, into interface{}) error {
	value, found, err := unstructured.NestedMap(obj.Object, field)
	if err != nil || !found {
		return err
	}
	return runtime.DefaultUnstructuredConverter.FromUnstructured(value, into)
}

func targetText(target k8s.GovernedTarget) string {
	return fmt.Sprintf("%s/%s/%s/%s", target.Namespace, target.Workload.Kind, target.Workload.Name, target.Container)
}

func epochText(epoch string) string {
	if epoch == "" {
		return "NOT_ROLLBACK_QUALIFIED — no custody epoch"
	}
	return epoch
}
