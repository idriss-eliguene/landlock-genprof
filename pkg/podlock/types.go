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
// Validated against PodLock's real schema (api/v1alpha1/landlockprofile_types.go
// as of 2026-07): Spec, ProfileByBinary, and Profile below mirror it
// field-for-field, including the naming — this found two real gaps our
// earlier guess had missed, ReadWriteExec (a fourth, distinct category,
// not just ReadExec+ReadWrite reported separately) and the fact that
// PodLock has no network fields at all (see internal/policy's decision to
// not synthesize network rules, docs/policy-synthesis.md).
//
// Deliberately NOT mirrored: real PodLock embeds full metav1.TypeMeta/
// ObjectMeta and a Status subresource (Conditions, etc.) — irrelevant
// here since landlock-genprof only ever writes a profile for a human to
// review and `kubectl apply`, never reads one back. Metadata below stays
// a deliberately minimal Name/Namespace, which is all a freshly generated
// manifest needs.
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
	ProfilesByContainer map[string]ProfileByBinary `json:"profilesByContainer,omitempty"`
}

// ProfileByBinary maps a binary path to its restrictions, matching
// PodLock's own named type of the same name (not just an anonymous map).
type ProfileByBinary map[string]Profile

// Profile lists the filesystem paths granted each access category.
// PodLock has no field for Landlock's network rights
// (LANDLOCK_ACCESS_NET_BIND_TCP / CONNECT_TCP) — see the package doc.
type Profile struct {
	ReadOnly      []string `json:"readOnly,omitempty"`
	ReadWrite     []string `json:"readWrite,omitempty"`
	ReadExec      []string `json:"readExec,omitempty"`
	ReadWriteExec []string `json:"readWriteExec,omitempty"`
}
