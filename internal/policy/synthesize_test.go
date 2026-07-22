// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package policy

import (
	"reflect"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func TestSynthesize_AggregatesByDirectory(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/usr/share/nginx/index.html", Mode: "read"},
		{Syscall: "openat", Path: "/usr/share/nginx/css/style.css", Mode: "read"},
		{Syscall: "openat", Path: "/tmp/nginx.pid", Mode: "write"},
	}

	rules, err := Synthesize(events)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	// No rule per individual file: the two files under /usr/share/nginx
	// (one of them in a css/ subdirectory) must merge into a single rule.
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2 (no per-file rule): %+v", len(rules), rules)
	}

	byPath := make(map[string]Rule, len(rules))
	for _, r := range rules {
		byPath[r.Path] = r
	}

	nginx, ok := byPath["/usr/share/nginx"]
	if !ok {
		t.Fatalf("expected a rule for /usr/share/nginx, got: %+v", rules)
	}
	if !reflect.DeepEqual(nginx.Access, []string{"readOnly"}) {
		t.Errorf("/usr/share/nginx Access = %v, want [readOnly]", nginx.Access)
	}
	if nginx.SeenCount != 2 {
		t.Errorf("/usr/share/nginx SeenCount = %d, want 2 (index.html + css/style.css)", nginx.SeenCount)
	}

	tmp, ok := byPath["/tmp"]
	if !ok {
		t.Fatalf("expected a rule for /tmp, got: %+v", rules)
	}
	if !reflect.DeepEqual(tmp.Access, []string{"readWrite"}) {
		t.Errorf("/tmp Access = %v, want [readWrite]", tmp.Access)
	}
}

func TestSynthesize_MockNginxEvents(t *testing.T) {
	rules, err := Synthesize(mockNginxEvents())
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	want := map[string][]string{
		"/usr/sbin":        {"readExec"},
		"/etc/nginx":       {"readOnly"},
		"/usr/share/nginx": {"readOnly"}, // html/index.html truncated to depth 3
		"/var/log/nginx":   {"readWrite"},
		"/tmp":             {"readWrite"},
	}

	// The connect event (no Path) must produce no rule: the current
	// PodLock format (pkg/podlock.BinaryProfile) doesn't represent
	// network rights.
	if len(rules) != len(want) {
		t.Fatalf("len(rules) = %d, want %d: %+v", len(rules), len(want), rules)
	}

	byPath := make(map[string]Rule, len(rules))
	for _, r := range rules {
		byPath[r.Path] = r
	}

	for path, wantAccess := range want {
		got, ok := byPath[path]
		if !ok {
			t.Errorf("missing rule for %s", path)
			continue
		}
		if !reflect.DeepEqual(got.Access, wantAccess) {
			t.Errorf("%s Access = %v, want %v", path, got.Access, wantAccess)
		}
	}
}

func TestSynthesize_EmptyInput(t *testing.T) {
	rules, err := Synthesize(nil)
	if err != nil {
		t.Fatalf("Synthesize(nil) error = %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("len(rules) = %d, want 0", len(rules))
	}
}

// TestSynthesize_DirectoryOpenIsNotItsOwnParent reproduces a real bug
// found on the first end-to-end run against a live cluster: `ls /etc`
// opens /etc itself (with O_DIRECTORY) to list it, not a file inside it.
// Treating that like a file access and taking filepath.Dir("/etc")
// produced a nonsensical `readOnly: [/]` rule — allowing read access to
// the entire filesystem. A directory that was opened directly must
// become a rule on itself, not on its parent.
func TestSynthesize_DirectoryOpenIsNotItsOwnParent(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/etc", Mode: "read", IsDir: true},
	}

	rules, err := Synthesize(events)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1: %+v", len(rules), rules)
	}
	if rules[0].Path != "/etc" {
		t.Errorf("Path = %q, want /etc (not its parent /)", rules[0].Path)
	}
}

// TestSynthesize_IgnoresRelativePaths reproduces the second bug found
// alongside the one above: some observed opens carry a path with no
// leading "/" (relative to the traced process's working directory, which
// we don't track). filepath.Dir() on a bare filename returns ".", which
// used to leak into a bogus "/." rule.
func TestSynthesize_IgnoresRelativePaths(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "nginx.conf", Mode: "read"},
	}

	rules, err := Synthesize(events)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("len(rules) = %d, want 0 (relative path should be ignored): %+v", len(rules), rules)
	}
}
