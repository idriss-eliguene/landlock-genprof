//go:build envtest

package history

import (
	"context"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type receiptGate struct {
	mode                                              string
	created, incomplete, commitReady, release         sync.Once
	createdCh, incompleteCh, commitReadyCh, releaseCh chan struct{}
}

func newReceiptGate(mode string) *receiptGate {
	return &receiptGate{mode: mode, createdCh: make(chan struct{}), incompleteCh: make(chan struct{}), commitReadyCh: make(chan struct{}), releaseCh: make(chan struct{})}
}

type gatedDynamic struct {
	dynamic.Interface
	gate   *receiptGate
	caller string
}
type gatedNamespaceable struct {
	dynamic.NamespaceableResourceInterface
	gate     *receiptGate
	caller   string
	resource schema.GroupVersionResource
}
type gatedResource struct {
	dynamic.ResourceInterface
	gate     *receiptGate
	caller   string
	resource schema.GroupVersionResource
}

func (c *gatedDynamic) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &gatedNamespaceable{NamespaceableResourceInterface: c.Interface.Resource(gvr), gate: c.gate, caller: c.caller, resource: gvr}
}
func (r *gatedNamespaceable) Namespace(namespace string) dynamic.ResourceInterface {
	return &gatedResource{ResourceInterface: r.NamespaceableResourceInterface.Namespace(namespace), gate: r.gate, caller: r.caller, resource: r.resource}
}
func (r *gatedResource) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	out, err := r.ResourceInterface.Create(ctx, obj, opts, subresources...)
	if err == nil && r.resource == contributionReceiptGVR && r.gate.mode == "init" {
		r.gate.created.Do(func() { close(r.gate.createdCh) })
	}
	return out, err
}
func (r *gatedResource) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	out, err := r.ResourceInterface.Get(ctx, name, opts, subresources...)
	if err == nil && r.resource == contributionReceiptGVR && r.gate.mode == "init" && r.caller == "B" {
		state, found, _ := unstructured.NestedString(out.Object, "status", "state")
		if !found || state == "" {
			r.gate.incomplete.Do(func() { close(r.gate.incompleteCh) })
			<-r.gate.releaseCh
		}
	}
	return out, err
}
func (r *gatedResource) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	if r.resource == contributionReceiptGVR {
		if r.gate.mode == "init" && r.caller == "A" {
			<-r.gate.releaseCh
		}
		if r.gate.mode == "commit" && r.caller == "B" {
			r.gate.commitReady.Do(func() { close(r.gate.commitReadyCh) })
			<-r.gate.releaseCh
		}
	}
	return r.ResourceInterface.UpdateStatus(ctx, obj, opts)
}

func TestReceiptInitializationVisibilityConvergesOnRealAPIServer(t *testing.T) {
	client := setupEnvtest(t)
	c := containerTestContribution()
	c.ObservationID = "envtest-receipt-init-window"
	c.Population.Container = "envtest-receipt-init-container"
	gate := newReceiptGate("init")
	clientA := &gatedDynamic{Interface: client, gate: gate, caller: "A"}
	clientB := &gatedDynamic{Interface: client, gate: gate, caller: "B"}
	ctx := context.Background()
	results := make(chan error, 2)
	go func() { _, err := ApplyContribution(ctx, clientA, "default", c); results <- err }()
	<-gate.createdCh
	go func() { _, err := ApplyContribution(ctx, clientB, "default", c); results <- err }()
	<-gate.incompleteCh
	gate.release.Do(func() { close(gate.releaseCh) })
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}
}

func TestReceiptCommitConflictConvergesOnRealAPIServer(t *testing.T) {
	client := setupEnvtest(t)
	c := containerTestContribution()
	c.ObservationID = "envtest-receipt-commit-conflict"
	c.Population.Container = "envtest-receipt-commit-container"
	normalized, err := c.normalize()
	if err != nil {
		t.Fatal(err)
	}
	key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
	digest, err := normalized.contentDigest()
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewReceiptStore(client)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreatePrepared(context.Background(), "default", key, "default", "history", digest); err != nil {
		t.Fatal(err)
	}
	_, rv, err := store.Get(context.Background(), "default", key)
	if err != nil {
		t.Fatal(err)
	}
	gate := newReceiptGate("commit")
	clientB := &gatedDynamic{Interface: client, gate: gate, caller: "B"}
	result := make(chan error, 1)
	go func() {
		_, _, err := mustReceiptStore(clientB).Commit(context.Background(), "default", key, rv, digest)
		result <- err
	}()
	<-gate.commitReadyCh
	if _, _, err := store.Commit(context.Background(), "default", key, rv, digest); err != nil {
		t.Fatal(err)
	}
	gate.release.Do(func() { close(gate.releaseCh) })
	if err := <-result; err != nil {
		t.Fatalf("losing commit: %v", err)
	}
}

func mustReceiptStore(client dynamic.Interface) *ReceiptStore {
	store, err := NewReceiptStore(client)
	if err != nil {
		panic(err)
	}
	return store
}
