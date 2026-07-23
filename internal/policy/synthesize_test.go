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

	behavior, err := Synthesize(events, nil)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	fsAccesses := behavior.Filesystem.Accesses

	// No access per individual file: the two files under /usr/share/nginx
	// (one of them in a css/ subdirectory) must merge into a single access.
	if len(fsAccesses) != 2 {
		t.Fatalf("len(Accesses) = %d, want 2 (no per-file access): %+v", len(fsAccesses), fsAccesses)
	}

	byPath := make(map[string]profile.FileAccess, len(fsAccesses))
	for _, a := range fsAccesses {
		byPath[a.Path] = a
	}

	nginx, ok := byPath["/usr/share/nginx"]
	if !ok {
		t.Fatalf("expected an access for /usr/share/nginx, got: %+v", fsAccesses)
	}
	if !reflect.DeepEqual(nginx.Permissions, []profile.FilePermission{profile.PermissionRead}) {
		t.Errorf("/usr/share/nginx Permissions = %v, want [read]", nginx.Permissions)
	}
	if nginx.SeenCount != 2 {
		t.Errorf("/usr/share/nginx SeenCount = %d, want 2 (index.html + css/style.css)", nginx.SeenCount)
	}

	tmp, ok := byPath["/tmp"]
	if !ok {
		t.Fatalf("expected an access for /tmp, got: %+v", fsAccesses)
	}
	if !reflect.DeepEqual(tmp.Permissions, []profile.FilePermission{profile.PermissionWrite}) {
		t.Errorf("/tmp Permissions = %v, want [write]", tmp.Permissions)
	}
}

func TestSynthesize_MockNginxEvents(t *testing.T) {
	behavior, err := Synthesize(mockNginxEvents(), nil)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	fsAccesses := behavior.Filesystem.Accesses

	want := map[string][]profile.FilePermission{
		"/usr/sbin":        {profile.PermissionExecute},
		"/etc/nginx":       {profile.PermissionRead},
		"/usr/share/nginx": {profile.PermissionRead}, // html/index.html truncated to depth 3
		"/var/log/nginx":   {profile.PermissionWrite},
		"/tmp":             {profile.PermissionWrite},
	}

	// The connect event (no Path) must produce no filesystem access: it
	// describes network activity, aggregated separately into
	// behavior.Network (see TestSynthesize_AggregatesNetworkByPortAndDirection).
	if len(fsAccesses) != len(want) {
		t.Fatalf("len(Accesses) = %d, want %d: %+v", len(fsAccesses), len(want), fsAccesses)
	}

	byPath := make(map[string]profile.FileAccess, len(fsAccesses))
	for _, a := range fsAccesses {
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

	// mockNginxEvents() has a single connect event on port 80 — must
	// surface as one egress NetworkAccess, not be silently dropped.
	if len(behavior.Network.Accesses) != 1 {
		t.Fatalf("len(Network.Accesses) = %d, want 1: %+v", len(behavior.Network.Accesses), behavior.Network.Accesses)
	}
	net := behavior.Network.Accesses[0]
	if net.Port != 80 || net.Direction != profile.DirectionEgress {
		t.Errorf("Network.Accesses[0] = %+v, want {Port: 80, Direction: egress}", net)
	}
}

func TestSynthesize_EmptyInput(t *testing.T) {
	behavior, err := Synthesize(nil, nil)
	if err != nil {
		t.Fatalf("Synthesize(nil) error = %v", err)
	}
	if len(behavior.Filesystem.Accesses) != 0 {
		t.Errorf("len(Filesystem.Accesses) = %d, want 0", len(behavior.Filesystem.Accesses))
	}
	if len(behavior.Network.Accesses) != 0 {
		t.Errorf("len(Network.Accesses) = %d, want 0", len(behavior.Network.Accesses))
	}
	if len(behavior.Syscalls.Accesses) != 0 {
		t.Errorf("len(Syscalls.Accesses) = %d, want 0", len(behavior.Syscalls.Accesses))
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

	behavior, err := Synthesize(events, nil)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	fsAccesses := behavior.Filesystem.Accesses

	if len(fsAccesses) != 1 {
		t.Fatalf("len(Accesses) = %d, want 1: %+v", len(fsAccesses), fsAccesses)
	}
	if fsAccesses[0].Path != "/etc" {
		t.Errorf("Path = %q, want /etc (not its parent /)", fsAccesses[0].Path)
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

	behavior, err := Synthesize(events, nil)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(behavior.Filesystem.Accesses) != 0 {
		t.Errorf("len(Accesses) = %d, want 0 (relative path should be ignored): %+v", len(behavior.Filesystem.Accesses), behavior.Filesystem.Accesses)
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

	behavior, err := Synthesize(events, nil)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	fsAccesses := behavior.Filesystem.Accesses
	if len(fsAccesses) != 1 {
		t.Fatalf("len(Accesses) = %d, want 1: %+v", len(fsAccesses), fsAccesses)
	}
	want := []profile.FilePermission{profile.PermissionWrite, profile.PermissionExecute}
	if !reflect.DeepEqual(fsAccesses[0].Permissions, want) {
		t.Errorf("Permissions = %v, want %v", fsAccesses[0].Permissions, want)
	}
}

// TestSynthesize_AggregatesNetworkByPortAndDirection mirrors
// TestSynthesize_AggregatesByDirectory for the network half of the IR:
// connect (egress) and bind (ingress) events aggregate by (port,
// direction), counting SeenCount and deriving Confidence the same way
// filesystem accesses do (see confidenceFor).
func TestSynthesize_AggregatesNetworkByPortAndDirection(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "connect", Port: 443, Mode: "egress"},
		{Syscall: "connect", Port: 443, Mode: "egress"},
		{Syscall: "connect", Port: 443, Mode: "egress"},
		{Syscall: "bind", Port: 8080, Mode: "ingress"},
		{Syscall: "connect", Port: 0, Mode: "egress"}, // no real port: must be skipped
	}

	behavior, err := Synthesize(events, nil)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	netAccesses := behavior.Network.Accesses

	if len(netAccesses) != 2 {
		t.Fatalf("len(Network.Accesses) = %d, want 2: %+v", len(netAccesses), netAccesses)
	}

	byKey := make(map[string]profile.NetworkAccess, len(netAccesses))
	for _, a := range netAccesses {
		byKey[string(a.Direction)] = a
	}

	egress, ok := byKey["egress"]
	if !ok {
		t.Fatalf("expected an egress access on port 443, got: %+v", netAccesses)
	}
	if egress.Port != 443 || egress.SeenCount != 3 || egress.Confidence != profile.ConfidenceHigh {
		t.Errorf("egress access = %+v, want {Port: 443, SeenCount: 3, Confidence: high}", egress)
	}

	ingress, ok := byKey["ingress"]
	if !ok {
		t.Fatalf("expected an ingress access on port 8080, got: %+v", netAccesses)
	}
	if ingress.Port != 8080 || ingress.SeenCount != 1 || ingress.Confidence != profile.ConfidenceLow {
		t.Errorf("ingress access = %+v, want {Port: 8080, SeenCount: 1, Confidence: low}", ingress)
	}

	// Filesystem must stay empty: none of these events carry a Path.
	if len(behavior.Filesystem.Accesses) != 0 {
		t.Errorf("len(Filesystem.Accesses) = %d, want 0: %+v", len(behavior.Filesystem.Accesses), behavior.Filesystem.Accesses)
	}
}

// TestSynthesize_SkipsEphemeralBindPorts reproduces a real false positive
// found on the live cluster: tracing a plain outbound `nc <ip> <port>`
// (no listener ever started) still produced a `bind` event on a
// kernel-assigned throwaway local port (busybox's nc binds explicitly
// before connect(), like the kernel's own implicit ephemeral-port
// assignment would). bind(2) can't be told apart from a real listen()
// prep at the syscall level, so ports >= ephemeralPortStart are dropped
// as a heuristic — see ephemeralPortStart's own comment.
func TestSynthesize_SkipsEphemeralBindPorts(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "bind", Port: 33847, Mode: "ingress"}, // ephemeral: dropped
		{Syscall: "bind", Port: 8080, Mode: "ingress"},  // below threshold: kept
	}

	behavior, err := Synthesize(events, nil)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	netAccesses := behavior.Network.Accesses

	if len(netAccesses) != 1 {
		t.Fatalf("len(Network.Accesses) = %d, want 1: %+v", len(netAccesses), netAccesses)
	}
	if netAccesses[0].Port != 8080 {
		t.Errorf("Network.Accesses[0].Port = %d, want 8080 (33847 should have been dropped as ephemeral)", netAccesses[0].Port)
	}
}

// TestSynthesize_AggregatesSyscalls mirrors
// TestSynthesize_AggregatesNetworkByPortAndDirection for the syscall half
// of the IR: events with Mode "syscall" (from internal/tracer's
// advise_seccomp integration) become one sorted, deduplicated
// SyscallAccess per name, and architectures passes straight through.
func TestSynthesize_AggregatesSyscalls(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "openat", Mode: "syscall"},
		{Syscall: "epoll_wait", Mode: "syscall"},
		{Syscall: "openat", Path: "/etc/nginx/nginx.conf", Mode: "read"}, // filesystem event, must not be counted as a syscall access
	}

	behavior, err := Synthesize(events, []string{"SCMP_ARCH_X86_64"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	syscalls := behavior.Syscalls.Accesses

	if len(syscalls) != 2 {
		t.Fatalf("len(Syscalls.Accesses) = %d, want 2: %+v", len(syscalls), syscalls)
	}
	// Sorted alphabetically, matching the deterministic-output convention
	// filesystem/network accesses already follow.
	if syscalls[0].Name != "epoll_wait" || syscalls[1].Name != "openat" {
		t.Errorf("Syscalls.Accesses names = [%s, %s], want [epoll_wait, openat] (sorted)", syscalls[0].Name, syscalls[1].Name)
	}

	if want := []string{"SCMP_ARCH_X86_64"}; !reflect.DeepEqual(behavior.Syscalls.Architectures, want) {
		t.Errorf("Syscalls.Architectures = %v, want %v", behavior.Syscalls.Architectures, want)
	}

	// The plain filesystem openat event must still produce its own
	// filesystem access, untouched by the syscall aggregation above.
	if len(behavior.Filesystem.Accesses) != 1 {
		t.Errorf("len(Filesystem.Accesses) = %d, want 1: %+v", len(behavior.Filesystem.Accesses), behavior.Filesystem.Accesses)
	}
}

// TestSynthesize_SyscallConfidenceAlwaysLowWithinASingleRun documents a
// deliberate difference from the filesystem/network domains: advise_seccomp
// reports one deduplicated set of syscalls per run, not one event per
// occurrence, so SeenCount can never exceed 1 within a single run — and
// confidenceFor(1) is always Low. Only internal/history's cross-run
// accumulation can raise it. This is intentional (see Synthesize's own
// doc comment), not a bug to fix here.
func TestSynthesize_SyscallConfidenceAlwaysLowWithinASingleRun(t *testing.T) {
	events := []tracer.Event{
		{Syscall: "brk", Mode: "syscall"},
	}

	behavior, err := Synthesize(events, nil)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(behavior.Syscalls.Accesses) != 1 {
		t.Fatalf("len(Syscalls.Accesses) = %d, want 1: %+v", len(behavior.Syscalls.Accesses), behavior.Syscalls.Accesses)
	}
	access := behavior.Syscalls.Accesses[0]
	if access.SeenCount != 1 || access.Confidence != profile.ConfidenceLow {
		t.Errorf("access = %+v, want {SeenCount: 1, Confidence: low}", access)
	}
}
