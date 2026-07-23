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
	"sync"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// autoFilenameSentinel is the value --network-out takes when the flag is
// given with no argument (`--network-out` alone, via NoOptDefVal below):
// opt into NetworkPolicy generation without having to name the file
// yourself, computed instead from the traced pod's name (see
// defaultOutFile/defaultNetworkOutFile). Distinct from "" (flag omitted
// entirely, meaning "don't generate a NetworkPolicy at all") — "" and "a
// value was given with no name" have to stay distinguishable, hence the
// sentinel rather than reusing "".
const autoFilenameSentinel = "-"

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
	restart    bool
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
	flags.StringVarP(&opts.out, "out", "o", "", "Output file for the generated LandlockProfile (default: <pod>-profile.yaml)")
	flags.StringVar(&opts.networkOut, "network-out", "",
		"Output file for a NetworkPolicy generated from observed connect/bind activity "+
			"(skipped entirely if this flag is omitted, or if no network activity was observed; "+
			"pass with no filename for the default <pod>-networkpolicy.yaml)")
	flags.Lookup("network-out").NoOptDefVal = autoFilenameSentinel
	flags.BoolVar(&opts.restart, "restart", false,
		"Restart the target pod (delete+recreate a bare pod, or trigger a rollout restart for a "+
			"Deployment-owned pod) right before tracing, to capture startup-time file opens "+
			"(pid files, log fds) invisible to a trace attached to an already-running container. "+
			"Requires additional RBAC — see deploy/rbac-restart.yaml. Disruptive: this restarts "+
			"the target workload.")

	for _, name := range []string{"pod", "binary"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err) // programming error (flag doesn't exist), not a user error
		}
	}

	return cmd
}

// defaultOutFile and defaultNetworkOutFile compute the pod-based default
// filenames used when --out/--network-out weren't given an explicit
// value — see autoFilenameSentinel's comment.
func defaultOutFile(podName string) string {
	return fmt.Sprintf("%s-profile.yaml", podName)
}

func defaultNetworkOutFile(podName string) string {
	return fmt.Sprintf("%s-networkpolicy.yaml", podName)
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

	var events []tracer.Event
	if opts.restart {
		target, events, err = traceWithRestart(ctx, stdout, client, target, opts)
	} else {
		events, err = tracer.Trace(tracer.Options{
			PodName:   target.PodName,
			Namespace: target.Namespace,
			Container: target.Container,
			Duration:  opts.duration,
			Binary:    opts.binary,
		}, nil)
	}
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

	out := opts.out
	if out == "" {
		out = defaultOutFile(target.PodName)
	}

	if err := os.WriteFile(out, yamlBytes, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}

	fmt.Fprintf(stdout, "Profile generated: %s\n", out)
	fmt.Fprintf(stdout, "For PodLock to enforce it, label the target pod: kubectl label pod %s %s=%s\n",
		target.PodName, podLockProfileLabel, target.PodName)

	if opts.networkOut != "" {
		networkOut := opts.networkOut
		if networkOut == autoFilenameSentinel {
			networkOut = defaultNetworkOutFile(target.PodName)
		}
		if err := writeNetworkPolicy(stdout, networkOut, target, behavior); err != nil {
			return err
		}
	}

	return nil
}

// traceWithRestart orchestrates --restart. For a bare pod, the tracer is
// started *first* and only restarted once its gadgets have confirmed
// they're attached (tracer.Trace's onReady): Inspektor Gadget's
// KubeManager filter dynamically re-attaches to any container matching
// the same pod name, so a tracer already listening on "nginx-demo" picks
// up the replacement container's startup activity automatically. The
// reverse order — restart, then attach — reliably loses that activity:
// confirmed live, an already-cached image's container starts (and nginx
// finishes its one-time startup opens) faster than the tracer's own gRPC
// gadget-attachment handshake completes. See docs/e2e-demo.md Finding 2.
//
// A Deployment-owned pod's replacement gets an unpredictable,
// controller-generated name that can't be pre-targeted this way, so it
// keeps the simpler restart-then-trace order — same residual timing gap
// this fix closes for bare pods only.
func traceWithRestart(ctx context.Context, stdout io.Writer, client kubernetes.Interface, target *k8s.TargetPod, opts traceOptions) (*k8s.TargetPod, []tracer.Event, error) {
	pod, err := client.CoreV1().Pods(target.Namespace).Get(ctx, target.PodName, metav1.GetOptions{})
	if err != nil {
		return target, nil, fmt.Errorf("fetching pod %s/%s: %w", target.Namespace, target.PodName, err)
	}
	owner, _, err := k8s.DetectOwner(ctx, client, target.Namespace, pod)
	if err != nil {
		return target, nil, fmt.Errorf("detecting pod owner: %w", err)
	}

	if owner != k8s.OwnerNone {
		fmt.Fprintf(stdout, "Restarting pod %s to capture startup activity...\n", target.PodName)
		newTarget, err := k8s.Restart(ctx, client, target)
		if err != nil {
			return target, nil, fmt.Errorf("restarting target pod: %w", err)
		}
		fmt.Fprintf(stdout, "Tracing replacement pod %s\n", newTarget.PodName)

		events, err := tracer.Trace(tracer.Options{
			PodName:   newTarget.PodName,
			Namespace: newTarget.Namespace,
			Container: newTarget.Container,
			Duration:  opts.duration,
			Binary:    opts.binary,
		}, nil)
		return newTarget, events, err
	}

	type traceResult struct {
		events []tracer.Event
		err    error
	}
	resultCh := make(chan traceResult, 1)
	ready := make(chan struct{})
	var closeReadyOnce sync.Once
	onReady := func() { closeReadyOnce.Do(func() { close(ready) }) }

	go func() {
		events, err := tracer.Trace(tracer.Options{
			PodName:   target.PodName,
			Namespace: target.Namespace,
			Container: target.Container,
			Duration:  opts.duration,
			Binary:    opts.binary,
		}, onReady)
		resultCh <- traceResult{events, err}
	}()

	select {
	case <-ready:
	case res := <-resultCh:
		// Trace returned (almost certainly with an error) before ever
		// signaling ready — surface that instead of restarting the pod
		// and then hanging waiting for a signal that will never come.
		return target, res.events, res.err
	}

	fmt.Fprintf(stdout, "Restarting pod %s to capture startup activity...\n", target.PodName)
	if _, err := k8s.Restart(ctx, client, target); err != nil {
		return target, nil, fmt.Errorf("restarting target pod: %w", err)
	}

	res := <-resultCh
	return target, res.events, res.err
}

// writeNetworkPolicy writes the NetworkPolicy generated from observed
// connect/bind activity to out, unless no network activity was observed
// — an empty NetworkPolicy would mean "deny all" (see
// networkpolicy.ToPolicy), which the CLI should never emit implicitly just
// because --network-out was passed.
func writeNetworkPolicy(stdout io.Writer, out string, target *k8s.TargetPod, behavior profile.BehaviorProfile) error {
	if len(behavior.Network.Accesses) == 0 {
		fmt.Fprintf(stdout, "No network activity observed, skipping %s\n", out)
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

	if err := os.WriteFile(out, yamlBytes, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}

	fmt.Fprintf(stdout, "NetworkPolicy generated: %s\n", out)
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
