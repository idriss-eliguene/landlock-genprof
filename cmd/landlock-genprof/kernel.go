// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// detectLocalKernelRelease reads the running host's kernel release string
// (uname -r) — shared by doctor and abi, so there's exactly one place
// that calls unix.Uname, not one per command.
func detectLocalKernelRelease() (string, error) {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return "", fmt.Errorf("reading local kernel version: %w", err)
	}
	return unix.ByteSliceToString(uts.Release[:]), nil
}

// parseKernelVersion extracts the major.minor prefix from a uname -r
// style string (e.g. "6.8.0-45-generic" -> 6, 8) — mirrors
// hack/check-kernel.sh's own `cut -d. -f1`/`cut -d. -f2` parsing, in Go.
func parseKernelVersion(release string) (major, minor int, err error) {
	fields := strings.SplitN(release, ".", 3)
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("expected a major.minor[.patch] version string")
	}

	major, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parsing major version: %w", err)
	}

	// minor's field can carry a trailing non-digit run once a real
	// uname -r string is split only twice (e.g. "8.0-45-generic" as
	// fields[1] here, since SplitN(..., 3) stops after the second dot) —
	// only the leading digits are the minor version itself.
	minorField := fields[1]
	digits := 0
	for digits < len(minorField) && minorField[digits] >= '0' && minorField[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, 0, fmt.Errorf("parsing minor version")
	}
	minor, err = strconv.Atoi(minorField[:digits])
	if err != nil {
		return 0, 0, fmt.Errorf("parsing minor version: %w", err)
	}
	return major, minor, nil
}

func kernelAtLeast(major, minor, minMajor, minMinor int) bool {
	if major != minMajor {
		return major > minMajor
	}
	return minor >= minMinor
}
