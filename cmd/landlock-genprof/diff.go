// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// diff compares two synthesized candidates (see
// internal/exporter/landlockjson) rule by rule — the command meant to
// catch a real, recurring problem: a dependency bump silently widening
// (or narrowing) what a workload needs, invisible if the only thing
// reviewed is the latest candidate on its own. This is also the first
// command to use exit code 3 for real: doctor/verify/abi only ever use
// 0/1/2, but diff's own consumers (a CI pipeline gating a merge on "did
// this PR's re-synthesis change anything") need to tell "the two
// candidates differ" (1) apart from "diff itself couldn't run" (3, a
// missing file or a malformed candidate) — collapsing those into the
// same code would make the gate unable to distinguish a real signal
// from a broken invocation.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/junit"
	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/landlockjson"
	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
)

type diffOptions struct {
	output string
}

func newDiffCmd() *cobra.Command {
	var opts diffOptions

	cmd := &cobra.Command{
		Use:   "diff <old-candidate-file> <new-candidate-file>",
		Short: "Compares two synthesized candidates rule by rule",
		Long: "Compares two synthesized candidates (see " +
			"internal/exporter/landlockjson), reporting rules added, removed, or " +
			"whose rights changed — the check for a dependency bump silently " +
			"widening or narrowing what a workload needs between two training runs. " +
			"--output junit renders one testcase per rule path (failed = changed) " +
			"instead of text, for CI dashboards that already render JUnit results — " +
			"the exit-code contract (0/1/3) is unchanged either way." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof diff nginx-demo-candidate-old.json nginx-demo-candidate-new.json
  kubectl landlock-genprof diff old.json new.json --output junit > diff.xml`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(cmd.OutOrStdout(), args[0], args[1], opts)
		},
	}
	cmd.Flags().StringVar(&opts.output, "output", "text", "Output format: text or junit")
	return cmd
}

func runDiff(stdout io.Writer, oldFile, newFile string, opts diffOptions) error {
	output := opts.output
	if output == "" {
		output = "text"
	}
	if output != "text" && output != "junit" {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("unsupported --output %q (supported: text, junit)", output)}
	}

	oldCandidate, err := readCandidateFile(oldFile)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("reading %s: %w", oldFile, err)}
	}
	newCandidate, err := readCandidateFile(newFile)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("reading %s: %w", newFile, err)}
	}

	oldByPath := rulesByPath(oldCandidate)
	newByPath := rulesByPath(newCandidate)

	paths := make(map[string]bool, len(oldByPath)+len(newByPath))
	for path := range oldByPath {
		paths[path] = true
	}
	for path := range newByPath {
		paths[path] = true
	}
	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	changed := false
	var cases []junit.TestCase
	for _, path := range sortedPaths {
		oldRule, hadOld := oldByPath[path]
		newRule, hasNew := newByPath[path]

		var textLine, failure string
		switch {
		case !hadOld && hasNew:
			textLine = fmt.Sprintf("+ %s: %v", path, newRule.Rights)
			failure = fmt.Sprintf("+%v", newRule.Rights)
		case hadOld && !hasNew:
			textLine = fmt.Sprintf("- %s: %v", path, oldRule.Rights)
			failure = fmt.Sprintf("-%v", oldRule.Rights)
		default:
			added, removed := rightsDelta(oldRule.Rights, newRule.Rights)
			if len(added) == 0 && len(removed) == 0 {
				break
			}
			var b strings.Builder
			if len(added) > 0 {
				fmt.Fprintf(&b, "+%v", added)
			}
			if len(removed) > 0 {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				fmt.Fprintf(&b, "-%v", removed)
			}
			failure = b.String()
			textLine = fmt.Sprintf("~ %s: %s", path, failure)
		}

		if failure == "" {
			if output == "junit" {
				cases = append(cases, junit.TestCase{ClassName: "diff", Name: path})
			}
			continue
		}

		changed = true
		if output == "text" {
			fmt.Fprintln(stdout, textLine)
		} else {
			cases = append(cases, junit.TestCase{ClassName: "diff", Name: path, Failure: failure})
		}
	}

	if output == "junit" {
		data, err := junit.ToXML(junit.Meta{SuiteName: "landlock-genprof diff"}, cases)
		if err != nil {
			return &exitCodeError{code: 3, wrapped: err}
		}
		fmt.Fprintln(stdout, string(data))
	} else if !changed {
		fmt.Fprintln(stdout, "No differences.")
	}

	if changed {
		return &exitCodeError{code: 1}
	}
	return nil
}

func readCandidateFile(path string) (landlock.Candidate, error) {
	// path is one of the two candidate files the operator named as
	// positional arguments to `diff` — comparing two operator-chosen
	// files, which routinely live in different directories, is the whole
	// command. Read with the operator's own privileges on their own
	// machine; there is no intended filesystem root to confine it to.
	// #nosec G304 -- explicit operator-supplied candidate path, see comment above
	data, err := os.ReadFile(path)
	if err != nil {
		return landlock.Candidate{}, err
	}
	return landlockjson.FromJSON(data)
}

func rulesByPath(c landlock.Candidate) map[string]landlock.Rule {
	m := make(map[string]landlock.Rule, len(c.Rules))
	for _, r := range c.Rules {
		m[r.Path] = r
	}
	return m
}

// rightsDelta returns the rights present in newRights but not old
// (added) and vice versa (removed) — order-independent, since Rights
// slices are already deterministically ordered by internal/landlock but
// a set comparison is what "changed" actually means here.
func rightsDelta(oldRights, newRights []landlock.LandlockRight) (added, removed []landlock.LandlockRight) {
	oldSet := make(map[landlock.LandlockRight]bool, len(oldRights))
	for _, r := range oldRights {
		oldSet[r] = true
	}
	newSet := make(map[landlock.LandlockRight]bool, len(newRights))
	for _, r := range newRights {
		newSet[r] = true
	}
	for _, r := range newRights {
		if !oldSet[r] {
			added = append(added, r)
		}
	}
	for _, r := range oldRights {
		if !newSet[r] {
			removed = append(removed, r)
		}
	}
	return added, removed
}
