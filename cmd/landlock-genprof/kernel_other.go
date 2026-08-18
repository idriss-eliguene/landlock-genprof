// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build !linux

package main

import "fmt"

// detectLocalKernelRelease is not available on non-Linux platforms:
// Landlock is a Linux facility, so "the local kernel version" is only a
// meaningful answer to give about a Linux host. Reporting the local
// release here would not merely be useless, it would be wrong — the
// version would be fed straight into Landlock's Linux thresholds and,
// on macOS, the Darwin/XNU release (24.x) compares as newer than every
// one of them, making doctor claim Landlock support on a kernel that has
// none.
//
// Returning an error instead keeps the answer honest and leaves the
// operator the accurate route: doctor, abi and verify all accept an
// explicit --kernel, which is fully supported on every platform because
// parseKernelVersion and kernelAtLeast (kernel.go) are portable. See
// kernel_linux.go for the real implementation.
func detectLocalKernelRelease() (string, error) {
	return "", fmt.Errorf("detecting the local kernel version is only supported on Linux (Landlock is a Linux facility); pass --kernel <version> to check a specific kernel instead")
}
