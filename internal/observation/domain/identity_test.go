package domain

import (
	"errors"
	"testing"
)

func testCluster() ClusterIdentity { return ClusterIdentity{NamespaceUID: "cluster-uid"} }

func testWorkload(uid string) WorkloadIdentity {
	return WorkloadIdentity{Cluster: testCluster(), Namespace: "team-a", GroupKind: GroupKind{Group: "apps", Kind: "Deployment"}, Name: "api", UID: uid}
}

func testSlot(uid, container string) ContainerSlot {
	return ContainerSlot{Workload: testWorkload(uid), Container: container}
}

func TestClusterIdentityIsOnlyNamespaceUID(t *testing.T) {
	a, err := NewClusterIdentity("uid-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewClusterIdentity("uid-a")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("equal kube-system UIDs differ: %#v %#v", a, b)
	}
	locators := []ClusterLocator{
		{KubeconfigSource: "/one", Context: "context-a", APIURL: "https://one"},
		{KubeconfigSource: "/two", Context: "context-b", APIURL: "https://two"},
	}
	if locators[0] == locators[1] {
		t.Fatal("locator values should be independently representable")
	}
	if a != b {
		t.Fatal("locator-independent identity changed unexpectedly")
	}
}

func TestClusterIdentityResolutionFailsClosed(t *testing.T) {
	for _, value := range []string{"", "   "} {
		_, err := NewClusterIdentity(value)
		if !errors.Is(err, ErrClusterIdentityUnresolved) {
			t.Fatalf("value %q error = %v, want ErrClusterIdentityUnresolved", value, err)
		}
	}
}

func TestWorkloadIdentityIncludesObjectUIDButNotRevision(t *testing.T) {
	base := testWorkload("uid-a")
	if base != testWorkload("uid-a") {
		t.Fatal("identical workload identity values are not equal")
	}
	if base == testWorkload("uid-b") {
		t.Fatal("object UID change did not change workload identity")
	}
	revisionA, _ := NewWorkloadRevision("revision-a")
	revisionB, _ := NewWorkloadRevision("revision-b")
	if revisionA == revisionB {
		t.Fatal("distinct revision values are equal")
	}
	if base != testWorkload("uid-a") {
		t.Fatal("revision values changed the workload identity")
	}
}

func TestContainerSlotExcludesImageAndRuntimeIdentity(t *testing.T) {
	a := testSlot("pod-object", "backend")
	b := testSlot("pod-object", "sidecar")
	if a == b {
		t.Fatal("container name change did not change slot")
	}
	imageA, err := NewContainerImageRevision(a, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	imageB, err := NewContainerImageRevision(a, "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	if imageA.Slot != imageB.Slot || imageA == imageB {
		t.Fatal("image revision did not remain distinct from the slot")
	}
}

func TestContainerImageRevisionRejectsMutableTagOrMissingDigest(t *testing.T) {
	for _, value := range []string{"", "nginx:latest", "sha256:short"} {
		if _, err := NewContainerImageRevision(testSlot("uid-a", "backend"), value); err == nil {
			t.Fatalf("image value %q was accepted", value)
		}
	}
}

func TestRuntimeContainerInstanceDistinguishesPodReplacement(t *testing.T) {
	slot := testSlot("workload-uid", "backend")
	a := RuntimeContainerInstance{Slot: slot, PodUID: "pod-a", ContainerID: "container-a"}
	b := RuntimeContainerInstance{Slot: slot, PodUID: "pod-b", ContainerID: "container-b"}
	if !a.Valid() || !b.Valid() {
		t.Fatal("concrete runtime instances should be valid")
	}
	if a == b {
		t.Fatal("Pod replacement was not representable as a new runtime instance")
	}
	if a.Slot != b.Slot {
		t.Fatal("Pod replacement incorrectly changed logical container slot")
	}
}
