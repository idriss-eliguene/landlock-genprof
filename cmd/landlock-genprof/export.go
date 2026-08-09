// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// export renders an already-synthesized candidate to a target output
// format — the missing half of Phase 1's five-verb set (see
// docs/cli-design.md): trace/synthesize already write a PodLock profile
// inline, but until this command existed there was no way to re-render
// an existing candidate.json to a format without re-running synthesis.
//
// Deliberately never mutates a cluster (unlike a future `apply`) — pure
// rendering, matching the terraform plan/apply split docs/cli-design.md
// commits to. --format is a real, if currently narrow, registry: only
// "podlock" is wired up today; a plugin/format registry (see
// docs/cli-design.md's plugin architecture) is how a second format gets
// added later, not a new top-level command.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/landlockjson"
	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/podlock"
	"github.com/idriss-eliguene/landlock-genprof/internal/landlock"
	"github.com/idriss-eliguene/landlock-genprof/internal/policy"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

type exportOptions struct {
	candidateFile string
	format        string
	podName       string
	namespace     string
	container     string
	binary        string
	out           string
}

func newExportCmd() *cobra.Command {
	var opts exportOptions

	cmd := &cobra.Command{
		Use:   "export --candidate-file <path> --format podlock --pod <name> --container <name> --binary <path>",
		Short: "Renders an already-synthesized candidate to a target output format",
		Long: "Renders an already-synthesized candidate (see " +
			"internal/exporter/landlockjson) to a target output format, without " +
			"re-running synthesis. Never mutates a cluster — pure rendering; see " +
			"a future `apply` for actually applying an approved artifact. " +
			"Prints to stdout unless --out is given." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof export --candidate-file nginx-demo-candidate.json \
    --format podlock --pod nginx-demo --namespace default --container nginx --binary /usr/sbin/nginx

  kubectl landlock-genprof export --candidate-file nginx-demo-candidate.json \
    --format podlock --pod nginx-demo --container nginx --binary /usr/sbin/nginx --out profile.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd.OutOrStdout(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.candidateFile, "candidate-file", "", "Path to a candidate JSON file (see internal/exporter/landlockjson) (required)")
	flags.StringVar(&opts.format, "format", "podlock", "Output format (supported today: podlock)")
	flags.StringVarP(&opts.podName, "pod", "p", "", "Pod name to label the output with (required)")
	flags.StringVarP(&opts.namespace, "namespace", "n", "default", "Namespace to label the output with")
	flags.StringVarP(&opts.container, "container", "c", "", "Container name to label the output with (required)")
	flags.StringVar(&opts.binary, "binary", "", "Binary path to label the output with (required)")
	flags.StringVarP(&opts.out, "out", "o", "", "Output file (default: stdout)")

	for _, name := range []string{"candidate-file", "pod", "container", "binary"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err) // programming error (flag doesn't exist), not a user error
		}
	}

	return cmd
}

func runExport(stdout io.Writer, opts exportOptions) error {
	data, err := os.ReadFile(opts.candidateFile)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("reading candidate file: %w", err)}
	}
	candidate, err := landlockjson.FromJSON(data)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("parsing candidate file: %w", err)}
	}

	format := opts.format
	if format == "" {
		format = "podlock"
	}

	var rendered []byte
	switch format {
	case "podlock":
		rendered, err = renderPodLock(candidate, opts)
	default:
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("unsupported --format %q (supported: podlock)", format)}
	}
	if err != nil {
		return err
	}

	if opts.out == "" {
		_, err := stdout.Write(rendered)
		return err
	}
	if err := os.WriteFile(opts.out, rendered, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", opts.out, err)
	}
	fmt.Fprintf(stdout, "Exported: %s\n", opts.out)
	return nil
}

func renderPodLock(candidate landlock.Candidate, opts exportOptions) ([]byte, error) {
	fs := profile.FilesystemProfile{Accesses: policy.FileAccessesFromCandidate(candidate)}

	result := podlock.ToProfile(podlock.ProfileMeta{
		Name:      opts.podName,
		Namespace: opts.namespace,
		Container: opts.container,
		Binary:    opts.binary,
	}, fs)

	yamlBytes, err := podlock.ToYAML(result, fs)
	if err != nil {
		return nil, fmt.Errorf("PodLock YAML serialization: %w", err)
	}
	return yamlBytes, nil
}
