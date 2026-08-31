// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package association contains pure, fail-closed relationships between
// canonical workload targets and explicitly supplied source provenance.
// It intentionally does not read or mutate Kubernetes objects.
package association

import (
	"fmt"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

type State string

const (
	Associated             State = "ASSOCIATED"
	Unassociated           State = "UNASSOCIATED"
	Ambiguous              State = "AMBIGUOUS"
	Stale                  State = "STALE"
	Orphaned               State = "ORPHANED"
	InsufficientProvenance State = "INSUFFICIENT_PROVENANCE"
)

// Result records both the association state and the bounded reason for it.
// Association is not approval, application, enforcement, or verification.
type Result struct {
	State  State
	Reason string
}

// Evidence is a read-side source. Target is nil for current v0.4
// populations: their persisted Target is a legacy string and cannot be
// safely reconstructed into a canonical GovernedTarget.
type Evidence struct {
	Target     *k8s.GovernedTarget
	Population history.Population
}

// Proposal is a read-side source. Target is nil for the current proposal
// schema because proposal.Spec contains rendered artifacts, not a canonical
// workload identity.
type Proposal struct {
	Namespace string
	Name      string
	Target    *k8s.GovernedTarget
	Spec      proposal.Spec
	Status    *proposal.Status
}

type RuntimeCompatibilityState string

const (
	RuntimeMatches RuntimeCompatibilityState = "MATCH"
	RuntimeDiffers RuntimeCompatibilityState = "DIFFERENT_POPULATION"
	RuntimeUnknown RuntimeCompatibilityState = "UNKNOWN"
)

type RuntimeCompatibility struct {
	State  RuntimeCompatibilityState
	Reason string
}

// AssociateEvidence compares only explicit canonical target identity. It
// does not use legacy strings, names, labels, ImageID, binary path, counts,
// confidence, coverage, or PodUID to repair missing target provenance.
func AssociateEvidence(target k8s.GovernedTarget, source Evidence) Result {
	return associate(target, source.Target, "evidence")
}

// AssociateProposal compares only an explicit canonical target binding.
// Proposal name, candidate digest, approval state, and rendered artifacts do
// not establish workload identity.
func AssociateProposal(target k8s.GovernedTarget, source Proposal) Result {
	return associate(target, source.Target, "proposal")
}

// AssociateEvidenceToTargets classifies a source against a discovered set.
// A complete source with no current target is orphaned; incomplete source is
// insufficient, never orphaned. Matching is independent of input order.
func AssociateEvidenceToTargets(source Evidence, targets []k8s.GovernedTarget) Result {
	return associateToTargets(source.Target, targets, "evidence")
}

func AssociateProposalToTargets(source Proposal, targets []k8s.GovernedTarget) Result {
	return associateToTargets(source.Target, targets, "proposal")
}

func associate(target k8s.GovernedTarget, source *k8s.GovernedTarget, kind string) Result {
	if !validTarget(target) || source == nil || !validTarget(*source) {
		return Result{State: InsufficientProvenance, Reason: fmt.Sprintf("%s lacks a complete canonical target", kind)}
	}
	if target.Equal(*source) {
		return Result{State: Associated, Reason: fmt.Sprintf("%s has an explicit canonical target binding", kind)}
	}
	return Result{State: Unassociated, Reason: fmt.Sprintf("%s has a different canonical target", kind)}
}

func associateToTargets(source *k8s.GovernedTarget, targets []k8s.GovernedTarget, kind string) Result {
	if source == nil || !validTarget(*source) {
		return Result{State: InsufficientProvenance, Reason: fmt.Sprintf("%s lacks a complete canonical target", kind)}
	}
	seen := make(map[k8s.GovernedTarget]struct{})
	matches := 0
	for _, target := range targets {
		if !validTarget(target) {
			continue
		}
		if target.Equal(*source) {
			if _, exists := seen[target]; !exists {
				seen[target] = struct{}{}
				matches++
			}
		}
	}
	switch matches {
	case 0:
		return Result{State: Orphaned, Reason: fmt.Sprintf("complete %s target is absent from discovered targets", kind)}
	case 1:
		return Result{State: Associated, Reason: fmt.Sprintf("%s has one matching canonical target", kind)}
	default:
		return Result{State: Ambiguous, Reason: fmt.Sprintf("%s matches multiple canonical targets", kind)}
	}
}

// CompareRuntimePopulation is deliberately separate from target association.
// Empty ImageID is unknown, never equal to another empty ImageID. BinaryPath
// and ImageID identify an evidence population's compatibility with a runtime;
// neither changes GovernedTarget identity.
func CompareRuntimePopulation(source Evidence, subject k8s.RuntimeSubject) RuntimeCompatibility {
	if source.Target == nil || !validTarget(*source.Target) || !subject.Target.Equal(*source.Target) {
		return RuntimeCompatibility{State: RuntimeUnknown, Reason: "target association is not established"}
	}
	if source.Population.ImageIdentity == "" || subject.ImageID == "" || source.Population.BinaryPath == "" || subject.BinaryPath == "" {
		return RuntimeCompatibility{State: RuntimeUnknown, Reason: "image identity or binary path is unknown"}
	}
	if source.Population.ImageIdentity != subject.ImageID || source.Population.BinaryPath != subject.BinaryPath {
		return RuntimeCompatibility{State: RuntimeDiffers, Reason: "runtime does not match the evidence population"}
	}
	return RuntimeCompatibility{State: RuntimeMatches, Reason: "runtime matches the evidence population identity fields"}
}

func validTarget(target k8s.GovernedTarget) bool {
	return target.Namespace != "" && target.Workload.Kind != "" && target.Workload.Name != "" && target.Container != ""
}
