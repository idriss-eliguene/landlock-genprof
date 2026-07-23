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
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/networkpolicy"
	"github.com/idriss-eliguene/landlock-genprof/internal/exporter/podlock"
	"github.com/idriss-eliguene/landlock-genprof/internal/history"
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
	history    bool
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
	flags.BoolVar(&opts.history, "history", false,
		"Record this run's observed accesses in a TrainingHistory custom resource, accumulating "+
			"across runs so Confidence reflects how many separate training runs actually saw each "+
			"access, not just this one. Requires the CRD and additional RBAC — see "+
			"deploy/crd-traininghistory.yaml and deploy/rbac-history.yaml. Query with: "+
			"kubectl get traininghistory <container>-<binary-basename>.")

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
	owner := k8s.OwnerNone // stays "none" (today's per-pod PodLock hint) unless --restart says otherwise
	if opts.restart {
		target, owner, events, err = traceWithRestart(ctx, stdout, client, target, opts)
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

	if opts.history {
		behavior, err = recordHistory(ctx, stdout, target, opts, behavior)
		if err != nil {
			return err
		}
	}

	result := podlock.ToProfile(podlock.ProfileMeta{
		Name:      target.PodName,
		Namespace: target.Namespace,
		Container: target.Container,
		Binary:    opts.binary,
	}, behavior.Filesystem)

	yamlBytes, err := podlock.ToYAML(result, behavior.Filesystem)
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
	fmt.Fprint(stdout, podLockLabelHint(owner, target.PodName))

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

// traceWithRestart orchestrates --restart with a single attach-first
// sequence for every owner kind: start the tracer, wait for its gadgets
// to confirm attachment (tracer.Trace's onReady), only then trigger the
// restart. What differs per owner is *what to pre-target the tracer
// with*, decided entirely before any of that starts:
//
//   - Stable name (bare pod, StatefulSet — k8s.KeepsStableName):
//     pre-target by the unchanging pod name. Inspektor Gadget's
//     KubeManager filter dynamically re-attaches to any container
//     matching it, so a tracer already listening on e.g. "nginx-demo"
//     picks up the replacement container's startup activity
//     automatically. Confirmed live for bare pods (docs/e2e-demo.md
//     Finding 2): restarting first and only then attaching reliably
//     lost the startup activity, since gadget attachment (a real gRPC
//     handshake per gadget) is slower than an already-cached image's
//     container start.
//   - Unstable name (Deployment, DaemonSet): the replacement gets an
//     unpredictable, controller-generated name, so it's pre-targeted by
//     the owning workload's label selector instead
//     (k8s.PodSelectorFor, tracer.Options.Selector) — confirmed present
//     in Inspektor Gadget's SDK, not a guess. The generated profile's
//     identity becomes the *workload's* name (e.g. "nginx-ds"), not the
//     now-about-to-be-replaced pod's: more honest about what was
//     actually captured (any replica matching the selector), and avoids
//     naming output after a pod that may no longer exist by the time
//     training finishes. This closes a real, confirmed bug: the
//     original restart-then-discover-the-new-name order for this case
//     produced a fully empty profile for a DaemonSet — the same class
//     of miss the bare-pod case had, just never fixed for unstable
//     names until now.
//
// k8s.Restart itself no longer needs to discover or report back a
// replacement's identity for any owner kind, since it's all decided
// here first — see its own doc comment.
func traceWithRestart(ctx context.Context, stdout io.Writer, client kubernetes.Interface, target *k8s.TargetPod, opts traceOptions) (*k8s.TargetPod, k8s.OwnerKind, []tracer.Event, error) {
	pod, err := client.CoreV1().Pods(target.Namespace).Get(ctx, target.PodName, metav1.GetOptions{})
	if err != nil {
		return target, k8s.OwnerNone, nil, fmt.Errorf("fetching pod %s/%s: %w", target.Namespace, target.PodName, err)
	}
	owner, ownerName, err := k8s.DetectOwner(ctx, client, target.Namespace, pod)
	if err != nil {
		return target, k8s.OwnerNone, nil, fmt.Errorf("detecting pod owner: %w", err)
	}

	traceTarget := *target
	var selectorStr string
	if !k8s.KeepsStableName(owner) {
		sel, err := k8s.PodSelectorFor(ctx, client, target.Namespace, owner, ownerName)
		if err != nil {
			return target, owner, nil, fmt.Errorf("resolving pod selector: %w", err)
		}
		selector, err := metav1.LabelSelectorAsSelector(sel)
		if err != nil {
			return target, owner, nil, fmt.Errorf("%s %s/%s has an invalid pod selector: %w", owner, target.Namespace, ownerName, err)
		}
		selectorStr = selector.String()
		traceTarget.PodName = ownerName // identity = the workload, not a soon-to-be-replaced pod
		traceTarget.Labels = sel.MatchLabels
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
			PodName:   traceTarget.PodName,
			Namespace: traceTarget.Namespace,
			Container: traceTarget.Container,
			Duration:  opts.duration,
			Binary:    opts.binary,
			Selector:  selectorStr,
		}, onReady)
		resultCh <- traceResult{events, err}
	}()

	select {
	case <-ready:
	case res := <-resultCh:
		// Trace returned (almost certainly with an error) before ever
		// signaling ready — surface that instead of restarting the pod
		// and then hanging waiting for a signal that will never come.
		return &traceTarget, owner, res.events, res.err
	}

	fmt.Fprintf(stdout, "Restarting %s to capture startup activity...\n", traceTarget.PodName)
	if err := k8s.Restart(ctx, client, target); err != nil {
		return &traceTarget, owner, nil, fmt.Errorf("restarting target: %w", err)
	}

	res := <-resultCh
	return &traceTarget, owner, res.events, res.err
}

// podLockLabelHint tells the user how to point PodLock at the generated
// profile. Labeling a single pod (today's message, and still correct
// for OwnerNone/OwnerStatefulSet, whose identity is a real, addressable
// pod) is wrong once the identity is a workload
// (OwnerDeployment/OwnerDaemonSet, see traceWithRestart): a rollout
// would replace the pod and drop the label with it, so the label needs
// to live on the pod *template* instead, propagating to every replica.
func podLockLabelHint(owner k8s.OwnerKind, name string) string {
	labelPatch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"labels":{%q:%q}}}}}`, podLockProfileLabel, name)
	switch owner {
	case k8s.OwnerDeployment:
		return fmt.Sprintf("For PodLock to enforce it, label the pod template: kubectl patch deployment %s -p '%s'\n", name, labelPatch)
	case k8s.OwnerDaemonSet:
		return fmt.Sprintf("For PodLock to enforce it, label the pod template: kubectl patch daemonset %s -p '%s'\n", name, labelPatch)
	default:
		return fmt.Sprintf("For PodLock to enforce it, label the target pod: kubectl label pod %s %s=%s\n", name, podLockProfileLabel, name)
	}
}

// recordHistory folds this run's behavior into the target's
// TrainingHistory custom resource (internal/history), creating it on
// the first `trace --history` run for this container/binary. See
// internal/history's package doc for why this exists: Confidence's own
// doc comment already claims "seen across how many distinct training
// runs" — this is what actually makes that true, instead of the
// single-run proxy internal/policy.Synthesize computes for lack of any
// persisted state.
func recordHistory(ctx context.Context, stdout io.Writer, target *k8s.TargetPod, opts traceOptions, behavior profile.BehaviorProfile) (profile.BehaviorProfile, error) {
	dynClient, err := newDynamicClient()
	if err != nil {
		return behavior, fmt.Errorf("connecting to cluster for history: %w", err)
	}

	name := history.RecordName(target.Container, opts.binary)
	existing, err := history.Get(ctx, dynClient, target.Namespace, name)
	if err != nil {
		return behavior, fmt.Errorf("reading TrainingHistory: %w", err)
	}

	record := history.Merge(existing, target.Container, opts.binary, behavior)
	if err := history.Save(ctx, dynClient, target.Namespace, name, record); err != nil {
		return behavior, fmt.Errorf("saving TrainingHistory: %w", err)
	}

	fmt.Fprintf(stdout, "History updated: %d run(s) recorded for %s (see kubectl get traininghistory %s)\n",
		record.RunsRecorded, name, name)

	// The generated YAML's Confidence comments (see
	// internal/exporter/podlock/networkpolicy's ToYAML) now reflect the
	// real cross-run ratio instead of internal/policy.Synthesize's
	// single-run proxy — the whole point of --history, see
	// docs/policy-synthesis.md.
	return history.ApplyConfidence(record, behavior), nil
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

	yamlBytes, err := networkpolicy.ToYAML(policyResult, behavior.Network)
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

// newDynamicClient wraps k8s.RestConfig() into a dynamic client for
// internal/history — TrainingHistory is a custom resource with no
// generated typed clientset, so the dynamic client (already vendored
// transitively via client-go) is what talks to it, same as
// unstructured.Unstructured everywhere else this project touches a CRD
// it doesn't own the schema of at compile time.
func newDynamicClient() (dynamic.Interface, error) {
	config, err := k8s.RestConfig()
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(config)
}
