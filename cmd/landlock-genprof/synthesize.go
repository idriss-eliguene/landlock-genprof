// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// synthesize is the first slice of the CLI's own longer-term "evidence
// -> synthesis -> verification -> ..." lifecycle (docs/cli-design.md)
// existing as a real, separate verb rather than only ever running as
// `trace`'s implicit last step. Deliberately minimal, not a full
// re-implementation of everything `trace` can produce: PodLock profile
// and candidate JSON only — every other artifact `trace` can write
// (NetworkPolicy, seccomp, capabilities, securityContext, patched
// manifest, report) or side effect it can perform (history recording,
// SecurityProfileProposal publishing) needs a live cluster connection
// this command doesn't have, by design. Widening this scope is a later,
// separate decision, not assumed here.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/idriss-eliguene/landlock-genprof/internal/evidence"
	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/podlock"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/policy"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

type synthesizeOptions struct {
	eventsFile   string
	podName      string
	namespace    string
	container    string
	binary       string
	out          string
	candidateOut string
}

func newSynthesizeCmd() *cobra.Command {
	var opts synthesizeOptions

	cmd := &cobra.Command{
		Use:   "synthesize --events-file <path> --pod <name> --container <name> --binary <path>",
		Short: "Re-runs synthesis offline from previously captured evidence",
		Long: "Re-runs synthesis from a training run's persisted evidence " +
			"(see `trace --events-out`), without re-tracing — produces the " +
			"same PodLock profile and candidate JSON `trace` writes inline. " +
			"Deliberately minimal today: unlike `trace`, this doesn't write " +
			"NetworkPolicy/seccomp/capabilities/securityContext/report, " +
			"doesn't record history, and doesn't publish a " +
			"SecurityProfileProposal — those all need a live cluster " +
			"connection this command doesn't have." + kubectlPrefixNote,
		Example: `  kubectl landlock-genprof synthesize --events-file nginx-demo-events.json \
    --pod nginx-demo --namespace default --container nginx --binary /usr/sbin/nginx --candidate-out`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSynthesize(cmd.OutOrStdout(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.eventsFile, "events-file", "", "Path to an evidence JSON file (see trace --events-out) (required)")
	flags.StringVarP(&opts.podName, "pod", "p", "", "Pod name to label the generated profile with (required)")
	flags.StringVarP(&opts.namespace, "namespace", "n", "default", "Namespace to label the generated profile with")
	flags.StringVarP(&opts.container, "container", "c", "", "Container name to label the generated profile with (required)")
	flags.StringVar(&opts.binary, "binary", "", "Binary path to label the generated profile with (required)")
	flags.StringVarP(&opts.out, "out", "o", "", "Output file for the generated LandlockProfile (default: <pod>-profile.yaml)")
	flags.StringVar(&opts.candidateOut, "candidate-out", "",
		"Output file for the raw candidate JSON (default <pod>-candidate.json); this is what `verify` reads")
	flags.Lookup("candidate-out").NoOptDefVal = autoFilenameSentinel

	for _, name := range []string{"events-file", "pod", "container", "binary"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err) // programming error (flag doesn't exist), not a user error
		}
	}

	return cmd
}

func runSynthesize(stdout io.Writer, opts synthesizeOptions) error {
	data, err := os.ReadFile(opts.eventsFile)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("reading events file: %w", err)}
	}
	events, architectures, err := evidence.FromJSON(data)
	if err != nil {
		return &exitCodeError{code: 3, wrapped: fmt.Errorf("parsing events file: %w", err)}
	}

	// convert raw tracer events to normalized observations for policy
	observations := make([]observation.Observation, 0, len(events))
	for _, ev := range events {
		observations = append(observations, tracer.ToObservation(ev))
	}

	behavior, err := policy.Synthesize(observations, architectures)
	if err != nil {
		return fmt.Errorf("policy synthesis: %w", err)
	}

	result := podlock.ToProfile(podlock.ProfileMeta{
		Name:      opts.podName,
		Namespace: opts.namespace,
		Container: opts.container,
		Binary:    opts.binary,
	}, behavior.Filesystem)

	yamlBytes, err := podlock.ToYAML(result, behavior.Filesystem)
	if err != nil {
		return fmt.Errorf("YAML serialization: %w", err)
	}

	out := opts.out
	if out == "" {
		out = defaultOutFile(opts.podName)
	}
	if err := os.WriteFile(out, yamlBytes, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}
	fmt.Fprintf(stdout, "Profile generated: %s\n", out)

	if opts.candidateOut != "" {
		candidateOut := opts.candidateOut
		if candidateOut == autoFilenameSentinel {
			candidateOut = defaultCandidateOutFile(opts.podName)
		}
		if err := writeCandidateJSON(stdout, candidateOut, events); err != nil {
			return err
		}
	}

	return nil
}
