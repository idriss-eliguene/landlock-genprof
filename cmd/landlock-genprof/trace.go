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
	"os"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/policy"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// traceOptions holds `trace`'s flags, passed through as-is to the rest of
// the pipeline (see runTrace).
type traceOptions struct {
	podName   string
	namespace string
	container string
	binary    string
	duration  time.Duration
	out       string
}

func newTraceCmd() *cobra.Command {
	var opts traceOptions

	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Starts a training run on a target pod and generates a Landlock profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrace(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.podName, "pod", "p", "", "Target pod name (required)")
	flags.StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	flags.StringVarP(&opts.container, "container", "c", "", "Target container (deduced if the pod has only one)")
	flags.StringVar(&opts.binary, "binary", "", "Path of the main binary observed, e.g. /usr/sbin/nginx (required)")
	flags.DurationVarP(&opts.duration, "duration", "d", 60*time.Second, "Training run duration")
	flags.StringVarP(&opts.out, "out", "o", "profile.yaml", "Output file")

	for _, name := range []string{"pod", "binary"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err) // programming error (flag doesn't exist), not a user error
		}
	}

	return cmd
}

// runTrace runs the full pipeline: pod resolution, training run, policy
// synthesis, YAML export. See docs/architecture.md §2 for the matching
// sequence diagram.
func runTrace(ctx context.Context, stdout io.Writer, opts traceOptions) error {
	client, err := newKubeClient()
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	target, err := k8s.Resolve(ctx, client, opts.namespace, opts.podName, opts.container)
	if err != nil {
		return fmt.Errorf("resolving target pod: %w", err)
	}

	events, err := tracer.Trace(tracer.Options{
		PodName:   target.PodName,
		Namespace: target.Namespace,
		Container: target.Container,
		Duration:  opts.duration,
	})
	if err != nil {
		return fmt.Errorf("training run: %w", err)
	}

	rules, err := policy.Synthesize(events)
	if err != nil {
		return fmt.Errorf("policy synthesis: %w", err)
	}

	profile := policy.ToProfile(policy.ProfileMeta{
		Name:      target.PodName,
		Namespace: target.Namespace,
		Container: target.Container,
		Binary:    opts.binary,
	}, rules)

	yamlBytes, err := policy.ToYAML(profile)
	if err != nil {
		return fmt.Errorf("YAML serialization: %w", err)
	}

	if err := os.WriteFile(opts.out, yamlBytes, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", opts.out, err)
	}

	fmt.Fprintf(stdout, "Profile generated: %s\n", opts.out)
	return nil
}

// newKubeClient tries the in-cluster config first (where the tracer will
// actually run), falling back to the local kubeconfig — useful for running
// the CLI from a dev machine.
func newKubeClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("no cluster configuration found (neither in-cluster nor %s): %w", kubeconfig, err)
		}
	}
	return kubernetes.NewForConfig(config)
}
