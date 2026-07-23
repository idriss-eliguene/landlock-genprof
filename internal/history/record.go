// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package history persists a training target's observed accesses across
// multiple `trace --history` runs, in a TrainingHistory custom resource
// (see internal/history/store.go), so Confidence can finally be computed
// the way internal/profile.Confidence's own doc comment already
// describes it: "seen across how many distinct training runs" — not the
// single-run seenCount proxy internal/policy.confidenceFor computes for
// lack of any persisted state (see docs/policy-synthesis.md's
// "Confidence: a deliberately provisional heuristic").
//
// No controller, no reconciler: the CLI reads/writes the CR directly.
// This package's own types (Record, FileAccessRecord,
// NetworkAccessRecord) have no Kubernetes imports — the k8s-specific
// conversion lives entirely in store.go, mirroring how
// internal/exporter/* keeps output-format types out of internal/profile.
package history

import (
	"sort"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// Record is a training target's accumulated observation history across
// every `trace --history` run recorded so far.
type Record struct {
	Container          string
	Binary             string
	RunsRecorded       int
	FilesystemAccesses []FileAccessRecord
	NetworkAccesses    []NetworkAccessRecord
}

// FileAccessRecord is one filesystem path's accumulated history.
type FileAccessRecord struct {
	Path        string
	Permissions []profile.FilePermission
	SeenInRuns  int
}

// NetworkAccessRecord is one (port, direction) pair's accumulated
// history.
type NetworkAccessRecord struct {
	Port       int
	Direction  profile.NetworkDirection
	SeenInRuns int
}

type netRecordKey struct {
	port      int
	direction profile.NetworkDirection
}

// Merge folds this run's BehaviorProfile into existing (nil for the
// first run ever recorded for this container/binary), incrementing
// RunsRecorded once and, for every access observed in this run,
// SeenInRuns. An access recorded previously but not observed this run
// keeps its SeenInRuns unchanged while RunsRecorded still grows — its
// ratio (and therefore its Confidence, see ApplyConfidence) naturally
// decays over successive runs that stop observing it, which is
// docs/roadmap.md M5's drift-detection prerequisite falling out of this
// for free, not a separate mechanism.
func Merge(existing *Record, container, binary string, behavior profile.BehaviorProfile) *Record {
	record := existing
	if record == nil {
		record = &Record{Container: container, Binary: binary}
	}
	record.RunsRecorded++

	fsIndex := make(map[string]int, len(record.FilesystemAccesses))
	for i, a := range record.FilesystemAccesses {
		fsIndex[a.Path] = i
	}
	for _, access := range behavior.Filesystem.Accesses {
		if idx, ok := fsIndex[access.Path]; ok {
			record.FilesystemAccesses[idx].SeenInRuns++
			record.FilesystemAccesses[idx].Permissions = mergePermissions(
				record.FilesystemAccesses[idx].Permissions, access.Permissions)
			continue
		}
		record.FilesystemAccesses = append(record.FilesystemAccesses, FileAccessRecord{
			Path:        access.Path,
			Permissions: access.Permissions,
			SeenInRuns:  1,
		})
		fsIndex[access.Path] = len(record.FilesystemAccesses) - 1
	}
	sort.Slice(record.FilesystemAccesses, func(i, j int) bool {
		return record.FilesystemAccesses[i].Path < record.FilesystemAccesses[j].Path
	})

	netIndex := make(map[netRecordKey]int, len(record.NetworkAccesses))
	for i, a := range record.NetworkAccesses {
		netIndex[netRecordKey{a.Port, a.Direction}] = i
	}
	for _, access := range behavior.Network.Accesses {
		key := netRecordKey{access.Port, access.Direction}
		if idx, ok := netIndex[key]; ok {
			record.NetworkAccesses[idx].SeenInRuns++
			continue
		}
		record.NetworkAccesses = append(record.NetworkAccesses, NetworkAccessRecord{
			Port:       access.Port,
			Direction:  access.Direction,
			SeenInRuns: 1,
		})
		netIndex[key] = len(record.NetworkAccesses) - 1
	}
	sort.Slice(record.NetworkAccesses, func(i, j int) bool {
		if record.NetworkAccesses[i].Port != record.NetworkAccesses[j].Port {
			return record.NetworkAccesses[i].Port < record.NetworkAccesses[j].Port
		}
		return record.NetworkAccesses[i].Direction < record.NetworkAccesses[j].Direction
	})

	return record
}

// mergePermissions returns the union of existing and observed
// permissions, in the fixed read/write/execute order
// internal/policy.permissionsFor already uses, deduplicated — a path's
// permission set can differ between runs (e.g. read-only one run,
// read-write the next if a rarely-taken code path writes to it), and the
// history keeps every permission ever observed for that path.
func mergePermissions(existing, observed []profile.FilePermission) []profile.FilePermission {
	seen := make(map[profile.FilePermission]bool, len(existing)+len(observed))
	for _, p := range existing {
		seen[p] = true
	}
	for _, p := range observed {
		seen[p] = true
	}
	var merged []profile.FilePermission
	for _, p := range []profile.FilePermission{profile.PermissionRead, profile.PermissionWrite, profile.PermissionExecute} {
		if seen[p] {
			merged = append(merged, p)
		}
	}
	return merged
}

// ApplyConfidence recomputes each access's Confidence from record's
// cross-run ratio (SeenInRuns/RunsRecorded — high only once seen on
// every recorded run), returning an updated BehaviorProfile. record may
// be nil (no history yet): behavior is returned unchanged, keeping
// internal/policy.confidenceFor's single-run heuristic as the fallback.
//
// Note: neither internal/exporter/podlock nor internal/exporter/networkpolicy
// currently reads Confidence at all — it's computed and then silently
// dropped at export time today, single-run or cross-run alike. This
// function makes the number correct; surfacing it in the exported YAML
// is a separate, not-yet-done change.
func ApplyConfidence(record *Record, behavior profile.BehaviorProfile) profile.BehaviorProfile {
	if record == nil {
		return behavior
	}

	fsSeenInRuns := make(map[string]int, len(record.FilesystemAccesses))
	for _, a := range record.FilesystemAccesses {
		fsSeenInRuns[a.Path] = a.SeenInRuns
	}
	accesses := make([]profile.FileAccess, len(behavior.Filesystem.Accesses))
	copy(accesses, behavior.Filesystem.Accesses)
	for i, a := range accesses {
		if seenInRuns, ok := fsSeenInRuns[a.Path]; ok {
			accesses[i].Confidence = confidenceForHistory(seenInRuns, record.RunsRecorded)
		}
	}

	netSeenInRuns := make(map[netRecordKey]int, len(record.NetworkAccesses))
	for _, a := range record.NetworkAccesses {
		netSeenInRuns[netRecordKey{a.Port, a.Direction}] = a.SeenInRuns
	}
	netAccesses := make([]profile.NetworkAccess, len(behavior.Network.Accesses))
	copy(netAccesses, behavior.Network.Accesses)
	for i, a := range netAccesses {
		if seenInRuns, ok := netSeenInRuns[netRecordKey{a.Port, a.Direction}]; ok {
			netAccesses[i].Confidence = confidenceForHistory(seenInRuns, record.RunsRecorded)
		}
	}

	return profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: accesses},
		Network:    profile.NetworkProfile{Accesses: netAccesses},
	}
}

// confidenceForHistory computes Confidence from the real, documented
// cross-run measure — see internal/profile.Confidence's own doc comment
// and the README's "seen on every run" / "seen once out of 5 runs"
// examples, which this actually implements (internal/policy.confidenceFor
// only approximates it from a single run).
func confidenceForHistory(seenInRuns, runsRecorded int) profile.Confidence {
	switch {
	case runsRecorded > 0 && seenInRuns == runsRecorded:
		return profile.ConfidenceHigh
	case runsRecorded > 0 && seenInRuns*2 >= runsRecorded:
		return profile.ConfidenceMedium
	default:
		return profile.ConfidenceLow
	}
}
