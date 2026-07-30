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
}

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

	var toApply []proposalArtifact
	for _, artifact := range proposalArtifacts(spec) {
		if artifact.available {
			toApply = append(toApply, artifact)
		}
	}
	if len(toApply) == 0 {
		fmt.Fprintln(stdout, "No artifacts to apply — this proposal generated nothing (empty training run?).")
		return nil
	}

	fmt.Fprintf(stdout, "\nThis will apply %d artifact(s) from %s/%s:\n", len(toApply), opts.namespace, proposalName)
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

	for _, artifact := range toApply {
		if err := k8s.Apply(ctx, dynClient, opts.namespace, artifact.content); err != nil {
			return fmt.Errorf("applying %s: %w", artifact.name, err)
		}
		fmt.Fprintf(stdout, "applied: %s\n", artifact.name)
	}

	fmt.Fprintln(stdout, "\nDone.")
	return nil
}

// newDynamicClientForApplyProposal is a test seam, same pattern as
// review.go's newDynamicClientForReview / trace.go's
// newDynamicClientForProposal.
var newDynamicClientForApplyProposal func() (dynamic.Interface, error) = newDynamicClient
