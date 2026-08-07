// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

func TestRunDiff_NoDifferences(t *testing.T) {
	candidate := landlock.Candidate{
		Rules: []landlock.Rule{{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}}},
	}
	oldFile := writeCandidateFixture(t, candidate)
	newFile := writeCandidateFixture(t, candidate)

	var buf bytes.Buffer
	err := runDiff(&buf, oldFile, newFile)
	if err != nil {
		t.Fatalf("runDiff() error = %v, want nil", err)
	}
	if !strings.Contains(buf.String(), "No differences") {
		t.Errorf("output missing no-differences message:\n%s", buf.String())
	}
}

func TestRunDiff_RuleAdded(t *testing.T) {
	oldFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}}},
	})
	newFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{
			{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}},
			{Path: "/var/cache/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightWriteFile}},
		},
	})

	var buf bytes.Buffer
	err := runDiff(&buf, oldFile, newFile)

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runDiff() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1 (differences found)", exitErr.ExitCode())
	}
	if !strings.Contains(buf.String(), "+ /var/cache/nginx") {
		t.Errorf("output missing the added rule:\n%s", buf.String())
	}
}

func TestRunDiff_RuleRemoved(t *testing.T) {
	oldFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{
			{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}},
			{Path: "/var/cache/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightWriteFile}},
		},
	})
	newFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}}},
	})

	var buf bytes.Buffer
	err := runDiff(&buf, oldFile, newFile)

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runDiff() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(buf.String(), "- /var/cache/nginx") {
		t.Errorf("output missing the removed rule:\n%s", buf.String())
	}
}

func TestRunDiff_RightsChanged(t *testing.T) {
	oldFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{Path: "/var/cache/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightWriteFile}}},
	})
	newFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{
			Path:   "/var/cache/nginx",
			Rights: []landlock.LandlockRight{landlock.LandlockRightWriteFile, landlock.LandlockRightTruncate},
		}},
	})

	var buf bytes.Buffer
	err := runDiff(&buf, oldFile, newFile)

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runDiff() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", exitErr.ExitCode())
	}
	out := buf.String()
	if !strings.Contains(out, "~ /var/cache/nginx") || !strings.Contains(out, "+[truncate]") {
		t.Errorf("output missing the rights-changed line:\n%s", out)
	}
}

func TestRunDiff_UsageErrorExitsThree(t *testing.T) {
	existingFile := writeCandidateFixture(t, landlock.Candidate{})

	var buf bytes.Buffer
	err := runDiff(&buf, "/nonexistent/old.json", existingFile)

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runDiff() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error, distinct from 1 = differences found)", exitErr.ExitCode())
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("Error() = %q, want it to mention the missing file", err.Error())
	}
}

func TestRunDiff_IdenticalRuleOrderDoesNotMatter(t *testing.T) {
	oldFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{
			{Path: "/a", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}},
			{Path: "/b", Rights: []landlock.LandlockRight{landlock.LandlockRightWriteFile}},
		},
	})
	newFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{
			{Path: "/b", Rights: []landlock.LandlockRight{landlock.LandlockRightWriteFile}},
			{Path: "/a", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}},
		},
	})

	var buf bytes.Buffer
	if err := runDiff(&buf, oldFile, newFile); err != nil {
		t.Fatalf("runDiff() error = %v, want nil (same rules, different order)", err)
	}
}
