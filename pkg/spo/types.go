// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package spo defines the Go types matching the SeccompProfile CRD
// schema of security-profiles-operator (SPO,
// github.com/kubernetes-sigs/security-profiles-operator), so
// landlock-genprof can generate a SeccompProfile directly usable without
// further transformation — same reasoning pkg/podlock already documents
// for PodLock's own CRD.
//
// Which SPO API version and scope these types target is not restated
// here: internal/spobackend owns that, and a compatibility claim in two
// places is a compatibility claim that will disagree with itself. These
// are the field shapes; the adapter decides what they are addressed as.
//
// Deliberately NOT mirrored: SPO's spec additionally supports
// baseProfileName/listenerPath/listenerMetadata/flags, and Syscall
// additionally supports args/errnoRet. This project never generates any
// of them — nothing observed maps to them — and docs/adr/0008 makes their
// absence normative for import, which must refuse a profile carrying an
// enforcement-relevant field these types cannot represent rather than
// drop it. A Status subresource is likewise omitted: it is populated by
// SPO's controller after reconciliation, and the only field this project
// reads back is status.localhostProfile, at apply time, through the
// dynamic client (see ADR-0007's readiness gate).
package spo

// SeccompProfile mirrors SPO's own SeccompProfile CRD. The apiVersion it
// is rendered with comes from internal/spobackend, not from this file.
//
// `json` tags, not `yaml`: serialization goes through sigs.k8s.io/yaml,
// which converts to JSON then to YAML (like the Kubernetes API server
// does) — it silently ignores `yaml:"..."` tags and would fall back to
// the Go field name (e.g. "APIVersion" instead of "apiVersion"). Same
// reasoning pkg/podlock.LandlockProfile's own doc comment gives.
type SeccompProfile struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Metadata   Metadata           `json:"metadata"`
	Spec       SeccompProfileSpec `json:"spec"`
}

// Metadata carries no Namespace: SeccompProfile is cluster-scoped on the
// targeted API (see internal/spobackend). Annotations carry this project's
// ownership marker and the identity tuple the apply path checks before it
// will update an object that already holds a governed name.
type Metadata struct {
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// SeccompProfileSpec mirrors SPO's own SeccompProfileSpec — deliberately
// just the three fields this project ever populates, not SPO's full
// schema (see the package doc for what's omitted and why).
type SeccompProfileSpec struct {
	DefaultAction string        `json:"defaultAction,omitempty"`
	Architectures []string      `json:"architectures,omitempty"`
	Syscalls      []SyscallRule `json:"syscalls,omitempty"`
}

// SyscallRule mirrors SPO's own Syscall type — same shape as
// pkg/seccomp.SyscallRule (see the package doc).
type SyscallRule struct {
	Names  []string `json:"names,omitempty"`
	Action string   `json:"action,omitempty"`
}
