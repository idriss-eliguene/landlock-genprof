// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// landlockFSMinMajor/Minor and landlockNetMinMajor/Minor are Landlock's
// own documented minimum kernel versions (landlock.io) — the same
// figures README.md's own kernel-prerequisites table and
// hack/check-kernel.sh already cite; doctor is this project's second,
// now Go-native, place these numbers live, not a third independent
// source of truth to keep in sync by hand.
const (
	landlockFSMinMajor, landlockFSMinMinor   = 5, 13
	landlockNetMinMajor, landlockNetMinMinor = 6, 4
)

// doctorOptions configures `doctor`. Kernel, if set, checks a kernel
// version this process isn't actually running on — e.g. a fleet's
// node-pool version — instead of the local host's; the bpffs check is
// skipped in that case, since it's a property of the machine running
// doctor, not of a kernel version string.
type doctorOptions struct {
	kernel string
}

func newDoctorCmd() *cobra.Command {
	var opts doctorOptions

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Checks this host's (or a given kernel version's) Landlock/eBPF prerequisites",
		Long: "Checks that a kernel supports Landlock (filesystem and network) and, for " +
			"the local host, that eBPF's bpffs is mounted — the same checks " +
			"hack/check-kernel.sh has always run, now built into the CLI itself " +
			"instead of a separate shell script a user has to already know exists. " +
			"--kernel lets you check a kernel you're not currently running on (a " +
			"fleet's node-pool version, for instance) without needing to run this on " +
			"that host directly." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof doctor
  kubectl landlock-genprof doctor --kernel 5.15.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.kernel, "kernel", "", "Check this kernel version instead of the current host's (e.g. 5.15.0)")
	return cmd
}

// exitCodeError carries a specific exit code so main() can propagate it
// — the exit-code contract docs/cli-design.md commits to (0 clean, 1
// non-blocking finding, 2 blocking failure, 3 usage error) started here,
// on the cheapest command (doctor), before it had to hold for anything
// CI-critical like verify/diff. wrapped is optional: nil for the common
// "the check itself ran fine but found something" case (doctor/verify/
// abi's own usage, a generic message is enough); set when the exit code
// comes with a real error to report (diff's usage-error case, code 3).
type exitCodeError struct {
	code    int
	wrapped error
}

func (e *exitCodeError) Error() string {
	if e.wrapped != nil {
		return e.wrapped.Error()
	}
	return "command reported a non-zero exit condition"
}
func (e *exitCodeError) ExitCode() int { return e.code }
func (e *exitCodeError) Unwrap() error { return e.wrapped }

func runDoctor(stdout io.Writer, opts doctorOptions) error {
	release := opts.kernel
	checkingLocalHost := release == ""
	if checkingLocalHost {
		var err error
		release, err = detectLocalKernelRelease()
		if err != nil {
			return err
		}
	}

	major, minor, err := parseKernelVersion(release)
	if err != nil {
		return fmt.Errorf("parsing kernel version %q: %w", release, err)
	}
	fmt.Fprintf(stdout, "Kernel: %s\n", release)

	blocking, warning := false, false

	if kernelAtLeast(major, minor, landlockFSMinMajor, landlockFSMinMinor) {
		fmt.Fprintf(stdout, "✓ Landlock filesystem supported (>= %d.%d)\n", landlockFSMinMajor, landlockFSMinMinor)
	} else {
		fmt.Fprintf(stdout, "✗ Landlock filesystem NOT supported (needs >= %d.%d)\n", landlockFSMinMajor, landlockFSMinMinor)
		blocking = true
	}

	if kernelAtLeast(major, minor, landlockNetMinMajor, landlockNetMinMinor) {
		fmt.Fprintf(stdout, "✓ Landlock network supported (>= %d.%d)\n", landlockNetMinMajor, landlockNetMinMinor)
	} else {
		fmt.Fprintf(stdout, "⚠ Landlock network NOT supported (needs >= %d.%d) — filesystem only\n", landlockNetMinMajor, landlockNetMinMinor)
		warning = true
	}

	if checkingLocalHost {
		if _, err := os.Stat("/sys/fs/bpf"); err != nil {
			fmt.Fprintln(stdout, "⚠ /sys/fs/bpf not found — check that bpffs is mounted")
			warning = true
		} else {
			fmt.Fprintln(stdout, "✓ bpffs mounted")
		}
	} else {
		fmt.Fprintln(stdout, "… bpffs check skipped — checking a kernel version, not the local host")
	}

	switch {
	case blocking:
		return &exitCodeError{code: 2}
	case warning:
		return &exitCodeError{code: 1}
	default:
		return nil
	}
}
