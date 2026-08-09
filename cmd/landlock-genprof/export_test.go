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

	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

func TestRunExport_PodLock_ToStdout(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{
			{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}},
			{Path: "/usr/sbin", Rights: []landlock.LandlockRight{landlock.LandlockRightExecute}},
		},
	})

	var buf bytes.Buffer
	err := runExport(&buf, exportOptions{
		candidateFile: candidateFile, format: "podlock",
		podName: "nginx-demo", namespace: "default", container: "nginx", binary: "/usr/sbin/nginx",
	})
	if err != nil {
		t.Fatalf("runExport() error = %v", err)
	}

	text := buf.String()
	for _, want := range []string{"kind: LandlockProfile", "name: nginx-demo", "readOnly:", "readExec:"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunExport_PodLock_ToFile(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}}},
	})
	out := filepath.Join(t.TempDir(), "profile.yaml")

	var buf bytes.Buffer
	err := runExport(&buf, exportOptions{
		candidateFile: candidateFile, format: "podlock",
		podName: "nginx-demo", namespace: "default", container: "nginx", binary: "/usr/sbin/nginx",
		out: out,
	})
	if err != nil {
		t.Fatalf("runExport() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Exported: "+out) {
		t.Errorf("stdout missing confirmation message: %s", buf.String())
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading exported file: %v", err)
	}
	if !strings.Contains(string(data), "kind: LandlockProfile") {
		t.Errorf("exported file missing expected content:\n%s", data)
	}
}

func TestRunExport_TruncateCollapsesToReadWrite(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{
			Path:   "/var/cache/nginx",
			Rights: []landlock.LandlockRight{landlock.LandlockRightWriteFile, landlock.LandlockRightTruncate},
		}},
	})

	var buf bytes.Buffer
	err := runExport(&buf, exportOptions{
		candidateFile: candidateFile, format: "podlock",
		podName: "nginx-demo", namespace: "default", container: "nginx", binary: "/usr/sbin/nginx",
	})
	if err != nil {
		t.Fatalf("runExport() error = %v", err)
	}
	// PodLock's schema has no field for truncate — it must still show up
	// as readWrite, not vanish silently (matches collapsePermissions'
	// own invariant, exercised here through the export path).
	if !strings.Contains(buf.String(), "readWrite:") {
		t.Errorf("output missing readWrite (truncate must collapse, not disappear):\n%s", buf.String())
	}
}

func TestRunExport_UnsupportedFormat(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{})

	var buf bytes.Buffer
	err := runExport(&buf, exportOptions{
		candidateFile: candidateFile, format: "yaml-but-not-really",
		podName: "nginx-demo", container: "nginx", binary: "/usr/sbin/nginx",
	})
	if !strings.Contains(err.Error(), "yaml-but-not-really") {
		t.Errorf("error = %v, want it to mention the unsupported format", err)
	}

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runExport() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}

func TestRunExport_DefaultsToPodlockFormat(t *testing.T) {
	candidateFile := writeCandidateFixture(t, landlock.Candidate{
		Rules: []landlock.Rule{{Path: "/etc/nginx", Rights: []landlock.LandlockRight{landlock.LandlockRightReadFile}}},
	})

	var buf bytes.Buffer
	err := runExport(&buf, exportOptions{
		candidateFile: candidateFile, // format left empty
		podName:       "nginx-demo", container: "nginx", binary: "/usr/sbin/nginx",
	})
	if err != nil {
		t.Fatalf("runExport() error = %v", err)
	}
	if !strings.Contains(buf.String(), "kind: LandlockProfile") {
		t.Errorf("empty --format should default to podlock:\n%s", buf.String())
	}
}

func TestRunExport_NonexistentCandidateFile(t *testing.T) {
	var buf bytes.Buffer
	err := runExport(&buf, exportOptions{
		candidateFile: "/nonexistent/candidate.json", format: "podlock",
		podName: "nginx-demo", container: "nginx", binary: "/usr/sbin/nginx",
	})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runExport() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}

func TestRunExport_MalformedCandidateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("writing malformed fixture: %v", err)
	}

	var buf bytes.Buffer
	err := runExport(&buf, exportOptions{
		candidateFile: path, format: "podlock",
		podName: "nginx-demo", container: "nginx", binary: "/usr/sbin/nginx",
	})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runExport() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}
