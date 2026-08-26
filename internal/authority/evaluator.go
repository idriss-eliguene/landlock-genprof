package authority

import (
	"fmt"
	"time"
)

// BackendSecurityEnvelope remains a P2 typed domain object. It is not an
// eligibility input and cannot directly establish a P3 result.
type EnvelopeDimension uint8

const (
	StartupBootstrap EnvelopeDimension = iota
	ContainerLifetime
	ProcessTree
	ExecutableSet
	WorkloadState
	ArchitectureABI
	ImageIdentity
	KernelRuntimeCompatibility
)

type EnvelopeState uint8

const (
	EnvelopeInvalid EnvelopeState = iota
	EnvelopeSatisfied
	EnvelopeUnsatisfied
	EnvelopeUnknown
)

type EnvelopeDimensionResult struct {
	Dimension EnvelopeDimension
	State     EnvelopeState
}

type BackendSecurityEnvelope struct{ dimensions []EnvelopeDimensionResult }

func NewBackendSecurityEnvelope(dimensions []EnvelopeDimensionResult) (BackendSecurityEnvelope, error) {
	if len(dimensions) != 8 {
		return BackendSecurityEnvelope{}, fmt.Errorf("invalid backend security envelope")
	}
	seen := map[EnvelopeDimension]bool{}
	for _, dimension := range dimensions {
		if dimension.Dimension > KernelRuntimeCompatibility || dimension.State < EnvelopeSatisfied || dimension.State > EnvelopeUnknown || seen[dimension.Dimension] {
			return BackendSecurityEnvelope{}, fmt.Errorf("invalid backend security envelope")
		}
		seen[dimension.Dimension] = true
	}
	return BackendSecurityEnvelope{dimensions: append([]EnvelopeDimensionResult(nil), dimensions...)}, nil
}

func (e BackendSecurityEnvelope) Evaluate() EnvelopeState {
	unknown := false
	for _, dimension := range e.dimensions {
		if dimension.State == EnvelopeUnsatisfied {
			return EnvelopeUnsatisfied
		}
		if dimension.State != EnvelopeSatisfied {
			unknown = true
		}
	}
	if unknown || len(e.dimensions) != 8 {
		return EnvelopeUnknown
	}
	return EnvelopeSatisfied
}

// EligibilityEvaluation is the complete typed P3 input. Mandatory membership
// comes exclusively from Requirements; callers cannot supply match outcomes.
type EligibilityEvaluation struct {
	Rule         TypedResolvedAuthorityRule
	Requirements ResolvedMandatoryRequirementSet
	Snapshot     EvaluationFactSnapshot
	EvaluationAt time.Time
}

// EligibilityDecision is an immutable P3 result bound to every
// security-significant evaluation input.
type EligibilityDecision struct {
	id               string
	result           EligibilityResult
	ruleRef          SemanticReference
	requirementSetID string
	attempt          ResolutionAttemptIdentity
	evaluationAt     time.Time
}

func (d EligibilityDecision) ID() string                         { return d.id }
func (d EligibilityDecision) Result() EligibilityResult          { return d.result }
func (d EligibilityDecision) RuleRef() SemanticReference         { return d.ruleRef }
func (d EligibilityDecision) RequirementSetID() string           { return d.requirementSetID }
func (d EligibilityDecision) Attempt() ResolutionAttemptIdentity { return d.attempt }
func (d EligibilityDecision) EvaluationAt() time.Time            { return d.evaluationAt }

func eligibilityDecisionIdentity(result EligibilityResult, ruleRef SemanticReference, setID string, attempt ResolutionAttemptIdentity, at time.Time) (string, error) {
	if !result.Valid() || !ruleRef.Valid() || setID == "" || !attempt.Valid() || at.IsZero() {
		return "", fmt.Errorf("invalid eligibility decision identity")
	}
	raw, err := canonical(map[string]any{
		"result":            int(result),
		"ruleRef":           referenceJSON(ruleRef),
		"requirementSetId":  setID,
		"resolutionAttempt": string(attempt),
		"evaluationAt":      at.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	})
	if err != nil {
		return "", err
	}
	return hashDomain("landlock-genprof/rfc0004/eligibility-decision/v1", raw), nil
}

func (d EligibilityDecision) Valid() bool {
	id, err := eligibilityDecisionIdentity(d.result, d.ruleRef, d.requirementSetID, d.attempt, d.evaluationAt)
	return err == nil && id == d.id
}

// EvaluateEligibility evaluates exactly Required(R) through the accepted
// P2.6-C adapter and P2.6-B matcher, then applies the closed P3 algebra.
func EvaluateEligibility(in EligibilityEvaluation) (EligibilityDecision, error) {
	if !in.Rule.Valid() || !in.Requirements.Valid() || !in.Snapshot.Valid() || in.EvaluationAt.IsZero() {
		return EligibilityDecision{}, fmt.Errorf("invalid eligibility evaluation")
	}
	if in.Requirements.RuleRef() != in.Rule.Reference() || in.Requirements.Attempt() != in.Snapshot.attempt {
		return EligibilityDecision{}, fmt.Errorf("incoherent eligibility evaluation")
	}
	setIdentity, err := in.Requirements.CanonicalBytes()
	if err != nil || hashDomain("landlock-genprof/rfc0004/resolved-mandatory-requirement-set/v1", setIdentity) != in.Requirements.ID() {
		return EligibilityDecision{}, fmt.Errorf("invalid requirement-set identity")
	}
	required := in.Requirements.Requirements()
	if len(required) == 0 {
		return EligibilityDecision{}, fmt.Errorf("empty mandatory requirement set")
	}
	refuted, unresolved := false, false
	for _, requirement := range required {
		request, err := in.Requirements.MatchRequest(requirement, in.EvaluationAt)
		if err != nil {
			return EligibilityDecision{}, fmt.Errorf("invalid mandatory requirement: %w", err)
		}
		match, err := MatchSnapshot(request, in.Snapshot)
		if err != nil || !match.outcome.Valid() {
			if err == nil {
				err = fmt.Errorf("invalid requirement match")
			}
			return EligibilityDecision{}, err
		}
		switch match.outcome {
		case MatchSatisfied:
		case MatchRefuted:
			refuted = true
		case MatchUnknown, MatchNonMatching:
			unresolved = true
		default:
			return EligibilityDecision{}, fmt.Errorf("invalid requirement match outcome")
		}
	}
	result := EligibilityEligible
	if refuted {
		result = EligibilityIneligible
	} else if unresolved {
		result = EligibilityUnknown
	}
	id, err := eligibilityDecisionIdentity(result, in.Rule.Reference(), in.Requirements.ID(), in.Requirements.Attempt(), in.EvaluationAt)
	if err != nil {
		return EligibilityDecision{}, err
	}
	return EligibilityDecision{id: id, result: result, ruleRef: in.Rule.Reference(), requirementSetID: in.Requirements.ID(), attempt: in.Requirements.Attempt(), evaluationAt: in.EvaluationAt}, nil
}
