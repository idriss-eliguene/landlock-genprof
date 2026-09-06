package k8s

import (
	"fmt"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
)

// TargetChangeDecision is a fact-only result for the Observation executor;
// it never performs a persistence mutation itself.
type TargetChangeDecision struct {
	Events []domain.TargetChangeEvent
	Stop   bool
	Reason domain.CompletionReason
}

// CompareObservationTargets compares two resolver snapshots. Pod identity
// changes within the same workload/image revision are continued; any
// revision boundary is terminal for the current Observation.
func CompareObservationTargets(previous, current []ObservationTarget, at time.Time) (TargetChangeDecision, error) {
	if at.IsZero() {
		return TargetChangeDecision{}, fmt.Errorf("target comparison requires a timestamp")
	}
	decision := TargetChangeDecision{}
	curByKey := make(map[string]ObservationTarget, len(current))
	for _, target := range current {
		curByKey[observationTargetKey(target)] = target
	}
	for _, old := range previous {
		key := observationTargetKey(old)
		newTarget, exists := curByKey[key]
		if !exists {
			replacement := false
			workloadRecreated := false
			for _, candidate := range current {
				if workloadSlotNameNoUID(old) == workloadSlotNameNoUID(candidate) {
					replacement = true
					workloadRecreated = old.Instance.Slot.Workload.UID != candidate.Instance.Slot.Workload.UID
					break
				}
			}
			kind := domain.PodRemoved
			if replacement && workloadRecreated {
				kind = domain.WorkloadRevisionChanged
			}
			event, err := domain.NewTargetChangeEvent(at, kind, key)
			if err != nil {
				return decision, err
			}
			decision.Events = append(decision.Events, event)
			if kind == domain.WorkloadRevisionChanged {
				decision.Stop = true
				decision.Reason = domain.TargetRevisionChanged
			}
			continue
		}
		if old.HasRevision && newTarget.HasRevision && old.Revision != newTarget.Revision {
			event, err := domain.NewTargetChangeEvent(at, domain.WorkloadRevisionChanged, key)
			if err != nil {
				return decision, err
			}
			decision.Events = append(decision.Events, event)
			decision.Stop = true
			decision.Reason = domain.TargetRevisionChanged
			continue
		}
		if old.Instance.ImageRevision != nil && newTarget.Instance.ImageRevision != nil && old.Instance.ImageRevision.ImageDigest != newTarget.Instance.ImageRevision.ImageDigest {
			event, err := domain.NewTargetChangeEvent(at, domain.ImageChanged, key)
			if err != nil {
				return decision, err
			}
			decision.Events = append(decision.Events, event)
			decision.Stop = true
			decision.Reason = domain.TargetRevisionChanged
		}
	}
	for key := range curByKey {
		found := false
		for _, old := range previous {
			if observationTargetKey(old) == key {
				found = true
				break
			}
		}
		if !found {
			event, err := domain.NewTargetChangeEvent(at, domain.PodAdded, key)
			if err != nil {
				return decision, err
			}
			decision.Events = append(decision.Events, event)
		}
	}
	if len(current) == 0 && len(previous) > 0 && !decision.Stop {
		decision.Stop = true
		decision.Reason = domain.TargetUnavailable
	}
	return decision, nil
}

func observationTargetKey(target ObservationTarget) string {
	return workloadSlotName(target) + "\x00" + target.Instance.PodUID
}

func workloadSlotName(target ObservationTarget) string {
	return target.Instance.Slot.Workload.Namespace + "\x00" + target.Instance.Slot.Workload.GroupKind.Group + "\x00" + target.Instance.Slot.Workload.GroupKind.Kind + "\x00" + target.Instance.Slot.Workload.Name + "\x00" + target.Instance.Slot.Workload.UID + "\x00" + target.Instance.Slot.Container
}

func workloadSlotNameNoUID(target ObservationTarget) string {
	return target.Instance.Slot.Workload.Namespace + "\x00" + target.Instance.Slot.Workload.GroupKind.Group + "\x00" + target.Instance.Slot.Workload.GroupKind.Kind + "\x00" + target.Instance.Slot.Workload.Name + "\x00" + target.Instance.Slot.Container
}
