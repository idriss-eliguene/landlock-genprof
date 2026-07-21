// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package policy aggregates tracing events (internal/tracer) into a
// minimal Landlock profile, in a format compatible with PodLock's
// LandlockProfile CRD (see pkg/podlock).
package policy

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// Confidence indicates how certain a generated rule is, based on how many
// training runs it was observed in.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"    // seen only once
	ConfidenceMedium Confidence = "medium" // seen across multiple runs, inconsistently
	ConfidenceHigh   Confidence = "high"   // seen consistently
)

// Rule is a candidate rule before export to the PodLock format.
type Rule struct {
	Path       string
	Access     []string // e.g. "readExec", "readOnly", "readWrite"
	Confidence Confidence
	SeenCount  int
}

// maxAggregationDepth caps the directory depth kept for a rule. Beyond it,
// a subdirectory is merged into its ancestor at that depth — e.g.
// /usr/share/nginx/html and /usr/share/nginx/css both become the rule
// /usr/share/nginx (see README §8).
const maxAggregationDepth = 3

// dirAccess accumulates the modes observed for a given directory, before
// being synthesized into PodLock access categories (readExec/readOnly/readWrite).
type dirAccess struct {
	seenCount int
	read      bool
	write     bool
	exec      bool
}

// Synthesize aggregates a list of events (from a training run) into a
// minimal set of rules, one per directory — not per file, to avoid
// overfitting on overly specific paths.
//
// Only events carrying a file path (openat/execve) are considered: the
// PodLock output format (pkg/podlock.BinaryProfile) doesn't represent
// network rights yet, so connect/bind events (with no Path) are ignored
// here.
//
// Confidence heuristic (v1, single run): based on how many events were
// aggregated into the directory. The multi-run calculation described in
// docs/threat-model.md §2 ("seen on every run" vs "seen once out of 5
// runs") requires persisting results across multiple Synthesize calls,
// which isn't wired up yet (see roadmap M5).
func Synthesize(events []tracer.Event) ([]Rule, error) {
	byDir := make(map[string]*dirAccess)

	for _, ev := range events {
		if ev.Path == "" {
			continue
		}
		dir := aggregationDir(ev.Path)

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

	rules := make([]Rule, 0, len(dirs))
	for _, dir := range dirs {
		acc := byDir[dir]

		var access []string
		if acc.exec {
			access = append(access, "readExec")
		}
		switch {
		case acc.write:
			access = append(access, "readWrite")
		case acc.read:
			access = append(access, "readOnly")
		}

		rules = append(rules, Rule{
			Path:       dir,
			Access:     access,
			Confidence: confidenceFor(acc.seenCount),
			SeenCount:  acc.seenCount,
		})
	}

	return rules, nil
}

// aggregationDir returns the file's parent directory, truncated to
// maxAggregationDepth segments from the root.
func aggregationDir(path string) string {
	dir := filepath.Dir(path)
	segments := strings.Split(strings.Trim(dir, "/"), "/")
	if len(segments) > maxAggregationDepth {
		segments = segments[:maxAggregationDepth]
	}
	return "/" + strings.Join(segments, "/")
}

func confidenceFor(seenCount int) Confidence {
	switch {
	case seenCount >= 3:
		return ConfidenceHigh
	case seenCount == 2:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}
