// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package landlock is a narrow, filesystem-only synthesis kernel: observed
// path accesses in, a reviewable, evidence-backed rule candidate out —
// nothing else.
//
// This is Phase 1 of a deliberately staged extraction (see
// docs/landlock-kernel-extraction.md for the full decision record):
// internal/policy/internal/profile's existing filesystem-aggregation logic
// stays exactly where it is for now (this package isn't wired into
// anything yet — that's Phase 2), and this package makes no public API
// commitment (it stays under internal/ through Phase 3, promoted to pkg/
// only once it has survived two independent exporters — see the decision
// doc).
//
// Deliberately excluded from this package, on purpose, not by oversight:
//   - Kubernetes types (internal/k8s, k8s.io/..., sigs.k8s.io/yaml)
//   - internal/profile's own cross-domain BehaviorProfile (network/
//     syscalls/capabilities) — publishing that shape risks duplicating
//     Kubescape's Software Bill of Behaviors; this package only ever
//     claims the one domain nothing else in the ecosystem owns
//   - internal/exporter/podlock or pkg/podlock — PodLock is one
//     serializer of this package's output, never the domain model itself
//   - internal/tracer — this package consumes plain FilesystemObservation
//     values, never a tracer.Event, so it never needs to know Inspektor
//     Gadget exists
//
// internal/landlock/deps_test.go enforces this statically, the same way
// internal/profile/deps_test.go already guards internal/profile's own
// independence from output formats.
//
// A Rule's Rights are real, ABI-versioned Landlock rights (LandlockRight,
// defined in abi.go) — not a coarse read/write/execute label. Today's
// honest ceiling: five rights this package can tell apart from a
// FilesystemObservation's Operation/IsDir/Truncate bits
// (LandlockRightReadFile, LandlockRightReadDir, LandlockRightWriteFile,
// LandlockRightExecute — all ABI1 — and LandlockRightTruncate, ABI3) are
// ever produced. REMOVE_DIR/REMOVE_FILE/MAKE_*/REFER/etc. would need the
// tracer to observe additional syscalls (unlink, mkdir, rename, ...) this
// project doesn't capture yet — Truncate is the one exception: it comes
// from the O_TRUNC bit of the same openat(2) flags value already read for
// Operation/IsDir, no new syscall hook needed. See
// docs/landlock-kernel-extraction.md's "known gap" section for the full
// accounting of what's honestly claimed here versus deferred.
package landlock

import (
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxAggregationDepth caps the directory depth kept for an observation.
// Beyond it, a subdirectory is merged into its ancestor at that depth —
// mirrors internal/policy.maxAggregationDepth exactly, since Phase 2
// replaces that package's filesystem branch with a call into this one and
// must not change its output (see the golden tests in
// internal/policy/golden_test.go and
// internal/exporter/podlock/golden_test.go).
const maxAggregationDepth = 3

// Operation is the kind of filesystem access a single observation
// recorded.
type Operation string

const (
	OperationRead    Operation = "read"
	OperationWrite   Operation = "write"
	OperationExecute Operation = "execute"
	// OperationReadWrite is a single observation carrying both rights at
	// once (e.g. an O_RDWR open) — kept distinct from reporting Read and
	// Write as two separate observations, since that would double the
	// evidence trail and SeenCount for what was really one occurrence.
	OperationReadWrite Operation = "read_write"
)

// EvidenceRef is a pointer back to what produced an observation —
// provenance, not a replay of the raw event itself. Source is opaque to
// this package (a caller-defined identifier, e.g. a trace-run ID); this
// package only ever accumulates and returns it, never interprets it.
type EvidenceRef struct {
	Source    string
	Timestamp time.Time
}

// FilesystemObservation is one filesystem access seen during a training
// run, already reduced to exactly what this package needs — a caller
// (internal/policy, eventually) is responsible for translating whatever
// its own capture mechanism produces (a tracer.Event today) into this
// shape.
type FilesystemObservation struct {
	Path      string
	Operation Operation
	// IsDir is true when Path itself was opened as a directory (e.g. `ls
	// <dir>` opens <dir> with O_DIRECTORY to list it), as opposed to a
	// regular file — aggregating by the *parent* of an opened path is
	// correct for a file, wrong for a directory. Mirrors
	// tracer.Event.IsDir's own doc comment.
	IsDir bool
	// Truncate is true when this access included Landlock's own TRUNCATE
	// right (ABI3) — independent of Operation: a write can truncate or
	// not. Mirrors tracer.Event.Truncate's own doc comment.
	Truncate bool
	Evidence EvidenceRef
}

// Confidence indicates how certain a synthesized Rule is, based on how
// many observations support it.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"    // seen only once
	ConfidenceMedium Confidence = "medium" // seen twice
	ConfidenceHigh   Confidence = "high"   // seen three or more times
)

// Rule is one path's synthesized right set. Evidence is never empty for a
// Rule this package produced — every generated right traces back to at
// least one real observation (see Synthesize's own invariant, enforced by
// this package's own tests, not just documented here).
type Rule struct {
	Path       string
	Rights     []LandlockRight
	Evidence   []EvidenceRef
	Confidence Confidence
	SeenCount  int
}

// Candidate is one synthesis run's rule set, pre-serialization — the type
// an eventual exporter (PodLock, a canonical JSON format, ...) consumes.
// Deliberately not PodLock-shaped: no joint "readWriteExec" category
// here, the same reasoning internal/profile.FileAccess's own doc comment
// already gives for why collapsing rights into an output format's own
// categories is an exporter's job, never this layer's.
type Candidate struct {
	Rules []Rule
}

// SynthesisReport is Synthesize's full, explainable output. Left
// deliberately minimal for now — Candidate is the only field with real
// content; explanatory fields (e.g. per-rule rationale) are added once a
// real consumer needs them, not speculatively ahead of one.
type SynthesisReport struct {
	Candidate Candidate
}

// Synthesize aggregates observations into a minimal rule candidate: one
// Rule per distinct directory (not per file, to avoid overfitting on
// overly specific paths), each carrying every right actually observed
// under it and the full evidence trail behind that decision.
//
// Relative paths (not starting with "/") are skipped: their actual target
// depends on the observed process's working directory, which observations
// don't carry, so there's no absolute filesystem location to turn into a
// rule — mirrors internal/policy.Synthesize's own documented reasoning.
//
// Always returns a nil error today — the signature matches
// internal/policy.Synthesize's own (which has the same property), kept
// for the same reason: a caller shouldn't have to change its own error
// handling the day this package's first real failure mode appears (e.g. a
// future input-validation rule).
func Synthesize(observations []FilesystemObservation) (SynthesisReport, error) {
	byDir := make(map[string]*dirAccess)

	for _, obs := range observations {
		if obs.Path == "" || !strings.HasPrefix(obs.Path, "/") {
			continue
		}
		dir := aggregationDir(obs.Path, obs.IsDir)

		acc, ok := byDir[dir]
		if !ok {
			acc = &dirAccess{}
			byDir[dir] = acc
		}
		acc.evidence = append(acc.evidence, obs.Evidence)

		// Truncate is orthogonal to Operation: co-occurs with a write
		// (O_WRONLY|O_TRUNC) but is its own right, not folded into the
		// write/read switch below.
		if obs.Truncate {
			acc.truncate = true
		}

		// Read is the one operation this package can honestly split into
		// two distinct real rights: READ_FILE vs. READ_DIR, using the same
		// IsDir bit aggregationDir already relies on. Write/execute stay
		// single-valued — opening a directory O_WRONLY/O_RDWR or executing
		// it isn't a real access the kernel permits via open(2) in the
		// first place, so there's no honest "WRITE_DIR"/directory-execute
		// case to split out here.
		switch obs.Operation {
		case OperationRead:
			if obs.IsDir {
				acc.readDir = true
			} else {
				acc.readFile = true
			}
		case OperationWrite:
			acc.write = true
		case OperationReadWrite:
			if obs.IsDir {
				acc.readDir = true
			} else {
				acc.readFile = true
			}
			acc.write = true
		case OperationExecute:
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
		rules = append(rules, Rule{
			Path:       dir,
			Rights:     rightsFor(acc),
			Evidence:   acc.evidence,
			Confidence: confidenceFor(len(acc.evidence)),
			SeenCount:  len(acc.evidence),
		})
	}

	return SynthesisReport{Candidate: Candidate{Rules: rules}}, nil
}

// dirAccess accumulates the rights and evidence observed for a given
// directory, before being turned into a Rule (see rightsFor).
type dirAccess struct {
	readFile, readDir, write, exec, truncate bool
	evidence                                 []EvidenceRef
}

// aggregationDir returns the directory a Rule should apply to, truncated
// to maxAggregationDepth segments from the root — byte-for-byte the same
// algorithm as internal/policy.aggregationDir, see maxAggregationDepth's
// own comment for why that has to stay true through Phase 2.
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

// rightsFor maps the observed readFile/readDir/write/exec bits to a
// Rule's real Landlock right set, in a fixed order (matching abi.go's own
// LandlockRight declaration order) for deterministic output.
func rightsFor(acc *dirAccess) []LandlockRight {
	var rights []LandlockRight
	if acc.readFile {
		rights = append(rights, LandlockRightReadFile)
	}
	if acc.readDir {
		rights = append(rights, LandlockRightReadDir)
	}
	if acc.write {
		rights = append(rights, LandlockRightWriteFile)
	}
	if acc.exec {
		rights = append(rights, LandlockRightExecute)
	}
	if acc.truncate {
		rights = append(rights, LandlockRightTruncate)
	}
	return rights
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
