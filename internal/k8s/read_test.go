// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var _ WorkbenchReadCapability = (*ReadSession)(nil)

func testReadSession(t *testing.T, objects ...runtime.Object) *ReadSession {
	t.Helper()
	core := kubefake.NewSimpleClientset()
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
	disc := core.Discovery().(*fake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{
		{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{
			{Name: "deployments"}, {Name: "statefulsets"}, {Name: "daemonsets"}, {Name: "replicasets"},
		}},
		{GroupVersion: "landlockgenprof.io/v1alpha1", APIResources: []metav1.APIResource{
			{Name: "securityprofileproposals"}, {Name: "traininghistories"},
		}},
		{GroupVersion: "networking.k8s.io/v1", APIResources: []metav1.APIResource{{Name: "networkpolicies"}}},
	}
	session, err := NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func TestReadCapabilityHasNoMutationSurface(t *testing.T) {
	session := testReadSession(t)
	if got := session.SessionIdentity().Namespace; got != "team-a" {
		t.Fatalf("namespace = %q, want team-a", got)
	}
	// The compile-time assertion above is intentional: the public type has
	// named reads only and no kubernetes.Interface/dynamic.Interface accessor.
}

func TestReadSessionRequiresBoundedNamespace(t *testing.T) {
	_, err := NewReadSession(&rest.Config{Host: "https://cluster.example"}, "")
	if err == nil {
		t.Fatal("empty namespace should be rejected")
	}
}

func TestReadSessionPinsSelectedKubeconfigContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "one"
	cfg.Contexts["one"] = &clientcmdapi.Context{Cluster: "cluster-one"}
	cfg.Contexts["two"] = &clientcmdapi.Context{Cluster: "cluster-two"}
	cfg.Clusters["cluster-one"] = &clientcmdapi.Cluster{Server: "https://one.example"}
	cfg.Clusters["cluster-two"] = &clientcmdapi.Cluster{Server: "https://two.example"}
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatal(err)
	}
	session, err := NewReadSessionFromKubeconfig(path, "one", "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if got := session.SessionIdentity(); got.Context != "one" || got.ClusterServer != "https://one.example" {
		t.Fatalf("session identity = %+v", got)
	}
	cfg.CurrentContext = "two"
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatal(err)
	}
	if got := session.SessionIdentity(); got.Context != "one" || got.ClusterServer != "https://one.example" {
		t.Fatalf("session changed after kubeconfig mutation: %+v", got)
	}
}

func TestReadSessionRejectsInvalidKubeconfig(t *testing.T) {
	_, err := NewReadSessionFromKubeconfig(filepath.Join(t.TempDir(), "missing"), "", "team-a")
	if err == nil {
		t.Fatal("missing kubeconfig should fail")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("not: a kubeconfig"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = NewReadSessionFromKubeconfig(path, "missing", "team-a")
	if err == nil {
		t.Fatal("missing context should fail")
	}
}

func TestReadCapabilityDistinguishesReadStates(t *testing.T) {
	session := testReadSession(t)
	_, err := session.GetPod(context.Background(), "missing")
	var readErr *ReadError
	if !errors.As(err, &readErr) || readErr.State != ReadNotFound {
		t.Fatalf("missing pod error = %v, state = %+v", err, readErr)
	}

	core := kubefake.NewSimpleClientset()
	core.PrependReactor("get", "pods", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "x", errors.New("denied"))
	})
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	disc := core.Discovery().(*fake.FakeDiscovery)
	session, err = NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.GetPod(context.Background(), "x")
	if !errors.As(err, &readErr) || readErr.State != ReadPermissionDenied {
		t.Fatalf("forbidden pod error = %v, state = %+v", err, readErr)
	}
	_ = disc

	session = testReadSession(t)
	_, err = session.GetSPOProfile(context.Background(), "missing")
	if !errors.As(err, &readErr) || readErr.State != ReadBackendNotInstalled {
		t.Fatalf("missing backend error = %v, state = %+v", err, readErr)
	}
	err = classifyReadError(context.DeadlineExceeded, "pods")
	if !errors.As(err, &readErr) || readErr.State != ReadTimeout {
		t.Fatalf("deadline error = %v, state = %+v", err, readErr)
	}
}
