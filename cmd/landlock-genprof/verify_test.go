// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/landlockjson"
	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

// writeCandidateFixture serializes candidate via the real
// internal/exporter/landlockjson.ToJSON (not a hand-typed fixture, so
// this test exercises the actual format runVerify has to parse) to a
// temp file and returns its path.
func writeCandidateFixture(t *testing.T, candidate landlock.Candidate) string {
	t.Helper()
	data, err := landlockjson.ToJSON(candidate)
	if err != nil {
		t.Fatalf("landlockjson.ToJSON() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing candidate fixture: %v", err)
	}
	return path
}

func TestRunVerify_AllABI1_CompatibleWithOlderKernel(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{
			{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}},
			{Path: "/usr/sbin", Rights: []landlock.LandlockRight{landlock.LandlockRightExecute}},
		},
	})

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: candidateFile, kernel: "5.13.0"})
	if err != nil {
		t.Fatalf("runVerify() error = %v, want nil (ABI1 rights fully compatible with ABI1 kernel)", err)
	}
	out := buf.String()
	if !strings.Contains(out, "All rules compatible") {
		t.Errorf("output missing compatibility confirmation:\n%s", out)
	}
	if !strings.Contains(out, "✓ /etc/nginx") || !strings.Contains(out, "✓ /usr/sbin") {
		t.Errorf("output missing per-rule confirmation:\n%s", out)
	}
}

func TestRunVerify_TruncateIncompatibleWithOlderKernel(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{
			{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}},
			{Path: "/var/lib/app/state.db", Rights: []landlock.LandlockRight{
				landlock.LandlockRightWriteFile, landlock.LandlockRightTruncate,
			}},
		},
	})

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: candidateFile, kernel: "5.19.0"}) // ABI2: has refer, not truncate (ABI3)

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runVerify() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("ExitCode() = %d, want 2 (blocking incompatibility)", exitErr.ExitCode())
	}

	out := buf.String()
	if !strings.Contains(out, "✓ /etc/nginx") {
		t.Errorf("output should still confirm the compatible rule:\n%s", out)
	}
	if !strings.Contains(out, "✗ /var/lib/app/state.db") {
		t.Errorf("output missing the incompatible rule:\n%s", out)
	}
	if !strings.Contains(out, "ABI 3") || !strings.Contains(out, "6.2") {
		t.Errorf("output should name the ABI level and kernel version truncate actually needs:\n%s", out)
	}
}

func TestRunVerify_KernelTooOldForLandlockAtAll(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}}},
	})

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: candidateFile, kernel: "5.10.0"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runVerify() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("ExitCode() = %d, want 2", exitErr.ExitCode())
	}
	if !strings.Contains(buf.String(), "not supported at all") {
		t.Errorf("output missing the no-Landlock-at-all message:\n%s", buf.String())
	}
}

func TestRunVerify_MissingCandidateFileFlag(t *testing.T) {
	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{kernel: "6.8.0"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runVerify() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error, matching diff's own contract, distinct from 2 = blocking incompatibility)", exitErr.ExitCode())
	}
}

func TestRunVerify_NonexistentCandidateFile(t *testing.T) {
	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: "/nonexistent/candidate.json", kernel: "6.8.0"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runVerify() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("Error() = %q, want it to mention the missing file", err.Error())
	}
}

func TestRunVerify_MalformedCandidateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("writing malformed fixture: %v", err)
	}

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: path, kernel: "6.8.0"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runVerify() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}

func TestRunVerify_InvalidKernelVersion(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{})

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: candidateFile, kernel: "not-a-version"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runVerify() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}

func TestRunVerify_SARIFOutput_Incompatible(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{
			{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}},
			{Path: "/var/lib/app/state.db", Rights: []landlock.LandlockRight{
				landlock.LandlockRightWriteFile, landlock.LandlockRightTruncate,
			}},
		},
	})

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: candidateFile, kernel: "5.19.0", output: "sarif"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runVerify() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("ExitCode() = %d, want 2 (blocking incompatibility, same contract as text output)", exitErr.ExitCode())
	}

	out := buf.String()
	if strings.Contains(out, "✓") || strings.Contains(out, "✗") {
		t.Errorf("SARIF output should not contain the text-mode checkmarks:\n%s", out)
	}
	for _, want := range []string{
		`"version": "2.1.0"`,
		`"ruleId": "landlock-abi-incompatible"`,
		`/var/lib/app/state.db`,
		`"landlock-genprof"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("SARIF output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/etc/nginx") {
		t.Errorf("SARIF output should not include a result for the compatible rule:\n%s", out)
	}
}

func TestRunVerify_SARIFOutput_AllCompatible(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}}},
	})

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: candidateFile, kernel: "6.8.0", output: "sarif"})
	if err != nil {
		t.Fatalf("runVerify() error = %v, want nil (compatible)", err)
	}
	if !strings.Contains(buf.String(), `"results": []`) {
		t.Errorf("SARIF output should have an explicit empty results array for a clean run:\n%s", buf.String())
	}
}

func TestRunVerify_UnsupportedOutputFormat(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{})

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: candidateFile, kernel: "6.8.0", output: "yaml"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runVerify() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}

func TestRunVerify_EmptyCandidateFileIsVacuouslyCompatible(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{})

	var buf bytes.Buffer
	err := runVerify(&buf, verifyOptions{candidateFile: candidateFile, kernel: "6.8.0"})
	if err != nil {
		t.Fatalf("runVerify() error = %v, want nil (no rules, nothing to be incompatible with)", err)
	}
	if !strings.Contains(buf.String(), "All rules compatible") {
		t.Errorf("output missing compatibility confirmation:\n%s", buf.String())
	}
}
