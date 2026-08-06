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

	"github.com/idriss-eliguene/landlock-genprof/internal/evidence"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func writeEventsFixture(t *testing.T, events []tracer.Event, architectures []string) string {
	t.Helper()
	data, err := evidence.ToJSON(events, architectures)
	if err != nil {
		t.Fatalf("evidence.ToJSON() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "events.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing events fixture: %v", err)
	}
	return path
}

func TestRunSynthesize_ProducesPodLockProfile(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/etc/nginx/nginx.conf", Mode: "read"},
		{Syscall: "openat", Path: "/usr/sbin/nginx", Mode: "exec"},
	}
	eventsFile := writeEventsFixture(t, events, nil)

	outDir := t.TempDir()
	out := filepath.Join(outDir, "profile.yaml")

	var buf bytes.Buffer
	err := runSynthesize(&buf, synthesizeOptions{
		eventsFile: eventsFile,
		podName:    "nginx-demo",
		namespace:  "default",
		container:  "nginx",
		binary:     "/usr/sbin/nginx",
		out:        out,
	})
	if err != nil {
		t.Fatalf("runSynthesize() error = %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading generated profile: %v", err)
	}
	text := string(data)
	for _, want := range []string{"kind: LandlockProfile", "name: nginx-demo", "readOnly:", "readExec:"} {
		if !strings.Contains(text, want) {
			t.Errorf("generated profile missing %q:\n%s", want, text)
		}
	}
}

func TestRunSynthesize_CandidateOutIncludesTruncate(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/var/cache/nginx/proxy_temp", Mode: "write", Truncate: true},
	}
	eventsFile := writeEventsFixture(t, events, nil)

	outDir := t.TempDir()
	profileOut := filepath.Join(outDir, "profile.yaml")
	candidateOut := filepath.Join(outDir, "candidate.json")

	var buf bytes.Buffer
	err := runSynthesize(&buf, synthesizeOptions{
		eventsFile:   eventsFile,
		podName:      "nginx-demo",
		namespace:    "default",
		container:    "nginx",
		binary:       "/usr/sbin/nginx",
		out:          profileOut,
		candidateOut: candidateOut,
	})
	if err != nil {
		t.Fatalf("runSynthesize() error = %v", err)
	}

	data, err := os.ReadFile(candidateOut)
	if err != nil {
		t.Fatalf("reading candidate file: %v", err)
	}
	if !strings.Contains(string(data), "truncate") {
		t.Errorf("candidate JSON should preserve truncate (lost by the PodLock YAML above):\n%s", data)
	}
}

func TestRunSynthesize_NonexistentEventsFile(t *testing.T) {
	var buf bytes.Buffer
	err := runSynthesize(&buf, synthesizeOptions{
		eventsFile: "/nonexistent/events.json",
		podName:    "nginx-demo",
		container:  "nginx",
		binary:     "/usr/sbin/nginx",
	})
	if err == nil {
		t.Fatal("runSynthesize() error = nil, want an error for a nonexistent events file")
	}
}

func TestRunSynthesize_MalformedEventsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("writing malformed fixture: %v", err)
	}

	var buf bytes.Buffer
	err := runSynthesize(&buf, synthesizeOptions{
		eventsFile: path,
		podName:    "nginx-demo",
		container:  "nginx",
		binary:     "/usr/sbin/nginx",
	})
	if err == nil {
		t.Fatal("runSynthesize() error = nil, want an error for a malformed events file")
	}
}
