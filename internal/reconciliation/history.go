package reconciliation

import (
	"sort"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

type HistoryEventKind string

const (
	HistoryObservationCreated       HistoryEventKind = "OBSERVATION_CREATED"
	HistoryObservationStarted       HistoryEventKind = "OBSERVATION_STARTED"
	HistoryObservationCompleted     HistoryEventKind = "OBSERVATION_COMPLETED"
	HistoryObservationFailed        HistoryEventKind = "OBSERVATION_FAILED"
	HistoryContributionPresent      HistoryEventKind = "CONTRIBUTION_PRESENT"
	HistoryProposalGenerated        HistoryEventKind = "PROPOSAL_GENERATED"
	HistoryProposalGovernance       HistoryEventKind = "PROPOSAL_CURRENT_GOVERNANCE_STATE"
	HistoryProposalApprovalSnapshot HistoryEventKind = "PROPOSAL_LAST_APPROVAL_SNAPSHOT"
	HistoryProposalApprovalStale    HistoryEventKind = "PROPOSAL_CURRENT_APPROVAL_STALE"
	HistoryApplyStarted             HistoryEventKind = "APPLY_ATTEMPT_STARTED"
	HistoryApplyCompleted           HistoryEventKind = "APPLY_ATTEMPT_COMPLETED"
	HistoryRollbackStarted          HistoryEventKind = "ROLLBACK_ATTEMPT_STARTED"
	HistoryRollbackCompleted        HistoryEventKind = "ROLLBACK_ATTEMPT_COMPLETED"
	HistoryMutationRecorded         HistoryEventKind = "MUTATION_RECORDED"
	HistoryFact                     HistoryEventKind = "HISTORY_FACT"
)

type HistoryTemporalClass string

const (
	HistoryTimestamped   HistoryTemporalClass = "TIMESTAMPED"
	HistoryUntimestamped HistoryTemporalClass = "UNTIMESTAMPED_UNORDERED"
)

type HistoryClaimTier string

const (
	HistoryEmpiricallyObserved HistoryClaimTier = "EMPIRICALLY_OBSERVED"
	HistoryStructurallyDerived HistoryClaimTier = "STRUCTURALLY_DERIVED"
	HistoryGovernanceRecorded  HistoryClaimTier = "GOVERNANCE_RECORDED"
	HistoryApplicationRecorded HistoryClaimTier = "APPLICATION_RECORDED"
	HistoryBookkeeping         HistoryClaimTier = "BOOKKEEPING"
)

const (
	HistoryLimitApprovalIncomplete = "APPROVAL_HISTORY_INCOMPLETE_BY_SCHEMA"
	HistoryLimitContributionTime   = "CONTRIBUTION_TIME_NOT_RECORDED"
	HistoryLimitDanglingReference  = "DANGLING_REFERENCE_PRESENT"
	HistoryLimitNonTransactional   = "MULTI_OBJECT_READ_NOT_TRANSACTIONAL"
	HistoryLimitTruncated          = "TRUNCATED"
)

type HistorySourceRef struct {
	Kind      string
	Namespace string
	Name      string
	UID       string
}

type HistoryEvent struct {
	Kind          HistoryEventKind
	SourceRef     HistorySourceRef
	RelatedRef    *HistorySourceRef
	Timestamp     *time.Time
	TemporalClass HistoryTemporalClass
	ClaimTier     HistoryClaimTier
	DetailCode    string
}

type HistoryProjection struct {
	TimestampedEvents  []HistoryEvent
	UntimestampedFacts []HistoryEvent
	Limitations        []string
	TotalCount         int
	Truncated          bool
}

type HistoryObservationInput struct {
	Observation observationdomain.Observation
	Namespace   string
	Name        string
	UID         string
	CreatedAt   time.Time
}

type HistoryPopulationInput struct {
	Population history.Population
	Namespace  string
	Name       string
	UID        string
}

type HistoryInputs struct {
	Populations      []HistoryPopulationInput
	Observations     []HistoryObservationInput
	Proposals        []ProposalInput
	ApplyAttempts    []ApplyAttemptInput
	RollbackAttempts []RollbackAttemptInput
	Limit            int
}

func populationRef(input HistoryPopulationInput) HistorySourceRef {
	return HistorySourceRef{Kind: "TrainingHistory", Namespace: input.Namespace, Name: input.Name, UID: input.UID}
}

func observationRef(input HistoryObservationInput) HistorySourceRef {
	return HistorySourceRef{Kind: "Observation", Namespace: input.Namespace, Name: input.Name, UID: input.UID}
}

func proposalRef(ref ProposalRef) HistorySourceRef {
	return HistorySourceRef{Kind: "SecurityProfileProposal", Namespace: ref.Namespace, Name: ref.Name, UID: ref.UID}
}

func applyRef(input ApplyAttemptInput) HistorySourceRef {
	return HistorySourceRef{Kind: "ApplyAttempt", Namespace: input.Namespace, Name: input.Name, UID: input.UID}
}

func rollbackRef(input RollbackAttemptInput) HistorySourceRef {
	return HistorySourceRef{Kind: "RollbackAttempt", Namespace: input.Namespace, Name: input.Name, UID: input.UID}
}

func ProjectPopulationHistory(subject EnvironmentSubject, input HistoryInputs) (HistoryProjection, error) {
	return projectHistory(&subject, nil, input)
}

func ProjectProposalHistory(ref ProposalRef, input HistoryInputs) (HistoryProjection, error) {
	return projectHistory(nil, &ref, input)
}

func projectHistory(subject *EnvironmentSubject, proposalSubject *ProposalRef, input HistoryInputs) (HistoryProjection, error) {
	var out HistoryProjection
	if subject != nil {
		if err := subject.Validate(); err != nil {
			return out, err
		}
	}
	if proposalSubject != nil && proposalSubject.UID == "" && proposalSubject.Name == "" {
		return out, &historySubjectError{"proposal history requires a proposal reference"}
	}

	proposalPresent := map[string]bool{}
	observationPresent := map[string]bool{}
	applyPresent := map[string]bool{}
	for _, p := range input.Proposals {
		proposalPresent[p.Ref.UID] = true
		proposalPresent[p.Ref.Namespace+"/"+p.Ref.Name] = true
	}
	for _, o := range input.Observations {
		observationPresent[string(o.Observation.ID())] = true
	}
	for _, a := range input.ApplyAttempts {
		applyPresent[a.UID] = true
	}

	selectedPopulations := input.Populations
	if subject != nil {
		selectedPopulations = nil
		for _, p := range input.Populations {
			id := history.PopulationIdentity{Scope: p.Population.Scope, Target: p.Population.Target, Container: p.Population.Container, ImageIdentity: p.Population.ImageIdentity, BinaryPath: p.Population.BinaryPath}
			if SubjectsEqual(*subject, id) {
				selectedPopulations = append(selectedPopulations, p)
			}
		}
	}
	for _, p := range selectedPopulations {
		ref := populationRef(p)
		for _, c := range p.Population.ObservationContributions {
			out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryContributionPresent, ref, HistoryBookkeeping, "CONTRIBUTION_MEMBERSHIP"))
			out.Limitations = appendUnique(out.Limitations, HistoryLimitContributionTime)
			if !observationPresent[c.ObservationID] {
				out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryFact, ref, HistoryBookkeeping, "REFERENCED_OBSERVATION_NOT_FOUND:"+c.ObservationID))
				out.Limitations = appendUnique(out.Limitations, HistoryLimitDanglingReference)
			}
		}
	}

	for _, o := range input.Observations {
		if subject != nil {
			derived, ok := ObservationEnvironmentSubject(o.Observation)
			if !ok || !SubjectsEqual(*subject, derived) {
				continue
			}
		}
		ref := observationRef(o)
		if !o.CreatedAt.IsZero() {
			t := o.CreatedAt
			out.TimestampedEvents = append(out.TimestampedEvents, event(HistoryObservationCreated, ref, &t, HistoryStructurallyDerived, "OBSERVATION_CREATED"))
		}
		exec := o.Observation.Execution()
		if !exec.StartedAt.IsZero() {
			t := exec.StartedAt
			out.TimestampedEvents = append(out.TimestampedEvents, event(HistoryObservationStarted, ref, &t, HistoryEmpiricallyObserved, "EXECUTION_STARTED"))
		}
		if !exec.CompletedAt.IsZero() && (exec.State == observationdomain.ExecutionCompleted || exec.State == observationdomain.ExecutionFailed) {
			t := exec.CompletedAt
			kind := HistoryObservationCompleted
			detail := "EXECUTION_COMPLETED"
			if exec.State == observationdomain.ExecutionFailed {
				kind = HistoryObservationFailed
				detail = "EXECUTION_FAILED"
			}
			out.TimestampedEvents = append(out.TimestampedEvents, event(kind, ref, &t, HistoryEmpiricallyObserved, detail))
		}
	}

	for _, p := range input.Proposals {
		if !proposalSelected(p, proposalSubject) {
			continue
		}
		version, _ := p.Spec.NormalizedCandidateVersion()
		ref := proposalRef(p.Ref)
		if !p.CreatedAt.IsZero() {
			t := p.CreatedAt
			out.TimestampedEvents = append(out.TimestampedEvents, event(HistoryProposalGenerated, ref, &t, HistoryStructurallyDerived, "PROPOSAL_GENERATED"))
		}
		if version == proposal.CandidateVersionV2 || proposalSubject != nil {
			out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryProposalGovernance, ref, HistoryGovernanceRecorded, string(p.Status.ApprovalState)))
			out.Limitations = appendUnique(out.Limitations, HistoryLimitApprovalIncomplete)
			if p.Status.LastApprovalSnapshot != nil {
				if t, err := time.Parse(time.RFC3339Nano, p.Status.LastApprovalSnapshot.ApprovedAt); err == nil {
					out.TimestampedEvents = append(out.TimestampedEvents, event(HistoryProposalApprovalSnapshot, ref, &t, HistoryGovernanceRecorded, "LAST_APPROVAL_SNAPSHOT"))
				} else {
					out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryProposalApprovalSnapshot, ref, HistoryGovernanceRecorded, "LAST_APPROVAL_SNAPSHOT"))
				}
			}
			if p.Status.ApprovalState == proposal.ApprovalApproved && proposal.ValidateApprovedCandidate(&p.Spec, &p.Status) != nil {
				out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryProposalApprovalStale, ref, HistoryGovernanceRecorded, "CURRENT_APPROVAL_STALE"))
			}
			if p.Spec.Provenance != nil {
				for _, id := range p.Spec.Provenance.ObservationIDs {
					if !observationPresent[id] {
						out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryFact, ref, HistoryBookkeeping, "REFERENCED_OBSERVATION_NOT_FOUND:"+id))
						out.Limitations = appendUnique(out.Limitations, HistoryLimitDanglingReference)
					}
				}
			}
		}
	}

	for _, a := range input.ApplyAttempts {
		if !attemptForHistory(a.Spec.ProposalUID, input.Proposals, proposalSubject) {
			continue
		}
		addApplyHistory(&out, a)
		if !proposalPresent[a.Spec.ProposalUID] && !proposalPresent[a.Spec.ProposalNamespace+"/"+a.Spec.ProposalName] {
			out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryFact, applyRef(a), HistoryBookkeeping, "REFERENCED_PROPOSAL_NOT_FOUND"))
			out.Limitations = appendUnique(out.Limitations, HistoryLimitDanglingReference)
		}
	}
	for _, r := range input.RollbackAttempts {
		if !attemptForHistory(r.Spec.ProposalUID, input.Proposals, proposalSubject) {
			continue
		}
		addRollbackHistory(&out, r)
		if !proposalPresent[r.Spec.ProposalUID] && !proposalPresent[r.Spec.ProposalNamespace+"/"+r.Spec.ProposalName] {
			out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryFact, rollbackRef(r), HistoryBookkeeping, "REFERENCED_PROPOSAL_NOT_FOUND"))
			out.Limitations = appendUnique(out.Limitations, HistoryLimitDanglingReference)
		}
		if r.Spec.SourceUID != "" && !applyPresent[r.Spec.SourceUID] {
			out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryFact, rollbackRef(r), HistoryBookkeeping, "SOURCE_ATTEMPT_NOT_FOUND"))
			out.Limitations = appendUnique(out.Limitations, HistoryLimitDanglingReference)
		}
	}
	if len(out.TimestampedEvents)+len(out.UntimestampedFacts) > 0 {
		out.Limitations = appendUnique(out.Limitations, HistoryLimitNonTransactional)
	}
	sortHistory(&out)
	applyHistoryLimit(&out, input.Limit)
	return out, nil
}

type historySubjectError struct{ text string }

func (e *historySubjectError) Error() string { return e.text }

func proposalSelected(p ProposalInput, ref *ProposalRef) bool {
	if ref == nil {
		return true
	}
	return p.Ref == *ref
}
func attemptForHistory(uid string, proposals []ProposalInput, ref *ProposalRef) bool {
	if ref != nil {
		return uid == ref.UID
	}
	return len(proposals) == 0 || anyProposalUID(proposals, uid)
}
func anyProposalUID(ps []ProposalInput, uid string) bool {
	for _, p := range ps {
		if p.Ref.UID == uid {
			return true
		}
	}
	return false
}
func event(kind HistoryEventKind, source HistorySourceRef, timestamp *time.Time, tier HistoryClaimTier, detail string) HistoryEvent {
	return HistoryEvent{Kind: kind, SourceRef: source, Timestamp: timestamp, TemporalClass: HistoryTimestamped, ClaimTier: tier, DetailCode: detail}
}
func fact(kind HistoryEventKind, source HistorySourceRef, tier HistoryClaimTier, detail string) HistoryEvent {
	return HistoryEvent{Kind: kind, SourceRef: source, TemporalClass: HistoryUntimestamped, ClaimTier: tier, DetailCode: detail}
}
func parseAttemptTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &t
}
func addApplyHistory(out *HistoryProjection, a ApplyAttemptInput) {
	ref := applyRef(a)
	if t := parseAttemptTime(a.Spec.StartedAt); t != nil {
		out.TimestampedEvents = append(out.TimestampedEvents, event(HistoryApplyStarted, ref, t, HistoryApplicationRecorded, "ATTEMPT_STARTED"))
	} else {
		out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryApplyStarted, ref, HistoryApplicationRecorded, "ATTEMPT_PRESENT"))
	}
	if t := parseAttemptTime(a.Status.CompletedAt); t != nil {
		out.TimestampedEvents = append(out.TimestampedEvents, event(HistoryApplyCompleted, ref, t, HistoryApplicationRecorded, a.Status.State))
	} else {
		out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryApplyCompleted, ref, HistoryApplicationRecorded, a.Status.State))
	}
	for _, m := range a.Status.Mutations {
		out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryMutationRecorded, ref, HistoryApplicationRecorded, m.Result))
	}
}
func addRollbackHistory(out *HistoryProjection, r RollbackAttemptInput) {
	ref := rollbackRef(r)
	related := applyRef(ApplyAttemptInput{Namespace: r.Spec.SourceNamespace, Name: r.Spec.SourceName, UID: r.Spec.SourceUID})
	if t := parseAttemptTime(r.Spec.StartedAt); t != nil {
		e := event(HistoryRollbackStarted, ref, t, HistoryApplicationRecorded, "ATTEMPT_STARTED")
		e.RelatedRef = &related
		out.TimestampedEvents = append(out.TimestampedEvents, e)
	} else {
		out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryRollbackStarted, ref, HistoryApplicationRecorded, "ATTEMPT_PRESENT"))
	}
	if t := parseAttemptTime(r.Status.CompletedAt); t != nil {
		e := event(HistoryRollbackCompleted, ref, t, HistoryApplicationRecorded, r.Status.State)
		e.RelatedRef = &related
		out.TimestampedEvents = append(out.TimestampedEvents, e)
	} else {
		out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryRollbackCompleted, ref, HistoryApplicationRecorded, r.Status.State))
	}
	for _, m := range r.Status.Mutations {
		out.UntimestampedFacts = append(out.UntimestampedFacts, fact(HistoryMutationRecorded, ref, HistoryApplicationRecorded, m.Result))
	}
}
func sortHistory(out *HistoryProjection) {
	sort.Slice(out.TimestampedEvents, func(i, j int) bool {
		a, b := out.TimestampedEvents[i], out.TimestampedEvents[j]
		if !a.Timestamp.Equal(*b.Timestamp) {
			return a.Timestamp.After(*b.Timestamp)
		}
		return historyEventKey(a) < historyEventKey(b)
	})
	sort.Slice(out.UntimestampedFacts, func(i, j int) bool {
		return historyEventKey(out.UntimestampedFacts[i]) < historyEventKey(out.UntimestampedFacts[j])
	})
	out.TotalCount = len(out.TimestampedEvents) + len(out.UntimestampedFacts)
}
func historyEventKey(e HistoryEvent) string {
	return string(e.Kind) + "|" + e.SourceRef.Kind + "|" + e.SourceRef.Namespace + "|" + e.SourceRef.Name + "|" + e.SourceRef.UID + "|" + e.DetailCode
}
func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	values = append(values, value)
	sort.Strings(values)
	return values
}
func applyHistoryLimit(out *HistoryProjection, limit int) {
	if limit <= 0 || out.TotalCount <= limit {
		return
	}
	keep := limit
	if len(out.TimestampedEvents) > keep {
		out.TimestampedEvents = out.TimestampedEvents[:keep]
		out.UntimestampedFacts = nil
	} else {
		keep -= len(out.TimestampedEvents)
		if len(out.UntimestampedFacts) > keep {
			out.UntimestampedFacts = out.UntimestampedFacts[:keep]
		}
	}
	out.Truncated = true
	out.Limitations = appendUnique(out.Limitations, HistoryLimitTruncated)
}
