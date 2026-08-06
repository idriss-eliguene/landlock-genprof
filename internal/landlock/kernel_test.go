// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package landlock

import (
	"reflect"
	"sort"
	"testing"
)

func TestSynthesize_AggregatesByDirectory(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/usr/share/nginx/index.html", Operation: OperationRead},
		{Path: "/usr/share/nginx/css/style.css", Operation: OperationRead},
		{Path: "/tmp/nginx.pid", Operation: OperationWrite},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	rules := report.Candidate.Rules

	// No rule per individual file: the two files under /usr/share/nginx
	// (one in a css/ subdirectory) must merge into a single rule.
	if len(rules) != 2 {
		t.Fatalf("len(Rules) = %d, want 2 (no per-file rule): %+v", len(rules), rules)
	}

	byPath := make(map[string]Rule, len(rules))
	for _, r := range rules {
		byPath[r.Path] = r
	}

	nginx, ok := byPath["/usr/share/nginx"]
	if !ok {
		t.Fatalf("expected a rule for /usr/share/nginx, got: %+v", rules)
	}
	if !reflect.DeepEqual(nginx.Rights, []LandlockRight{LandlockRightReadFile}) {
		t.Errorf("/usr/share/nginx Rights = %v, want [read_file]", nginx.Rights)
	}
	if nginx.SeenCount != 2 {
		t.Errorf("/usr/share/nginx SeenCount = %d, want 2", nginx.SeenCount)
	}

	tmp, ok := byPath["/tmp"]
	if !ok {
		t.Fatalf("expected a rule for /tmp, got: %+v", rules)
	}
	if !reflect.DeepEqual(tmp.Rights, []LandlockRight{LandlockRightWriteFile}) {
		t.Errorf("/tmp Rights = %v, want [write_file]", tmp.Rights)
	}
}

// TestSynthesize_DistinguishesReadFileFromReadDir is the actual point of
// deepening Rights beyond a coarse read/write/execute label: a plain
// file read and a directory listing are different real Landlock rights
// (READ_FILE vs. READ_DIR), and this package already carries the IsDir
// bit needed to tell them apart honestly — see kernel.go's own package
// doc for why this is currently the ceiling (the other ABI1 rights, and
// everything ABI2+, would need the tracer to observe syscalls this
// project doesn't capture yet).
func TestSynthesize_DistinguishesReadFileFromReadDir(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/etc/nginx/nginx.conf", Operation: OperationRead},   // file, aggregates to parent /etc/nginx
		{Path: "/etc/nginx", Operation: OperationRead, IsDir: true}, // directory itself, aggregates to /etc/nginx too
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	rules := report.Candidate.Rules
	if len(rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1 (both aggregate under /etc/nginx): %+v", len(rules), rules)
	}
	want := []LandlockRight{LandlockRightReadFile, LandlockRightReadDir}
	if !reflect.DeepEqual(rules[0].Rights, want) {
		t.Errorf("Rights = %v, want %v", rules[0].Rights, want)
	}
}

// TestSynthesize_TruncateIsOrthogonalToOperation checks that Truncate
// (ABI3) is tracked independently of the read/write operation switch —
// a write that also truncates produces both LandlockRightWriteFile and
// LandlockRightTruncate, not one or the other.
func TestSynthesize_TruncateIsOrthogonalToOperation(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/var/lib/app/state.db", Operation: OperationWrite, Truncate: true},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	rules := report.Candidate.Rules
	if len(rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1: %+v", len(rules), rules)
	}
	want := []LandlockRight{LandlockRightWriteFile, LandlockRightTruncate}
	if !reflect.DeepEqual(rules[0].Rights, want) {
		t.Errorf("Rights = %v, want %v", rules[0].Rights, want)
	}
}

// TestSynthesize_TruncateWithoutWrite checks that a read-only observation
// carrying Truncate still produces LandlockRightTruncate on its own — the
// bit is read unconditionally, not gated on Operation being a write.
func TestSynthesize_TruncateWithoutWrite(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/tmp/scratch", Operation: OperationRead, Truncate: true},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	rules := report.Candidate.Rules
	if len(rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1: %+v", len(rules), rules)
	}
	want := []LandlockRight{LandlockRightReadFile, LandlockRightTruncate}
	if !reflect.DeepEqual(rules[0].Rights, want) {
		t.Errorf("Rights = %v, want %v", rules[0].Rights, want)
	}
}

// TestSynthesize_DirectoryOpenIsNotItsOwnParent mirrors
// internal/policy.TestSynthesize_DirectoryOpenIsNotItsOwnParent — same
// real bug, same fix, now guarded here too since this package inherits
// the algorithm.
func TestSynthesize_DirectoryOpenIsNotItsOwnParent(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/etc", Operation: OperationRead, IsDir: true},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	rules := report.Candidate.Rules

	if len(rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1: %+v", len(rules), rules)
	}
	if rules[0].Path != "/etc" {
		t.Errorf("Path = %q, want /etc (not its parent /)", rules[0].Path)
	}
}

func TestSynthesize_IgnoresRelativePaths(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "nginx.conf", Operation: OperationRead},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(report.Candidate.Rules) != 0 {
		t.Errorf("len(Rules) = %d, want 0 (relative path should be ignored): %+v",
			len(report.Candidate.Rules), report.Candidate.Rules)
	}
}

func TestSynthesize_ExecAndWriteBothInRightSet(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/opt/app/run", Operation: OperationExecute},
		{Path: "/opt/app/state.db", Operation: OperationWrite},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	rules := report.Candidate.Rules
	if len(rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1: %+v", len(rules), rules)
	}
	want := []LandlockRight{LandlockRightWriteFile, LandlockRightExecute}
	if !reflect.DeepEqual(rules[0].Rights, want) {
		t.Errorf("Rights = %v, want %v", rules[0].Rights, want)
	}
}

// TestSynthesize_ReadWriteIsOneObservationNotTwo checks that a single
// OperationReadWrite observation (e.g. an O_RDWR open) contributes exactly
// one evidence entry and one SeenCount increment — not two, which
// splitting it into separate Read and Write observations would cause.
// This is the exact translation this package's caller (internal/policy)
// relies on to stay byte-for-byte equivalent to tracer.Event's own
// "read_write" Mode.
func TestSynthesize_ReadWriteIsOneObservationNotTwo(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/data/db.sqlite", Operation: OperationReadWrite},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	rules := report.Candidate.Rules
	if len(rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1: %+v", len(rules), rules)
	}
	rule := rules[0]
	if rule.SeenCount != 1 {
		t.Errorf("SeenCount = %d, want 1", rule.SeenCount)
	}
	if len(rule.Evidence) != 1 {
		t.Errorf("len(Evidence) = %d, want 1", len(rule.Evidence))
	}
	want := []LandlockRight{LandlockRightReadFile, LandlockRightWriteFile}
	if !reflect.DeepEqual(rule.Rights, want) {
		t.Errorf("Rights = %v, want %v", rule.Rights, want)
	}
}

func TestSynthesize_EmptyInput(t *testing.T) {
	report, err := Synthesize(nil)
	if err != nil {
		t.Fatalf("Synthesize(nil) error = %v", err)
	}
	if len(report.Candidate.Rules) != 0 {
		t.Errorf("len(Rules) = %d, want 0", len(report.Candidate.Rules))
	}
}

// TestSynthesize_ProvenancePreserved enforces the invariant this package
// exists to guarantee: every Rule traces back to real evidence, and the
// evidence count always matches SeenCount exactly — no right is ever
// synthesized without a caller being able to show why.
func TestSynthesize_ProvenancePreserved(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/etc/nginx/nginx.conf", Operation: OperationRead, Evidence: EvidenceRef{Source: "run-1"}},
		{Path: "/etc/nginx/mime.types", Operation: OperationRead, Evidence: EvidenceRef{Source: "run-1"}},
		{Path: "/etc/nginx/conf.d", Operation: OperationRead, Evidence: EvidenceRef{Source: "run-2"}, IsDir: true},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	for _, rule := range report.Candidate.Rules {
		if len(rule.Evidence) == 0 {
			t.Errorf("rule %q has no evidence — every synthesized right must be traceable", rule.Path)
		}
		if len(rule.Evidence) != rule.SeenCount {
			t.Errorf("rule %q: len(Evidence) = %d, SeenCount = %d, want equal",
				rule.Path, len(rule.Evidence), rule.SeenCount)
		}
	}
}

// TestSynthesize_NoRightsInventedForUnobservedPaths is the structural
// counterpart of the provenance test above: every Rule's Path must be
// derivable from at least one real observation's own path (via
// aggregationDir) — nothing here should ever synthesize a rule for a
// directory that was never actually touched.
func TestSynthesize_NoRightsInventedForUnobservedPaths(t *testing.T) {
	observations := []FilesystemObservation{
		{Path: "/usr/sbin/nginx", Operation: OperationExecute},
		{Path: "/var/log/nginx/access.log", Operation: OperationWrite},
	}

	report, err := Synthesize(observations)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	observedDirs := make(map[string]bool)
	for _, obs := range observations {
		observedDirs[aggregationDir(obs.Path, obs.IsDir)] = true
	}

	for _, rule := range report.Candidate.Rules {
		if !observedDirs[rule.Path] {
			t.Errorf("rule for %q has no corresponding observation", rule.Path)
		}
	}
}

// TestSynthesize_OrderIndependent checks that Synthesize's output doesn't
// depend on the order observations arrive in — a caller (a future
// internal/policy delegating here) shouldn't need to pre-sort anything,
// and any accidental order-dependence would make Phase 2's golden-test
// comparison against internal/policy's current output flaky.
func TestSynthesize_OrderIndependent(t *testing.T) {
	forward := []FilesystemObservation{
		{Path: "/etc/nginx/nginx.conf", Operation: OperationRead},
		{Path: "/usr/sbin/nginx", Operation: OperationExecute},
		{Path: "/var/log/nginx/access.log", Operation: OperationWrite},
		{Path: "/etc/nginx/mime.types", Operation: OperationRead},
	}
	reversed := make([]FilesystemObservation, len(forward))
	for i, obs := range forward {
		reversed[len(forward)-1-i] = obs
	}

	got, err := Synthesize(forward)
	if err != nil {
		t.Fatalf("Synthesize(forward) error = %v", err)
	}
	gotReversed, err := Synthesize(reversed)
	if err != nil {
		t.Fatalf("Synthesize(reversed) error = %v", err)
	}

	sortRules := func(rules []Rule) {
		sort.Slice(rules, func(i, j int) bool { return rules[i].Path < rules[j].Path })
	}
	sortRules(got.Candidate.Rules)
	sortRules(gotReversed.Candidate.Rules)

	if !reflect.DeepEqual(got, gotReversed) {
		t.Errorf("Synthesize order-dependent:\nforward  = %+v\nreversed = %+v", got, gotReversed)
	}
}

// TestConfidenceFor_MonotonicInSeenCount checks that Confidence never
// decreases as SeenCount grows — a caller relying on Confidence to gate
// trust shouldn't be able to observe it going backwards from more
// evidence.
func TestConfidenceFor_MonotonicInSeenCount(t *testing.T) {
	rank := map[Confidence]int{ConfidenceLow: 0, ConfidenceMedium: 1, ConfidenceHigh: 2}

	prev := confidenceFor(0)
	for n := 1; n <= 5; n++ {
		cur := confidenceFor(n)
		if rank[cur] < rank[prev] {
			t.Errorf("confidenceFor(%d) = %v ranks below confidenceFor(%d) = %v", n, cur, n-1, prev)
		}
		prev = cur
	}
}
