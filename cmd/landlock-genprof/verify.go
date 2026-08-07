// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// verify is this project's first real assurance check, not just a
// prerequisite probe: given a synthesized landlock.Candidate (as
// exported by internal/exporter/landlockjson — not a PodLock
// LandlockProfile, whose schema has already lost LandlockRightTruncate
// by the time it exists, see that package's own doc comment) and a
// target kernel, it reports whether every right the candidate needs is
// actually available at that kernel's Landlock ABI level.
//
// Not yet wired into `trace` — there is no --candidate-out flag to
// produce this command's input from a real training run yet. That's
// deliberately a separate, later step (touching trace.go's larger,
// riskier surface); this command is complete and testable on its own
// in the meantime, the same way `abi check`/`abi list` shipped before
// anything consumed them end to end.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/landlockjson"
	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

type verifyOptions struct {
	candidateFile string
	kernel        string
}

func newVerifyCmd() *cobra.Command {
	var opts verifyOptions

	cmd := &cobra.Command{
		Use:   "verify --candidate-file <path>",
		Short: "Checks a synthesized Landlock candidate against a target kernel's ABI level",
		Long: "Checks every rule in a synthesized Landlock candidate (see " +
			"internal/exporter/landlockjson) against a target kernel's Landlock ABI " +
			"level — reports which rules need a right the target kernel doesn't " +
			"support, and at which ABI level that right actually exists. --kernel " +
			"defaults to the local host's, matching doctor/abi check." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof verify --candidate-file nginx-demo-candidate.json
  kubectl landlock-genprof verify --candidate-file nginx-demo-candidate.json --kernel 5.19`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd.OutOrStdout(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.candidateFile, "candidate-file", "", "Path to a candidate JSON file (see internal/exporter/landlockjson)")
	cmd.Flags().StringVar(&opts.kernel, "kernel", "", "Kernel version to verify against (e.g. 6.2); defaults to the local host's")
	return cmd
}

func runVerify(stdout io.Writer, opts verifyOptions) error {
	if opts.candidateFile == "" {
		return fmt.Errorf("--candidate-file is required")
	}

	data, err := os.ReadFile(opts.candidateFile)
	if err != nil {
		return fmt.Errorf("reading candidate file: %w", err)
	}
	candidate, err := landlockjson.FromJSON(data)
	if err != nil {
		return fmt.Errorf("parsing candidate file: %w", err)
	}

	release := opts.kernel
	if release == "" {
		release, err = detectLocalKernelRelease()
		if err != nil {
			return err
		}
	}
	major, minor, err := parseKernelVersion(release)
	if err != nil {
		return fmt.Errorf("parsing kernel version %q: %w", release, err)
	}

	abi, ok := landlock.ABIForKernel(major, minor)
	if !ok {
		fmt.Fprintf(stdout, "Kernel %s: Landlock is not supported at all (needs >= 5.13)\n", release)
		return &exitCodeError{code: 2}
	}
	fmt.Fprintf(stdout, "Kernel %s: Landlock ABI %d\n", release, abi)

	available := make(map[landlock.LandlockRight]bool)
	for _, right := range landlock.RightsAt(abi) {
		available[right] = true
	}

	incompatible := false
	for _, rule := range candidate.Rules {
		var missing []landlock.LandlockRight
		for _, right := range rule.Rights {
			if !available[right] {
				missing = append(missing, right)
			}
		}

		if len(missing) == 0 {
			fmt.Fprintf(stdout, "✓ %s: compatible\n", rule.Path)
			continue
		}

		incompatible = true
		neededABI, found := landlock.HighestABI(missing)
		if !found {
			fmt.Fprintf(stdout, "✗ %s: needs %v, unavailable at ABI %d\n", rule.Path, missing, abi)
			continue
		}
		minMajor, minMinor, hasVersion := landlock.MinKernelFor(neededABI)
		if hasVersion {
			fmt.Fprintf(stdout, "✗ %s: needs %v (ABI %d, kernel >= %d.%d) — unavailable at this kernel's ABI %d\n",
				rule.Path, missing, neededABI, minMajor, minMinor, abi)
		} else {
			fmt.Fprintf(stdout, "✗ %s: needs %v (ABI %d) — unavailable at this kernel's ABI %d\n",
				rule.Path, missing, neededABI, abi)
		}
	}

	if incompatible {
		return &exitCodeError{code: 2}
	}
	fmt.Fprintln(stdout, "All rules compatible with this kernel's Landlock ABI.")
	return nil
}
