package authority

import (
	"fmt"
	"reflect"
	"time"
)

type CompatibilityRequirement struct {
	Authority, Requirement, Field string
	Schema                        CompatibilitySchema
	Predicate                     CompatibilityPredicate
}

type FactFamily uint8

const (
	FamilyInvalid FactFamily = iota
	FamilyTrust
	FamilyVerification
	FamilyRevocation
	FamilyCompatibility
	FamilyCoverage
	FamilyCompleteness
	FamilyAdequacy
	FamilyCertification
)

type MatchRequest struct {
	Family                                                                                  FactFamily
	Attempt                                                                                 ResolutionAttemptIdentity
	Authority, Requirement, Subject, Backend, Context                                       string
	TypedContext                                                                            SecurityContextIdentity
	Scope                                                                                   Scope
	At                                                                                      time.Time
	Policy, Root, Property, Producer, Source, Candidate, Baseline, Schema, Predicate, Field string
	RequiredCompletenessClass                                                               CompletenessClass
	RequiredAdequacyClass                                                                   AdequacyClass
	CompatibilityRequirementRef                                                             string
	Verifier                                                                                VerifierSemanticIdentity
}

func MatchSnapshot(req MatchRequest, snap EvaluationFactSnapshot) (RequirementMatch, error) {
	// RevocationStatusRequirement.v1 has no scope operand; every other family does.
	if req.Family < FamilyTrust || req.Family > FamilyCertification || !req.Attempt.Valid() || req.Attempt != snap.attempt || req.Authority == "" || req.Requirement == "" || req.Subject == "" || (req.Family != FamilyRevocation && !req.Scope.Valid()) {
		return RequirementMatch{}, fmt.Errorf("invalid match request")
	}
	if req.At == (time.Time{}) {
		return RequirementMatch{}, fmt.Errorf("evaluation time required")
	}
	state := MatchNonMatching
	switch req.Family {
	case FamilyTrust:
		for _, f := range snap.trusts {
			if f.attempt != req.Attempt || f.subject != req.Subject || f.policy != req.Policy || f.root != req.Root || !reflect.DeepEqual(f.context, req.TypedContext) || !reflect.DeepEqual(f.scope, req.Scope) {
				continue
			}
			state = trustMatchState(f, req.At)
			break
		}
	case FamilyVerification:
		for _, f := range snap.verifications {
			if f.attempt != req.Attempt || f.subject != req.Subject || f.verifier != req.Producer || f.property != req.Property || !reflect.DeepEqual(f.context, req.TypedContext) || !reflect.DeepEqual(f.scope, req.Scope) {
				continue
			}
			state = verificationMatchState(f, req.At)
			break
		}
	case FamilyRevocation:
		for _, f := range snap.revocations {
			if f.attempt != req.Attempt || f.subject != req.Subject || f.source != req.Source {
				continue
			}
			state = revocationMatchState(f, req.At)
			break
		}
	case FamilyCompatibility:
		return MatchCompatibility(req, snap.compatibilities)
	case FamilyCoverage:
		for _, f := range snap.coverages {
			if f.attempt != req.Attempt || f.subject != req.Subject || (req.Backend != "" && f.backend != req.Backend) || (req.Source != "" && f.source != req.Source) || !validContext(req.TypedContext) || !reflect.DeepEqual(f.context, req.TypedContext) || !reflect.DeepEqual(f.scope, req.Scope) {
				continue
			}
			if CheckValidityAt(f.validity, req.At) == TemporalApplicable && f.revocation.state == RevocationNotRevoked && f.state == ScopeCovers {
				state = MatchSatisfied
			} else if f.state == ScopeCoverageUnknown {
				state = MatchUnknown
			} else {
				state = MatchRefuted
			}
			break
		}
	case FamilyCompleteness:
		if !req.RequiredCompletenessClass.Valid() {
			return RequirementMatch{}, fmt.Errorf("completeness class required")
		}
		for _, f := range snap.completeness {
			if f.attempt != req.Attempt || f.subject != req.Subject || f.class != req.RequiredCompletenessClass || !reflect.DeepEqual(f.scope, req.Scope) {
				continue
			}
			state = applicableState(f.validity, f.revocation, req.At, f.class == CompletenessClass(0))
			break
		}
	case FamilyAdequacy:
		if !req.RequiredAdequacyClass.Valid() {
			return RequirementMatch{}, fmt.Errorf("adequacy class required")
		}
		for _, f := range snap.adequacies {
			if f.attempt != req.Attempt || f.subject != req.Subject || f.class != req.RequiredAdequacyClass || !reflect.DeepEqual(f.context, req.TypedContext) || !reflect.DeepEqual(f.verifier, req.Verifier) || !reflect.DeepEqual(f.scope, req.Scope) {
				continue
			}
			state = applicableState(f.validity, f.revocation, req.At, f.class == AdequacyClass(0))
			break
		}
	case FamilyCertification:
		for _, f := range snap.certifications {
			if f.attempt != req.Attempt || f.subject != req.Subject || f.identity != req.Source || f.property != req.Property || !reflect.DeepEqual(f.context, req.TypedContext) || !reflect.DeepEqual(f.verifier, req.Verifier) || !reflect.DeepEqual(f.scope, req.Scope) {
				continue
			}
			state = applicableState(f.validity, f.revocation, req.At, false)
			break
		}
	}
	return RequirementMatch{req.Attempt, req.Authority, req.Requirement, req.Subject, req.Backend, req.Context, req.Scope, state}, nil
}

func trustMatchState(f TrustFact, at time.Time) RequirementMatchOutcome {
	if CheckValidityAt(f.validity, at) != TemporalApplicable {
		return MatchUnknown
	}
	if f.revocation.state != RevocationNotRevoked {
		return MatchUnknown
	}
	if f.state == Trusted {
		return MatchSatisfied
	}
	if f.state == Untrusted {
		return MatchRefuted
	}
	return MatchUnknown
}
func verificationMatchState(f ResolvedVerificationFact, at time.Time) RequirementMatchOutcome {
	if CheckValidityAt(f.validity, at) != TemporalApplicable || f.revocation.state != RevocationNotRevoked {
		return MatchUnknown
	}
	if f.state == VerificationFactVerified {
		return MatchSatisfied
	}
	if f.state == VerificationFactFailed {
		return MatchRefuted
	}
	return MatchUnknown
}
func revocationMatchState(f CurrentRevocationFact, at time.Time) RequirementMatchOutcome {
	if CheckValidityAt(f.validUntil, at) != TemporalApplicable {
		return MatchUnknown
	}
	if f.state == RevocationNotRevoked {
		return MatchSatisfied
	}
	if f.state == RevocationRevoked {
		return MatchRefuted
	}
	return MatchUnknown
}
func applicableState(v Validity, r CurrentRevocationFact, at time.Time, negative bool) RequirementMatchOutcome {
	if CheckValidityAt(v, at) != TemporalApplicable || r.state != RevocationNotRevoked {
		return MatchUnknown
	}
	if negative {
		return MatchRefuted
	}
	return MatchSatisfied
}

// AggregateCompatibility applies the closed fail-closed algebra to all
// requirements selected by one AuthorityRule. Empty selection is invalid;
// absence of a compatibility requirement is represented by the caller as
// NOT_APPLICABLE rather than by this function.
func AggregateCompatibility(requirements []CompatibilityRequirement, outcomes []CompatibilityOutcome) (CompatibilityOutcome, error) {
	if requirements == nil && outcomes == nil {
		return CompatibilityResultNotApplicable, nil
	}
	if len(requirements) == 0 || len(requirements) != len(outcomes) {
		return CompatibilityResultInvalid, fmt.Errorf("invalid compatibility requirement collection")
	}
	seen := map[string]CompatibilityOutcome{}
	rank := func(o CompatibilityOutcome) int {
		switch o {
		case CompatibilityResultInvalid:
			return 4
		case CompatibilityResultIncompatible:
			return 3
		case CompatibilityResultUnknown:
			return 2
		case CompatibilityResultCompatible:
			return 1
		}
		return 4
	}
	result := CompatibilityResultCompatible
	for i, r := range requirements {
		if r.Authority == "" || r.Requirement == "" || r.Field == "" || r.Schema == "" || !r.Predicate.Valid() {
			return CompatibilityResultInvalid, fmt.Errorf("invalid compatibility requirement")
		}
		if old, ok := seen[r.Authority+"\x00"+r.Requirement]; ok {
			if old != outcomes[i] {
				return CompatibilityResultInvalid, fmt.Errorf("conflicting compatibility requirement")
			}
			continue
		}
		seen[r.Authority+"\x00"+r.Requirement] = outcomes[i]
		if rank(outcomes[i]) > rank(result) {
			result = outcomes[i]
		}
	}
	return result, nil
}

// MatchCompatibility is the canonical compatibility requirement matcher.
// It requires the complete request, including explicit applicability and
// every identity-significant operand, and is also used by MatchSnapshot.
func MatchCompatibility(req MatchRequest, facts []CompatibilityFact) (RequirementMatch, error) {
	if req.Family != FamilyCompatibility || req.At.IsZero() {
		return RequirementMatch{}, fmt.Errorf("evaluation time required")
	}
	if !req.Attempt.Valid() || req.Authority == "" || req.Requirement == "" || req.Subject == "" || req.Backend == "" || req.Context == "" || req.Schema == "" || req.Predicate == "" || req.Field == "" || req.Candidate == "" || req.Baseline == "" || !req.Scope.Valid() {
		return RequirementMatch{}, fmt.Errorf("invalid matching input")
	}
	attempt, authority, requirement, subject, backend, context, scope := req.Attempt, req.Authority, req.Requirement, req.Subject, req.Backend, req.Context, req.Scope
	found, unknown, positive := false, false, false
	for _, f := range facts {
		if !f.Valid() || f.attempt != attempt {
			return RequirementMatch{}, fmt.Errorf("invalid or mixed fact")
		}
		requirementRef := req.CompatibilityRequirementRef
		if requirementRef == "" {
			requirementRef = requirement
		}
		if f.authority != authority || f.requirement != requirementRef || f.subject != subject || f.schema != req.Schema || f.predicate != req.Predicate || f.field != req.Field || f.candidate != req.Candidate || f.baseline != req.Baseline || f.context != context || f.backend != backend || !reflect.DeepEqual(f.scope, scope) {
			continue
		}
		found = true
		if CheckValidityAt(f.validity, req.At) != TemporalApplicable || f.revocation.state != RevocationNotRevoked {
			unknown = true
			continue
		}
		switch f.state {
		case CompatibilityCompatible:
			positive = true
		case CompatibilityIncompatible:
			return RequirementMatch{attempt, authority, requirement, subject, backend, context, scope, MatchRefuted}, nil
		case CompatibilityUnknown:
			unknown = true
		}
	}
	if unknown {
		return RequirementMatch{attempt, authority, requirement, subject, backend, context, scope, MatchUnknown}, nil
	}
	if positive {
		return RequirementMatch{attempt, authority, requirement, subject, backend, context, scope, MatchSatisfied}, nil
	}
	if !found {
		return RequirementMatch{attempt, authority, requirement, subject, backend, context, scope, MatchNonMatching}, nil
	}
	return RequirementMatch{attempt, authority, requirement, subject, backend, context, scope, MatchNonMatching}, nil
}
