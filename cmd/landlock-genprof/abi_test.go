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

func TestRunABIList_AllLevels(t *testing.T) {
	var buf bytes.Buffer
	if err := runABIList(&buf, 0); err != nil {
		t.Fatalf("runABIList(0) error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"execute", "refer", "truncate", "net_bind_tcp", "ioctl_dev", "scope_signal"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunABIList_FilteredByLevel(t *testing.T) {
	var buf bytes.Buffer
	if err := runABIList(&buf, 1); err != nil {
		t.Fatalf("runABIList(1) error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "execute") {
		t.Errorf("output should include ABI1's execute right:\n%s", out)
	}
	if strings.Contains(out, "refer") {
		t.Errorf("output should NOT include refer (ABI2) when filtered to ABI1:\n%s", out)
	}
}

func TestRunABICheck_KernelOverride_ABI3(t *testing.T) {
	var buf bytes.Buffer
	err := runABICheck(&buf, abiCheckOptions{kernel: "6.2.0"})
	if err != nil {
		t.Fatalf("runABICheck() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ABI 3") {
		t.Errorf("output missing expected ABI level:\n%s", out)
	}
	if !strings.Contains(out, "truncate") {
		t.Errorf("output missing a right introduced at ABI3:\n%s", out)
	}
	if strings.Contains(out, "net_bind_tcp") {
		t.Errorf("output should not include ABI4's net_bind_tcp at kernel 6.2:\n%s", out)
	}
}

func TestRunABICheck_KernelOverride_TooOldForLandlock(t *testing.T) {
	var buf bytes.Buffer
	err := runABICheck(&buf, abiCheckOptions{kernel: "5.10.0"})

	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runABICheck() error = %v, want a *exitCodeError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("ExitCode() = %d, want 2", exitErr.ExitCode())
	}
}
