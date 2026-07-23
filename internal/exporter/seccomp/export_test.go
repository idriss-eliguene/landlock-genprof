// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package seccomp

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	pkgseccomp "github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
)

// mockNginxSyscallProfile mirrors the shape internal/policy.Synthesize
// would produce for a typical nginx training run. Built directly as an IR
// fixture, not derived by calling Synthesize — same pattern as
// internal/exporter/networkpolicy's own tests.
func mockNginxSyscallProfile() profile.SyscallProfile {
	return profile.SyscallProfile{
		Accesses: []profile.SyscallAccess{
			{Name: "openat", Confidence: profile.ConfidenceHigh, SeenCount: 1},
			{Name: "epoll_wait", Confidence: profile.ConfidenceHigh, SeenCount: 1},
			{Name: "accept4", Confidence: profile.ConfidenceLow, SeenCount: 1},
		},
		Architectures: []string{"SCMP_ARCH_X86_64", "SCMP_ARCH_X86"},
	}
}

func TestToProfile_MockNginxSyscallProfile(t *testing.T) {
	result := ToProfile(mockNginxSyscallProfile())

	if result.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("DefaultAction = %q, want SCMP_ACT_ERRNO", result.DefaultAction)
	}
	if !reflect.DeepEqual(result.Architectures, []string{"SCMP_ARCH_X86_64", "SCMP_ARCH_X86"}) {
		t.Errorf("Architectures = %v, want passthrough of the IR's own value", result.Architectures)
	}
	if len(result.Syscalls) != 1 {
		t.Fatalf("len(Syscalls) = %d, want 1 (a single allow bucket)", len(result.Syscalls))
	}
	rule := result.Syscalls[0]
	if rule.Action != "SCMP_ACT_ALLOW" {
		t.Errorf("Action = %q, want SCMP_ACT_ALLOW", rule.Action)
	}
	// Sorted alphabetically, matching Synthesize's own deterministic
	// ordering convention for the other two domains.
	want := []string{"accept4", "epoll_wait", "openat"}
	if !reflect.DeepEqual(rule.Names, want) {
		t.Errorf("Names = %v, want %v (sorted)", rule.Names, want)
	}
}

// TestToProfile_EmptySyscallProfile checks that no observed syscalls
// produces no Syscalls rule at all — matching
// internal/exporter/networkpolicy.ToPolicy's own "don't assert from
// absence" precedent (an empty allow rule would still deny everything,
// same defaultAction either way, but a nil Syscalls list is clearer about
// "nothing was ever observed" than an empty non-nil rule list).
func TestToProfile_EmptySyscallProfile(t *testing.T) {
	result := ToProfile(profile.SyscallProfile{})

	if len(result.Syscalls) != 0 {
		t.Errorf("Syscalls = %+v, want empty", result.Syscalls)
	}
	if result.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("DefaultAction = %q, want SCMP_ACT_ERRNO even with nothing observed", result.DefaultAction)
	}
}

func TestToJSON_RoundTrips(t *testing.T) {
	result := ToProfile(mockNginxSyscallProfile())

	out, err := ToJSON(result)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	// Must stay plain, comment-free JSON: this file is loaded directly by
	// the kubelet/container runtime, never kubectl apply'd — see the
	// package doc.
	var roundTripped pkgseccomp.Profile
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(&roundTripped, result) {
		t.Errorf("round-tripped profile = %+v, want %+v", roundTripped, *result)
	}
}
