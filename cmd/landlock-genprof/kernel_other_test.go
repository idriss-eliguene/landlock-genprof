// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build !linux

package main

import (
	"bytes"
	"strings"
	"testing"
)

// Off Linux there is no local kernel to report, so detection must fail
// rather than hand back a release string for a kernel that has no
// Landlock at all.
func TestDetectLocalKernelRelease_NotSupportedOffLinux(t *testing.T) {
	release, err := detectLocalKernelRelease()
	if err == nil {
		t.Fatalf("detectLocalKernelRelease() = %q, nil; want an error off Linux", release)
	}
	if release != "" {
		t.Errorf("detectLocalKernelRelease() release = %q, want \"\"", release)
	}
	// The operator needs to be told the way forward, not just refused.
	if !strings.Contains(err.Error(), "--kernel") {
		t.Errorf("detectLocalKernelRelease() error = %q, want it to point at --kernel", err)
	}
}

// The regression this split exists to prevent: on macOS, unix.Uname
// happily returns the Darwin/XNU release (24.x), which compares as newer
// than every Landlock threshold — so doctor used to report Landlock as
// supported on a kernel that has none. Auto-detection must now fail
// loudly instead of printing any capability verdict.
func TestRunDoctor_LocalHost_ClaimsNoLandlockSupportOffLinux(t *testing.T) {
	var buf bytes.Buffer
	err := runDoctor(&buf, doctorOptions{})
	if err == nil {
		t.Fatal("runDoctor() error = nil, want an error when auto-detecting off Linux")
	}

	out := buf.String()
	for _, unwanted := range []string{"✓", "Landlock filesystem supported", "Landlock network supported"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("runDoctor() output contains %q off Linux, want no capability verdict:\n%s", unwanted, out)
		}
	}
}

// --kernel stays fully supported off Linux: parseKernelVersion and
// kernelAtLeast are platform-independent, so checking an explicit kernel
// from a macOS or Windows workstation must still work.
func TestRunDoctor_KernelOverride_WorksOffLinux(t *testing.T) {
	var buf bytes.Buffer
	err := runDoctor(&buf, doctorOptions{kernel: "6.8.0-45-generic"})
	if err != nil {
		t.Fatalf("runDoctor(--kernel 6.8.0-45-generic) error = %v, want nil", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✓ Landlock filesystem supported") {
		t.Errorf("runDoctor() output missing the filesystem verdict:\n%s", out)
	}
	if !strings.Contains(out, "✓ Landlock network supported") {
		t.Errorf("runDoctor() output missing the network verdict:\n%s", out)
	}
}

// Same contract for abi, the other command that falls back to local
// detection when --kernel is absent.
func TestRunABICheck_KernelOverride_WorksOffLinux(t *testing.T) {
	var buf bytes.Buffer
	if err := runABICheck(&buf, abiCheckOptions{kernel: "6.2.0"}); err != nil {
		t.Fatalf("runABICheck(--kernel 6.2.0) error = %v, want nil", err)
	}
	if buf.Len() == 0 {
		t.Error("runABICheck() produced no output")
	}
}
