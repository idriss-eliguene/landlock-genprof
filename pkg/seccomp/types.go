// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package seccomp defines the Go types matching the standard OCI runtime-
// spec / Kubernetes "localhost" seccomp profile JSON format (see
// https://kubernetes.io/docs/tutorials/security/seccomp/), so that
// landlock-genprof generates a profile directly usable without further
// transformation — same reasoning as pkg/podlock, and confirmed against
// the exact shape Inspektor Gadget's own advise_seccomp gadget produces
// (gadgets/advise_seccomp/README.mdx, vendored SDK v0.54.1): this schema
// is small and stable enough that a hand-rolled type is safer than pulling
// in a dependency for it.
package seccomp

// Profile is a seccomp profile document, as loaded by the kubelet from a
// file (referenced via a pod's securityContext.seccompProfile.
// localhostProfile) or by a container runtime directly — never applied
// via `kubectl apply` the way a NetworkPolicy or LandlockProfile is, which
// is why this package serializes to plain JSON, not YAML (see
// internal/exporter/seccomp.ToJSON).
type Profile struct {
	DefaultAction string        `json:"defaultAction"`
	Architectures []string      `json:"architectures"`
	Syscalls      []SyscallRule `json:"syscalls"`
}

// SyscallRule groups a set of syscall names under a single seccomp action.
type SyscallRule struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
}
