// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// evidence is the noun group over internal/evidence's raw capture
// format (see docs/cli-design.md). `import` still isn't built: no
// external source (SPO, strace, auditd) is wired up to import from yet
// — building it now would be speculative, ahead of a real need. `list`
// deliberately doesn't invent a registry either: it scans a directory
// for files that happen to parse as evidence (silently skipping ones
// that don't, e.g. a candidate.json sitting next to them), which is
// honest about the actual reality — evidence files are just files, not
// entries in a store this project doesn't have.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/idriss-eliguene/landlock-genprof/internal/evidence"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// observationWindow returns the earliest and latest non-zero Timestamp
// across events, or two zero Times if none carry a timestamp — shared by
// `evidence show` (one file, full detail) and `evidence list` (many
// files, one summary line each) so both report the same span the same way.
func observationWindow(events []tracer.Event) (first, last time.Time) {
	for _, ev := range events {
		if ev.Timestamp.IsZero() {
			continue
		}
		if first.IsZero() || ev.Timestamp.Before(first) {
			first = ev.Timestamp
		}
		if ev.Timestamp.After(last) {
			last = ev.Timestamp
		}
	}
	return first, last
}

func newEvidenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Inspects raw captured evidence",
		Long:  "Inspects raw captured evidence (see internal/evidence). See `evidence show`/`evidence list`." + kubectlPrefixNote,
	}
	cmd.AddCommand(newEvidenceShowCmd())
	cmd.AddCommand(newEvidenceListCmd())
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

	for _, ev := range events {
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

	first, last := observationWindow(events)
	if !first.IsZero() && !last.IsZero() {
		fmt.Fprintf(stdout, "Observed: %s to %s\n",
			first.Format("2006-01-02T15:04:05Z07:00"),
			last.Format("2006-01-02T15:04:05Z07:00"))
	}

	return nil
}

func newEvidenceListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [directory]",
		Short: "Lists evidence files in a directory",
		Long: "Scans a directory (default: current directory) for files that parse as " +
			"evidence (see `trace --events-out`), one summary line each: event count " +
			"and observation window. Not a registry — there isn't one — just a scan; " +
			"files that don't parse as evidence (e.g. a candidate.json sitting next to " +
			"them) are silently skipped." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof evidence list
  kubectl landlock-genprof evidence list ./captures`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return runEvidenceList(cmd.OutOrStdout(), dir)
		},
	}
	return cmd
}

func runEvidenceList(stdout io.Writer, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	found := 0
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		events, _, err := evidence.FromJSON(data)
		if err != nil {
			continue
		}

		found++
		fmt.Fprintf(stdout, "%-40s %d event(s)", name, len(events))
		first, last := observationWindow(events)
		if !first.IsZero() && !last.IsZero() {
			fmt.Fprintf(stdout, ", observed %s to %s",
				first.Format("2006-01-02T15:04:05Z07:00"),
				last.Format("2006-01-02T15:04:05Z07:00"))
		}
		fmt.Fprintln(stdout)
	}

	if found == 0 {
		fmt.Fprintf(stdout, "No evidence files found in %s.\n", dir)
	}
	return nil
}
