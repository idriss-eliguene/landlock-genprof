// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package projection builds bounded, read-only security-state projections.
// Each section preserves its own proof level; the package intentionally has no
// CurrentAuthority or EffectivePolicy aggregate.
package projection

import (
	"context"
	"errors"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/idriss-eliguene/landlock-genprof/internal/association"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/internal/workload"
)

type State string

const (
	Available           State = "AVAILABLE"
	NotAvailable        State = "NOT_AVAILABLE"
	Empty               State = "EMPTY"
	BackendNotInstalled State = "BACKEND_NOT_INSTALLED"
	PermissionDenied    State = "PERMISSION_DENIED"
	NotFound            State = "NOT_FOUND"
	Timeout             State = "TIMEOUT"
	Unsupported         State = "UNSUPPORTED"
	Unknown             State = "UNKNOWN"
)

type SourceRef struct {
	Kind            string
	Namespace       string
	Name            string
	UID             string
	ResourceVersion string
}

type Section struct {
	State   State
	Reason  string
	Sources []SourceRef
}

type DeclaredConfiguration struct {
	Section
	Containers []DeclaredContainer
}

type DeclaredContainer struct {
	PodName          string
	Container        string
	CapabilitiesAdd  []string
	CapabilitiesDrop []string
	SeccompType      string
	LocalhostProfile string
}

type MaterializedPolicy struct {
	Section
	PodLockState    State
	SPOState        State
	PodLocks        []SourceRef
	SPOProfiles     []SourceRef
	NetworkPolicies []NetworkPolicy
}

type NetworkPolicy struct {
	Source       SourceRef
	PolicyTypes  []string
	IngressRules int
	EgressRules  int
}

type BindingEvidence struct {
	Section
	Bindings []Binding
}

type Binding struct {
	Backend string
	Source  SourceRef
	Target  k8s.GovernedTarget
	Detail  string
}

type EnforcementEvidence struct{ Section }
type BehavioralVerification struct{ Section }

type RuntimeEvidence struct {
	Section
	Evidence []Evidence
}

type Evidence struct {
	Source        association.Evidence
	Association   association.Result
	Compatibility association.RuntimeCompatibility
}

type DerivedPolicy struct {
	Section
	Artifacts []DerivedArtifact
}

type DerivedArtifact struct {
	Proposal string
	Backend  string
	Present  bool
}

type ProposalGovernance struct {
	Section
	Proposals []ProposalState
}

type ProposalState struct {
	Source          SourceRef
	Association     association.Result
	CandidateDigest string
	ApprovalState   string
	Applied         State
}

type WorkloadSecurityProjection struct {
	Target                 k8s.GovernedTarget
	Declared               DeclaredConfiguration
	Materialized           MaterializedPolicy
	Binding                BindingEvidence
	Enforcement            EnforcementEvidence
	BehavioralVerification BehavioralVerification
	Runtime                RuntimeEvidence
	Derived                DerivedPolicy
	Governance             ProposalGovernance
}

type Inputs struct {
	Evidence        []association.Evidence
	Proposals       []association.Proposal
	RuntimeSubjects []k8s.RuntimeSubject
}

type Service struct{ reads k8s.WorkbenchReadCapability }

func NewService(reads k8s.WorkbenchReadCapability) (*Service, error) {
	if reads == nil {
		return nil, fmt.Errorf("security projection requires a read capability")
	}
	return &Service{reads: reads}, nil
}

// Project assembles bounded reads for one discovered workload. It reads only
// the workload's selected Pod names and the selected namespace's policies;
// it does not claim an atomic cluster snapshot.
func (s *Service) Project(ctx context.Context, target k8s.GovernedTarget, item workload.Workload, in Inputs) (WorkloadSecurityProjection, error) {
	if target.Namespace == "" || target.Workload.Kind == "" || target.Workload.Name == "" || target.Container == "" {
		return WorkloadSecurityProjection{}, fmt.Errorf("invalid governed target")
	}
	if item.Target != target.Workload {
		return WorkloadSecurityProjection{}, fmt.Errorf("workload does not match governed target")
	}
	result := WorkloadSecurityProjection{Target: target}
	result.Enforcement = EnforcementEvidence{Section: Section{State: NotAvailable, Reason: "no target-bound enforcement proof is persisted"}}
	result.BehavioralVerification = BehavioralVerification{Section: Section{State: NotAvailable, Reason: "no target-bound behavioral verification is persisted"}}
	result.Runtime = RuntimeEvidence{Section: Section{State: NotAvailable, Reason: "runtime evidence must be supplied through certified association"}}
	result.Derived = DerivedPolicy{Section: Section{State: NotAvailable, Reason: "no associated proposal artifact supplied"}}
	result.Governance = ProposalGovernance{Section: Section{State: Empty, Reason: "no associated proposals"}}

	var pods []*corev1.Pod
	var firstErr error
	for _, discovered := range item.Pods {
		for _, container := range discovered.Containers {
			if container.Target == nil || !container.Target.Equal(target) {
				continue
			}
			pod, err := s.reads.GetPod(ctx, discovered.Name)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			pods = append(pods, pod)
			break
		}
	}
	if len(pods) == 0 && firstErr != nil {
		return result, firstErr
	}
	if len(pods) == 0 {
		result.Declared = DeclaredConfiguration{Section: Section{State: NotFound, Reason: "no current Pod carries the selected target"}}
	} else {
		result.Declared = declaredFromPods(target, pods)
	}

	policies, err := s.reads.ListNetworkPolicies(ctx)
	if err != nil {
		result.Materialized = MaterializedPolicy{Section: sectionFromError(err, "NetworkPolicy"), PodLockState: NotAvailable, SPOState: NotAvailable}
	} else {
		result.Materialized = materializedNetworkPolicies(policies, target.Namespace, pods)
	}
	result.Binding = bindingFromPolicies(target, result.Materialized.NetworkPolicies)
	if len(result.Binding.Bindings) == 0 {
		result.Binding = BindingEvidence{Section: Section{State: Empty, Reason: "no policy selection or attachment evidence found"}}
	}

	for _, pod := range pods {
		result.Materialized = s.addOptionalMaterialized(ctx, result.Materialized, pod.Name, target.Container)
	}
	result.Runtime = runtimeEvidence(target, in.Evidence, in.RuntimeSubjects)
	proposals := in.Proposals
	if proposals == nil {
		var proposalErr error
		proposals, proposalErr = s.loadProposals(ctx)
		if proposalErr != nil {
			result.Governance = ProposalGovernance{Section: sectionFromError(proposalErr, "SecurityProfileProposal")}
		} else {
			result.Governance, result.Derived = governanceProjection(target, proposals)
		}
	} else {
		result.Governance, result.Derived = governanceProjection(target, proposals)
	}
	return result, nil
}

func declaredFromPods(target k8s.GovernedTarget, pods []*corev1.Pod) DeclaredConfiguration {
	section := Section{State: Available, Reason: "security declarations read from current Pod objects"}
	result := DeclaredConfiguration{Section: section}
	for _, pod := range pods {
		for _, c := range pod.Spec.Containers {
			if c.Name != target.Container {
				continue
			}
			declared := DeclaredContainer{PodName: pod.Name, Container: c.Name}
			if c.SecurityContext != nil {
				if c.SecurityContext.Capabilities != nil {
					for _, value := range c.SecurityContext.Capabilities.Add {
						declared.CapabilitiesAdd = append(declared.CapabilitiesAdd, string(value))
					}
					for _, value := range c.SecurityContext.Capabilities.Drop {
						declared.CapabilitiesDrop = append(declared.CapabilitiesDrop, string(value))
					}
				}
				if c.SecurityContext.SeccompProfile != nil {
					declared.SeccompType = string(c.SecurityContext.SeccompProfile.Type)
					if c.SecurityContext.SeccompProfile.LocalhostProfile != nil {
						declared.LocalhostProfile = *c.SecurityContext.SeccompProfile.LocalhostProfile
					}
				}
			}
			if declared.SeccompType == "" && pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
				declared.SeccompType = string(pod.Spec.SecurityContext.SeccompProfile.Type)
				if pod.Spec.SecurityContext.SeccompProfile.LocalhostProfile != nil {
					declared.LocalhostProfile = *pod.Spec.SecurityContext.SeccompProfile.LocalhostProfile
				}
			}
			result.Containers = append(result.Containers, declared)
			result.Sources = append(result.Sources, sourceForPod(pod))
		}
	}
	sort.Slice(result.Containers, func(i, j int) bool { return result.Containers[i].PodName < result.Containers[j].PodName })
	return result
}

func materializedNetworkPolicies(list *unstructured.UnstructuredList, namespace string, pods []*corev1.Pod) MaterializedPolicy {
	result := MaterializedPolicy{Section: Section{State: Empty, Reason: "no NetworkPolicy selects the target Pods"}}
	for _, obj := range list.Items {
		selector := podSelector(obj)
		matches := false
		for _, pod := range pods {
			if selector.Matches(labels.Set(pod.Labels)) {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}
		item := NetworkPolicy{Source: sourceForObject(obj, "NetworkPolicy"), PolicyTypes: stringSlice(obj.Object, "spec", "policyTypes")}
		if ingress, found, _ := unstructured.NestedSlice(obj.Object, "spec", "ingress"); found {
			item.IngressRules = len(ingress)
		}
		if egress, found, _ := unstructured.NestedSlice(obj.Object, "spec", "egress"); found {
			item.EgressRules = len(egress)
		}
		result.NetworkPolicies = append(result.NetworkPolicies, item)
		result.Sources = append(result.Sources, item.Source)
	}
	if len(result.NetworkPolicies) > 0 {
		result.State = Available
		result.Reason = "NetworkPolicy objects selecting target Pods and their declared rules"
	}
	sort.Slice(result.NetworkPolicies, func(i, j int) bool {
		return result.NetworkPolicies[i].Source.Name < result.NetworkPolicies[j].Source.Name
	})
	return result
}

func bindingFromPolicies(target k8s.GovernedTarget, policies []NetworkPolicy) BindingEvidence {
	result := BindingEvidence{Section: Section{State: Empty}}
	for _, policy := range policies {
		result.Bindings = append(result.Bindings, Binding{Backend: "NetworkPolicy", Source: policy.Source, Target: target, Detail: "Pod selector selects at least one target runtime Pod"})
	}
	if len(result.Bindings) > 0 {
		result.State = Available
		result.Reason = "selector-based binding evidence"
	}
	return result
}

func (s *Service) addOptionalMaterialized(ctx context.Context, current MaterializedPolicy, podName, container string) MaterializedPolicy {
	lock, err := s.reads.GetPodLock(ctx, podName)
	if err == nil && lock != nil {
		current.PodLocks = append(current.PodLocks, sourceForObject(*lock, "LandlockProfile"))
		current.PodLockState = Available
	} else if current.PodLockState == NotAvailable || current.PodLockState == "" {
		current.PodLockState = stateFromError(err)
	}
	profileName := spobackend.GovernedProfileName(s.reads.SessionIdentity().Namespace, podName, container)
	spo, err := s.reads.GetSPOProfile(ctx, profileName)
	if err == nil && spo != nil {
		current.SPOProfiles = append(current.SPOProfiles, sourceForObject(*spo, "SeccompProfile"))
		current.SPOState = Available
	} else if current.SPOState == NotAvailable || current.SPOState == "" {
		current.SPOState = stateFromError(err)
	}
	if len(current.PodLocks) > 0 || len(current.SPOProfiles) > 0 {
		if current.State == Empty {
			current.State = Available
			current.Reason = "materialized optional backend objects were read"
		}
	}
	return current
}

func runtimeEvidence(target k8s.GovernedTarget, sources []association.Evidence, subjects []k8s.RuntimeSubject) RuntimeEvidence {
	result := RuntimeEvidence{Section: Section{State: Empty, Reason: "no associated evidence supplied"}}
	for _, source := range sources {
		associationResult := association.AssociateEvidence(target, source)
		if associationResult.State != association.Associated {
			continue
		}
		compatibility := association.RuntimeCompatibility{State: association.RuntimeUnknown, Reason: "no current RuntimeSubject supplied"}
		for _, subject := range subjects {
			if subject.Target.Equal(target) {
				compatibility = association.CompareRuntimePopulation(source, subject)
				break
			}
		}
		result.Evidence = append(result.Evidence, Evidence{Source: source, Association: associationResult, Compatibility: compatibility})
	}
	if len(result.Evidence) > 0 {
		result.State = Available
		result.Reason = "only G1.5-associated evidence is attributed"
	}
	return result
}

func (s *Service) loadProposals(ctx context.Context) ([]association.Proposal, error) {
	list, err := s.reads.ListProposals(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]association.Proposal, 0, len(list.Items))
	for _, obj := range list.Items {
		value, found, err := unstructured.NestedMap(obj.Object, "spec")
		if err != nil || !found {
			continue
		}
		var spec proposal.Spec
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value, &spec); err != nil {
			continue
		}
		var status *proposal.Status
		if raw, found, _ := unstructured.NestedMap(obj.Object, "status"); found {
			var decoded proposal.Status
			if runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &decoded) == nil {
				status = &decoded
			}
		}
		result = append(result, association.ProposalFromSpec(obj.GetNamespace(), obj.GetName(), spec, status))
	}
	return result, nil
}

func governanceProjection(target k8s.GovernedTarget, sources []association.Proposal) (ProposalGovernance, DerivedPolicy) {
	gov := ProposalGovernance{Section: Section{State: Empty, Reason: "no associated proposals"}}
	derived := DerivedPolicy{Section: Section{State: Empty, Reason: "no associated proposal artifacts"}}
	for _, source := range sources {
		result := association.AssociateProposal(target, source)
		if result.State != association.Associated {
			continue
		}
		digest, _ := proposal.CandidateDigest(source.Spec)
		approval := proposal.ApprovalDraft
		if source.Status != nil {
			approval = source.Status.ApprovalState
		}
		item := ProposalState{Source: SourceRef{Kind: "SecurityProfileProposal", Namespace: source.Namespace, Name: source.Name}, Association: result, CandidateDigest: digest, ApprovalState: string(approval), Applied: NotAvailable}
		gov.Proposals = append(gov.Proposals, item)
		for backend, present := range map[string]bool{"PodLock": source.Spec.PodLock != "", "NetworkPolicy": source.Spec.NetworkPolicy != "", "SPO SeccompProfile": source.Spec.SPOSeccompProfile != ""} {
			derived.Artifacts = append(derived.Artifacts, DerivedArtifact{Proposal: source.Name, Backend: backend, Present: present})
		}
	}
	if len(gov.Proposals) > 0 {
		gov.State = Available
		gov.Reason = "associated proposals; governance remains separate from application and enforcement"
	}
	if len(derived.Artifacts) > 0 {
		derived.State = Available
		derived.Reason = "rendered proposal artifacts are derived policy, not cluster materialization"
	}
	return gov, derived
}

func sectionFromError(err error, resource string) Section {
	var readErr *k8s.ReadError
	if errors.As(err, &readErr) {
		state := map[k8s.ReadState]State{k8s.ReadPermissionDenied: PermissionDenied, k8s.ReadBackendNotInstalled: BackendNotInstalled, k8s.ReadNotFound: NotFound, k8s.ReadTimeout: Timeout, k8s.ReadUnsupported: Unsupported}[readErr.State]
		if state == "" {
			state = Unknown
		}
		return Section{State: state, Reason: fmt.Sprintf("%s read: %v", resource, readErr)}
	}
	return Section{State: Unknown, Reason: fmt.Sprintf("%s read: %v", resource, err)}
}

func stateFromError(err error) State {
	if err == nil {
		return Unknown
	}
	return sectionFromError(err, "optional backend").State
}

func sourceForPod(pod *corev1.Pod) SourceRef {
	return SourceRef{Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, UID: string(pod.UID), ResourceVersion: pod.ResourceVersion}
}
func sourceForObject(obj unstructured.Unstructured, kind string) SourceRef {
	return SourceRef{Kind: kind, Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), ResourceVersion: obj.GetResourceVersion()}
}

func podSelector(obj unstructured.Unstructured) labels.Selector {
	value, found, err := unstructured.NestedMap(obj.Object, "spec", "podSelector")
	if err != nil || !found {
		return labels.Everything()
	}
	var selector metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value, &selector); err != nil {
		return labels.Nothing()
	}
	converted, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return labels.Nothing()
	}
	return converted
}

func stringSlice(obj map[string]interface{}, fields ...string) []string {
	values, found, _ := unstructured.NestedStringSlice(obj, fields...)
	if !found {
		return nil
	}
	return append([]string(nil), values...)
}
