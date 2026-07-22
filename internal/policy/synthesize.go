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

// Rule is a candidate rule before export to the PodLock format. Access
// holds exactly one of PodLock's four categories once populated
// ("readOnly", "readWrite", "readExec", "readWriteExec") — see
// categoryFor — never more than one.
type Rule struct {
	Path       string
	Access     []string
	Confidence Confidence
	SeenCount  int
}

// maxAggregationDepth caps the directory depth kept for a rule. Beyond it,
// a subdirectory is merged into its ancestor at that depth — e.g.
// /usr/share/nginx/html and /usr/share/nginx/css both become the rule
// /usr/share/nginx (see README §8).
const maxAggregationDepth = 3

// dirAccess accumulates the modes observed for a given directory, before
// being synthesized into a single PodLock access category (see categoryFor).
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
// PodLock output format (pkg/podlock.Profile) doesn't represent network
// rights at all — verified against PodLock's actual schema, not just our
// own earlier assumption — so connect/bind events (with no Path) are
// ignored here. Relative paths (not starting with "/") are also skipped: their
// actual target depends on the observed process's working directory,
// which we don't track, so there's no absolute filesystem location to
// turn into a Landlock rule.
//
// Confidence heuristic (v1, single run): based on how many events were
// aggregated into the directory. The multi-run calculation described in
// docs/threat-model.md §2 ("seen on every run" vs "seen once out of 5
// runs") requires persisting results across multiple Synthesize calls,
// which isn't wired up yet (see roadmap M5).
func Synthesize(events []tracer.Event) ([]Rule, error) {
	byDir := make(map[string]*dirAccess)

	for _, ev := range events {
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

	rules := make([]Rule, 0, len(dirs))
	for _, dir := range dirs {
		acc := byDir[dir]

		var access []string
		// category is "" only if no read/write/exec bit ended up set at
		// all, which shouldn't happen in practice (every event sets at
		// least one) — skip rather than emit an empty-access rule if it
		// ever does.
		if category := categoryFor(acc); category != "" {
			access = []string{category}
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

// aggregationDir returns the directory a rule should apply to, truncated
// to maxAggregationDepth segments from the root.
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

// categoryFor maps the observed read/write/exec bits to exactly one of
// PodLock's four access categories (see pkg/podlock.Profile) — not a
// combination of several. A path that's both executed and written to is
// "readWriteExec", a distinct category of its own, not "readExec" and
// "readWrite" reported side by side: that mismatch was caught by
// checking PodLock's real schema (github.com/flavio/podlock), which
// mirrors what Landlock itself groups as one enforcement decision. Each
// named category also implies read access, matching Landlock's own
// rights (there's no "execute but not read" bucket): executing or
// writing a file requires reading it first.
func categoryFor(acc *dirAccess) string {
	switch {
	case acc.exec && acc.write:
		return "readWriteExec"
	case acc.exec:
		return "readExec"
	case acc.write:
		return "readWrite"
	case acc.read:
		return "readOnly"
	default:
		return ""
	}
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
