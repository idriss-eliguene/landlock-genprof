package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
)

type rollbackOptions struct {
	namespace string
	yes       bool
}

type inverseFailure struct {
	result string
	err    error
}

var newDynamicClientForRollback = newDynamicClient
var createRollbackAttempt = attempt.CreateRollback
var saveRollbackAttemptStatus = attempt.SaveRollbackStatus
var executeInverseForRollback = executeInverse

func (f *inverseFailure) Error() string { return f.err.Error() }

func preDispatchFailure(err error) error {
	return &inverseFailure{result: attempt.ResultFailed, err: err}
}

func postDispatchFailure(err error) error {
	return &inverseFailure{result: attempt.ResultUnknown, err: err}
}

func inverseFailureResult(err error) string {
	var failure *inverseFailure
	if errors.As(err, &failure) {
		return failure.result
	}
	return attempt.ResultUnknown
}

func newRollbackCmd() *cobra.Command {
	var opts rollbackOptions
	cmd := &cobra.Command{
		Use:   "rollback <apply-attempt>",
		Short: "Explicitly rolls back an eligible ApplyAttempt",
		Long: "Creates a durable RollbackAttempt and reverses only eligible, strict-resource-version-bound mutations. " +
			"Rollback is explicit, sequential, nontransactional, and CLI-only." + kubectlPrefixNote,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(cmd.Context(), cmd.OutOrStdout(), cmd.InOrStdin(), opts, args[0])
		},
	}
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Skip the explicit rollback confirmation")
	return cmd
}

func runRollback(ctx context.Context, stdout io.Writer, stdin io.Reader, opts rollbackOptions, name string) error {
	client, err := newDynamicClientForRollback()
	if err != nil {
		return fmt.Errorf("connecting to cluster for rollback: %w", err)
	}
	source, err := client.Resource(attempt.GVR).Namespace(opts.namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("ApplyAttempt %s/%s not found", opts.namespace, name)
	}
	if err != nil {
		return err
	}
	var spec attempt.Spec
	if err := fromUnstructured(source.Object["spec"], &spec); err != nil {
		return fmt.Errorf("reading source custody: %w", err)
	}
	var status attempt.Status
	if err := fromUnstructured(source.Object["status"], &status); err != nil {
		return fmt.Errorf("reading source status: %w", err)
	}
	if source.GetUID() == "" {
		return fmt.Errorf("REFUSE_SOURCE_NOT_ROLLBACK_QUALIFIED: source UID is absent")
	}
	epoch, hardened, err := attempt.CustodyEpochAndHardening(ctx, client)
	if err != nil {
		return err
	}
	if !hardened || epoch == "" || spec.CustodyEpoch == "" || spec.CustodyEpoch != epoch {
		return fmt.Errorf("REFUSE_SOURCE_EPOCH_MISMATCH: source epoch does not match the current ApplyAttempt CRD epoch")
	}
	if status.State == attempt.StateInProgress {
		return fmt.Errorf("REFUSE_SOURCE_NOT_ELIGIBLE: ApplyAttempt is still in progress")
	}
	completed, unknownDescendants, previous, err := rollbackHistory(ctx, client, opts.namespace, string(source.GetUID()))
	if err != nil {
		return fmt.Errorf("reading rollback history: %w", err)
	}

	eligible := make([]attempt.MutationRecord, 0, len(status.Mutations))
	for _, record := range status.Mutations {
		if record.Kind == "Pod" && record.Operation == "DELETE_THEN_CREATE" {
			return fmt.Errorf("REFUSE_UNSUPPORTED_BARE_POD: rollback request contains a bare-Pod replacement")
		}
		if record.Result != attempt.ResultSucceeded {
			continue
		}
		if completed[record.ID] {
			continue
		}
		if unknownDescendants[record.ID] {
			return fmt.Errorf("REFUSE_UNKNOWN_ROLLBACK_OUTCOME: source mutation %s has an unknown rollback descendant", record.ID)
		}
		if record.AttributableAfterRV == "" || record.ObservedAfter == "" {
			continue
		}
		eligible = append(eligible, record)
	}
	if len(eligible) == 0 {
		if previous != nil {
			return fmt.Errorf("REFUSE_ALREADY_ROLLED_BACK: every eligible source mutation has a successful rollback descendant")
		}
		return fmt.Errorf("REFUSE_NO_ROLLBACK_ELIGIBLE_MUTATIONS")
	}

	fmt.Fprintf(stdout, "Rollback source: %s/%s\nTarget: %s/%s\nSource state: %s\n", opts.namespace, name, spec.Target.Workload.Kind, spec.Target.Workload.Name, status.State)
	for i := len(eligible) - 1; i >= 0; i-- {
		fmt.Fprintf(stdout, "  - %s %s/%s (%s)\n", eligible[i].Operation, eligible[i].Namespace, eligible[i].Name, inverseName(eligible[i]))
	}
	fmt.Fprintln(stdout, "Warning: rollback is nontransactional; no automatic recovery or compensation is provided.")
	if !opts.yes {
		fmt.Fprint(stdout, "Execute this rollback? [y/N] ")
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			return nil
		}
	}

	rbSpec := attempt.RollbackSpec{
		SourceNamespace: opts.namespace, SourceName: name, SourceUID: string(source.GetUID()),
		ProposalNamespace: spec.ProposalNamespace, ProposalName: spec.ProposalName, ProposalUID: spec.ProposalUID,
		ApprovedCandidateDigest: spec.ApprovedCandidateDigest, Target: spec.Target, CustodyEpoch: epoch,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if previous != nil {
		rbSpec.PreviousNamespace = opts.namespace
		rbSpec.PreviousName = previous.GetName()
		rbSpec.PreviousUID = string(previous.GetUID())
	}
	rbName, rbObj, err := createRollbackAttempt(ctx, client, opts.namespace, rbSpec)
	if err != nil {
		return fmt.Errorf("creating rollback custody: %w", err)
	}
	rbStatus := attempt.Status{State: attempt.StateInProgress}
	if err := saveRollbackAttemptStatus(ctx, client, opts.namespace, rbName, rbObj, rbStatus); err != nil {
		return fmt.Errorf("initial rollback custody persistence failed; no inverse mutation executed: %w", err)
	}

	for i := len(eligible) - 1; i >= 0; i-- {
		sourceRecord := eligible[i]
		prepared, prepareErr := prepareInverse(ctx, client, sourceRecord)
		if prepareErr != nil {
			prepared = sourceRecord
			prepared.ID = sourceRecord.ID + "-inverse"
			prepared.Operation = inverseOperation(sourceRecord)
			prepared.SourceMutationID = sourceRecord.ID
			prepared.Result = attempt.ResultFailed
			prepared.Error = prepareErr.Error()
			rbStatus.Mutations = append(rbStatus.Mutations, prepared)
			_ = saveRollbackAttemptStatus(ctx, client, opts.namespace, rbName, rbObj, rbStatus)
			return fmt.Errorf("rollback %s refused before mutation: %w", sourceRecord.ID, prepareErr)
		}
		rbStatus.Mutations = append(rbStatus.Mutations, prepared)
		if err := saveRollbackAttemptStatus(ctx, client, opts.namespace, rbName, rbObj, rbStatus); err != nil {
			return fmt.Errorf("rollback pre-mutation custody failed; no inverse mutation executed: %w", err)
		}
		if err := executeInverseForRollback(ctx, client, &sourceRecord, spec.Target); err != nil {
			sourceRecord.Result = inverseFailureResult(err)
			prepared.Result = sourceRecord.Result
			prepared.ObservedAfter = sourceRecord.ObservedAfter
			prepared.ObservedAfterDigest = sourceRecord.ObservedAfterDigest
			prepared.Error = err.Error()
			rbStatus.Mutations[len(rbStatus.Mutations)-1] = prepared
			if sourceRecord.Result == attempt.ResultUnknown {
				rbStatus.State = attempt.StateOutcomeUnknown
			} else if hasSuccessfulMutation(rbStatus.Mutations) {
				rbStatus.State = attempt.StatePartiallyApplied
			} else {
				rbStatus.State = attempt.StateFailed
			}
			_ = saveRollbackAttemptStatus(ctx, client, opts.namespace, rbName, rbObj, rbStatus)
			return fmt.Errorf("rollback %s failed: %w", sourceRecord.ID, err)
		}
		prepared.Result = sourceRecord.Result
		prepared.ObservedAfter = sourceRecord.ObservedAfter
		prepared.ObservedAfterDigest = sourceRecord.ObservedAfterDigest
		rbStatus.Mutations[len(rbStatus.Mutations)-1] = prepared
		if err := saveRollbackAttemptStatus(ctx, client, opts.namespace, rbName, rbObj, rbStatus); err != nil {
			rbStatus.State = attempt.StateOutcomeUnknown
			_ = saveRollbackAttemptStatus(ctx, client, opts.namespace, rbName, rbObj, rbStatus)
			return fmt.Errorf("rollback result custody failed: %w", err)
		}
	}
	rbStatus.State = attempt.StateApplied
	rbStatus.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveRollbackAttemptStatus(ctx, client, opts.namespace, rbName, rbObj, rbStatus); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "RollbackAttempt: %s/%s\n", opts.namespace, rbName)
	return nil
}

func prepareInverse(ctx context.Context, client dynamic.Interface, source attempt.MutationRecord) (attempt.MutationRecord, error) {
	gvr, cluster := rollbackGVR(source)
	if gvr.Resource == "" {
		return attempt.MutationRecord{}, fmt.Errorf("unsupported rollback kind %s/%s", source.Group, source.Kind)
	}
	var resource dynamic.ResourceInterface
	if cluster {
		resource = client.Resource(gvr)
	} else {
		resource = client.Resource(gvr).Namespace(source.Namespace)
	}
	current, err := resource.Get(ctx, source.Name, metav1.GetOptions{})
	if err != nil {
		return attempt.MutationRecord{}, fmt.Errorf("reading rollback Before: %w", err)
	}
	if string(current.GetUID()) != recordUID(source) || current.GetResourceVersion() != source.AttributableAfterRV {
		return attempt.MutationRecord{}, fmt.Errorf("strict UID/resourceVersion guard refused inverse")
	}
	if !controlledMatches(source, current) {
		return attempt.MutationRecord{}, fmt.Errorf("controlled state drift refused inverse")
	}
	before := string(mutationSnapshot(schema.GroupVersionKind{Group: source.Group, Version: source.Version, Kind: source.Kind}, current))
	intended := source.Before
	return attempt.MutationRecord{
		ID: source.ID + "-inverse", SourceMutationID: source.ID,
		Group: source.Group, Version: source.Version, Kind: source.Kind,
		Namespace: source.Namespace, Name: source.Name, UID: string(current.GetUID()),
		ResourceVersion: current.GetResourceVersion(), Operation: inverseOperation(source),
		Before: before, IntendedAfter: intended, BeforeDigest: digestJSON([]byte(before)),
		IntendedAfterDigest: digestJSON([]byte(intended)), Result: attempt.ResultUnknown,
	}, nil
}

func inverseOperation(source attempt.MutationRecord) string {
	if source.Operation == "CREATE" {
		return "DELETE"
	}
	return "UPDATE"
}

func rollbackHistory(ctx context.Context, client dynamic.Interface, namespace, sourceUID string) (map[string]bool, map[string]bool, *unstructured.Unstructured, error) {
	list, err := client.Resource(attempt.RollbackGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, nil, err
	}
	completed := map[string]bool{}
	unknown := map[string]bool{}
	var previous *unstructured.Unstructured
	for i := range list.Items {
		item := &list.Items[i]
		var spec attempt.RollbackSpec
		if fromUnstructured(item.Object["spec"], &spec) != nil || spec.SourceUID != sourceUID {
			continue
		}
		if previous == nil || item.GetCreationTimestamp().After(previous.GetCreationTimestamp().Time) {
			previous = item
		}
		var status attempt.Status
		if fromUnstructured(item.Object["status"], &status) != nil {
			continue
		}
		for _, record := range status.Mutations {
			if record.Result == attempt.ResultSucceeded && record.SourceMutationID != "" {
				completed[record.SourceMutationID] = true
			}
			if record.Result == attempt.ResultUnknown && record.SourceMutationID != "" {
				unknown[record.SourceMutationID] = true
			}
		}
	}
	return completed, unknown, previous, nil
}

func fromUnstructured(value interface{}, out interface{}) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func inverseName(record attempt.MutationRecord) string {
	if record.Operation == "CREATE" {
		return "DELETE"
	}
	return "RESTORE_CONTROLLED_BEFORE"
}

func rollbackGVR(record attempt.MutationRecord) (schema.GroupVersionResource, bool) {
	gvk := schema.GroupVersionKind{Group: record.Group, Version: record.Version, Kind: record.Kind}
	switch {
	case gvk == (schema.GroupVersionKind{Group: "podlock.kubewarden.io", Version: "v1alpha1", Kind: "LandlockProfile"}):
		return schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: "landlockprofiles"}, false
	case gvk == (schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}):
		return schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: "networkpolicies"}, false
	case gvk == spobackend.SeccompProfileGVK():
		return spobackend.SeccompProfileGVR(), true
	case gvk == (schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}):
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, false
	case gvk == (schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}):
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, false
	case gvk == (schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}):
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, false
	default:
		return schema.GroupVersionResource{}, false
	}
}

func executeInverse(ctx context.Context, client dynamic.Interface, record *attempt.MutationRecord, target k8s.GovernedTarget) error {
	gvr, cluster := rollbackGVR(*record)
	if gvr.Resource == "" {
		return preDispatchFailure(fmt.Errorf("unsupported rollback kind %s/%s", record.Group, record.Kind))
	}
	var resource dynamic.ResourceInterface
	if cluster {
		resource = client.Resource(gvr)
	} else {
		resource = client.Resource(gvr).Namespace(record.Namespace)
	}
	current, err := resource.Get(ctx, record.Name, metav1.GetOptions{})
	if err != nil {
		return preDispatchFailure(fmt.Errorf("reading current object: %w", err))
	}
	if string(current.GetUID()) != recordUID(*record) || current.GetResourceVersion() != record.AttributableAfterRV {
		return preDispatchFailure(fmt.Errorf("strict UID/resourceVersion guard refused inverse"))
	}
	if !controlledMatches(*record, current) {
		return preDispatchFailure(fmt.Errorf("controlled state drift refused inverse"))
	}
	if record.Operation == "CREATE" {
		if isPolicyKind(record.Kind) && targetReferencesPolicy(ctx, record, target, client) {
			return preDispatchFailure(fmt.Errorf("policy is still referenced by the governed workload"))
		}
		uid, rv := current.GetUID(), current.GetResourceVersion()
		if err := resource.Delete(ctx, record.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &rv}}); err != nil {
			if apierrors.IsConflict(err) {
				return preDispatchFailure(fmt.Errorf("delete CAS conflict: %w", err))
			}
			return postDispatchFailure(err)
		}
		for i := 0; i < 50; i++ {
			_, getErr := resource.Get(ctx, record.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(getErr) {
				record.Result = attempt.ResultSucceeded
				return nil
			}
			if getErr != nil {
				return postDispatchFailure(fmt.Errorf("delete outcome unknown: %w", getErr))
			}
			time.Sleep(100 * time.Millisecond)
		}
		record.Result = attempt.ResultUnknown
		return postDispatchFailure(fmt.Errorf("delete outcome unknown: object was not confirmed absent"))
	}
	before, err := snapshotFromRecord(record.Before)
	if err != nil {
		return preDispatchFailure(err)
	}
	if err := restoreControlledState(current, before); err != nil {
		return preDispatchFailure(err)
	}
	if current.GetKind() == "Deployment" || current.GetKind() == "StatefulSet" || current.GetKind() == "DaemonSet" {
		if err := verifyRestoredReferencesReady(ctx, client, before); err != nil {
			return preDispatchFailure(err)
		}
		var plans []plannedArtifact
		if hasPodLockReference(current) {
			plans = append(plans, plannedArtifact{slug: "podlock"})
		}
		if hasSeccompReference(current) {
			plans = append(plans, plannedArtifact{slug: "spo-seccompprofile"})
		}
		if err := validateCompositionCompatibility(plans); err != nil {
			return preDispatchFailure(err)
		}
	}
	updated, err := resource.Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsConflict(err) {
			return preDispatchFailure(fmt.Errorf("update CAS conflict: %w", err))
		}
		return postDispatchFailure(err)
	}
	after, err := resource.Get(ctx, record.Name, metav1.GetOptions{})
	if err != nil {
		return postDispatchFailure(fmt.Errorf("rollback outcome unknown: %w", err))
	}
	if updated.GetUID() != after.GetUID() || updated.GetResourceVersion() != after.GetResourceVersion() {
		return postDispatchFailure(fmt.Errorf("rollback outcome unknown: response and reread differ"))
	}
	record.Result = attempt.ResultSucceeded
	record.ObservedAfter = string(mutationSnapshot(schema.GroupVersionKind{Group: record.Group, Version: record.Version, Kind: record.Kind}, after))
	record.ObservedAfterDigest = digestJSON([]byte(record.ObservedAfter))
	return nil
}

func verifyRestoredReferencesReady(ctx context.Context, client dynamic.Interface, before map[string]interface{}) error {
	template, ok := nestedMap(before, "spec", "template")
	if !ok {
		return nil
	}
	containers, ok := nestedSlice(template, "spec", "containers")
	if !ok {
		return nil
	}
	for _, raw := range containers {
		container, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		sc, ok := container["securityContext"].(map[string]interface{})
		if !ok {
			continue
		}
		profile, ok := sc["seccompProfile"].(map[string]interface{})
		if !ok {
			continue
		}
		localhost, _ := profile["localhostProfile"].(string)
		if localhost == "" {
			continue
		}
		name, ok := spobackend.ParseLocalhostProfilePath(localhost)
		if !ok {
			return fmt.Errorf("refusing workload restore: cannot establish SeccompProfile identity from %q", localhost)
		}
		obj, err := client.Resource(spobackend.SeccompProfileGVR()).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("refusing workload restore: SeccompProfile %s is not ready: %w", name, err)
		}
		installed, found, err := unstructured.NestedString(obj.Object, "status", "localhostProfile")
		if err != nil || !found || installed != localhost {
			return fmt.Errorf("refusing workload restore: SeccompProfile %s readiness is not established", name)
		}
	}
	return nil
}

func isPolicyKind(kind string) bool {
	return kind == "LandlockProfile" || kind == "NetworkPolicy" || kind == "SeccompProfile"
}

func hasPodLockReference(obj *unstructured.Unstructured) bool {
	labels, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
	if labels == nil {
		labels, _, _ = unstructured.NestedStringMap(obj.Object, "metadata", "labels")
	}
	return labels[podLockProfileLabel] != ""
}

func hasSeccompReference(obj *unstructured.Unstructured) bool {
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if containers == nil {
		containers, _, _ = unstructured.NestedSlice(obj.Object, "spec", "containers")
	}
	for _, raw := range containers {
		c, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		sc, ok := c["securityContext"].(map[string]interface{})
		if !ok {
			continue
		}
		if _, ok := sc["seccompProfile"]; ok {
			return true
		}
	}
	return false
}

func targetReferencesPolicy(ctx context.Context, record *attempt.MutationRecord, target k8s.GovernedTarget, client dynamic.Interface) bool {
	gvk := schema.GroupVersionKind{Group: target.Workload.Group, Kind: target.Workload.Kind, Version: "v1"}
	gvr, cluster := rollbackGVR(attempt.MutationRecord{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind})
	if gvr.Resource == "" {
		return true
	}
	var workloadResource dynamic.ResourceInterface
	if cluster {
		workloadResource = client.Resource(gvr)
	} else {
		workloadResource = client.Resource(gvr).Namespace(target.Namespace)
	}
	workload, err := workloadResource.Get(ctx, target.Workload.Name, metav1.GetOptions{})
	if err != nil {
		return true
	}
	switch record.Kind {
	case "LandlockProfile":
		return podLockReferenceName(workload) == record.Name
	case "SeccompProfile":
		references, known := seccompReferenceNames(workload)
		if !known {
			return true
		}
		for _, name := range references {
			if name == record.Name {
				return true
			}
		}
		return false
	case "NetworkPolicy":
		selector, found, err := nestedStringMapFromJSON(record.ObservedAfter, "spec", "podSelector", "matchLabels")
		if err != nil || !found {
			return true
		}
		labels, _, _ := unstructured.NestedStringMap(workload.Object, "spec", "template", "metadata", "labels")
		if labels == nil {
			labels = workload.GetLabels()
		}
		for key, value := range selector {
			if labels[key] != value {
				return false
			}
		}
		return true
	default:
		return true
	}
}

func podLockReferenceName(obj *unstructured.Unstructured) string {
	labels, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
	if labels == nil {
		labels, _, _ = unstructured.NestedStringMap(obj.Object, "metadata", "labels")
	}
	return labels[podLockProfileLabel]
}

func seccompReferenceNames(obj *unstructured.Unstructured) ([]string, bool) {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if !found {
		containers, found, err = unstructured.NestedSlice(obj.Object, "spec", "containers")
	}
	if err != nil {
		return nil, false
	}
	if !found {
		return nil, true
	}
	var names []string
	for _, raw := range containers {
		container, ok := raw.(map[string]interface{})
		if !ok {
			return nil, false
		}
		sc, present := container["securityContext"].(map[string]interface{})
		if !present {
			continue
		}
		profile, present := sc["seccompProfile"].(map[string]interface{})
		if !present {
			continue
		}
		localhost, present := profile["localhostProfile"].(string)
		if !present {
			return nil, false
		}
		name, ok := spobackend.ParseLocalhostProfilePath(localhost)
		if !ok {
			return nil, false
		}
		names = append(names, name)
	}
	return names, true
}

func nestedStringMapFromJSON(raw string, fields ...string) (map[string]string, bool, error) {
	var object map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		return nil, false, err
	}
	value, found, err := unstructured.NestedStringMap(object, fields...)
	return value, found, err
}

func recordUID(record attempt.MutationRecord) string {
	if record.UID != "" {
		return record.UID
	}
	var snap map[string]interface{}
	if json.Unmarshal([]byte(record.ObservedAfter), &snap) == nil {
		if m, ok := snap["metadata"].(map[string]interface{}); ok {
			if uid, ok := m["uid"].(string); ok {
				return uid
			}
		}
	}
	return ""
}

func snapshotFromRecord(raw string) (map[string]interface{}, error) {
	var snap map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return nil, err
	}
	return snap, nil
}

func controlledMatches(record attempt.MutationRecord, current *unstructured.Unstructured) bool {
	var observed map[string]interface{}
	if json.Unmarshal([]byte(record.ObservedAfter), &observed) != nil {
		return false
	}
	var currentSnap map[string]interface{}
	gvk := schema.GroupVersionKind{Group: record.Group, Version: record.Version, Kind: record.Kind}
	if json.Unmarshal(mutationSnapshot(gvk, current), &currentSnap) != nil {
		return false
	}
	delete(observed, "metadata")
	delete(currentSnap, "metadata")
	return reflect.DeepEqual(observed, currentSnap)
}

func restoreControlledState(current *unstructured.Unstructured, before map[string]interface{}) error {
	spec, ok := before["spec"]
	if !ok {
		return fmt.Errorf("rollback Before has no controlled spec")
	}
	if current.GetKind() == "Deployment" || current.GetKind() == "StatefulSet" || current.GetKind() == "DaemonSet" {
		return mergeWorkloadBefore(current.Object, spec)
	}
	current.Object["spec"] = spec
	return nil
}

func mergeWorkloadBefore(object map[string]interface{}, raw interface{}) error {
	before, ok := raw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid workload Before")
	}
	template, ok := before["template"].(map[string]interface{})
	if !ok {
		return nil
	}
	curTemplate, _, _ := unstructured.NestedMap(object, "spec", "template")
	if curTemplate == nil {
		curTemplate = map[string]interface{}{}
	}
	if labels, ok := nestedMap(template, "metadata", "labels"); ok {
		curLabels, _, _ := unstructured.NestedStringMap(curTemplate, "metadata", "labels")
		if curLabels == nil {
			curLabels = map[string]string{}
		}
		for k, v := range labels {
			if s, ok := v.(string); ok {
				curLabels[k] = s
			}
		}
		_ = unstructured.SetNestedStringMap(curTemplate, curLabels, "metadata", "labels")
	} else if labels, found, _ := unstructured.NestedStringMap(curTemplate, "metadata", "labels"); found {
		delete(labels, podLockProfileLabel)
		if len(labels) == 0 {
			if metadata, ok := curTemplate["metadata"].(map[string]interface{}); ok {
				delete(metadata, "labels")
			}
		} else {
			_ = unstructured.SetNestedStringMap(curTemplate, labels, "metadata", "labels")
		}
	}
	if spec, ok := template["spec"].(map[string]interface{}); ok {
		if saved, ok := spec["containers"].([]interface{}); ok {
			current, _, _ := unstructured.NestedSlice(curTemplate, "spec", "containers")
			for _, savedRaw := range saved {
				savedContainer, ok := savedRaw.(map[string]interface{})
				if !ok {
					continue
				}
				name, _ := savedContainer["name"].(string)
				for _, currentRaw := range current {
					currentContainer, ok := currentRaw.(map[string]interface{})
					if !ok || currentContainer["name"] != name {
						continue
					}
					if savedSC, ok := savedContainer["securityContext"].(map[string]interface{}); ok {
						currentSC, _ := currentContainer["securityContext"].(map[string]interface{})
						if currentSC == nil {
							currentSC = map[string]interface{}{}
						}
						for _, key := range []string{"capabilities", "seccompProfile"} {
							if value, present := savedSC[key]; present {
								currentSC[key] = value
							} else {
								delete(currentSC, key)
							}
						}
						if len(currentSC) == 0 {
							delete(currentContainer, "securityContext")
						} else {
							currentContainer["securityContext"] = currentSC
						}
					} else {
						if currentSC, ok := currentContainer["securityContext"].(map[string]interface{}); ok {
							delete(currentSC, "capabilities")
							delete(currentSC, "seccompProfile")
							if len(currentSC) == 0 {
								delete(currentContainer, "securityContext")
							}
						}
					}
				}
			}
			_ = unstructured.SetNestedSlice(curTemplate, current, "spec", "containers")
		}
	}
	return unstructured.SetNestedMap(object, curTemplate, "spec", "template")
}

func nestedMap(m map[string]interface{}, keys ...string) (map[string]interface{}, bool) {
	var v interface{} = m
	for _, k := range keys {
		x, ok := v.(map[string]interface{})
		if !ok {
			return nil, false
		}
		v, ok = x[k]
		if !ok {
			return nil, false
		}
	}
	x, ok := v.(map[string]interface{})
	return x, ok
}

func nestedSlice(m map[string]interface{}, keys ...string) ([]interface{}, bool) {
	var value interface{} = m
	for _, key := range keys {
		object, ok := value.(map[string]interface{})
		if !ok {
			return nil, false
		}
		value, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	slice, ok := value.([]interface{})
	return slice, ok
}
