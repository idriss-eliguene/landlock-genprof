// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version/commit/date are injected at build time via -ldflags (see
// Makefile's build-plugin target) — go run/go build without -ldflags
// keeps these defaults, which is correct: a dev build should say so
// rather than claim a version it wasn't actually tagged/built as.
//
// Deliberately untagged (unlike main.go/gendocs.go): newRootCmd is the
// one thing both builds need to share — main.go's real main() executes
// it, gendocs.go's main() (see its own doc comment) introspects it to
// regenerate book/src/cli/ via cobra's own doc.GenMarkdownTree.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "landlock-genprof",
		Short: "Generates least-privilege Kubernetes security profiles by observing a running pod",
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
	root.AddCommand(newApplyProposalCmd())
	root.AddCommand(newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints the version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "landlock-genprof %s (commit %s, built %s)\n", version, commit, date)
			return nil
		},
	}
}
