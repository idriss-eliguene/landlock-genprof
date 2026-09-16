// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package environment

import (
	"context"
	"fmt"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

// NamespaceContext is an immutable namespace-scoped view derived from one
// EnvironmentSession. Its capability map is a snapshot, not a grant; every
// later Kubernetes operation remains subject to Kubernetes authorization.
type NamespaceContext struct {
	parent  *EnvironmentSession
	context EnvironmentContext
	caps    map[authz.Capability]bool
}

func (n *NamespaceContext) Context() EnvironmentContext { return n.context }
func (n *NamespaceContext) Capabilities() map[authz.Capability]bool {
	result := make(map[authz.Capability]bool, len(n.caps))
	for capability, allowed := range n.caps {
		result[capability] = allowed
	}
	return result
}
func (n *NamespaceContext) Allowed(capability authz.Capability) bool { return n.caps[capability] }

func (s *EnvironmentSession) DiscoverNamespaces(ctx context.Context) (authz.NamespaceDiscoveryResult, error) {
	if s == nil || s.core == nil {
		return authz.NamespaceDiscoveryResult{}, fmt.Errorf("environment session has no Kubernetes client")
	}
	return authz.DiscoverNamespaces(ctx, s.core)
}

// SelectNamespace creates a new immutable namespace context. A namespace is
// usable only when Kubernetes grants at least one bounded product capability;
// the method never asks for cluster-wide Namespace LIST permission.
func (s *EnvironmentSession) SelectNamespace(ctx context.Context, namespace string) (*NamespaceContext, error) {
	if s == nil || s.core == nil {
		return nil, fmt.Errorf("environment session has no Kubernetes client")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	caps, err := authz.ExplicitNamespaceAccess(ctx, s.core, namespace)
	if err != nil {
		return nil, err
	}
	selected := s.context
	selected.namespace = namespace
	selected.contextVersion++
	selected.sessionID = s.context.sessionID
	return &NamespaceContext{parent: s, context: selected, caps: caps}, nil
}

// ValidateContext rejects a namespace context from another cluster, session,
// or version. It is intended as the backend guard before request dispatch.
func (n *NamespaceContext) ValidateContext(sessionID string, version uint64, cluster observationdomain.ClusterIdentity) error {
	if n == nil {
		return ErrStaleContext
	}
	if n.context.sessionID != sessionID || n.context.contextVersion != version || n.context.cluster != cluster {
		return ErrStaleContext
	}
	return nil
}
