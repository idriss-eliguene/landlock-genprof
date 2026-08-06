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

// kubectlPrefixNote is appended to every command's Long: cobra/doc's
// generated Usage code block (book/src/cli/, see gendocs.go) always
// shows the bare `landlock-genprof <command> ...` form — it's built
// from cmd.UseLine(), which reflects the real Use chain, and Use can
// only ever say "landlock-genprof" (this binary also runs standalone,
// where that bare form is exactly right) without misrepresenting that
// case. Repeating this note in each subcommand's own Synopsis, not just
// the root's, means it's visible on whichever page a reader actually
// lands on first — confirmed live: a reader on apply-proposal's own
// page never necessarily saw the root page's version of this note.
const kubectlPrefixNote = "\n\nInstalled as a kubectl plugin (the common case): run this as " +
	"`kubectl landlock-genprof <command>`. Running this binary directly instead " +
	"(standalone, not via kubectl) works the same way, without that prefix."

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
		// Long, not just Short: cobra's default help template and
		// cobra/doc's GenMarkdownTree (book/src/cli/, see gendocs.go) both
		// print this — the one place to say once that every command below
		// is normally invoked as `kubectl landlock-genprof <command>`, not
		// bare, since Use only ever says "landlock-genprof" (this binary
		// also runs standalone, same command names either way — kubectl
		// itself supplies the "kubectl " prefix by rewriting
		// kubectl-landlock_genprof, the installed binary name, into a
		// plugin invocation).
		Long: "Observes a running Kubernetes pod and generates least-privilege security " +
			"profiles from what it actually saw — a PodLock LandlockProfile always, plus " +
			"NetworkPolicy/seccomp/Linux capabilities/securityContext outputs behind their " +
			"own flags." + kubectlPrefixNote,
		// SilenceErrors: main() already prints the error returned by Execute();
		// without this, cobra would print it a second time (prefixed "Error: ").
		SilenceErrors: true,
		// SilenceUsage: the usage block only makes sense for a flag/argument
		// error, not a runtime failure (unreachable cluster, pod not found...) —
		// cobra prints it by default in both cases.
		SilenceUsage: true,
	}
	root.AddCommand(newTraceCmd())
	root.AddCommand(newSynthesizeCmd())
	root.AddCommand(newReviewCmd())
	root.AddCommand(newApplyProposalCmd())
	root.AddCommand(newApproveCmd())
	root.AddCommand(newRejectCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newABICmd())
	root.AddCommand(newVerifyCmd())
	root.AddCommand(newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Prints the version",
		Long:    "Prints the version." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "landlock-genprof %s (commit %s, built %s)\n", version, commit, date)
			return nil
		},
	}
}
