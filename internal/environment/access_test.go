// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package environment

import (
	"context"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestNamespaceContextIsImmutableAndBoundToSession(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action clienttesting.Action) (bool, runtime.Object, error) {
		request := action.(clienttesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		return true, &authorizationv1.SelfSubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: request.Spec.ResourceAttributes.Resource == "pods"}}, nil
	})
	cluster, err := observationdomain.NewClusterIdentity("cluster-a")
	if err != nil {
		t.Fatal(err)
	}
	session := &EnvironmentSession{core: client, context: EnvironmentContext{cluster: cluster, namespace: "", sessionID: "session-a", contextVersion: 1}}
	selected, err := session.SelectNamespace(context.Background(), "payments")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Context().Namespace() != "payments" || selected.Context().ContextVersion() != 2 || !selected.Allowed(authz.WorkloadView) {
		t.Fatalf("selected context=%#v caps=%#v", selected.Context(), selected.Capabilities())
	}
	if session.Context().Namespace() != "" || session.Context().ContextVersion() != 1 {
		t.Fatalf("parent session mutated: %#v", session.Context())
	}
	if err := selected.ValidateContext("session-a", 2, cluster); err != nil {
		t.Fatal(err)
	}
	if err := selected.ValidateContext("session-a", 1, cluster); err != ErrStaleContext {
		t.Fatalf("stale version error=%v", err)
	}
	other, err := observationdomain.NewClusterIdentity("cluster-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := selected.ValidateContext("session-a", 2, other); err != ErrStaleContext {
		t.Fatalf("cross-cluster error=%v", err)
	}
}

func TestNamespaceContextRejectsNoUsableAccess(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SelfSubjectAccessReview{}, nil
	})
	session := &EnvironmentSession{core: client, context: EnvironmentContext{sessionID: "session-a", contextVersion: 1}}
	if _, err := session.SelectNamespace(context.Background(), "restricted"); err != authz.ErrNamespaceUnavailable {
		t.Fatalf("error=%v, want namespace unavailable", err)
	}
}
