// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package authz

import (
	"context"
	"errors"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type NamespaceDiscoveryMode string

const (
	NamespaceDiscovered   NamespaceDiscoveryMode = "DISCOVERED"
	NamespaceExplicitOnly NamespaceDiscoveryMode = "EXPLICIT_ONLY"
)

var ErrNamespaceExplicitRequired = errors.New("namespace listing is not permitted; explicit namespace required")
var ErrNamespaceUnavailable = errors.New("explicit namespace has no usable capability")

type NamespaceDiscoveryResult struct {
	Mode              NamespaceDiscoveryMode `json:"mode"`
	Namespaces        []string               `json:"namespaces,omitempty"`
	CanListNamespaces bool                   `json:"canListNamespaces"`
}

// DiscoverNamespaces intentionally treats a namespaced user's inability to
// list Namespace objects as a supported explicit-selection mode. Other API
// failures remain errors and are not converted into an empty list.
func DiscoverNamespaces(ctx context.Context, client kubernetes.Interface) (NamespaceDiscoveryResult, error) {
	if client == nil {
		return NamespaceDiscoveryResult{}, fmt.Errorf("namespace discovery requires a Kubernetes client")
	}
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return NamespaceDiscoveryResult{Mode: NamespaceExplicitOnly, CanListNamespaces: false}, nil
		}
		return NamespaceDiscoveryResult{}, fmt.Errorf("listing namespaces: %w", err)
	}
	result := NamespaceDiscoveryResult{Mode: NamespaceDiscovered, CanListNamespaces: true, Namespaces: make([]string, 0, len(list.Items))}
	for _, namespace := range list.Items {
		result.Namespaces = append(result.Namespaces, namespace.Name)
	}
	sort.Strings(result.Namespaces)
	return result, nil
}

// ExplicitNamespaceAccess evaluates the actual namespaced product
// capabilities. It does not require Namespace LIST or Namespace GET and does
// not claim whether an inaccessible namespace exists.
func ExplicitNamespaceAccess(ctx context.Context, client kubernetes.Interface, namespace string) (map[Capability]bool, error) {
	caps, err := DiscoverCapabilities(ctx, client, namespace)
	if err != nil {
		return nil, err
	}
	for _, allowed := range caps {
		if allowed {
			return caps, nil
		}
	}
	return caps, ErrNamespaceUnavailable
}

// NamespaceExists is deliberately not used as an authorization oracle. It is
// provided only for callers that already have an independently authorized
// Namespace GET and need to classify a direct API result.
func NamespaceExists(ctx context.Context, client kubernetes.Interface, name string) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("namespace lookup requires a Kubernetes client")
	}
	_, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}
