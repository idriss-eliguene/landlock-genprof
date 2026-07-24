// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Command landlock-genprof observes a running Kubernetes pod and generates
// a minimal Landlock profile in the PodLock format.
//
// Usage:
//
//	landlock-genprof trace --pod <name> --namespace <ns> --binary <path> --duration 60s --out profile.yaml
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "landlock-genprof",
		Short: "Generates a Landlock profile by observing a Kubernetes pod",
		// SilenceErrors: main() already prints the error returned by Execute();
		// without this, cobra would print it a second time (prefixed "Error: ").
		SilenceErrors: true,
		// SilenceUsage: the usage block only makes sense for a flag/argument
		// error, not a runtime failure (unreachable cluster, pod not found...) —
		// cobra prints it by default in both cases.
		SilenceUsage: true,
	}
	root.AddCommand(newTraceCmd())
	root.AddCommand(newReviewCmd())
	root.AddCommand(newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints the version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "landlock-genprof (dev)")
			return nil
		},
	}
}
