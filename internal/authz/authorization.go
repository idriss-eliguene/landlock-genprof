// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package authz contains Kubernetes request-authorization plumbing only. It
// does not decide proposal, observation, or application semantics.
package authz

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Capability string

const (
	WorkloadView       Capability = "workload.view"
	ObservationView    Capability = "observation.view"
	ObservationOperate Capability = "observation.operate"
	ProposalView       Capability = "proposal.view"
	ProposalGenerate   Capability = "proposal.generate"
	ProposalReview     Capability = "proposal.review"
	ProposalApprove    Capability = "proposal.approve"
	ProposalApply      Capability = "proposal.apply"
	RollbackExecute    Capability = "rollback.execute"
	WorkloadRestart    Capability = "workload.restart"
	HistoryView        Capability = "history.view"
)

type Clients struct {
	Config        *rest.Config
	Core          kubernetes.Interface
	Dynamic       dynamic.Interface
	Discovery     discovery.DiscoveryInterface
	Authorization kubernetes.Interface
}

// NewImpersonatedConfig always copies the base config and rejects a caller
// that attempts to layer impersonation over an existing identity. This keeps
// each request's effective identity explicit and prevents global-client reuse.
func NewImpersonatedConfig(base *rest.Config, identity authn.Identity) (*rest.Config, error) {
	if base == nil {
		return nil, fmt.Errorf("authorization requires a base Kubernetes config")
	}
	identity, err := authn.Normalize(identity)
	if err != nil {
		return nil, err
	}
	if err := authn.ValidateImpersonationIdentity(identity); err != nil {
		return nil, err
	}
	if base.Impersonate.UserName != "" || len(base.Impersonate.Groups) != 0 || base.Impersonate.UID != "" || len(base.Impersonate.Extra) != 0 {
		return nil, fmt.Errorf("base Kubernetes config already carries impersonation")
	}
	config := rest.CopyConfig(base)
	config.Impersonate.UserName = identity.Username
	config.Impersonate.Groups = append([]string(nil), identity.Groups...)
	return config, nil
}

func NewImpersonatedClients(base *rest.Config, identity authn.Identity) (*Clients, error) {
	config, err := NewImpersonatedConfig(base, identity)
	if err != nil {
		return nil, err
	}
	core, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("constructing impersonated Kubernetes client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("constructing impersonated dynamic client: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("constructing impersonated discovery client: %w", err)
	}
	return &Clients{Config: config, Core: core, Dynamic: dyn, Discovery: disc, Authorization: core}, nil
}

// NewConfiguredClients loads a technical executor identity from an explicit
// startup kubeconfig. It never accepts request data and never adds
// impersonation to the executor config.
func NewConfiguredClients(kubeconfigPath, contextName string) (*Clients, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if strings.TrimSpace(kubeconfigPath) != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	raw, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("loading executor kubeconfig: %w", err)
	}
	if contextName == "" {
		contextName = raw.CurrentContext
	}
	if strings.TrimSpace(contextName) == "" {
		return nil, fmt.Errorf("executor kubeconfig has no context")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("selecting executor kubeconfig context %q: %w", contextName, err)
	}
	if config.Impersonate.UserName != "" || len(config.Impersonate.Groups) != 0 || config.Impersonate.UID != "" || len(config.Impersonate.Extra) != 0 {
		return nil, fmt.Errorf("executor kubeconfig must not contain impersonation")
	}
	if config.ExecProvider != nil || config.AuthProvider != nil {
		return nil, fmt.Errorf("executor kubeconfig must use direct credentials, not exec or auth-provider plugins")
	}
	core, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("constructing executor Kubernetes client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("constructing executor dynamic client: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("constructing executor discovery client: %w", err)
	}
	return &Clients{Config: rest.CopyConfig(config), Core: core, Dynamic: dyn, Discovery: disc, Authorization: core}, nil
}

type accessRule struct {
	Group, Resource, Verb string
}

var capabilityRules = map[Capability][]accessRule{
	WorkloadView:       {{Group: "", Resource: "pods", Verb: "list"}},
	ObservationView:    {{Group: "landlockgenprof.io", Resource: "observations", Verb: "list"}},
	ObservationOperate: {{Group: "landlockgenprof.io", Resource: "observations", Verb: "create"}, {Group: "landlockgenprof.io", Resource: "observations/status", Verb: "update"}},
	ProposalView:       {{Group: "landlockgenprof.io", Resource: "securityprofileproposals", Verb: "list"}},
	ProposalGenerate:   {{Group: "landlockgenprof.io", Resource: "securityprofileproposals", Verb: "create"}},
	ProposalReview:     {{Group: "landlockgenprof.io", Resource: "securityprofileproposals/status", Verb: "update"}},
	ProposalApprove:    {{Group: "landlockgenprof.io", Resource: "securityprofileproposals/status", Verb: "update"}},
	ProposalApply:      {{Group: "landlockgenprof.io", Resource: "applyattempts", Verb: "create"}},
	RollbackExecute:    {{Group: "landlockgenprof.io", Resource: "rollbackattempts", Verb: "create"}},
	WorkloadRestart:    {{Group: "", Resource: "pods", Verb: "delete"}},
	HistoryView:        {{Group: "landlockgenprof.io", Resource: "traininghistories", Verb: "list"}, {Group: "landlockgenprof.io", Resource: "observationcontributionreceipts", Verb: "list"}, {Group: "landlockgenprof.io", Resource: "applyattempts", Verb: "list"}, {Group: "landlockgenprof.io", Resource: "rollbackattempts", Verb: "list"}},
}

func Capabilities() []Capability {
	result := make([]Capability, 0, len(capabilityRules))
	for capability := range capabilityRules {
		result = append(result, capability)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// DiscoverCapabilities is advisory. Every subsequent Kubernetes operation
// still executes through the impersonated client and is authorized by the API
// server at that point.
func DiscoverCapabilities(ctx context.Context, client kubernetes.Interface, namespace string) (map[Capability]bool, error) {
	if client == nil {
		return nil, fmt.Errorf("capability discovery requires a Kubernetes client")
	}
	if errs := validation.IsDNS1123Label(namespace); len(errs) != 0 {
		return nil, fmt.Errorf("invalid namespace: %s", strings.Join(errs, "; "))
	}
	result := make(map[Capability]bool, len(capabilityRules))
	checked := make(map[accessRule]bool)
	for _, capability := range Capabilities() {
		allowed := true
		for _, rule := range capabilityRules[capability] {
			if cached, ok := checked[rule]; ok {
				if !cached {
					allowed = false
				}
				continue
			}
			started := time.Now()
			check, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authorizationv1.ResourceAttributes{Group: rule.Group, Resource: rule.Resource, Verb: rule.Verb, Namespace: namespace}}}, metav1.CreateOptions{})
			if stats := observability.RequestStatsFromContext(ctx); stats != nil {
				elapsed := time.Since(started)
				stats.AddAuthorization(elapsed)
				stats.AddKubernetes(elapsed)
			}
			if err != nil {
				return nil, fmt.Errorf("checking %s: %w", capability, err)
			}
			if !check.Status.Allowed {
				allowed = false
			}
			checked[rule] = check.Status.Allowed
		}
		result[capability] = allowed
	}
	return result, nil
}
