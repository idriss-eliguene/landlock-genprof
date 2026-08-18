// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build linux

package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// detectLocalKernelRelease reads the running host's kernel release string
// (uname -r) — shared by doctor, abi and verify, so there's exactly one
// place that calls unix.Uname, not one per command.
//
// This is the real implementation; see kernel_other.go for the non-Linux
// one. golang.org/x/sys/unix is imported here and nowhere else in this
// package, which is what keeps the CLI cross-compiling for the whole
// release matrix (see .goreleaser.yaml).
func detectLocalKernelRelease() (string, error) {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return "", fmt.Errorf("reading local kernel version: %w", err)
	}
	return unix.ByteSliceToString(uts.Release[:]), nil
}
