package reconciliation

import (
	"fmt"
	"sort"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

// PopulationPresence distinguishes a durable TrainingHistory population from
// a subject projected only from durable Observation identity.
type PopulationPresence string

const (
	PopulationPresent  PopulationPresence = "POPULATION_PRESENT"
	PopulationNotFound PopulationPresence = "POPULATION_NOT_FOUND"
)

type EvidenceSummary struct {
	ObservationID    string
	Source           string
	EvidenceState    string
	AttributionState string
}

// EnvironmentEntry is a bounded read projection, not durable Environment
// state. PopulationFacts is populated only from a durable Population.
type EnvironmentEntry struct {
	Subject                EnvironmentSubject
	PopulationPresence     PopulationPresence
	PopulationRuns         int
	PopulationCapabilities []string
	ObservationRefs        []string
	ObservationEvidence    []EvidenceSummary
	Derivation             DerivationState
	Governance             GovernanceState
	Application            ApplicationState
	Structural             StructuralKnowledge
	Behavioral             BehavioralVerification
	Attention              []AttentionReason
}

// EnvironmentProjection is deterministic for its fixed, best-effort input
// snapshot. It makes truncation explicit and is never persisted.
type EnvironmentProjection struct {
	Entries                            []EnvironmentEntry
	TotalCount                         int
	Truncated                          bool
	UnattributedFailedObservationCount int
}

// EnvironmentInputs are bulk-loaded domain objects. The projector performs
// no API reads, per-row GETs, cache operations, or writes.
type EnvironmentInputs struct {
	Populations      []history.Population
	Observations     []observationdomain.Observation
	Proposals        []ProposalInput
	ApplyAttempts    []ApplyAttemptInput
	RollbackAttempts []RollbackAttemptInput
	Limit            int
}

type environmentKey struct {
	Scope         history.PopulationScope
	Target        string
	Container     string
	ImageIdentity string
	BinaryPath    string
}

type environmentAccumulator struct {
	subject      EnvironmentSubject
	population   *history.Population
	observations map[string]observationdomain.Observation
}

// ProjectEnvironment groups durable Population and exact-identity
// Observation inputs into one row per normalized PopulationIdentity, then
// computes G1-G3 state using only data belonging to that subject.
func ProjectEnvironment(input EnvironmentInputs) (EnvironmentProjection, error) {
	groups := make(map[environmentKey]*environmentAccumulator)
	failedUnbound := 0

	for index := range input.Populations {
		population, err := history.NormalizeLegacyScope(input.Populations[index])
		if err != nil {
			return EnvironmentProjection{}, fmt.Errorf("population %d has invalid identity: %w", index, err)
		}
		subject, err := population.Identity()
		if err != nil {
			return EnvironmentProjection{}, fmt.Errorf("population %d identity: %w", index, err)
		}
		group := ensureEnvironmentGroup(groups, subject)
		copyPopulation := population
		if group.population == nil {
			group.population = &copyPopulation
		}
	}

	for index := range input.Observations {
		observation := input.Observations[index]
		subject, bound := ObservationEnvironmentSubject(observation)
		if !bound {
			if observation.Execution().State == observationdomain.ExecutionFailed {
				failedUnbound++
			}
			continue
		}
		group := ensureEnvironmentGroup(groups, subject)
		if group.observations == nil {
			group.observations = make(map[string]observationdomain.Observation)
		}
		group.observations[string(observation.ID())] = observation
	}

	entries := make([]EnvironmentEntry, 0, len(groups))
	for _, group := range groups {
		entry, err := projectEnvironmentEntry(*group, input)
		if err != nil {
			return EnvironmentProjection{}, err
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool {
		return compareEnvironmentSubjects(entries[left].Subject, entries[right].Subject) < 0
	})

	projection := EnvironmentProjection{Entries: entries, TotalCount: len(entries), UnattributedFailedObservationCount: failedUnbound}
	if input.Limit > 0 && len(projection.Entries) > input.Limit {
		projection.Entries = projection.Entries[:input.Limit]
		projection.Truncated = true
	}
	return projection, nil
}

func ensureEnvironmentGroup(groups map[environmentKey]*environmentAccumulator, subject EnvironmentSubject) *environmentAccumulator {
	key := environmentKey{Scope: subject.Scope, Target: subject.Target, Container: subject.Container, ImageIdentity: subject.ImageIdentity, BinaryPath: subject.BinaryPath}
	if group := groups[key]; group != nil {
		return group
	}
	group := &environmentAccumulator{subject: subject, observations: make(map[string]observationdomain.Observation)}
	groups[key] = group
	return group
}

func projectEnvironmentEntry(group environmentAccumulator, input EnvironmentInputs) (EnvironmentEntry, error) {
	entry := EnvironmentEntry{Subject: group.subject, PopulationPresence: PopulationNotFound, Derivation: DerivationNoProposal, Governance: GovernanceNone, Application: ApplicationNoCurrentPolicy, Structural: StructuralNone, Behavioral: BehavioralVerificationUnknown}
	if group.population != nil {
		entry.PopulationPresence = PopulationPresent
		entry.PopulationRuns = group.population.RunsRecorded
		for _, capability := range group.population.CapabilityAccesses {
			if capability.Name != "" {
				entry.PopulationCapabilities = append(entry.PopulationCapabilities, capability.Name)
			}
		}
		sort.Strings(entry.PopulationCapabilities)
		entry.PopulationCapabilities = uniqueStrings(entry.PopulationCapabilities)
	}
	for observationID, observation := range group.observations {
		entry.ObservationRefs = append(entry.ObservationRefs, observationID)
		for _, source := range observation.Result().Sources() {
			entry.ObservationEvidence = append(entry.ObservationEvidence, EvidenceSummary{ObservationID: observationID, Source: source.Source.Name, EvidenceState: string(source.Evidence), AttributionState: string(source.Qualification.Attribution)})
		}
	}
	sort.Strings(entry.ObservationRefs)
	sort.Slice(entry.ObservationEvidence, func(left, right int) bool {
		if entry.ObservationEvidence[left].ObservationID != entry.ObservationEvidence[right].ObservationID {
			return entry.ObservationEvidence[left].ObservationID < entry.ObservationEvidence[right].ObservationID
		}
		return entry.ObservationEvidence[left].Source < entry.ObservationEvidence[right].Source
	})

	compatible := make([]ProposalInput, 0, len(input.Proposals))
	for _, proposal := range input.Proposals {
		candidate, err := proposal.Spec.CandidateV2()
		if err == nil && SubjectMatchesCandidateV2(group.subject, candidate) {
			compatible = append(compatible, proposal)
		}
	}
	candidates := make([]CandidateProposal, len(compatible))
	for index := range compatible {
		candidates[index] = compatible[index].CandidateProposal
	}
	attentionInputs := AttentionInput{Subject: group.subject, Proposals: compatible, ApplyAttempts: input.ApplyAttempts, RollbackAttempts: input.RollbackAttempts}
	if group.population != nil {
		attentionInputs.Population = *group.population
	}
	for _, observation := range group.observations {
		if observation.Execution().State == observationdomain.ExecutionFailed {
			attentionInputs.FailedObservations = append(attentionInputs.FailedObservations, observation)
		}
	}
	attention, err := EvaluateAttention(attentionInputs)
	if err != nil {
		return EnvironmentEntry{}, err
	}
	verification, err := ProjectVerification(group.subject, candidates, input.ApplyAttempts, input.RollbackAttempts)
	if err != nil {
		return EnvironmentEntry{}, err
	}
	entry.Derivation = verification.Derivation
	entry.Governance = verification.Governance
	entry.Application = verification.Application
	entry.Structural = verification.Structural
	entry.Behavioral = verification.Behavioral
	entry.Attention = attention.Reasons
	return entry, nil
}

func compareEnvironmentSubjects(left, right EnvironmentSubject) int {
	values := [][2]string{{string(left.Scope), string(right.Scope)}, {left.Target, right.Target}, {left.Container, right.Container}, {left.ImageIdentity, right.ImageIdentity}, {left.BinaryPath, right.BinaryPath}}
	for _, value := range values {
		if value[0] < value[1] {
			return -1
		}
		if value[0] > value[1] {
			return 1
		}
	}
	return 0
}
