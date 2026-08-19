// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package history

import (
	"context"
	"reflect"
	"sync"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// conflictInjector tracks which Update calls should return 409 Conflict.
type conflictInjector struct {
	mu              sync.Mutex
	updateConflicts int
}

func (ci *conflictInjector) shouldFailUpdate() bool {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	if ci.updateConflicts > 0 {
		ci.updateConflicts--
		return true
	}
	return false
}

// conflictInjectingResourceInterface wraps a dynamic.ResourceInterface and
// injects 409 Conflict on demand.
type conflictInjectingResourceInterface struct {
	underlying dynamic.ResourceInterface
	injector   *conflictInjector
}

func (ci *conflictInjectingResourceInterface) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Create(ctx, obj, opts, subresources...)
}

func (ci *conflictInjectingResourceInterface) Update(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	if ci.injector.shouldFailUpdate() {
		return nil, apierrors.NewConflict(schema.GroupResource{Resource: "traininghistory"}, "test", nil)
	}
	return ci.underlying.Update(ctx, obj, opts, subresources...)
}

func (ci *conflictInjectingResourceInterface) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return ci.underlying.UpdateStatus(ctx, obj, opts)
}

func (ci *conflictInjectingResourceInterface) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Get(ctx, name, opts, subresources...)
}

func (ci *conflictInjectingResourceInterface) Delete(ctx context.Context, name string, opts metav1.DeleteOptions, subresources ...string) error {
	return ci.underlying.Delete(ctx, name, opts, subresources...)
}

func (ci *conflictInjectingResourceInterface) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return ci.underlying.DeleteCollection(ctx, opts, listOpts)
}

func (ci *conflictInjectingResourceInterface) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return ci.underlying.List(ctx, opts)
}

func (ci *conflictInjectingResourceInterface) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return ci.underlying.Watch(ctx, opts)
}

func (ci *conflictInjectingResourceInterface) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Patch(ctx, name, pt, data, opts, subresources...)
}

func (ci *conflictInjectingResourceInterface) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Apply(ctx, name, obj, opts, subresources...)
}

func (ci *conflictInjectingResourceInterface) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return ci.underlying.ApplyStatus(ctx, name, obj, opts)
}

// conflictInjectingNamespaceableResourceInterface wraps both methods.
type conflictInjectingNamespaceableResourceInterface struct {
	underlying dynamic.NamespaceableResourceInterface
	injector   *conflictInjector
}

func (ci *conflictInjectingNamespaceableResourceInterface) Namespace(namespace string) dynamic.ResourceInterface {
	return &conflictInjectingResourceInterface{
		underlying: ci.underlying.Namespace(namespace),
		injector:   ci.injector,
	}
}

// Implement ResourceInterface methods on NamespaceableResourceInterface
func (ci *conflictInjectingNamespaceableResourceInterface) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Create(ctx, obj, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterface) Update(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	if ci.injector.shouldFailUpdate() {
		return nil, apierrors.NewConflict(schema.GroupResource{Resource: "traininghistory"}, "test", nil)
	}
	return ci.underlying.Update(ctx, obj, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterface) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return ci.underlying.UpdateStatus(ctx, obj, opts)
}

func (ci *conflictInjectingNamespaceableResourceInterface) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Get(ctx, name, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterface) Delete(ctx context.Context, name string, opts metav1.DeleteOptions, subresources ...string) error {
	return ci.underlying.Delete(ctx, name, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterface) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return ci.underlying.DeleteCollection(ctx, opts, listOpts)
}

func (ci *conflictInjectingNamespaceableResourceInterface) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return ci.underlying.List(ctx, opts)
}

func (ci *conflictInjectingNamespaceableResourceInterface) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return ci.underlying.Watch(ctx, opts)
}

func (ci *conflictInjectingNamespaceableResourceInterface) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Patch(ctx, name, pt, data, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterface) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Apply(ctx, name, obj, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterface) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return ci.underlying.ApplyStatus(ctx, name, obj, opts)
}

// conflictInjectingClient wraps a dynamic.Interface and injects 409 Conflict
// on Update calls.
type conflictInjectingClient struct {
	underlying dynamic.Interface
	injector   *conflictInjector
}

func (ci *conflictInjectingClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &conflictInjectingNamespaceableResourceInterface{
		underlying: ci.underlying.Resource(gvr),
		injector:   ci.injector,
	}
}

func newConflictInjectingClient(underlying dynamic.Interface, updateConflicts int) dynamic.Interface {
	return &conflictInjectingClient{
		underlying: underlying,
		injector: &conflictInjector{
			updateConflicts: updateConflicts,
		},
	}
}

func TestRecordName(t *testing.T) {
	if got := RecordName("nginx", "/usr/sbin/nginx"); got != "nginx-nginx" {
		t.Errorf("RecordName() = %q, want nginx-nginx", got)
	}
}

func TestGet_NotFoundReturnsNilNil(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	record, err := Get(context.Background(), client, "default", "nginx-nginx")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if record != nil {
		t.Errorf("Get() = %+v, want nil (no record yet)", record)
	}
}

func TestSave_ThenGet_RoundTrips(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	record := &Record{
		Container:    "nginx",
		Binary:       "/usr/sbin/nginx",
		RunsRecorded: 2,
		FilesystemAccesses: []FileAccessRecord{
			{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, SeenInRuns: 2},
			{Path: "/var/cache/nginx/proxy", Permissions: []profile.FilePermission{profile.PermissionWrite}, SeenInRuns: 1},
		},
		NetworkAccesses: []NetworkAccessRecord{
			{Port: 443, Direction: profile.DirectionEgress, SeenInRuns: 2},
		},
		SyscallAccesses: []SyscallAccessRecord{
			{Name: "openat", SeenInRuns: 2},
			{Name: "brk", SeenInRuns: 1},
		},
		CapabilityAccesses: []CapabilityAccessRecord{
			{Name: "CAP_NET_BIND_SERVICE", SeenInRuns: 2},
		},
	}

	if err := Save(context.Background(), client, "default", "nginx-nginx", record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-nginx")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(got, record) {
		t.Errorf("round-tripped record = %+v, want %+v", got, record)
	}
}

func TestSave_UpdatesExistingRecord(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	first := &Record{Container: "nginx", Binary: "/usr/sbin/nginx", RunsRecorded: 1,
		FilesystemAccesses: []FileAccessRecord{{Path: "/etc/nginx", SeenInRuns: 1}}}
	if err := Save(context.Background(), client, "default", "nginx-nginx", first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	second := &Record{Container: "nginx", Binary: "/usr/sbin/nginx", RunsRecorded: 2,
		FilesystemAccesses: []FileAccessRecord{{Path: "/etc/nginx", SeenInRuns: 2}}}
	if err := Save(context.Background(), client, "default", "nginx-nginx", second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-nginx")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.RunsRecorded != 2 {
		t.Errorf("RunsRecorded = %d, want 2", got.RunsRecorded)
	}
}

func TestSaveWithMerge_RetriesOnConflict(t *testing.T) {
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	initial := &Record{
		Container:    "nginx",
		Binary:       "/usr/sbin/nginx",
		RunsRecorded: 1,
		FilesystemAccesses: []FileAccessRecord{
			{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, SeenInRuns: 1},
		},
	}
	if err := Save(context.Background(), underlying, "default", "nginx-nginx", initial); err != nil {
		t.Fatalf("initial Save: error = %v", err)
	}

	client := newConflictInjectingClient(underlying, 1)
	behavior := profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{
			Accesses: []profile.FileAccess{
				{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, Confidence: profile.ConfidenceHigh},
			},
		},
	}

	persisted, err := SaveWithMerge(context.Background(), client, "default", "nginx-nginx", "nginx", "/usr/sbin/nginx", behavior)
	if err != nil {
		t.Fatalf("SaveWithMerge() with conflict: error = %v", err)
	}
	if persisted == nil {
		t.Fatalf("SaveWithMerge returned nil persisted record")
	}

	updated, err := Get(context.Background(), underlying, "default", "nginx-nginx")
	if err != nil {
		t.Fatalf("Get() after retry: error = %v", err)
	}
	if updated.RunsRecorded != 2 {
		t.Errorf("RunsRecorded = %d, want 2", updated.RunsRecorded)
	}
}

func TestSaveWithMerge_RetryExhaustion(t *testing.T) {
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	initial := &Record{
		Container:    "nginx",
		Binary:       "/usr/sbin/nginx",
		RunsRecorded: 1,
	}
	if err := Save(context.Background(), underlying, "default", "nginx-nginx", initial); err != nil {
		t.Fatalf("initial Save: error = %v", err)
	}

	client := newConflictInjectingClient(underlying, 15)
	behavior := profile.BehaviorProfile{}

	// Inject more conflicts (15) than the retry budget allows (DefaultRetry.Steps = 5),
	// so initial attempt + 5 retries will exhaust before all conflicts are consumed.
	_, err := SaveWithMerge(context.Background(), client, "default", "nginx-nginx", "nginx", "/usr/sbin/nginx", behavior)
	if err == nil {
		t.Fatal("SaveWithMerge() with exhausted retries: want conflict error")
	}
	if !apierrors.IsConflict(err) {
		t.Errorf("SaveWithMerge() error = %v, want Conflict error", err)
	}
}

// Additional tests for V2 naming and compatibility
func TestRecordNameV2Deterministic(t *testing.T) {
	n1 := RecordNameV2("nginx", "/usr/sbin/nginx")
	n2 := RecordNameV2("nginx", "/usr/sbin/nginx")
	if n1 != n2 {
		t.Fatalf("RecordNameV2 nondeterministic: %s vs %s", n1, n2)
	}
}

func TestRecordNameV2PathDifferentiation(t *testing.T) {
	n1 := RecordNameV2("nginx", "/opt/tools/run-helper")
	n2 := RecordNameV2("nginx", "/usr/local/bin/run-helper")
	if n1 == n2 {
		t.Fatalf("RecordNameV2 collision for different paths: %s", n1)
	}
}

func TestLegacyFallbackDoesNotCreateV2(t *testing.T) {
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	legacyName := RecordNameLegacy("nginx", "/opt/tools/run-helper")
	initial := &Record{Container: "nginx", Binary: "/opt/tools/run-helper", RunsRecorded: 1}
	if err := Save(context.Background(), underlying, "default", legacyName, initial); err != nil {
		t.Fatalf("initial Save legacy: %v", err)
	}

	client := underlying
	// call SaveWithMerge; it should detect legacy and update it, not create V2
	behavior := profile.BehaviorProfile{}
	persisted, err := SaveWithMerge(context.Background(), client, "default", legacyName, "nginx", "/opt/tools/run-helper", behavior)
	if err != nil {
		t.Fatalf("SaveWithMerge error: %v", err)
	}
	if persisted == nil {
		t.Fatalf("SaveWithMerge returned nil persisted record")
	}

	v2Name := RecordNameV2("nginx", "/opt/tools/run-helper")
	gotLegacy, _ := Get(context.Background(), underlying, "default", legacyName)
	gotV2, _ := Get(context.Background(), underlying, "default", v2Name)
	if gotLegacy == nil {
		t.Fatalf("legacy record disappeared")
	}
	if gotLegacy.RunsRecorded != 2 {
		t.Fatalf("legacy RunsRecorded = %d, want 2", gotLegacy.RunsRecorded)
	}
	if gotV2 != nil {
		t.Fatalf("unexpected V2 record created when legacy existed: %s", v2Name)
	}
}

func TestV2Preference(t *testing.T) {
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	v2Name := RecordNameV2("nginx", "/opt/tools/run-helper")
	initial := &Record{Container: "nginx", Binary: "/opt/tools/run-helper", RunsRecorded: 1}
	if err := Save(context.Background(), underlying, "default", v2Name, initial); err != nil {
		t.Fatalf("initial Save v2: %v", err)
	}

	client := underlying
	behavior := profile.BehaviorProfile{}
	persisted, err := SaveWithMerge(context.Background(), client, "default", v2Name, "nginx", "/opt/tools/run-helper", behavior)
	if err != nil {
		t.Fatalf("SaveWithMerge error: %v", err)
	}
	if persisted == nil {
		t.Fatalf("SaveWithMerge returned nil persisted record")
	}

	gotV2, _ := Get(context.Background(), underlying, "default", v2Name)
	if gotV2 == nil || gotV2.RunsRecorded != 2 {
		t.Fatalf("v2 record not updated as expected")
	}
}

func TestCreateWhenNeitherExistsCreatesV2(t *testing.T) {
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	client := underlying
	behavior := profile.BehaviorProfile{}
	v2Name := RecordNameV2("nginx", "/opt/tools/run-helper")
	persisted, err := SaveWithMerge(context.Background(), client, "default", v2Name, "nginx", "/opt/tools/run-helper", behavior)
	if err != nil {
		t.Fatalf("SaveWithMerge create error: %v", err)
	}
	if persisted == nil {
		t.Fatalf("SaveWithMerge returned nil persisted record")
	}
	gotV2, _ := Get(context.Background(), underlying, "default", v2Name)
	if gotV2 == nil {
		t.Fatalf("expected V2 record created but none found")
	}
}

func TestLegacyNameCollisionProof(t *testing.T) {
	legacy1 := RecordNameLegacy("nginx", "/opt/tools/foo")
	legacy2 := RecordNameLegacy("nginx", "/usr/local/bin/foo")
	if legacy1 != legacy2 {
		t.Fatalf("expected legacy names to collide but they differ: %s vs %s", legacy1, legacy2)
	}
	v21 := RecordNameV2("nginx", "/opt/tools/foo")
	v22 := RecordNameV2("nginx", "/usr/local/bin/foo")
	if v21 == v22 {
		t.Fatalf("v2 names unexpectedly collided: %s", v21)
	}
}
