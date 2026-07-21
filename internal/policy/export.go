// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package policy

import (
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/pkg/podlock"
)

// ProfileMeta identifies the pod/container/binary a set of synthesized
// rules applies to — Rule has no notion of container or binary (see
// docs/architecture.md); it's the caller (typically the CLI, from
// internal/k8s.TargetPod and the trace options) that supplies this
// context at export time.
type ProfileMeta struct {
	Name      string // name of the generated LandlockProfile (usually the pod's name)
	Namespace string
	Container string
	Binary    string // path of the container's main binary (observed entry point)
}

// ToProfile assembles synthesized rules (Synthesize) into a
// LandlockProfile ready to be serialized, in the format consumed by the
// PodLock operator. Only one profilesByContainer entry is produced
// (meta.Container -> meta.Binary), since a training run targets a single
// container at a time.
func ToProfile(meta ProfileMeta, rules []Rule) *podlock.LandlockProfile {
	var bp podlock.BinaryProfile
	for _, r := range rules {
		for _, access := range r.Access {
			switch access {
			case "readExec":
				bp.ReadExec = append(bp.ReadExec, r.Path)
			case "readOnly":
				bp.ReadOnly = append(bp.ReadOnly, r.Path)
			case "readWrite":
				bp.ReadWrite = append(bp.ReadWrite, r.Path)
			}
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
			ProfilesByContainer: map[string]map[string]podlock.BinaryProfile{
				meta.Container: {
					meta.Binary: bp,
				},
			},
		},
	}
}

// ToYAML serializes a LandlockProfile to YAML, as written to profile.yaml
// by the CLI (see cmd/landlock-genprof).
func ToYAML(profile *podlock.LandlockProfile) ([]byte, error) {
	return yaml.Marshal(profile)
}
