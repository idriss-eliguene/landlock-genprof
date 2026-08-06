// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// explain is the fifth and last of the five commands that define this
// project's identity (see docs/cli-design.md — trace/synthesize/verify/
// explain/approve, deliberately not "generate"): the answer to "why does
// this rule exist and should I trust it," not just "is it valid" (verify)
// or "here it is" (export). Reuses landlockjson/landlock directly — no
// new data added, only a human-readable view of what a Candidate already
// carries (Rights, their ABI level, Confidence, SeenCount, Evidence).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/landlockjson"
	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

type explainOptions struct {
	candidateFile string
	path          string
}

func newExplainCmd() *cobra.Command {
	var opts explainOptions

	cmd := &cobra.Command{
		Use:   "explain --candidate-file <path>",
		Short: "Explains why a synthesized candidate's rules exist",
		Long: "Explains why each rule in a synthesized candidate (see " +
			"internal/exporter/landlockjson) exists: which rights it carries, " +
			"the ABI level and minimum kernel version each right actually needs, " +
			"how confident the synthesis is, and how many observations support " +
			"it. --path restricts this to a single rule." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof explain --candidate-file nginx-demo-candidate.json
  kubectl landlock-genprof explain --candidate-file nginx-demo-candidate.json --path /etc/nginx`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExplain(cmd.OutOrStdout(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.candidateFile, "candidate-file", "", "Path to a candidate JSON file (see internal/exporter/landlockjson) (required)")
	cmd.Flags().StringVar(&opts.path, "path", "", "Explain only the rule for this path (default: every rule)")
	return cmd
}

func runExplain(stdout io.Writer, opts explainOptions) error {
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

	rules := candidate.Rules
	if opts.path != "" {
		rules = nil
		for _, r := range candidate.Rules {
			if r.Path == opts.path {
				rules = append(rules, r)
				break
			}
		}
		if len(rules) == 0 {
			return fmt.Errorf("no rule for path %q in %s", opts.path, opts.candidateFile)
		}
	}

	if len(rules) == 0 {
		fmt.Fprintln(stdout, "No rules in this candidate.")
		return nil
	}

	for i, rule := range rules {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		explainRule(stdout, rule)
	}
	return nil
}

func explainRule(stdout io.Writer, rule landlock.Rule) {
	fmt.Fprintf(stdout, "%s\n", rule.Path)
	fmt.Fprintf(stdout, "  Confidence: %s (seen %d time(s))\n", rule.Confidence, rule.SeenCount)

	fmt.Fprintln(stdout, "  Rights:")
	for _, right := range rule.Rights {
		abi, ok := landlock.ABIForRight(right)
		if !ok {
			fmt.Fprintf(stdout, "    - %s (unknown ABI level)\n", right)
			continue
		}
		major, minor, hasVersion := landlock.MinKernelFor(abi)
		if hasVersion {
			fmt.Fprintf(stdout, "    - %-16s ABI %d (kernel >= %d.%d)\n", right, abi, major, minor)
		} else {
			fmt.Fprintf(stdout, "    - %-16s ABI %d\n", right, abi)
		}
	}

	if len(rule.Evidence) == 0 {
		fmt.Fprintln(stdout, "  Evidence: none recorded")
		return
	}
	fmt.Fprintf(stdout, "  Evidence (%d observation(s)):\n", len(rule.Evidence))
	for _, ev := range rule.Evidence {
		switch {
		case ev.Source != "" && !ev.Timestamp.IsZero():
			fmt.Fprintf(stdout, "    - %s at %s\n", ev.Source, ev.Timestamp.Format("2006-01-02T15:04:05Z07:00"))
		case ev.Source != "":
			fmt.Fprintf(stdout, "    - %s\n", ev.Source)
		case !ev.Timestamp.IsZero():
			fmt.Fprintf(stdout, "    - observed at %s\n", ev.Timestamp.Format("2006-01-02T15:04:05Z07:00"))
		default:
			fmt.Fprintln(stdout, "    - (no source or timestamp recorded)")
		}
	}
}
