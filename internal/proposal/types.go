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
// Spec's fields hold the exact rendered text (YAML/JSON) each exporter's
// own ToYAML/ToJSON already produces for the corresponding local file —
// not a structured sub-spec. First built with structured
// podlock.LandlockProfileSpec/networkingv1.NetworkPolicySpec/
// seccomp.Profile/corev1.SecurityContext fields instead, but live
// testing surfaced the real problem with that: none of those include
// apiVersion/kind/metadata, so none were directly copy-pasteable or
// `kubectl apply -f`-able — defeating the point of a *reviewable*
// artifact. Storing the real rendered text means each field is exactly
// what a human would copy out of `kubectl get securityprofileproposal
// -o yaml` and use as-is.
package proposal

// Spec is a training run's generated multi-domain profile, ready to be
// stored as a SecurityProfileProposal object.
type Spec struct {
	Container   string `json:"container"`
	Binary      string `json:"binary"`
	GeneratedAt string `json:"generatedAt"` // RFC3339
	HistoryUsed bool   `json:"historyUsed"`

	// Each field below holds the exact content the corresponding local
	// file gets (see cmd/landlock-genprof/trace.go's write* functions
	// and publishProposal) — a full, directly usable artifact. Empty
	// string means that domain wasn't generated this run, the same
	// "empty means not generated" convention
	// internal/exporter/report.GeneratedFiles already uses — except
	// PodLock, which is never empty: profile.yaml is always written
	// unconditionally today.
	PodLock         string `json:"podLock,omitempty"`         // full profile.yaml content
	NetworkPolicy   string `json:"networkPolicy,omitempty"`   // full networkpolicy.yaml content
	PatchedManifest string `json:"patchedManifest,omitempty"` // full <identity>-patched.yaml content
	// SPOSeccompProfile is the sole seccomp-related field: its own
	// spec.syscalls already carries the same data the plain seccomp.json
	// format would (see pkg/spo's own doc comment on the field-for-field
	// match), so there's nothing a separate raw-JSON field here would
	// add — --seccomp-out's local file remains available independently,
	// this just isn't duplicated a second time inside the proposal.
	SPOSeccompProfile string `json:"spoSeccompProfile,omitempty"` // full <pod>-seccompprofile.yaml content
}
