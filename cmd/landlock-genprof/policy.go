// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// policy is the noun group over SecurityProfileProposal's own real,
// already-existing state (see internal/proposal — the same store
// review/approve/reject already use), not new infrastructure: `policy
// list` and `policy status` just add a list/current-state view on top
// of a CRD this project has managed since v0.2's approval lifecycle.
//
// `policy status` is deliberately not a copy of `review`: review prints
// the full human-readable spec for a person about to decide; status
// only prints the current approval state and, unlike every other
// read-only command in this project, gates on it — exit 0 only if
// Approved, exit 2 (blocking) otherwise. This is the one command in the
// whole CLI whose job is "has a human signed off," a distinct question
// from every ABI/correctness check the rest of the CLI asks.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Inspects SecurityProfileProposal approval state",
		Long:  "Inspects SecurityProfileProposal approval state. See `policy list`/`policy status`." + kubectlPrefixNote,
	}
	cmd.AddCommand(newPolicyListCmd())
	cmd.AddCommand(newPolicyStatusCmd())
	return cmd
}

type policyListOptions struct {
	namespace string
}

func newPolicyListCmd() *cobra.Command {
	var opts policyListOptions

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists SecurityProfileProposals and their approval state",
		Long:  "Lists every SecurityProfileProposal in a namespace and its current approval state." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof policy list
  kubectl landlock-genprof policy list --namespace prod`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyList(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	return cmd
}

func runPolicyList(ctx context.Context, stdout io.Writer, opts policyListOptions) error {
	client, err := newDynamicClientForPolicy()
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	items, err := proposal.List(ctx, client, opts.namespace)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintf(stdout, "No SecurityProfileProposals in namespace %s.\n", opts.namespace)
		return nil
	}

	for _, item := range items {
		fmt.Fprintf(stdout, "%-30s %s\n", item.Name, item.Status.ApprovalState)
	}
	return nil
}

type policyStatusOptions struct {
	namespace string
}

func newPolicyStatusCmd() *cobra.Command {
	var opts policyStatusOptions

	cmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Reports whether a SecurityProfileProposal has been approved",
		Long: "Reports a SecurityProfileProposal's current approval state — exits 0 " +
			"only if Approved, 2 (blocking) otherwise. Meant as a CI gate: \"has a " +
			"human signed off on this before it gets applied,\" distinct from every " +
			"correctness/ABI check the rest of this CLI does." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof policy status nginx-demo`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyStatus(cmd.Context(), cmd.OutOrStdout(), opts, args[0])
		},
	}
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	return cmd
}

func runPolicyStatus(ctx context.Context, stdout io.Writer, opts policyStatusOptions, name string) error {
	client, err := newDynamicClientForPolicy()
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	status, err := proposal.GetStatus(ctx, client, opts.namespace, name)
	if err != nil {
		return err
	}
	if status == nil {
		return fmt.Errorf("securityprofileproposal %s/%s not found", opts.namespace, name)
	}

	fmt.Fprintf(stdout, "%s/%s: %s\n", opts.namespace, name, status.ApprovalState)
	if status.Reason != "" {
		fmt.Fprintf(stdout, "  Reason: %s\n", status.Reason)
	}

	if status.ApprovalState != proposal.ApprovalApproved {
		return &exitCodeError{code: 2, wrapped: fmt.Errorf(
			"%s/%s is not Approved (currently %s)", opts.namespace, name, status.ApprovalState)}
	}
	return nil
}

// newDynamicClientForPolicy is a test seam, same pattern as review.go's
// newDynamicClientForReview / approve.go's newDynamicClientForApproval.
var newDynamicClientForPolicy func() (dynamic.Interface, error) = newDynamicClient
