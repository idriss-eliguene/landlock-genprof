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
	Containers   []DeclaredContainer
	Observations []PodReadObservation
}

// PodReadObservation records the outcome of one target-carrying Pod read.
// Discovery supplies the authoritative denominator, so an unreadable Pod stays
// visible instead of silently shrinking the observed population. Contributed
// is true only when that Pod's declarations reached Containers.
type PodReadObservation struct {
	Pod         SourceRef
	State       State
	Reason      string
	Contributed bool
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
	PodLockState        State
	SPOState            State
	PodLocks            []SourceRef
	SPOProfiles         []SourceRef
	NetworkPolicies     []NetworkPolicy
	PodLockObservations []OptionalBackendObservation
	SPOObservations     []OptionalBackendObservation
}

type OptionalBackendObservation struct {
	Pod     SourceRef
	State   State
	Reason  string
	Sources []SourceRef
}

type NetworkPolicy struct {
	Source       SourceRef
	MatchedPods  []SourceRef
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
	Excluded []ExcludedEvidence
}

// ExcludedEvidence records a source that was considered for this target and
// deliberately not attributed to it. It preserves the G1.5 association state
// so that "no evidence was observed" stays distinguishable from "evidence was
// observed but could not be attributed". Excluded evidence carries no
// authority and never contributes to attributed Runtime evidence.
type ExcludedEvidence struct {
	Source      association.Evidence
	Association association.Result
}

type Evidence struct {
	Source        association.Evidence
	Association   association.Result
	Compatibility []RuntimeCompatibilityObservation
}

type RuntimeCompatibilityObservation struct {
	Subject       k8s.RuntimeSubject
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
	Excluded  []ExcludedProposal
}

// ProposalExclusion says at which stage an observed proposal stopped being
// usable. Interpretation happens before association is reachable, so the two
// outcomes are deliberately distinct rather than one merged failure.
type ProposalExclusion string

const (
	ProposalNotInterpreted ProposalExclusion = "NOT_INTERPRETED"
	ProposalNotAssociated  ProposalExclusion = "NOT_ASSOCIATED"
)

// ExcludedProposal records a proposal object that was observed but did not
// contribute to attributed governance. Retaining its identity is provenance,
// never attribution: the association layer stays authoritative, and an
// excluded proposal never carries approval, application, or enforcement
// meaning for this target.
type ExcludedProposal struct {
	Source      SourceRef
	Exclusion   ProposalExclusion
	Association *association.Result
	Reason      string
}

type ProposalState struct {
	Source                  SourceRef
	Association             association.Result
	CandidateDigest         string
	ApprovedCandidateDigest string
	ApprovalBindingValid    bool
	ApprovalBindingReason   string
	ApprovalState           string
	Applied                 State
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
	if target.Namespace != s.reads.SessionIdentity().Namespace {
		return WorkloadSecurityProjection{}, fmt.Errorf("governed target namespace %q differs from read session namespace %q", target.Namespace, s.reads.SessionIdentity().Namespace)
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

	// Discovery already enumerated every Pod carrying this target, so that set
	// is the authoritative denominator for completeness. A read failure on one
	// Pod must not silently reduce it.
	relevant := make([]string, 0, len(item.Pods))
	for _, discovered := range item.Pods {
		for _, container := range discovered.Containers {
			if container.Target != nil && container.Target.Equal(target) {
				relevant = append(relevant, discovered.Name)
				break
			}
		}
	}
	sort.Strings(relevant)

	var pods []*corev1.Pod
	var firstErr error
	observations := make([]PodReadObservation, 0, len(relevant))
	for _, name := range relevant {
		pod, err := s.reads.GetPod(ctx, name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			failure := sectionFromError(err, "Pod")
			observations = append(observations, PodReadObservation{
				Pod:    SourceRef{Kind: "Pod", Namespace: target.Namespace, Name: name},
				State:  failure.State,
				Reason: failure.Reason,
			})
			continue
		}
		pods = append(pods, pod)
		observations = append(observations, PodReadObservation{Pod: sourceForPod(pod), State: Available, Reason: "Pod object read"})
	}
	if len(pods) == 0 && firstErr != nil {
		return result, firstErr
	}
	if len(relevant) == 0 {
		result.Declared = DeclaredConfiguration{Section: Section{State: NotFound, Reason: "no current Pod carries the selected target"}}
	} else {
		result.Declared = declaredFromPods(target, pods)
	}
	result.Declared.Observations = markContributingPods(observations, result.Declared.Sources)
	if len(pods) > 0 && len(pods) < len(relevant) {
		result.Declared.State = Unknown
		result.Declared.Reason = fmt.Sprintf("declarations read from %d of %d target-carrying Pods; %d unreadable", len(pods), len(relevant), len(relevant)-len(pods))
	}

	policies, err := s.reads.ListNetworkPolicies(ctx)
	if err != nil {
		result.Materialized = MaterializedPolicy{Section: sectionFromError(err, "NetworkPolicy"), PodLockState: NotAvailable, SPOState: NotAvailable}
	} else {
		result.Materialized = materializedNetworkPolicies(policies, pods)
	}
	result.Binding = BindingEvidence{Section: Section{State: NotAvailable, Reason: "no backend-specific attachment proof is persisted"}}

	for _, pod := range pods {
		result.Materialized = s.addOptionalMaterialized(ctx, result.Materialized, pod.Name, target.Container)
	}
	result.Runtime = runtimeEvidence(target, in.Evidence, in.RuntimeSubjects)
	proposals := in.Proposals
	if proposals == nil {
		loaded, uninterpreted, proposalErr := s.loadProposals(ctx)
		if proposalErr != nil {
			result.Governance = ProposalGovernance{Section: sectionFromError(proposalErr, "SecurityProfileProposal")}
		} else {
			result.Governance, result.Derived = governanceProjection(target, loaded, uninterpreted)
		}
	} else {
		// Caller-supplied proposals were interpreted upstream, so no
		// interpretation observations belong to this projection.
		result.Governance, result.Derived = governanceProjection(target, proposals, nil)
	}
	return result, nil
}

// markContributingPods records which successfully read Pods actually supplied
// declarations. A readable Pod can still contribute nothing when the live
// object no longer carries the selected container.
func markContributingPods(observations []PodReadObservation, sources []SourceRef) []PodReadObservation {
	contributed := make(map[string]bool, len(sources))
	for _, source := range sources {
		contributed[source.Name] = true
	}
	for i := range observations {
		if observations[i].State == Available {
			observations[i].Contributed = contributed[observations[i].Pod.Name]
		}
	}
	sort.Slice(observations, func(i, j int) bool {
		return sourceKey(observations[i].Pod) < sourceKey(observations[j].Pod)
	})
	return observations
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
	sort.Slice(result.Sources, func(i, j int) bool { return sourceKey(result.Sources[i]) < sourceKey(result.Sources[j]) })
	return result
}

func materializedNetworkPolicies(list *unstructured.UnstructuredList, pods []*corev1.Pod) MaterializedPolicy {
	result := MaterializedPolicy{Section: Section{State: Empty, Reason: "no NetworkPolicy selects the target Pods"}}
	for _, obj := range list.Items {
		selector, valid := podSelector(obj)
		if !valid {
			continue
		}
		matched := make([]SourceRef, 0)
		for _, pod := range pods {
			if selector.Matches(labels.Set(pod.Labels)) {
				matched = append(matched, sourceForPod(pod))
			}
		}
		if len(matched) == 0 {
			continue
		}
		item := NetworkPolicy{Source: sourceForObject(obj, "NetworkPolicy"), MatchedPods: matched, PolicyTypes: stringSlice(obj.Object, "spec", "policyTypes")}
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
		return sourceKey(result.NetworkPolicies[i].Source) < sourceKey(result.NetworkPolicies[j].Source)
	})
	for i := range result.NetworkPolicies {
		sort.Slice(result.NetworkPolicies[i].MatchedPods, func(a, b int) bool {
			return sourceKey(result.NetworkPolicies[i].MatchedPods[a]) < sourceKey(result.NetworkPolicies[i].MatchedPods[b])
		})
	}
	return result
}

func (s *Service) addOptionalMaterialized(ctx context.Context, current MaterializedPolicy, podName, container string) MaterializedPolicy {
	lock, err := s.reads.GetPodLock(ctx, podName)
	if err == nil && lock != nil {
		current.PodLocks = append(current.PodLocks, sourceForObject(*lock, "LandlockProfile"))
	}
	current.PodLockObservations = append(current.PodLockObservations, optionalObservation(s.reads.SessionIdentity().Namespace, podName, err, lock, "LandlockProfile"))
	profileName := spobackend.GovernedProfileName(s.reads.SessionIdentity().Namespace, podName, container)
	spo, err := s.reads.GetSPOProfile(ctx, profileName)
	if err == nil && spo != nil {
		current.SPOProfiles = append(current.SPOProfiles, sourceForObject(*spo, "SeccompProfile"))
	}
	current.SPOObservations = append(current.SPOObservations, optionalObservation(s.reads.SessionIdentity().Namespace, podName, err, spo, "SeccompProfile"))
	current.PodLockState = summarizeOptional(current.PodLockObservations)
	current.SPOState = summarizeOptional(current.SPOObservations)
	if len(current.PodLocks) > 0 || len(current.SPOProfiles) > 0 {
		if current.State == Empty {
			current.State = Available
			current.Reason = "materialized optional backend objects were read"
		}
	}
	sort.Slice(current.PodLocks, func(i, j int) bool { return sourceKey(current.PodLocks[i]) < sourceKey(current.PodLocks[j]) })
	sort.Slice(current.SPOProfiles, func(i, j int) bool { return sourceKey(current.SPOProfiles[i]) < sourceKey(current.SPOProfiles[j]) })
	sort.Slice(current.PodLockObservations, func(i, j int) bool {
		return sourceKey(current.PodLockObservations[i].Pod) < sourceKey(current.PodLockObservations[j].Pod)
	})
	sort.Slice(current.SPOObservations, func(i, j int) bool {
		return sourceKey(current.SPOObservations[i].Pod) < sourceKey(current.SPOObservations[j].Pod)
	})
	return current
}

func runtimeEvidence(target k8s.GovernedTarget, sources []association.Evidence, subjects []k8s.RuntimeSubject) RuntimeEvidence {
	result := RuntimeEvidence{Section: Section{State: Empty, Reason: "no associated evidence supplied"}}
	sortedSubjects := append([]k8s.RuntimeSubject(nil), subjects...)
	sort.Slice(sortedSubjects, func(i, j int) bool {
		return runtimeSubjectKey(sortedSubjects[i]) < runtimeSubjectKey(sortedSubjects[j])
	})
	for _, source := range sources {
		associationResult := association.AssociateEvidence(target, source)
		if associationResult.State != association.Associated {
			// Attribution stays fail-closed: the source is recorded as
			// considered-and-excluded, never promoted to attributed evidence.
			result.Excluded = append(result.Excluded, ExcludedEvidence{Source: source, Association: associationResult})
			continue
		}
		compatibility := make([]RuntimeCompatibilityObservation, 0)
		for _, subject := range sortedSubjects {
			if subject.Target.Equal(target) {
				compatibility = append(compatibility, RuntimeCompatibilityObservation{Subject: subject, Compatibility: association.CompareRuntimePopulation(source, subject)})
			}
		}
		if len(compatibility) == 0 {
			compatibility = append(compatibility, RuntimeCompatibilityObservation{Compatibility: association.RuntimeCompatibility{State: association.RuntimeUnknown, Reason: "no current RuntimeSubject supplied"}})
		}
		result.Evidence = append(result.Evidence, Evidence{Source: source, Association: associationResult, Compatibility: compatibility})
	}
	sort.Slice(result.Evidence, func(i, j int) bool {
		return evidenceKey(result.Evidence[i].Source) < evidenceKey(result.Evidence[j].Source)
	})
	sort.Slice(result.Excluded, func(i, j int) bool {
		return evidenceKey(result.Excluded[i].Source) < evidenceKey(result.Excluded[j].Source)
	})
	switch {
	case len(result.Evidence) > 0:
		result.State = Available
		result.Reason = fmt.Sprintf("only G1.5-associated evidence is attributed; %d source(s) attributed, %d excluded", len(result.Evidence), len(result.Excluded))
	case len(result.Excluded) > 0:
		// Sources were observed but none could be attributed to this target.
		// That is unknown attribution, not an absence of evidence.
		result.State = Unknown
		result.Reason = fmt.Sprintf("%d evidence source(s) were considered and none could be attributed to this target", len(result.Excluded))
	}
	return result
}

// loadProposals interprets every listed proposal object. An object that
// cannot be interpreted is returned as an explicit observation rather than
// dropped, so an unreadable proposal never becomes indistinguishable from an
// absent one. Interpretation failure is fail-closed: such an object supplies
// no governance semantics.
func (s *Service) loadProposals(ctx context.Context) ([]association.Proposal, []ExcludedProposal, error) {
	list, err := s.reads.ListProposals(ctx)
	if err != nil {
		return nil, nil, err
	}
	result := make([]association.Proposal, 0, len(list.Items))
	uninterpreted := make([]ExcludedProposal, 0)
	for _, obj := range list.Items {
		source := SourceRef{Kind: "SecurityProfileProposal", Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), ResourceVersion: obj.GetResourceVersion()}
		value, found, err := unstructured.NestedMap(obj.Object, "spec")
		if err != nil {
			uninterpreted = append(uninterpreted, notInterpreted(source, fmt.Sprintf("proposal spec is not readable: %v", err)))
			continue
		}
		if !found {
			uninterpreted = append(uninterpreted, notInterpreted(source, "proposal object has no spec"))
			continue
		}
		var spec proposal.Spec
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value, &spec); err != nil {
			uninterpreted = append(uninterpreted, notInterpreted(source, fmt.Sprintf("proposal spec does not convert to a candidate: %v", err)))
			continue
		}
		var status *proposal.Status
		raw, found, err := unstructured.NestedMap(obj.Object, "status")
		if err != nil {
			uninterpreted = append(uninterpreted, notInterpreted(source, fmt.Sprintf("proposal status is not readable: %v", err)))
			continue
		}
		if found {
			var decoded proposal.Status
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &decoded); err != nil {
				// An unreadable status must not be reported as "not approved";
				// that would state a conclusion the object does not support.
				uninterpreted = append(uninterpreted, notInterpreted(source, fmt.Sprintf("proposal status does not convert, so approval state is unreadable: %v", err)))
				continue
			}
			status = &decoded
		}
		result = append(result, association.ProposalFromSpec(obj.GetNamespace(), obj.GetName(), spec, status))
	}
	return result, uninterpreted, nil
}

func notInterpreted(source SourceRef, reason string) ExcludedProposal {
	return ExcludedProposal{Source: source, Exclusion: ProposalNotInterpreted, Reason: reason}
}

func governanceProjection(target k8s.GovernedTarget, sources []association.Proposal, uninterpreted []ExcludedProposal) (ProposalGovernance, DerivedPolicy) {
	gov := ProposalGovernance{Section: Section{State: Empty, Reason: "no proposal object was observed in the read scope"}}
	derived := DerivedPolicy{Section: Section{State: Empty, Reason: "no associated proposal artifacts"}}
	gov.Excluded = append(gov.Excluded, uninterpreted...)
	for _, source := range sources {
		result := association.AssociateProposal(target, source)
		if result.State != association.Associated {
			// Attribution stays fail-closed: the proposal is recorded as
			// observed-and-excluded, never promoted to attributed governance.
			excluded := result
			gov.Excluded = append(gov.Excluded, ExcludedProposal{
				Source:      SourceRef{Kind: "SecurityProfileProposal", Namespace: source.Namespace, Name: source.Name},
				Exclusion:   ProposalNotAssociated,
				Association: &excluded,
				Reason:      result.Reason,
			})
			continue
		}
		digest, _ := proposal.CandidateDigest(source.Spec)
		approval := proposal.ApprovalDraft
		if source.Status != nil {
			approval = source.Status.ApprovalState
		}
		approvedDigest := ""
		validApproval := false
		approvalReason := "proposal is not approved for its current candidate"
		if source.Status != nil {
			approvedDigest = source.Status.ApprovedCandidateDigest
			if err := proposal.ValidateApprovedCandidate(&source.Spec, source.Status); err == nil {
				validApproval = true
				approvalReason = "approved candidate digest matches the current candidate"
			} else {
				approvalReason = err.Error()
			}
		}
		item := ProposalState{Source: SourceRef{Kind: "SecurityProfileProposal", Namespace: source.Namespace, Name: source.Name}, Association: result, CandidateDigest: digest, ApprovedCandidateDigest: approvedDigest, ApprovalBindingValid: validApproval, ApprovalBindingReason: approvalReason, ApprovalState: string(approval), Applied: NotAvailable}
		gov.Proposals = append(gov.Proposals, item)
		for _, backend := range []struct {
			name    string
			present bool
		}{{"PodLock", source.Spec.PodLock != ""}, {"NetworkPolicy", source.Spec.NetworkPolicy != ""}, {"SPO SeccompProfile", source.Spec.SPOSeccompProfile != ""}} {
			derived.Artifacts = append(derived.Artifacts, DerivedArtifact{Proposal: source.Name, Backend: backend.name, Present: backend.present})
		}
	}
	sort.Slice(gov.Proposals, func(i, j int) bool { return sourceKey(gov.Proposals[i].Source) < sourceKey(gov.Proposals[j].Source) })
	sort.Slice(derived.Artifacts, func(i, j int) bool {
		if derived.Artifacts[i].Proposal != derived.Artifacts[j].Proposal {
			return derived.Artifacts[i].Proposal < derived.Artifacts[j].Proposal
		}
		return derived.Artifacts[i].Backend < derived.Artifacts[j].Backend
	})
	sort.SliceStable(gov.Excluded, func(i, j int) bool {
		if key := sourceKey(gov.Excluded[i].Source); key != sourceKey(gov.Excluded[j].Source) {
			return key < sourceKey(gov.Excluded[j].Source)
		}
		return gov.Excluded[i].Exclusion < gov.Excluded[j].Exclusion
	})
	switch {
	case len(gov.Proposals) > 0 && len(gov.Excluded) == 0:
		gov.State = Available
		gov.Reason = "associated proposals; governance remains separate from application and enforcement"
	case len(gov.Proposals) > 0:
		// Positive facts survive; the aggregate stops short of claiming the
		// governance picture is complete.
		gov.State = Unknown
		gov.Reason = fmt.Sprintf("%d proposal(s) attributed and %d observed but excluded; governance knowledge is incomplete", len(gov.Proposals), len(gov.Excluded))
	case len(gov.Excluded) > 0:
		gov.State = Unknown
		gov.Reason = fmt.Sprintf("%d proposal(s) were observed and none could be attributed to this target", len(gov.Excluded))
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

func optionalObservation(namespace, podName string, err error, obj *unstructured.Unstructured, kind string) OptionalBackendObservation {
	observation := OptionalBackendObservation{Pod: SourceRef{Kind: "Pod", Namespace: namespace, Name: podName}}
	if err != nil {
		observation.State = sectionFromError(err, kind).State
		observation.Reason = err.Error()
		return observation
	}
	if obj == nil {
		observation.State = NotFound
		observation.Reason = kind + " object absent"
		return observation
	}
	observation.State = Available
	observation.Reason = kind + " object read"
	observation.Sources = []SourceRef{sourceForObject(*obj, kind)}
	return observation
}

func summarizeOptional(observations []OptionalBackendObservation) State {
	if len(observations) == 0 {
		return NotAvailable
	}
	state := observations[0].State
	for _, observation := range observations[1:] {
		if observation.State != state {
			return Unknown
		}
	}
	return state
}

func sourceForPod(pod *corev1.Pod) SourceRef {
	return SourceRef{Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, UID: string(pod.UID), ResourceVersion: pod.ResourceVersion}
}
func sourceForObject(obj unstructured.Unstructured, kind string) SourceRef {
	return SourceRef{Kind: kind, Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), ResourceVersion: obj.GetResourceVersion()}
}

func podSelector(obj unstructured.Unstructured) (labels.Selector, bool) {
	value, found, err := unstructured.NestedMap(obj.Object, "spec", "podSelector")
	if err != nil || !found {
		return nil, false
	}
	var selector metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value, &selector); err != nil {
		return nil, false
	}
	converted, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, false
	}
	return converted, true
}

func sourceKey(source SourceRef) string {
	return source.Kind + "\x00" + source.Namespace + "\x00" + source.Name + "\x00" + source.UID
}
func evidenceKey(source association.Evidence) string {
	target := ""
	if source.Target != nil {
		target = source.Target.LegacyString()
	}
	return target + "\x00" + source.Population.ImageIdentity + "\x00" + source.Population.BinaryPath
}
func runtimeSubjectKey(subject k8s.RuntimeSubject) string {
	return subject.PodUID + "\x00" + subject.ImageID + "\x00" + subject.BinaryPath
}

func stringSlice(obj map[string]interface{}, fields ...string) []string {
	values, found, _ := unstructured.NestedStringSlice(obj, fields...)
	if !found {
		return nil
	}
	return append([]string(nil), values...)
}
