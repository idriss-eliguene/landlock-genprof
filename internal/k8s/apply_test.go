// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
)

const exampleNetworkPolicyYAML = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nginx-demo
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: nginx-demo
  policyTypes:
    - Egress
`

func TestApply_CreatesNewResource(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	if err := Apply(context.Background(), client, "default", exampleNetworkPolicyYAML); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	got, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fetching applied NetworkPolicy: %v", err)
	}
	if got.GetName() != "nginx-demo" {
		t.Errorf("created object name = %q, want nginx-demo", got.GetName())
	}
}

// TestApply_UpdatesExistingResource checks the Create-vs-Update branch: a
// second Apply for the same object must overwrite in place, not fail on a
// missing/stale resourceVersion or error out as already-exists — same
// create-or-update contract as internal/proposal.Save.
func TestApply_UpdatesExistingResource(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	if err := Apply(context.Background(), client, "default", exampleNetworkPolicyYAML); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}

	updated := strings.Replace(exampleNetworkPolicyYAML, "app: nginx-demo", "app: nginx-demo-v2", 1)
	if err := Apply(context.Background(), client, "default", updated); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}

	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	got, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fetching updated NetworkPolicy: %v", err)
	}
	labels, _, _ := unstructured.NestedString(got.Object, "spec", "podSelector", "matchLabels", "app")
	if labels != "nginx-demo-v2" {
		t.Errorf("podSelector label after update = %q, want nginx-demo-v2 — update didn't apply", labels)
	}
}

// TestApply_NamespaceFallback checks that a manifest with no
// metadata.namespace set (PodLock/SeccompProfile YAML this tool
// generates locally never sets one — only a live-fetched PatchedManifest
// does) falls back to the namespace parameter instead of landing in ""
// or failing.
func TestApply_NamespaceFallback(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	const noNamespaceYAML = `apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
spec:
  profilesByContainer: {}
`
	if err := Apply(context.Background(), client, "staging", noNamespaceYAML); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	gvr := schema.GroupVersionResource{Group: "podlock.kubewarden.io", Version: "v1alpha1", Resource: "landlockprofiles"}
	got, err := client.Resource(gvr).Namespace("staging").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fetching applied LandlockProfile in fallback namespace: %v", err)
	}
	if got.GetNamespace() != "staging" {
		t.Errorf("applied object namespace = %q, want staging", got.GetNamespace())
	}
}

func TestApply_LandlockProfilePreservesSemanticStructure(t *testing.T) {
	expected := podlock.LandlockProfile{
		APIVersion: "podlock.kubewarden.io/v1alpha1",
		Kind:       "LandlockProfile",
		Metadata:   podlock.Metadata{Name: "worker-profile", Namespace: "team-a"},
		Spec: podlock.LandlockProfileSpec{ProfilesByContainer: map[string]podlock.ProfileByBinary{
			"api": {
				"/usr/bin/api": {
					ReadOnly:      []string{"/etc/ssl/certs", "/usr/share/zoneinfo"},
					ReadWrite:     []string{"/tmp/api-write"},
					ReadExec:      []string{"/usr/bin/helper"},
					ReadWriteExec: []string{"/opt/runtime/tool"},
				},
				"/usr/bin/sidecar": {
					ReadOnly:  []string{"/etc/sidecar.conf"},
					ReadWrite: []string{"/var/lib/sidecar"},
				},
			},
			"worker": {
				"/usr/local/bin/worker": {
					ReadExec:      []string{"/usr/local/libexec/worker-helper"},
					ReadWriteExec: []string{"/opt/worker/runtime"},
				},
			},
		}},
	}

	manifest, err := yaml.Marshal(&expected)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := Apply(context.Background(), client, "fallback-namespace", string(manifest)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	gvr := schema.GroupVersionResource{Group: "podlock.kubewarden.io", Version: "v1alpha1", Resource: "landlockprofiles"}
	got, err := client.Resource(gvr).Namespace("team-a").Get(context.Background(), "worker-profile", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fetching persisted LandlockProfile: %v", err)
	}
	if got.GetNamespace() != "team-a" {
		t.Fatalf("persisted namespace = %q, want team-a", got.GetNamespace())
	}

	var persisted podlock.LandlockProfile
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(got.Object, &persisted); err != nil {
		t.Fatalf("converting persisted LandlockProfile: %v", err)
	}
	if got := canonicalLandlockProfile(persisted); got != canonicalLandlockProfile(expected) {
		t.Fatalf("persisted LandlockProfile semantics differ:\n got: %s\nwant: %s", got, canonicalLandlockProfile(expected))
	}
}

// canonicalLandlockProfile compares only Landlock-relevant identity and
// access semantics. Map keys and path slices are sorted so the assertion does
// not depend on YAML or API-server map ordering; server metadata is excluded.
func canonicalLandlockProfile(profile podlock.LandlockProfile) string {
	var containers []string
	for container := range profile.Spec.ProfilesByContainer {
		containers = append(containers, container)
	}
	sort.Strings(containers)
	parts := []string{profile.APIVersion, profile.Kind, profile.Metadata.Name, profile.Metadata.Namespace}
	for _, container := range containers {
		binaries := profile.Spec.ProfilesByContainer[container]
		var paths []string
		for binary := range binaries {
			paths = append(paths, binary)
		}
		sort.Strings(paths)
		for _, binary := range paths {
			p := binaries[binary]
			parts = append(parts, container, binary,
				fmt.Sprintf("ro=%v", sortedCopy(p.ReadOnly)),
				fmt.Sprintf("rw=%v", sortedCopy(p.ReadWrite)),
				fmt.Sprintf("rx=%v", sortedCopy(p.ReadExec)),
				fmt.Sprintf("rwx=%v", sortedCopy(p.ReadWriteExec)))
		}
	}
	return strings.Join(parts, "|")
}

func sortedCopy(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return copyValues
}

// TestApply_PodIsRecreatedNotUpdated confirms the delete+recreate path:
// a Pod that already exists with a *different* spec (simulating the live
// pod's real containers/volumes vs. cleanPod's minimal, merged-securityContext
// version) must end up replaced by the new content, not rejected the way
// a generic Update against the fake client would happily allow but a
// real API server forbids on most Pod fields.
func TestApply_PodIsRecreatedNotUpdated(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	const originalPodYAML = `apiVersion: v1
kind: Pod
metadata:
  name: nginx-demo
  namespace: default
spec:
  containers:
    - name: nginx
      image: nginx:alpine
`
	if err := Apply(context.Background(), client, "default", originalPodYAML); err != nil {
		t.Fatalf("initial Apply() error = %v", err)
	}

	const patchedPodYAML = `apiVersion: v1
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
          drop: ["ALL"]
`
	if err := Apply(context.Background(), client, "default", patchedPodYAML); err != nil {
		t.Fatalf("recreate Apply() error = %v", err)
	}

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	got, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("fetching recreated Pod: %v", err)
	}
	containers, _, err := unstructured.NestedSlice(got.Object, "spec", "containers")
	if err != nil || len(containers) != 1 {
		t.Fatalf("spec.containers = %v (err %v), want exactly one container", containers, err)
	}
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("containers[0] = %#v, want a map", containers[0])
	}
	drop, _, _ := unstructured.NestedStringSlice(container, "securityContext", "capabilities", "drop")
	if len(drop) != 1 || drop[0] != "ALL" {
		t.Errorf("recreated Pod securityContext.capabilities.drop = %v, want [ALL] — recreate didn't apply the new spec", drop)
	}
}

func TestApply_PodCreatesWhenAbsent(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	const podYAML = `apiVersion: v1
kind: Pod
metadata:
  name: nginx-demo
  namespace: default
spec:
  containers:
    - name: nginx
      image: nginx:alpine
`
	if err := Apply(context.Background(), client, "default", podYAML); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	if _, err := client.Resource(gvr).Namespace("default").Get(context.Background(), "nginx-demo", metav1.GetOptions{}); err != nil {
		t.Errorf("Pod was not created: %v", err)
	}
}

func TestApply_UnrecognizedKindReturnsError(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	const unknownYAML = `apiVersion: example.com/v1
kind: SomethingElse
metadata:
  name: whatever
`
	err := Apply(context.Background(), client, "default", unknownYAML)
	if err == nil {
		t.Fatal("Apply() error = nil, want an error for an unrecognized kind")
	}
	if !strings.Contains(err.Error(), "unrecognized resource kind") {
		t.Errorf("Apply() error = %q, want it to mention 'unrecognized resource kind'", err.Error())
	}
}

func TestApply_MalformedYAMLReturnsError(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	err := Apply(context.Background(), client, "default", "not: valid: yaml: at: all: [")
	if err == nil {
		t.Fatal("Apply() error = nil, want a parse error for malformed YAML")
	}
}

// TestApply_AllKnownKinds is a light smoke test over every entry in
// applyGVRs — each must resolve to a resource client and successfully
// create, catching a typo'd Group/Version/Resource in the table itself
// (the kind of mistake that would otherwise only surface against a real
// cluster, as a confusing 404).
func TestApply_AllKnownKinds(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"LandlockProfile", "apiVersion: podlock.kubewarden.io/v1alpha1\nkind: LandlockProfile\nmetadata:\n  name: x\n"},
		{"NetworkPolicy", "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: x\n"},
		{"SeccompProfile", "apiVersion: " + spobackend.APIVersion + "\nkind: SeccompProfile\nmetadata:\n  name: x\n"},
		{"Pod", "apiVersion: v1\nkind: Pod\nmetadata:\n  name: x\n"},
		{"Deployment", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: x\n"},
		{"StatefulSet", "apiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: x\n"},
		{"DaemonSet", "apiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: x\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
			if err := Apply(context.Background(), client, "default", tc.yaml); err != nil {
				t.Errorf("Apply() for %s error = %v", tc.name, err)
			}
		})
	}
}

// SeccompProfile is cluster-scoped on the targeted SPO API, so Apply must
// neither stamp a namespace onto it nor address it through a namespaced
// resource — the API server rejects both.
func TestApply_ClusterScopedResourceGetsNoNamespace(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	manifest := "apiVersion: " + spobackend.APIVersion + "\nkind: SeccompProfile\nmetadata:\n  name: cluster-scoped-x\n"

	if err := Apply(context.Background(), client, "some-namespace", manifest); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := client.Resource(spobackend.SeccompProfileGVR()).
		Get(context.Background(), "cluster-scoped-x", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("cluster-scoped Get() error = %v — the object was not applied cluster-scoped", err)
	}
	if ns := got.GetNamespace(); ns != "" {
		t.Errorf("applied object namespace = %q, want empty for a cluster-scoped resource", ns)
	}
}

// Regression guard: everything else must keep its existing namespaced
// behavior, including inheriting the fallback namespace.
func TestApply_NamespacedResourceKeepsNamespace(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	manifest := "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: np-x\n"

	if err := Apply(context.Background(), client, "fallback-ns", manifest); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	gvr := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	got, err := client.Resource(gvr).Namespace("fallback-ns").Get(context.Background(), "np-x", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("namespaced Get() error = %v", err)
	}
	if ns := got.GetNamespace(); ns != "fallback-ns" {
		t.Errorf("applied object namespace = %q, want %q", ns, "fallback-ns")
	}
}

func governedSeccompManifest(name, namespace, pod, container string) string {
	return "apiVersion: " + spobackend.APIVersion + "\n" +
		"kind: SeccompProfile\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"  annotations:\n" +
		"    " + spobackend.ManagedByAnnotation + ": " + spobackend.ManagedByValue + "\n" +
		"    " + spobackend.NameSchemeAnnotation + ": \"" + spobackend.NameScheme + "\"\n" +
		"    " + spobackend.TargetNamespaceAnnotation + ": " + namespace + "\n" +
		"    " + spobackend.TargetPodAnnotation + ": " + pod + "\n" +
		"    " + spobackend.TargetContainerAnnotation + ": " + container + "\n" +
		"spec:\n  defaultAction: SCMP_ACT_ERRNO\n"
}

// Cluster-scoped collision safety (docs/adr/0008). A name we compute can
// already be taken by an object we do not own — impossible when the
// resource was namespaced.
func TestApply_ClusterScopedCollisionSafety(t *testing.T) {
	name := spobackend.GovernedProfileName("default", "nginx-demo", "nginx")
	ours := governedSeccompManifest(name, "default", "nginx-demo", "nginx")

	t.Run("no existing object is created", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		if err := Apply(context.Background(), client, "default", ours); err != nil {
			t.Fatalf("Apply() error = %v, want creation to succeed", err)
		}
	})

	t.Run("object we own for the same workload is updated", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		if err := Apply(context.Background(), client, "default", ours); err != nil {
			t.Fatalf("first Apply() error = %v", err)
		}
		// This is what makes a retry after a readiness timeout work.
		if err := Apply(context.Background(), client, "default", ours); err != nil {
			t.Errorf("second Apply() error = %v, want an idempotent update", err)
		}
	})

	t.Run("object we own for a different workload is refused", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		// Same name, but recorded against another workload — the
		// digest-collision case the identity tuple exists to catch.
		other := governedSeccompManifest(name, "staging", "nginx-demo", "nginx")
		if err := Apply(context.Background(), client, "default", other); err != nil {
			t.Fatalf("seeding Apply() error = %v", err)
		}
		err := Apply(context.Background(), client, "default", ours)
		if err == nil {
			t.Fatal("Apply() error = nil, want refusal to overwrite a profile governing another workload")
		}
		if !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Errorf("error = %q, want a refusal", err.Error())
		}
	})

	t.Run("object we do not own is never overwritten", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		foreign := "apiVersion: " + spobackend.APIVersion + "\nkind: SeccompProfile\nmetadata:\n  name: " + name + "\nspec:\n  defaultAction: SCMP_ACT_ALLOW\n"
		if err := Apply(context.Background(), client, "default", foreign); err != nil {
			t.Fatalf("seeding Apply() error = %v", err)
		}
		err := Apply(context.Background(), client, "default", ours)
		if err == nil {
			t.Fatal("Apply() error = nil, want refusal to overwrite an unmanaged profile")
		}
		if !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Errorf("error = %q, want a refusal", err.Error())
		}

		// And the foreign object must be untouched.
		got, getErr := client.Resource(spobackend.SeccompProfileGVR()).
			Get(context.Background(), name, metav1.GetOptions{})
		if getErr != nil {
			t.Fatalf("Get() error = %v", getErr)
		}
		action, _, _ := unstructured.NestedString(got.Object, "spec", "defaultAction")
		if action != "SCMP_ACT_ALLOW" {
			t.Errorf("existing profile defaultAction = %q, want it left untouched as SCMP_ACT_ALLOW", action)
		}
	})
}
