// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package policy aggregates tracing events (internal/tracer) into a
// Behavior IR (internal/profile) — one FileAccess per directory, not per
// file, to avoid overfitting on overly specific paths. This package knows
// nothing about PodLock or any other output format: that translation is
// an exporter's job (see internal/exporter/podlock).
//
// The filesystem domain specifically delegates to internal/landlock (see
// docs/landlock-kernel-extraction.md): this package's own job there is
// reduced to translating tracer.Event into landlock.FilesystemObservation
// going in, and landlock.Candidate back into profile.FileAccess coming
// out — the directory-aggregation algorithm itself
// (maxAggregationDepth, the per-directory right/evidence accumulation)
// lives in internal/landlock now, not here. Network/syscalls/
// capabilities are unchanged — internal/landlock is filesystem-only by
// design (see its own package doc for why).
package policy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// ephemeralPortStart is the low end of Linux's default ephemeral port
// range (net.ipv4.ip_local_port_range, typically 32768-60999). A bind(2)
// on a port in this range is far more likely to be the kernel (or an nc-
// style client that binds explicitly before connect(), as busybox's nc
// does) picking a throwaway local port for an *outbound* connection than
// an actual server choosing to listen there — confirmed live: tracing a
// plain outbound `nc <ip> <port>` produced a `bind` event on a port in
// this range with no listener ever started. bind(2) can't be told apart
// from listen(2) at the syscall level trace_bind hooks, so this is a
// heuristic, not a certainty — same spirit as internal/landlock's own
// directory-aggregation heuristic, and same caveat: a service that
// deliberately listens above this
// threshold would be filtered out too. See docs/policy-synthesis.md.
const ephemeralPortStart = 32768

// netKey aggregates network accesses by the (port, direction) pair the
// NetworkPolicy exporter cares about — see internal/exporter/networkpolicy.
type netKey struct {
	port      int
	direction profile.NetworkDirection
}

// Synthesize aggregates a list of events (from a training run) into a
// minimal Behavior IR: one FilesystemProfile access per directory, one
// NetworkProfile access per (port, direction) pair, one SyscallProfile
// access per syscall name.
//
// Filesystem: only events carrying a file path (openat/execve) are
// considered. Relative paths (not starting with "/") are skipped: their
// actual target depends on the observed process's working directory,
// which we don't track, so there's no absolute filesystem location to
// turn into an access.
//
// Network: connect/bind events (no Path, but a nonzero Port) are
// aggregated separately into NetworkProfile — see
// internal/exporter/networkpolicy, the destination this data didn't have
// when network tracing was originally deferred (see docs/roadmap.md).
// bind events on an ephemeral port (>= ephemeralPortStart) are skipped:
// see ephemeralPortStart's own comment for why.
//
// Syscalls: events with Mode "syscall" (from internal/tracer's
// advise_seccomp integration) are aggregated into SyscallProfile.
// architectures is threaded through separately from events — see
// tracer.Trace's own doc comment for why it's a second return value
// rather than part of the Event stream.
//
// Capabilities: events with Mode "capability" (from internal/tracer's
// trace_capabilities integration) are aggregated into CapabilityProfile
// by capability name — a normal per-occurrence stream like filesystem/
// network, not the seccomp exception (see "Confidence heuristic" below).
//
// Confidence heuristic (v1, single run), shared across filesystem/
// network/capabilities: based on how many events were aggregated into
// the directory/port/capability, which can exceed 1 within a single run
// (an access observed on separate openat/connect/cap_capable calls).
// Syscalls are the one exception — advise_seccomp reports one
// deduplicated set per run, not one event per occurrence — so a
// syscall's Confidence is always Low without --history. This isn't a
// bug: it's the conservative default this domain needs, given a single
// training run can never prove a syscall is safe to omit going forward
// (see docs/policy-synthesis.md). The multi-run calculation described in
// docs/threat-model.md §2 ("seen on every run" vs "seen once out of 5
// runs") requires persisting results across multiple Synthesize calls —
// internal/history.
func Synthesize(events []tracer.Event, architectures []string) (profile.BehaviorProfile, error) {
	var fsObservations []landlock.FilesystemObservation
	byNet := make(map[netKey]int)        // seenCount
	bySyscall := make(map[string]int)    // seenCount
	byCapability := make(map[string]int) // seenCount

	for _, ev := range events {
		if ev.Mode == "syscall" {
			if ev.Syscall == "" {
				continue
			}
			bySyscall[ev.Syscall]++
			continue
		}
		if ev.Mode == "capability" {
			if ev.Syscall == "" {
				continue
			}
			byCapability[ev.Syscall]++
			continue
		}

		switch ev.Syscall {
		case "connect", "bind":
			if ev.Port == 0 {
				continue
			}
			direction := profile.DirectionEgress
			if ev.Syscall == "bind" {
				if ev.Port >= ephemeralPortStart {
					continue
				}
				direction = profile.DirectionIngress
			}
			byNet[netKey{port: ev.Port, direction: direction}]++
			continue
		}

		if ev.Path == "" || !strings.HasPrefix(ev.Path, "/") {
			continue
		}
		fsObservations = append(fsObservations, landlock.FilesystemObservation{
			Path:      ev.Path,
			Operation: operationFor(ev.Mode),
			IsDir:     ev.IsDir,
			Truncate:  ev.Truncate,
			Evidence:  landlock.EvidenceRef{Timestamp: ev.Timestamp},
		})
	}

	report, err := landlock.Synthesize(fsObservations)
	if err != nil {
		return profile.BehaviorProfile{}, fmt.Errorf("synthesizing filesystem rules: %w", err)
	}
	fsAccesses := fileAccessesFromCandidate(report.Candidate)

	netKeys := make([]netKey, 0, len(byNet))
	for k := range byNet {
		netKeys = append(netKeys, k)
	}
	sort.Slice(netKeys, func(i, j int) bool {
		if netKeys[i].port != netKeys[j].port {
			return netKeys[i].port < netKeys[j].port
		}
		return netKeys[i].direction < netKeys[j].direction
	})

	netAccesses := make([]profile.NetworkAccess, 0, len(netKeys))
	for _, k := range netKeys {
		seenCount := byNet[k]
		netAccesses = append(netAccesses, profile.NetworkAccess{
			Port:       k.port,
			Direction:  k.direction,
			Confidence: confidenceFor(seenCount),
			SeenCount:  seenCount,
		})
	}

	syscallNames := make([]string, 0, len(bySyscall))
	for name := range bySyscall {
		syscallNames = append(syscallNames, name)
	}
	sort.Strings(syscallNames)

	syscallAccesses := make([]profile.SyscallAccess, 0, len(syscallNames))
	for _, name := range syscallNames {
		seenCount := bySyscall[name]
		syscallAccesses = append(syscallAccesses, profile.SyscallAccess{
			Name:       name,
			Confidence: confidenceFor(seenCount),
			SeenCount:  seenCount,
		})
	}

	capabilityNames := make([]string, 0, len(byCapability))
	for name := range byCapability {
		capabilityNames = append(capabilityNames, name)
	}
	sort.Strings(capabilityNames)

	capabilityAccesses := make([]profile.CapabilityAccess, 0, len(capabilityNames))
	for _, name := range capabilityNames {
		seenCount := byCapability[name]
		capabilityAccesses = append(capabilityAccesses, profile.CapabilityAccess{
			Name:       name,
			Confidence: confidenceFor(seenCount),
			SeenCount:  seenCount,
		})
	}

	return profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: fsAccesses},
		Network:    profile.NetworkProfile{Accesses: netAccesses},
		Syscalls: profile.SyscallProfile{
			Accesses:      syscallAccesses,
			Architectures: architectures,
		},
		Capabilities: profile.CapabilityProfile{Accesses: capabilityAccesses},
	}, nil
}

// operationFor translates a tracer.Event's own Mode vocabulary ("exec",
// not "execute") into internal/landlock's Operation vocabulary — the two
// packages are deliberately allowed to name things differently
// (internal/landlock doesn't know Inspektor Gadget's Mode strings exist,
// see its own package doc), so this mapping has to be explicit, not a
// bare string conversion. An unrecognized mode is passed through as an
// opaque Operation value that won't match any of internal/landlock's own
// cases — the same graceful "observation recorded, no right set" behavior
// the original inline switch had for a Mode it didn't recognize.
func operationFor(mode string) landlock.Operation {
	switch mode {
	case "read":
		return landlock.OperationRead
	case "write":
		return landlock.OperationWrite
	case "read_write":
		return landlock.OperationReadWrite
	case "exec":
		return landlock.OperationExecute
	default:
		return landlock.Operation(mode)
	}
}

// fileAccessesFromCandidate translates internal/landlock's output back
// into the IR's own profile.FileAccess shape. Confidence stays a direct
// conversion (landlock.Confidence and profile.Confidence share identical
// low/medium/high values); Rights do not — see collapsePermissions.
func fileAccessesFromCandidate(c landlock.Candidate) []profile.FileAccess {
	accesses := make([]profile.FileAccess, 0, len(c.Rules))
	for _, r := range c.Rules {
		accesses = append(accesses, profile.FileAccess{
			Path:        r.Path,
			Permissions: collapsePermissions(r.Rights),
			Confidence:  profile.Confidence(r.Confidence),
			SeenCount:   r.SeenCount,
		})
	}
	return accesses
}

// collapsePermissions folds internal/landlock's real, ABI-versioned
// rights (e.g. LandlockRightReadFile vs. LandlockRightReadDir — finer
// than this package used to track itself, see
// docs/landlock-kernel-extraction.md's "known gap" section) down to the
// IR's coarser read/write/execute vocabulary every downstream exporter
// still expects, deduplicated (a Rule with both ReadFile and ReadDir
// rights must produce exactly one "read", not two) and in the same fixed
// read/write/execute order the golden tests already pin.
//
// LandlockRightTruncate has no coarse equivalent of its own — it's
// folded into "write", never dropped silently: a rule that needs
// truncate always needs at least write-level access in this coarser
// view, so collapsing it to write can never under-claim what an exported
// artifact actually needs (only lose the ABI3-specific distinction,
// which PodLock's own schema has no field for anyway).
func collapsePermissions(rights []landlock.LandlockRight) []profile.FilePermission {
	var read, write, exec bool
	for _, right := range rights {
		switch right {
		case landlock.LandlockRightReadFile, landlock.LandlockRightReadDir:
			read = true
		case landlock.LandlockRightWriteFile, landlock.LandlockRightTruncate:
			write = true
		case landlock.LandlockRightExecute:
			exec = true
		}
	}

	var perms []profile.FilePermission
	if read {
		perms = append(perms, profile.PermissionRead)
	}
	if write {
		perms = append(perms, profile.PermissionWrite)
	}
	if exec {
		perms = append(perms, profile.PermissionExecute)
	}
	return perms
}

func confidenceFor(seenCount int) profile.Confidence {
	switch {
	case seenCount >= 3:
		return profile.ConfidenceHigh
	case seenCount == 2:
		return profile.ConfidenceMedium
	default:
		return profile.ConfidenceLow
	}
}
