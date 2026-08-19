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
)

// TestSynthesize_FullNginxRun_Golden pins Synthesize's complete output for
// mockNginxEvents() field-for-field, across all four domains at once —
// unlike the per-domain tests elsewhere in this package, which each check
// one slice of the result in isolation. This is Phase 0 of the planned
// internal/landlock kernel extraction (see the architecture review that
// led to it): a single, precise "did anything change" signal to run the
// filesystem-aggregation refactor (Phase 2) against, so a behavior drift
// introduced by moving code is caught here before it's ever visible to a
// user. Not a substitute for the existing targeted tests — those still
// explain *why* each behavior exists; this one only proves the whole
// output stays byte-for-byte identical while the internals move.
func TestSynthesize_FullNginxRun_Golden(t *testing.T) {
	behavior, err := Synthesize(mockNginxEvents(), []string{"SCMP_ARCH_X86_64"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	want := profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{
			Accesses: []profile.FileAccess{
				{Path: "/etc/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, Confidence: profile.ConfidenceLow, SeenCount: 1},
				{Path: "/tmp", Permissions: []profile.FilePermission{profile.PermissionWrite}, Confidence: profile.ConfidenceLow, SeenCount: 1},
				{Path: "/usr/sbin", Permissions: []profile.FilePermission{profile.PermissionExecute}, Confidence: profile.ConfidenceLow, SeenCount: 1},
				{Path: "/usr/share/nginx", Permissions: []profile.FilePermission{profile.PermissionRead}, Confidence: profile.ConfidenceLow, SeenCount: 1},
				{Path: "/var/log/nginx", Permissions: []profile.FilePermission{profile.PermissionWrite}, Confidence: profile.ConfidenceLow, SeenCount: 1},
			},
		},
		Network: profile.NetworkProfile{
			Accesses: []profile.NetworkAccess{
				{Port: 80, Direction: profile.DirectionEgress, Confidence: profile.ConfidenceLow, SeenCount: 1},
			},
		},
		Syscalls: profile.SyscallProfile{
			Accesses:      []profile.SyscallAccess{},
			Architectures: []string{"SCMP_ARCH_X86_64"},
		},
		Capabilities: profile.CapabilityProfile{
			Accesses: []profile.CapabilityAccess{},
		},
	}

	if !reflect.DeepEqual(behavior, want) {
		t.Errorf("Synthesize() = %#v,\nwant %#v", behavior, want)
	}
}
