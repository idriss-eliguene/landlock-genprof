// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package history

import (
	"reflect"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

func TestMerge_FirstRun(t *testing.T) {
	behavior := profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
			{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}},
		}},
		Network: profile.NetworkProfile{Accesses: []profile.NetworkAccess{
			{Port: 443, Direction: profile.DirectionEgress},
		}},
	}

	record := Merge(nil, "nginx", "/usr/sbin/nginx", behavior)

	if record.Container != "nginx" || record.Binary != "/usr/sbin/nginx" {
		t.Errorf("Container/Binary = %q/%q, want nginx//usr/sbin/nginx", record.Container, record.Binary)
	}
	if record.RunsRecorded != 1 {
		t.Errorf("RunsRecorded = %d, want 1", record.RunsRecorded)
	}
	if len(record.FilesystemAccesses) != 1 || record.FilesystemAccesses[0].SeenInRuns != 1 {
		t.Errorf("FilesystemAccesses = %+v, want one access with SeenInRuns=1", record.FilesystemAccesses)
	}
	if len(record.NetworkAccesses) != 1 || record.NetworkAccesses[0].SeenInRuns != 1 {
		t.Errorf("NetworkAccesses = %+v, want one access with SeenInRuns=1", record.NetworkAccesses)
	}
}

func TestMerge_SecondRun_SameAccessIncrementsSeenInRuns(t *testing.T) {
	behavior := profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
			{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}},
		}},
	}

	record := Merge(nil, "nginx", "/usr/sbin/nginx", behavior)
	record = Merge(record, "nginx", "/usr/sbin/nginx", behavior)

	if record.RunsRecorded != 2 {
		t.Errorf("RunsRecorded = %d, want 2", record.RunsRecorded)
	}
	if len(record.FilesystemAccesses) != 1 || record.FilesystemAccesses[0].SeenInRuns != 2 {
		t.Errorf("FilesystemAccesses = %+v, want one access with SeenInRuns=2", record.FilesystemAccesses)
	}
}

// TestMerge_SecondRun_UnseenAccessRatioDecays reproduces the drift
// property Merge's own doc comment claims: an access recorded on run 1
// but absent from run 2 keeps SeenInRuns=1 while RunsRecorded grows to
// 2 — its ratio drops without any special-casing.
func TestMerge_SecondRun_UnseenAccessRatioDecays(t *testing.T) {
	run1 := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
		{Path: "/var/cache/nginx/proxy", Permissions: []profile.FilePermission{profile.PermissionWrite}},
	}}}
	run2 := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
		{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}},
	}}}

	record := Merge(nil, "nginx", "/usr/sbin/nginx", run1)
	record = Merge(record, "nginx", "/usr/sbin/nginx", run2)

	if record.RunsRecorded != 2 {
		t.Fatalf("RunsRecorded = %d, want 2", record.RunsRecorded)
	}
	if len(record.FilesystemAccesses) != 2 {
		t.Fatalf("FilesystemAccesses = %+v, want 2 distinct paths", record.FilesystemAccesses)
	}

	byPath := make(map[string]FileAccessRecord, len(record.FilesystemAccesses))
	for _, a := range record.FilesystemAccesses {
		byPath[a.Path] = a
	}
	if byPath["/var/cache/nginx/proxy"].SeenInRuns != 1 {
		t.Errorf("/var/cache/nginx/proxy SeenInRuns = %d, want 1 (not observed in run 2)",
			byPath["/var/cache/nginx/proxy"].SeenInRuns)
	}
	if byPath["/etc/nginx"].SeenInRuns != 1 {
		t.Errorf("/etc/nginx SeenInRuns = %d, want 1 (first observed in run 2)", byPath["/etc/nginx"].SeenInRuns)
	}
}

func TestMerge_PermissionsUnionAcrossRuns(t *testing.T) {
	run1 := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
		{Path: "/opt/app/state.db", Permissions: []profile.FilePermission{profile.PermissionRead}},
	}}}
	run2 := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
		{Path: "/opt/app/state.db", Permissions: []profile.FilePermission{profile.PermissionWrite}},
	}}}

	record := Merge(nil, "app", "/opt/app/run", run1)
	record = Merge(record, "app", "/opt/app/run", run2)

	want := []profile.FilePermission{profile.PermissionRead, profile.PermissionWrite}
	if !reflect.DeepEqual(record.FilesystemAccesses[0].Permissions, want) {
		t.Errorf("Permissions = %v, want %v (union of both runs, fixed order)", record.FilesystemAccesses[0].Permissions, want)
	}
}

func TestApplyConfidence_HighWhenSeenEveryRun(t *testing.T) {
	record := &Record{
		RunsRecorded:       3,
		FilesystemAccesses: []FileAccessRecord{{Path: "/etc/nginx", SeenInRuns: 3}},
	}
	behavior := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
		{Path: "/etc/nginx"},
	}}}

	got := ApplyConfidence(record, behavior)
	if got.Filesystem.Accesses[0].Confidence != profile.ConfidenceHigh {
		t.Errorf("Confidence = %q, want high (3/3 runs)", got.Filesystem.Accesses[0].Confidence)
	}
}

func TestApplyConfidence_LowWhenSeenOnceOutOfFive(t *testing.T) {
	record := &Record{
		RunsRecorded:       5,
		FilesystemAccesses: []FileAccessRecord{{Path: "/var/cache/nginx/proxy", SeenInRuns: 1}},
	}
	behavior := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
		{Path: "/var/cache/nginx/proxy"},
	}}}

	got := ApplyConfidence(record, behavior)
	if got.Filesystem.Accesses[0].Confidence != profile.ConfidenceLow {
		t.Errorf("Confidence = %q, want low (1/5 runs) — the README's own example", got.Filesystem.Accesses[0].Confidence)
	}
}

func TestApplyConfidence_MediumAtHalf(t *testing.T) {
	record := &Record{
		RunsRecorded:       4,
		FilesystemAccesses: []FileAccessRecord{{Path: "/tmp", SeenInRuns: 2}},
	}
	behavior := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
		{Path: "/tmp"},
	}}}

	got := ApplyConfidence(record, behavior)
	if got.Filesystem.Accesses[0].Confidence != profile.ConfidenceMedium {
		t.Errorf("Confidence = %q, want medium (2/4 runs)", got.Filesystem.Accesses[0].Confidence)
	}
}

// TestMerge_SyscallAccesses mirrors TestMerge_SecondRun_UnseenAccessRatioDecays
// for the syscall domain: same fold-in/decay behavior, keyed by name
// instead of path.
func TestMerge_SyscallAccesses(t *testing.T) {
	run1 := profile.BehaviorProfile{Syscalls: profile.SyscallProfile{Accesses: []profile.SyscallAccess{
		{Name: "openat"}, {Name: "brk"},
	}}}
	run2 := profile.BehaviorProfile{Syscalls: profile.SyscallProfile{Accesses: []profile.SyscallAccess{
		{Name: "openat"},
	}}}

	record := Merge(nil, "nginx", "/usr/sbin/nginx", run1)
	record = Merge(record, "nginx", "/usr/sbin/nginx", run2)

	if record.RunsRecorded != 2 {
		t.Fatalf("RunsRecorded = %d, want 2", record.RunsRecorded)
	}
	if len(record.SyscallAccesses) != 2 {
		t.Fatalf("SyscallAccesses = %+v, want 2 distinct names", record.SyscallAccesses)
	}

	byName := make(map[string]SyscallAccessRecord, len(record.SyscallAccesses))
	for _, a := range record.SyscallAccesses {
		byName[a.Name] = a
	}
	if byName["openat"].SeenInRuns != 2 {
		t.Errorf("openat SeenInRuns = %d, want 2 (seen both runs)", byName["openat"].SeenInRuns)
	}
	if byName["brk"].SeenInRuns != 1 {
		t.Errorf("brk SeenInRuns = %d, want 1 (not observed in run 2 — ratio decays)", byName["brk"].SeenInRuns)
	}
}

// TestApplyConfidence_Syscalls mirrors TestApplyConfidence_HighWhenSeenEveryRun
// for the syscall domain.
func TestApplyConfidence_Syscalls(t *testing.T) {
	record := &Record{
		RunsRecorded:    3,
		SyscallAccesses: []SyscallAccessRecord{{Name: "openat", SeenInRuns: 3}},
	}
	behavior := profile.BehaviorProfile{Syscalls: profile.SyscallProfile{
		Accesses:      []profile.SyscallAccess{{Name: "openat"}},
		Architectures: []string{"SCMP_ARCH_X86_64"},
	}}

	got := ApplyConfidence(record, behavior)
	if got.Syscalls.Accesses[0].Confidence != profile.ConfidenceHigh {
		t.Errorf("Confidence = %q, want high (3/3 runs)", got.Syscalls.Accesses[0].Confidence)
	}
	if !reflect.DeepEqual(got.Syscalls.Architectures, []string{"SCMP_ARCH_X86_64"}) {
		t.Errorf("Architectures = %v, want passthrough of behavior's own value", got.Syscalls.Architectures)
	}
}

// TestMerge_CapabilityAccesses mirrors TestMerge_SyscallAccesses for the
// capabilities domain.
func TestMerge_CapabilityAccesses(t *testing.T) {
	run1 := profile.BehaviorProfile{Capabilities: profile.CapabilityProfile{Accesses: []profile.CapabilityAccess{
		{Name: "CAP_NET_BIND_SERVICE"}, {Name: "CAP_SYS_NICE"},
	}}}
	run2 := profile.BehaviorProfile{Capabilities: profile.CapabilityProfile{Accesses: []profile.CapabilityAccess{
		{Name: "CAP_NET_BIND_SERVICE"},
	}}}

	record := Merge(nil, "nginx", "/usr/sbin/nginx", run1)
	record = Merge(record, "nginx", "/usr/sbin/nginx", run2)

	if record.RunsRecorded != 2 {
		t.Fatalf("RunsRecorded = %d, want 2", record.RunsRecorded)
	}
	if len(record.CapabilityAccesses) != 2 {
		t.Fatalf("CapabilityAccesses = %+v, want 2 distinct names", record.CapabilityAccesses)
	}

	byName := make(map[string]CapabilityAccessRecord, len(record.CapabilityAccesses))
	for _, a := range record.CapabilityAccesses {
		byName[a.Name] = a
	}
	if byName["CAP_NET_BIND_SERVICE"].SeenInRuns != 2 {
		t.Errorf("CAP_NET_BIND_SERVICE SeenInRuns = %d, want 2 (seen both runs)", byName["CAP_NET_BIND_SERVICE"].SeenInRuns)
	}
	if byName["CAP_SYS_NICE"].SeenInRuns != 1 {
		t.Errorf("CAP_SYS_NICE SeenInRuns = %d, want 1 (not observed in run 2 — ratio decays)", byName["CAP_SYS_NICE"].SeenInRuns)
	}
}

// TestApplyConfidence_Capabilities mirrors TestApplyConfidence_Syscalls
// for the capabilities domain.
func TestApplyConfidence_Capabilities(t *testing.T) {
	record := &Record{
		RunsRecorded:       3,
		CapabilityAccesses: []CapabilityAccessRecord{{Name: "CAP_NET_BIND_SERVICE", SeenInRuns: 3}},
	}
	behavior := profile.BehaviorProfile{Capabilities: profile.CapabilityProfile{
		Accesses: []profile.CapabilityAccess{{Name: "CAP_NET_BIND_SERVICE"}},
	}}

	got := ApplyConfidence(record, behavior)
	if got.Capabilities.Accesses[0].Confidence != profile.ConfidenceHigh {
		t.Errorf("Confidence = %q, want high (3/3 runs)", got.Capabilities.Accesses[0].Confidence)
	}
}

func TestApplyConfidence_NilRecordReturnsBehaviorUnchanged(t *testing.T) {
	behavior := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{
		{Path: "/etc/nginx", Confidence: profile.ConfidenceLow},
	}}}

	got := ApplyConfidence(nil, behavior)
	if !reflect.DeepEqual(got, behavior) {
		t.Errorf("ApplyConfidence(nil, behavior) = %+v, want behavior unchanged", got)
	}
}
