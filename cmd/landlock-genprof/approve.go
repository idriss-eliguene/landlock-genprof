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

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

// approveRejectOptions is shared between `approve` and `reject` — same
// two flags, same target-resolution logic, only the resulting
// proposal.ApprovalState differs.
type approveRejectOptions struct {
	namespace string
	reason    string
	// expectedDigest is the reviewer's asserted CandidateDigest; required
	// for approve to protect against stale-reviewer misbinding.
	expectedDigest string
}

func newApproveCmd() *cobra.Command {
	var opts approveRejectOptions

	cmd := &cobra.Command{
		Use:   "approve <proposal>",
		Short: "Records an explicit approval decision on a SecurityProfileProposal",
		Long: "Records an explicit approval decision on a SecurityProfileProposal — " +
			"a v0.2 addition to the review workflow (see docs/product-roadmap-v1.md). " +
			"Purely informational today: apply-proposal does not require Approved " +
			"yet, and still has its own separate [y/N] confirmation regardless of " +
			"approval state." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof approve nginx-demo

  kubectl landlock-genprof approve nginx-demo --reason "reviewed with the platform team, looks right" --expected-digest sha256:...`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetApprovalState(cmd.Context(), cmd.OutOrStdout(), opts, args[0], proposal.ApprovalApproved)
		},
	}

	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().StringVar(&opts.reason, "reason", "", "Optional free-text note explaining this decision")
	cmd.Flags().StringVar(&opts.expectedDigest, "expected-digest", "", "(approve only) Expected candidate digest to bind approval to (format: sha256:<hex>)")
	return cmd
}

func newRejectCmd() *cobra.Command {
	var opts approveRejectOptions

	cmd := &cobra.Command{
		Use:   "reject <proposal>",
		Short: "Records an explicit rejection decision on a SecurityProfileProposal",
		Long: "Records an explicit rejection decision on a SecurityProfileProposal — " +
			"a v0.2 addition to the review workflow (see docs/product-roadmap-v1.md). " +
			"Doesn't stop apply-proposal from running against a rejected proposal " +
			"today — that enforcement is a deliberate, separate future change, not " +
			"yet built. Re-run `trace` and reject again, or approve instead, once " +
			"whatever caused the rejection is addressed." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof reject nginx-demo --reason "syscalls list looks too broad, retrace with more traffic"`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetApprovalState(cmd.Context(), cmd.OutOrStdout(), opts, args[0], proposal.ApprovalRejected)
		},
	}

	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().StringVar(&opts.reason, "reason", "", "Optional free-text note explaining this decision")
	return cmd
}

func runSetApprovalState(ctx context.Context, stdout io.Writer, opts approveRejectOptions, proposalName string, state proposal.ApprovalState) error {
	client, err := newDynamicClientForApproval()
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	if err := proposal.SetApprovalState(ctx, client, opts.namespace, proposalName, state, opts.reason, opts.expectedDigest); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "%s/%s: %s\n", opts.namespace, proposalName, state)
	if opts.reason != "" {
		fmt.Fprintf(stdout, "  Reason: %s\n", opts.reason)
	}
	return nil
}

// newDynamicClientForApproval is a test seam, same pattern as
// review.go's newDynamicClientForReview / apply_proposal.go's
// newDynamicClientForApplyProposal.
var newDynamicClientForApproval func() (dynamic.Interface, error) = newDynamicClient
