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
// what a proposal can contain. slug is the --skip-matching identifier —
// lowercase, stable, independent of name's display formatting.
type proposalArtifact struct {
	name      string
	slug      string
	content   string
	available bool
}

// patchedManifestSlug is proposalArtifacts' Patched Manifest slug, pulled
// out as a constant since apply_proposal.go's --restart gating (unlike
// --skip) needs to name this one specific artifact rather than just
// matching against knownArtifactSlugs generically — it's the only
// artifact whose apply deletes and recreates the target pod.
const patchedManifestSlug = "patched-manifest"

func proposalArtifacts(spec *proposal.Spec) []proposalArtifact {
	return []proposalArtifact{
		{name: "PodLock", slug: "podlock", content: spec.PodLock, available: spec.PodLock != ""},
		{name: "NetworkPolicy", slug: "networkpolicy", content: spec.NetworkPolicy, available: spec.NetworkPolicy != ""},
		{name: "Patched Manifest", slug: patchedManifestSlug, content: spec.PatchedManifest, available: spec.PatchedManifest != ""},
		{name: "SPO SeccompProfile", slug: "spo-seccompprofile", content: spec.SPOSeccompProfile, available: spec.SPOSeccompProfile != ""},
	}
}

// printProposalSummary prints the "WORKLOAD SECURITY REVIEW" block —
// shared between review (the whole point of that command) and
// apply-proposal (shown before its confirmation prompt, so approving a
// change is never based on less context than a standalone review would
// give: what container/binary this came from, whether it used
// cross-run history, and whether the PodLock label made it into the
// patched manifest).
func printProposalSummary(stdout io.Writer, namespace, proposalName string, spec *proposal.Spec, status *proposal.Status, artifacts []proposalArtifact) {
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
	if status != nil {
		fmt.Fprintf(stdout, "Approval: %s\n", status.ApprovalState)
		if status.Reason != "" {
			fmt.Fprintf(stdout, "  Reason: %s\n", status.Reason)
		}
	}
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

	// Filesystem and network rules come from this project's own
	// observation and carry a confidence tier. Seccomp may not — under
	// docs/adr/0008 it can be policy another system derived, with no
	// occurrence data behind it. A reviewer must be able to tell which,
	// without inferring it, so the distinction is printed rather than left
	// to be deduced from the artifact's contents.
	printSeccompProvenance(stdout, spec.SPOSeccompProfile)
}

type reviewOptions struct {
	namespace string
}

func newReviewCmd() *cobra.Command {
	var opts reviewOptions

	cmd := &cobra.Command{
		Use:   "review <proposal>",
		Short: "Reviews a published SecurityProfileProposal",
		Long:  "Reviews a published SecurityProfileProposal." + kubectlPrefixNote,
		Example: `  # Same proposal name as the pod trace was run against
  kubectl landlock-genprof review nginx-demo

  kubectl landlock-genprof review nginx-demo --namespace prod`,
		Args: cobra.ExactArgs(1),
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

	// Compute and display the CandidateDigest before attempting to mark
	// the proposal Reviewed — the reviewer must see the exact digest they
	// are authorizing, and a digest computation failure must prevent a
	// Reviewed stamp being written (not the other way round).
	digest, err := proposal.CandidateDigest(*spec)
	if err != nil {
		return fmt.Errorf("computing candidate digest for review: %w", err)
	}
	fmt.Fprintf(stdout, "Candidate digest: %s\n", digest)

	// Best-effort: mark Reviewed but do not fail the review if the caller
	// lacks permission to write status. The digest was already shown.
	if err := proposal.MarkReviewed(ctx, client, opts.namespace, proposalName); err != nil {
		fmt.Fprintf(stdout, "Warning: could not mark this proposal as reviewed: %v\n", err)
	}

	status, err := proposal.GetStatus(ctx, client, opts.namespace, proposalName)
	if err != nil {
		fmt.Fprintf(stdout, "Warning: could not fetch approval status: %v\n", err)
	}

	artifacts := proposalArtifacts(spec)
	printProposalSummary(stdout, opts.namespace, proposalName, spec, status, artifacts)

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next steps:")
	fmt.Fprintf(stdout, "- Inspect the full proposal: kubectl get securityprofileproposal %s -n %s -o yaml\n", proposalName, opts.namespace)
	fmt.Fprintf(stdout, "- Approve this reviewed digest: kubectl landlock-genprof approve %s -n %s --expected-digest %s\n", proposalName, opts.namespace, digest)
	fmt.Fprintf(stdout, "- Apply the approved proposal: kubectl landlock-genprof apply-proposal %s -n %s\n", proposalName, opts.namespace)
	fmt.Fprintf(stdout, "- Inspection only (non-authoritative): make export-proposal PROPOSAL=%s NS=%s\n", proposalName, opts.namespace)
	return nil
}

var newDynamicClientForReview func() (dynamic.Interface, error) = newDynamicClient
