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

	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/networkpolicy"
	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/podlock"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/policy"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// podLockProfileLabel is the label key PodLock's own admission webhook
// looks for on a pod to know which LandlockProfile object applies to it
// — matching is done this way, by a label on the *pod* pointing at the
// LandlockProfile's name, not by anything embedded in the
// LandlockProfile CRD itself (which only carries container/binary rules,
// see pkg/podlock.LandlockProfileSpec). Applying the generated YAML alone
// has no effect until the target pod carries this label.
const podLockProfileLabel = "podlock.kubewarden.io/profile"

// traceOptions holds `trace`'s flags, passed through as-is to the rest of
// the pipeline (see runTrace).
type traceOptions struct {
	podName    string
	namespace  string
	container  string
	binary     string
	duration   time.Duration
	out        string
	networkOut string
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
	flags.StringVar(&opts.networkOut, "network-out", "", "Output file for a NetworkPolicy generated from observed connect/bind activity (skipped if unset, or if no network activity was observed)")

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

	behavior, err := policy.Synthesize(events)
	if err != nil {
		return fmt.Errorf("policy synthesis: %w", err)
	}

	result := podlock.ToProfile(podlock.ProfileMeta{
		Name:      target.PodName,
		Namespace: target.Namespace,
		Container: target.Container,
		Binary:    opts.binary,
	}, behavior.Filesystem)

	yamlBytes, err := podlock.ToYAML(result)
	if err != nil {
		return fmt.Errorf("YAML serialization: %w", err)
	}

	if err := os.WriteFile(opts.out, yamlBytes, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", opts.out, err)
	}

	fmt.Fprintf(stdout, "Profile generated: %s\n", opts.out)
	fmt.Fprintf(stdout, "For PodLock to enforce it, label the target pod: kubectl label pod %s %s=%s\n",
		target.PodName, podLockProfileLabel, target.PodName)

	if opts.networkOut != "" {
		if err := writeNetworkPolicy(stdout, opts, target, behavior); err != nil {
			return err
		}
	}

	return nil
}

// writeNetworkPolicy writes the NetworkPolicy generated from observed
// connect/bind activity to opts.networkOut, unless no network activity was
// observed — an empty NetworkPolicy would mean "deny all" (see
// networkpolicy.ToPolicy), which the CLI should never emit implicitly just
// because --network-out was passed.
func writeNetworkPolicy(stdout io.Writer, opts traceOptions, target *k8s.TargetPod, behavior profile.BehaviorProfile) error {
	if len(behavior.Network.Accesses) == 0 {
		fmt.Fprintf(stdout, "No network activity observed, skipping %s\n", opts.networkOut)
		return nil
	}

	policyResult := networkpolicy.ToPolicy(networkpolicy.PolicyMeta{
		Name:      target.PodName,
		Namespace: target.Namespace,
		PodLabels: target.Labels,
	}, behavior.Network)

	yamlBytes, err := networkpolicy.ToYAML(policyResult)
	if err != nil {
		return fmt.Errorf("NetworkPolicy YAML serialization: %w", err)
	}

	if err := os.WriteFile(opts.networkOut, yamlBytes, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", opts.networkOut, err)
	}

	fmt.Fprintf(stdout, "NetworkPolicy generated: %s\n", opts.networkOut)
	return nil
}

// newKubeClient wraps k8s.RestConfig() into a clientset for Resolve().
func newKubeClient() (kubernetes.Interface, error) {
	config, err := k8s.RestConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}
