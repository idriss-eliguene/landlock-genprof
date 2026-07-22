// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package policy

import (
	"reflect"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func TestSynthesize_AggregatesByDirectory(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/usr/share/nginx/index.html", Mode: "read"},
		{Syscall: "openat", Path: "/usr/share/nginx/css/style.css", Mode: "read"},
		{Syscall: "openat", Path: "/tmp/nginx.pid", Mode: "write"},
	}

	fsProfile, err := Synthesize(events)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	// No access per individual file: the two files under /usr/share/nginx
	// (one of them in a css/ subdirectory) must merge into a single access.
	if len(fsProfile.Accesses) != 2 {
		t.Fatalf("len(Accesses) = %d, want 2 (no per-file access): %+v", len(fsProfile.Accesses), fsProfile.Accesses)
	}

	byPath := make(map[string]profile.FileAccess, len(fsProfile.Accesses))
	for _, a := range fsProfile.Accesses {
		byPath[a.Path] = a
	}

	nginx, ok := byPath["/usr/share/nginx"]
	if !ok {
		t.Fatalf("expected an access for /usr/share/nginx, got: %+v", fsProfile.Accesses)
	}
	if !reflect.DeepEqual(nginx.Permissions, []profile.FilePermission{profile.PermissionRead}) {
		t.Errorf("/usr/share/nginx Permissions = %v, want [read]", nginx.Permissions)
	}
	if nginx.SeenCount != 2 {
		t.Errorf("/usr/share/nginx SeenCount = %d, want 2 (index.html + css/style.css)", nginx.SeenCount)
	}

	tmp, ok := byPath["/tmp"]
	if !ok {
		t.Fatalf("expected an access for /tmp, got: %+v", fsProfile.Accesses)
	}
	if !reflect.DeepEqual(tmp.Permissions, []profile.FilePermission{profile.PermissionWrite}) {
		t.Errorf("/tmp Permissions = %v, want [write]", tmp.Permissions)
	}
}

func TestSynthesize_MockNginxEvents(t *testing.T) {
	fsProfile, err := Synthesize(mockNginxEvents())
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	want := map[string][]profile.FilePermission{
		"/usr/sbin":        {profile.PermissionExecute},
		"/etc/nginx":       {profile.PermissionRead},
		"/usr/share/nginx": {profile.PermissionRead}, // html/index.html truncated to depth 3
		"/var/log/nginx":   {profile.PermissionWrite},
		"/tmp":             {profile.PermissionWrite},
	}

	// The connect event (no Path) must produce no access: it describes
	// network activity, which has nothing to do with the filesystem IR.
	if len(fsProfile.Accesses) != len(want) {
		t.Fatalf("len(Accesses) = %d, want %d: %+v", len(fsProfile.Accesses), len(want), fsProfile.Accesses)
	}

	byPath := make(map[string]profile.FileAccess, len(fsProfile.Accesses))
	for _, a := range fsProfile.Accesses {
		byPath[a.Path] = a
	}

	for path, wantPerms := range want {
		got, ok := byPath[path]
		if !ok {
			t.Errorf("missing access for %s", path)
			continue
		}
		if !reflect.DeepEqual(got.Permissions, wantPerms) {
			t.Errorf("%s Permissions = %v, want %v", path, got.Permissions, wantPerms)
		}
	}
}

func TestSynthesize_EmptyInput(t *testing.T) {
	fsProfile, err := Synthesize(nil)
	if err != nil {
		t.Fatalf("Synthesize(nil) error = %v", err)
	}
	if len(fsProfile.Accesses) != 0 {
		t.Errorf("len(Accesses) = %d, want 0", len(fsProfile.Accesses))
	}
}

// TestSynthesize_DirectoryOpenIsNotItsOwnParent reproduces a real bug
// found on the first end-to-end run against a live cluster: `ls /etc`
// opens /etc itself (with O_DIRECTORY) to list it, not a file inside it.
// Treating that like a file access and taking filepath.Dir("/etc")
// produced a nonsensical `readOnly: [/]` rule — allowing read access to
// the entire filesystem. A directory that was opened directly must
// become an access on itself, not on its parent.
func TestSynthesize_DirectoryOpenIsNotItsOwnParent(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/etc", Mode: "read", IsDir: true},
	}

	fsProfile, err := Synthesize(events)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	if len(fsProfile.Accesses) != 1 {
		t.Fatalf("len(Accesses) = %d, want 1: %+v", len(fsProfile.Accesses), fsProfile.Accesses)
	}
	if fsProfile.Accesses[0].Path != "/etc" {
		t.Errorf("Path = %q, want /etc (not its parent /)", fsProfile.Accesses[0].Path)
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

	fsProfile, err := Synthesize(events)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(fsProfile.Accesses) != 0 {
		t.Errorf("len(Accesses) = %d, want 0 (relative path should be ignored): %+v", len(fsProfile.Accesses), fsProfile.Accesses)
	}
}

// TestSynthesize_ExecAndWriteBothInPermissionSet checks that a directory
// both executed and written to records BOTH permissions in the IR's set,
// rather than collapsing them into a single joint category. That
// collapsing (PodLock's "readWriteExec", a fourth distinct category, not
// "readExec"+"readWrite" reported separately) is now the exporter's job —
// see internal/exporter/podlock's own tests for that invariant.
func TestSynthesize_ExecAndWriteBothInPermissionSet(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Path: "/opt/app/run", Mode: "exec"},
		{Syscall: "openat", Path: "/opt/app/state.db", Mode: "write"},
	}

	fsProfile, err := Synthesize(events)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(fsProfile.Accesses) != 1 {
		t.Fatalf("len(Accesses) = %d, want 1: %+v", len(fsProfile.Accesses), fsProfile.Accesses)
	}
	want := []profile.FilePermission{profile.PermissionWrite, profile.PermissionExecute}
	if !reflect.DeepEqual(fsProfile.Accesses[0].Permissions, want) {
		t.Errorf("Permissions = %v, want %v", fsProfile.Accesses[0].Permissions, want)
	}
}
