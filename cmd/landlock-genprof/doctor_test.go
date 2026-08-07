// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestParseKernelVersion(t *testing.T) {
	tests := []struct {
		release   string
		wantMajor int
		wantMinor int
		wantErr   bool
	}{
		{release: "6.8.0-45-generic", wantMajor: 6, wantMinor: 8},
		{release: "5.13", wantMajor: 5, wantMinor: 13},
		{release: "5.15.0", wantMajor: 5, wantMinor: 15},
		{release: "not-a-version", wantErr: true},
		{release: "6", wantErr: true},
	}

	for _, tt := range tests {
		major, minor, err := parseKernelVersion(tt.release)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseKernelVersion(%q) error = nil, want error", tt.release)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseKernelVersion(%q) error = %v, want nil", tt.release, err)
			continue
		}
		if major != tt.wantMajor || minor != tt.wantMinor {
			t.Errorf("parseKernelVersion(%q) = (%d, %d), want (%d, %d)",
				tt.release, major, minor, tt.wantMajor, tt.wantMinor)
		}
	}
}

func TestKernelAtLeast(t *testing.T) {
	tests := []struct {
		major, minor, minMajor, minMinor int
		want                             bool
	}{
		{major: 5, minor: 13, minMajor: 5, minMinor: 13, want: true},  // exact match
		{major: 5, minor: 12, minMajor: 5, minMinor: 13, want: false}, // minor below
		{major: 4, minor: 20, minMajor: 5, minMinor: 13, want: false}, // major below
		{major: 6, minor: 0, minMajor: 5, minMinor: 13, want: true},   // major above, minor irrelevant
	}

	for _, tt := range tests {
		got := kernelAtLeast(tt.major, tt.minor, tt.minMajor, tt.minMinor)
		if got != tt.want {
			t.Errorf("kernelAtLeast(%d.%d, min %d.%d) = %v, want %v",
				tt.major, tt.minor, tt.minMajor, tt.minMinor, got, tt.want)
		}
	}
}

func TestRunDoctor_KernelOverride_TooOldForLandlockFS(t *testing.T) {
	var buf bytes.Buffer
	err := runDoctor(&buf, doctorOptions{kernel: "5.10.0"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runDoctor() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("ExitCode() = %d, want 2 (blocking: Landlock FS unsupported)", exitErr.ExitCode())
	}
	if !strings.Contains(buf.String(), "Landlock filesystem NOT supported") {
		t.Errorf("output missing filesystem-unsupported line:\n%s", buf.String())
	}
}

func TestRunDoctor_KernelOverride_FSOnlyNoNetwork(t *testing.T) {
	var buf bytes.Buffer
	err := runDoctor(&buf, doctorOptions{kernel: "5.15.0"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runDoctor() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1 (warning: network unsupported, FS fine)", exitErr.ExitCode())
	}
	out := buf.String()
	if !strings.Contains(out, "Landlock filesystem supported") {
		t.Errorf("output missing filesystem-supported line:\n%s", out)
	}
	if !strings.Contains(out, "Landlock network NOT supported") {
		t.Errorf("output missing network-unsupported line:\n%s", out)
	}
}

func TestRunDoctor_KernelOverride_FullySupported(t *testing.T) {
	var buf bytes.Buffer
	err := runDoctor(&buf, doctorOptions{kernel: "6.8.0"})
	if err != nil {
		t.Fatalf("runDoctor() error = %v, want nil (fully supported kernel)", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Landlock filesystem supported") || !strings.Contains(out, "Landlock network supported") {
		t.Errorf("output missing expected support lines:\n%s", out)
	}
	if !strings.Contains(out, "bpffs check skipped") {
		t.Errorf("output should skip the local-host-only bpffs check when --kernel is set:\n%s", out)
	}
}

func TestRunDoctor_KernelOverride_InvalidVersion(t *testing.T) {
	var buf bytes.Buffer
	err := runDoctor(&buf, doctorOptions{kernel: "not-a-version"})
	if err == nil {
		t.Fatal("runDoctor() error = nil, want a parse error")
	}
	var exitErr *exitCodeError
	if errors.As(err, &exitErr) {
		t.Errorf("runDoctor() with an unparseable --kernel should be a plain usage error, not an exitCodeError")
	}
}
