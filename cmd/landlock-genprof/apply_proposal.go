// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
)

type applyProposalOptions struct {
	namespace string
	yes       bool
	skip      []string
	restart   bool
	// readinessTimeout bounds the wait for an external controller to
	// materialize an enforcement artifact the workload binding depends
	// on (ADR-0007). Operational, not authority: it never influences the
	// candidate digest or approval state.
	readinessTimeout time.Duration
}

// knownArtifactSlugs is every valid --skip value, for validating the
// flag up front rather than silently no-op'ing on a typo (e.g.
// --skip=podlok would otherwise apply everything, including PodLock,
// while looking like it worked).
var knownArtifactSlugs = []string{"podlock", "networkpolicy", "patched-manifest", "spo-seccompprofile"}

func newApplyProposalCmd() *cobra.Command {
	var opts applyProposalOptions

	cmd := &cobra.Command{
		Use:   "apply-proposal <proposal>",
		Short: "Reviews and applies an approved, digest-bound SecurityProfileProposal",
		Long: "Reviews and applies a published SecurityProfileProposal's artifacts. " +
			"Requires approvalState=Approved with a valid candidate digest and " +
			"candidate-v1 mechanism; fails closed before planning or applying when " +
			"that binding is missing, malformed, stale, or changed. A confirmation " +
			"prompt is additional operator confirmation." + kubectlPrefixNote,
		Example: `  # Applies PodLock/NetworkPolicy/SPO SeccompProfile if available — Patched
  # Manifest is left out unless --restart is also passed, see below
  kubectl landlock-genprof apply-proposal nginx-demo --namespace default

  # Also apply the Patched Manifest artifact, restarting the target pod
  kubectl landlock-genprof apply-proposal nginx-demo --restart

  # Skip PodLock (e.g. its operator isn't installed on this cluster)
  kubectl landlock-genprof apply-proposal nginx-demo --skip=podlock

  # Non-interactive, for CI/scripted use — still prints what it applied
  kubectl landlock-genprof apply-proposal nginx-demo --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApplyProposal(cmd.Context(), cmd.OutOrStdout(), cmd.InOrStdin(), opts, args[0])
		},
	}

	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Skip the confirmation prompt (for CI/non-interactive use); still prints what it applied")
	cmd.Flags().StringSliceVar(&opts.skip, "skip", nil,
		"Artifact(s) to leave out of this apply, comma-separated or repeated — one of: "+
			strings.Join(knownArtifactSlugs, ", ")+
			". Patched Manifest is already left out by default (see --restart); --skip=patched-manifest "+
			"is accepted but redundant with it.")
	cmd.Flags().DurationVar(&opts.readinessTimeout, "readiness-timeout", 2*time.Minute,
		"How long to wait for an external controller to make an enforcement artifact usable before "+
			"binding the workload to it — see docs/adr/0007-governed-apply-ordering-and-enforcement-readiness.md. "+
			"Applies only when the Patched Manifest is being applied and references a generated profile; "+
			"on timeout the workload binding is not applied.")
	cmd.Flags().BoolVar(&opts.restart, "restart", false,
		"Also apply the Patched Manifest artifact, if available. Opt-in, not on by default: unlike the "+
			"other three artifacts, applying it deletes and recreates the target pod outright (see "+
			"internal/k8s.applyPod) — every other artifact is either inert until its operator reconciles "+
			"it or a live-updatable resource. Confirmed live: repeatedly force-restarting a pod whose "+
			"enforcement side wasn't actually ready yet (SPO/PodLock) is how nginx-demo ended up in a "+
			"73-minute, 15-restart CrashLoopBackOff with no single moment where restarting it was an "+
			"actual decision — --skip=patched-manifest used to be the only way to avoid that, but it's "+
			"easy to not know to reach for an opt-out flag you've never needed before; an opt-in one "+
			"can't be missed by accident the same way.")
	return cmd
}

// runApplyProposal implements a hardened two-phase apply: plan/validate
// everything first, present the exact planned artifacts for confirmation,
// re-validate authorization after confirmation, then execute the plan
// sequentially. No cluster mutation occurs until after confirmation and
// re-validation.
func runApplyProposal(ctx context.Context, stdout io.Writer, stdin io.Reader, opts applyProposalOptions, proposalName string) error {
	skip, err := parseSkipArtifacts(opts.skip)
	if err != nil {
		return err
	}

	dynClient, err := newDynamicClientForApplyProposal()
	if err != nil {
		return fmt.Errorf("connecting to cluster for apply-proposal: %w", err)
	}

	spec, err := proposal.Get(ctx, dynClient, opts.namespace, proposalName)
	if err != nil {
		return err
	}
	if spec == nil {
		return fmt.Errorf("securityprofileproposal %s/%s not found", opts.namespace, proposalName)
	}

	status, err := proposal.GetStatus(ctx, dynClient, opts.namespace, proposalName)
	if err != nil {
		return fmt.Errorf("could not fetch approval status: %w", err)
	}

	// Enforce apply-time approval binding before preparing or mutating
	// any external state. Fail-closed if validation fails.
	if err := proposal.ValidateApprovedCandidate(spec, status); err != nil {
		return fmt.Errorf("apply preflight failed: %w", err)
	}

	artifacts := proposalArtifacts(spec)
	printProposalSummary(stdout, opts.namespace, proposalName, spec, status, artifacts)

	var toApply, skipped, needsRestartFlag []proposalArtifact
	for _, artifact := range artifacts {
		if !artifact.available {
			continue
		}
		if skip[artifact.slug] {
			skipped = append(skipped, artifact)
			continue
		}
		if artifact.slug == patchedManifestSlug && !opts.restart {
			needsRestartFlag = append(needsRestartFlag, artifact)
			continue
		}
		toApply = append(toApply, artifact)
	}

	if len(skipped) > 0 {
		fmt.Fprintf(stdout, "\nSkipping %d artifact(s), per --skip:\n", len(skipped))
		for _, artifact := range skipped {
			fmt.Fprintf(stdout, "  - %s\n", artifact.name)
		}
	}

	if len(needsRestartFlag) > 0 {
		fmt.Fprintf(stdout, "\nLeaving out %d artifact(s) that would restart the target pod — pass --restart to include:\n", len(needsRestartFlag))
		for _, artifact := range needsRestartFlag {
			fmt.Fprintf(stdout, "  - %s\n", artifact.name)
		}
	}

	if len(toApply) == 0 {
		switch {
		case len(skipped) > 0 || len(needsRestartFlag) > 0:
			fmt.Fprintln(stdout, "\nNothing left to apply — every available artifact was skipped or left out.")
		default:
			fmt.Fprintln(stdout, "\nNo artifacts to apply — this proposal generated nothing (empty training run?).")
		}
		return nil
	}

	fmt.Fprintf(stdout, "\nThis will apply %d artifact(s):\n", len(toApply))
	for _, artifact := range toApply {
		fmt.Fprintf(stdout, "  - %s\n", artifact.name)
	}
	fmt.Fprintln(stdout)

	// Phase 1: Build the plan by parsing and validating every selected
	// artifact. Fail-closed on any parsing/validation error before mutating
	// the cluster.
	initialDigest, err := proposal.CandidateDigest(*spec)
	if err != nil {
		return fmt.Errorf("computing candidate digest for preflight: %w", err)
	}

	var plan []plannedArtifact
	for _, artifact := range toApply {
		pa, err := buildPlannedArtifact(artifact, opts.namespace)
		if err != nil {
			return fmt.Errorf("apply preflight failed for %s: %w", artifact.name, err)
		}
		plan = append(plan, pa)
	}

	// Phase 2: Duplicate target detection
	seen := make(map[string][]string)
	for _, p := range plan {
		id := fmt.Sprintf("%s/%s/%s/%s", p.gvk.Group, p.gvk.Version, p.gvk.Kind, p.ns+"/"+p.nameStr)
		seen[id] = append(seen[id], p.slug)
	}
	var dupErrors []string
	for id, slugs := range seen {
		if len(slugs) > 1 {
			dupErrors = append(dupErrors, fmt.Sprintf("%s -> %v", id, slugs))
		}
	}
	if len(dupErrors) > 0 {
		return fmt.Errorf("duplicate target artifacts detected: %s", strings.Join(dupErrors, "; "))
	}

	// Order the plan by dependency, not by declaration order: enforcement
	// artifacts an external controller must reconcile first, independent
	// live policy next, and the workload-binding artifact last. See
	// docs/adr/0007-governed-apply-ordering-and-enforcement-readiness.md —
	// applying the binding artifact before the profile it references is
	// what produces a workload that cannot start.
	sort.SliceStable(plan, func(i, j int) bool {
		return applyClassFor(plan[i].slug) < applyClassFor(plan[j].slug)
	})

	// What the binding artifact needs before it can be applied. Derived
	// from the approved candidate, so a --skip'd SeccompProfile the
	// workload still references is checked against the live cluster
	// rather than silently assumed present.
	readinessReqs := enforcementRequirements(plan, approvedSeccompProfile(artifacts, opts.namespace))

	if afterApplyProposalPlanBuilt != nil {
		afterApplyProposalPlanBuilt()
	}

	// Present the exact planned artifacts (GVK + ns/name) for confirmation.
	fmt.Fprintln(stdout, "Planned artifacts:")
	for _, p := range plan {
		fmt.Fprintf(stdout, "  - %s: %s %s/%s\n", p.name, p.gvk.String(), p.ns, p.nameStr)
	}
	fmt.Fprintln(stdout)

	if !opts.yes {
		fmt.Fprint(stdout, "Apply these planned artifacts to the cluster? [y/N] ")
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(stdout, "Aborted — nothing applied.")
			return nil
		}
	}

	// Re-validate authorization and candidate binding immediately before the
	// first mutation. Fail-closed on any change.
	currentSpec, err := proposal.Get(ctx, dynClient, opts.namespace, proposalName)
	if err != nil {
		return fmt.Errorf("re-reading proposal before apply: %w", err)
	}
	currentStatus, err := proposal.GetStatus(ctx, dynClient, opts.namespace, proposalName)
	if err != nil {
		return fmt.Errorf("re-reading proposal status before apply: %w", err)
	}
	if err := proposal.ValidateApprovedCandidate(currentSpec, currentStatus); err != nil {
		return fmt.Errorf("authorization changed before apply: %w", err)
	}
	currentDigest, err := proposal.CandidateDigest(*currentSpec)
	if err != nil {
		return fmt.Errorf("computing candidate digest before apply: %w", err)
	}
	if currentDigest != initialDigest {
		return fmt.Errorf("candidate changed since plan creation; aborting")
	}

	// Phase 3: execute the plan in dependency order, fail-stop. A failure
	// stops the sequence — artifacts already applied remain (this is not
	// transactional and does not pretend to be), but nothing further is
	// applied, and the binding artifact in particular is never reached.
	for _, p := range plan {
		if applyClassFor(p.slug) == classBinding {
			// Everything the binding depends on has been applied; prove it
			// is actually usable, and that authority still holds, before
			// touching the workload.
			if err := waitForEnforcementReady(ctx, stdout, dynClient, readinessReqs, opts.readinessTimeout); err != nil {
				return err
			}
			if afterEnforcementReady != nil {
				afterEnforcementReady()
			}
			if err := revalidateBeforeBinding(ctx, dynClient, opts.namespace, proposalName, initialDigest); err != nil {
				return err
			}
		}

		if err := applyManifest(ctx, dynClient, p.ns, p.content); err != nil {
			fmt.Fprintf(stdout, "failed: %s — %v\n", p.name, err)
			return fmt.Errorf("apply-proposal: %s failed to apply; stopping before any further artifact: %w", p.name, err)
		}
		fmt.Fprintf(stdout, "applied: %s\n", p.name)
	}

	fmt.Fprintln(stdout, "\nDone.")
	return nil
}

// revalidateBeforeBinding is ADR-0007's third authority gate. The
// readiness wait above can take as long as an external controller takes,
// which is a window Gate 2 closed before it existed: an approval revoked
// during that wait must not still authorize recreating the workload.
func revalidateBeforeBinding(ctx context.Context, dynClient dynamic.Interface, namespace, proposalName, plannedDigest string) error {
	currentSpec, err := proposal.Get(ctx, dynClient, namespace, proposalName)
	if err != nil {
		return fmt.Errorf("re-reading proposal before workload binding: %w", err)
	}
	if currentSpec == nil {
		return fmt.Errorf("securityprofileproposal %s/%s disappeared before workload binding", namespace, proposalName)
	}
	currentStatus, err := proposal.GetStatus(ctx, dynClient, namespace, proposalName)
	if err != nil {
		return fmt.Errorf("re-reading proposal status before workload binding: %w", err)
	}
	if err := proposal.ValidateApprovedCandidate(currentSpec, currentStatus); err != nil {
		return fmt.Errorf("authorization changed before workload binding: %w", err)
	}
	currentDigest, err := proposal.CandidateDigest(*currentSpec)
	if err != nil {
		return fmt.Errorf("computing candidate digest before workload binding: %w", err)
	}
	if currentDigest != plannedDigest {
		return fmt.Errorf("candidate changed during enforcement readiness wait; workload binding not applied")
	}
	return nil
}

// approvedSeccompProfile parses the proposal's own SeccompProfile
// artifact, whether or not this run is applying it: it is the approved
// enforcement content the readiness gate compares the live object
// against.
func approvedSeccompProfile(artifacts []proposalArtifact, fallbackNamespace string) *unstructured.Unstructured {
	for _, artifact := range artifacts {
		if artifact.slug != "spo-seccompprofile" || !artifact.available {
			continue
		}
		pa, err := buildPlannedArtifact(artifact, fallbackNamespace)
		if err != nil {
			return nil
		}
		return pa.obj
	}
	return nil
}

// parseSkipArtifacts validates --skip against knownArtifactSlugs up
// front and returns a lookup set — failing fast on a typo (e.g.
// --skip=podlok) matters here specifically: silently ignoring an
// unrecognized slug would mean the command applies *everything*,
// including whatever the caller meant to exclude, while still looking
// like the flag was honored.
func parseSkipArtifacts(skip []string) (map[string]bool, error) {
	known := make(map[string]bool, len(knownArtifactSlugs))
	for _, s := range knownArtifactSlugs {
		known[s] = true
	}

	result := make(map[string]bool, len(skip))
	for _, raw := range skip {
		s := strings.ToLower(strings.TrimSpace(raw))
		if !known[s] {
			return nil, fmt.Errorf("--skip=%q: not a known artifact — one of: %s",
				raw, strings.Join(knownArtifactSlugs, ", "))
		}
		result[s] = true
	}
	return result, nil
}

// newDynamicClientForApplyProposal is a test seam, same pattern as
// review.go's newDynamicClientForReview / trace.go's
// newDynamicClientForProposal.
// plannedArtifact is a preflight representation of an artifact ready to apply.
type plannedArtifact struct {
	slug    string
	name    string
	content string
	gvk     schema.GroupVersionKind
	ns      string
	nameStr string
	obj     *unstructured.Unstructured
}

// allowed GVKs — must match internal/k8s.applyGVRs
var allowedGVKs = []schema.GroupVersionKind{
	{Group: "podlock.kubewarden.io", Version: "v1alpha1", Kind: "LandlockProfile"},
	{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
	spobackend.SeccompProfileGVK(),
	{Version: "v1", Kind: "Pod"},
	{Group: "apps", Version: "v1", Kind: "Deployment"},
	{Group: "apps", Version: "v1", Kind: "StatefulSet"},
	{Group: "apps", Version: "v1", Kind: "DaemonSet"},
}

func isAllowedGVK(gvk schema.GroupVersionKind) bool {
	for _, a := range allowedGVKs {
		if a.Group == gvk.Group && a.Version == gvk.Version && a.Kind == gvk.Kind {
			return true
		}
	}
	return false
}

// buildPlannedArtifact parses and validates the YAML content without mutating
// the cluster. It rejects multi-document YAML and missing metadata.
func buildPlannedArtifact(a proposalArtifact, fallbackNamespace string) (plannedArtifact, error) {
	var pa plannedArtifact
	pa.slug = a.slug
	pa.name = a.name
	pa.content = a.content

	// Detect multi-doc and parse exactly one document.
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(a.content)), 4096)
	var raw map[string]interface{}
	if err := dec.Decode(&raw); err != nil {
		return pa, fmt.Errorf("parsing manifest: %w", err)
	}
	// Check for extra document
	var extra map[string]interface{}
	if err := dec.Decode(&extra); err == nil {
		return pa, fmt.Errorf("multi-document YAML is not supported")
	} else if err != io.EOF {
		return pa, fmt.Errorf("parsing manifest: %w", err)
	}

	if raw == nil || len(raw) == 0 {
		return pa, fmt.Errorf("empty manifest")
	}

	obj := &unstructured.Unstructured{Object: raw}
	gvk := obj.GroupVersionKind()
	if gvk.Kind == "" {
		return pa, fmt.Errorf("manifest missing kind/apiVersion")
	}
	if !isAllowedGVK(gvk) {
		return pa, fmt.Errorf("unrecognized resource kind %q (apiVersion %q)", gvk.Kind, gvk.GroupVersion())
	}

	name := obj.GetName()
	if name == "" {
		return pa, fmt.Errorf("manifest missing metadata.name")
	}
	ns := obj.GetNamespace()
	if ns == "" {
		ns = fallbackNamespace
		obj.SetNamespace(ns)
	}
	// basic namespace validation: non-empty string only
	if ns == "" {
		return pa, fmt.Errorf("effective namespace empty")
	}

	pa.gvk = gvk
	pa.ns = ns
	pa.nameStr = name
	pa.obj = obj
	return pa, nil
}

var newDynamicClientForApplyProposal func() (dynamic.Interface, error) = newDynamicClient

// afterApplyProposalPlanBuilt is a test seam invoked after planned artifacts are
// built (T5) and before proposal reload/revalidation (T7/T8/T9).
var afterApplyProposalPlanBuilt func()

// applyManifest is a test seam for applying one prebuilt artifact payload.
var applyManifest = k8s.Apply
