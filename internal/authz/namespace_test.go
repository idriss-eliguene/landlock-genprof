// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package authz

import (
	"context"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestDiscoverNamespacesFallsBackToExplicitOnlyOnForbidden(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "namespaces", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "", nil)
	})
	result, err := DiscoverNamespaces(context.Background(), client)
	if err != nil || result.Mode != NamespaceExplicitOnly || result.CanListNamespaces || len(result.Namespaces) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestDiscoverNamespacesReturnsAuthorizedNames(t *testing.T) {
	client := fake.NewSimpleClientset()
	client = fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "payments"}}, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "platform"}})
	result, err := DiscoverNamespaces(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != NamespaceDiscovered || !result.CanListNamespaces || len(result.Namespaces) != 2 || result.Namespaces[0] != "payments" {
		t.Fatalf("result=%+v", result)
	}
}

func TestExplicitNamespaceAccessUsesNamespacedSSAR(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action clienttesting.Action) (bool, runtime.Object, error) {
		req := action.(clienttesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		if req.Spec.ResourceAttributes.Namespace != "payments" {
			t.Fatalf("namespace=%q", req.Spec.ResourceAttributes.Namespace)
		}
		return true, &authorizationv1.SelfSubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: req.Spec.ResourceAttributes.Resource == "pods"}}, nil
	})
	caps, err := ExplicitNamespaceAccess(context.Background(), client, "payments")
	if err != nil {
		t.Fatal(err)
	}
	if !caps[WorkloadView] || caps[ProposalApply] {
		t.Fatalf("caps=%+v", caps)
	}
}
