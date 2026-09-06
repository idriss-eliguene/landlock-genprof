package history

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// ErrObservationNotEligible indicates that an Observation is not a frozen,
// successfully completed source of policy evidence.
var ErrObservationNotEligible = fmt.Errorf("observation is not eligible for contribution")

// ContributionFromObservation is the G6.4 derivation boundary. It validates
// the frozen Observation and converts only its bounded, target-attributed
// normalized facts to the already-certified G6.3 Contribution primitive.
// Exec facts are retained in the source summaries as provenance, but are not
// copied into any TrainingHistory policy fact collection.
func ContributionFromObservation(observation observationdomain.Observation) (Contribution, error) {
	if !observation.Frozen() || observation.Execution().State != observationdomain.ExecutionCompleted || observation.Execution().Completion == "" {
		return Contribution{}, fmt.Errorf("%w: observation must be frozen and COMPLETED", ErrObservationNotEligible)
	}
	population, err := containerPopulationFromObservation(observation)
	if err != nil {
		return Contribution{}, err
	}

	result := observation.Result()
	sources := result.Sources()
	if len(sources) == 0 {
		return Contribution{}, fmt.Errorf("%w: completed observation has no source results", ErrObservationNotEligible)
	}

	contribution := Contribution{
		ObservationID: string(observation.ID()),
		Population: PopulationFingerprint{
			Scope:         population.Scope,
			Target:        population.Target,
			Container:     population.Container,
			ImageIdentity: population.ImageIdentity,
		},
	}
	for _, source := range sources {
		if !validObservationContributionSource(source.Source.Name) {
			return Contribution{}, fmt.Errorf("%w: unsupported Observation source %q", ErrObservationNotEligible, source.Source.Name)
		}
		facts := source.Facts
		contribution.Sources = append(contribution.Sources, ObservationSourceContribution{
			Source:              source.Source.Name,
			EvidenceState:       string(source.Evidence),
			AttributionState:    string(source.Qualification.Attribution),
			BackendHealthy:      source.Qualification.BackendHealthConfirmed,
			AttachedForWindow:   source.Qualification.SourceAttachedForBoundWindow,
			FlushConfirmed:      source.Qualification.FlushConfirmed,
			AttributedCount:     int64(source.Qualification.AttributedCount),
			ExcludedCount:       int64(source.Qualification.ExcludedCount),
			NormalizedFactCount: int64(facts.Count()),
		})
		switch source.Source.Name {
		case string(SourceFilesystem):
			for _, fact := range facts.Filesystem {
				contribution.Filesystem = append(contribution.Filesystem, profile.FileAccess{Path: fact.Path, Permissions: append([]profile.FilePermission(nil), fact.Permissions...)})
			}
		case string(SourceExec):
			// Exec is provenance-only by the frozen G6.1/G6.4 contract.
		case string(SourceNetworkConnect):
			for _, fact := range facts.NetworkConnect {
				contribution.NetworkConnect = append(contribution.NetworkConnect, profile.NetworkAccess{Port: fact.Port, Direction: fact.Direction})
			}
		case string(SourceNetworkBind):
			for _, fact := range facts.NetworkBind {
				contribution.NetworkBind = append(contribution.NetworkBind, profile.NetworkAccess{Port: fact.Port, Direction: fact.Direction})
			}
		case string(SourceCapabilities):
			for _, fact := range facts.Capabilities {
				contribution.Capabilities = append(contribution.Capabilities, profile.CapabilityAccess{Name: fact.Name})
			}
		}
	}
	sort.Slice(contribution.Sources, func(i, j int) bool { return contribution.Sources[i].Source < contribution.Sources[j].Source })
	if _, err := contribution.normalize(); err != nil {
		return Contribution{}, err
	}
	return contribution, nil
}

// ApplyObservationContribution validates and derives one Contribution, then
// delegates all mutation, idempotency, receipt, marker, and recovery behavior
// to the certified G6.3 ApplyContribution primitive.
func ApplyObservationContribution(ctx context.Context, client dynamic.Interface, namespace string, observation observationdomain.Observation) (ContributionApplyResult, error) {
	if strings.TrimSpace(namespace) == "" {
		return "", fmt.Errorf("%w: history namespace is empty", ErrObservationNotEligible)
	}
	contribution, err := ContributionFromObservation(observation)
	if err != nil {
		return "", err
	}
	return ApplyContribution(ctx, client, namespace, contribution)
}

func validObservationContributionSource(source string) bool {
	switch source {
	case string(SourceFilesystem), string(SourceExec), string(SourceNetworkConnect), string(SourceNetworkBind), string(SourceCapabilities):
		return true
	default:
		return false
	}
}

func containerPopulationFromObservation(observation observationdomain.Observation) (PopulationIdentity, error) {
	binding := observation.Binding()
	targets := binding.ResolvedTargets.Items()
	if len(targets) != 1 || !targets[0].Valid() {
		return PopulationIdentity{}, fmt.Errorf("%w: Observation must have exactly one valid bound runtime target", ErrObservationNotEligible)
	}
	slot := targets[0].Slot
	if slot != observation.Spec().Target.Slot {
		return PopulationIdentity{}, fmt.Errorf("%w: resolved target differs from requested target", ErrObservationNotEligible)
	}
	target := k8s.GovernedTarget{
		Namespace: slot.Workload.Namespace,
		Workload: k8s.WorkloadRef{
			Group: slot.Workload.GroupKind.Group,
			Kind:  slot.Workload.GroupKind.Kind,
			Name:  slot.Workload.Name,
		},
		Container: slot.Container,
	}
	if target.LegacyString() == "" {
		return PopulationIdentity{}, fmt.Errorf("%w: bound target has no canonical identity", ErrObservationNotEligible)
	}

	var image string
	for _, revision := range binding.ImageRevisionValues() {
		if revision.Slot != slot {
			continue
		}
		if _, err := observationdomain.NewContainerImageRevision(revision.Slot, revision.ImageDigest); err != nil {
			return PopulationIdentity{}, fmt.Errorf("%w: invalid bound image identity: %v", ErrObservationNotEligible, err)
		}
		if image != "" && image != revision.ImageDigest {
			return PopulationIdentity{}, fmt.Errorf("%w: conflicting bound image identities", ErrObservationNotEligible)
		}
		image = revision.ImageDigest
	}
	if targets[0].ImageRevision != nil {
		revision := targets[0].ImageRevision
		if _, err := observationdomain.NewContainerImageRevision(revision.Slot, revision.ImageDigest); err != nil {
			return PopulationIdentity{}, fmt.Errorf("%w: invalid runtime image identity: %v", ErrObservationNotEligible, err)
		}
		if revision.Slot != slot {
			return PopulationIdentity{}, fmt.Errorf("%w: runtime image identity targets another container", ErrObservationNotEligible)
		}
		if image != "" && image != revision.ImageDigest {
			return PopulationIdentity{}, fmt.Errorf("%w: runtime and binding image identities differ", ErrObservationNotEligible)
		}
		image = revision.ImageDigest
	}
	if image == "" {
		return PopulationIdentity{}, fmt.Errorf("%w: immutable bound image identity is missing", ErrObservationNotEligible)
	}
	identity := PopulationIdentity{Scope: ScopeContainer, Target: target.LegacyString(), Container: slot.Container, ImageIdentity: image}
	if err := identity.Validate(); err != nil {
		return PopulationIdentity{}, err
	}
	return identity, nil
}
