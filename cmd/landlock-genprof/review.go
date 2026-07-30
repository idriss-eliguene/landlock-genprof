// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

// proposalArtifact is one of a SecurityProfileProposal's four possible
// generated artifacts, shared between review (lists them) and
// apply-proposal (applies the available ones) so both stay in sync on
// what a proposal can contain.
type proposalArtifact struct {
	name      string
	content   string
	available bool
}

func proposalArtifacts(spec *proposal.Spec) []proposalArtifact {
	return []proposalArtifact{
		{name: "PodLock", content: spec.PodLock, available: spec.PodLock != ""},
		{name: "NetworkPolicy", content: spec.NetworkPolicy, available: spec.NetworkPolicy != ""},
		{name: "Patched Manifest", content: spec.PatchedManifest, available: spec.PatchedManifest != ""},
		{name: "SPO SeccompProfile", content: spec.SPOSeccompProfile, available: spec.SPOSeccompProfile != ""},
	}
}

// printProposalSummary prints the "WORKLOAD SECURITY REVIEW" block —
// shared between review (the whole point of that command) and
// apply-proposal (shown before its confirmation prompt, so approving a
// change is never based on less context than a standalone review would
// give: what container/binary this came from, whether it used
// cross-run history, and whether the PodLock label made it into the
// patched manifest).
func printProposalSummary(stdout io.Writer, namespace, proposalName string, spec *proposal.Spec, artifacts []proposalArtifact) {
	availableCount := 0
	for _, artifact := range artifacts {
		if artifact.available {
			availableCount++
		}
	}

	fmt.Fprintln(stdout, "\nWORKLOAD SECURITY REVIEW")
	fmt.Fprintf(stdout, "Proposal: %s/%s\n", namespace, proposalName)
	fmt.Fprintf(stdout, "Container: %s\n", spec.Container)
	fmt.Fprintf(stdout, "Binary: %s\n", spec.Binary)
	fmt.Fprintf(stdout, "Generated at: %s\n", spec.GeneratedAt)
	fmt.Fprintf(stdout, "History used: %t\n", spec.HistoryUsed)
	fmt.Fprintf(stdout, "Artifacts available: %d/%d\n", availableCount, len(artifacts))

	for _, artifact := range artifacts {
		status := "not generated"
		if artifact.available {
			status = "available"
		}
		fmt.Fprintf(stdout, "- %s: %s\n", artifact.name, status)
	}

	if spec.PatchedManifest != "" {
		labelStatus := "missing"
		if strings.Contains(spec.PatchedManifest, podLockProfileLabel) {
			labelStatus = "present"
		}
		fmt.Fprintf(stdout, "Patched manifest PodLock label: %s\n", labelStatus)
	}
}

type reviewOptions struct {
	namespace string
}

func newReviewCmd() *cobra.Command {
	var opts reviewOptions

	cmd := &cobra.Command{
		Use:   "review <proposal>",
		Short: "Reviews a published SecurityProfileProposal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(cmd.Context(), cmd.OutOrStdout(), opts, args[0])
		},
	}

	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	return cmd
}

func runReview(ctx context.Context, stdout io.Writer, opts reviewOptions, proposalName string) error {
	client, err := newDynamicClientForReview()
	if err != nil {
		return fmt.Errorf("connecting to cluster for review: %w", err)
	}

	spec, err := proposal.Get(ctx, client, opts.namespace, proposalName)
	if err != nil {
		return err
	}
	if spec == nil {
		return fmt.Errorf("securityprofileproposal %s/%s not found", opts.namespace, proposalName)
	}

	artifacts := proposalArtifacts(spec)
	printProposalSummary(stdout, opts.namespace, proposalName, spec, artifacts)

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next steps:")
	fmt.Fprintf(stdout, "- Inspect the full proposal: kubectl get securityprofileproposal %s -n %s -o yaml\n", proposalName, opts.namespace)
	fmt.Fprintf(stdout, "- Review and apply, with a confirmation prompt: kubectl landlock-genprof apply-proposal %s -n %s\n", proposalName, opts.namespace)
	fmt.Fprintf(stdout, "- From a local clone instead: make export-proposal PROPOSAL=%s NS=%s (then make apply-proposal for the same, unprompted)\n", proposalName, opts.namespace)
	return nil
}

var newDynamicClientForReview func() (dynamic.Interface, error) = newDynamicClient
