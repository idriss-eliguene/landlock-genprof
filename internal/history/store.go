// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package history

import (
	"context"
	"fmt"
	"path"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

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

// RecordName is the TrainingHistory object's name for a given
// container/binary: <container>-<basename(binary)>. Deliberately not the
// pod name — see the package doc and internal/k8s.Restart: pod names are
// too volatile (especially with --restart) to key history that's meant
// to persist across restarts/redeploys of the same logical target.
func RecordName(container, binary string) string {
	return fmt.Sprintf("%s-%s", container, path.Base(binary))
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

// Save creates or updates the TrainingHistory record for name in
// namespace. Re-fetches immediately before writing to carry over the
// current resourceVersion Update needs — record itself never carries
// Kubernetes bookkeeping fields, by design (see the package doc).
func Save(ctx context.Context, client dynamic.Interface, namespace, name string, record *Record) error {
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

	return &Record{
		Container:          container,
		Binary:             binary,
		RunsRecorded:       int(runsRecorded),
		FilesystemAccesses: fsAccesses,
		NetworkAccesses:    netAccesses,
	}
}
