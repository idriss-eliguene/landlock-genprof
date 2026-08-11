package adapter

import (
	"fmt"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
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
// EvidenceGroups maps AssertionEventIdentity -> indexes of input events
// (positions in the supplied []tracer.Event).
type BuildResult struct {
	Graph          *semantic.Graph
	AssertionIDs   []semantic.AssertionEventIdentity
	EvidenceGroups map[semantic.AssertionEventIdentity][]int
}

// error types
var (
	ErrInvalidRecordTime     = fmt.Errorf("invalid (zero) RecordTime")
	ErrInvalidEventTimestamp = fmt.Errorf("invalid event timestamp: zero value present")
)

// BuildGraphFromEvents implements Model A: one acquisition Act per non-empty
// input slice, Proposition.ValidTime remains unset, and structurally equal
// Propositions within the Act deduplicate into single AssertionEvents.
// Raw occurrence multiplicity is retained in EvidenceGroups and is not
// persisted into the semantic.Graph as identity entropy.
func BuildGraphFromEvents(meta RunMeta, events []tracer.Event) (BuildResult, error) {
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
	for i, ev := range events {
		if ev.Timestamp.IsZero() {
			return BuildResult{}, fmt.Errorf("%w at index %d", ErrInvalidEventTimestamp, i)
		}
		if i == 0 || ev.Timestamp.Before(min) {
			min = ev.Timestamp
		}
		if i == 0 || ev.Timestamp.After(max) {
			max = ev.Timestamp
		}
	}
	// use caller-supplied Start/End if present
	startPtr := meta.Start
	endPtr := meta.End
	if startPtr == nil {
		startPtr = &min
	}
	if endPtr == nil {
		endPtr = &max
	}
	interval := semantic.NewValidTime(startPtr, endPtr)
	// construct acquisition Act using RFC-valid kind (ActContact)
	act := semantic.NewAct(meta.Source, semantic.ActContact, interval, nil, nil)
	// append Act to Graph
	if err := res.Graph.AppendAct(act); err != nil {
		return BuildResult{}, fmt.Errorf("append act: %w", err)
	}
	// grouping by structural Proposition equality with CanonicalString as accelerator
	groups := make([]semantic.Proposition, 0)
	groupEvidence := make([][]int, 0)

	for idx, ev := range events {
		// build Proposition from tracer.Event (filesystem vertical slice)
		argsRecord := map[string]semantic.SnapshotValue{}
		// include only semantic fields
		if ev.Path != "" {
			argsRecord["path"] = semantic.NewLiteral("string", ev.Path)
		}
		if ev.Mode != "" {
			argsRecord["mode"] = semantic.NewLiteral("string", ev.Mode)
		}
		if ev.Port != 0 {
			argsRecord["port"] = semantic.NewLiteral("int", fmt.Sprintf("%d", ev.Port))
		}
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
