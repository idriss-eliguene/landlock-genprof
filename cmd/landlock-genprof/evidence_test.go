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
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/evidence"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func writeEvidenceFixtureFile(t *testing.T, events []tracer.Event, architectures []string) string {
	t.Helper()
	data, err := evidence.ToJSON(events, architectures)
	if err != nil {
		t.Fatalf("evidence.ToJSON() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "events.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing evidence fixture: %v", err)
	}
	return path
}

func TestRunEvidenceShow_MixedEvents(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/etc/nginx/nginx.conf", Mode: "read", Timestamp: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)},
		{Syscall: "openat", Path: "/usr/sbin/nginx", Mode: "exec", Timestamp: time.Date(2026, 8, 6, 12, 0, 1, 0, time.UTC)},
		{Syscall: "openat", Path: "/var/cache/nginx", Mode: "write", Timestamp: time.Date(2026, 8, 6, 12, 0, 2, 0, time.UTC)},
		{Syscall: "connect", Port: 443, Mode: "egress", Timestamp: time.Date(2026, 8, 6, 12, 0, 3, 0, time.UTC)},
		{Syscall: "bind", Port: 80, Mode: "ingress", Timestamp: time.Date(2026, 8, 6, 12, 0, 4, 0, time.UTC)},
		{Syscall: "epoll_wait", Mode: "syscall"},
		{Syscall: "CAP_NET_BIND_SERVICE", Mode: "capability"},
	}
	path := writeEvidenceFixtureFile(t, events, []string{"SCMP_ARCH_X86_64"})

	var buf bytes.Buffer
	if err := runEvidenceShow(&buf, path); err != nil {
		t.Fatalf("runEvidenceShow() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"7 event(s)",
		"Filesystem: 3 event(s) across 3 distinct path(s)",
		"exec: 1",
		"read: 1",
		"write: 1",
		"Network: 2 event(s) across 2 distinct port(s)",
		"Syscalls: 1 distinct",
		"Capabilities: 1 distinct",
		"SCMP_ARCH_X86_64",
		"Observed: 2026-08-06T12:00:00Z to 2026-08-06T12:00:04Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunEvidenceShow_EmptyEvidence(t *testing.T) {
	path := writeEvidenceFixtureFile(t, nil, nil)

	var buf bytes.Buffer
	if err := runEvidenceShow(&buf, path); err != nil {
		t.Fatalf("runEvidenceShow() error = %v", err)
	}
	if !strings.Contains(buf.String(), "0 event(s)") {
		t.Errorf("output missing zero-event message:\n%s", buf.String())
	}
}

func TestRunEvidenceShow_NoTimestamps(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/etc/nginx/nginx.conf", Mode: "read"},
	}
	path := writeEvidenceFixtureFile(t, events, nil)

	var buf bytes.Buffer
	if err := runEvidenceShow(&buf, path); err != nil {
		t.Fatalf("runEvidenceShow() error = %v", err)
	}
	if strings.Contains(buf.String(), "Observed:") {
		t.Errorf("output should not print an observation window when no event has a timestamp:\n%s", buf.String())
	}
}

func TestRunEvidenceShow_NonexistentFile(t *testing.T) {
	var buf bytes.Buffer
	err := runEvidenceShow(&buf, "/nonexistent/events.json")

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runEvidenceShow() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}

func TestRunEvidenceShow_MalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("writing malformed fixture: %v", err)
	}

	var buf bytes.Buffer
	err := runEvidenceShow(&buf, path)

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runEvidenceShow() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}

func writeEvidenceFixtureNamed(t *testing.T, dir, name string, events []tracer.Event, architectures []string) {
	t.Helper()
	data, err := evidence.ToJSON(events, architectures)
	if err != nil {
		t.Fatalf("evidence.ToJSON() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("writing evidence fixture: %v", err)
	}
}

func TestRunEvidenceList_Empty(t *testing.T) {
	dir := t.TempDir()

	var buf bytes.Buffer
	if err := runEvidenceList(&buf, dir); err != nil {
		t.Fatalf("runEvidenceList() error = %v", err)
	}
	if !strings.Contains(buf.String(), "No evidence files found") {
		t.Errorf("output missing empty-directory message:\n%s", buf.String())
	}
}

func TestRunEvidenceList_MultipleFilesSortedByName(t *testing.T) {
	dir := t.TempDir()
	writeEvidenceFixtureNamed(t, dir, "zebra.json", []tracer.Event{
		{Syscall: "openat", Path: "/a", Mode: "read", Timestamp: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)},
	}, nil)
	writeEvidenceFixtureNamed(t, dir, "apple.json", []tracer.Event{
		{Syscall: "openat", Path: "/b", Mode: "read"},
		{Syscall: "openat", Path: "/c", Mode: "write"},
	}, nil)

	var buf bytes.Buffer
	if err := runEvidenceList(&buf, dir); err != nil {
		t.Fatalf("runEvidenceList() error = %v", err)
	}
	out := buf.String()

	appleIdx := strings.Index(out, "apple.json")
	zebraIdx := strings.Index(out, "zebra.json")
	if appleIdx == -1 || zebraIdx == -1 || appleIdx > zebraIdx {
		t.Errorf("expected apple.json before zebra.json (sorted by name):\n%s", out)
	}
	if !strings.Contains(out, "apple.json") || !strings.Contains(out, "2 event(s)") {
		t.Errorf("output missing apple.json summary:\n%s", out)
	}
	if !strings.Contains(out, "zebra.json") || !strings.Contains(out, "1 event(s)") {
		t.Errorf("output missing zebra.json summary:\n%s", out)
	}
	if !strings.Contains(out, "Observed: ") && !strings.Contains(out, "observed 2026-08-06T12:00:00Z to 2026-08-06T12:00:00Z") {
		t.Errorf("output missing zebra.json's observation window:\n%s", out)
	}
}

func TestRunEvidenceList_SkipsNonEvidenceFiles(t *testing.T) {
	dir := t.TempDir()
	writeEvidenceFixtureNamed(t, dir, "events.json", []tracer.Event{
		{Syscall: "openat", Path: "/a", Mode: "read"},
	}, nil)
	if err := os.WriteFile(filepath.Join(dir, "candidate.json"), []byte(`{"apiVersion":"v1","rules":[]}`), 0o600); err != nil {
		t.Fatalf("writing non-evidence fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not json at all"), 0o600); err != nil {
		t.Fatalf("writing non-json fixture: %v", err)
	}

	var buf bytes.Buffer
	if err := runEvidenceList(&buf, dir); err != nil {
		t.Fatalf("runEvidenceList() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "events.json") {
		t.Errorf("output missing events.json:\n%s", out)
	}
	if strings.Contains(out, "candidate.json") || strings.Contains(out, "README.md") {
		t.Errorf("output should skip files that don't parse as evidence:\n%s", out)
	}
}

func TestRunEvidenceList_NonexistentDirectory(t *testing.T) {
	var buf bytes.Buffer
	err := runEvidenceList(&buf, "/nonexistent/evidence-dir")

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runEvidenceList() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("ExitCode() = %d, want 3 (usage error)", exitErr.ExitCode())
	}
}
