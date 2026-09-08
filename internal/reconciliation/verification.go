package reconciliation

import (
	"fmt"
	"sort"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

// GovernanceState is a projection of Proposal status, not a replacement for
// proposal.ApprovalState. STALE_APPROVAL and AMBIGUOUS are derived states.
type GovernanceState string

const (
	GovernanceNone          GovernanceState = "NONE"
	GovernanceDraft         GovernanceState = "DRAFT"
	GovernanceReviewed      GovernanceState = "REVIEWED"
	GovernanceApproved      GovernanceState = "APPROVED"
	GovernanceRejected      GovernanceState = "REJECTED"
	GovernanceStaleApproval GovernanceState = "STALE_APPROVAL"
	GovernanceAmbiguous     GovernanceState = "AMBIGUOUS"
)

// DerivationState describes only durable Proposal derivation shape.
type DerivationState string

const (
	DerivationNoProposal        DerivationState = "NO_PROPOSAL"
	DerivationStructurallyValid DerivationState = "STRUCTURALLY_DERIVED"
	DerivationInvalid           DerivationState = "INVALID_OR_UNUSABLE_PROPOSAL"
)

// ApplicationState is relative to the latest ordered attempt for the
// selected Proposal. It does not claim current enforcement.
type ApplicationState string

const (
	ApplicationNoCurrentPolicy               ApplicationState = "NO_CURRENT_APPROVED_POLICY"
	ApplicationNotAttempted                  ApplicationState = "NOT_ATTEMPTED"
	ApplicationInProgress                    ApplicationState = "IN_PROGRESS"
	ApplicationApplied                       ApplicationState = "APPLIED"
	ApplicationPartiallyApplied              ApplicationState = "PARTIALLY_APPLIED"
	ApplicationFailed                        ApplicationState = "FAILED"
	ApplicationOutcomeUnknown                ApplicationState = "OUTCOME_UNKNOWN"
	ApplicationRolledBack                    ApplicationState = "ROLLED_BACK"
	ApplicationRollbackPartiallyApplied      ApplicationState = "ROLLBACK_PARTIALLY_APPLIED"
	ApplicationRollbackFailed                ApplicationState = "ROLLBACK_FAILED"
	ApplicationRollbackOutcomeUnknown        ApplicationState = "ROLLBACK_OUTCOME_UNKNOWN"
	ApplicationRollbackInProgress            ApplicationState = "ROLLBACK_IN_PROGRESS"
	ApplicationNotProjectableAmbiguousPolicy ApplicationState = "NOT_PROJECTABLE_AMBIGUOUS_GOVERNANCE"
)

// StructuralKnowledge describes write-time Kubernetes object read-back only.
type StructuralKnowledge string

const (
	StructuralNone                   StructuralKnowledge = "NONE"
	StructuralNotConfirmed           StructuralKnowledge = "NOT_CONFIRMED"
	StructuralConfirmedAtApplication StructuralKnowledge = "CONFIRMED_AT_APPLICATION"
	StructuralPartial                StructuralKnowledge = "PARTIAL"
	StructuralUnknown                StructuralKnowledge = "UNKNOWN"
)

// BehavioralVerification is intentionally closed to UNKNOWN in v0.8.
type BehavioralVerification string

const BehavioralVerificationUnknown BehavioralVerification = "UNKNOWN"

// ApplyAttemptInput and RollbackAttemptInput are already-loaded durable
// objects plus adapter-supplied Kubernetes metadata. CreatedAt is metadata
// used only for deterministic ordering; the core performs no API reads.
type ApplyAttemptInput struct {
	Namespace string
	Name      string
	UID       string
	CreatedAt time.Time
	Spec      attempt.Spec
	Status    attempt.Status
}

type RollbackAttemptInput struct {
	Namespace string
	Name      string
	UID       string
	CreatedAt time.Time
	Spec      attempt.RollbackSpec
	Status    attempt.Status
}

// VerificationProjection is the independent five-axis projection for one
// selected Proposal context. Proposal is nil when no single authority exists.
type VerificationProjection struct {
	Derivation             DerivationState
	Governance             GovernanceState
	Application            ApplicationState
	Structural             StructuralKnowledge
	Behavioral             BehavioralVerification
	Proposal               *CandidateProposal
	CurrentAttemptRefs     []AttemptRef
	CurrentAttemptsIgnored int
}

// AttemptRef identifies the durable attempt that supplied the current
// application projection.
type AttemptRef struct {
	Kind      string
	Namespace string
	Name      string
	UID       string
}

// ProjectVerification derives all five independent axes from loaded domain
// values. Attempts for another ProposalUID are ignored. No axis synthesizes
// a stronger state on another axis.
func ProjectVerification(subject EnvironmentSubject, candidates []CandidateProposal, applies []ApplyAttemptInput, rollbacks []RollbackAttemptInput) (VerificationProjection, error) {
	projection := VerificationProjection{
		Derivation:  DerivationNoProposal,
		Governance:  GovernanceNone,
		Application: ApplicationNoCurrentPolicy,
		Structural:  StructuralNone,
		Behavioral:  BehavioralVerificationUnknown,
	}
	if err := subject.Validate(); err != nil {
		return projection, fmt.Errorf("invalid verification subject: %w", err)
	}

	projection.Derivation, projection.Governance = deriveProposalAxes(subject, candidates)
	selection := SelectApprovedPolicy(subject, candidates)
	switch selection.State {
	case SelectionAmbiguous:
		projection.Governance = GovernanceAmbiguous
		projection.Application = ApplicationNotProjectableAmbiguousPolicy
		return projection, nil
	case SelectionSelected:
		projection.Proposal = selection.Proposal
		projection.Application = ApplicationNotAttempted
		return projectAttempts(projection, selection.Proposal.Ref.UID, applies, rollbacks), nil
	default:
		return projection, nil
	}
}

func deriveProposalAxes(subject EnvironmentSubject, candidates []CandidateProposal) (DerivationState, GovernanceState) {
	derivation := DerivationNoProposal
	governance := GovernanceNone
	for index := range candidates {
		candidate := &candidates[index]
		version, err := candidate.Spec.NormalizedCandidateVersion()
		if err != nil || version != proposal.CandidateVersionV2 {
			continue
		}
		candidateV2, err := candidate.Spec.CandidateV2()
		if err != nil || !SubjectMatchesCandidateV2(subject, candidateV2) {
			continue
		}
		derivation = DerivationStructurallyValid
		switch candidate.Status.ApprovalState {
		case proposal.ApprovalDraft:
			governance = preferGovernance(governance, GovernanceDraft)
		case proposal.ApprovalReviewed:
			governance = preferGovernance(governance, GovernanceReviewed)
		case proposal.ApprovalRejected:
			governance = preferGovernance(governance, GovernanceRejected)
		case proposal.ApprovalApproved:
			if err := proposal.ValidateApprovedCandidate(&candidate.Spec, &candidate.Status); err != nil {
				governance = preferGovernance(governance, GovernanceStaleApproval)
			} else {
				governance = preferGovernance(governance, GovernanceApproved)
			}
		}
	}
	return derivation, governance
}

func preferGovernance(current, candidate GovernanceState) GovernanceState {
	rank := map[GovernanceState]int{
		GovernanceNone: 0, GovernanceDraft: 1, GovernanceReviewed: 2,
		GovernanceRejected: 3, GovernanceStaleApproval: 4, GovernanceApproved: 5,
	}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}

type orderedAttempt struct {
	kind      string
	namespace string
	name      string
	uid       string
	started   time.Time
	created   time.Time
	apply     *ApplyAttemptInput
	rollback  *RollbackAttemptInput
}

func projectAttempts(projection VerificationProjection, proposalUID string, applies []ApplyAttemptInput, rollbacks []RollbackAttemptInput) VerificationProjection {
	ordered := make([]orderedAttempt, 0, len(applies)+len(rollbacks))
	for index := range applies {
		if applies[index].Spec.ProposalUID != proposalUID {
			projection.CurrentAttemptsIgnored++
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, applies[index].Spec.StartedAt)
		if err != nil {
			continue
		}
		ordered = append(ordered, orderedAttempt{kind: "APPLY", namespace: applies[index].Namespace, name: applies[index].Name, uid: applies[index].UID, started: started, created: applies[index].CreatedAt, apply: &applies[index]})
	}
	for index := range rollbacks {
		if rollbacks[index].Spec.ProposalUID != proposalUID {
			projection.CurrentAttemptsIgnored++
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, rollbacks[index].Spec.StartedAt)
		if err != nil {
			continue
		}
		ordered = append(ordered, orderedAttempt{kind: "ROLLBACK", namespace: rollbacks[index].Namespace, name: rollbacks[index].Name, uid: rollbacks[index].UID, started: started, created: rollbacks[index].CreatedAt, rollback: &rollbacks[index]})
	}
	if len(ordered) == 0 {
		return projection
	}
	sort.SliceStable(ordered, func(left, right int) bool { return attemptBefore(ordered[left], ordered[right]) })
	latest := ordered[0]
	if latest.apply != nil {
		projection.Application = applicationState(latest.apply.Status.State, false)
		projection.Structural = structuralKnowledge(latest.apply.Status)
		projection.CurrentAttemptRefs = []AttemptRef{{Kind: "ApplyAttempt", Namespace: latest.apply.Namespace, Name: latest.apply.Name, UID: latest.apply.UID}}
		return projection
	}
	projection.Application = applicationState(latest.rollback.Status.State, true)
	projection.Structural = structuralKnowledge(latest.rollback.Status)
	projection.CurrentAttemptRefs = []AttemptRef{{Kind: "RollbackAttempt", Namespace: latest.rollback.Namespace, Name: latest.rollback.Name, UID: latest.rollback.UID}}
	return projection
}

func attemptBefore(left, right orderedAttempt) bool {
	if !left.started.Equal(right.started) {
		return left.started.After(right.started)
	}
	if !left.created.Equal(right.created) {
		return left.created.After(right.created)
	}
	if left.namespace != right.namespace {
		return left.namespace < right.namespace
	}
	if left.name != right.name {
		return left.name < right.name
	}
	if left.uid != right.uid {
		return left.uid < right.uid
	}
	return left.kind < right.kind
}

func applicationState(state string, rollback bool) ApplicationState {
	if !rollback {
		switch state {
		case attempt.StateInProgress:
			return ApplicationInProgress
		case attempt.StateApplied:
			return ApplicationApplied
		case attempt.StatePartiallyApplied:
			return ApplicationPartiallyApplied
		case attempt.StateFailed:
			return ApplicationFailed
		case attempt.StateOutcomeUnknown:
			return ApplicationOutcomeUnknown
		}
		return ApplicationOutcomeUnknown
	}
	switch state {
	case attempt.StateInProgress:
		return ApplicationRollbackInProgress
	case attempt.StateApplied:
		return ApplicationRolledBack
	case attempt.StatePartiallyApplied:
		return ApplicationRollbackPartiallyApplied
	case attempt.StateFailed:
		return ApplicationRollbackFailed
	case attempt.StateOutcomeUnknown:
		return ApplicationRollbackOutcomeUnknown
	}
	return ApplicationRollbackOutcomeUnknown
}

func structuralKnowledge(status attempt.Status) StructuralKnowledge {
	if len(status.Mutations) == 0 {
		return StructuralNone
	}
	confirmed := 0
	unknown := status.State == attempt.StateOutcomeUnknown
	for _, mutation := range status.Mutations {
		switch mutation.Result {
		case attempt.ResultUnknown:
			unknown = true
		case attempt.ResultSucceeded:
			if mutation.ObservedAfter != "" && mutation.ObservedAfterDigest != "" && mutation.IntendedAfterDigest != "" && mutation.ObservedAfterDigest == mutation.IntendedAfterDigest {
				confirmed++
			}
		}
	}
	if unknown {
		return StructuralUnknown
	}
	if confirmed == len(status.Mutations) {
		return StructuralConfirmedAtApplication
	}
	if confirmed > 0 {
		return StructuralPartial
	}
	return StructuralNotConfirmed
}
