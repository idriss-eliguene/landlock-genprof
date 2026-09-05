package k8s

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

func TestResolveClusterIdentityUsesOnlyKubeSystemUID(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: "uid-a"}})
	got, err := ResolveClusterIdentity(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if got.NamespaceUID != "uid-a" {
		t.Fatalf("identity = %#v, want kube-system UID uid-a", got)
	}
	other := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: "uid-b"}})
	otherGot, err := ResolveClusterIdentity(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	if got == otherGot {
		t.Fatal("different kube-system UIDs produced equal identities")
	}
}

func TestResolveClusterIdentityFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		client *fake.Clientset
	}{
		{name: "missing", client: fake.NewSimpleClientset()},
		{name: "empty UID", client: fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveClusterIdentity(context.Background(), tt.client)
			if !errors.Is(err, observationdomain.ErrClusterIdentityUnresolved) {
				t.Fatalf("error = %v, want ErrClusterIdentityUnresolved", err)
			}
		})
	}
}

func TestResolveClusterIdentityPropagatesForbiddenAsUnresolved(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "namespaces", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "namespaces"}, "kube-system", errors.New("denied"))
	})
	_, err := ResolveClusterIdentity(context.Background(), client)
	if !errors.Is(err, observationdomain.ErrClusterIdentityUnresolved) {
		t.Fatalf("error = %v, want ErrClusterIdentityUnresolved", err)
	}
}
