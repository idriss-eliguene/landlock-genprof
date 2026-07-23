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
	SyscallAccesses    []SyscallAccessRecord
	CapabilityAccesses []CapabilityAccessRecord
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

// SyscallAccessRecord is one syscall name's accumulated history.
type SyscallAccessRecord struct {
	Name       string
	SeenInRuns int
}

// CapabilityAccessRecord is one Linux capability's accumulated history.
type CapabilityAccessRecord struct {
	Name       string
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

	syscallIndex := make(map[string]int, len(record.SyscallAccesses))
	for i, a := range record.SyscallAccesses {
		syscallIndex[a.Name] = i
	}
	for _, access := range behavior.Syscalls.Accesses {
		if idx, ok := syscallIndex[access.Name]; ok {
			record.SyscallAccesses[idx].SeenInRuns++
			continue
		}
		record.SyscallAccesses = append(record.SyscallAccesses, SyscallAccessRecord{
			Name:       access.Name,
			SeenInRuns: 1,
		})
		syscallIndex[access.Name] = len(record.SyscallAccesses) - 1
	}
	sort.Slice(record.SyscallAccesses, func(i, j int) bool {
		return record.SyscallAccesses[i].Name < record.SyscallAccesses[j].Name
	})

	capabilityIndex := make(map[string]int, len(record.CapabilityAccesses))
	for i, a := range record.CapabilityAccesses {
		capabilityIndex[a.Name] = i
	}
	for _, access := range behavior.Capabilities.Accesses {
		if idx, ok := capabilityIndex[access.Name]; ok {
			record.CapabilityAccesses[idx].SeenInRuns++
			continue
		}
		record.CapabilityAccesses = append(record.CapabilityAccesses, CapabilityAccessRecord{
			Name:       access.Name,
			SeenInRuns: 1,
		})
		capabilityIndex[access.Name] = len(record.CapabilityAccesses) - 1
	}
	sort.Slice(record.CapabilityAccesses, func(i, j int) bool {
		return record.CapabilityAccesses[i].Name < record.CapabilityAccesses[j].Name
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
// internal/exporter/podlock, internal/exporter/networkpolicy, and
// internal/exporter/capabilities surface this as a `# confidence: ...`
// YAML comment; internal/exporter/seccomp cannot (its output must stay
// valid JSON) and prints it to stdout instead — see
// cmd/landlock-genprof/trace.go's writeSeccompProfile.
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

	syscallSeenInRuns := make(map[string]int, len(record.SyscallAccesses))
	for _, a := range record.SyscallAccesses {
		syscallSeenInRuns[a.Name] = a.SeenInRuns
	}
	syscallAccesses := make([]profile.SyscallAccess, len(behavior.Syscalls.Accesses))
	copy(syscallAccesses, behavior.Syscalls.Accesses)
	for i, a := range syscallAccesses {
		if seenInRuns, ok := syscallSeenInRuns[a.Name]; ok {
			syscallAccesses[i].Confidence = confidenceForHistory(seenInRuns, record.RunsRecorded)
		}
	}

	capabilitySeenInRuns := make(map[string]int, len(record.CapabilityAccesses))
	for _, a := range record.CapabilityAccesses {
		capabilitySeenInRuns[a.Name] = a.SeenInRuns
	}
	capabilityAccesses := make([]profile.CapabilityAccess, len(behavior.Capabilities.Accesses))
	copy(capabilityAccesses, behavior.Capabilities.Accesses)
	for i, a := range capabilityAccesses {
		if seenInRuns, ok := capabilitySeenInRuns[a.Name]; ok {
			capabilityAccesses[i].Confidence = confidenceForHistory(seenInRuns, record.RunsRecorded)
		}
	}

	return profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: accesses},
		Network:    profile.NetworkProfile{Accesses: netAccesses},
		Syscalls: profile.SyscallProfile{
			Accesses:      syscallAccesses,
			Architectures: behavior.Syscalls.Architectures,
		},
		Capabilities: profile.CapabilityProfile{Accesses: capabilityAccesses},
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
