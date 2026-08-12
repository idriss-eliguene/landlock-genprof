// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package history

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

const (
	apiGroup   = "landlockgenprof.io"
	apiVersion = "v1alpha1"
	kind       = "TrainingHistory"
)

// trainingHistoryGVR must match deploy/crd-traininghistory.yaml's group/
// version/plural exactly — there's no code-level link between them, this
// is it.
var trainingHistoryGVR = schema.GroupVersionResource{
	Group:    apiGroup,
	Version:  apiVersion,
	Resource: "traininghistories",
}

// RecordNameLegacy returns the legacy TrainingHistory object name for a
// given container/binary: <container>-<basename(binary)>.
// Keep this separate so callers can explicitly opt into legacy semantics.
func RecordNameLegacy(container, binary string) string {
	return fmt.Sprintf("%s-%s", container, path.Base(binary))
}

// RecordNameV2 returns the V2 collision-resistant name for a
// given container/binary: <container>-<basename(binary)>-<short-hash>.
// The hash is derived deterministically from the full binary path using
// SHA-256 and truncated to 8 lowercase hex chars.
func RecordNameV2(container, binary string) string {
	// compute short hex from full binary path
	h := sha256.Sum256([]byte(binary))
	hexh := hex.EncodeToString(h[:])
	short := hexh[:8]
	return fmt.Sprintf("%s-%s-%s", container, path.Base(binary), short)
}

// RecordName preserves the original convenience helper but continues to
// return the legacy name to avoid surprising callers — explicit V2
// helpers exist for the new behavior.
func RecordName(container, binary string) string {
	return RecordNameLegacy(container, binary)
}

// Get fetches the TrainingHistory record for name in namespace, or
// returns (nil, nil) if it doesn't exist yet — the first `trace
// --history` run for this target.
func Get(ctx context.Context, client dynamic.Interface, namespace, name string) (*Record, error) {
	obj, err := client.Resource(trainingHistoryGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching TrainingHistory %s/%s: %w", namespace, name, err)
	}
	return fromUnstructured(obj), nil
}

// SaveWithMerge creates or updates the TrainingHistory record for name in
// namespace by merging the provided behavior with the existing record (if
// any). On update conflict, retries the entire get-merge-update cycle
// against the fresh object to ensure the merge is recomputed with the
// latest state.
//
// SaveWithMerge is the primary save path when history already exists or
// merge state is known. For simple overwrites without merge logic, use
// SaveSnapshot instead.
func SaveWithMerge(ctx context.Context, client dynamic.Interface, namespace, name, container, binary string, behavior profile.BehaviorProfile) (*Record, error) {
	resource := client.Resource(trainingHistoryGVR).Namespace(namespace)

	v2Name := RecordNameV2(container, binary)
	legacyName := RecordNameLegacy(container, binary)

	var finalRecord *Record
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Attempt V2 first, then legacy. This preserves legacy records and
		// creates V2 only when neither exists.
		var (
			existingObj *unstructured.Unstructured
			existingRec *Record
			chosenName  string
		)

		// Try V2
		objV2, errV2 := resource.Get(ctx, v2Name, metav1.GetOptions{})
		switch {
		case errV2 == nil:
			existingObj = objV2
			chosenName = v2Name
		case apierrors.IsNotFound(errV2):
			// try legacy
			objLegacy, errLegacy := resource.Get(ctx, legacyName, metav1.GetOptions{})
			switch {
			case errLegacy == nil:
				existingObj = objLegacy
				chosenName = legacyName
			case apierrors.IsNotFound(errLegacy):
				// neither exists
				existingObj = nil
				chosenName = ""
			default:
				return fmt.Errorf("fetching TrainingHistory %s/%s before update: %w", namespace, legacyName, errLegacy)
			}
		default:
			return fmt.Errorf("fetching TrainingHistory %s/%s before update: %w", namespace, v2Name, errV2)
		}

		if existingObj != nil {
			existingRec = fromUnstructured(existingObj)
		}

		// Merge the new behavior into the fetched (or nil) state
		record := Merge(existingRec, container, binary, behavior)

		// Decide write target: existing object's name (legacy or v2) if present,
		// otherwise create V2.
		writeName := v2Name
		if chosenName != "" {
			writeName = chosenName
		}

		obj := toUnstructured(namespace, writeName, record)

		if chosenName == "" {
			// Create path
			created, err := resource.Create(ctx, obj, metav1.CreateOptions{})
			if err != nil {
				// If a concurrent creator raced and already created the object,
				// translate AlreadyExists into Conflict so RetryOnConflict will
				// re-run the closure, fetch the fresh object, and perform the
				// merge/update path.
				if apierrors.IsAlreadyExists(err) {
					return apierrors.NewConflict(schema.GroupResource{Resource: "traininghistory"}, writeName, err)
				}
				return fmt.Errorf("creating TrainingHistory %s/%s: %w", namespace, writeName, err)
			}
			finalRecord = fromUnstructured(created)
			return nil
		}

		// Update path: carry resourceVersion
		obj.SetResourceVersion(existingObj.GetResourceVersion())
		updated, err := resource.Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating TrainingHistory %s/%s: %w", namespace, writeName, err)
		}
		finalRecord = fromUnstructured(updated)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return finalRecord, nil
}

// Save creates or updates the TrainingHistory record for name in
// namespace. Re-fetches immediately before writing to carry over the
// current resourceVersion Update needs — record itself never carries
// Kubernetes bookkeeping fields, by design (see the package doc).
//
// Deprecated: Use SaveWithMerge instead. Save is kept for backward
// compatibility with tests that pass pre-computed records, but new code
// should pass the merge inputs directly so retries can recompute the merge
// against fresh state.
func Save(ctx context.Context, client dynamic.Interface, namespace, name string, record *Record) error {
	// Snapshot save: no merge computation, just overwrite
	resource := client.Resource(trainingHistoryGVR).Namespace(namespace)
	obj := toUnstructured(namespace, name, record)

	existing, err := resource.Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, err := resource.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating TrainingHistory %s/%s: %w", namespace, name, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("fetching TrainingHistory %s/%s before update: %w", namespace, name, err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	if _, err := resource.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating TrainingHistory %s/%s: %w", namespace, name, err)
	}
	return nil
}

func toUnstructured(namespace, name string, record *Record) *unstructured.Unstructured {
	fsAccesses := make([]interface{}, len(record.FilesystemAccesses))
	for i, a := range record.FilesystemAccesses {
		perms := make([]interface{}, len(a.Permissions))
		for j, p := range a.Permissions {
			perms[j] = string(p)
		}
		fsAccesses[i] = map[string]interface{}{
			"path":        a.Path,
			"permissions": perms,
			"seenInRuns":  int64(a.SeenInRuns),
		}
	}

	netAccesses := make([]interface{}, len(record.NetworkAccesses))
	for i, a := range record.NetworkAccesses {
		netAccesses[i] = map[string]interface{}{
			"port":       int64(a.Port),
			"direction":  string(a.Direction),
			"seenInRuns": int64(a.SeenInRuns),
		}
	}

	syscallAccesses := make([]interface{}, len(record.SyscallAccesses))
	for i, a := range record.SyscallAccesses {
		syscallAccesses[i] = map[string]interface{}{
			"name":       a.Name,
			"seenInRuns": int64(a.SeenInRuns),
		}
	}

	capabilityAccesses := make([]interface{}, len(record.CapabilityAccesses))
	for i, a := range record.CapabilityAccesses {
		capabilityAccesses[i] = map[string]interface{}{
			"name":       a.Name,
			"seenInRuns": int64(a.SeenInRuns),
		}
	}

	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiGroup + "/" + apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"container":          record.Container,
			"binary":             record.Binary,
			"runsRecorded":       int64(record.RunsRecorded),
			"filesystemAccesses": fsAccesses,
			"networkAccesses":    netAccesses,
			"syscallAccesses":    syscallAccesses,
			"capabilityAccesses": capabilityAccesses,
		},
	}}
}

// fromUnstructured is deliberately forgiving: a missing or malformed
// field falls back to its zero value rather than failing the whole
// read. Fields are only ever written by toUnstructured (this project
// controls both ends), so a mismatch would mean manual editing/
// corruption, not a real integration to guard strictly against.
func fromUnstructured(obj *unstructured.Unstructured) *Record {
	container, _, _ := unstructured.NestedString(obj.Object, "spec", "container")
	binary, _, _ := unstructured.NestedString(obj.Object, "spec", "binary")
	runsRecorded, _, _ := unstructured.NestedInt64(obj.Object, "spec", "runsRecorded")

	fsRaw, _, _ := unstructured.NestedSlice(obj.Object, "spec", "filesystemAccesses")
	fsAccesses := make([]FileAccessRecord, 0, len(fsRaw))
	for _, item := range fsRaw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		p, _, _ := unstructured.NestedString(m, "path")
		seenInRuns, _, _ := unstructured.NestedInt64(m, "seenInRuns")
		permsRaw, _, _ := unstructured.NestedStringSlice(m, "permissions")
		perms := make([]profile.FilePermission, len(permsRaw))
		for i, s := range permsRaw {
			perms[i] = profile.FilePermission(s)
		}
		fsAccesses = append(fsAccesses, FileAccessRecord{
			Path:        p,
			Permissions: perms,
			SeenInRuns:  int(seenInRuns),
		})
	}

	netRaw, _, _ := unstructured.NestedSlice(obj.Object, "spec", "networkAccesses")
	netAccesses := make([]NetworkAccessRecord, 0, len(netRaw))
	for _, item := range netRaw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		port, _, _ := unstructured.NestedInt64(m, "port")
		direction, _, _ := unstructured.NestedString(m, "direction")
		seenInRuns, _, _ := unstructured.NestedInt64(m, "seenInRuns")
		netAccesses = append(netAccesses, NetworkAccessRecord{
			Port:       int(port),
			Direction:  profile.NetworkDirection(direction),
			SeenInRuns: int(seenInRuns),
		})
	}

	syscallRaw, _, _ := unstructured.NestedSlice(obj.Object, "spec", "syscallAccesses")
	syscallAccesses := make([]SyscallAccessRecord, 0, len(syscallRaw))
	for _, item := range syscallRaw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		syscallName, _, _ := unstructured.NestedString(m, "name")
		seenInRuns, _, _ := unstructured.NestedInt64(m, "seenInRuns")
		syscallAccesses = append(syscallAccesses, SyscallAccessRecord{
			Name:       syscallName,
			SeenInRuns: int(seenInRuns),
		})
	}

	capabilityRaw, _, _ := unstructured.NestedSlice(obj.Object, "spec", "capabilityAccesses")
	capabilityAccesses := make([]CapabilityAccessRecord, 0, len(capabilityRaw))
	for _, item := range capabilityRaw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		capabilityName, _, _ := unstructured.NestedString(m, "name")
		seenInRuns, _, _ := unstructured.NestedInt64(m, "seenInRuns")
		capabilityAccesses = append(capabilityAccesses, CapabilityAccessRecord{
			Name:       capabilityName,
			SeenInRuns: int(seenInRuns),
		})
	}

	return &Record{
		Container:          container,
		Binary:             binary,
		RunsRecorded:       int(runsRecorded),
		FilesystemAccesses: fsAccesses,
		NetworkAccesses:    netAccesses,
		SyscallAccesses:    syscallAccesses,
		CapabilityAccesses: capabilityAccesses,
	}
}
