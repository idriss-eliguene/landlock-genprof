// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build linux

package main

import "testing"

// On Linux, local detection is the real thing: it must succeed and hand
// back a release string parseKernelVersion actually accepts, since every
// caller (doctor, abi, verify) feeds it straight into that parser. No
// uname mock — the point is that the real syscall path still works.
func TestDetectLocalKernelRelease_Linux(t *testing.T) {
	release, err := detectLocalKernelRelease()
	if err != nil {
		t.Fatalf("detectLocalKernelRelease() error = %v, want nil on Linux", err)
	}
	if release == "" {
		t.Fatal("detectLocalKernelRelease() = \"\", want a non-empty kernel release")
	}

	major, minor, err := parseKernelVersion(release)
	if err != nil {
		t.Fatalf("parseKernelVersion(%q) error = %v, want the detected release to parse", release, err)
	}
	if major <= 0 {
		t.Errorf("parseKernelVersion(%q) major = %d, want a positive Linux major version", release, major)
	}
	if minor < 0 {
		t.Errorf("parseKernelVersion(%q) minor = %d, want a non-negative minor version", release, minor)
	}
}
