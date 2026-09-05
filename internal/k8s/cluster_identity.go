// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

// ResolveClusterIdentity reads only the kube-system Namespace UID. Locator,
// context, endpoint, and credential details are intentionally ignored.
func ResolveClusterIdentity(ctx context.Context, client kubernetes.Interface) (observationdomain.ClusterIdentity, error) {
	if client == nil {
		return observationdomain.ClusterIdentity{}, fmt.Errorf("%w: Kubernetes client is nil", observationdomain.ErrClusterIdentityUnresolved)
	}
	namespace, err := client.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return observationdomain.ClusterIdentity{}, fmt.Errorf("%w: reading kube-system Namespace: %v", observationdomain.ErrClusterIdentityUnresolved, err)
	}
	identity, err := observationdomain.NewClusterIdentity(string(namespace.UID))
	if err != nil {
		return observationdomain.ClusterIdentity{}, err
	}
	return identity, nil
}
