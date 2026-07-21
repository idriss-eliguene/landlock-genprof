// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package podlock defines the Go types matching the LandlockProfile CRD
// schema of the PodLock project (github.com/flavio/podlock, Kubewarden
// ecosystem), so that landlock-genprof generates profiles that are
// directly usable without further transformation.
//
// TODO(M2): validate these types against PodLock's real schema at
// implementation time (the format may evolve) — see
// https://github.com/flavio/podlock
package podlock

// LandlockProfile mirrors the PodLock CRD.
//
// `json` tags, not `yaml`: serialization goes through sigs.k8s.io/yaml,
// which converts to JSON then to YAML (like the Kubernetes API server
// does) — it silently ignores `yaml:"..."` tags and would fall back to
// the Go field name (e.g. "APIVersion" instead of "apiVersion").
type LandlockProfile struct {
	APIVersion string              `json:"apiVersion"`
	Kind       string              `json:"kind"`
	Metadata   Metadata            `json:"metadata"`
	Spec       LandlockProfileSpec `json:"spec"`
}

type Metadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type LandlockProfileSpec struct {
	// ProfilesByContainer: container name -> binary path -> restrictions
	ProfilesByContainer map[string]map[string]BinaryProfile `json:"profilesByContainer"`
}

type BinaryProfile struct {
	ReadExec  []string `json:"readExec,omitempty"`
	ReadOnly  []string `json:"readOnly,omitempty"`
	ReadWrite []string `json:"readWrite,omitempty"`
}
