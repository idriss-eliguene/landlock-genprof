// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package attempt persists durable custody for one governed apply execution.
package attempt

import (
	"fmt"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

const (
	StateInProgress       = "IN_PROGRESS"
	StateApplied          = "APPLIED"
	StatePartiallyApplied = "PARTIALLY_APPLIED"
	StateFailed           = "FAILED"
	StateOutcomeUnknown   = "OUTCOME_UNKNOWN"

	ResultSucceeded = "SUCCEEDED"
	ResultFailed    = "FAILED"
	ResultUnknown   = "OUTCOME_UNKNOWN"
)

// Spec is immutable authorization context captured when an attempt starts.
type Spec struct {
	ProposalNamespace       string             `json:"proposalNamespace"`
	ProposalName            string             `json:"proposalName"`
	ProposalUID             string             `json:"proposalUID"`
	ApprovedCandidateDigest string             `json:"approvedCandidateDigest"`
	Target                  k8s.GovernedTarget `json:"target"`
	StartedAt               string             `json:"startedAt"`
	OperatorIdentity        string             `json:"operatorIdentity,omitempty"`
}

// MutationRecord is append-only custody for one planned mutation.
type MutationRecord struct {
	ID                  string `json:"id"`
	Group               string `json:"group,omitempty"`
	Version             string `json:"version"`
	Kind                string `json:"kind"`
	Namespace           string `json:"namespace,omitempty"`
	Name                string `json:"name"`
	UID                 string `json:"uid,omitempty"`
	ResourceVersion     string `json:"resourceVersion,omitempty"`
	Operation           string `json:"operation"`
	Before              string `json:"before"`
	IntendedAfter       string `json:"intendedAfter"`
	ObservedAfter       string `json:"observedAfter,omitempty"`
	BeforeDigest        string `json:"beforeDigest,omitempty"`
	IntendedAfterDigest string `json:"intendedAfterDigest,omitempty"`
	ObservedAfterDigest string `json:"observedAfterDigest,omitempty"`
	Result              string `json:"result"`
	Error               string `json:"error,omitempty"`
}

// Status is the durable lifecycle and per-resource custody.
type Status struct {
	State       string           `json:"state"`
	UpdatedAt   string           `json:"updatedAt"`
	CompletedAt string           `json:"completedAt,omitempty"`
	Failure     string           `json:"failure,omitempty"`
	Mutations   []MutationRecord `json:"mutations,omitempty"`
}

// Validate rejects unknown lifecycle/result values before they become
// durable custody. OUTCOME_UNKNOWN is intentionally a distinct valid state.
func (s Status) Validate() error {
	switch s.State {
	case StateInProgress, StateApplied, StatePartiallyApplied, StateFailed, StateOutcomeUnknown:
	default:
		return fmt.Errorf("invalid ApplyAttempt state %q", s.State)
	}
	for _, mutation := range s.Mutations {
		switch mutation.Result {
		case ResultSucceeded, ResultFailed, ResultUnknown:
		default:
			return fmt.Errorf("invalid ApplyAttempt mutation result %q", mutation.Result)
		}
	}
	return nil
}
