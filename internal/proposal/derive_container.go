package proposal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
)

const (
	derivationSupported    = "SUPPORTED"
	derivationNotAvailable = "NOT_AVAILABLE"
)

// DeriveContainerCapabilityProposal derives a candidate-v2 proposal from one
// already-loaded, persisted CONTAINER population. The population identity and
// observation provenance are authoritative; no live workload lookup occurs.
func DeriveContainerCapabilityProposal(population history.Population) (Spec, error) {
	identity, err := population.Identity()
	if err != nil {
		return Spec{}, fmt.Errorf("invalid TrainingHistory population identity: %w", err)
	}
	if identity.Scope != history.ScopeContainer {
		return Spec{}, fmt.Errorf("container capability derivation requires CONTAINER scope")
	}
	if err := population.ValidateObservationMetadata(); err != nil {
		return Spec{}, fmt.Errorf("invalid TrainingHistory provenance: %w", err)
	}
	if len(population.CapabilityAccesses) == 0 {
		return Spec{}, fmt.Errorf("no attributable capability facts; candidate not available")
	}

	capabilities := make([]string, 0, len(population.CapabilityAccesses))
	seen := make(map[string]struct{}, len(population.CapabilityAccesses))
	for _, access := range population.CapabilityAccesses {
		name := access.Name
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		capabilities = append(capabilities, name)
	}
	sort.Strings(capabilities)

	capabilityQualification, err := populationSourceQualification(population, string(history.SourceCapabilities), len(population.CapabilityAccesses))
	if err != nil {
		return Spec{}, err
	}
	if capabilityQualification == "EMPTY" {
		return Spec{}, fmt.Errorf("capability facts are inconsistent with EMPTY evidence")
	}

	observationIDs := make([]string, 0, len(population.ObservationContributions))
	for _, contribution := range population.ObservationContributions {
		observationIDs = append(observationIDs, contribution.ObservationID)
	}
	sort.Strings(observationIDs)
	if len(observationIDs) > 256 {
		return Spec{}, fmt.Errorf("proposal provenance exceeds 256 observation IDs")
	}

	qualification, err := proposalQualificationFromPopulation(population)
	if err != nil {
		return Spec{}, err
	}
	// Keep the capability source's exact state visible even when other source
	// summaries are absent or empty.
	qualification.Capabilities = capabilityQualification

	spec := Spec{
		CandidateVersion: CandidateVersionV2,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		HistoryUsed:      true,
		Subject: &SubjectV2{
			Scope:         CandidateV2ScopeContainer,
			Target:        identity.Target,
			Container:     identity.Container,
			ImageIdentity: identity.ImageIdentity,
		},
		CapabilityArtifact: &ArtifactV2{
			Type: CandidateV2ArtifactContainerCaps,
			ContainerCapabilities: ContainerCapabilitiesV2{
				Drop: []string{"ALL"},
				Add:  capabilities,
			},
		},
		Provenance: &ProposalProvenance{
			PopulationScope: CandidateV2ScopeContainer,
			ObservationIDs:  observationIDs,
		},
		Qualification:    &qualification,
		DerivationStatus: &ProposalDerivationStatus{Capabilities: derivationSupported, PodLock: derivationNotAvailable, NetworkPolicy: derivationNotAvailable, Seccomp: derivationNotAvailable},
	}
	if err := ValidateProposalSpec(spec); err != nil {
		return Spec{}, fmt.Errorf("derived candidate-v2 proposal is invalid: %w", err)
	}
	return spec, nil
}

// GenerateContainerCapabilityProposal loads a population through the
// scope-aware history lookup, derives a candidate, and persists it using the
// existing Proposal Save path. It never changes approval or custody status.
func GenerateContainerCapabilityProposal(ctx context.Context, client dynamic.Interface, namespace string, identity history.PopulationIdentity, proposalName string) (Spec, error) {
	if strings.TrimSpace(proposalName) == "" {
		return Spec{}, fmt.Errorf("proposal name is empty")
	}
	record, err := history.GetPopulation(ctx, client, namespace, identity)
	if err != nil {
		return Spec{}, err
	}
	if record == nil {
		return Spec{}, fmt.Errorf("TrainingHistory population not found")
	}
	for _, population := range record.Populations {
		if history.PopulationIdentityEqual(population, history.Population{Scope: identity.Scope, Target: identity.Target, Container: identity.Container, ImageIdentity: identity.ImageIdentity, BinaryPath: identity.BinaryPath}) {
			spec, deriveErr := DeriveContainerCapabilityProposal(population)
			if deriveErr != nil {
				return Spec{}, deriveErr
			}
			if err := Save(ctx, client, namespace, proposalName, spec); err != nil {
				return Spec{}, err
			}
			return spec, nil
		}
	}
	return Spec{}, fmt.Errorf("TrainingHistory population identity mismatch")
}

func proposalQualificationFromPopulation(population history.Population) (ProposalQualification, error) {
	filesystem, err := populationSourceQualification(population, string(history.SourceFilesystem), len(population.FilesystemAccesses))
	if err != nil {
		return ProposalQualification{}, err
	}
	connect, err := populationSourceQualification(population, string(history.SourceNetworkConnect), countNetwork(population, "egress"))
	if err != nil {
		return ProposalQualification{}, err
	}
	bind, err := populationSourceQualification(population, string(history.SourceNetworkBind), countNetwork(population, "ingress"))
	if err != nil {
		return ProposalQualification{}, err
	}
	exec, err := populationSourceQualification(population, string(history.SourceExec), 0)
	if err != nil {
		return ProposalQualification{}, err
	}
	return ProposalQualification{Filesystem: filesystem, Exec: exec, NetworkConnect: connect, NetworkBind: bind}, nil
}

func countNetwork(population history.Population, direction string) int {
	count := 0
	for _, access := range population.NetworkAccesses {
		if string(access.Direction) == direction {
			count++
		}
	}
	return count
}

func populationSourceQualification(population history.Population, source string, positiveFacts int) (string, error) {
	state := "EMPTY"
	found := false
	for _, contribution := range population.ObservationContributions {
		for _, summary := range contribution.Sources {
			if summary.Source != source {
				continue
			}
			found = true
			if summary.EvidenceState == "UNKNOWN" {
				state = "UNKNOWN"
			} else if summary.EvidenceState == "AVAILABLE" && state != "UNKNOWN" {
				state = "AVAILABLE"
			} else if summary.EvidenceState != "EMPTY" {
				return "", fmt.Errorf("invalid %s evidence state %q", source, summary.EvidenceState)
			}
		}
	}
	if positiveFacts > 0 && found && state == "EMPTY" {
		return "", fmt.Errorf("%s evidence is EMPTY despite positive attributable facts", source)
	}
	if !found {
		return "EMPTY", nil
	}
	return state, nil
}
