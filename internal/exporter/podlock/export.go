// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package podlock converts a Behavior IR (internal/profile) into the
// PodLock format (pkg/podlock) and serializes it to YAML.
//
// This is the only package that depends on both internal/profile and
// pkg/podlock — the dependency runs exporter -> IR, never the other way
// (see docs/architecture.md): internal/profile itself has no notion that
// PodLock exists, so this package is free to change or be joined by
// siblings (a Kubernetes NetworkPolicy exporter, Cilium, ...) without
// internal/profile or internal/policy ever needing to know about any of
// them.
package podlock

import (
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
)

// ProfileMeta identifies the pod/container/binary a BehaviorProfile
// applies to. Binary (the observed entry point's path) is specific to
// how PodLock indexes its rules — a future exporter (e.g. a Kubernetes
// NetworkPolicy one) would key its own output differently (podSelector,
// labels, ...), so this stays local to this package rather than becoming
// a shared, premature abstraction across exporters that don't exist yet.
type ProfileMeta struct {
	Name      string // name of the generated LandlockProfile (usually the pod's name)
	Namespace string
	Container string
	Binary    string // path of the container's main binary (observed entry point)
}

// ToProfile converts a BehaviorProfile's filesystem observations into a
// LandlockProfile ready to be serialized, in the format consumed by the
// PodLock operator. Only one profilesByContainer entry is produced
// (meta.Container -> meta.Binary), since a training run targets a single
// container at a time. Domains PodLock doesn't support (today: network)
// are simply never read from the IR — see docs/policy-synthesis.md.
func ToProfile(meta ProfileMeta, fs profile.FilesystemProfile) *podlock.LandlockProfile {
	var bp podlock.Profile
	for _, access := range fs.Accesses {
		switch categoryFor(access) {
		case "readExec":
			bp.ReadExec = append(bp.ReadExec, access.Path)
		case "readOnly":
			bp.ReadOnly = append(bp.ReadOnly, access.Path)
		case "readWrite":
			bp.ReadWrite = append(bp.ReadWrite, access.Path)
		case "readWriteExec":
			bp.ReadWriteExec = append(bp.ReadWriteExec, access.Path)
		}
	}

	return &podlock.LandlockProfile{
		APIVersion: "podlock.kubewarden.io/v1alpha1",
		Kind:       "LandlockProfile",
		Metadata: podlock.Metadata{
			Name:      meta.Name,
			Namespace: meta.Namespace,
		},
		Spec: podlock.LandlockProfileSpec{
			ProfilesByContainer: map[string]podlock.ProfileByBinary{
				meta.Container: {
					meta.Binary: bp,
				},
			},
		},
	}
}

// categoryFor maps an IR permission set to exactly one of PodLock's four
// access categories (see pkg/podlock.Profile) — not a combination of
// several. A path that's both executed and written to is
// "readWriteExec", a distinct category of its own, not "readExec" and
// "readWrite" reported side by side: that mismatch was caught by
// checking PodLock's real schema (github.com/flavio/podlock), which
// mirrors what Landlock itself groups as one enforcement decision. Every
// named category also implies read access — there's no "execute but not
// read" bucket in PodLock's schema, matching the practical reality that
// executing or writing a file requires reading it first.
func categoryFor(access profile.FileAccess) string {
	switch {
	case access.HasPermission(profile.PermissionExecute) && access.HasPermission(profile.PermissionWrite):
		return "readWriteExec"
	case access.HasPermission(profile.PermissionExecute):
		return "readExec"
	case access.HasPermission(profile.PermissionWrite):
		return "readWrite"
	case access.HasPermission(profile.PermissionRead):
		return "readOnly"
	default:
		return ""
	}
}

// ToYAML serializes a LandlockProfile to YAML, as written to profile.yaml
// by the CLI (see cmd/landlock-genprof).
func ToYAML(p *podlock.LandlockProfile) ([]byte, error) {
	return yaml.Marshal(p)
}
