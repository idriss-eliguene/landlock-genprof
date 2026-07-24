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
// Validated against SPO's real schema
// (sigs.k8s.io/security-profiles-operator/api/seccompprofile/v1beta1's
// Go source, as of 2026-07): SeccompProfileSpec's
// defaultAction/architectures/syscalls[].names/.action json tags are
// identical to pkg/seccomp.Profile/SyscallRule's own — this project's
// existing IR output for the plain kubelet-loaded seccomp.json format
// (internal/exporter/seccomp.ToProfile) is reused directly as Spec here,
// not re-derived (see internal/exporter/spo.ToSeccompProfile). Arch/
// Action enum values (e.g. SCMP_ARCH_X86_64, SCMP_ACT_ALLOW) also match
// exactly — both are libseccomp's own standard token names.
//
// Deliberately NOT mirrored: SPO's real spec additionally supports
// baseProfileName/listenerPath/listenerMetadata/flags (this tool never
// generates any of those — nothing observed maps to them) and a Status
// subresource populated by SPO's own controller after reconciliation
// (irrelevant here, landlock-genprof only ever writes a profile for a
// human to review and kubectl apply, never reads one back — see
// internal/exporter/spo.LocalhostProfilePath for why the eventual
// status.localhostProfile value can still be computed ahead of time).
package spo

// SeccompProfile mirrors SPO's own CRD (apiVersion
// security-profiles-operator.x-k8s.io/v1, kind SeccompProfile).
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

type Metadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
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
