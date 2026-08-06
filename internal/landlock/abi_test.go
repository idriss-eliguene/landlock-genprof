// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package landlock

import (
	"reflect"
	"testing"
)

func TestRightsAt_ABI1_OnlyBaseRights(t *testing.T) {
	rights := RightsAt(ABI1)
	if len(rights) != 13 {
		t.Fatalf("len(RightsAt(ABI1)) = %d, want 13 (the base filesystem rights only): %v", len(rights), rights)
	}
	for _, r := range rights {
		if abi, _ := ABIForRight(r); abi != ABI1 {
			t.Errorf("RightsAt(ABI1) included %q, which needs ABI %d", r, abi)
		}
	}
}

func TestRightsAt_IsCumulative(t *testing.T) {
	abi1 := RightsAt(ABI1)
	abi6 := RightsAt(ABI6)

	if len(abi6) <= len(abi1) {
		t.Fatalf("RightsAt(ABI6) should be a strict superset of RightsAt(ABI1): got %d vs %d", len(abi6), len(abi1))
	}

	abi6Set := make(map[LandlockRight]bool, len(abi6))
	for _, r := range abi6 {
		abi6Set[r] = true
	}
	for _, r := range abi1 {
		if !abi6Set[r] {
			t.Errorf("RightsAt(ABI6) is missing %q, present at ABI1 — must be cumulative", r)
		}
	}
}

func TestRightsAt_SpecificRightsAtExpectedLevel(t *testing.T) {
	tests := []struct {
		right LandlockRight
		abi   ABILevel
	}{
		{LandlockRightRefer, ABI2},
		{LandlockRightTruncate, ABI3},
		{LandlockRightNetBindTCP, ABI4},
		{LandlockRightNetConnectTCP, ABI4},
		{LandlockRightIoctlDev, ABI5},
		{LandlockRightScopeSignal, ABI6},
	}

	for _, tt := range tests {
		abi, ok := ABIForRight(tt.right)
		if !ok {
			t.Errorf("ABIForRight(%q) not found", tt.right)
			continue
		}
		if abi != tt.abi {
			t.Errorf("ABIForRight(%q) = %d, want %d", tt.right, abi, tt.abi)
		}

		// Below its introducing ABI, the right must be absent.
		below := RightsAt(tt.abi - 1)
		for _, r := range below {
			if r == tt.right {
				t.Errorf("RightsAt(%d) should not include %q, introduced at ABI %d", tt.abi-1, tt.right, tt.abi)
			}
		}
	}
}

func TestRightsAt_DeterministicOrder(t *testing.T) {
	first := RightsAt(ABI6)
	second := RightsAt(ABI6)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("RightsAt(ABI6) is non-deterministic across calls:\n%v\n%v", first, second)
	}
}

func TestMinKernelFor(t *testing.T) {
	tests := []struct {
		abi       ABILevel
		wantMajor int
		wantMinor int
	}{
		{ABI1, 5, 13},
		{ABI2, 5, 19},
		{ABI3, 6, 2},
		{ABI4, 6, 7},
		{ABI5, 6, 10},
		{ABI6, 6, 12},
		{ABI7, 6, 15},
	}
	for _, tt := range tests {
		major, minor, ok := MinKernelFor(tt.abi)
		if !ok {
			t.Errorf("MinKernelFor(ABI%d) not found", tt.abi)
			continue
		}
		if major != tt.wantMajor || minor != tt.wantMinor {
			t.Errorf("MinKernelFor(ABI%d) = %d.%d, want %d.%d", tt.abi, major, minor, tt.wantMajor, tt.wantMinor)
		}
	}
}

func TestMinKernelFor_UnknownABI(t *testing.T) {
	if _, _, ok := MinKernelFor(ABILevel(99)); ok {
		t.Error("MinKernelFor(99) should report not-found for an unknown ABI level")
	}
}

func TestHighestABI(t *testing.T) {
	tests := []struct {
		name      string
		rights    []LandlockRight
		wantABI   ABILevel
		wantFound bool
	}{
		{
			name:      "all ABI1",
			rights:    []LandlockRight{LandlockRightReadFile, LandlockRightWriteFile, LandlockRightExecute},
			wantABI:   ABI1,
			wantFound: true,
		},
		{
			name:      "mixed ABI1 and ABI3",
			rights:    []LandlockRight{LandlockRightReadFile, LandlockRightTruncate},
			wantABI:   ABI3,
			wantFound: true,
		},
		{
			name:      "empty",
			rights:    nil,
			wantFound: false,
		},
		{
			name:      "unknown right only",
			rights:    []LandlockRight{"not_a_real_right"},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := HighestABI(tt.rights)
			if found != tt.wantFound {
				t.Fatalf("HighestABI(%v) found = %v, want %v", tt.rights, found, tt.wantFound)
			}
			if found && got != tt.wantABI {
				t.Errorf("HighestABI(%v) = %d, want %d", tt.rights, got, tt.wantABI)
			}
		})
	}
}

func TestABIForKernel(t *testing.T) {
	tests := []struct {
		major, minor int
		want         ABILevel
		wantFound    bool
	}{
		{5, 12, 0, false}, // predates Landlock entirely
		{5, 13, ABI1, true},
		{5, 18, ABI1, true},
		{5, 19, ABI2, true},
		{6, 2, ABI3, true},
		{6, 7, ABI4, true},
		{6, 10, ABI5, true},
		{6, 12, ABI6, true},
		{6, 15, ABI7, true},
		{6, 20, ABI7, true}, // beyond the table's newest known ABI: reports the highest we know about
	}
	for _, tt := range tests {
		got, found := ABIForKernel(tt.major, tt.minor)
		if found != tt.wantFound {
			t.Errorf("ABIForKernel(%d.%d) found = %v, want %v", tt.major, tt.minor, found, tt.wantFound)
			continue
		}
		if found && got != tt.want {
			t.Errorf("ABIForKernel(%d.%d) = %d, want %d", tt.major, tt.minor, got, tt.want)
		}
	}
}
