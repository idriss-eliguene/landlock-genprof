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
	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/sarif"
	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

// SARIF rule IDs for this pass's two finding categories — see
// internal/exporter/sarif's own doc comment on why these are generic
// Rule/Result values, not ABI-specific types.
const (
	ruleIDUnsupportedKernel = "landlock-unsupported-kernel"
	ruleIDABIIncompatible   = "landlock-abi-incompatible"
)

type verifyOptions struct {
	candidateFile string
	kernel        string
	output        string
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
			"defaults to the local host's, matching doctor/abi check. --output sarif " +
			"renders findings as a SARIF 2.1.0 log instead of text, for CI dashboards " +
			"(GitHub Code Scanning and similar) that already know how to annotate it — " +
			"the exit-code contract (0/2/3) is unchanged either way." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof verify --candidate-file nginx-demo-candidate.json
  kubectl landlock-genprof verify --candidate-file nginx-demo-candidate.json --kernel 5.19
  kubectl landlock-genprof verify --candidate-file nginx-demo-candidate.json --output sarif > verify.sarif`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd.OutOrStdout(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.candidateFile, "candidate-file", "", "Path to a candidate JSON file (see internal/exporter/landlockjson)")
	cmd.Flags().StringVar(&opts.kernel, "kernel", "", "Kernel version to verify against (e.g. 6.2); defaults to the local host's")
	cmd.Flags().StringVar(&opts.output, "output", "text", "Output format: text or sarif")
	return cmd
}

func runVerify(stdout io.Writer, opts verifyOptions) error {
	output := opts.output
	if output == "" {
		output = "text"
	}
	if output != "text" && output != "sarif" {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("unsupported --output %q (supported: text, sarif)", output)}
	}

	if opts.candidateFile == "" {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("--candidate-file is required")}
	}

	data, err := os.ReadFile(opts.candidateFile)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("reading candidate file: %w", err)}
	}
	candidate, err := landlockjson.FromJSON(data)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("parsing candidate file: %w", err)}
	}

	release := opts.kernel
	if release == "" {
		release, err = detectLocalKernelRelease()
		if err != nil {
			return &exitCodeError{code: 3, wrapped: err}
		}
	}
	major, minor, err := parseKernelVersion(release)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("parsing kernel version %q: %w", release, err)}
	}

	var sarifResults []sarif.Result
	incompatible := false

	abi, ok := landlock.ABIForKernel(major, minor)
	if !ok {
		incompatible = true
		msg := fmt.Sprintf("Kernel %s: Landlock is not supported at all (needs >= 5.13)", release)
		if output == "text" {
			fmt.Fprintln(stdout, msg)
		} else {
			sarifResults = append(sarifResults, sarif.Result{RuleID: ruleIDUnsupportedKernel, Path: "*", Message: msg})
		}
	} else {
		if output == "text" {
			fmt.Fprintf(stdout, "Kernel %s: Landlock ABI %d\n", release, abi)
		}

		available := make(map[landlock.LandlockRight]bool)
		for _, right := range landlock.RightsAt(abi) {
			available[right] = true
		}

		for _, rule := range candidate.Rules {
			var missing []landlock.LandlockRight
			for _, right := range rule.Rights {
				if !available[right] {
					missing = append(missing, right)
				}
			}

			if len(missing) == 0 {
				if output == "text" {
					fmt.Fprintf(stdout, "✓ %s: compatible\n", rule.Path)
				}
				continue
			}

			incompatible = true
			var msg string
			neededABI, found := landlock.HighestABI(missing)
			switch {
			case !found:
				msg = fmt.Sprintf("needs %v, unavailable at ABI %d", missing, abi)
			default:
				if minMajor, minMinor, hasVersion := landlock.MinKernelFor(neededABI); hasVersion {
					msg = fmt.Sprintf("needs %v (ABI %d, kernel >= %d.%d) — unavailable at this kernel's ABI %d",
						missing, neededABI, minMajor, minMinor, abi)
				} else {
					msg = fmt.Sprintf("needs %v (ABI %d) — unavailable at this kernel's ABI %d", missing, neededABI, abi)
				}
			}

			if output == "text" {
				fmt.Fprintf(stdout, "✗ %s: %s\n", rule.Path, msg)
			} else {
				sarifResults = append(sarifResults, sarif.Result{RuleID: ruleIDABIIncompatible, Path: rule.Path, Message: msg})
			}
		}
	}

	if output == "sarif" {
		if err := writeVerifySARIF(stdout, sarifResults); err != nil {
			return &exitCodeError{code: 3, wrapped: err}
		}
	} else if !incompatible {
		fmt.Fprintln(stdout, "All rules compatible with this kernel's Landlock ABI.")
	}

	if incompatible {
		return &exitCodeError{code: 2}
	}
	return nil
}

func writeVerifySARIF(stdout io.Writer, results []sarif.Result) error {
	data, err := sarif.ToJSON(
		sarif.Meta{
			ToolName:       "landlock-genprof",
			ToolVersion:    version,
			InformationURI: "https://github.com/idriss-eliguene/landlock-genprof",
		},
		[]sarif.Rule{
			{ID: ruleIDUnsupportedKernel, ShortDescription: "the target kernel does not support Landlock at all"},
			{ID: ruleIDABIIncompatible, ShortDescription: "a candidate rule needs a Landlock right unavailable at the target kernel's ABI level"},
		},
		results,
	)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, string(data))
	return nil
}
