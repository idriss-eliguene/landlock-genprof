// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

type applyProposalOptions struct {
	namespace string
	yes       bool
	skip      []string
}

// knownArtifactSlugs is every valid --skip value, for validating the
// flag up front rather than silently no-op'ing on a typo (e.g.
// --skip=podlok would otherwise apply everything, including PodLock,
// while looking like it worked).
var knownArtifactSlugs = []string{"podlock", "networkpolicy", "patched-manifest", "spo-seccompprofile"}

func newApplyProposalCmd() *cobra.Command {
	var opts applyProposalOptions

	cmd := &cobra.Command{
		Use:   "apply-proposal <proposal>",
		Short: "Reviews and applies a published SecurityProfileProposal's artifacts, with a confirmation prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApplyProposal(cmd.Context(), cmd.OutOrStdout(), cmd.InOrStdin(), opts, args[0])
		},
	}

	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Skip the confirmation prompt (for CI/non-interactive use); still prints what it applied")
	cmd.Flags().StringSliceVar(&opts.skip, "skip", nil,
		"Artifact(s) to leave out of this apply, comma-separated or repeated — one of: "+
			strings.Join(knownArtifactSlugs, ", ")+
			". E.g. --skip=patched-manifest if the enforcement side (SPO/PodLock) isn't ready yet: "+
			"applying the patched manifest restarts the target pod referencing a seccompProfile/PodLock "+
			"label that won't resolve to anything until the matching operator is, breaking the pod outright.")
	return cmd
}

// runApplyProposal fetches proposalName, prints exactly what it's about
// to do (same artifact list review prints), asks for confirmation unless
// opts.yes, then applies each available artifact via internal/k8s.Apply.
//
// This is the CLI-native equivalent of the Makefile's export-proposal +
// apply-proposal targets, which stay as they are for contributors/local
// testing — those shell out to `kubectl apply -f` blindly, with no
// preview or confirmation step at all. This command exists specifically
// to add that review step for anyone using the tool, not developing it.
func runApplyProposal(ctx context.Context, stdout io.Writer, stdin io.Reader, opts applyProposalOptions, proposalName string) error {
	skip, err := parseSkipArtifacts(opts.skip)
	if err != nil {
		return err
	}

	dynClient, err := newDynamicClientForApplyProposal()
	if err != nil {
		return fmt.Errorf("connecting to cluster for apply-proposal: %w", err)
	}

	spec, err := proposal.Get(ctx, dynClient, opts.namespace, proposalName)
	if err != nil {
		return err
	}
	if spec == nil {
		return fmt.Errorf("securityprofileproposal %s/%s not found", opts.namespace, proposalName)
	}

	artifacts := proposalArtifacts(spec)
	printProposalSummary(stdout, opts.namespace, proposalName, spec, artifacts)

	var toApply, skipped []proposalArtifact
	for _, artifact := range artifacts {
		if !artifact.available {
			continue
		}
		if skip[artifact.slug] {
			skipped = append(skipped, artifact)
			continue
		}
		toApply = append(toApply, artifact)
	}

	if len(skipped) > 0 {
		fmt.Fprintf(stdout, "\nSkipping %d artifact(s), per --skip:\n", len(skipped))
		for _, artifact := range skipped {
			fmt.Fprintf(stdout, "  - %s\n", artifact.name)
		}
	}

	if len(toApply) == 0 {
		if len(skipped) > 0 {
			fmt.Fprintln(stdout, "\nNothing left to apply — every available artifact was skipped.")
		} else {
			fmt.Fprintln(stdout, "\nNo artifacts to apply — this proposal generated nothing (empty training run?).")
		}
		return nil
	}

	fmt.Fprintf(stdout, "\nThis will apply %d artifact(s):\n", len(toApply))
	for _, artifact := range toApply {
		fmt.Fprintf(stdout, "  - %s\n", artifact.name)
	}
	fmt.Fprintln(stdout)

	if !opts.yes {
		fmt.Fprint(stdout, "Apply these to the cluster? [y/N] ")
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(stdout, "Aborted — nothing applied.")
			return nil
		}
	}

	// Keep going past a failed artifact rather than aborting on the first
	// one: PodLock/SPO need their own operator installed to even have a
	// matching CRD (see docs/usage.md's prerequisite breakdown), so on a
	// cluster with only some of PodLock/CNI/SPO present, stopping at the
	// first failure would mean artifacts that *would* succeed (e.g. a
	// plain NetworkPolicy) never even get attempted.
	var failed []string
	for _, artifact := range toApply {
		if err := k8s.Apply(ctx, dynClient, opts.namespace, artifact.content); err != nil {
			fmt.Fprintf(stdout, "failed: %s — %v\n", artifact.name, err)
			failed = append(failed, artifact.name)
			continue
		}
		fmt.Fprintf(stdout, "applied: %s\n", artifact.name)
	}

	if len(failed) > 0 {
		fmt.Fprintf(stdout, "\n%d of %d artifact(s) failed to apply: %s\n",
			len(failed), len(toApply), strings.Join(failed, ", "))
		return fmt.Errorf("apply-proposal: %d artifact(s) failed", len(failed))
	}

	fmt.Fprintln(stdout, "\nDone.")
	return nil
}

// parseSkipArtifacts validates --skip against knownArtifactSlugs up
// front and returns a lookup set — failing fast on a typo (e.g.
// --skip=podlok) matters here specifically: silently ignoring an
// unrecognized slug would mean the command applies *everything*,
// including whatever the caller meant to exclude, while still looking
// like the flag was honored.
func parseSkipArtifacts(skip []string) (map[string]bool, error) {
	known := make(map[string]bool, len(knownArtifactSlugs))
	for _, s := range knownArtifactSlugs {
		known[s] = true
	}

	result := make(map[string]bool, len(skip))
	for _, raw := range skip {
		s := strings.ToLower(strings.TrimSpace(raw))
		if !known[s] {
			return nil, fmt.Errorf("--skip=%q: not a known artifact — one of: %s",
				raw, strings.Join(knownArtifactSlugs, ", "))
		}
		result[s] = true
	}
	return result, nil
}

// newDynamicClientForApplyProposal is a test seam, same pattern as
// review.go's newDynamicClientForReview / trace.go's
// newDynamicClientForProposal.
var newDynamicClientForApplyProposal func() (dynamic.Interface, error) = newDynamicClient
