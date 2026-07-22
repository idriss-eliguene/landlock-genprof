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
package policy

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// maxAggregationDepth caps the directory depth kept for an access. Beyond
// it, a subdirectory is merged into its ancestor at that depth — e.g.
// /usr/share/nginx/html and /usr/share/nginx/css both become the access
// /usr/share/nginx (see README §8).
const maxAggregationDepth = 3

// dirAccess accumulates the modes observed for a given directory, before
// being turned into an IR permission set (see permissionsFor).
type dirAccess struct {
	seenCount int
	read      bool
	write     bool
	exec      bool
}

// netKey aggregates network accesses by the (port, direction) pair the
// NetworkPolicy exporter cares about — see internal/exporter/networkpolicy.
type netKey struct {
	port      int
	direction profile.NetworkDirection
}

// Synthesize aggregates a list of events (from a training run) into a
// minimal Behavior IR: one FilesystemProfile access per directory, one
// NetworkProfile access per (port, direction) pair.
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
//
// Confidence heuristic (v1, single run), shared by both domains: based on
// how many events were aggregated into the directory/port. The multi-run
// calculation described in docs/threat-model.md §2 ("seen on every run"
// vs "seen once out of 5 runs") requires persisting results across
// multiple Synthesize calls, which isn't wired up yet (see roadmap M5).
func Synthesize(events []tracer.Event) (profile.BehaviorProfile, error) {
	byDir := make(map[string]*dirAccess)
	byNet := make(map[netKey]int) // seenCount

	for _, ev := range events {
		switch ev.Syscall {
		case "connect", "bind":
			if ev.Port == 0 {
				continue
			}
			direction := profile.DirectionEgress
			if ev.Syscall == "bind" {
				direction = profile.DirectionIngress
			}
			byNet[netKey{port: ev.Port, direction: direction}]++
			continue
		}

		if ev.Path == "" || !strings.HasPrefix(ev.Path, "/") {
			continue
		}
		dir := aggregationDir(ev.Path, ev.IsDir)

		acc, ok := byDir[dir]
		if !ok {
			acc = &dirAccess{}
			byDir[dir] = acc
		}
		acc.seenCount++

		switch ev.Mode {
		case "read":
			acc.read = true
		case "write":
			acc.write = true
		case "read_write":
			acc.read = true
			acc.write = true
		case "exec":
			acc.exec = true
		}
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	fsAccesses := make([]profile.FileAccess, 0, len(dirs))
	for _, dir := range dirs {
		acc := byDir[dir]

		fsAccesses = append(fsAccesses, profile.FileAccess{
			Path:        dir,
			Permissions: permissionsFor(acc),
			Confidence:  confidenceFor(acc.seenCount),
			SeenCount:   acc.seenCount,
		})
	}

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

	return profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: fsAccesses},
		Network:    profile.NetworkProfile{Accesses: netAccesses},
	}, nil
}

// aggregationDir returns the directory an access should apply to,
// truncated to maxAggregationDepth segments from the root.
//
// For a regular file, that's its parent directory (filepath.Dir). For a
// path that was itself opened as a directory (isDir — e.g. `ls /etc`
// opens /etc with O_DIRECTORY to list it), the parent would be wrong:
// /etc opened directly is not "some file under /", it's /etc itself.
// Found from a real training run that produced a nonsensical
// `readOnly: [/]` rule before this distinction existed — see
// docs/policy-synthesis.md.
func aggregationDir(path string, isDir bool) string {
	dir := path
	if !isDir {
		dir = filepath.Dir(path)
	}
	segments := strings.Split(strings.Trim(dir, "/"), "/")
	if len(segments) > maxAggregationDepth {
		segments = segments[:maxAggregationDepth]
	}
	return "/" + strings.Join(segments, "/")
}

// permissionsFor maps the observed read/write/exec bits to the IR's
// permission set, in a fixed read/write/execute order for deterministic
// output. Collapsing this set into a single joint category (like
// PodLock's "readWriteExec") is an exporter's job, not this package's —
// see internal/exporter/podlock.
func permissionsFor(acc *dirAccess) []profile.FilePermission {
	var perms []profile.FilePermission
	if acc.read {
		perms = append(perms, profile.PermissionRead)
	}
	if acc.write {
		perms = append(perms, profile.PermissionWrite)
	}
	if acc.exec {
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
