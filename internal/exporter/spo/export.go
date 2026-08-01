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
//
// Re-checked (2026-08-01), prompted by SPO's "v1 stable APIs" CNCF blog
// post (2026-06-26): `v1` for SeccompProfile did ship for real, but only
// as of the SPO v1.0.0 tag specifically — confirmed by pulling the
// SeccompProfile CRD manifest at v0.8.4/v0.9.0/v0.10.0/v1.0.0 directly
// from SPO's repo (bundle/manifests/…_seccompprofiles.yaml) and grepping
// each for its versions list. v0.8.4 through v0.10.0 (everything this
// project has ever targeted) still serve only `v1beta1`; `v1` only
// appears at v1.0.0, alongside `v1beta1` still served (not storage, so
// still applyable via SPO's own conversion webhook). No change needed
// here while pinned to v0.8.4 — revisit only if/when this project
// upgrades its targeted SPO version to v1.0.0 or later.
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
// "operator/<namespace>/<name>.json" convention SPO actually uses.
//
// Confirmed live (2026-07-30) against a real SPO v0.7.1 reconciliation:
// `kubectl get seccompprofile nginx-demo -o yaml` (namespace default)
// reported `status.localhostProfile: operator/default/nginx-demo.json`
// — the namespace segment used to be missing here ("operator/<name>.json"
// only), which meant every patched manifest this tool ever generated
// referenced a path SPO never actually writes to, breaking the target
// pod outright once the SeccompProfile CR was applied: containerd
// refuses to start a container whose securityContext.seccompProfile.
// localhostProfile doesn't resolve to a real file on the node
// ("cannot load seccomp profile ...: no such file or directory").
// Computed here rather than left blank, since this tool never waits for
// SPO's own reconciliation to actually run — it only holds if the
// generated SeccompProfile is applied and SPO is installed in the
// cluster.
func LocalhostProfilePath(meta Meta) string {
	return fmt.Sprintf("operator/%s/%s.json", meta.Namespace, meta.Name)
}
