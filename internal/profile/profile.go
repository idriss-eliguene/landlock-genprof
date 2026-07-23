// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package profile defines the Behavior IR (intermediate representation):
// the internal, technology-neutral description of what a workload was
// observed doing, independent of any specific output format (PodLock,
// Kubernetes NetworkPolicy, Cilium, ...).
//
// internal/policy builds a BehaviorProfile from raw tracer events;
// exporters (internal/exporter/...) read a BehaviorProfile and produce a
// format-specific artifact. The dependency only ever runs exporter -> IR,
// never the other way — this package must not import a specific output
// format, YAML, or Kubernetes/collector types. deps_test.go enforces this
// statically, since nothing at the Go type level would otherwise catch an
// accidental import the other way.
package profile

// BehaviorProfile is the full observed behavior of a workload during a
// training run.
type BehaviorProfile struct {
	Filesystem FilesystemProfile
	Network    NetworkProfile
	Syscalls   SyscallProfile
}

// FilesystemProfile is the filesystem part of a BehaviorProfile: one
// FileAccess per distinct path, deduplicated and aggregated by
// internal/policy.Synthesize.
type FilesystemProfile struct {
	Accesses []FileAccess
}

// FileAccess records the set of permissions observed on a single path.
// Permissions is a set, not a single joint label (like PodLock's
// "readWriteExec"): collapsing read/write/execute into a
// technology-specific joint category is an exporter's job, not the IR's
// — see internal/exporter/podlock.
type FileAccess struct {
	Path        string
	Permissions []FilePermission
	Confidence  Confidence
	SeenCount   int
}

// HasPermission reports whether p was observed for this access.
func (fa FileAccess) HasPermission(p FilePermission) bool {
	for _, perm := range fa.Permissions {
		if perm == p {
			return true
		}
	}
	return false
}

// NetworkProfile is the network part of a BehaviorProfile: one
// NetworkAccess per distinct (port, direction) pair, aggregated by
// internal/policy.Synthesize.
type NetworkProfile struct {
	Accesses []NetworkAccess
}

// NetworkAccess records a TCP port observed either as a connect (egress)
// or bind (ingress) target — the only two rights the trace_tcpconnect/
// trace_bind gadgets and Landlock's own LANDLOCK_ACCESS_NET_* rights cover
// (see README's gadget table). No Protocol field: TCP is the only thing
// either gadget or Landlock's network rights represent today.
type NetworkAccess struct {
	Port       int
	Direction  NetworkDirection
	Confidence Confidence
	SeenCount  int
}

// NetworkDirection is which side of a TCP handshake a NetworkAccess was
// observed on.
type NetworkDirection string

const (
	DirectionEgress  NetworkDirection = "egress"  // connect()
	DirectionIngress NetworkDirection = "ingress" // bind()
)

// SyscallProfile is the syscall part of a BehaviorProfile: one
// SyscallAccess per distinct syscall name, produced from Inspektor
// Gadget's advise_seccomp gadget (see internal/tracer/trace_linux.go),
// which already reports one deduplicated set of observed syscalls per
// container rather than a per-occurrence stream — so, unlike Filesystem/
// Network, SeenCount here is always 1 within a single run; only
// --history's cross-run accumulation (internal/history) makes Confidence
// meaningful for this domain. See internal/policy.Synthesize.
type SyscallProfile struct {
	Accesses []SyscallAccess
	// Architectures is the seccomp architecture list advise_seccomp
	// reported for the node the training run executed on (e.g.
	// "SCMP_ARCH_X86_64") — a single per-run fact, not per-syscall, so
	// it lives here rather than on each SyscallAccess.
	Architectures []string
}

// SyscallAccess records one syscall observed as allowed for the traced
// container.
type SyscallAccess struct {
	Name       string
	Confidence Confidence
	SeenCount  int
}

// FilePermission is one elementary filesystem right observed on a path.
type FilePermission string

const (
	PermissionRead    FilePermission = "read"
	PermissionWrite   FilePermission = "write"
	PermissionExecute FilePermission = "execute"
)

// Confidence indicates how certain a generated access is, based on how
// many training runs it was observed in.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"    // seen only once
	ConfidenceMedium Confidence = "medium" // seen across multiple runs, inconsistently
	ConfidenceHigh   Confidence = "high"   // seen consistently
)
