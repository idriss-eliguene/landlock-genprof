// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package proposalapp contains application orchestration for generating a
// SecurityProfileProposal from a completed Observation. The history and
// proposal packages remain the semantic authorities for contribution and
// candidate generation.
package proposalapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	observationdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"k8s.io/client-go/dynamic"
)

type GenerateResult struct {
	ProposalName     string
	CandidateVersion string
	Scope            string
	Target           string
	Container        string
	ImageIdentity    string
	Approved         bool
}

func Generate(ctx context.Context, client dynamic.Interface, namespace, observationID, proposalName string, load func(context.Context, string, string) (observationdomain.Observation, error)) (GenerateResult, error) {
	if client == nil {
		return GenerateResult{}, fmt.Errorf("proposal generation requires a Kubernetes client")
	}
	if load == nil {
		return GenerateResult{}, fmt.Errorf("proposal generation requires an Observation loader")
	}
	observation, err := load(ctx, namespace, observationID)
	if err != nil {
		return GenerateResult{}, err
	}
	if !observation.Frozen() || observation.Execution().State != observationdomain.ExecutionCompleted {
		return GenerateResult{}, fmt.Errorf("conflict: Observation is not a completed frozen result")
	}
	contribution, err := history.ContributionFromObservation(observation)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("conflict: Observation is not eligible: %w", err)
	}
	if _, err := history.ApplyObservationContribution(ctx, client, namespace, observation); err != nil {
		return GenerateResult{}, fmt.Errorf("contributing Observation: %w", err)
	}
	identity, err := contribution.Population.Identity()
	if err != nil {
		return GenerateResult{}, err
	}
	spec, err := proposal.GenerateContainerCapabilityProposal(ctx, client, namespace, identity, proposalName)
	if err != nil {
		if errors.Is(err, proposal.ErrNoCandidate) {
			return GenerateResult{}, fmt.Errorf("no candidate: %w", err)
		}
		return GenerateResult{}, fmt.Errorf("generating proposal: %w", err)
	}
	return GenerateResult{ProposalName: proposalName, CandidateVersion: spec.CandidateVersion, Scope: spec.Subject.Scope, Target: spec.Subject.Target, Container: spec.Subject.Container, ImageIdentity: spec.Subject.ImageIdentity, Approved: false}, nil
}
