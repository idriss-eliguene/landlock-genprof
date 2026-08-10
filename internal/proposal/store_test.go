// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package proposal

import (
	"context"
	"reflect"
	"sync"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestGet_NotFoundReturnsNilNil(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Errorf("Get() = %+v, want nil (no proposal yet)", got)
	}
}

const examplePodLockYAML = `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
  namespace: default
spec:
  profilesByContainer:
    nginx:
      /usr/sbin/nginx:
        readOnly:
          - /etc/nginx
`

const exampleNetworkPolicyYAML = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nginx-demo
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: nginx
  ingress:
    - ports:
        - port: 80
`

const examplePatchedManifestYAML = `apiVersion: v1
kind: Pod
metadata:
  name: nginx-demo
  namespace: default
spec:
  containers:
    - name: nginx
      image: nginx:alpine
      securityContext:
        capabilities:
          add:
            - SETUID
          drop:
            - ALL
        seccompProfile:
          type: Localhost
          localhostProfile: nginx-demo-seccomp.json
`

const exampleSPOSeccompProfileYAML = `apiVersion: security-profiles-operator.x-k8s.io/v1beta1
kind: SeccompProfile
metadata:
  name: nginx-demo
  namespace: default
spec:
  defaultAction: SCMP_ACT_ERRNO
  syscalls:
    - action: SCMP_ACT_ALLOW
      names:
        - openat
        - read
`

// TestSave_ThenGet_RoundTrips exercises every field populated at once —
// plain rendered text, exactly what cmd/landlock-genprof/trace.go's
// publishProposal stores (see its own doc comment for why this isn't a
// structured sub-spec).
func TestSave_ThenGet_RoundTrips(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{
		Container:         "nginx",
		Binary:            "/usr/sbin/nginx",
		GeneratedAt:       "2026-07-24T10:00:00Z",
		HistoryUsed:       true,
		PodLock:           examplePodLockYAML,
		NetworkPolicy:     exampleNetworkPolicyYAML,
		PatchedManifest:   examplePatchedManifestYAML,
		SPOSeccompProfile: exampleSPOSeccompProfileYAML,
	}

	if err := Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(got, &spec) {
		t.Errorf("round-tripped spec = %+v, want %+v", got, spec)
	}
}

// TestSave_ThenGet_EmptyFieldsRoundTrip checks that a field left empty
// (nothing observed for that domain this run) round-trips back as an
// empty string, not some non-empty placeholder — the plain-string
// equivalent of the nil-vs-empty-value gotcha already hit once building
// TrainingHistory's own round-trip test.
func TestSave_ThenGet_EmptyFieldsRoundTrip(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{
		Container:   "nginx",
		Binary:      "/usr/sbin/nginx",
		GeneratedAt: "2026-07-24T10:00:00Z",
		PodLock:     examplePodLockYAML,
		// NetworkPolicy/PatchedManifest/SPOSeccompProfile deliberately
		// left empty: no network/syscall/capability activity was
		// observed this run.
	}

	if err := Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.NetworkPolicy != "" {
		t.Errorf("NetworkPolicy = %q, want empty", got.NetworkPolicy)
	}
	if got.PatchedManifest != "" {
		t.Errorf("PatchedManifest = %q, want empty", got.PatchedManifest)
	}
	if got.SPOSeccompProfile != "" {
		t.Errorf("SPOSeccompProfile = %q, want empty", got.SPOSeccompProfile)
	}
	if !reflect.DeepEqual(got, &spec) {
		t.Errorf("round-tripped spec = %+v, want %+v", got, spec)
	}
}

// TestSave_UpdatesExistingProposal checks the Create-vs-Update branch in
// Save: a second Save for the same name must overwrite (a proposal is
// the latest snapshot, not an accumulation — see Save's own doc
// comment), not fail on a missing/stale resourceVersion or create a
// duplicate.
func TestSave_UpdatesExistingProposal(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	first := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(context.Background(), client, "default", "nginx-demo", first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	second := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T11:00:00Z", HistoryUsed: true}
	if err := Save(context.Background(), client, "default", "nginx-demo", second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	got, err := Get(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.GeneratedAt != "2026-07-24T11:00:00Z" || !got.HistoryUsed {
		t.Errorf("got = %+v, want the second Save's values (overwritten, not accumulated)", got)
	}
}

func TestSave_SetsInitialDraftStatus(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(context.Background(), client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	status, err := GetStatus(context.Background(), client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.ApprovalState != ApprovalDraft {
		t.Errorf("ApprovalState = %q, want %q", status.ApprovalState, ApprovalDraft)
	}
	if status.UpdatedAt == "" {
		t.Error("UpdatedAt is empty, want a stamped timestamp")
	}
}

// TestSave_DoesNotClobberApprovalStatus is the entire point of splitting
// Status into the CRD's status subresource: a human's approve decision
// must survive `trace` regenerating .spec against the same pod (Save's
// update path is a full overwrite of the object it builds — see Save's
// own doc comment). If this test ever fails, the status-subresource
// split has broken somehow and every approval would be silently wiped
// on the next training run.
func TestSave_DoesNotClobberApprovalStatus(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	first := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(ctx, client, "default", "nginx-demo", first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := SetApprovalState(ctx, client, "default", "nginx-demo", ApprovalApproved, "looks good"); err != nil {
		t.Fatalf("SetApprovalState() error = %v", err)
	}

	second := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T11:00:00Z"}
	if err := Save(ctx, client, "default", "nginx-demo", second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	status, err := GetStatus(ctx, client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.ApprovalState != ApprovalApproved {
		t.Errorf("ApprovalState = %q after a Save re-run, want it to stay %q", status.ApprovalState, ApprovalApproved)
	}
	if status.Reason != "looks good" {
		t.Errorf("Reason = %q after a Save re-run, want it preserved", status.Reason)
	}
}

func TestMarkReviewed_DraftToReviewed(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(ctx, client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := MarkReviewed(ctx, client, "default", "nginx-demo"); err != nil {
		t.Fatalf("MarkReviewed() error = %v", err)
	}

	status, err := GetStatus(ctx, client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.ApprovalState != ApprovalReviewed {
		t.Errorf("ApprovalState = %q, want %q", status.ApprovalState, ApprovalReviewed)
	}
}

// TestMarkReviewed_NoopPastDraft checks that re-running `review` after an
// explicit approve/reject decision never silently reverts it back to
// Reviewed — see MarkReviewed's own doc comment.
func TestMarkReviewed_NoopPastDraft(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(ctx, client, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := SetApprovalState(ctx, client, "default", "nginx-demo", ApprovalApproved, ""); err != nil {
		t.Fatalf("SetApprovalState() error = %v", err)
	}

	if err := MarkReviewed(ctx, client, "default", "nginx-demo"); err != nil {
		t.Fatalf("MarkReviewed() error = %v", err)
	}

	status, err := GetStatus(ctx, client, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.ApprovalState != ApprovalApproved {
		t.Errorf("ApprovalState = %q after MarkReviewed on an already-Approved proposal, want it to stay %q", status.ApprovalState, ApprovalApproved)
	}
}

func TestSetApprovalState_NotFound(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	err := SetApprovalState(context.Background(), client, "default", "nginx-demo", ApprovalRejected, "")
	if err == nil {
		t.Fatal("SetApprovalState() on a nonexistent proposal: error = nil, want an error")
	}
}

// newFakeClientForList is dynamicfake.NewSimpleDynamicClient's List-aware
// sibling — required (not just stylistic) once a test actually calls
// List(): List/Get/Save don't need this, but the fake client's own List
// implementation panics without an explicit GVR -> List-Kind hint for any
// resource with no generated Go type registered in the scheme (this
// project's CRDs are all handled as unstructured.Unstructured, by
// design, so none ever are). "SecurityProfileProposalList" matches the
// standard Kubernetes <Kind>List convention — there's no generated type
// to derive it from, this project never needed one before List existed.
func newFakeClientForList() dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			securityProfileProposalGVR: "SecurityProfileProposalList",
		},
	)
}

func TestList_EmptyNamespace(t *testing.T) {
	client := newFakeClientForList()

	items, err := List(context.Background(), client, "default")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

func TestList_SortedByNameWithApprovalState(t *testing.T) {
	ctx := context.Background()
	client := newFakeClientForList()

	if err := Save(ctx, client, "default", "zebra", Spec{Container: "c"}); err != nil {
		t.Fatalf("Save(zebra) error = %v", err)
	}
	if err := Save(ctx, client, "default", "apple", Spec{Container: "c"}); err != nil {
		t.Fatalf("Save(apple) error = %v", err)
	}
	if err := SetApprovalState(ctx, client, "default", "apple", ApprovalApproved, "looks good"); err != nil {
		t.Fatalf("SetApprovalState() error = %v", err)
	}

	items, err := List(ctx, client, "default")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2: %+v", len(items), items)
	}
	if items[0].Name != "apple" || items[1].Name != "zebra" {
		t.Errorf("names = [%s, %s], want [apple, zebra] (sorted)", items[0].Name, items[1].Name)
	}
	if items[0].Status.ApprovalState != ApprovalApproved {
		t.Errorf("apple's ApprovalState = %q, want %q", items[0].Status.ApprovalState, ApprovalApproved)
	}
	if items[1].Status.ApprovalState != ApprovalDraft {
		t.Errorf("zebra's ApprovalState = %q, want %q (never explicitly approved/rejected)", items[1].Status.ApprovalState, ApprovalDraft)
	}
}

func TestList_DifferentNamespacesDontMix(t *testing.T) {
	ctx := context.Background()
	client := newFakeClientForList()

	if err := Save(ctx, client, "default", "nginx-demo", Spec{Container: "c"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := Save(ctx, client, "prod", "nginx-prod", Spec{Container: "c"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	items, err := List(ctx, client, "default")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || items[0].Name != "nginx-demo" {
		t.Errorf("List(default) = %+v, want just [nginx-demo]", items)
	}
}

// conflictInjectorProposal tracks which UpdateStatus calls should return 409 Conflict.
type conflictInjectorProposal struct {
	mu                    sync.Mutex
	updateStatusConflicts int
}

func (ci *conflictInjectorProposal) shouldFailUpdateStatus() bool {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	if ci.updateStatusConflicts > 0 {
		ci.updateStatusConflicts--
		return true
	}
	return false
}

// conflictInjectingResourceInterfaceProposal wraps a dynamic.ResourceInterface and
// injects 409 Conflict on UpdateStatus calls.
type conflictInjectingResourceInterfaceProposal struct {
	underlying dynamic.ResourceInterface
	injector   *conflictInjectorProposal
}

func (ci *conflictInjectingResourceInterfaceProposal) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Create(ctx, obj, opts, subresources...)
}

func (ci *conflictInjectingResourceInterfaceProposal) Update(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Update(ctx, obj, opts, subresources...)
}

func (ci *conflictInjectingResourceInterfaceProposal) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	if ci.injector.shouldFailUpdateStatus() {
		return nil, apierrors.NewConflict(schema.GroupResource{Resource: "securityprofileproposals"}, "test", nil)
	}
	return ci.underlying.UpdateStatus(ctx, obj, opts)
}

func (ci *conflictInjectingResourceInterfaceProposal) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Get(ctx, name, opts, subresources...)
}

func (ci *conflictInjectingResourceInterfaceProposal) Delete(ctx context.Context, name string, opts metav1.DeleteOptions, subresources ...string) error {
	return ci.underlying.Delete(ctx, name, opts, subresources...)
}

func (ci *conflictInjectingResourceInterfaceProposal) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return ci.underlying.DeleteCollection(ctx, opts, listOpts)
}

func (ci *conflictInjectingResourceInterfaceProposal) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return ci.underlying.List(ctx, opts)
}

func (ci *conflictInjectingResourceInterfaceProposal) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return ci.underlying.Watch(ctx, opts)
}

func (ci *conflictInjectingResourceInterfaceProposal) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Patch(ctx, name, pt, data, opts, subresources...)
}

func (ci *conflictInjectingResourceInterfaceProposal) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Apply(ctx, name, obj, opts, subresources...)
}

func (ci *conflictInjectingResourceInterfaceProposal) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return ci.underlying.ApplyStatus(ctx, name, obj, opts)
}

// conflictInjectingNamespaceableResourceInterfaceProposal wraps both methods.
type conflictInjectingNamespaceableResourceInterfaceProposal struct {
	underlying dynamic.NamespaceableResourceInterface
	injector   *conflictInjectorProposal
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) Namespace(namespace string) dynamic.ResourceInterface {
	return &conflictInjectingResourceInterfaceProposal{
		underlying: ci.underlying.Namespace(namespace),
		injector:   ci.injector,
	}
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Create(ctx, obj, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) Update(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Update(ctx, obj, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	if ci.injector.shouldFailUpdateStatus() {
		return nil, apierrors.NewConflict(schema.GroupResource{Resource: "securityprofileproposals"}, "test", nil)
	}
	return ci.underlying.UpdateStatus(ctx, obj, opts)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Get(ctx, name, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) Delete(ctx context.Context, name string, opts metav1.DeleteOptions, subresources ...string) error {
	return ci.underlying.Delete(ctx, name, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return ci.underlying.DeleteCollection(ctx, opts, listOpts)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return ci.underlying.List(ctx, opts)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return ci.underlying.Watch(ctx, opts)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Patch(ctx, name, pt, data, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return ci.underlying.Apply(ctx, name, obj, opts, subresources...)
}

func (ci *conflictInjectingNamespaceableResourceInterfaceProposal) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return ci.underlying.ApplyStatus(ctx, name, obj, opts)
}

// conflictInjectingClientProposal wraps a dynamic.Interface and injects 409 Conflict
// on UpdateStatus calls.
type conflictInjectingClientProposal struct {
	underlying dynamic.Interface
	injector   *conflictInjectorProposal
}

func (ci *conflictInjectingClientProposal) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &conflictInjectingNamespaceableResourceInterfaceProposal{
		underlying: ci.underlying.Resource(gvr),
		injector:   ci.injector,
	}
}

func newConflictInjectingClientProposal(underlying dynamic.Interface, updateStatusConflicts int) dynamic.Interface {
	return &conflictInjectingClientProposal{
		underlying: underlying,
		injector: &conflictInjectorProposal{
			updateStatusConflicts: updateStatusConflicts,
		},
	}
}

func TestMarkReviewed_RetriesOnConflict(t *testing.T) {
	ctx := context.Background()
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(ctx, underlying, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := newConflictInjectingClientProposal(underlying, 1)

	if err := MarkReviewed(ctx, client, "default", "nginx-demo"); err != nil {
		t.Fatalf("MarkReviewed() with one injected conflict: error = %v", err)
	}

	status, err := GetStatus(ctx, underlying, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() after retry: error = %v", err)
	}
	if status.ApprovalState != ApprovalReviewed {
		t.Errorf("ApprovalState = %q, want %q after retry", status.ApprovalState, ApprovalReviewed)
	}
}

func TestMarkReviewed_RetryStaysIdempotent(t *testing.T) {
	ctx := context.Background()
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(ctx, underlying, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := newConflictInjectingClientProposal(underlying, 1)

	if err := MarkReviewed(ctx, client, "default", "nginx-demo"); err != nil {
		t.Fatalf("MarkReviewed() with one injected conflict: error = %v", err)
	}

	// Call again without conflicts — should still be a no-op after being Reviewed
	if err := MarkReviewed(ctx, underlying, "default", "nginx-demo"); err != nil {
		t.Fatalf("second MarkReviewed(): error = %v", err)
	}

	status, err := GetStatus(ctx, underlying, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus(): error = %v", err)
	}
	if status.ApprovalState != ApprovalReviewed {
		t.Errorf("ApprovalState = %q, want to stay %q", status.ApprovalState, ApprovalReviewed)
	}
}

func TestSetApprovalState_RetriesOnConflict(t *testing.T) {
	ctx := context.Background()
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(ctx, underlying, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := newConflictInjectingClientProposal(underlying, 1)

	if err := SetApprovalState(ctx, client, "default", "nginx-demo", ApprovalApproved, "looks good"); err != nil {
		t.Fatalf("SetApprovalState() with one injected conflict: error = %v", err)
	}

	status, err := GetStatus(ctx, underlying, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("GetStatus() after retry: error = %v", err)
	}
	if status.ApprovalState != ApprovalApproved {
		t.Errorf("ApprovalState = %q, want %q after retry", status.ApprovalState, ApprovalApproved)
	}
	if status.Reason != "looks good" {
		t.Errorf("Reason = %q, want %q after retry", status.Reason, "looks good")
	}
}

func TestSetApprovalState_RetryExhaustion(t *testing.T) {
	ctx := context.Background()
	underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	spec := Spec{Container: "nginx", Binary: "/usr/sbin/nginx", GeneratedAt: "2026-07-24T10:00:00Z"}
	if err := Save(ctx, underlying, "default", "nginx-demo", spec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	client := newConflictInjectingClientProposal(underlying, 15)

	err := SetApprovalState(ctx, client, "default", "nginx-demo", ApprovalApproved, "looks good")
	if err == nil {
		t.Fatal("SetApprovalState() with exhausted retries: want error")
	}
	if !apierrors.IsConflict(err) {
		t.Errorf("error = %v, want Conflict error", err)
	}
}
