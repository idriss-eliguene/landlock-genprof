// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// abi is deliberately standalone: it answers "what does kernel/ABI X
// support" from internal/landlock's verified ABI table
// (internal/landlock/abi.go), independent of any synthesized candidate.
// It is NOT yet wired into a `verify` command against real synthesized
// output — Synthesize only ever produces 5 of the ~19 rights this table
// knows about (LandlockRightReadFile/ReadDir/WriteFile/Execute, all
// ABI1, plus LandlockRightTruncate, ABI3 — read from openat(2)'s
// O_TRUNC flag, already flowing through the tracer pipeline unread until
// now, no new syscall hook needed). A per-candidate verification pass
// can therefore now say something a bare kernel-version check (doctor)
// cannot for a candidate needing truncate: "this needs kernel >= 6.2."
// See docs/landlock-kernel-extraction.md's "known gap" section: the
// remaining rights (REMOVE_*/MAKE_*/REFER) still need the tracer to
// observe syscalls it doesn't capture today (unlink/mkdir/rename/...),
// separate, larger work not bundled into this command.
package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

func newABICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "abi",
		Short: "Inspects Landlock's ABI-versioned right vocabulary",
		Long: "Inspects Landlock's ABI-versioned right vocabulary — which rights " +
			"exist at which ABI level, and which kernel version documents each " +
			"level. See `abi list`/`abi check`." + kubectlPrefixNote,
	}
	cmd.AddCommand(newABIListCmd())
	cmd.AddCommand(newABICheckCmd())
	return cmd
}

func newABIListCmd() *cobra.Command {
	var abiLevel int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists Landlock rights, optionally filtered to one ABI level and below",
		Long: "Lists every Landlock right this project's ABI table knows about, its " +
			"introducing ABI level, and that level's documented minimum kernel " +
			"version. --abi restricts the list to rights available at or below a " +
			"given level (e.g. --abi 3 for what a 6.2 kernel supports)." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof abi list
  kubectl landlock-genprof abi list --abi 3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runABIList(cmd.OutOrStdout(), abiLevel)
		},
	}
	cmd.Flags().IntVar(&abiLevel, "abi", 0, "Restrict to rights available at or below this ABI level (0 = all known levels)")
	return cmd
}

func runABIList(stdout io.Writer, abiLevel int) error {
	maxABI := landlock.ABI7
	if abiLevel > 0 {
		maxABI = landlock.ABILevel(abiLevel)
	}

	rights := landlock.RightsAt(maxABI)
	if len(rights) == 0 {
		fmt.Fprintln(stdout, "No known rights at or below that ABI level.")
		return nil
	}

	for _, right := range rights {
		abi, _ := landlock.ABIForRight(right)
		major, minor, ok := landlock.MinKernelFor(abi)
		if ok {
			fmt.Fprintf(stdout, "%-32s ABI %d  (kernel >= %d.%d)\n", right, abi, major, minor)
		} else {
			fmt.Fprintf(stdout, "%-32s ABI %d  (kernel version not confirmed)\n", right, abi)
		}
	}
	return nil
}

type abiCheckOptions struct {
	kernel string
}

func newABICheckCmd() *cobra.Command {
	var opts abiCheckOptions

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Reports the highest Landlock ABI level a kernel version supports",
		Long: "Reports the highest Landlock ABI level a kernel version supports, per " +
			"this project's documented kernel-version table — an approximation, not " +
			"the authoritative detection method (a live kernel's real ABI level is " +
			"only truly known via landlock_create_ruleset(NULL, 0, " +
			"LANDLOCK_CREATE_RULESET_VERSION), which correctly reports backported " +
			"support this table can't predict). Useful for planning against a kernel " +
			"you're not currently running on — a fleet's node-pool version, for " +
			"instance." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof abi check --kernel 6.2
  kubectl landlock-genprof abi check` + " # checks the local host",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runABICheck(cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.kernel, "kernel", "", "Kernel version to check (e.g. 6.2); defaults to the local host's")
	return cmd
}

func runABICheck(stdout io.Writer, opts abiCheckOptions) error {
	release := opts.kernel
	if release == "" {
		var err error
		release, err = detectLocalKernelRelease()
		if err != nil {
			return err
		}
	}

	major, minor, err := parseKernelVersion(release)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("parsing kernel version %q: %w", release, err)}
	}

	abi, ok := landlock.ABIForKernel(major, minor)
	if !ok {
		fmt.Fprintf(stdout, "Kernel %s: no Landlock ABI level supported (Landlock needs >= 5.13)\n", release)
		return &exitCodeError{code: 2}
	}

	fmt.Fprintf(stdout, "Kernel %s: Landlock ABI %d\n", release, abi)
	rights := landlock.RightsAt(abi)
	fmt.Fprintf(stdout, "%d right(s) available:\n", len(rights))
	for _, right := range rights {
		fmt.Fprintf(stdout, "  %s\n", right)
	}
	return nil
}
