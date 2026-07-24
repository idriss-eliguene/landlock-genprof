// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package proposal persists a training run's generated multi-domain
// profile as a SecurityProfileProposal custom resource (see
// internal/proposal/store.go), so it can be reviewed via kubectl/GitOps
// instead of only as local files.
//
// This is the first slice of a three-stage evidence/proposal/approved-
// policy model discussed for this project: TrainingHistory is the
// evidence stage (already built, no controller — see internal/history),
// SecurityProfileProposal is this one (still no controller: publishing a
// snapshot is simple CRUD, the same reasoning that kept TrainingHistory
// controller-free), and an eventual WorkloadSecurityProfile + operator
// (deliberately not part of this change) would be the approved-policy /
// enforcement stage — the one stage that genuinely needs a reconciliation
// loop, since keeping applied resources from drifting is what operators
// are actually for.
//
// Unlike internal/history's Record (a hand-rolled, k8s-agnostic type),
// Spec's own fields are real Kubernetes/PodLock/seccomp API types
// already used unchanged elsewhere in this codebase
// (pkg/podlock.LandlockProfileSpec, k8s.io/api/networking/v1.
// NetworkPolicySpec, pkg/seccomp.Profile, k8s.io/api/core/v1.
// SecurityContext) — there's no "pure Go, no k8s imports" boundary to
// keep here the way internal/history/record.go keeps one, since this
// package's whole job is storing what the exporters already produced,
// not a novel transformation of the IR.
package proposal

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
)

// Spec is a training run's generated multi-domain profile, ready to be
// stored as a SecurityProfileProposal object. Each pointer field is nil
// when that domain had nothing to report this run — the same "empty
// means not generated" convention internal/exporter/report.GeneratedFiles
// already established — so the stored object never claims an empty/
// misleading sub-object exists.
type Spec struct {
	Container   string `json:"container"`
	Binary      string `json:"binary"`
	GeneratedAt string `json:"generatedAt"` // RFC3339
	HistoryUsed bool   `json:"historyUsed"`

	PodLock         *podlock.LandlockProfileSpec    `json:"podLock,omitempty"`
	NetworkPolicy   *networkingv1.NetworkPolicySpec `json:"networkPolicy,omitempty"`
	Seccomp         *seccomp.Profile                `json:"seccomp,omitempty"`
	SecurityContext *corev1.SecurityContext         `json:"securityContext,omitempty"`
}
