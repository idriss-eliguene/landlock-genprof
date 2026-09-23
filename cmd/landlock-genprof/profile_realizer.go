package main

import (
	"context"
	"fmt"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// authorizeClusterScopedProfile is the application-side contract for the
// separate profile-realizer identity. The identity is never used for
// proposal reads or namespaced mutations. The approved proposal digest is
// checked by applyproposal before this callback and the target tuple below
// prevents a forged profile reference or cross-namespace collision.
func authorizeClusterScopedProfile(_ context.Context, namespace, _ string, _ string, target k8s.GovernedTarget, obj *unstructured.Unstructured) error {
	if obj == nil || obj.GroupVersionKind() != spobackend.SeccompProfileGVK() {
		return fmt.Errorf("unexpected cluster-scoped resource")
	}
	if obj.GetNamespace() != "" || namespace == "" || target.Namespace != namespace {
		return fmt.Errorf("cluster-scoped profile namespace binding is invalid")
	}
	if obj.GetName() != spobackend.GovernedProfileName(target.Namespace, target.Workload.Name, target.Container) {
		return fmt.Errorf("profile name is not bound to the approved target")
	}
	annotations := obj.GetAnnotations()
	want := spobackend.OwnershipAnnotations(target.Namespace, target.Workload.Name, target.Container)
	for key, value := range want {
		if annotations[key] != value {
			return fmt.Errorf("profile ownership annotation %q does not match the approved target", key)
		}
	}
	return nil
}

func profileClientOrUnavailable(client dynamic.Interface) (dynamic.Interface, error) {
	if client == nil {
		return nil, fmt.Errorf("cluster-scoped profile realizer is unavailable")
	}
	return client, nil
}

func authorizeClusterScopedInverse(_ context.Context, namespace string, record attempt.MutationRecord, target k8s.GovernedTarget) error {
	if record.Group != spobackend.Group || record.Version != spobackend.Version || record.Kind != spobackend.SeccompProfileKind {
		return fmt.Errorf("unexpected cluster-scoped rollback resource")
	}
	if namespace == "" || target.Namespace != namespace || record.Namespace != "" {
		return fmt.Errorf("cluster-scoped rollback namespace binding is invalid")
	}
	want := spobackend.GovernedProfileName(target.Namespace, target.Workload.Name, target.Container)
	if record.Name != want {
		return fmt.Errorf("rollback profile name is not bound to the approved target")
	}
	return nil
}
