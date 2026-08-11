package adapter

import (
	"fmt"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
)

// RunMeta carries caller-supplied, truthful run metadata used for Act and
// AssertionEvent construction. RecordTime is mandatory and must be a
// non-zero time.Time.
type RunMeta struct {
	Source     semantic.SubjectIdentity
	Start      *time.Time
	End        *time.Time
	RecordTime time.Time
}

// BuildResult is an ephemeral, in-memory mapping from the constructed
// semantic Graph to the groupings that link back to the input slice.
// EvidenceGroups maps AssertionEventIdentity -> indexes of input observations
// (positions in the supplied []observation.Observation).
type BuildResult struct {
	Graph          *semantic.Graph
	AssertionIDs   []semantic.AssertionEventIdentity
	EvidenceGroups map[semantic.AssertionEventIdentity][]int
}

// error types
var (
	ErrInvalidRecordTime     = fmt.Errorf("invalid (zero) RecordTime")
	ErrInvalidEventTimestamp = fmt.Errorf("invalid event timestamp: zero value present")
	ErrUnsupportedEvent      = fmt.Errorf("unsupported event for filesystem adapter")
	ErrInvalidInterval       = fmt.Errorf("invalid interval: start after end")
)

// BuildGraphFromObservations implements Model A: one acquisition Act per non-empty
// input slice, Proposition.ValidTime remains unset, and structurally equal
// Propositions within the Act deduplicate into single AssertionEvents.
// Raw occurrence multiplicity is retained in EvidenceGroups and is not
// persisted into the semantic.Graph as identity entropy.
func BuildGraphFromObservations(meta RunMeta, events []observation.Observation) (BuildResult, error) {
	// validate RecordTime
	if meta.RecordTime.IsZero() {
		return BuildResult{}, ErrInvalidRecordTime
	}
	// prepare result
	res := BuildResult{Graph: semantic.NewGraph(), EvidenceGroups: make(map[semantic.AssertionEventIdentity][]int)}
	// zero events => empty Graph per design
	if len(events) == 0 {
		return res, nil
	}
	// validate event timestamps and compute interval
	var min time.Time
	var max time.Time
	derivedTimeFound := false
	for i, ev := range events {
		// syscall-profile observations may legitimately have zero timestamps
		if ev.Kind != observation.KindSyscall {
			if ev.Timestamp.IsZero() {
				return BuildResult{}, fmt.Errorf("%w at index %d", ErrInvalidEventTimestamp, i)
			}
			if !derivedTimeFound || ev.Timestamp.Before(min) {
				min = ev.Timestamp
			}
			if !derivedTimeFound || ev.Timestamp.After(max) {
				max = ev.Timestamp
			}
			derivedTimeFound = true
		} else {
			// For KindSyscall, accept zero timestamps and do not contribute
			// to derived interval bounds.
			continue
		}
	}
	// use caller-supplied Start/End if present
	startPtr := meta.Start
	endPtr := meta.End
	// If no derived non-zero timestamps exist and no explicit Start/End were
	// supplied, leave the interval unbounded (nil pointers) rather than
	// fabricating min/max zero times. Otherwise, use derived bounds where
	// callers didn't provide explicit Start/End.
	if !derivedTimeFound {
		// no non-zero timestamps found
		if startPtr == nil && endPtr == nil {
			// leave both nil -> NewValidTime(nil,nil)
			interval := semantic.NewValidTime(nil, nil)
			// attach below after validation branch
			_ = interval
		} else {
			// explicit bounds provided: validate them below
		}
	} else {
		if startPtr == nil {
			startPtr = &min
		}
		if endPtr == nil {
			endPtr = &max
		}
	}
	interval := semantic.NewValidTime(startPtr, endPtr)
	// validate effective interval when both bounds are present
	if startPtr != nil && endPtr != nil && startPtr.After(*endPtr) {
		return BuildResult{}, fmt.Errorf("%w: start=%v end=%v", ErrInvalidInterval, startPtr.UTC(), endPtr.UTC())
	}
	// If derived bounds exist and explicit Start/End were supplied, ensure
	// explicit bounds do not contradict derived interval.
	if derivedTimeFound {
		if meta.Start != nil && meta.Start.After(max) {
			return BuildResult{}, fmt.Errorf("%w: explicit Start > derived End", ErrInvalidInterval)
		}
		if meta.End != nil && meta.End.Before(min) {
			return BuildResult{}, fmt.Errorf("%w: explicit End < derived Start", ErrInvalidInterval)
		}
	}
	// construct acquisition Act using RFC-valid kind (ActContact)
	act := semantic.NewAct(meta.Source, semantic.ActContact, interval, nil, nil)
	// append Act to Graph
	if err := res.Graph.AppendAct(act); err != nil {
		return BuildResult{}, fmt.Errorf("append act: %w", err)
	}
	// grouping by structural Proposition equality with CanonicalString as accelerator
	groups := make([]semantic.Proposition, 0)
	groupEvidence := make([][]int, 0)

	// admission and grouping: accept filesystem and network observations
	allowed := map[string]struct{}{"read": {}, "write": {}, "read_write": {}, "exec": {}}
	for idx, ev := range events {
		switch ev.Kind {
		case observation.KindFilesystem:
			// filesystem handling
			if ev.Path == "" {
				return BuildResult{}, fmt.Errorf("%w at index %d: empty path (mode=%q)", ErrUnsupportedEvent, idx, ev.Mode)
			}
			if _, ok := allowed[ev.Mode]; !ok {
				return BuildResult{}, fmt.Errorf("%w at index %d: unsupported mode=%q", ErrUnsupportedEvent, idx, ev.Mode)
			}
			// build Proposition from observation (filesystem vertical slice)
			argsRecord := map[string]semantic.SnapshotValue{}
			// include only filesystem semantic fields
			if ev.Path != "" {
				argsRecord["path"] = semantic.NewLiteral("string", ev.Path)
			}
			if ev.Mode != "" {
				// include mode as semantic attribute
				argsRecord["mode"] = semantic.NewLiteral("string", ev.Mode)
			}
			// isDir/truncate are legitimate filesystem attributes
			if ev.IsDir {
				argsRecord["isDir"] = semantic.NewLiteral("bool", "true")
			}
			if ev.Truncate {
				argsRecord["truncate"] = semantic.NewLiteral("bool", "true")
			}
			rec := semantic.NewRecord(argsRecord)
			prop := semantic.NewProposition(semantic.NewIdentityRef("Phase", "p"), semantic.Actual, semantic.NewIdentityRef("Term", "FileAccess"), []semantic.SnapshotValue{rec}, semantic.NewValidTime(nil, nil), semantic.QuantThisInstance)
			// attempt to find an existing structural-equal group
			found := false
			for gi, existing := range groups {
				if semantic.StructuralEqual(existing, prop) {
					groupEvidence[gi] = append(groupEvidence[gi], idx)
					found = true
					break
				}
			}
			if !found {
				groups = append(groups, prop)
				groupEvidence = append(groupEvidence, []int{idx})
			}

		case observation.KindNetwork:
			// network handling
			// validate timestamp already done earlier
			if ev.Port == 0 {
				return BuildResult{}, fmt.Errorf("%w at index %d: missing port", ErrUnsupportedEvent, idx)
			}
			// derive direction from syscall/mode; require at least one to be usable
			var dirFromSyscall, dirFromMode string
			if ev.Syscall == "bind" {
				dirFromSyscall = "ingress"
			} else if ev.Syscall == "connect" {
				dirFromSyscall = "egress"
			}
			if ev.Mode == "ingress" {
				dirFromMode = "ingress"
			} else if ev.Mode == "egress" {
				dirFromMode = "egress"
			}
			// if both present and conflict, reject as malformed
			if dirFromSyscall != "" && dirFromMode != "" && dirFromSyscall != dirFromMode {
				return BuildResult{}, fmt.Errorf("%w at index %d: conflicting syscall/mode (syscall=%q mode=%q)", ErrUnsupportedEvent, idx, ev.Syscall, ev.Mode)
			}
			// choose whichever is present, default to egress
			direction := "egress"
			if dirFromSyscall != "" {
				direction = dirFromSyscall
			} else if dirFromMode != "" {
				direction = dirFromMode
			}
			// require at least one to be usable
			if direction == "" {
				// malformed network observation
				return BuildResult{}, fmt.Errorf("%w at index %d: unsupported network mode/syscall (mode=%q syscall=%q)", ErrUnsupportedEvent, idx, ev.Mode, ev.Syscall)
			}
			// build network proposition with identity = (port, direction) only
			argsRecord := map[string]semantic.SnapshotValue{}
			argsRecord["port"] = semantic.NewLiteral("int", fmt.Sprintf("%d", ev.Port))
			argsRecord["direction"] = semantic.NewLiteral("string", direction)
			rec := semantic.NewRecord(argsRecord)
			prop := semantic.NewProposition(semantic.NewIdentityRef("Phase", "p"), semantic.Actual, semantic.NewIdentityRef("Term", "NetworkAccess"), []semantic.SnapshotValue{rec}, semantic.NewValidTime(nil, nil), semantic.QuantThisInstance)
			// attempt to find an existing structural-equal group
			found := false
			for gi, existing := range groups {
				if semantic.StructuralEqual(existing, prop) {
					groupEvidence[gi] = append(groupEvidence[gi], idx)
					found = true
					break
				}
			}
			if !found {
				groups = append(groups, prop)
				groupEvidence = append(groupEvidence, []int{idx})
			}

		case observation.KindCapability:
			// capability handling
			if ev.Mode != "capability" {
				return BuildResult{}, fmt.Errorf("%w at index %d: expected mode=\"capability\", got %q", ErrUnsupportedEvent, idx, ev.Mode)
			}
			name := strings.TrimSpace(ev.Syscall)
			if name == "" {
				return BuildResult{}, fmt.Errorf("%w at index %d: empty capability name", ErrUnsupportedEvent, idx)
			}
			argsRecordCap := map[string]semantic.SnapshotValue{"name": semantic.NewLiteral("string", name)}
			recCap := semantic.NewRecord(argsRecordCap)
			propCap := semantic.NewProposition(semantic.NewIdentityRef("Phase", "p"), semantic.Actual, semantic.NewIdentityRef("Term", "CapabilityCheckObserved"), []semantic.SnapshotValue{recCap}, semantic.NewValidTime(nil, nil), semantic.QuantThisInstance)
			foundCap := false
			for gi, existing := range groups {
				if semantic.StructuralEqual(existing, propCap) {
					groupEvidence[gi] = append(groupEvidence[gi], idx)
					foundCap = true
					break
				}
			}
			if !foundCap {
				groups = append(groups, propCap)
				groupEvidence = append(groupEvidence, []int{idx})
			}

		case observation.KindSyscall:
			// syscall profile handling
			if ev.Mode != "syscall" {
				return BuildResult{}, fmt.Errorf("%w at index %d: expected mode=\"syscall\", got %q", ErrUnsupportedEvent, idx, ev.Mode)
			}
			name := strings.TrimSpace(ev.Syscall)
			if name == "" {
				return BuildResult{}, fmt.Errorf("%w at index %d: empty syscall name", ErrUnsupportedEvent, idx)
			}
			argsRecordSc := map[string]semantic.SnapshotValue{"name": semantic.NewLiteral("string", name)}
			recSc := semantic.NewRecord(argsRecordSc)
			propSc := semantic.NewProposition(semantic.NewIdentityRef("Phase", "p"), semantic.Actual, semantic.NewIdentityRef("Term", "SyscallProfileAllowed"), []semantic.SnapshotValue{recSc}, semantic.NewValidTime(nil, nil), semantic.QuantThisInstance)
			foundSc := false
			for gi, existing := range groups {
				if semantic.StructuralEqual(existing, propSc) {
					groupEvidence[gi] = append(groupEvidence[gi], idx)
					foundSc = true
					break
				}
			}
			if !foundSc {
				groups = append(groups, propSc)
				groupEvidence = append(groupEvidence, []int{idx})
			}

		default:
			// ignore KindOther and any unknown kinds
		}
	}

	// construct AssertionEvents for each group
	rt, err := semantic.NewRecordTime(meta.RecordTime)
	if err != nil {
		return BuildResult{}, fmt.Errorf("invalid record time: %w", err)
	}
	producer := semantic.NewProducerRefFromAct(act.Identity())
	for i, prop := range groups {
		ev, err := semantic.NewAssertionEvent(producer, prop, rt)
		if err != nil {
			return BuildResult{}, fmt.Errorf("construct assertion: %w", err)
		}
		id, err := res.Graph.AppendAssertionEvent(ev)
		if err != nil {
			return BuildResult{}, fmt.Errorf("append assertion: %w", err)
		}
		res.AssertionIDs = append(res.AssertionIDs, id)
		res.EvidenceGroups[id] = append([]int(nil), groupEvidence[i]...)
	}
	return res, nil
}
