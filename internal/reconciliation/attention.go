package reconciliation

import (
	"fmt"
	"sort"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

// AttentionCategory is a stable predicate category, not a severity.
type AttentionCategory string

const (
	AttentionCapabilityOutsideApprovedPolicy AttentionCategory = "CAPABILITY_OUTSIDE_APPROVED_POLICY"
	AttentionApprovedNotApplied              AttentionCategory = "APPROVED_NOT_APPLIED"
	AttentionApplicationStateUnknown         AttentionCategory = "APPLICATION_STATE_UNKNOWN"
	AttentionNewContributionSinceCandidate   AttentionCategory = "NEW_CONTRIBUTION_SINCE_CANDIDATE"
	AttentionObservationFailed               AttentionCategory = "OBSERVATION_FAILED"
	AttentionMultipleValidApprovedProposals  AttentionCategory = "MULTIPLE_VALID_APPROVED_PROPOSALS"
)

// AttentionReason is derived explanatory data. It has no persistence or
// authority semantics; references are only emitted when durable inputs prove
// them.
type AttentionReason struct {
	Category        AttentionCategory
	Subject         *EnvironmentSubject
	EvidenceRefs    []string
	ObservationRefs []string
	ProposalRefs    []ProposalRef
	GovernanceRefs  []ProposalRef
	ApplicationRefs []AttemptRef
	ExplanationCode string
}

type AttentionResult struct {
	Reasons []AttentionReason
}

// ProposalInput adds the adapter's immutable creation timestamp to the G1
// pure proposal value for derivation-baseline ordering.
type ProposalInput struct {
	CandidateProposal
	CreatedAt time.Time
}

// AttentionInput is already-loaded pure domain state. No function in this
// package performs Kubernetes reads or writes.
type AttentionInput struct {
	Subject            EnvironmentSubject
	Population         history.Population
	Proposals          []ProposalInput
	ApplyAttempts      []ApplyAttemptInput
	RollbackAttempts   []RollbackAttemptInput
	FailedObservations []observationdomain.Observation
}

// EvaluateAttention evaluates the frozen A-F predicates in canonical order.
func EvaluateAttention(input AttentionInput) (AttentionResult, error) {
	if err := input.Subject.Validate(); err != nil {
		return AttentionResult{}, fmt.Errorf("invalid attention subject: %w", err)
	}
	populationIdentity := history.PopulationIdentity{Scope: input.Population.Scope, Target: input.Population.Target, Container: input.Population.Container, ImageIdentity: input.Population.ImageIdentity, BinaryPath: input.Population.BinaryPath}
	populationMatches := populationIdentity.Validate() == nil && SubjectsEqual(input.Subject, populationIdentity)

	candidates := make([]CandidateProposal, len(input.Proposals))
	for index := range input.Proposals {
		candidates[index] = input.Proposals[index].CandidateProposal
	}
	selection := SelectApprovedPolicy(input.Subject, candidates)
	verification, err := ProjectVerification(input.Subject, candidates, input.ApplyAttempts, input.RollbackAttempts)
	if err != nil {
		return AttentionResult{}, err
	}
	result := AttentionResult{}

	if populationMatches && selection.State == SelectionSelected && selection.Proposal != nil {
		candidate, candidateErr := selection.Proposal.Spec.CandidateV2()
		if candidateErr == nil {
			allowed := make(map[string]struct{}, len(candidate.Artifact.ContainerCapabilities.Add))
			for _, capability := range candidate.Artifact.ContainerCapabilities.Add {
				allowed[capability] = struct{}{}
			}
			outside := make([]string, 0)
			for _, capability := range input.Population.CapabilityAccesses {
				if capability.Name == "" {
					continue
				}
				if _, ok := allowed[capability.Name]; !ok {
					outside = append(outside, capability.Name)
				}
			}
			sort.Strings(outside)
			outside = uniqueStrings(outside)
			if len(outside) > 0 {
				result.Reasons = append(result.Reasons, AttentionReason{Category: AttentionCapabilityOutsideApprovedPolicy, Subject: subjectPtr(input.Subject), EvidenceRefs: outside, GovernanceRefs: []ProposalRef{selection.Proposal.Ref}, ExplanationCode: string(AttentionCapabilityOutsideApprovedPolicy)})
			}
		}
	}

	if selection.State == SelectionSelected {
		switch verification.Application {
		case ApplicationNotAttempted, ApplicationFailed, ApplicationPartiallyApplied, ApplicationRolledBack, ApplicationRollbackFailed:
			result.Reasons = append(result.Reasons, AttentionReason{Category: AttentionApprovedNotApplied, Subject: subjectPtr(input.Subject), GovernanceRefs: []ProposalRef{selection.Proposal.Ref}, ApplicationRefs: append([]AttemptRef(nil), verification.CurrentAttemptRefs...), ExplanationCode: string(AttentionApprovedNotApplied)})
		case ApplicationOutcomeUnknown, ApplicationRollbackOutcomeUnknown:
			result.Reasons = append(result.Reasons, AttentionReason{Category: AttentionApplicationStateUnknown, Subject: subjectPtr(input.Subject), GovernanceRefs: []ProposalRef{selection.Proposal.Ref}, ApplicationRefs: append([]AttemptRef(nil), verification.CurrentAttemptRefs...), ExplanationCode: string(AttentionApplicationStateUnknown)})
		}
	}

	if baseline, ok := derivationBaseline(input.Subject, input.Proposals); ok {
		current := make(map[string]struct{}, len(input.Population.ObservationContributions))
		for _, contribution := range input.Population.ObservationContributions {
			if contribution.ObservationID != "" {
				current[contribution.ObservationID] = struct{}{}
			}
		}
		baselineIDs := make(map[string]struct{}, len(baseline.Spec.Provenance.ObservationIDs))
		if baseline.Spec.Provenance != nil {
			for _, id := range baseline.Spec.Provenance.ObservationIDs {
				baselineIDs[id] = struct{}{}
			}
		}
		delta := make([]string, 0)
		for id := range current {
			if _, included := baselineIDs[id]; !included {
				delta = append(delta, id)
			}
		}
		sort.Strings(delta)
		if len(delta) > 0 {
			result.Reasons = append(result.Reasons, AttentionReason{Category: AttentionNewContributionSinceCandidate, Subject: subjectPtr(input.Subject), ObservationRefs: delta, ProposalRefs: []ProposalRef{baseline.Ref}, ExplanationCode: string(AttentionNewContributionSinceCandidate)})
		}
	}

	for _, observation := range input.FailedObservations {
		if observation.Execution().State != observationdomain.ExecutionFailed {
			continue
		}
		subject, bound := observationEnvironmentSubject(observation)
		reason := AttentionReason{Category: AttentionObservationFailed, EvidenceRefs: []string{string(observation.ID())}, ObservationRefs: []string{string(observation.ID())}, ExplanationCode: "OBSERVATION_FAILED_UNBOUND"}
		if bound {
			reason.Subject = subjectPtr(subject)
			reason.ExplanationCode = "OBSERVATION_FAILED_BOUND"
		}
		result.Reasons = append(result.Reasons, reason)
	}

	if selection.State == SelectionAmbiguous {
		result.Reasons = append(result.Reasons, AttentionReason{Category: AttentionMultipleValidApprovedProposals, Subject: subjectPtr(input.Subject), GovernanceRefs: append([]ProposalRef(nil), selection.AmbiguousRefs...), ExplanationCode: string(AttentionMultipleValidApprovedProposals)})
	}
	sortAttentionReasons(result.Reasons)
	return result, nil
}

func derivationBaseline(subject EnvironmentSubject, proposals []ProposalInput) (ProposalInput, bool) {
	compatible := make([]ProposalInput, 0, len(proposals))
	for _, input := range proposals {
		version, err := input.Spec.NormalizedCandidateVersion()
		if err != nil || version != proposal.CandidateVersionV2 {
			continue
		}
		candidate, err := input.Spec.CandidateV2()
		if err == nil && SubjectMatchesCandidateV2(subject, candidate) {
			compatible = append(compatible, input)
		}
	}
	if len(compatible) == 0 {
		return ProposalInput{}, false
	}
	sort.SliceStable(compatible, func(left, right int) bool {
		if !compatible[left].CreatedAt.Equal(compatible[right].CreatedAt) {
			return compatible[left].CreatedAt.After(compatible[right].CreatedAt)
		}
		return compareRefs(compatible[left].Ref, compatible[right].Ref) < 0
	})
	return compatible[0], true
}

func observationEnvironmentSubject(observation observationdomain.Observation) (EnvironmentSubject, bool) {
	binding := observation.Binding()
	targets := binding.ResolvedTargets.Items()
	if len(targets) != 1 || !targets[0].Valid() || targets[0].Slot != observation.Spec().Target.Slot {
		return EnvironmentSubject{}, false
	}
	slot := targets[0].Slot
	image := ""
	for _, revision := range binding.ImageRevisionValues() {
		if revision.Slot != slot {
			continue
		}
		if _, err := observationdomain.NewContainerImageRevision(revision.Slot, revision.ImageDigest); err != nil {
			return EnvironmentSubject{}, false
		}
		if image != "" && image != revision.ImageDigest {
			return EnvironmentSubject{}, false
		}
		image = revision.ImageDigest
	}
	if targets[0].ImageRevision != nil {
		revision := targets[0].ImageRevision
		if revision.Slot != slot {
			return EnvironmentSubject{}, false
		}
		if _, err := observationdomain.NewContainerImageRevision(revision.Slot, revision.ImageDigest); err != nil {
			return EnvironmentSubject{}, false
		}
		if image != "" && image != revision.ImageDigest {
			return EnvironmentSubject{}, false
		}
		image = revision.ImageDigest
	}
	if image == "" {
		return EnvironmentSubject{}, false
	}
	target := k8s.GovernedTarget{Namespace: slot.Workload.Namespace, Workload: k8s.WorkloadRef{Group: slot.Workload.GroupKind.Group, Kind: slot.Workload.GroupKind.Kind, Name: slot.Workload.Name}, Container: slot.Container}
	identity := EnvironmentSubject{Scope: history.ScopeContainer, Target: target.LegacyString(), Container: slot.Container, ImageIdentity: image}
	return identity, identity.Validate() == nil
}

func subjectPtr(subject EnvironmentSubject) *EnvironmentSubject {
	copy := subject
	return &copy
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func sortAttentionReasons(reasons []AttentionReason) {
	rank := map[AttentionCategory]int{AttentionCapabilityOutsideApprovedPolicy: 0, AttentionApprovedNotApplied: 1, AttentionApplicationStateUnknown: 2, AttentionNewContributionSinceCandidate: 3, AttentionObservationFailed: 4, AttentionMultipleValidApprovedProposals: 5}
	sort.SliceStable(reasons, func(left, right int) bool {
		if rank[reasons[left].Category] != rank[reasons[right].Category] {
			return rank[reasons[left].Category] < rank[reasons[right].Category]
		}
		if reasons[left].ExplanationCode != reasons[right].ExplanationCode {
			return reasons[left].ExplanationCode < reasons[right].ExplanationCode
		}
		leftRef, rightRef := "", ""
		if len(reasons[left].ObservationRefs) > 0 {
			leftRef = reasons[left].ObservationRefs[0]
		}
		if len(reasons[right].ObservationRefs) > 0 {
			rightRef = reasons[right].ObservationRefs[0]
		}
		return leftRef < rightRef
	})
}
