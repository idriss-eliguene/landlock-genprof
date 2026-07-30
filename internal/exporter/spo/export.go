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

	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
	"github.com/idriss-eliguene/landlock-genprof/pkg/spo"
)

// apiVersion is SeccompProfile's actual served CRD version —
// confirmed live (2026-07-30) against a real SPO v0.7.1 install:
// `kubectl get crd seccompprofiles.security-profiles-operator.x-k8s.io
// -o jsonpath='{.spec.versions[*].name}'` returns only `v1beta1`, and
// the same is true of the CRD manifest in SPO's own v0.7.1 tag
// (deploy/base-crds/crds/seccompprofile.yaml). The `v1` this constant
// used to hold was wrong — every SeccompProfile this project ever
// generated with it would fail to apply against a real cluster with
// "the server could not find the requested resource", not a cosmetic
// difference. Re-check against
// https://github.com/kubernetes-sigs/security-profiles-operator's own
// installation-usage.md before assuming this has been promoted to v1
// for real in some future SPO release.
const apiVersion = "security-profiles-operator.x-k8s.io/v1beta1"

// Meta identifies the SeccompProfile object a rendered profile is
// wrapped in — SPO's SeccompProfile is namespaced, so Namespace matters
// (also used by LocalhostProfilePath below).
type Meta struct {
	Name      string
	Namespace string
}

// ToSeccompProfile wraps p (see internal/exporter/seccomp.ToProfile) in
// an SPO SeccompProfile manifest, ready to `kubectl apply -f -`.
func ToSeccompProfile(meta Meta, p *seccomp.Profile) *spo.SeccompProfile {
	syscalls := make([]spo.SyscallRule, len(p.Syscalls))
	for i, rule := range p.Syscalls {
		syscalls[i] = spo.SyscallRule{Names: rule.Names, Action: rule.Action}
	}
	return &spo.SeccompProfile{
		APIVersion: apiVersion,
		Kind:       "SeccompProfile",
		Metadata:   spo.Metadata{Name: meta.Name, Namespace: meta.Namespace},
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
// localhostProfile value SPO's own controller will populate once it
// reconciles a SeccompProfile named meta.Name in meta.Namespace — the
// fixed "operator/<name>.json" convention SPO always uses,
// confirmed against SPO's own installation-usage.md and its
// SeccompProfileStatus.LocalhostProfile field comment ("the path that
// should be provided to the securityContext.seccompProfile.
// localhostProfile field"). Computed here rather than left blank, since
// this tool never waits for SPO's own reconciliation to actually run —
// it only holds if the generated SeccompProfile is applied and SPO is
// installed in the cluster.
func LocalhostProfilePath(meta Meta) string {
	return fmt.Sprintf("operator/%s.json", meta.Name)
}
