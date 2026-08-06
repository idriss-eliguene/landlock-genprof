// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

func mockExplainCandidate() landlock.Candidate {
	return landlock.Candidate{
		Rules: []landlock.Rule{
			{
				Path:       "/etc/nginx",
				Rights:     []landlock.LandlockRight{landlock.LandlockRightReadFile, landlock.LandlockRightReadDir},
				Confidence: landlock.ConfidenceHigh,
				SeenCount:  3,
				Evidence: []landlock.EvidenceRef{
					{Source: "run-1", Timestamp: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)},
					{Source: "run-2", Timestamp: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)},
				},
			},
			{
				Path:       "/var/cache/nginx",
				Rights:     []landlock.LandlockRight{landlock.LandlockRightWriteFile, landlock.LandlockRightTruncate},
				Confidence: landlock.ConfidenceLow,
				SeenCount:  1,
			},
		},
	}
}

// writeCandidateFixture is defined in verify_test.go (same package).

func TestRunExplain_AllRules(t *testing.T) {
	candidateFile := writeCandidateFixture(t, mockExplainCandidate())

	var buf bytes.Buffer
	if err := runExplain(&buf, explainOptions{candidateFile: candidateFile}); err != nil {
		t.Fatalf("runExplain() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"/etc/nginx",
		"Confidence: high (seen 3 time(s))",
		"read_file",
		"ABI 1",
		"read_dir",
		"/var/cache/nginx",
		"truncate",
		"ABI 3",
		"kernel >= 6.2",
		"run-1",
		"run-2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunExplain_FilterByPath(t *testing.T) {
	candidateFile := writeCandidateFixture(t, mockExplainCandidate())

	var buf bytes.Buffer
	if err := runExplain(&buf, explainOptions{candidateFile: candidateFile, path: "/etc/nginx"}); err != nil {
		t.Fatalf("runExplain() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "/etc/nginx") {
		t.Errorf("output missing the requested rule:\n%s", out)
	}
	if strings.Contains(out, "/var/cache/nginx") {
		t.Errorf("output should not include a rule that wasn't requested:\n%s", out)
	}
}

func TestRunExplain_UnknownPath(t *testing.T) {
	candidateFile := writeCandidateFixture(t, mockExplainCandidate())

	var buf bytes.Buffer
	err := runExplain(&buf, explainOptions{candidateFile: candidateFile, path: "/nonexistent"})
	if err == nil {
		t.Fatal("runExplain() error = nil, want an error for a path with no matching rule")
	}
}

func TestRunExplain_NoEvidence(t *testing.T) {
	candidateFile := writeCandidateFixture(t, mockExplainCandidate())

	var buf bytes.Buffer
	if err := runExplain(&buf, explainOptions{candidateFile: candidateFile, path: "/var/cache/nginx"}); err != nil {
		t.Fatalf("runExplain() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Evidence: none recorded") {
		t.Errorf("output missing the no-evidence message:\n%s", buf.String())
	}
}

func TestRunExplain_MissingCandidateFileFlag(t *testing.T) {
	var buf bytes.Buffer
	err := runExplain(&buf, explainOptions{})
	if err == nil {
		t.Fatal("runExplain() error = nil, want an error when --candidate-file is empty")
	}
}

func TestRunExplain_EmptyCandidate(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{})

	var buf bytes.Buffer
	if err := runExplain(&buf, explainOptions{candidateFile: candidateFile}); err != nil {
		t.Fatalf("runExplain() error = %v", err)
	}
	if !strings.Contains(buf.String(), "No rules") {
		t.Errorf("output missing the no-rules message:\n%s", buf.String())
	}
}
