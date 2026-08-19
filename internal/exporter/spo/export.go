// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package spo converts a Behavior IR (internal/profile) into a
// security-profiles-operator (SPO) SeccompProfile custom resource
// (pkg/spo) and serializes it to YAML.
//
// A sibling of internal/exporter/seccomp, not a variant of it: that
// package produces the plain, comment-free seccomp.json format loaded
// directly by the kubelet from a file path (--seccomp-out) — a human has
// to manually copy that file onto every node themselves. This package
// instead produces a directly `kubectl apply`-able Kubernetes object:
// for clusters with SPO
// (https://github.com/kubernetes-sigs/security-profiles-operator)
// installed, its own controller/DaemonSet materializes the profile onto
// every node's seccomp directory automatically once applied, closing
// that manual-copy gap. Reuses internal/exporter/seccomp.ToProfile's
// output directly (confirmed field-for-field identical to SPO's own
// schema, see pkg/spo's own doc comment) rather than a second, parallel
// conversion from the IR.
package spo

import (
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
	"github.com/idriss-eliguene/landlock-genprof/pkg/spo"
)

// The SPO API shape this file emits — version, scope, and the node-local
// profile path — is owned by internal/spobackend, not restated here. An
// earlier version of this comment reasoned only from served API versions
// and concluded no migration was needed; it missed that SeccompProfile
// became cluster-scoped at SPO v0.9.0, which a conversion webhook cannot
// bridge. That is exactly the kind of compatibility claim that should
// exist in one place, so it now lives in the adapter.

// Meta identifies the workload a rendered profile governs. Namespace and
// Container are not part of the SeccompProfile's own identity — it is
// cluster-scoped — but they are what the governed name is derived from and
// what the ownership annotations record, so both are required here.
type Meta struct {
	Namespace string
	Pod       string
	Container string
}

// ProfileName is the cluster-unique name of the SeccompProfile generated
// for meta's workload.
func ProfileName(meta Meta) string {
	return spobackend.GovernedProfileName(meta.Namespace, meta.Pod, meta.Container)
}

// ToSeccompProfile wraps p (see internal/exporter/seccomp.ToProfile) in
// an SPO SeccompProfile manifest, ready to `kubectl apply -f -`.
func ToSeccompProfile(meta Meta, p *seccomp.Profile) *spo.SeccompProfile {
	syscalls := make([]spo.SyscallRule, len(p.Syscalls))
	for i, rule := range p.Syscalls {
		syscalls[i] = spo.SyscallRule{Names: rule.Names, Action: rule.Action}
	}
	return &spo.SeccompProfile{
		APIVersion: spobackend.APIVersion,
		Kind:       spobackend.SeccompProfileKind,
		Metadata: spo.Metadata{
			Name:        ProfileName(meta),
			Annotations: spobackend.OwnershipAnnotations(meta.Namespace, meta.Pod, meta.Container),
		},
		Spec: spo.SeccompProfileSpec{
			DefaultAction: p.DefaultAction,
			Architectures: p.Architectures,
			Syscalls:      syscalls,
		},
	}
}

// ToYAML serializes an SPO SeccompProfile manifest to YAML — the same
// sigs.k8s.io/yaml round-trip internal/exporter/podlock.ToYAML and
// internal/k8s/patch.go already use for directly appliable manifests.
func ToYAML(cr *spo.SeccompProfile) ([]byte, error) {
	out, err := yaml.Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("marshaling SeccompProfile: %w", err)
	}
	return out, nil
}

// LocalhostProfilePath returns the securityContext.seccompProfile.
// localhostProfile value SPO populates in status.localhostProfile once it
// reconciles the profile generated for meta's workload.
//
// Computed rather than left blank, since this tool never waits for SPO's
// reconciliation while generating — it only holds once the generated
// SeccompProfile is applied and SPO is installed. ADR-0007's readiness
// gate is what turns that into a checked precondition at apply time;
// containerd refuses to start a container whose localhostProfile does not
// resolve to a real file on the node.
func LocalhostProfilePath(meta Meta) string {
	return spobackend.LocalhostProfilePath(ProfileName(meta))
}
