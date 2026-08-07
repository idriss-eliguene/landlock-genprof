// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
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
	if err == nil {
		t.Fatal("runEvidenceShow() error = nil, want an error for a nonexistent file")
	}
}

func TestRunEvidenceShow_MalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("writing malformed fixture: %v", err)
	}

	var buf bytes.Buffer
	err := runEvidenceShow(&buf, path)
	if err == nil {
		t.Fatal("runEvidenceShow() error = nil, want an error for a malformed file")
	}
}
