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
	"slices"
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
	// Populations contains compatibility-qualified observations. The legacy
	// fields above are intentionally retained for objects written before
	// populations existed; they are never merged with a qualified population.
	Populations []Population
}

// PopulationFingerprint identifies the narrow observation population used
// for confidence aggregation. It is not a claim of complete execution
// equivalence.
type PopulationFingerprint struct {
	Target        string `json:"target"`
	Container     string `json:"container"`
	ImageIdentity string `json:"imageIdentity"`
	BinaryPath    string `json:"binaryPath"`
}

func (f PopulationFingerprint) Valid() bool {
	return f.Target != "" && f.Container != "" && f.ImageIdentity != "" && f.BinaryPath != ""
}

// Population is an explicit confidence population. Contributors are
// attribution only; they do not participate in population identity.
type Population struct {
	Qualified          bool                     `json:"qualified"`
	Target             string                   `json:"target"`
	Container          string                   `json:"container"`
	ImageIdentity      string                   `json:"imageIdentity"`
	BinaryPath         string                   `json:"binaryPath"`
	RunsRecorded       int                      `json:"runsRecorded"`
	Contributors       []string                 `json:"contributors,omitempty"`
	FilesystemAccesses []FileAccessRecord       `json:"filesystemAccesses,omitempty"`
	NetworkAccesses    []NetworkAccessRecord    `json:"networkAccesses,omitempty"`
	SyscallAccesses    []SyscallAccessRecord    `json:"syscallAccesses,omitempty"`
	CapabilityAccesses []CapabilityAccessRecord `json:"capabilityAccesses,omitempty"`
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

// MergePopulation merges into exactly one explicit population. Unknown image
// identity is retained in an unqualified population but is never allowed to
// merge with a qualified population.
func MergePopulation(existing *Record, fingerprint PopulationFingerprint, subject string, behavior profile.BehaviorProfile) *Record {
	legacyPresent := existing != nil && (len(existing.FilesystemAccesses) > 0 || len(existing.NetworkAccesses) > 0 || len(existing.SyscallAccesses) > 0 || len(existing.CapabilityAccesses) > 0 || (len(existing.Populations) == 0 && existing.RunsRecorded > 0))
	if existing == nil {
		existing = &Record{Container: fingerprint.Container, Binary: fingerprint.BinaryPath}
	}
	idx := -1
	for i := range existing.Populations {
		if populationFingerprint(existing.Populations[i]) == fingerprint {
			idx = i
			break
		}
	}
	if idx < 0 {
		existing.Populations = append(existing.Populations, Population{
			Qualified:     fingerprint.Valid(),
			Target:        fingerprint.Target,
			Container:     fingerprint.Container,
			ImageIdentity: fingerprint.ImageIdentity,
			BinaryPath:    fingerprint.BinaryPath,
		})
		idx = len(existing.Populations) - 1
	}
	p := &existing.Populations[idx]
	p.RunsRecorded++
	if subject != "" && !slices.Contains(p.Contributors, subject) {
		p.Contributors = append(p.Contributors, subject)
		sort.Strings(p.Contributors)
	}
	mergeBehaviorIntoPopulation(p, behavior)
	sort.Slice(existing.Populations, func(i, j int) bool {
		return populationKey(populationFingerprint(existing.Populations[i])) < populationKey(populationFingerprint(existing.Populations[j]))
	})
	if !legacyPresent {
		existing.RunsRecorded = qualifiedPopulationRuns(existing)
	}
	return existing
}

func populationFingerprint(p Population) PopulationFingerprint {
	return PopulationFingerprint{Target: p.Target, Container: p.Container, ImageIdentity: p.ImageIdentity, BinaryPath: p.BinaryPath}
}

func populationKey(f PopulationFingerprint) string {
	return f.Target + "\x00" + f.Container + "\x00" + f.ImageIdentity + "\x00" + f.BinaryPath
}

func mergeBehaviorIntoPopulation(record *Population, behavior profile.BehaviorProfile) {
	record.FilesystemAccesses = mergeFileAccesses(record.FilesystemAccesses, behavior.Filesystem.Accesses)
	record.NetworkAccesses = mergeNetworkAccesses(record.NetworkAccesses, behavior.Network.Accesses)
	record.SyscallAccesses = mergeSyscallAccesses(record.SyscallAccesses, behavior.Syscalls.Accesses)
	record.CapabilityAccesses = mergeCapabilityAccesses(record.CapabilityAccesses, behavior.Capabilities.Accesses)
}

func mergeFileAccesses(existing []FileAccessRecord, observed []profile.FileAccess) []FileAccessRecord {
	index := make(map[string]int, len(existing))
	for i, access := range existing {
		index[access.Path] = i
	}
	for _, access := range observed {
		if i, ok := index[access.Path]; ok {
			existing[i].SeenInRuns++
			existing[i].Permissions = mergePermissions(existing[i].Permissions, access.Permissions)
		} else {
			existing = append(existing, FileAccessRecord{Path: access.Path, Permissions: access.Permissions, SeenInRuns: 1})
			index[access.Path] = len(existing) - 1
		}
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].Path < existing[j].Path })
	return existing
}

func mergeNetworkAccesses(existing []NetworkAccessRecord, observed []profile.NetworkAccess) []NetworkAccessRecord {
	index := make(map[netRecordKey]int, len(existing))
	for i, access := range existing {
		index[netRecordKey{access.Port, access.Direction}] = i
	}
	for _, access := range observed {
		key := netRecordKey{access.Port, access.Direction}
		if i, ok := index[key]; ok {
			existing[i].SeenInRuns++
		} else {
			existing = append(existing, NetworkAccessRecord{Port: access.Port, Direction: access.Direction, SeenInRuns: 1})
			index[key] = len(existing) - 1
		}
	}
	sort.Slice(existing, func(i, j int) bool {
		if existing[i].Port != existing[j].Port {
			return existing[i].Port < existing[j].Port
		}
		return existing[i].Direction < existing[j].Direction
	})
	return existing
}

func mergeSyscallAccesses(existing []SyscallAccessRecord, observed []profile.SyscallAccess) []SyscallAccessRecord {
	index := make(map[string]int, len(existing))
	for i, access := range existing {
		index[access.Name] = i
	}
	for _, access := range observed {
		if i, ok := index[access.Name]; ok {
			existing[i].SeenInRuns++
		} else {
			existing = append(existing, SyscallAccessRecord{Name: access.Name, SeenInRuns: 1})
			index[access.Name] = len(existing) - 1
		}
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].Name < existing[j].Name })
	return existing
}

func mergeCapabilityAccesses(existing []CapabilityAccessRecord, observed []profile.CapabilityAccess) []CapabilityAccessRecord {
	index := make(map[string]int, len(existing))
	for i, access := range existing {
		index[access.Name] = i
	}
	for _, access := range observed {
		if i, ok := index[access.Name]; ok {
			existing[i].SeenInRuns++
		} else {
			existing = append(existing, CapabilityAccessRecord{Name: access.Name, SeenInRuns: 1})
			index[access.Name] = len(existing) - 1
		}
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].Name < existing[j].Name })
	return existing
}

func qualifiedPopulationRuns(record *Record) int {
	total := 0
	for _, p := range record.Populations {
		if p.Qualified {
			total += p.RunsRecorded
		}
	}
	return total
}

// ApplyPopulationConfidence applies confidence only from the matching
// qualified population. Unknown fingerprints and legacy records retain the
// current run's unqualified behavior.
func ApplyPopulationConfidence(record *Record, fingerprint PopulationFingerprint, behavior profile.BehaviorProfile) profile.BehaviorProfile {
	if record == nil || !fingerprint.Valid() {
		return behavior
	}
	for _, p := range record.Populations {
		if p.Qualified && populationFingerprint(p) == fingerprint {
			return ApplyConfidence(&Record{RunsRecorded: p.RunsRecorded, FilesystemAccesses: p.FilesystemAccesses, NetworkAccesses: p.NetworkAccesses, SyscallAccesses: p.SyscallAccesses, CapabilityAccesses: p.CapabilityAccesses}, behavior)
		}
	}
	return behavior
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

// ApplyConfidence returns a BehaviorProfile built from record's full
// cross-run history — every access ever recorded, not just this run's —
// with each access's Confidence recomputed from record's cross-run ratio
// (SeenInRuns/RunsRecorded — high only once seen on every recorded run).
// record may be nil (no history yet): behavior is returned unchanged,
// keeping internal/policy.confidenceFor's single-run heuristic as the
// fallback.
//
// Building from record rather than behavior matters: SaveWithMerge has
// already folded this run's behavior into record before calling this
// function, so record is already the authoritative union of everything
// ever observed for this container/binary. An earlier version of this
// function built the result from behavior (this run's accesses only)
// and merely patched in a Confidence value for entries record happened
// to also know about — so an access seen in an earlier run but not
// re-observed in the latest one (e.g. a rarely-hit code path) silently
// vanished from every exported artifact (NetworkPolicy, PodLock,
// SeccompProfile, ...) the moment a run didn't re-trigger it, even
// though its Confidence (Medium/Low) was specifically meant to
// communicate "seen before, just not every time" — not "forget it".
// Confidence is meaningless if the access it describes isn't there to
// read it on.
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

	accesses := make([]profile.FileAccess, len(record.FilesystemAccesses))
	for i, a := range record.FilesystemAccesses {
		accesses[i] = profile.FileAccess{
			Path:        a.Path,
			Permissions: a.Permissions,
			Confidence:  confidenceForHistory(a.SeenInRuns, record.RunsRecorded),
			SeenCount:   a.SeenInRuns,
		}
	}

	netAccesses := make([]profile.NetworkAccess, len(record.NetworkAccesses))
	for i, a := range record.NetworkAccesses {
		netAccesses[i] = profile.NetworkAccess{
			Port:       a.Port,
			Direction:  a.Direction,
			Confidence: confidenceForHistory(a.SeenInRuns, record.RunsRecorded),
			SeenCount:  a.SeenInRuns,
		}
	}

	syscallAccesses := make([]profile.SyscallAccess, len(record.SyscallAccesses))
	for i, a := range record.SyscallAccesses {
		syscallAccesses[i] = profile.SyscallAccess{
			Name:       a.Name,
			Confidence: confidenceForHistory(a.SeenInRuns, record.RunsRecorded),
			SeenCount:  a.SeenInRuns,
		}
	}

	capabilityAccesses := make([]profile.CapabilityAccess, len(record.CapabilityAccesses))
	for i, a := range record.CapabilityAccesses {
		capabilityAccesses[i] = profile.CapabilityAccess{
			Name:       a.Name,
			Confidence: confidenceForHistory(a.SeenInRuns, record.RunsRecorded),
			SeenCount:  a.SeenInRuns,
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
	case seenInRuns < 0 || runsRecorded < 0 || seenInRuns > runsRecorded:
		return profile.ConfidenceLow
	case runsRecorded >= 2 && seenInRuns == runsRecorded:
		return profile.ConfidenceHigh
	case runsRecorded >= 2 && seenInRuns*2 >= runsRecorded:
		return profile.ConfidenceMedium
	default:
		return profile.ConfidenceLow
	}
}
