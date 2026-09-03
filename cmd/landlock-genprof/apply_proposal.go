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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
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
	return runApplyProposalInternal(ctx, stdout, stdin, opts, proposalName, false)
}

// runApplyProposalInternal is shared with the in-package certification
// harness.  The certification-only path is never exposed by the CLI and
// still performs the complete approved-plan validation and readiness checks.
func runApplyProposalInternal(ctx context.Context, stdout io.Writer, stdin io.Reader, opts applyProposalOptions, proposalName string, certification bool) error {
	skip, err := parseSkipArtifacts(opts.skip)
	if err != nil {
		return err
	}

	dynClient, err := newDynamicClientForApplyProposal()
	if err != nil {
		return fmt.Errorf("connecting to cluster for apply-proposal: %w", err)
	}

	spec, proposalUID, err := proposal.GetWithIdentity(ctx, dynClient, opts.namespace, proposalName)
	if err != nil {
		return err
	}
	if spec == nil {
		return fmt.Errorf("securityprofileproposal %s/%s not found", opts.namespace, proposalName)
	}
	if proposalUID == "" {
		return fmt.Errorf("securityprofileproposal %s/%s has no Kubernetes UID; apply custody cannot be established", opts.namespace, proposalName)
	}
	if spec.TargetBinding == nil {
		return fmt.Errorf("securityprofileproposal %s/%s has no canonical target binding; apply custody cannot be established", opts.namespace, proposalName)
	}
	target, err := spec.TargetBinding.GovernedTarget(spec.Container)
	if err != nil {
		return fmt.Errorf("invalid canonical target binding: %w", err)
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
		if err := alignBindingWithArtifactPlan(&pa, skip); err != nil {
			return fmt.Errorf("apply preflight failed for %s: %w", artifact.name, err)
		}
		plan = append(plan, pa)
	}
	if !certification {
		if err := validateCompositionCompatibility(plan); err != nil {
			return fmt.Errorf("apply preflight failed: %w", err)
		}
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
	currentSpec, currentProposalUID, err := proposal.GetWithIdentity(ctx, dynClient, opts.namespace, proposalName)
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
	if currentProposalUID != proposalUID {
		return fmt.Errorf("proposal identity changed before apply: original UID=%s current UID=%s", proposalUID, currentProposalUID)
	}
	if currentSpec.TargetBinding == nil {
		return fmt.Errorf("canonical target binding disappeared before apply")
	}
	currentTarget, err := currentSpec.TargetBinding.GovernedTarget(currentSpec.Container)
	if err != nil || !target.Equal(currentTarget) {
		return fmt.Errorf("canonical target binding changed before apply")
	}
	currentDigest, err := proposal.CandidateDigest(*currentSpec)
	if err != nil {
		return fmt.Errorf("computing candidate digest before apply: %w", err)
	}
	if currentDigest != initialDigest {
		return fmt.Errorf("candidate changed since plan creation; aborting")
	}

	attemptSpec := attempt.Spec{
		ProposalNamespace: opts.namespace, ProposalName: proposalName, ProposalUID: proposalUID,
		ApprovedCandidateDigest: currentStatus.ApprovedCandidateDigest,
		Target:                  target, StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	attemptID, attemptObj, err := createAttempt(ctx, dynClient, opts.namespace, attemptSpec)
	if err != nil {
		return fmt.Errorf("apply preflight failed: durable ApplyAttempt creation is required before mutation: %w", err)
	}
	attemptStatus := attempt.Status{State: attempt.StateInProgress}
	persistStatus := func() error {
		return saveAttemptStatus(ctx, dynClient, opts.namespace, attemptID, attemptObj, attemptStatus)
	}
	if err := persistStatus(); err != nil {
		return fmt.Errorf("apply preflight failed: durable ApplyAttempt IN_PROGRESS status could not be established; no mutation executed: %w", err)
	}
	markFailure := func(state, reason string) error {
		attemptStatus.State = state
		attemptStatus.Failure = reason
		if hasSuccessfulMutation(attemptStatus.Mutations) && state == attempt.StateFailed {
			attemptStatus.State = attempt.StatePartiallyApplied
		}
		if err := persistStatus(); err != nil {
			return fmt.Errorf("persisting ApplyAttempt failure: %w (original: %s)", err, reason)
		}
		return fmt.Errorf("%s", reason)
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
			if err := validatePodLockBeforeBinding(ctx, dynClient, bindingArtifacts(artifacts, skip), p.obj, spec.Container, spec.Binary, opts.namespace); err != nil {
				return err
			}
			if afterEnforcementReady != nil {
				afterEnforcementReady()
			}
			if err := revalidateBeforeBinding(ctx, dynClient, opts.namespace, proposalName, initialDigest); err != nil {
				return markFailure(attempt.StateFailed, err.Error())
			}
		}

		if err := revalidateAttemptBeforeMutation(ctx, dynClient, opts.namespace, proposalName, proposalUID, target, initialDigest); err != nil {
			return markFailure(attempt.StateFailed, err.Error())
		}
		before, err := readApplyResource(ctx, dynClient, p.ns, p.obj)
		if err != nil {
			return markFailure(attempt.StateFailed, fmt.Sprintf("reading %s before mutation: %v", p.name, err))
		}
		record := mutationRecordFor(p, before)
		attemptStatus.Mutations = append(attemptStatus.Mutations, record)
		if err := persistStatus(); err != nil {
			attemptStatus.Mutations = attemptStatus.Mutations[:len(attemptStatus.Mutations)-1]
			reason := fmt.Sprintf("ApplyAttempt pre-state persistence failed for %s; mutation not executed: %v", p.name, err)
			attemptStatus.Failure = reason
			if hasSuccessfulMutation(attemptStatus.Mutations) {
				attemptStatus.State = attempt.StatePartiallyApplied
			} else {
				attemptStatus.State = attempt.StateFailed
			}
			_ = persistStatus()
			return errors.New(reason)
		}
		guard := k8s.ApplyGuard{Present: before != nil}
		if before != nil {
			guard.UID = before.GetUID()
			guard.ResourceVersion = before.GetResourceVersion()
		}
		currentApplyGuard = &guard
		err = applyManifest(ctx, dynClient, p.ns, p.content)
		currentApplyGuard = nil
		if err != nil {
			after, readErr := readApplyResource(ctx, dynClient, p.ns, p.obj)
			if readErr != nil {
				record.Result = attempt.ResultUnknown
				record.Error = err.Error() + "; outcome read failed: " + readErr.Error()
				attemptStatus.Mutations[len(attemptStatus.Mutations)-1] = record
				attemptStatus.State = attempt.StateOutcomeUnknown
				_ = persistStatus()
				return fmt.Errorf("apply-proposal: %s outcome unknown: %w", p.name, err)
			}
			record.Result = attempt.ResultFailed
			record.Error = err.Error()
			record.ObservedAfter, record.ObservedAfterDigest = observedMutationValue(p.gvk, after)
			if record.Before != record.ObservedAfter {
				record.Result = attempt.ResultUnknown
				record.Error = err.Error() + "; live state differs from the durable pre-state"
			}
			attemptStatus.Mutations[len(attemptStatus.Mutations)-1] = record
			if saveErr := persistStatus(); saveErr != nil {
				attemptStatus.State = attempt.StateOutcomeUnknown
				attemptStatus.Failure = saveErr.Error()
				_ = persistStatus()
				return fmt.Errorf("apply-proposal: %s failed and outcome custody failed: %w", p.name, saveErr)
			}
			if record.Result == attempt.ResultUnknown {
				attemptStatus.State = attempt.StateOutcomeUnknown
				_ = persistStatus()
				return fmt.Errorf("apply-proposal: %s outcome unknown after failed mutation: %w", p.name, err)
			}
			fmt.Fprintf(stdout, "failed: %s — %v\n", p.name, err)
			return markFailure(attempt.StateFailed, fmt.Sprintf("apply-proposal: %s failed to apply; stopping before any further artifact: %v", p.name, err))
		}
		after, readErr := readApplyResource(ctx, dynClient, p.ns, p.obj)
		if readErr != nil {
			record.Result = attempt.ResultUnknown
			record.Error = readErr.Error()
			attemptStatus.Mutations[len(attemptStatus.Mutations)-1] = record
			attemptStatus.State = attempt.StateOutcomeUnknown
			_ = persistStatus()
			return fmt.Errorf("apply-proposal: %s applied but outcome is unknown: %w", p.name, readErr)
		}
		if after == nil {
			record.Result = attempt.ResultUnknown
			record.Error = "live object was not found after a successful mutation response"
			attemptStatus.Mutations[len(attemptStatus.Mutations)-1] = record
			attemptStatus.State = attempt.StateOutcomeUnknown
			_ = persistStatus()
			return fmt.Errorf("apply-proposal: %s applied but live outcome is unknown", p.name)
		}
		record.Result = attempt.ResultSucceeded
		record.ObservedAfter, record.ObservedAfterDigest = observedMutationValue(p.gvk, after)
		attemptStatus.Mutations[len(attemptStatus.Mutations)-1] = record
		if err := persistStatus(); err != nil {
			attemptStatus.State = attempt.StateOutcomeUnknown
			attemptStatus.Failure = err.Error()
			_ = persistStatus()
			return fmt.Errorf("apply-proposal: %s succeeded but observed outcome custody failed: %w", p.name, err)
		}
		fmt.Fprintf(stdout, "applied: %s\n", p.name)
	}

	attemptStatus.State = attempt.StateApplied
	attemptStatus.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := persistStatus(); err != nil {
		return fmt.Errorf("apply-proposal completed but ApplyAttempt finalization failed: %w", err)
	}
	fmt.Fprintln(stdout, "\nDone.")
	return nil
}

// validateCompositionCompatibility rejects compositions whose bootstrap
// requirements cannot be separated from application syscall authority. The
// selected plan (rather than proposal contents) is authoritative, and this
// check runs before the first mutation.
func validateCompositionCompatibility(plan []plannedArtifact) error {
	var podlock, seccomp bool
	for _, p := range plan {
		podlock = podlock || p.slug == "podlock"
		seccomp = seccomp || p.slug == "spo-seccompprofile"
	}
	if podlock && seccomp {
		return fmt.Errorf("PodLock + application-derived Seccomp composition is unsupported: runtime compatibility is unproven")
	}
	return nil
}

// alignBindingWithArtifactPlan removes only references to an enforcement
// artifact the operator explicitly excluded. The approved proposal remains
// unchanged; this is the concrete execution plan shown and applied by this
// invocation.
func alignBindingWithArtifactPlan(p *plannedArtifact, skip map[string]bool) error {
	if p == nil || p.slug != patchedManifestSlug {
		return nil
	}
	changed := false
	if skip["podlock"] {
		for _, path := range [][]string{
			{"metadata", "labels", podLockProfileLabel},
			{"spec", "template", "metadata", "labels", podLockProfileLabel},
		} {
			unstructured.RemoveNestedField(p.obj.Object, path...)
		}
		changed = true
	}
	if skip["spo-seccompprofile"] {
		removeSeccompBindings(p.obj.Object)
		changed = true
	}
	if !changed {
		return nil
	}
	content, err := json.Marshal(p.obj.Object)
	if err != nil {
		return fmt.Errorf("serializing binding without skipped PodLock reference: %w", err)
	}
	p.content = string(content)
	return nil
}

// removeSeccompBindings removes only securityContext.seccompProfile fields
// from a patched workload when the corresponding enforcement artifact is
// explicitly skipped. Other security context fields and all metadata remain
// untouched; a skipped artifact must never leave a dangling workload binding.
func removeSeccompBindings(obj map[string]interface{}) {
	var walk func(map[string]interface{})
	walk = func(m map[string]interface{}) {
		for key, value := range m {
			switch child := value.(type) {
			case map[string]interface{}:
				if key == "securityContext" {
					delete(child, "seccompProfile")
				}
				walk(child)
			case []interface{}:
				for _, item := range child {
					if nested, ok := item.(map[string]interface{}); ok {
						walk(nested)
					}
				}
			}
		}
	}
	walk(obj)
}

func bindingArtifacts(artifacts []proposalArtifact, skip map[string]bool) []proposalArtifact {
	if !skip["podlock"] {
		return artifacts
	}
	out := make([]proposalArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.slug != "podlock" {
			out = append(out, artifact)
		}
	}
	return out
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

func revalidateAttemptBeforeMutation(ctx context.Context, client dynamic.Interface, namespace, name, uid string, target k8s.GovernedTarget, digest string) error {
	spec, currentUID, err := proposal.GetWithIdentity(ctx, client, namespace, name)
	if err != nil {
		return fmt.Errorf("re-reading proposal before mutation: %w", err)
	}
	if spec == nil || currentUID != uid {
		return fmt.Errorf("proposal UID changed before mutation")
	}
	status, err := proposal.GetStatus(ctx, client, namespace, name)
	if err != nil {
		return fmt.Errorf("re-reading approval before mutation: %w", err)
	}
	if err := proposal.ValidateApprovedCandidate(spec, status); err != nil {
		return fmt.Errorf("authorization changed before mutation: %w", err)
	}
	currentDigest, err := proposal.CandidateDigest(*spec)
	if err != nil || currentDigest != digest {
		if err != nil {
			return fmt.Errorf("computing candidate digest before mutation: %w", err)
		}
		return fmt.Errorf("candidate digest changed before mutation")
	}
	if spec.TargetBinding == nil {
		return fmt.Errorf("canonical target binding disappeared before mutation")
	}
	currentTarget, err := spec.TargetBinding.GovernedTarget(spec.Container)
	if err != nil || !target.Equal(currentTarget) {
		return fmt.Errorf("canonical target binding changed before mutation")
	}
	return nil
}

func mutationRecordFor(p plannedArtifact, before *unstructured.Unstructured) attempt.MutationRecord {
	intended := mutationSnapshot(p.gvk, p.obj)
	beforeValue, beforeDigest := observedValue(before)
	if before != nil {
		beforeValue = string(mutationSnapshotBytes(p.gvk, before))
		beforeDigest = digestJSON([]byte(beforeValue))
	}
	operation := "CREATE"
	uid, resourceVersion := "", ""
	if before != nil {
		operation = "UPDATE"
		uid, resourceVersion = string(before.GetUID()), before.GetResourceVersion()
		if p.gvk.Kind == "Pod" {
			operation = "DELETE_THEN_CREATE"
		}
	}
	return attempt.MutationRecord{
		ID: p.slug, Group: p.gvk.Group, Version: p.gvk.Version, Kind: p.gvk.Kind,
		Namespace: p.ns, Name: p.nameStr, UID: uid, ResourceVersion: resourceVersion,
		Operation: operation, Before: beforeValue, IntendedAfter: string(intended),
		BeforeDigest: beforeDigest, IntendedAfterDigest: digestJSON(intended), Result: attempt.ResultUnknown,
	}
}

func hasSuccessfulMutation(records []attempt.MutationRecord) bool {
	for _, record := range records {
		if record.Result == attempt.ResultSucceeded {
			return true
		}
	}
	return false
}

// mutationSnapshot records the state relevant to a future guarded restore.
// Policy artifacts retain their whole spec; workload artifacts retain only
// the fields this project patches, avoiding a stale whole-object rollback
// payload that could overwrite unrelated operator changes.
func mutationSnapshot(gvk schema.GroupVersionKind, obj *unstructured.Unstructured) []byte {
	if obj == nil {
		return []byte("null")
	}
	snapshot := map[string]interface{}{
		"apiVersion": obj.GetAPIVersion(),
		"kind":       obj.GetKind(),
		"metadata": map[string]interface{}{
			"name":            obj.GetName(),
			"namespace":       obj.GetNamespace(),
			"uid":             string(obj.GetUID()),
			"resourceVersion": obj.GetResourceVersion(),
		},
	}
	if gvk.Kind == "Deployment" || gvk.Kind == "StatefulSet" || gvk.Kind == "DaemonSet" {
		snapshot["spec"] = workloadMutationSpec(obj)
	} else if spec, found, _ := unstructured.NestedFieldCopy(obj.Object, "spec"); found {
		snapshot["spec"] = spec
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		return []byte("null")
	}
	return b
}

func mutationSnapshotBytes(gvk schema.GroupVersionKind, obj *unstructured.Unstructured) []byte {
	return mutationSnapshot(gvk, obj)
}

func workloadMutationSpec(obj *unstructured.Unstructured) map[string]interface{} {
	snapshot := map[string]interface{}{}
	template := map[string]interface{}{}
	if labels, found, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels"); found {
		if profile, ok := labels[podLockProfileLabel]; ok {
			template["metadata"] = map[string]interface{}{"labels": map[string]interface{}{podLockProfileLabel: profile}}
		}
	}
	containers, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if found {
		controlled := make([]interface{}, 0, len(containers))
		for _, raw := range containers {
			container, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			entry := map[string]interface{}{"name": container["name"]}
			if securityContext, ok := container["securityContext"].(map[string]interface{}); ok {
				controlledContext := map[string]interface{}{}
				for _, key := range []string{"capabilities", "seccompProfile"} {
					if value, present := securityContext[key]; present {
						controlledContext[key] = runtime.DeepCopyJSONValue(value)
					}
				}
				if len(controlledContext) > 0 {
					entry["securityContext"] = controlledContext
				}
			}
			controlled = append(controlled, entry)
		}
		template["spec"] = map[string]interface{}{"containers": controlled}
	}
	if len(template) > 0 {
		snapshot["template"] = template
	}
	return snapshot
}

func observedValue(obj *unstructured.Unstructured) (string, string) {
	if obj == nil {
		return "null", digestJSON([]byte("null"))
	}
	b, err := json.Marshal(obj.Object)
	if err != nil {
		return "null", ""
	}
	return string(b), digestJSON(b)
}

func observedMutationValue(gvk schema.GroupVersionKind, obj *unstructured.Unstructured) (string, string) {
	if obj == nil {
		return "null", digestJSON([]byte("null"))
	}
	b := mutationSnapshot(gvk, obj)
	return string(b), digestJSON(b)
}

func digestJSON(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
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

var saveAttemptStatus = attempt.SaveStatus

var createAttempt = attempt.Create

var readApplyResource = k8s.ReadApplyResource

// afterApplyProposalPlanBuilt is a test seam invoked after planned artifacts are
// built (T5) and before proposal reload/revalidation (T7/T8/T9).
var afterApplyProposalPlanBuilt func()

// applyManifest is a test seam for applying one prebuilt artifact payload.
var currentApplyGuard *k8s.ApplyGuard
var applyManifest = func(ctx context.Context, client dynamic.Interface, namespace, content string) error {
	if currentApplyGuard != nil {
		return k8s.ApplyWithGuard(ctx, client, namespace, content, *currentApplyGuard)
	}
	return k8s.Apply(ctx, client, namespace, content)
}
