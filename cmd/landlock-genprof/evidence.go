// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// evidence is the noun group over internal/evidence's raw capture
// format (see docs/cli-design.md) — deliberately just `show` for now.
// `list`/`import` aren't built: there's no registry or directory
// convention yet for multiple evidence files to enumerate, and no
// external source (SPO, strace, auditd) wired up to import from —
// building either now would be speculative, ahead of a real need,
// unlike `show`, which answers a real, distinct question `explain`
// doesn't: not "what rules did synthesis produce" but "what did the
// tracer actually see."
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/idriss-eliguene/landlock-genprof/internal/evidence"
)

func newEvidenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Inspects raw captured evidence",
		Long:  "Inspects raw captured evidence (see internal/evidence). See `evidence show`." + kubectlPrefixNote,
	}
	cmd.AddCommand(newEvidenceShowCmd())
	return cmd
}

func newEvidenceShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <events-file>",
		Short: "Summarizes a raw evidence file",
		Long: "Summarizes a raw evidence file (see `trace --events-out`): how many " +
			"events of each kind, distinct filesystem paths and network ports " +
			"touched, and the observation time span. Answers \"what did the tracer " +
			"actually see,\" distinct from `explain`, which answers \"what rules did " +
			"synthesis produce from it.\"" + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof evidence show nginx-demo-events.json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvidenceShow(cmd.OutOrStdout(), args[0])
		},
	}
	return cmd
}

func runEvidenceShow(stdout io.Writer, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading evidence file: %w", err)
	}
	events, architectures, err := evidence.FromJSON(data)
	if err != nil {
		return fmt.Errorf("parsing evidence file: %w", err)
	}

	fmt.Fprintf(stdout, "%d event(s)\n", len(events))
	if len(events) == 0 {
		return nil
	}

	var fsCount, netCount, syscallCount, capCount int
	fsModeCounts := make(map[string]int)
	paths := make(map[string]bool)
	ports := make(map[int]bool)
	syscalls := make(map[string]bool)
	capabilities := make(map[string]bool)
	var first, last time.Time

	for _, ev := range events {
		if !ev.Timestamp.IsZero() {
			if first.IsZero() || ev.Timestamp.Before(first) {
				first = ev.Timestamp
			}
			if ev.Timestamp.After(last) {
				last = ev.Timestamp
			}
		}

		switch ev.Mode {
		case "syscall":
			syscallCount++
			syscalls[ev.Syscall] = true
		case "capability":
			capCount++
			capabilities[ev.Syscall] = true
		case "egress", "ingress":
			netCount++
			ports[ev.Port] = true
		default:
			if ev.Path != "" {
				fsCount++
				fsModeCounts[ev.Mode]++
				paths[ev.Path] = true
			}
		}
	}

	fmt.Fprintf(stdout, "Filesystem: %d event(s) across %d distinct path(s)", fsCount, len(paths))
	if len(fsModeCounts) > 0 {
		fmt.Fprint(stdout, " (")
		modes := make([]string, 0, len(fsModeCounts))
		for m := range fsModeCounts {
			modes = append(modes, m)
		}
		sort.Strings(modes)
		for i, m := range modes {
			if i > 0 {
				fmt.Fprint(stdout, ", ")
			}
			fmt.Fprintf(stdout, "%s: %d", m, fsModeCounts[m])
		}
		fmt.Fprint(stdout, ")")
	}
	fmt.Fprintln(stdout)

	fmt.Fprintf(stdout, "Network: %d event(s) across %d distinct port(s)\n", netCount, len(ports))
	fmt.Fprintf(stdout, "Syscalls: %d distinct\n", len(syscalls))
	fmt.Fprintf(stdout, "Capabilities: %d distinct\n", len(capabilities))

	if len(architectures) > 0 {
		fmt.Fprintf(stdout, "Architectures: %v\n", architectures)
	}

	if !first.IsZero() && !last.IsZero() {
		fmt.Fprintf(stdout, "Observed: %s to %s\n",
			first.Format("2006-01-02T15:04:05Z07:00"),
			last.Format("2006-01-02T15:04:05Z07:00"))
	}

	return nil
}
